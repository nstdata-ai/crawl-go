package crawl_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode"
)

func moduleConsumerEnv() []string {
	var environment []string
	for _, name := range []string{"PATH", "HOME", "TMPDIR", "SystemRoot", "GOCACHE"} {
		if value, ok := os.LookupEnv(name); ok {
			environment = append(environment, name+"="+value)
		}
	}
	return append(environment, "GOENV=off", "GOWORK=off", "GOPROXY=off", "GOSUMDB=off",
		"GOTOOLCHAIN=local", "GOFLAGS=", "GOPRIVATE=", "GONOPROXY=", "GONOSUMDB=")
}

func TestReadmeExamplesBuildAsModuleConsumers(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	var first string
	for _, name := range []string{"README.md", "README_zh.md"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			match := regexp.MustCompile("(?s)```go\\n(.*?)\\n```").FindSubmatch(data)
			if len(match) != 2 {
				t.Fatal("missing Go quick start")
			}
			source := string(match[1])
			if first == "" {
				first = source
			} else if source != first {
				t.Fatal("bilingual quick starts differ")
			}
			dir := t.TempDir()
			module := "module example.com/crawl-consumer\n\ngo 1.20\n\nrequire github.com/nstdata-ai/crawl-go v1.0.0\nreplace github.com/nstdata-ai/crawl-go => " + strconv.Quote(root) + "\n"
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(module), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-mod=readonly", "-o", filepath.Join(dir, "consumer"), ".")
			cmd.Dir = dir
			cmd.Env = moduleConsumerEnv()
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("installed consumer: %v\n%s", err, output)
			}
		})
	}
}

func TestReadmeQuickStartsRunWithActualClient(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"README.md", "README_zh.md"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			program := regexp.MustCompile("(?s)```go\\n(.*?)\\n```").FindSubmatch(data)
			if len(program) != 2 {
				t.Fatal("missing Go quick start")
			}
			var runArguments []string
			for _, block := range regexp.MustCompile("(?s)```sh\\n(.*?)\\n```").FindAllSubmatch(data, -1) {
				arguments := strings.Fields(string(block[1]))
				if reflect.DeepEqual(arguments, []string{"go", "run", "."}) {
					runArguments = arguments[1:]
				}
			}
			if runArguments == nil {
				t.Fatal("missing native Go quick-start command")
			}
			dir := t.TempDir()
			module := "module example.com/crawl-consumer\n\ngo 1.20\n\nrequire github.com/nstdata-ai/crawl-go v1.0.0\nreplace github.com/nstdata-ai/crawl-go => " + strconv.Quote(root) + "\n"
			for filename, body := range map[string][]byte{
				"go.mod": []byte(module), "main.go": program[1], "fixture.go": []byte(readmeTransportFixture),
			} {
				if err := os.WriteFile(filepath.Join(dir, filename), body, 0600); err != nil {
					t.Fatal(err)
				}
			}
			for _, scenario := range []string{"inline", "empty", "reference", "truncated", "failed", "pending", "missing", "http", "no-token"} {
				t.Run(scenario, func(t *testing.T) {
					var posts, reads atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
						if request.Header.Get("x-api-key") != "fixture-key" {
							t.Error("unexpected authentication")
						}
						switch request.Method + " " + request.URL.Path {
						case "POST /api/v1/crawl/scrape":
							posts.Add(1)
							var body struct {
								URL             string
								Formats         []string
								OnlyMainContent bool
							}
							if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body.URL != "https://example.com/" || !reflect.DeepEqual(body.Formats, []string{"markdown"}) || !body.OnlyMainContent || request.URL.RawQuery != "" {
								t.Error("unexpected single-page input")
							}
							if scenario == "http" {
								w.WriteHeader(http.StatusServiceUnavailable)
								fmt.Fprint(w, "fixture-provider-private")
								return
							}
							status, success := "completed", true
							content := map[string]interface{}{"markdown": "fixture body"}
							switch scenario {
							case "empty":
								content["markdown"] = ""
							case "reference":
								content = map[string]interface{}{"markdownRef": "fixture-ref"}
							case "truncated":
								content["truncated"] = true
							case "failed":
								status, success, content = "failed", false, nil
							case "pending":
								status = "processing"
							case "missing":
								content = map[string]interface{}{}
							}
							json.NewEncoder(w).Encode(map[string]interface{}{"code": 200, "err": false, "data": map[string]interface{}{"status": status, "success": success, "data": content}})
						case "GET /api/v1/crawl/storage/read":
							reads.Add(1)
							if scenario != "reference" || request.URL.RawQuery != "st=fixture-ref" {
								t.Error("unexpected reference input")
							}
							fmt.Fprint(w, "fixture body")
						default:
							t.Error("unexpected request")
							http.NotFound(w, request)
						}
					}))
					defer server.Close()
					ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
					defer cancel()
					command := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin", "go"), runArguments...)
					command.Dir = dir
					command.Env = append(moduleConsumerEnv(), "NSTDATA_TEST_ORIGIN="+server.URL)
					if scenario != "no-token" {
						command.Env = append(command.Env, "NSTDATA_API_TOKEN=fixture-key")
					}
					var stdout, stderr bytes.Buffer
					command.Stdout, command.Stderr = &stdout, &stderr
					err := command.Run()
					success := scenario == "inline" || scenario == "empty" || scenario == "reference" || scenario == "truncated"
					if ctx.Err() != nil || (err == nil) != success {
						t.Fatalf("quick start exit: %v, stderr=%s", err, stderr.String())
					}
					wantOutput := ""
					if success {
						wantOutput = "fixture body\n"
						if scenario == "empty" {
							wantOutput = "\n"
						}
					}
					if stdout.String() != wantOutput {
						t.Fatalf("quick start output: %q", stdout.String())
					}
					wantPosts, wantReads := int32(1), int32(0)
					if scenario == "no-token" {
						wantPosts = 0
					}
					if scenario == "reference" {
						wantReads = 1
					}
					if posts.Load() != wantPosts || reads.Load() != wantReads {
						t.Fatalf("unexpected request count: POST=%d GET=%d", posts.Load(), reads.Load())
					}
					if strings.Contains(stderr.String(), "fixture-key") || strings.Contains(stderr.String(), "fixture-provider-private") {
						t.Fatal("quick start exposed credentials or provider response")
					}
					if scenario == "truncated" && !strings.Contains(stderr.String(), "content is truncated") {
						t.Fatal("quick start omitted service truncation")
					}
				})
			}
		})
	}
}

