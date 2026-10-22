package crawl

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
)

type ScrapeRequestDto struct {
	URL             string            `json:"url"`                       // The URL to scrape
	Cookies         []CookieDto       `json:"cookies,omitempty"`         // Optional cookies to include in the request
	Proxy           *ProxyDto         `json:"proxy,omitempty"`           // Proxy settings, such as exit country
	ExcludeTags     []string          `json:"excludeTags,omitempty"`     // HTML tags to remove entirely (e.g., ["nav", "footer"])
	Formats         []string          `json:"formats,omitempty"`         // Desired output formats (e.g., ["markdown", "html", "links"])
	Headers         map[string]string `json:"headers,omitempty"`         // Custom HTTP headers
	IncludeTags     []string          `json:"includeTags,omitempty"`     // Included tags; extraction behavior is service-defined
	JsRender        *JsRenderDto      `json:"jsRender,omitempty"`        // JavaScript rendering instructions
	Mobile          bool              `json:"mobile,omitempty"`          // Whether to emulate a mobile device
	OnlyMainContent bool              `json:"onlyMainContent,omitempty"` // Automatically extract main article content and remove noise
	RemoveSelectors []string          `json:"removeSelectors,omitempty"` // CSS selectors for elements to remove (e.g., [".ads", "#popup"])
	TargetSelector  string            `json:"targetSelector,omitempty"`  // CSS selector to restrict extraction to a specific element
	Timeout         float64           `json:"timeout,omitempty"`         // Optional integer milliseconds, 1000–60000; zero omits the setting
}

// CrawlRequestDto extends the scrape options with crawl traversal controls
// (MaxDepth, MaxPages, IncludeUrls, ExcludeUrls, IgnoreQuery).
type CrawlRequestDto struct {
	URL             string            `json:"url"`                       // The starting URL to crawl
	Cookies         []CookieDto       `json:"cookies,omitempty"`         // Optional cookies to include in the request
	Proxy           *ProxyDto         `json:"proxy,omitempty"`           // Proxy settings, such as exit country
	ExcludeTags     []string          `json:"excludeTags,omitempty"`     // HTML tags to remove entirely
	ExcludeUrls     []string          `json:"excludeUrls,omitempty"`     // Native URL filters; interpreted by the service
	Formats         []string          `json:"formats,omitempty"`         // Desired output formats (e.g., ["markdown", "html"])
	Headers         map[string]string `json:"headers,omitempty"`         // Custom HTTP headers
	IgnoreQuery     bool              `json:"ignoreQuery,omitempty"`     // Whether to ignore URL query parameters for deduplication
	IncludeTags     []string          `json:"includeTags,omitempty"`     // Included tags; extraction behavior is service-defined
	IncludeUrls     []string          `json:"includeUrls,omitempty"`     // Native URL filters; interpreted by the service
	JsRender        *JsRenderDto      `json:"jsRender,omitempty"`        // JavaScript rendering instructions
	MaxDepth        int               `json:"maxDepth,omitempty"`        // Maximum depth of links to follow
	MaxPages        int               `json:"maxPages,omitempty"`        // Maximum number of pages to crawl
	Mobile          bool              `json:"mobile,omitempty"`          // Whether to emulate a mobile device
	OnlyMainContent bool              `json:"onlyMainContent,omitempty"` // Automatically extract main article content
	RemoveSelectors []string          `json:"removeSelectors,omitempty"` // CSS selectors for elements to remove
	TargetSelector  string            `json:"targetSelector,omitempty"`  // CSS selector to restrict extraction
	Timeout         float64           `json:"timeout,omitempty"`         // Optional integer milliseconds, 1000–60000; zero omits the setting
}

type CookieDto struct {
	Domain   string `json:"domain,omitempty"`   // Domain the cookie belongs to
	HttpOnly bool   `json:"httpOnly,omitempty"` // True if the cookie is HTTP-only
	Name     string `json:"name,omitempty"`     // Name of the cookie
	Path     string `json:"path,omitempty"`     // Path for the cookie
	Secure   bool   `json:"secure,omitempty"`   // True if the cookie requires HTTPS
	Value    string `json:"value,omitempty"`    // Value of the cookie
}

type ProxyDto struct {
	Country string `json:"country,omitempty"` // Proxy exit country code (e.g., "us", "gb")
}

type JsRenderDto struct {
	Actions         []ActionDto `json:"actions,omitempty"`         // List of browser actions to perform
	WaitFor         float64     `json:"waitFor,omitempty"`         // Optional integer milliseconds, 1000–30000; zero omits the setting
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
	UrlPattern           string  `json:"urlPattern,omitempty"`           // Native network-wait pattern interpreted by the service
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
	Id           string         `json:"id,omitempty"`           // Service ID; synchronous results do not guarantee a queryable task
	Code         int            `json:"code,omitempty"`         // Custom status code
	Data         *ScrapeDataDto `json:"data,omitempty"`         // The extracted scrape/crawl data
	ErrorCode    string         `json:"errorCode,omitempty"`    // Error code identifier if any
	ErrorMessage string         `json:"errorMessage,omitempty"` // Detailed error message
	Status       string         `json:"status,omitempty"`       // Current status (e.g., "completed", "failed")
	Success      bool           `json:"success,omitempty"`      // True if the task executed successfully
}

// ErrContentUnavailable distinguishes an absent field from a present empty value.
var ErrContentUnavailable = errors.New("crawl: requested content is unavailable")

