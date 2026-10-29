package crawl_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	crawl "github.com/nstdata-ai/crawl-go"
)

func TestRoutesAuthenticationAndPagination(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Error("missing API authentication")
		}
		seen = append(seen, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/crawl/scrape":
			if r.Header.Get("Content-Type") != "application/json" {
				t.Error("missing JSON type")
			}
			var payload map[string]interface{}
			if json.NewDecoder(r.Body).Decode(&payload) != nil || payload["url"] != "https://example.com/" {
				t.Error("request payload")
			}
			if r.URL.Query().Get("async") == "true" {
				fmt.Fprint(w, `{"code":200,"err":false,"data":{"id":"async-1","status":"processing"}}`)
			} else {
				fmt.Fprint(w, `{"code":200,"err":false,"data":{"id":"sync-1","status":"completed","success":true,"data":{"markdown":"inline"}}}`)
			}
		case "/api/v1/crawl":
			fmt.Fprint(w, `{"code":200,"err":false,"data":{"id":"site-1"}}`)
		case "/api/v1/crawl/site-1":
			fmt.Fprint(w, `{"code":200,"err":false,"data":{"status":"completed","failed":1,"completed":1}}`)
		case "/api/v1/crawl/scrape/async-1":
			fmt.Fprint(w, `{"code":200,"err":false,"data":{"status":"failed","success":false,"errorCode":"timeout","data":null}}`)
		case "/api/v1/crawl/site-1/pages":
			if r.URL.Query().Get("after_cursor") != "opaque+/=&" || r.URL.Query().Get("limit") != "2" {
				t.Error("cursor encoding or limit")
			}
			fmt.Fprint(w, `{"code":200,"err":false,"data":{"data":[{"success":true,"data":{"markdownRef":"opaque+/=&"}},{"success":false,"errorCode":"timeout","data":null}],"nextCursor":"next","prevCursor":"previous"}}`)
		case "/api/v1/crawl/storage/read":
			if r.URL.Query().Get("st") != "opaque+/=&" {
				t.Error("reference encoding")
			}
			fmt.Fprint(w, "stored content")
		default:
			t.Errorf("unexpected route: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := crawl.NewClient("test-key")
	c.BaseURL = server.URL
	ctx := context.Background()
	req := &crawl.ScrapeRequestDto{URL: "https://example.com/", Formats: []string{crawl.FormatMarkdown}}
	syncResult, err := c.SubmitScrapeTaskSync(ctx, req)
	if err != nil || syncResult.Id != "sync-1" {
		t.Fatalf("sync: %v %v", syncResult, err)
	}
	submitted, err := c.SubmitScrapeTask(ctx, req)
	if err != nil || submitted.Id != "async-1" {
		t.Fatalf("async: %v %v", submitted, err)
	}
	failed, err := c.GetScrapeTaskResult(ctx, submitted.Id)
	if err != nil || failed.Success || failed.Data != nil || failed.ErrorCode != "timeout" {
		t.Fatalf("task failure: %v %v", failed, err)
	}
	site, err := c.SubmitCrawlTask(ctx, &crawl.CrawlRequestDto{URL: req.URL, Formats: req.Formats, MaxPages: 2})
	if err != nil || site.Id != "site-1" {
		t.Fatalf("site: %v %v", site, err)
	}
	status, err := c.GetCrawlStatus(ctx, site.Id)
	if err != nil || status.Failed != 1 {
		t.Fatalf("status: %v %v", status, err)
	}
	pages, err := c.ListCrawlPages(ctx, site.Id, crawl.ListCrawlPagesOptions{AfterCursor: "opaque+/=&", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages.Data) != 2 || pages.NextCursor != "next" || pages.PrevCursor != "previous" || pages.Data[1].Success || pages.Data[1].Data != nil {
		t.Fatal("page result or failure lost")
	}
	for i := 0; i < 2; i++ {
		value, e := pages.Data[0].Data.GetMarkdown(ctx)
		if e != nil || value != "stored content" {
			t.Fatalf("stored: %q %v", value, e)
		}
	}
	if len(seen) != 7 {
		t.Fatalf("unexpected request count (cached storage should be read once): %v", seen)
	}
	want := []string{
		"POST /api/v1/crawl/scrape", "POST /api/v1/crawl/scrape", "GET /api/v1/crawl/scrape/async-1",
		"POST /api/v1/crawl", "GET /api/v1/crawl/site-1", "GET /api/v1/crawl/site-1/pages", "GET /api/v1/crawl/storage/read",
	}
	if strings.Join(seen, "\n") != strings.Join(want, "\n") {
		t.Fatalf("request methods/routes: %v", seen)
	}
}

func TestContextCancelsResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	c := crawl.NewClient("test")
	c.BaseURL = server.URL
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := c.ReadStorage(ctx, "private-reference")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline identity: %v", err)
	}
}