const readmeTransportFixture = `package main

import (
 "context"
 "errors"
 "net"
 "net/http"
 "net/url"
 "os"
)

type fixtureTransport struct {
 origin *url.URL
 local *http.Transport
}

func (transport fixtureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
 known := request.Method == "POST" && request.URL.Path == "/api/v1/crawl/scrape"
 known = known || request.Method == "GET" && request.URL.Path == "/api/v1/crawl/storage/read"
 if request.URL.Scheme != "https" || request.URL.Host != "api.nstdata.io" || !known {
  return nil, errors.New("fixture blocked an unexpected destination")
 }
 mapped := request.Clone(request.Context())
 mapped.URL.Scheme, mapped.URL.Host, mapped.Host = transport.origin.Scheme, transport.origin.Host, transport.origin.Host
 return transport.local.RoundTrip(mapped)
}

func init() {
 origin, err := url.Parse(os.Getenv("NSTDATA_TEST_ORIGIN"))
 if err != nil || origin.Scheme != "http" || origin.Hostname() != "127.0.0.1" {
  panic("invalid loopback fixture")
 }
 local := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
  if address != origin.Host { return nil, errors.New("fixture blocked an unexpected connection") }
  return (&net.Dialer{}).DialContext(ctx, network, address)
 }}
 http.DefaultTransport = fixtureTransport{origin: origin, local: local}
 blocked, _ := http.NewRequest("GET", "https://unexpected.invalid/", nil)
 if _, err := http.DefaultTransport.RoundTrip(blocked); err == nil {
  panic("fixture network guard failed")
 }
}
`

