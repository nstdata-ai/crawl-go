package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const reviewed = "0123456789012345678901234567890123456789"
const checksum = "h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type fixture struct {
	t           *testing.T
	apiCalls    []*http.Request
	commands    [][]string
	settings    string
	policies    string
	existingTag bool
	posts       int
	postStatus  int
	postBody    string
	directHash  string
	proxySum    string
	corruptFile bool
}

func newFixture(t *testing.T) (release, *fixture) {
	t.Helper()
	f := &fixture{t: t, postStatus: 201, directHash: reviewed, proxySum: checksum,
		settings: newSettings(),
		policies: `{"total_count":1,"branch_policies":[{"name":"main","type":"branch"}]}`}
	root := t.TempDir()
	for name, content := range map[string]string{
		"README.md":    "# SDK\n\ngo get " + modulePath + "@v1.0.0\n\n```go\npackage main\nfunc main(){}\n```\n",
		"README_zh.md": "# SDK\n\ngo get " + modulePath + "@v1.0.0\n",
		"CHANGELOG.md": "# Changelog\n\n## v1.0.0\n",
		"go.mod":       "module " + modulePath + "\n\ngo 1.20\n",
		"client.go":    "package crawl\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	r := release{root: root, version: "v1.0.0", commit: reviewed, token: "fixture-token",
		env: map[string]string{"GITHUB_EVENT_NAME": "workflow_dispatch", "GITHUB_REPOSITORY": repository,
			"GITHUB_REF": "refs/heads/main", "GITHUB_SHA": reviewed},
		client: &http.Client{Transport: transport(f.respond), CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}}}
	r.run = func(_ context.Context, dir, name string, args, env []string) ([]byte, error) {
		f.commands = append(f.commands, append([]string{name}, args...))
		if name == "git" {
			switch args[0] {
			case "rev-parse":
				return []byte(reviewed + "\n"), nil
			case "status":
				return nil, nil
			case "ls-files":
				return []byte("client.go\x00go.mod\x00README.md\x00README_zh.md\x00CHANGELOG.md\x00"), nil
			}
		}
		if name != filepath.Join(runtime.GOROOT(), "bin", "go") {
			t.Fatal("unexpected command")
		}
		values := map[string]string{}
		for _, value := range env {
			key, value, _ := strings.Cut(value, "=")
			values[key] = value
		}
		for key, want := range map[string]string{"GOENV": "off", "GOWORK": "off", "GOTOOLCHAIN": "local", "GOSUMDB": "sum.golang.org", "GOPRIVATE": "", "GONOPROXY": "", "GONOSUMDB": "", "GIT_CONFIG_NOSYSTEM": "1"} {
			if values[key] != want {
				t.Fatalf("non-isolated Go option %s", key)
			}
		}
		for key := range values {
			if strings.HasPrefix(key, "NSTDATA_") || key == "GH_TOKEN" || key == "GITHUB_TOKEN" {
				t.Fatal("credential entered public download environment")
			}
		}
		if len(args) == 4 && args[0] == "mod" && args[1] == "download" {
			cache := filepath.Join(dir, "modcache", "module")
			if err := os.MkdirAll(cache, 0700); err != nil {
				t.Fatal(err)
			}
			for _, file := range []string{"client.go", "go.mod", "README.md", "README_zh.md", "CHANGELOG.md"} {
				body, err := os.ReadFile(filepath.Join(root, file))
				if err != nil {
					t.Fatal(err)
				}
				if f.corruptFile && values["GOPROXY"] != "direct" && file == "client.go" {
					body = []byte("different")
				}
				if err := os.WriteFile(filepath.Join(cache, file), body, 0600); err != nil {
					t.Fatal(err)
				}
			}
			result := downloaded{Path: modulePath, Version: "v1.0.0", Dir: cache, Sum: checksum, GoModSum: checksum}
			if values["GOPROXY"] == "direct" {
				result.Origin.VCS, result.Origin.URL, result.Origin.Hash = "git", "https://github.com/"+repository, f.directHash
			} else {
				result.Sum = f.proxySum
			}
			return json.Marshal(result)
		}
		if len(args) == 3 && args[0] == "mod" && args[1] == "init" ||
			len(args) == 2 && args[0] == "get" && args[1] == modulePath+"@v1.0.0" || args[0] == "build" {
			return nil, nil
		}
		t.Fatalf("unexpected Go arguments %v", args)
		return nil, nil
	}
	return r, f
}

