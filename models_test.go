package crawl_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	crawl "github.com/nstdata-ai/crawl-go"
)

func TestContentPresenceAndBinaryDecoding(t *testing.T) {
	var value crawl.ScrapeDataDto
	if err := json.Unmarshal([]byte(`{"html":"","markdown":null,"pdf":"","rawData":"raw ✓","screenshot":"BAU="}`), &value); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if html, err := value.GetHtml(ctx); err != nil || html != "" {
		t.Fatalf("present empty HTML: %q %v", html, err)
	}
	if _, err := value.GetMarkdown(ctx); !errors.Is(err, crawl.ErrContentUnavailable) {
		t.Fatalf("null content: %v", err)
	}
	if pdf, err := value.GetPdf(ctx); err != nil || pdf == nil || len(pdf) != 0 {
		t.Fatalf("present empty PDF: %v %v", pdf, err)
	}
	if image, err := value.GetScreenshot(ctx); err != nil || string(image) != "\x04\x05" {
		t.Fatalf("screenshot: %v %v", image, err)
	}
	var missing *crawl.ScrapeDataDto
	if _, err := missing.GetMarkdown(ctx); !errors.Is(err, crawl.ErrContentUnavailable) {
		t.Fatalf("nil result: %v", err)
	}
	standalone := &crawl.ScrapeDataDto{MarkdownRef: "opaque"}
	if _, err := standalone.GetMarkdown(ctx); !errors.Is(err, crawl.ErrContentUnavailable) {
		t.Fatalf("unattached reference: %v", err)
	}
}

func TestMalformedContentIsRejected(t *testing.T) {
	for _, body := range []string{
		`[]`, `null`, `{"markdown":false}`, `{"html":{}}`, `{"pdf":"not base64"}`,
		`{"screenshot":"AQI"}`, `{"links":[null]}`, `{"links":[123]}`,
		`{"markdownRef":5}`, `{"metadata":{"title":3}}`, `{"truncated":"false"}`,
	} {
		t.Run(body, func(t *testing.T) {
			var value crawl.ScrapeDataDto
			if err := json.Unmarshal([]byte(body), &value); err == nil {
				t.Error("invalid content accepted")
			}
		})
	}
}

func TestMetadataPreservesAdditionalFields(t *testing.T) {
	var value crawl.ScrapeDataDto
	if err := json.Unmarshal([]byte(`{"metadata":{"title":"Title","statusCode":200,"cached":false,"length":0,"custom":{"source":"page"}}}`), &value); err != nil {
		t.Fatal(err)
	}
	metadata := value.Metadata
	if metadata.Title != "Title" || metadata.StatusCode != 200 || string(metadata.AdditionalFields["cached"]) != "false" || string(metadata.AdditionalFields["length"]) != "0" {
		t.Fatal("typed or additional metadata lost")
	}
	metadata.AdditionalFields["title"] = json.RawMessage(`"must not override"`)
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip map[string]interface{}
	if json.Unmarshal(encoded, &roundTrip) != nil || roundTrip["title"] != "Title" || roundTrip["cached"] != false || roundTrip["length"] != float64(0) {
		t.Fatalf("metadata serialization: %s", encoded)
	}
}
