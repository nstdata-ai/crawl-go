package crawl

import (
	"context"
	"encoding/json"
)

type ScrapeRequestDto struct {
	URL             string            `json:"url"`                       // The URL to scrape
	Cookies         []CookieDto       `json:"cookies,omitempty"`         // Optional cookies to include in the request
	Country         string            `json:"country,omitempty"`         // Country code for proxy location (e.g., 'us', 'gb')
	ExcludeTags     []string          `json:"excludeTags,omitempty"`     // HTML tags to remove entirely (e.g., ["nav", "footer"])
	Formats         []string          `json:"formats,omitempty"`         // Desired output formats (e.g., ["markdown", "html", "links"])
	Headers         map[string]string `json:"headers,omitempty"`         // Custom HTTP headers
	IncludeTags     []string          `json:"includeTags,omitempty"`     // Tags to keep. WARNING: strips all other tags (keeping text), which may break markdown
	JsRender        *JsRenderDto      `json:"jsRender,omitempty"`        // JavaScript rendering instructions
	Mobile          bool              `json:"mobile,omitempty"`          // Whether to emulate a mobile device
	OnlyMainContent bool              `json:"onlyMainContent,omitempty"` // Automatically extract main article content and remove noise
	Proxy           string            `json:"proxy,omitempty"`           // Proxy type (e.g., datacenter, residential)
	RemoveSelectors []string          `json:"removeSelectors,omitempty"` // CSS selectors for elements to remove (e.g., [".ads", "#popup"])
	TargetSelector  string            `json:"targetSelector,omitempty"`  // CSS selector to restrict extraction to a specific element
	Timeout         float64           `json:"timeout,omitempty"`         // Request timeout in milliseconds
}

// CrawlRequestDto extends the scrape options with crawl traversal controls
// (MaxDepth, MaxPages, IncludeUrls, ExcludeUrls, IgnoreQuery).
type CrawlRequestDto struct {
	URL             string            `json:"url"`                       // The starting URL to crawl
	Cookies         []CookieDto       `json:"cookies,omitempty"`         // Optional cookies to include in the request
	Country         string            `json:"country,omitempty"`         // Country code for proxy location
	ExcludeTags     []string          `json:"excludeTags,omitempty"`     // HTML tags to remove entirely
	ExcludeUrls     []string          `json:"excludeUrls,omitempty"`     // Regex patterns to exclude specific URLs from crawling
	Formats         []string          `json:"formats,omitempty"`         // Desired output formats (e.g., ["markdown", "html"])
	Headers         map[string]string `json:"headers,omitempty"`         // Custom HTTP headers
	IgnoreQuery     bool              `json:"ignoreQuery,omitempty"`     // Whether to ignore URL query parameters for deduplication
	IncludeTags     []string          `json:"includeTags,omitempty"`     // Tags to keep. WARNING: strips all other tags
	IncludeUrls     []string          `json:"includeUrls,omitempty"`     // Regex patterns to only crawl matching URLs
	JsRender        *JsRenderDto      `json:"jsRender,omitempty"`        // JavaScript rendering instructions
	MaxDepth        int               `json:"maxDepth,omitempty"`        // Maximum depth of links to follow
	MaxPages        int               `json:"maxPages,omitempty"`        // Maximum number of pages to crawl
	Mobile          bool              `json:"mobile,omitempty"`          // Whether to emulate a mobile device
	OnlyMainContent bool              `json:"onlyMainContent,omitempty"` // Automatically extract main article content
	Proxy           string            `json:"proxy,omitempty"`           // Proxy type
	RemoveSelectors []string          `json:"removeSelectors,omitempty"` // CSS selectors for elements to remove
	TargetSelector  string            `json:"targetSelector,omitempty"`  // CSS selector to restrict extraction
	Timeout         float64           `json:"timeout,omitempty"`         // Request timeout in milliseconds
}

type CookieDto struct {
	Domain   string `json:"domain,omitempty"`   // Domain the cookie belongs to
	HttpOnly bool   `json:"httpOnly,omitempty"` // True if the cookie is HTTP-only
	Name     string `json:"name,omitempty"`     // Name of the cookie
	Path     string `json:"path,omitempty"`     // Path for the cookie
	Secure   bool   `json:"secure,omitempty"`   // True if the cookie requires HTTPS
	Value    string `json:"value,omitempty"`    // Value of the cookie
}

