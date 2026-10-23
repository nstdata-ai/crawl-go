# Go SDK usage guide

[中文](guide_zh.md) · [Quick start](../README.md)

## Methods

All operations take a `context.Context` and return a typed result plus an error. Configure the client before sharing it between goroutines.

| Method | Input → result |
|---|---|
| `SubmitScrapeTaskSync` | `*ScrapeRequestDto` → `*TaskResultResponseDto` |
| `SubmitScrapeTask` | `*ScrapeRequestDto` → `*SubmitResponseDto` with `Id` |
| `GetScrapeTaskResult` | Saved asynchronous ID → `*TaskResultResponseDto` |
| `SubmitCrawlTask` | `*CrawlRequestDto` → `*SubmitResponseDto` with `Id` |
| `GetCrawlStatus` | Crawl ID → `*CrawlStatusResponseDto` |
| `ListCrawlPages` | Crawl ID and `ListCrawlPagesOptions` → `*CrawlPageListResponseDto` |
| `ReadStorage` | Opaque reference → `[]byte` |

Each method performs one operation. The SDK does not poll, iterate batches or retry task creation automatically. A synchronous result's optional `Id` is a diagnostic identifier, not a guarantee that asynchronous queries can recover it.

## Asynchronous tasks

Submit once and persist the returned ID before continuing. This function submits once and checks once; it returns the ID even if the query or content read fails. A pending result requires later continuation with that same ID.

The following functions reuse the client and deadline-bound context from the quick start; import `context`, `fmt` and the SDK as `crawl`.

```go
func submitAndInspect(ctx context.Context, client *crawl.Client, target string) (string, string, error) {
    submitted, err := client.SubmitScrapeTask(ctx, &crawl.ScrapeRequestDto{
        URL: target, Formats: []string{crawl.FormatMarkdown}, OnlyMainContent: true,
    })
    if err != nil { return "", "", err }
    if submitted.Id == "" { return "", "", fmt.Errorf("submission returned no ID; investigate before resubmitting") }
    id := submitted.Id // Persist this ID before further work.
    result, err := client.GetScrapeTaskResult(ctx, id)
    if err != nil { return id, "", err }
    if result.Status != crawl.StatusCompleted || !result.Success || result.Data == nil {
        return id, "", fmt.Errorf("task is %s; inspect or resume the saved ID", result.Status)
    }
    text, err := result.Data.GetMarkdown(ctx)
    return id, text, err
}
```

For a complete executable workflow, use the [example](../example/example.go): `submit-scrape URL` prints the ID; `resume-scrape SAVED_TASK_ID` polls that existing task. Run these as separate commands. A failed submission can have an unknown outcome: inspect task history before deciding whether to submit again.

## Site crawling

Create with `SubmitCrawlTask`, then save its `Id` and query `GetCrawlStatus`. `StatusCompleted` only describes the aggregate task; inspect individual page states and counts. `MaxPages` is an upper bound, not a promise of that many results.

Use `OnlyMainContent: false` when navigation content is needed for discovery. `IncludeUrls`, `ExcludeUrls`, depth and query controls follow native service rules. Check actual returned URLs rather than inferring coverage from a successful task status.

```go
func readBatch(ctx context.Context, client *crawl.Client, id, cursor string) (string, error) {
    pages, err := client.ListCrawlPages(ctx, id, crawl.ListCrawlPagesOptions{
        AfterCursor: cursor, Limit: 20,
    })
    if err != nil { return "", err }
    for _, page := range pages.Data {
        fmt.Println(page.Url, page.TaskId, page.Status, page.Success)
        if page.Status != crawl.StatusCompleted || !page.Success || page.Data == nil { continue }
        text, err := page.Data.GetMarkdown(ctx)
        if err != nil { return "", err }
        fmt.Println(text)
    }
    return pages.NextCursor, nil
}
```

`AfterCursor` and `BeforeCursor` are mutually exclusive. `Limit` is 1–100; zero uses the service default. Pass returned cursors unchanged. An empty batch can still have a next cursor; retain order, duplicate URLs and failed rows. If a later read fails, earlier output remains partial.

The executable `resume-crawl SAVED_CRAWL_ID` bounds status checks at 20 and page batches at 100, rejects repeated cursors and uses a two-minute overall context. It retains page failures and partial content, and stops immediately on cancellation, deadlines or output errors. `submit-crawl URL` creates at most two pages at depth one and returns the ID without polling.

## Content and references

| Getter | Result |
|---|---|
| `GetHtml`, `GetMarkdown` | UTF-8 text |
| `GetPdf`, `GetScreenshot` | Binary `[]byte` |
| `GetRawData` | Original bytes; inline text is encoded as UTF-8 |

