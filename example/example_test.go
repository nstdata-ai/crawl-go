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
