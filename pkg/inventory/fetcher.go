package inventory

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Fetcher struct {
	client  *http.Client
	timeout time.Duration
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func NewFetcher(client HTTPClient) *Fetcher {
	var hc *http.Client
	if c, ok := client.(*http.Client); ok {
		hc = c
	} else if client == nil {
		hc = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	return &Fetcher{
		client:  hc,
		timeout: 30 * time.Second,
	}
}

func (f *Fetcher) Fetch(ctx context.Context, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("openapi-cli/0.1.0"))

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	etag := resp.Header.Get("ETag")

	return data, etag, nil
}

func (f *Fetcher) FetchRaw(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
