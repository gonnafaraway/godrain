package pprofclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client fetches heap profiles from a Go process exposing net/http/pprof.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// New returns a Client for the given pprof base URL.
func New(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// FetchHeap downloads a heap profile. When gc is true, requests ?gc=1 so the
// profile reflects retained (inuse) memory after a forced garbage collection.
func (c *Client) FetchHeap(ctx context.Context, gc bool) ([]byte, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}

	path := u.Path
	switch {
	case path == "" || path == "/":
		u.Path = "/debug/pprof/heap"
	case !strings.HasSuffix(path, "/heap"):
		u.Path = strings.TrimRight(path, "/") + "/debug/pprof/heap"
	}

	q := u.Query()
	if gc {
		q.Set("gc", "1")
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch heap profile: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("heap profile: status %d: %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read heap profile: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("empty heap profile from %s", u.String())
	}
	return data, nil
}

// WaitReady polls until the pprof endpoint responds or the context is done.
func (c *Client) WaitReady(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = 200 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastErr error
	for {
		_, err := c.FetchHeap(ctx, false)
		if err == nil {
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("pprof not ready: %w: %v", ctx.Err(), lastErr)
			}
			return fmt.Errorf("pprof not ready: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
