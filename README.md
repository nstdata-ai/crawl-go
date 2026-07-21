# nstdata crawl SDK (Golang)

[中文版](README_zh.md)

Golang SDK for nstdata crawl, an advanced web scraping and crawling service.

## Installation

```bash
go get github.com/nstdata-ai/crawl-go
```

## Features

- **Basic Scraping:** Get HTML and Markdown synchronously.
- **Screenshots & PDFs:** Take full-page screenshots and export pages as PDFs.
- **Browser Actions:** Orchestrate browser actions like wait for elements, fill forms, click buttons, scroll, and execute JS.
- **Custom Configuration:** Bring your own cookies, headers, and simulate mobile devices.
- **Content Filtering:** Automatically extract high-quality main content, filter out ads and navigational elements.
- **Proxy Location:** Specify proxy exit country.
- **Asynchronous Execution & Polling:** Submit time-consuming tasks and poll for results.
- **Batch Crawling:** Traverse entire sites with depth and page limits.

## Quick Start

See [example.go](./example/example.go) for a complete list of usage examples.

```go
package main

import (
	"context"
	"fmt"
	"github.com/nstdata-ai/crawl-go"
	"log"
)

func main() {
	// Initialize the client with your API token
	client := crawl.NewClient("YOUR_API_TOKEN")
	
	// Submit a basic scraping task
	res, err := client.SubmitScrapeTaskSync(context.Background(), &crawl.ScrapeRequestDto{
		URL:     "https://example.com/",
		Formats: []string{crawl.FormatMarkdown},
		Timeout: 60000,
	})
	
	if err != nil {
		log.Fatal(err)
	}
	
	// Output the resulting markdown
	md, _ := res.Data.GetMarkdown(context.Background())
	fmt.Println(md)
}
```