func (f *fixture) respond(request *http.Request) (*http.Response, error) {
	f.apiCalls = append(f.apiCalls, request)
	if request.URL.Host != "api.github.com" || request.Header.Get("Authorization") != "Bearer fixture-token" {
		f.t.Fatal("unexpected destination or authentication")
	}
	status, body := 200, ""
	switch {
	case strings.HasSuffix(request.URL.Path, "/environments/go-production"):
		body = f.settings
	case strings.HasSuffix(request.URL.Path, "/deployment-branch-policies"):
		body = f.policies
	case strings.HasSuffix(request.URL.Path, "/git/ref/heads/main"):
		body = `{"object":{"type":"commit","sha":"` + reviewed + `"}}`
	case strings.HasSuffix(request.URL.Path, "/git/ref/tags/v1.0.0"):
		if !f.existingTag {
			status = 404
		} else {
			body = `{"ref":"refs/tags/v1.0.0","object":{"type":"commit","sha":"` + reviewed + `"}}`
		}
	case strings.HasSuffix(request.URL.Path, "/git/refs") && request.Method == http.MethodPost:
		f.posts++
		var input map[string]string
		if json.NewDecoder(request.Body).Decode(&input) != nil || input["ref"] != "refs/tags/v1.0.0" || input["sha"] != reviewed {
			f.t.Fatal("tag request differs from reviewed source")
		}
		status = f.postStatus
		body = f.postBody
		if body == "" {
			body = `{"ref":"refs/tags/v1.0.0","object":{"type":"commit","sha":"` + reviewed + `"}}`
		}
		f.existingTag = true
	default:
		f.t.Fatal("unexpected GitHub operation")
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: request}, nil
}

func TestSourceGuardRejectsUnreviewedContextsBeforeRequests(t *testing.T) {
	for _, change := range []func(*release){
		func(r *release) { r.env["GITHUB_EVENT_NAME"] = "push" },
		func(r *release) { r.env["GITHUB_REPOSITORY"] = "other/repository" },
		func(r *release) { r.env["GITHUB_REF"] = "refs/tags/v1.0.0" },
		func(r *release) { r.commit = "short" },
		func(r *release) { r.env["GITHUB_SHA"] = strings.Repeat("a", 40) },
		func(r *release) { r.version = "v1.0.0-rc.1" },
		func(r *release) { r.version = "v2.0.0" },
	} {
		r, f := newFixture(t)
		change(&r)
		if err := r.perform(context.Background(), "tag"); err == nil || len(f.apiCalls) != 0 {
			t.Fatal("invalid source was not rejected before requests")
		}
	}
}

