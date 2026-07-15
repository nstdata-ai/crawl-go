package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/nstdata-ai/crawl-go"
)

var token = os.Getenv("NSTDATA_API_TOKEN")

func main() {
	client := crawl.NewClient(token)
	ctx := context.Background()

	// Uncomment the scenario you want to run
	exampleBasicScrape(ctx, client)
	//exampleScreenshot(ctx, client)
	//examplePDF(ctx, client)
	//exampleBrowserActions(ctx, client)
	//exampleCookiesAndHeaders(ctx, client)
	//exampleContentFilter(ctx, client)
	//exampleProxyAndCountry(ctx, client)
	//exampleAsyncPolling(ctx, client)
	//exampleLinksAndRawData(ctx, client)
	//exampleCrawl(ctx, client)
}

// Scenario 1: Basic scrape, synchronously return markdown and html
func exampleBasicScrape(ctx context.Context, client *crawl.Client) {
	res, err := client.SubmitScrapeTaskSync(ctx, &crawl.ScrapeRequestDto{
		URL:     "https://example.com/",
		Formats: []string{crawl.FormatMarkdown, crawl.FormatHTML},
		Timeout: 60000,
	})
	if err != nil {
		log.Fatal(err)
	}
	md, _ := res.Data.GetMarkdown(ctx)
	fmt.Println(md)
}

// Scenario 2: Full page screenshot
func exampleScreenshot(ctx context.Context, client *crawl.Client) {
	res, err := client.SubmitScrapeTaskSync(ctx, &crawl.ScrapeRequestDto{
		URL:     "https://example.com/",
		Formats: []string{crawl.FormatScreenshot},
		JsRender: &crawl.JsRenderDto{
			WaitFor: 3000,
			Actions: []crawl.ActionDto{
				crawl.FullScreenshotAction(),
			},
		},
		Timeout: 60000,
	})
	if err != nil {
		log.Fatal(err)
	}
	ss, _ := res.Data.GetScreenshot(ctx)
	fmt.Println(ss)
}

// Scenario 3: Export page to PDF
func examplePDF(ctx context.Context, client *crawl.Client) {
	res, err := client.SubmitScrapeTaskSync(ctx, &crawl.ScrapeRequestDto{
		URL:     "https://example.com/",
		Formats: []string{crawl.FormatPDF},
		Timeout: 60000,
	})
	if err != nil {
		log.Fatal(err)
	}
	pdf, _ := res.Data.GetPdf(ctx)
	fmt.Println(pdf)
}

// Scenario 4: Browser action orchestration - wait for element, fill form, click, scroll, execute JS, wait for response
func exampleBrowserActions(ctx context.Context, client *crawl.Client) {
	res, err := client.SubmitScrapeTaskSync(ctx, &crawl.ScrapeRequestDto{
		URL:     "https://example.com/",
		Formats: []string{crawl.FormatMarkdown},
		JsRender: &crawl.JsRenderDto{
			WaitForSelector: "body",
			Actions: []crawl.ActionDto{
				crawl.WaitSelectorAction("h1", 5000),
				crawl.FillAction("#search-input", "hello"),
				crawl.ClickAction("#search-button"),
				crawl.WaitAction(1000),
				crawl.ScrollYAction(800),
				crawl.ExecuteAction("window.scrollTo(0, document.body.scrollHeight)"),
				crawl.WaitForRequestAction("*example.com/api/*", 10000),
			},
		},
		Timeout: 60000,
	})
	if err != nil {
		log.Fatal(err)
	}
	md, _ := res.Data.GetMarkdown(ctx)
	fmt.Println(md)
}

// Scenario 5: Bring Cookies, custom headers, simulate mobile device
func exampleCookiesAndHeaders(ctx context.Context, client *crawl.Client) {
	res, err := client.SubmitScrapeTaskSync(ctx, &crawl.ScrapeRequestDto{
		URL:     "https://example.com/",
		Formats: []string{crawl.FormatHTML},
		Cookies: []crawl.CookieDto{
			{Name: "session_id", Value: "abc123", Domain: "example.com", Path: "/", Secure: true},
		},
		Headers: map[string]string{
			"Referer": "https://example.com/home",
		},
		Mobile:  true,
		Timeout: 60000,
	})
	if err != nil {
		log.Fatal(err)
	}
	html, _ := res.Data.GetHtml(ctx)
	fmt.Println(html)
}