func TestRedirectDoesNotForwardCredentialsOrResubmit(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var received atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Add(1) }))
			defer target.Close()
			var submitted atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				submitted.Add(1)
				http.Redirect(w, r, target.URL, status)
			}))
			defer server.Close()
			c := crawl.NewClient("secret")
			c.BaseURL = server.URL
			_, err := c.SubmitScrapeTask(context.Background(), &crawl.ScrapeRequestDto{URL: "https://example.com/", Formats: []string{crawl.FormatMarkdown}})
			var api *crawl.APIError
			if !errors.As(err, &api) || api.StatusCode != status || received.Load() != 0 || submitted.Load() != 1 {
				t.Fatalf("redirect/retry: %v %d %d", err, received.Load(), submitted.Load())
			}
		})
	}
}

func TestResponseLimitAndSafeErrors(t *testing.T) {
	for _, operation := range []string{"api", "storage"} {
		t.Run(operation, func(t *testing.T) {
			c := crawl.NewClient("private-key")
			c.MaxResponseBytes = 10
			c.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return response(strings.Repeat("x", 11)), nil })}
			var err error
			if operation == "api" {
				_, err = c.GetCrawlStatus(context.Background(), "task")
			} else {
				_, err = c.ReadStorage(context.Background(), "private-ref")
			}
			if !errors.Is(err, crawl.ErrResponseTooLarge) {
				t.Fatalf("limit: %v", err)
			}
		})
	}
	for _, body := range []string{`{"code":401,"err":true,"msg":"private-key private-ref","data":{}}`, `{"code":200,"err":false,"data":{"data":{"markdown":123}}}`, `private-key private-ref`} {
		c := crawl.NewClient("private-key")
		c.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return response(body), nil })}
		_, err := c.GetScrapeTaskResult(context.Background(), "task")
		if err == nil || strings.Contains(err.Error(), "private-key") || strings.Contains(err.Error(), "private-ref") {
			t.Fatalf("unsafe error: %v", err)
		}
	}
}

func TestInvalidConfigurationAndInputDoNotDispatch(t *testing.T) {
	for _, origin := range []string{"http://example.com", "https://user:pass@example.com", "https://example.com/path", "https://example.com?token=secret", "https://example.com/#secret", "https://example.com?", "https://example.com#", "ftp://localhost"} {
		c := crawl.NewClient("test")
		c.BaseURL = origin
		c.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { t.Fatal("invalid origin dispatched"); return nil, nil })}
		if _, err := c.ReadStorage(context.Background(), "ref"); err == nil {
			t.Errorf("accepted origin: %s", origin)
		}
	}
	c := crawl.NewClient("test")
	c.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { t.Fatal("invalid input dispatched"); return nil, nil })}
	for _, id := range []string{"", " ", "..", "a/b", "a?b"} {
		if _, err := c.GetScrapeTaskResult(context.Background(), id); err == nil {
			t.Errorf("invalid ID %q", id)
		}
	}
	if _, err := c.ListCrawlPages(context.Background(), "known", crawl.ListCrawlPagesOptions{AfterCursor: "a", BeforeCursor: "b"}); err == nil {
		t.Error("opposing cursors accepted")
	}
	if _, err := c.ListCrawlPages(context.Background(), "known", crawl.ListCrawlPagesOptions{Limit: 101}); err == nil {
		t.Error("oversized limit accepted")
	}
	for _, timeout := range []float64{-1, 999, 1000.5, 60001} {
		if _, err := c.SubmitScrapeTask(context.Background(), &crawl.ScrapeRequestDto{URL: "https://example.com", Formats: []string{"markdown"}, Timeout: timeout}); err == nil {
			t.Errorf("invalid timeout %v", timeout)
		}
	}
	for _, wait := range []float64{1, 999, 1000.5, 30001} {
		if _, err := c.SubmitCrawlTask(context.Background(), &crawl.CrawlRequestDto{URL: "https://example.com", Formats: []string{"markdown"}, JsRender: &crawl.JsRenderDto{WaitFor: wait}}); err == nil {
			t.Errorf("invalid wait %v", wait)
		}
	}
}

