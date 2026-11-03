package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	crawl "github.com/nstdata-ai/crawl-go"
)

func TestExampleStopsOnFailedSyncTask(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":200,"err":false,"data":{"status":"failed","success":false,"data":null}}`)
	}))
	defer server.Close()
	client := crawl.NewClient("test")
	client.BaseURL = server.URL
	var out bytes.Buffer
	if err := run(context.Background(), client, []string{"scrape", "https://example.com/"}, &out); err == nil || out.Len() != 0 {
		t.Fatalf("failed task should stop safely: %v", err)
	}
}

func TestExampleStopsRepeatedCursorAndRetainsPartialOutput(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if strings.HasSuffix(r.URL.Path, "/pages") {
			fmt.Fprint(w, `{"code":200,"err":false,"data":{"data":[{"status":"completed","success":true,"data":{"markdown":"Partial page"}}],"nextCursor":"same"}}`)
		} else {
			fmt.Fprint(w, `{"code":200,"err":false,"data":{"status":"completed","completed":1,"failed":0}}`)
		}
	}))
	defer server.Close()
	client := crawl.NewClient("test")
	client.BaseURL = server.URL
	var out bytes.Buffer
	err := run(context.Background(), client, []string{"resume-crawl", "known"}, &out)
	if err == nil || !strings.Contains(err.Error(), "repeated cursor") || !strings.Contains(out.String(), "Partial page") || requests != 3 {
		t.Fatalf("cursor stop: %v, requests %d, output %q", err, requests, out.String())
	}
}

func TestExamplePollingStopsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		fmt.Fprint(w, `{"code":200,"err":false,"data":{"status":"processing"}}`)
		w.(http.Flusher).Flush()
		close(started)
	}))
	defer server.Close()
	client := crawl.NewClient("test")
	client.BaseURL = server.URL
	result := make(chan error, 1)
	go func() { var out bytes.Buffer; result <- run(ctx, client, []string{"resume-scrape", "known"}, &out) }()
	select {
	case <-started:
		cancel()
	case <-time.After(5 * time.Second):
		t.Fatal("request did not start")
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) || requests != 1 {
			t.Fatalf("poll cancellation: %v, requests %d", err, requests)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("context did not stop continuation")
	}
}

type failFirstWriter struct {
	calls int
	err   error
}

func (w *failFirstWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls == 1 {
		return 0, w.err
	}
	return len(p), nil
}

func TestPageOutputFailureStopsAndPreservesCause(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":200,"err":false,"data":{"data":[{"status":"completed","success":true,"data":{"markdown":"first"}},{"status":"completed","success":true,"data":{"markdown":"second"}}]}}`)
	}))
	defer server.Close()
	client := crawl.NewClient("fixture-key")
	client.BaseURL = server.URL
	failure := errors.New("fixture output failure")
	writer := &failFirstWriter{err: failure}
	err := printPages(context.Background(), client, "existing", writer, "")
	if !errors.Is(err, failure) || writer.calls != 1 {
		t.Fatalf("output failure: %v writes=%d", err, writer.calls)
	}
}

func TestPageCancellationStopsWithoutFailedPageOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if strings.HasSuffix(r.URL.Path, "/read") {
			cancel()
			// Keep the response pending until the client observes cancellation.
			<-r.Context().Done()
			return
		}
		fmt.Fprint(w, `{"code":200,"err":false,"data":{"data":[{"status":"completed","success":true,"data":{"markdownRef":"opaque"}}]}}`)
	}))
	defer server.Close()
	client := crawl.NewClient("fixture-key")
	client.BaseURL = server.URL
	var out bytes.Buffer
	err := printPages(ctx, client, "existing", &out, "")
	if !errors.Is(err, context.Canceled) || out.Len() != 0 || calls != 2 {
		t.Fatalf("page cancellation: %v output=%q calls=%d", err, out.String(), calls)
	}
}

func TestResumeCrawlContinuesAfterPaginationLimit(t *testing.T) {
	pages, statuses := 0, 0
	resumed := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/pages") {
			statuses++
			fmt.Fprint(w, `{"code":200,"err":false,"data":{"status":"completed"}}`)
			return
		}
		pages++
		expected := ""
		if pages > 1 {
			expected = fmt.Sprintf("cursor-%d", pages-1)
		}
		if got := r.URL.Query().Get("after_cursor"); got != expected {
			t.Errorf("cursor=%q want %q", got, expected)
		}
		if resumed {
			fmt.Fprint(w, `{"code":200,"err":false,"data":{"data":[{"status":"completed","success":true,"data":{"markdown":"恢复正文"}}]}}`)
		} else {
			fmt.Fprintf(w, `{"code":200,"err":false,"data":{"data":[],"nextCursor":"cursor-%d"}}`, pages)
		}
	}))
	defer server.Close()
	client := crawl.NewClient("fixture-key")
	client.BaseURL = server.URL
	var out bytes.Buffer
	err := run(context.Background(), client, []string{"resume-crawl", "saved"}, &out)
	if err == nil || !strings.Contains(err.Error(), `"cursor-100"`) || pages != 100 {
		t.Fatalf("limit: %v pages=%d", err, pages)
	}
	resumed = true
	out.Reset()
	err = run(context.Background(), client, []string{"resume-crawl", "saved", "cursor-100"}, &out)
	if err != nil || !strings.Contains(out.String(), "恢复正文") || pages != 101 || statuses != 2 {
		t.Fatalf("resume: %v pages=%d statuses=%d output=%q", err, pages, statuses, out.String())
	}
}

func TestResumeCrawlRejectsImmediateRepeatedResumeCursor(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if strings.HasSuffix(r.URL.Path, "/pages") {
			if r.URL.Query().Get("after_cursor") != "saved &雪" {
				t.Errorf("cursor changed")
			}
			fmt.Fprint(w, `{"code":200,"err":false,"data":{"data":[],"nextCursor":"saved &雪"}}`)
		} else {
			fmt.Fprint(w, `{"code":200,"err":false,"data":{"status":"completed"}}`)
		}
	}))
	defer server.Close()
	client := crawl.NewClient("fixture-key")
	client.BaseURL = server.URL
	var out bytes.Buffer
	err := run(context.Background(), client, []string{"resume-crawl", "saved", "saved &雪"}, &out)
	if err == nil || !strings.Contains(err.Error(), "repeated cursor") || calls != 2 {
		t.Fatalf("cycle: %v calls=%d", err, calls)
	}
}

func TestOptionalCursorOnlyAcceptedForCrawlResumption(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer server.Close()
	client := crawl.NewClient("fixture-key")
	client.BaseURL = server.URL
	for _, args := range [][]string{{"resume-crawl", "saved", ""}, {"scrape", "https://example.com/", "cursor"}, {"resume-scrape", "saved", "cursor"}, {"submit-crawl", "https://example.com/", "cursor"}, {"resume-crawl", "saved", "cursor", "extra"}} {
		var out bytes.Buffer
		if err := run(context.Background(), client, args, &out); err == nil {
			t.Fatalf("accepted invalid arguments %q", args)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid args caused %d requests", calls)
	}
}