// Scenario 6: Main content extraction and deduplication filtering, extract high-quality article content
func exampleContentFilter(ctx context.Context, client *crawl.Client) {
	res, err := client.SubmitScrapeTaskSync(ctx, &crawl.ScrapeRequestDto{
		URL:     "https://example.com/article/123",
		Formats: []string{crawl.FormatMarkdown}, // Depends on standard typesetting tags (p, a, h1, etc.), avoid abusing IncludeTags which causes layout loss
		// TargetSelector: "main article",           // Optional: If the structure is known, precisely locate the main content area
		OnlyMainContent: true,                       // Enable automatic main content extraction, which automatically removes surrounding navigation bars and footers
		RemoveSelectors: []string{".ads", ".share"}, // Manually remove residual ads or share buttons within the main content area
		Timeout:         60000,
	})
	if err != nil {
		log.Fatal(err)
	}
	md, _ := res.Data.GetMarkdown(ctx)
	fmt.Println(md)
}

// Scenario 7: Specify proxy type and exit country
func exampleProxyAndCountry(ctx context.Context, client *crawl.Client) {
	res, err := client.SubmitScrapeTaskSync(ctx, &crawl.ScrapeRequestDto{
		URL:     "https://example.com/",
		Formats: []string{crawl.FormatMarkdown},
		Proxy:   crawl.ProxyDatacenter, // ProxyResidential can also be used (default)
		Country: "us",
		Timeout: 60000,
	})
	if err != nil {
		log.Fatal(err)
	}
	md, _ := res.Data.GetMarkdown(ctx)
	fmt.Println(md)
}

// Scenario 8: Asynchronous submission + polling results, suitable for time-consuming tasks
func exampleAsyncPolling(ctx context.Context, client *crawl.Client) {
	submitted, err := client.SubmitScrapeTask(ctx, &crawl.ScrapeRequestDto{
		URL:     "https://example.com/",
		Formats: []string{crawl.FormatMarkdown},
		Timeout: 60000,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("task id:", submitted.Id)

	for {
		res, err := client.GetScrapeTaskResult(ctx, submitted.Id)
		if err != nil {
			log.Fatal(err)
		}
		switch res.Status {
		case crawl.StatusCompleted:
			md, _ := res.Data.GetMarkdown(ctx)
			fmt.Println(md)
			return
		case crawl.StatusFailed:
			log.Fatalf("task failed: %s %s", res.ErrorCode, res.ErrorMessage)
		default: // processing
			time.Sleep(3 * time.Second)
		}
	}
}

// Scenario 11: Extract all links on the page and unprocessed raw response
func exampleLinksAndRawData(ctx context.Context, client *crawl.Client) {
	res, err := client.SubmitScrapeTaskSync(ctx, &crawl.ScrapeRequestDto{
		URL:     "https://example.com/",
		Formats: []string{crawl.FormatLinks, crawl.FormatRawData},
		Timeout: 60000,
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, link := range res.Data.Links {
		fmt.Println(link)
	}
	fmt.Println("rawData length:", func() int { rd, _ := res.Data.GetRawData(ctx); return len(rd) }())
}

// Scenario 12: Submit batch crawl tasks, traverse the site from the entry page by depth/page count
func exampleCrawl(ctx context.Context, client *crawl.Client) {
	submitted, err := client.SubmitCrawlTask(ctx, &crawl.CrawlRequestDto{
		URL:             "https://example.com/",
		MaxDepth:        3,
		MaxPages:        10,
		IncludeUrls:     []string{"*example.com/blog/*", "*/docs/*"},
		ExcludeUrls:     []string{"*example.com/admin/*"},
		IgnoreQuery:     true, // Ignore URL query parameters when expanding links to avoid duplicate pages
		Formats:         []string{crawl.FormatMarkdown},
		OnlyMainContent: true,
		Proxy:           crawl.ProxyResidential,
		Timeout:         60000,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("crawl task id:", submitted.Id)

	// Poll session progress until terminal state; content of each page can be queried via task list (type=crawl), see scenario 9
	for {
		st, err := client.GetCrawlStatus(ctx, submitted.Id)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("status=%s total=%d completed=%d failed=%d pending=%d\n",
			st.Status, st.Total, st.Completed, st.Failed, st.Pending)
		if st.Status == crawl.StatusCompleted || st.Status == crawl.StatusFailed {
			break
		}
		time.Sleep(3 * time.Second) // scraping / processing
	}

	// Cursor pagination to get crawl content for each page
	cursor := ""
	for {
		pages, err := client.ListCrawlPages(ctx, submitted.Id, crawl.ListCrawlPagesOptions{
			AfterCursor: cursor,
			Limit:       20,
		})
		if err != nil {
			log.Fatal(err)
		}
		for _, page := range pages.Data {
			fmt.Println("==", page.Url, page.Status)
			if page.Success && page.Data != nil {
				md, _ := page.Data.GetMarkdown(ctx)
				fmt.Println(md)
			} else if page.ErrorMessage != "" {
				fmt.Println("error:", page.ErrorCode, page.ErrorMessage)
			}
		}
		if pages.NextCursor == "" {
			break
		}
		cursor = pages.NextCursor
	}
}
