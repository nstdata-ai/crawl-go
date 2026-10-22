// Package crawl provides a context-aware client for Nstdata Crawl.
package crawl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const baseURL = "https://api.nstdata.io"

// ErrResponseTooLarge indicates that a response exceeded MaxResponseBytes.
var ErrResponseTooLarge = errors.New("crawl: response size limit exceeded")

// APIError identifies an HTTP or API-envelope failure without retaining response bodies.
type APIError struct {
	StatusCode int
	Code       int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("crawl: API error (HTTP %d, code %d)", e.StatusCode, e.Code)
}

// Client may be shared after configuration. Do not mutate its fields while requests are running.
type Client struct {
	Token      string
	HTTPClient *http.Client
	// BaseURL defaults to https://api.nstdata.io. HTTP is permitted only on loopback hosts.
	BaseURL string
	// MaxResponseBytes bounds each decoded API or storage body; zero uses 1 MiB.
	MaxResponseBytes int64
}

func NewClient(token string) *Client {
	return &Client{Token: token, HTTPClient: &http.Client{Timeout: 120 * time.Second}}
}

func validID(id string) bool {
	return strings.TrimSpace(id) != "" && id != "." && id != ".." && !strings.ContainsAny(id, "/\\?#")
}

func validateOptions(timeout float64, js *JsRenderDto) error {
	integer := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && math.Trunc(v) == v }
	if timeout != 0 && (!integer(timeout) || timeout < 1000 || timeout > 60000) {
		return errors.New("crawl: timeout must be integer milliseconds from 1000 to 60000")
	}
	if js != nil {
		if js.WaitFor != 0 && (!integer(js.WaitFor) || js.WaitFor < 1000 || js.WaitFor > 30000) {
			return errors.New("crawl: waitFor must be integer milliseconds from 1000 to 30000")
		}
		for _, action := range js.Actions {
			if !integer(action.Milliseconds) || !integer(action.X) || !integer(action.Y) {
				return errors.New("crawl: action milliseconds and coordinates must be finite integers")
			}
		}
	}
	return nil
}

