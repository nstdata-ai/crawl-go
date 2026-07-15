package crawl

// Supported formats for ScrapeRequestDto
const (
	FormatHTML       = "html"
	FormatMarkdown   = "markdown"
	FormatLinks      = "links"
	FormatPDF        = "pdf"
	FormatScreenshot = "screenshot"
	FormatRawData    = "rawData"
)

// Task types
const (
	TaskTypeScrape = "scrape"
	TaskTypeCrawl  = "crawl"
)

// Task / crawl session statuses
const (
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
	// StatusScraping means a crawl session is still fetching pages.
	StatusScraping = "scraping"
)

// Action types for ActionDto in JsRenderDto
const (
	// ActionTypeClick clicks an element. Requires Selector.
	ActionTypeClick = "click"
	// ActionTypeWait waits a fixed time. Requires Milliseconds.
	ActionTypeWait = "wait"
	// ActionTypeWaitSelector waits until an element is visible. Requires Selector; Milliseconds optional.
	ActionTypeWaitSelector = "waitSelector"
	// ActionTypeScrollX scrolls horizontally by X pixels.
	ActionTypeScrollX = "scrollX"
	// ActionTypeScrollY scrolls vertically by Y pixels.
	ActionTypeScrollY = "scrollY"
	// ActionTypeScrollTo scrolls to an element. Requires Selector.
	ActionTypeScrollTo = "scrollTo"
	// ActionTypeFill fills an input. Requires Selector and Text.
	ActionTypeFill = "fill"
	// ActionTypeExecute runs JavaScript on the page. Requires Execute.
	ActionTypeExecute = "execute"
	// ActionTypeScreenshot captures a screenshot. Set FullScreenshot for the full
	// page, ParticularScreenshot for a specific element, otherwise the viewport.
	ActionTypeScreenshot = "screenshot"
	// ActionTypeWaitForRequestCompletion waits for a matching network response.
	// Requires UrlPattern (supports * wildcard); Milliseconds optional.
	ActionTypeWaitForRequestCompletion = "waitForRequestCompletion"
)

// Proxy types for ScrapeRequestDto.Proxy (defaults to residential when empty)
const (
	ProxyResidential = "residential"
	ProxyDatacenter  = "datacenter"
)
