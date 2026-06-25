package newapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Option func(*Client)

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if timeout > 0 {
			c.httpClient.Timeout = timeout
		}
	}
}

type Client struct {
	baseURL     string
	accessToken string
	userID      string
	httpClient  *http.Client
}

func NewClient(baseURL, accessToken, userID string, opts ...Option) *Client {
	c := &Client{
		baseURL:     strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		accessToken: strings.TrimSpace(accessToken),
		userID:      strings.TrimSpace(userID),
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) do(ctx context.Context, method, reqPath string, query url.Values, out any) error {
	if c == nil {
		return fmt.Errorf("newapi client is nil")
	}
	if c.httpClient == nil {
		c.httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	if strings.TrimSpace(c.baseURL) == "" {
		return fmt.Errorf("newapi base url is empty")
	}
	if method != http.MethodGet {
		return fmt.Errorf("newapi client only allows GET")
	}

	u, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("parse newapi base url: %w", err)
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + reqPath
	if query != nil {
		u.RawQuery = query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return fmt.Errorf("create newapi request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if c.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}
	if c.userID != "" {
		req.Header.Set("New-Api-User", c.userID)
		req.Header.Set("X-User-Id", c.userID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("newapi request %s: %w", reqPath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("newapi %s returned http %d", reqPath, resp.StatusCode)
	}

	var envelope Response[json.RawMessage]
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("decode newapi %s response: %w", reqPath, err)
	}
	if !envelope.Success {
		if envelope.Message != "" {
			return fmt.Errorf("newapi %s: %s", reqPath, envelope.Message)
		}
		return fmt.Errorf("newapi %s returned unsuccessful response", reqPath)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("decode newapi %s data: %w", reqPath, err)
	}
	return nil
}

func (c *Client) ListChannels(ctx context.Context) (*ChannelList, error) {
	var data ChannelList
	if err := c.do(ctx, http.MethodGet, "/api/channel/", nil, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

func (c *Client) ListLogs(ctx context.Context, cursor string) (*LogList, error) {
	var query url.Values
	if strings.TrimSpace(cursor) != "" {
		query = url.Values{}
		query.Set("cursor", cursor)
	}
	var data LogList
	if err := c.do(ctx, http.MethodGet, "/api/log/", query, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

func (c *Client) GetLogStat(ctx context.Context) (*LogStat, error) {
	var data LogStat
	if err := c.do(ctx, http.MethodGet, "/api/log/stat", nil, &data); err != nil {
		return nil, err
	}
	return &data, nil
}