// ScrapeDataDto exposes content and opaque storage references. Configure exported fields
// before concurrent use; getters cache successful reads and return independent byte slices.
type ScrapeDataDto struct {
	client        *Client
	Links         []string         `json:"links,omitempty"`
	HtmlRef       string           `json:"htmlRef,omitempty"`
	MarkdownRef   string           `json:"markdownRef,omitempty"`
	Metadata      *PageMetadataDto `json:"metadata,omitempty"`
	PdfRef        string           `json:"pdfRef,omitempty"`
	RawDataRef    string           `json:"rawDataRef,omitempty"`
	ScreenshotRef string           `json:"screenshotRef,omitempty"`
	Truncated     *bool            `json:"truncated,omitempty"`
	cache         *contentCache
}

type contentCache struct {
	mu      sync.Mutex
	content [5][]byte
	loaded  [5]bool
}

func (s *ScrapeDataDto) UnmarshalJSON(data []byte) error {
	type plain ScrapeDataDto
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return errors.New("crawl: invalid content fields")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return errors.New("crawl: content must be an object")
	}
	var content [5][]byte
	var loaded [5]bool
	for i, key := range []string{"html", "markdown", "pdf", "rawData", "screenshot"} {
		raw, ok := fields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			continue
		}
		var text string
		if json.Unmarshal(raw, &text) != nil {
			return errors.New("crawl: inline content must be a string")
		}
		content[i] = []byte(text)
		if i == 2 || i == 4 {
			// Reject malformed encodings instead of returning silently corrupted binary data.
			if bytes.ContainsAny([]byte(text), "\r\n") {
				return errors.New("crawl: invalid inline Base64")
			}
			decoded, err := base64.StdEncoding.Strict().DecodeString(text)
			if err != nil {
				return errors.New("crawl: invalid inline Base64")
			}
			content[i] = decoded
		}
		loaded[i] = true
	}
	if raw, ok := fields["links"]; ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		var links []json.RawMessage
		if json.Unmarshal(raw, &links) != nil {
			return errors.New("crawl: links must be strings")
		}
		for _, link := range links {
			if len(bytes.TrimSpace(link)) == 0 || bytes.TrimSpace(link)[0] != '"' {
				return errors.New("crawl: links must be strings")
			}
		}
	}
	s.Links = value.Links
	s.HtmlRef = value.HtmlRef
	s.MarkdownRef = value.MarkdownRef
	s.Metadata = value.Metadata
	s.PdfRef = value.PdfRef
	s.RawDataRef = value.RawDataRef
	s.ScreenshotRef = value.ScreenshotRef
	s.Truncated = value.Truncated
	s.cache = &contentCache{content: content, loaded: loaded}
	s.client = nil
	return nil
}

func (s *ScrapeDataDto) get(ctx context.Context, index int) ([]byte, error) {
	if s == nil || ctx == nil || s.cache == nil {
		return nil, ErrContentUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.cache.mu.Lock()
	if s.cache.loaded[index] {
		value := append([]byte{}, s.cache.content[index]...)
		s.cache.mu.Unlock()
		return value, nil
	}
	refs := [5]string{s.HtmlRef, s.MarkdownRef, s.PdfRef, s.RawDataRef, s.ScreenshotRef}
	ref, client := refs[index], s.client
	s.cache.mu.Unlock()
	if ref == "" || client == nil {
		return nil, ErrContentUnavailable
	}
	value, err := client.ReadStorage(ctx, ref)
	if err != nil {
		return nil, err
	}
	s.cache.mu.Lock()
	s.cache.content[index] = append([]byte{}, value...)
	s.cache.loaded[index] = true
	s.cache.mu.Unlock()
	return value, nil
}

func (s *ScrapeDataDto) GetHtml(ctx context.Context) (string, error) {
	value, err := s.get(ctx, 0)
	return string(value), err
}
func (s *ScrapeDataDto) GetMarkdown(ctx context.Context) (string, error) {
	value, err := s.get(ctx, 1)
	return string(value), err
}
func (s *ScrapeDataDto) GetPdf(ctx context.Context) ([]byte, error)        { return s.get(ctx, 2) }
func (s *ScrapeDataDto) GetRawData(ctx context.Context) ([]byte, error)    { return s.get(ctx, 3) }
func (s *ScrapeDataDto) GetScreenshot(ctx context.Context) ([]byte, error) { return s.get(ctx, 4) }

type PageMetadataDto struct {
	Canonical   string `json:"canonical,omitempty"`   // Canonical URL of the page
	Description string `json:"description,omitempty"` // Meta description text
	Language    string `json:"language,omitempty"`    // Document language code
	OgImage     string `json:"ogImage,omitempty"`     // OpenGraph preview image URL
	StatusCode  int    `json:"statusCode,omitempty"`  // HTTP response status code
	Title       string `json:"title,omitempty"`       // Page title
	// AdditionalFields preserves service metadata not represented by the typed fields.
	AdditionalFields map[string]json.RawMessage `json:"-"`
}

func (m *PageMetadataDto) UnmarshalJSON(data []byte) error {
	type plain PageMetadataDto
	var value plain
	if json.Unmarshal(data, &value) != nil {
		return errors.New("crawl: invalid page metadata")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || fields == nil {
		return errors.New("crawl: page metadata must be an object")
	}
	for _, key := range []string{"canonical", "description", "language", "ogImage", "statusCode", "title"} {
		delete(fields, key)
	}
	value.AdditionalFields = fields
	*m = PageMetadataDto(value)
	return nil
}

func (m PageMetadataDto) MarshalJSON() ([]byte, error) {
	type plain PageMetadataDto
	data, err := json.Marshal(plain(m))
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for key, value := range m.AdditionalFields {
		switch key {
		case "canonical", "description", "language", "ogImage", "statusCode", "title":
			continue
		}
		fields[key] = value
	}
	return json.Marshal(fields)
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
