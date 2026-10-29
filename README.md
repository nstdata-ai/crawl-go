# Nstdata Crawl SDK for Go

[中文](README_zh.md)

Retrieve webpage content through Nstdata Crawl using a context-aware Go client. No third-party runtime dependencies.

## Features

| Capability | Guide |
|---|---|
| Single-page scraping | Read Markdown, HTML, links, PDF, screenshots or raw bytes. |
| Asynchronous tasks | [Submit once and continue with the saved ID](docs/guide.md#asynchronous-tasks). |
| Site crawling | [Bound traversal and read cursor batches](docs/guide.md#site-crawling). |
| Deferred content | [Read and cache content references](docs/guide.md#content-and-references). |
| Request options | [Rendering, actions, filtering, cookies and proxy country](docs/guide.md#request-options). |

## Installation

Requires Go 1.20+. Use a [maintained Go release](https://go.dev/doc/devel/release#policy) in production.

For a new application, run this once in its directory. Skip it if the project already has `go.mod`:

```sh
go mod init example.com/crawl-example
```

Install the SDK in your application module:

```sh
go get github.com/nstdata-ai/crawl-go@v1.0.0
```

When upgrading from `v0.1.1`, review the [changes](CHANGELOG.md#v100), including explicit `OnlyMainContent` serialization.

## Quick start

Set your API key in the environment (macOS/Linux):

```sh
export NSTDATA_API_TOKEN='your-api-key'
```

Save the following as `main.go`. It submits one scrape task and prints Markdown; service calls consume credits.

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
	token := os.Getenv("NSTDATA_API_TOKEN")
	if token == "" {
		log.Fatal("Set NSTDATA_API_TOKEN in your environment.")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client := crawl.NewClient(token)
	result, err := client.SubmitScrapeTaskSync(ctx, &crawl.ScrapeRequestDto{
		URL: "https://example.com/", Formats: []string{crawl.FormatMarkdown},
		OnlyMainContent: true,
	})
	if err != nil {
		log.Fatal(err)
	}
	if result.Status != crawl.StatusCompleted || !result.Success || result.Data == nil {
		log.Fatal("task did not return successful completed content")
	}
	text, err := result.Data.GetMarkdown(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(text)
	if result.Data.Truncated != nil && *result.Data.Truncated {
		log.Print("content is truncated")
	}
}
```

Run it from the same application module:

```sh
go run .
```

Missing content returns `ErrContentUnavailable`; present empty content succeeds. For complete task flows, method inputs/results and error handling, see the [usage guide](docs/guide.md).

[API documentation](https://docs.nstdata.io/docs/crawl) · [Contributing](CONTRIBUTING.md) · [MIT license](LICENSE)
