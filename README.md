# Nstdata Crawl SDK for Go

[中文](README_zh.md)

A Go client for synchronous and asynchronous scraping, bounded site crawling, task queries and content references. Uses the standard library only.

## Installation

Go 1.20 compatibility is retained. Use a [maintained Go release](https://go.dev/doc/devel/release#policy) for production.

Version `v1.0.0` is prepared locally and has not been published. After publication:

```sh
go get github.com/nstdata-ai/crawl-go@v1.0.0
```

For checkout installation and verification, see [Contributing](CONTRIBUTING.md).

## Quick start

Set `NSTDATA_API_TOKEN` to an authorized Crawl key. This example submits one paid synchronous scrape; use a target you are allowed to access.

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "time"

    crawl "github.com/nstdata-ai/crawl-go"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()
    client := crawl.NewClient(os.Getenv("NSTDATA_API_TOKEN"))
    result, err := client.SubmitScrapeTaskSync(ctx, &crawl.ScrapeRequestDto{
        URL: "https://example.com/", Formats: []string{crawl.FormatMarkdown}, OnlyMainContent: true,
    })
    if err != nil { log.Fatal(err) }
    if result.Status != crawl.StatusCompleted || !result.Success || result.Data == nil {
        log.Fatal("task did not return successful completed content")
    }
    text, err := result.Data.GetMarkdown(ctx)
    if err != nil { log.Fatal(err) }
    fmt.Println(text)
    if result.Data.Truncated != nil && *result.Data.Truncated {
        log.Print("content is truncated")
    }
}
```

## Operations

| Method | Purpose |
|---|---|
| `SubmitScrapeTaskSync` | One synchronous scrape; returns task state and optional content |
| `SubmitScrapeTask` | Submit asynchronously; save `Id` before querying |
| `GetScrapeTaskResult` | Query a previously saved asynchronous task ID |
| `SubmitCrawlTask` | Submit a site crawl with native traversal options |
| `GetCrawlStatus` | Read aggregate site progress, including failures |
| `ListCrawlPages` | Read one cursor batch; failed pages remain in the result |
| `ReadStorage` | Read an opaque reference through the authenticated API origin |

The client performs one operation per method. It does not poll, paginate or retry submissions automatically. A completed task can still have unsuccessful or missing content. A synchronous response ID is not guaranteed to support later task queries.

`ListCrawlPagesOptions` accepts one of `AfterCursor` or `BeforeCursor`, and `Limit` from 1 to 100; zero uses the server default. Pass cursors unchanged. `MaxPages` is an upper bound, not a promise of that many pages. URL filters and depth counting follow the service's native rules.

[The executable example](example/example.go) provides `scrape`, `submit-scrape`, `submit-crawl`, `resume-scrape` and `resume-crawl` commands. Submission and continuation are separate. Polling stops after 20 queries, pagination after 100 batches or a repeated cursor, and the overall context after two minutes. Preserve task IDs and partial results when local waiting ends; this does not cancel the remote task.

## Content and errors

- `GetHtml` / `GetMarkdown` return text; `GetPdf` / `GetScreenshot` return decoded bytes; `GetRawData` returns UTF-8 bytes, not schema extraction.
- Getters prefer inline content, including empty values, then read a reference if present. Successful reads are cached. Concurrent initial reads may each fetch the same reference; subsequent reads use the cache. Returned byte slices can be modified without changing cached content.
- Missing/null content, nil results or unattached references return `ErrContentUnavailable`. Empty present content succeeds. `Truncated == nil` means unknown; an explicit false remains distinguishable.
- Known metadata fields are typed; `Metadata.AdditionalFields` retains other fields as `json.RawMessage`. Native output availability and extraction quality depend on the service.
- HTTP/envelope errors return `*APIError` with `StatusCode` and optional envelope `Code`. Failed task status remains a normal response to inspect. Error strings exclude response bodies, references and transport URLs.
- Context cancellation/deadlines remain detectable with `errors.Is`. Invalid JSON, envelope or content types return errors. Oversized responses return `ErrResponseTooLarge`.

## Configuration

Configure `Client` before concurrent use:

- `BaseURL` defaults to `https://api.nstdata.io`. Custom origins require HTTPS, with HTTP allowed only for loopback tests; credentials, paths, queries and fragments are rejected.
- `HTTPClient` defaults to an independent client with a 120-second per-operation timeout. A nil value uses that default. Custom clients retain their transport and timeout, but redirects are always refused to avoid forwarding authentication or signed references. Keep TLS certificate verification enabled; custom transports must honor request contexts.
- `MaxResponseBytes` defaults to 1 MiB per decoded API/storage response; set a positive byte limit for larger authorized outputs. This is separate from service content truncation.
- Request `Timeout` is an optional integer from 1000–60000 milliseconds; `JsRender.WaitFor` is 1000–30000. Action milliseconds and coordinates must be finite integers. Existing numeric field types remain unchanged; zero omits an optional setting. The request DTO timeout is separate from local context/HTTP timeouts.

Never mutate configuration or exported result fields during concurrent use. Store keys in local environment or a secret store; do not log keys, signed references or private content.

## Compatibility

The existing import path, constructor, public methods and action helpers are retained. Compared with `v0.1.1`, the default origin is updated, malformed responses are rejected, missing content returns an error, redirects are blocked and responses have a default size limit. Callers must handle these errors and configure larger limits explicitly. See [changes](CHANGELOG.md).

[API documentation](https://docs.nstdata.io/docs/crawl) · [License](LICENSE)