func TestConcurrentContentGettersAndCache(t *testing.T) {
	c := crawl.NewClient("test")
	var reads atomic.Int32
	c.HTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "storage") {
			reads.Add(1)
			return response("bytes"), nil
		}
		return response(`{"code":200,"err":false,"data":{"data":{"pdfRef":"ref"}}}`), nil
	})}
	result, err := c.GetScrapeTaskResult(context.Background(), "task")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() {
			b, e := result.Data.GetPdf(context.Background())
			if e == nil {
				b[0] = 'X'
			}
			done <- e
		}()
	}
	for i := 0; i < 8; i++ {
		if e := <-done; e != nil {
			t.Fatal(e)
		}
	}
	value, err := result.Data.GetPdf(context.Background())
	if err != nil || string(value) != "bytes" {
		t.Fatalf("mutable cache: %q %v", value, err)
	}
	copyOfResult := *result.Data
	copied, copyErr := copyOfResult.GetPdf(context.Background())
	if copyErr != nil || string(copied) != "bytes" {
		t.Fatalf("copied result: %q %v", copied, copyErr)
	}
	if reads.Load() > 8 {
		t.Fatalf("unexpected reads: %d", reads.Load())
	}
}

func TestTransportFailurePreservesCancellationAndRedactsURL(t *testing.T) {
	c := crawl.NewClient("test")
	c.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("private-key private-ref") })}
	_, err := c.ReadStorage(context.Background(), "private-ref")
	if err == nil || strings.Contains(err.Error(), "private-ref") || strings.Contains(err.Error(), "private-key") {
		t.Fatalf("unsafe transport error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.ReadStorage(ctx, "ref")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel identity: %v", err)
	}
}

// Ensure read failures close the body without exposing transport error details.
type failedBody struct {
	closed bool
	reads  int
}

func (b *failedBody) Read([]byte) (int, error) { b.reads++; return 0, errors.New("private-response") }
func (b *failedBody) Close() error             { b.closed = true; return nil }
func TestReadFailureClosesBody(t *testing.T) {
	b := &failedBody{}
	c := crawl.NewClient("test")
	c.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: b}, nil
	})}
	_, err := c.ReadStorage(context.Background(), "ref")
	if err == nil || !b.closed || strings.Contains(err.Error(), "private-response") {
		t.Fatalf("body cleanup: %v closed=%v", err, b.closed)
	}
}

func TestHTTPErrorsCloseBodyWithoutReadingIt(t *testing.T) {
	body := &failedBody{}
	c := crawl.NewClient("test")
	c.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Header: make(http.Header), Body: body}, nil
	})}
	_, err := c.ReadStorage(context.Background(), "ref")
	var api *crawl.APIError
	if !errors.As(err, &api) || api.StatusCode != 429 || !body.closed || body.reads != 0 {
		t.Fatalf("HTTP error: %v, closed=%v", err, body.closed)
	}
}

func TestTLSVerificationAndCustomTrust(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "trusted content")
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	c := crawl.NewClient("test")
	c.BaseURL = server.URL
	if _, err := c.ReadStorage(context.Background(), "ref"); err == nil {
		t.Fatal("untrusted TLS certificate accepted")
	}
	c.HTTPClient = server.Client()
	content, err := c.ReadStorage(context.Background(), "ref")
	if err != nil || string(content) != "trusted content" {
		t.Fatalf("explicit local CA trust: %q %v", content, err)
	}
}

func TestExactResponseLimitAndNilHTTPClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "123")
	}))
	defer server.Close()
	c := crawl.NewClient("test")
	c.BaseURL = server.URL
	c.HTTPClient = nil
	c.MaxResponseBytes = 3
	content, err := c.ReadStorage(context.Background(), "ref")
	if err != nil || string(content) != "123" {
		t.Fatalf("exact response boundary: %q %v", content, err)
	}
}

var _ io.ReadCloser = (*failedBody)(nil)