func TestSourceGuardRejectsDocumentationAndWorkingTreeDrift(t *testing.T) {
	for _, file := range []string{"README.md", "README_zh.md", "go.mod", "CHANGELOG.md"} {
		r, f := newFixture(t)
		if err := os.WriteFile(filepath.Join(r.root, file), []byte("different"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := r.perform(context.Background(), "check"); err == nil || len(f.apiCalls) != 0 {
			t.Fatal("source drift passed")
		}
	}
	r, f := newFixture(t)
	r.run = func(context.Context, string, string, []string, []string) ([]byte, error) {
		return []byte("different"), nil
	}
	if err := r.perform(context.Background(), "tag"); err == nil || len(f.apiCalls) != 0 {
		t.Fatal("changed Git source passed")
	}
}

func TestEnvironmentRequiresReviewersExplicitNoBypassAndOnlyMain(t *testing.T) {
	for _, settings := range []string{
		`{}`, strings.ReplaceAll(newSettings(), `"can_admins_bypass":false`, `"can_admins_bypass":true`),
		strings.ReplaceAll(newSettings(), `"can_admins_bypass":false,`, ``),
		strings.ReplaceAll(newSettings(), `"reviewers":[{}]`, `"reviewers":[]`),
		strings.ReplaceAll(newSettings(), `"protected_branches":false`, `"protected_branches":true`),
	} {
		r, f := newFixture(t)
		f.settings = settings
		if err := r.perform(context.Background(), "tag"); err == nil || f.posts != 0 {
			t.Fatal("invalid environment permitted tag creation")
		}
	}
	r, f := newFixture(t)
	f.policies = `{"total_count":1,"branch_policies":[{"name":"main","type":"tag"}]}`
	if err := r.perform(context.Background(), "tag"); err == nil || f.posts != 0 {
		t.Fatal("tag deployment rule passed")
	}
}

func newSettings() string {
	return `{"can_admins_bypass":false,"protection_rules":[{"type":"required_reviewers","reviewers":[{}]}],"deployment_branch_policy":{"custom_branch_policies":true,"protected_branches":false}}`
}

func TestCheckIsReadOnlyAndExistingVersionCannotBeRetagged(t *testing.T) {
	r, f := newFixture(t)
	if err := r.perform(context.Background(), "check"); err != nil || f.posts != 0 {
		t.Fatalf("check: %v", err)
	}
	f.existingTag = true
	if err := r.perform(context.Background(), "tag"); err == nil || f.posts != 0 {
		t.Fatal("existing version was overwritten")
	}
}

func TestTagCreationUsesOneExactRequestAndConfirmsReference(t *testing.T) {
	r, f := newFixture(t)
	if err := r.perform(context.Background(), "tag"); err != nil || f.posts != 1 {
		t.Fatalf("tag: %v, attempts=%d", err, f.posts)
	}
	if !strings.HasSuffix(f.apiCalls[len(f.apiCalls)-1].URL.Path, "/git/ref/tags/v1.0.0") {
		t.Fatal("created reference was not read back")
	}
}

func TestFailedTagCreationStopsWithoutRetryOrProviderData(t *testing.T) {
	for _, status := range []int{403, 422, 500} {
		r, f := newFixture(t)
		f.postStatus, f.postBody = status, "fixture-private-response"
		err := r.perform(context.Background(), "tag")
		if err == nil || f.posts != 1 || strings.Contains(err.Error(), "fixture-private-response") {
			t.Fatalf("failed creation: %v attempts=%d", err, f.posts)
		}
	}
}

func TestPublicVerificationChecksOriginHashesBytesAndDefaultGet(t *testing.T) {
	r, f := newFixture(t)
	f.existingTag = true
	if err := r.perform(context.Background(), "verify"); err != nil {
		t.Fatal(err)
	}
	gets := 0
	for _, args := range f.commands {
		if len(args) == 3 && args[1] == "get" && args[2] == modulePath+"@v1.0.0" {
			gets++
		}
	}
	if gets != 1 || f.posts != 0 {
		t.Fatal("public verification must use one standard go get and no writes")
	}
	for _, change := range []func(*fixture){
		func(f *fixture) { f.directHash = "" },
		func(f *fixture) { f.proxySum = "h1:different" },
		func(f *fixture) { f.corruptFile = true },
	} {
		r, f := newFixture(t)
		f.existingTag = true
		change(f)
		if err := r.perform(context.Background(), "verify"); err == nil {
			t.Fatal("mismatched public module passed")
		}
	}
}

func TestInvalidModeAndMissingTokenCannotMakeRequests(t *testing.T) {
	r, f := newFixture(t)
	if err := r.perform(context.Background(), "unknown"); err == nil || len(f.apiCalls) != 0 {
		t.Fatal("invalid mode made a request")
	}
	r.token = ""
	if err := r.perform(context.Background(), "tag"); err == nil || len(f.apiCalls) != 0 {
		t.Fatal("missing token made a request")
	}
}

func TestApiErrorsRemainBoundedAndDoNotExposeRawMessages(t *testing.T) {
	for _, body := range []string{"fixture-private-response", strings.Repeat("X", 1048577)} {
		r, _ := newFixture(t)
		r.client.Transport = transport(func(request *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		if err := r.perform(context.Background(), "check"); err == nil || strings.Contains(err.Error(), "fixture-private") {
			t.Fatal("malformed metadata exposed provider content")
		}
	}
	r, _ := newFixture(t)
	r.client.Transport = transport(func(*http.Request) (*http.Response, error) { return nil, errors.New("fixture-token in provider URL") })
	if err := r.perform(context.Background(), "check"); err == nil || strings.Contains(err.Error(), "fixture-token") {
		t.Fatal("transport error leaked credentials")
	}
}

func TestFailedCommandsDoNotExposeArgumentsOrDiagnostics(t *testing.T) {
	_, err := execute(context.Background(), t.TempDir(), filepath.Join(runtime.GOROOT(), "bin", "go"),
		[]string{"env", "-invalid", "fixture-sensitive"}, hostEnvironment())
	if err == nil || strings.Contains(err.Error(), "fixture-sensitive") || !strings.Contains(err.Error(), "go env") {
		t.Fatalf("unsafe command error: %v", err)
	}
}