func (c *Client) read(ctx context.Context, method, path string, body interface{}) ([]byte, int, error) {
	if c == nil || ctx == nil {
		return nil, 0, errors.New("crawl: client and context are required")
	}
	if strings.TrimSpace(c.Token) == "" || strings.ContainsAny(c.Token, "\r\n") {
		return nil, 0, errors.New("crawl: API token is required and must not contain newlines")
	}
	origin := c.BaseURL
	if origin == "" {
		origin = baseURL
	}
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" || u.User != nil || strings.ContainsAny(origin, "?#") || (u.Path != "" && u.Path != "/") || u.Opaque != "" {
		return nil, 0, errors.New("crawl: BaseURL must be an API origin without credentials, path, query or fragment")
	}
	ip := net.ParseIP(u.Hostname())
	loopback := strings.EqualFold(u.Hostname(), "localhost") || (ip != nil && ip.IsLoopback())
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return nil, 0, errors.New("crawl: BaseURL requires HTTPS except on loopback")
	}
	limit := c.MaxResponseBytes
	if limit == 0 {
		limit = 1 << 20
	}
	if limit < 1 || limit == math.MaxInt64 {
		return nil, 0, errors.New("crawl: MaxResponseBytes must be positive and less than MaxInt64")
	}
	var reader io.Reader
	if body != nil {
		data, e := json.Marshal(body)
		if e != nil {
			return nil, 0, errors.New("crawl: request could not be encoded")
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(origin, "/")+path, reader)
	if err != nil {
		return nil, 0, errors.New("crawl: request could not be constructed")
	}
	req.Header.Set("x-api-key", c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	transport := c.HTTPClient
	if transport == nil {
		transport = &http.Client{Timeout: 120 * time.Second}
	}
	httpClient := *transport
	// Never forward a key or signed reference to a redirected endpoint.
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		var timeout net.Error
		if errors.As(err, &timeout) && timeout.Timeout() {
			return nil, 0, fmt.Errorf("crawl: HTTP timeout: %w", context.DeadlineExceeded)
		}
		return nil, 0, errors.New("crawl: HTTP transport failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, &APIError{StatusCode: resp.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, resp.StatusCode, ctx.Err()
		}
		var timeout net.Error
		if errors.As(err, &timeout) && timeout.Timeout() {
			return nil, resp.StatusCode, fmt.Errorf("crawl: body timeout: %w", context.DeadlineExceeded)
		}
		return nil, resp.StatusCode, errors.New("crawl: response body could not be read")
	}
	if int64(len(data)) > limit {
		return nil, resp.StatusCode, ErrResponseTooLarge
	}
	return data, resp.StatusCode, nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body, result interface{}) error {
	data, status, err := c.read(ctx, method, path, body)
	if err != nil {
		return err
	}
	var envelope struct {
		Code *int            `json:"code"`
		Err  *bool           `json:"err"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(data, &envelope) != nil || envelope.Code == nil || envelope.Err == nil {
		return errors.New("crawl: invalid API response envelope")
	}
	if *envelope.Err || *envelope.Code != 200 {
		return &APIError{StatusCode: status, Code: *envelope.Code}
	}
	payload := bytes.TrimSpace(envelope.Data)
	if len(payload) == 0 || payload[0] != '{' {
		return errors.New("crawl: API response data must be an object")
	}
	if json.Unmarshal(payload, result) != nil {
		return errors.New("crawl: invalid API response data")
	}
	return nil
}

func (c *Client) SubmitScrapeTask(ctx context.Context, req *ScrapeRequestDto) (*SubmitResponseDto, error) {
	if req == nil || strings.TrimSpace(req.URL) == "" {
		return nil, fmt.Errorf("URL is required")
	}
	if len(req.Formats) == 0 {
		return nil, fmt.Errorf("formats must contain at least one format")
	}
	if err := validateOptions(req.Timeout, req.JsRender); err != nil {
		return nil, err
	}
	var res SubmitResponseDto
	if err := c.doRequest(ctx, http.MethodPost, "/api/v1/crawl/scrape?async=true", req, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) SubmitScrapeTaskSync(ctx context.Context, req *ScrapeRequestDto) (*TaskResultResponseDto, error) {
	if req == nil || strings.TrimSpace(req.URL) == "" {
		return nil, fmt.Errorf("URL is required")
	}
	if len(req.Formats) == 0 {
		return nil, fmt.Errorf("formats must contain at least one format")
	}
	if err := validateOptions(req.Timeout, req.JsRender); err != nil {
		return nil, err
	}
	var res TaskResultResponseDto
	if err := c.doRequest(ctx, http.MethodPost, "/api/v1/crawl/scrape", req, &res); err != nil {
		return nil, err
	}
	if res.Data != nil {
		res.Data.client = c
	}
	return &res, nil
}

func (c *Client) SubmitCrawlTask(ctx context.Context, req *CrawlRequestDto) (*SubmitResponseDto, error) {
	if req == nil || strings.TrimSpace(req.URL) == "" {
		return nil, fmt.Errorf("URL is required")
	}
	if len(req.Formats) == 0 {
		return nil, fmt.Errorf("formats must contain at least one format")
	}
	if err := validateOptions(req.Timeout, req.JsRender); err != nil {
		return nil, err
	}
	var res SubmitResponseDto
	if err := c.doRequest(ctx, http.MethodPost, "/api/v1/crawl", req, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) GetCrawlStatus(ctx context.Context, id string) (*CrawlStatusResponseDto, error) {
	if !validID(id) {
		return nil, fmt.Errorf("a nonempty task ID without path separators is required")
	}
	var res CrawlStatusResponseDto
	if err := c.doRequest(ctx, http.MethodGet, "/api/v1/crawl/"+url.PathEscape(id), nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) ListCrawlPages(ctx context.Context, id string, opts ListCrawlPagesOptions) (*CrawlPageListResponseDto, error) {
	if !validID(id) {
		return nil, fmt.Errorf("a nonempty task ID without path separators is required")
	}
	if opts.Limit < 0 || opts.Limit > 100 || (opts.AfterCursor != "" && opts.BeforeCursor != "") {
		return nil, fmt.Errorf("invalid pagination options")
	}

	u, _ := url.Parse("/api/v1/crawl/" + url.PathEscape(id) + "/pages")
	q := u.Query()
	if opts.AfterCursor != "" {
		q.Set("after_cursor", opts.AfterCursor)
	}
	if opts.BeforeCursor != "" {
		q.Set("before_cursor", opts.BeforeCursor)
	}
	if opts.Limit > 0 {
		q.Set("limit", fmt.Sprint(opts.Limit))
	}
	u.RawQuery = q.Encode()

	var res CrawlPageListResponseDto
	if err := c.doRequest(ctx, http.MethodGet, u.String(), nil, &res); err != nil {
		return nil, err
	}
	for i := range res.Data {
		if res.Data[i].Data != nil {
			res.Data[i].Data.client = c
		}
	}
	return &res, nil
}

func (c *Client) GetScrapeTaskResult(ctx context.Context, id string) (*TaskResultResponseDto, error) {
	if !validID(id) {
		return nil, fmt.Errorf("a nonempty task ID without path separators is required")
	}
	var res TaskResultResponseDto
	if err := c.doRequest(ctx, http.MethodGet, "/api/v1/crawl/scrape/"+url.PathEscape(id), nil, &res); err != nil {
		return nil, err
	}
	if res.Data != nil {
		res.Data.client = c
	}
	return &res, nil
}

// ReadStorage retrieves an opaque reference through the authenticated API origin.
func (c *Client) ReadStorage(ctx context.Context, st string) ([]byte, error) {
	if strings.TrimSpace(st) == "" {
		return nil, errors.New("crawl: storage reference is required")
	}
	data, _, err := c.read(ctx, http.MethodGet, "/api/v1/crawl/storage/read?st="+url.QueryEscape(st), nil)
	return data, err
}