func TestInvalidUTF8APIResponsesAreRejected(t *testing.T) {
	for _, operation := range []string{"sync", "query", "pages"} {
		t.Run(operation, func(t *testing.T) {
			calls := 0
			client := crawl.NewClient("fixture-key")
			client.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				payload := `{"data":{"markdown":"private-` + string([]byte{0xff}) + `"}}`
				if operation == "pages" {
					payload = `{"data":[` + payload + `]}`
				}
				return response(`{"code":200,"err":false,"data":` + payload + `}`), nil
			})}
			var err error
			switch operation {
			case "sync":
				_, err = client.SubmitScrapeTaskSync(context.Background(), &crawl.ScrapeRequestDto{URL: "https://target.invalid/", Formats: []string{"markdown"}})
			case "query":
				_, err = client.GetScrapeTaskResult(context.Background(), "existing")
			case "pages":
				_, err = client.ListCrawlPages(context.Background(), "existing", crawl.ListCrawlPagesOptions{})
			}
			if err == nil || strings.Contains(err.Error(), "private-") || calls != 1 {
				t.Fatalf("invalid UTF-8: err=%v calls=%d", err, calls)
			}
		})
	}
}

func TestTextReferencesRejectInvalidUTF8WithoutCaching(t *testing.T) {
	for _, format := range []string{"html", "markdown"} {
		t.Run(format, func(t *testing.T) {
			client := crawl.NewClient("fixture-key")
			reads := 0
			client.HTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, "/read") {
					reads++
					if reads == 1 {
						return response(string([]byte{0xff})), nil
					}
					return response("valid text"), nil
				}
				return response(`{"code":200,"err":false,"data":{"data":{"` + format + `Ref":"opaque"}}}`), nil
			})}
			result, err := client.GetScrapeTaskResult(context.Background(), "existing")
			if err != nil {
				t.Fatal(err)
			}
			read := result.Data.GetMarkdown
			if format == "html" {
				read = result.Data.GetHtml
			}
			text, err := read(context.Background())
			if err == nil || text != "" || reads != 1 {
				t.Fatalf("invalid text: %q %v reads=%d", text, err, reads)
			}
			text, err = read(context.Background())
			if err != nil || text != "valid text" || reads != 2 {
				t.Fatalf("later read: %q %v reads=%d", text, err, reads)
			}
			text, err = read(context.Background())
			if err != nil || text != "valid text" || reads != 2 {
				t.Fatalf("cache: %q %v reads=%d", text, err, reads)
			}
		})
	}
}

func TestRawReferenceBytesRemainUnchanged(t *testing.T) {
	client := crawl.NewClient("fixture-key")
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/read") {
			return response(string([]byte{0xff, 0x00, 0x80})), nil
		}
		return response(`{"code":200,"err":false,"data":{"data":{"rawDataRef":"opaque"}}}`), nil
	})}
	result, err := client.GetScrapeTaskResult(context.Background(), "existing")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := result.Data.GetRawData(context.Background())
	if err != nil || string(raw) != string([]byte{0xff, 0x00, 0x80}) {
		t.Fatalf("raw bytes changed: %v %v", raw, err)
	}
}

func TestMainContentBooleanIsSentExplicitly(t *testing.T) {
	for _, value := range []bool{false, true} {
		calls := 0
		client := crawl.NewClient("fixture-key")
		client.HTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			actual, present := body["onlyMainContent"]
			if !present || actual != value {
				t.Errorf("onlyMainContent: value=%v present=%v expected=%v", actual, present, value)
			}
			return response(`{"code":200,"err":false,"data":{}}`), nil
		})}
		ctx := context.Background()
		scrape := &crawl.ScrapeRequestDto{URL: "https://target.invalid/", Formats: []string{"markdown"}, OnlyMainContent: value}
		if _, err := client.SubmitScrapeTaskSync(ctx, scrape); err != nil {
			t.Fatal(err)
		}
		if _, err := client.SubmitScrapeTask(ctx, scrape); err != nil {
			t.Fatal(err)
		}
		if _, err := client.SubmitCrawlTask(ctx, &crawl.CrawlRequestDto{URL: scrape.URL, Formats: scrape.Formats, OnlyMainContent: value}); err != nil {
			t.Fatal(err)
		}
		if calls != 3 {
			t.Fatalf("unexpected requests: %d", calls)
		}
	}
}
