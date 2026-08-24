package etf

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type ETFClient struct {
	baseURL string
}

func NewETFClient(baseURL string) *ETFClient {
	if baseURL == "" {
		baseURL = "http://localhost:8001"
	}
	return &ETFClient{
		baseURL: baseURL,
	}
}

func (c *ETFClient) GetConstituents(ctx context.Context, symbol string) ([]Constituent, error) {
	url := fmt.Sprintf("%s/constituents?symbol=%s", c.baseURL, symbol)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch from remote: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Detail string `json:"detail"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil {
			return nil, fmt.Errorf("remote server returned error: %s", errResp.Detail)
		}
		return nil, fmt.Errorf("remote server returned status: %d", resp.StatusCode)
	}

	var result ConstituentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Constituents, nil
}