type JsRenderDto struct {
	Actions         []ActionDto `json:"actions,omitempty"`         // List of browser actions to perform
	WaitFor         float64     `json:"waitFor,omitempty"`         // Global wait time in milliseconds
	WaitForSelector string      `json:"waitForSelector,omitempty"` // CSS selector to wait for before proceeding
}

type ActionDto struct {
	Execute              string  `json:"execute,omitempty"`              // JavaScript code snippet to execute
	FullScreenshot       bool    `json:"fullScreenshot,omitempty"`       // Set to true to capture a full-page screenshot
	Key                  string  `json:"key,omitempty"`                  // Keyboard key to press
	Milliseconds         float64 `json:"milliseconds,omitempty"`         // Milliseconds to wait
	ParticularScreenshot string  `json:"particularScreenshot,omitempty"` // CSS selector of an element to screenshot
	Selector             string  `json:"selector,omitempty"`             // CSS selector for the action target
	Text                 string  `json:"text,omitempty"`                 // Text to type into an input field
	Type                 string  `json:"type,omitempty"`                 // Type of action (e.g., "click", "wait", "fill")
	UrlPattern           string  `json:"urlPattern,omitempty"`           // URL pattern to wait for (regex support)
	WaitSelector         string  `json:"waitSelector,omitempty"`         // CSS selector to wait for
	X                    float64 `json:"x,omitempty"`                    // X coordinate for scroll/click
	Y                    float64 `json:"y,omitempty"`                    // Y coordinate for scroll/click
}

type SubmitResponseDto struct {
	Cached bool   `json:"cached,omitempty"` // True if the response was served from cache
	Code   int    `json:"code,omitempty"`   // HTTP status code
	Id     string `json:"id,omitempty"`     // Task ID
	Status string `json:"status,omitempty"` // Task status message
}

type TaskResultResponseDto struct {
	Code         int            `json:"code,omitempty"`         // Custom status code
	Data         *ScrapeDataDto `json:"data,omitempty"`         // The extracted scrape/crawl data
	ErrorCode    string         `json:"errorCode,omitempty"`    // Error code identifier if any
	ErrorMessage string         `json:"errorMessage,omitempty"` // Detailed error message
	Status       string         `json:"status,omitempty"`       // Current status (e.g., "completed", "failed")
	Success      bool           `json:"success,omitempty"`      // True if the task executed successfully
}

type ScrapeDataDto struct {
	client *Client

	Links         []string         `json:"links,omitempty"`         // List of extracted hyperlinks
	HtmlRef       string           `json:"htmlRef,omitempty"`       // Reference link to fetch the HTML content
	MarkdownRef   string           `json:"markdownRef,omitempty"`   // Reference link to fetch the Markdown content
	Metadata      *PageMetadataDto `json:"metadata,omitempty"`      // Extracted page metadata and SEO tags
	PdfRef        string           `json:"pdfRef,omitempty"`        // Reference link to fetch the PDF
	RawDataRef    string           `json:"rawDataRef,omitempty"`    // Underlying raw response data
	ScreenshotRef string           `json:"screenshotRef,omitempty"` // Reference link to fetch the screenshot

	html       string
	markdown   string
	pdf        []byte
	rawData    []byte
	screenshot []byte
}

