package crawl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// baseURL is the built-in API endpoint.
const baseURL = "https://api.nstproxy.com"

type Client struct {
	Token      string
	HTTPClient *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		Token:      token,
		HTTPClient: http.DefaultClient,
	}
}

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	var reqBody io.Reader
	if body != nil {
		bs, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(bs)
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, reqBody)
	if err != nil {
		return err
	}

	req.Header.Set("x-api-key", c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		var apiResp struct {
			Code int             `json:"code"`
			Err  bool            `json:"err"`
			Msg  string          `json:"msg"`
			Data json.RawMessage `json:"data"`
		}
		if err = json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
			return err
		}
		if apiResp.Err || apiResp.Code != 200 {
			return fmt.Errorf("API error: code=%d msg=%s", apiResp.Code, apiResp.Msg)
		}
		if len(apiResp.Data) > 0 && string(apiResp.Data) != "null" {
			if err = json.Unmarshal(apiResp.Data, result); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Client) SubmitScrapeTask(ctx context.Context, req *ScrapeRequestDto) (*SubmitResponseDto, error) {
	if req.URL == "" {
		return nil, fmt.Errorf("URL is required")
	}
	if len(req.Formats) == 0 {
		return nil, fmt.Errorf("formats must contain at least one format")
	}
	var res SubmitResponseDto
	if err := c.doRequest(ctx, http.MethodPost, "/api/v1/crawl/scrape/submit", req, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) SubmitScrapeTaskSync(ctx context.Context, req *ScrapeRequestDto) (*TaskResultResponseDto, error) {
	if req.URL == "" {
		return nil, fmt.Errorf("URL is required")
	}
	if len(req.Formats) == 0 {
		return nil, fmt.Errorf("formats must contain at least one format")
	}
	var res TaskResultResponseDto
	if err := c.doRequest(ctx, http.MethodPost, "/api/v1/crawl/scrape/submit-sync", req, &res); err != nil {
		return nil, err
	}
	if res.Data != nil {
		res.Data.client = c
	}
	return &res, nil
}

func (c *Client) SubmitCrawlTask(ctx context.Context, req *CrawlRequestDto) (*SubmitResponseDto, error) {
	if req.URL == "" {
		return nil, fmt.Errorf("URL is required")
	}
	if len(req.Formats) == 0 {
		return nil, fmt.Errorf("formats must contain at least one format")
	}
	var res SubmitResponseDto
	if err := c.doRequest(ctx, http.MethodPost, "/api/v1/crawl/submit", req, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) GetCrawlStatus(ctx context.Context, id string) (*CrawlStatusResponseDto, error) {
	var res CrawlStatusResponseDto
	if err := c.doRequest(ctx, http.MethodGet, "/api/v1/crawl/status/"+url.PathEscape(id), nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) ListCrawlPages(ctx context.Context, id string, opts ListCrawlPagesOptions) (*CrawlPageListResponseDto, error) {
	u, _ := url.Parse("/api/v1/crawl/pages/" + url.PathEscape(id))
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
	var res TaskResultResponseDto
	if err := c.doRequest(ctx, http.MethodGet, "/api/v1/crawl/scrape/result/"+url.PathEscape(id), nil, &res); err != nil {
		return nil, err
	}
	if res.Data != nil {
		res.Data.client = c
	}
	return &res, nil
}

func (c *Client) ReadStorage(ctx context.Context, st string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/crawl/storage/read?st="+url.QueryEscape(st), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", c.Token)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("readStorage error: status=%d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