Getters prefer inline content, including present empty values; otherwise they read the corresponding reference through the same client. Missing/null content, nil results and unattached references return `ErrContentUnavailable`. An empty string or byte slice with no error is successful empty content.

Successful reads are cached. Failed reads, including invalid UTF-8 HTML/Markdown, are not cached; another getter call is a new explicit attempt. Concurrent initial reads may each download the same reference. Copied result DTOs share their initialized cache; returned byte slices are independent copies.

`ReadStorage` and binary getters preserve arbitrary bytes. PDF/screenshots embedded in JSON are Base64-decoded; raw data does not imply structured JSON extraction. API JSON and text reference reads require valid UTF-8.

Keep `Id`/`TaskId`, `Url`, status/success, `Metadata` and `Truncated` with downstream content. `Truncated == nil` means unknown; true denotes service truncation. Known metadata fields are typed; `AdditionalFields` retains unknown fields as `json.RawMessage`. Legacy scalar fields use Go zero values for missing/null values; they do not preserve presence separately.

Configure exported fields before concurrent use. Do not mutate client configuration or result fields while getters are running.

## Request options

| Fields | Behavior |
|---|---|
| `URL`, `Formats` | Required; formats include `markdown`, `html`, `links`, `pdf`, `screenshot`, `rawData`. Exported `Format...` constants are available. |
| `OnlyMainContent` | Always serialized as true/false. Zero value false requests content without main-content filtering. Set true to request filtering. |
| `IncludeTags`, `ExcludeTags`, `RemoveSelectors`, `TargetSelector` | Service-defined extraction filters. |
| `Cookies`, `Headers`, `Proxy`, `Mobile` | Cookies, custom headers, proxy exit country and mobile emulation. |
| `JsRender` | `WaitFor`, `WaitForSelector`, `Actions`; helpers support clicks, input, waits, scrolling, JavaScript and screenshots. |
| `Timeout` | Optional integer milliseconds, 1000–60000; zero omits it. |
| `MaxDepth`, `MaxPages`, `IncludeUrls`, `ExcludeUrls`, `IgnoreQuery` | Crawl-only traversal controls. |

`JsRender.WaitFor` uses integer milliseconds from 1000–30000. Action milliseconds and coordinates must be finite integers; existing float64 field types are retained. Other optional zero-valued fields are omitted, leaving their default behavior to the service. Native request timing is separate from local HTTP/context deadlines.

## Configuration and errors

| Client field | Default / constraint |
|---|---|
| `BaseURL` | `https://api.nstdata.io`; HTTPS origin without credentials, paths, queries or fragments. HTTP is allowed only for loopback tests. |
| `HTTPClient` | Independent client with a 120-second timeout; nil uses that default. Custom transports/timeouts are retained, but redirects are always refused. |
| `MaxResponseBytes` | 1 MiB per decoded API/storage body. Set a positive value below MaxInt64 for larger authorized content. |

Use a deadline-bound context for the whole workflow. Cancellation/deadlines remain detectable with `errors.Is`; stopping locally does not cancel a remote task. Custom transports must honor contexts and retain TLS certificate verification. Response bodies are closed; the configured byte cap excludes additional parsing/copying memory.

| Error | Meaning |
|---|---|
| `*APIError` | HTTP or API-envelope failure; inspect `StatusCode` and optional envelope `Code` with `errors.As`. |
| `ErrContentUnavailable` | Requested content is absent. |
| `ErrResponseTooLarge` | Decoded response exceeds the configured byte limit. |
| `context.Canceled`, `context.DeadlineExceeded` | Local cancellation or deadline; detect through `errors.Is`. |
| Other errors | Invalid local arguments, malformed JSON/fields, invalid text encoding or transport/read failure. |

Task failure is a typed result to inspect, not an HTTP error. SDK error strings omit raw provider bodies, references and transport URLs. Treat webpage content and task fields as untrusted data; keep keys and signed references out of logs.

## API routes

The client uses `x-api-key` at the configured origin. Task IDs cannot contain path separators; storage references and cursor values are query-encoded.

| Operation | Route |
|---|---|
| Sync/async scrape | `POST /api/v1/crawl/scrape`; async adds `?async=true` |
| Scrape result | `GET /api/v1/crawl/scrape/{id}` |
| Crawl/status | `POST /api/v1/crawl`, `GET /api/v1/crawl/{id}` |
| Pages | `GET /api/v1/crawl/{id}/pages`, with cursor/limit options |
| Storage | `GET /api/v1/crawl/storage/read?st=...` |