func (s *ScrapeDataDto) UnmarshalJSON(data []byte) error {
	type Alias ScrapeDataDto
	aux := &struct {
		Html     string `json:"html,omitempty"`
		Markdown string `json:"markdown,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(s),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	s.html = aux.Html
	s.markdown = aux.Markdown
	return nil
}

func (s *ScrapeDataDto) GetHtml(ctx context.Context) (string, error) {
	if s.html != "" {
		return s.html, nil
	}
	if s.HtmlRef != "" && s.client != nil {
		bs, err := s.client.ReadStorage(ctx, s.HtmlRef)
		if err != nil {
			return "", err
		}
		s.html = string(bs)
		return s.html, nil
	}
	return "", nil
}

func (s *ScrapeDataDto) GetMarkdown(ctx context.Context) (string, error) {
	if s.markdown != "" {
		return s.markdown, nil
	}
	if s.MarkdownRef != "" && s.client != nil {
		bs, err := s.client.ReadStorage(ctx, s.MarkdownRef)
		if err != nil {
			return "", err
		}
		s.markdown = string(bs)
		return s.markdown, nil
	}
	return "", nil
}

func (s *ScrapeDataDto) GetPdf(ctx context.Context) ([]byte, error) {
	if len(s.pdf) > 0 {
		return s.pdf, nil
	}
	if s.PdfRef != "" && s.client != nil {
		bs, err := s.client.ReadStorage(ctx, s.PdfRef)
		if err != nil {
			return nil, err
		}
		s.pdf = bs
		return s.pdf, nil
	}
	return nil, nil
}

func (s *ScrapeDataDto) GetRawData(ctx context.Context) ([]byte, error) {
	if len(s.rawData) > 0 {
		return s.rawData, nil
	}
	if s.RawDataRef != "" && s.client != nil {
		bs, err := s.client.ReadStorage(ctx, s.RawDataRef)
		if err != nil {
			return nil, err
		}
		s.rawData = bs
		return s.rawData, nil
	}
	return nil, nil
}

func (s *ScrapeDataDto) GetScreenshot(ctx context.Context) ([]byte, error) {
	if len(s.screenshot) > 0 {
		return s.screenshot, nil
	}
	if s.ScreenshotRef != "" && s.client != nil {
		bs, err := s.client.ReadStorage(ctx, s.ScreenshotRef)
		if err != nil {
			return nil, err
		}
		s.screenshot = bs
		return s.screenshot, nil
	}
	return nil, nil
}

type PageMetadataDto struct {
	Canonical   string `json:"canonical,omitempty"`   // Canonical URL of the page
	Description string `json:"description,omitempty"` // Meta description text
	Language    string `json:"language,omitempty"`    // Document language code
	OgImage     string `json:"ogImage,omitempty"`     // OpenGraph preview image URL
	StatusCode  int    `json:"statusCode,omitempty"`  // HTTP response status code
	Title       string `json:"title,omitempty"`       // Page title
}

// CrawlStatusResponseDto is the aggregate progress of a crawl session.
type CrawlStatusResponseDto struct {
	Code      int    `json:"code,omitempty"`
	Completed int    `json:"completed,omitempty"`
	Failed    int    `json:"failed,omitempty"`
	Id        string `json:"id,omitempty"`
	Pending   int    `json:"pending,omitempty"`
	Status    string `json:"status,omitempty"`
	Total     int    `json:"total,omitempty"`
}

// CrawlPageItemResponseDto is one page's normalized result within a crawl session.
type CrawlPageItemResponseDto struct {
	Data         *ScrapeDataDto `json:"data,omitempty"`
	ErrorCode    string         `json:"errorCode,omitempty"`
	ErrorMessage string         `json:"errorMessage,omitempty"`
	Status       string         `json:"status,omitempty"`
	Success      bool           `json:"success,omitempty"`
	TaskId       string         `json:"taskId,omitempty"`
	Url          string         `json:"url,omitempty"`
}

// CrawlPageListResponseDto is a cursor-paginated list of crawl page results.
type CrawlPageListResponseDto struct {
	Code       int                        `json:"code,omitempty"`
	Completed  int                        `json:"completed,omitempty"`
	Data       []CrawlPageItemResponseDto `json:"data,omitempty"`
	NextCursor string                     `json:"nextCursor,omitempty"`
	PrevCursor string                     `json:"prevCursor,omitempty"`
	Total      int                        `json:"total,omitempty"`
}

type ListCrawlPagesOptions struct {
	// AfterCursor pages forward; take it from the previous response's NextCursor.
	AfterCursor string
	// BeforeCursor pages backward; take it from the previous response's PrevCursor.
	BeforeCursor string
	// Limit is the page size (server default 20, max 100).
	Limit int
}