func TestDocumentationLinksAndBilingualPrograms(t *testing.T) {
	code := regexp.MustCompile("(?s)```go\\n(.*?)\\n```")
	shell := regexp.MustCompile("(?s)```sh\\n(.*?)\\n```")
	for _, pair := range [][2]string{{"README.md", "README_zh.md"}, {"docs/guide.md", "docs/guide_zh.md"}} {
		left, err := os.ReadFile(pair[0])
		if err != nil {
			t.Fatal(err)
		}
		right, err := os.ReadFile(pair[1])
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(code.FindAllSubmatch(left, -1), code.FindAllSubmatch(right, -1)) || !reflect.DeepEqual(shell.FindAllSubmatch(left, -1), shell.FindAllSubmatch(right, -1)) {
			t.Fatalf("bilingual programs differ: %v", pair)
		}
	}
	links := regexp.MustCompile(`\[[^\]\n]+\]\(([^)\n]+)\)`)
	headings := regexp.MustCompile(`(?m)^#+ (.+)$`)
	for _, name := range []string{"README.md", "README_zh.md", "docs/guide.md", "docs/guide_zh.md", "CONTRIBUTING.md", "CHANGELOG.md", "SECURITY.md"} {
		text, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range links.FindAllSubmatch(text, -1) {
			target := string(match[1])
			if regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`).MatchString(target) {
				continue
			}
			path, anchor, _ := strings.Cut(target, "#")
			linked := name
			if path != "" {
				linked = filepath.Join(filepath.Dir(name), path)
			}
			data, err := os.ReadFile(linked)
			if err != nil {
				t.Fatalf("%s link %s: %v", name, target, err)
			}
			if anchor != "" {
				found := false
				for _, heading := range headings.FindAllSubmatch(data, -1) {
					slug := strings.Map(func(r rune) rune {
						if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '-' || r == '_' {
							return r
						}
						if r == ' ' {
							return '-'
						}
						return -1
					}, strings.ToLower(string(heading[1])))
					if slug == anchor {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("missing anchor %s in %s", anchor, linked)
				}
			}
		}
	}
}

func TestGuideFunctionsRunAgainstSyntheticService(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"guide.md", "guide_zh.md"} {
		text, err := os.ReadFile(filepath.Join("docs", name))
		if err != nil {
			t.Fatal(err)
		}
		blocks := regexp.MustCompile("(?s)```go\\n(.*?)\\n```").FindAllSubmatch(text, -1)
		if len(blocks) != 2 {
			t.Fatal("expected asynchronous and page guide functions")
		}
		for index, block := range blocks {
			t.Run(name+"/"+strconv.Itoa(index), func(t *testing.T) {
				dir := t.TempDir()
				module := "module example.com/guide-consumer\n\ngo 1.20\n\nrequire github.com/nstdata-ai/crawl-go v1.0.0\nreplace github.com/nstdata-ai/crawl-go => " + strconv.Quote(root) + "\n"
				source := "package main\nimport (\"context\";\"fmt\";crawl \"github.com/nstdata-ai/crawl-go\")\n" + string(block[1]) + "\n"
				call := `id,text,err := submitAndInspect(ctx,client,"https://target.invalid/"); if err!=nil||id!="fixture-task"||text!="fixture text" {t.Fatalf("async: %q %q %v",id,text,err)}`
				if index == 1 {
					call = `cursor,err := readBatch(ctx,client,"fixture-task",""); if err!=nil||cursor!="next-cursor" {t.Fatalf("pages: %q %v",cursor,err)}`
				}
				harness := `package main
import("context";"fmt";"net/http";"net/http/httptest";"sync/atomic";"testing";crawl "github.com/nstdata-ai/crawl-go")
func TestWorkflow(t *testing.T) {
 var calls atomic.Int32
 server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request) {
  calls.Add(1)
  if r.Header.Get("x-api-key")!="fixture-key" {t.Error("missing authentication")}
  switch r.URL.Path {
  case "/api/v1/crawl/scrape":
   if r.Method!="POST"||r.URL.Query().Get("async")!="true" {t.Error("invalid submission")}
   fmt.Fprint(w,"{\"err\":false,\"code\":200,\"data\":{\"id\":\"fixture-task\"}}")
  case "/api/v1/crawl/scrape/fixture-task":
   fmt.Fprint(w,"{\"err\":false,\"code\":200,\"data\":{\"status\":\"completed\",\"success\":true,\"data\":{\"markdownRef\":\"opaque\"}}}")
  case "/api/v1/crawl/fixture-task/pages":
   if r.Method!="GET"||r.URL.Query().Get("limit")!="20" {t.Error("invalid page request")}
   fmt.Fprint(w,"{\"err\":false,\"code\":200,\"data\":{\"data\":[{\"url\":\"https://target.invalid/\",\"taskId\":\"page\",\"status\":\"completed\",\"success\":true,\"data\":{\"markdownRef\":\"opaque\"}},{\"url\":\"https://target.invalid/failed\",\"status\":\"failed\",\"success\":false}],\"nextCursor\":\"next-cursor\"}}")
  case "/api/v1/crawl/storage/read":
   if r.Method!="GET"||r.URL.Query().Get("st")!="opaque" {t.Error("invalid reference request")};fmt.Fprint(w,"fixture text")
  default:t.Error("unexpected request");http.NotFound(w,r)
  }
 }))
 defer server.Close()
 client:=crawl.NewClient("fixture-key");client.BaseURL=server.URL
 ctx:=context.Background()
 ` + call + `
 expected:=int32(3);if ` + strconv.Itoa(index) + `==1 {expected=2};if calls.Load()!=expected {t.Fatalf("unexpected calls: %d",calls.Load())}
}
`
				for name, data := range map[string]string{"go.mod": module, "guide.go": source, "guide_test.go": harness} {
					if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
						t.Fatal(err)
					}
				}
				cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "test", "-mod=readonly", "-count=1", ".")
				cmd.Dir = dir
				cmd.Env = moduleConsumerEnv()
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("guide function: %v\n%s", err, output)
				}
			})
		}
	}
}
