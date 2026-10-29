package crawl_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	crawl "github.com/nstdata-ai/crawl-go"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func response(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestNilRequestsReturnErrors(t *testing.T) {
	c := crawl.NewClient("test")
	c.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid requests must not leave the process")
		return nil, nil
	})}
	for name, call := range map[string]func() error{
		"sync":  func() error { _, err := c.SubmitScrapeTaskSync(context.Background(), nil); return err },
		"async": func() error { _, err := c.SubmitScrapeTask(context.Background(), nil); return err },
		"crawl": func() error { _, err := c.SubmitCrawlTask(context.Background(), nil); return err },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() != nil {
					t.Error("nil input caused a panic")
				}
			}()
			if err := call(); err == nil {
				t.Error("expected input error")
			}
		})
	}
}

func TestResponseEnvelopeRequiresObjectData(t *testing.T) {
	for _, body := range []string{
		`{"code":200,"err":false,"data":null}`,
		`{"code":200,"err":false}`,
		`{"code":200,"data":{}}`,
		`{"code":200,"err":false,"data":[]}`,
	} {
		t.Run(body, func(t *testing.T) {
			c := crawl.NewClient("test")
			c.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return response(body), nil })}
			if _, err := c.GetScrapeTaskResult(context.Background(), "known-id"); err == nil {
				t.Error("malformed envelope accepted")
			}
		})
	}
}

func TestInlineContentAndTaskIdentity(t *testing.T) {
	c := crawl.NewClient("test")
	c.HTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.nstdata.io" {
			t.Errorf("API origin: %s", r.URL.Host)
		}
		return response(`{"code":200,"err":false,"data":{"id":"task-1","status":"completed","success":true,"data":{"markdown":"","markdownRef":"must-not-read","pdf":"AQID","screenshot":"BAU=","rawData":"中文","truncated":false}}}`), nil
	})}
	r, err := c.SubmitScrapeTaskSync(context.Background(), &crawl.ScrapeRequestDto{URL: "https://example.com/", Formats: []string{crawl.FormatMarkdown}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Data == nil {
		t.Fatal("missing data")
	}
	if r.Id != "task-1" || r.Data.Truncated == nil || *r.Data.Truncated {
		t.Fatal("task identity or explicit false truncation was lost")
	}
	md, err := r.Data.GetMarkdown(context.Background())
	if err != nil || md != "" {
		t.Fatalf("empty inline Markdown: %q, %v", md, err)
	}
	pdf, err := r.Data.GetPdf(context.Background())
	if err != nil || string(pdf) != "\x01\x02\x03" {
		t.Fatalf("inline PDF: %v, %v", pdf, err)
	}
	raw, err := r.Data.GetRawData(context.Background())
	if err != nil || string(raw) != "中文" {
		t.Fatalf("raw data: %q, %v", raw, err)
	}
}
