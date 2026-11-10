// Package clients contains HTTP clients and adapters that talk to the
// external services (broker, ETF universe) and present them in the
// shape the framework's internal interfaces expect.
//
// Strategies should not import this package directly. The App wires
// the adapters on their behalf.
//
// This package has no submodules - all broker/etf/adapter code lives
// in a single Go package.
package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/marcuspeh/trading-framework/broker"
)

// Broker HTTP client and request/response types.
//
// The request/response types live in the broker package so that this
// client and the framework's execution package speak exactly the same
// types. *BrokerClient therefore satisfies broker.Client and can be
// passed directly to app.WithLive.

type BrokerConfig struct {
	BaseURL string
	Port    int
}

// BrokerClient implements broker.Client over HTTP.
type BrokerClient struct {
	baseURL string
	client  *http.Client
}

// Compile-time proof that the HTTP client satisfies the framework's
// broker interface.
var _ broker.Client = (*BrokerClient)(nil)

func NewBrokerClient(ctx context.Context, config BrokerConfig) *BrokerClient {
	return &BrokerClient{
		baseURL: fmt.Sprintf("%s:%d", config.BaseURL, config.Port),
		client:  &http.Client{},
	}
}

func (c *BrokerClient) doRequest(ctx context.Context, method, path string, body interface{}, response interface{}, idempotencyKey string) error {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("X-Idempotency-Key", idempotencyKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	if response != nil {
		if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

func (c *BrokerClient) PlaceLimitOrder(ctx context.Context, req broker.LimitOrderRequest, idempotencyKey string) (*broker.OrderResponse, error) {
	var res broker.OrderResponse
	if err := c.doRequest(ctx, "POST", "/orders/limit", req, &res, idempotencyKey); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *BrokerClient) PlaceMarketOrder(ctx context.Context, req broker.MarketOrderRequest, idempotencyKey string) (*broker.OrderResponse, error) {
	var res broker.OrderResponse
	if err := c.doRequest(ctx, "POST", "/orders/market", req, &res, idempotencyKey); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *BrokerClient) PlaceBracketOrder(ctx context.Context, req broker.BracketOrderRequest, idempotencyKey string) (*broker.BracketOrderResponse, error) {
	var res broker.BracketOrderResponse
	if err := c.doRequest(ctx, "POST", "/orders/bracket", req, &res, idempotencyKey); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *BrokerClient) CancelOrder(ctx context.Context, orderID int) error {
	return c.doRequest(ctx, "DELETE", fmt.Sprintf("/orders/%d", orderID), nil, nil, "")
}

func (c *BrokerClient) UpdateOrder(ctx context.Context, orderID int, req broker.UpdateOrderRequest) (*broker.OrderResponse, error) {
	var res broker.OrderResponse
	if err := c.doRequest(ctx, "PUT", fmt.Sprintf("/orders/%d", orderID), req, &res, ""); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *BrokerClient) GetPositions(ctx context.Context) ([]broker.PositionResponse, error) {
	var res []broker.PositionResponse
	if err := c.doRequest(ctx, "GET", "/positions", nil, &res, ""); err != nil {
		return nil, err
	}
	return res, nil
}

func (c *BrokerClient) GetAccountNAV(ctx context.Context) (*broker.AccountNAVResponse, error) {
	var res broker.AccountNAVResponse
	if err := c.doRequest(ctx, "GET", "/account/nav", nil, &res, ""); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *BrokerClient) GetOpenOrders(ctx context.Context) ([]broker.OpenOrderResponse, error) {
	var res []broker.OpenOrderResponse
	if err := c.doRequest(ctx, "GET", "/orders/open", nil, &res, ""); err != nil {
		return nil, err
	}
	return res, nil
}

func (c *BrokerClient) GetOHLC(ctx context.Context, symbols []string) ([]broker.OHLCResponse, error) {
	var res []broker.OHLCResponse
	symbolsStr := strings.Join(symbols, ",")
	path := fmt.Sprintf("/ohlc?symbols=%s", symbolsStr)
	if err := c.doRequest(ctx, "GET", path, nil, &res, ""); err != nil {
		return nil, err
	}
	return res, nil
}
