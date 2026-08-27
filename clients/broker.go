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
)

// Broker HTTP client and request/response types.

type BrokerConfig struct {
	BaseURL string
	Port    int
}

type BrokerClient struct {
	baseURL string
	client  *http.Client
}

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

func (c *BrokerClient) PlaceLimitOrder(ctx context.Context, req LimitOrderRequest, idempotencyKey string) (*OrderResponse, error) {
	var res OrderResponse
	if err := c.doRequest(ctx, "POST", "/orders/limit", req, &res, idempotencyKey); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *BrokerClient) PlaceMarketOrder(ctx context.Context, req MarketOrderRequest, idempotencyKey string) (*OrderResponse, error) {
	var res OrderResponse
	if err := c.doRequest(ctx, "POST", "/orders/market", req, &res, idempotencyKey); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *BrokerClient) PlaceBracketOrder(ctx context.Context, req BracketOrderRequest, idempotencyKey string) (*BracketOrderResponse, error) {
	var res BracketOrderResponse
	if err := c.doRequest(ctx, "POST", "/orders/bracket", req, &res, idempotencyKey); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *BrokerClient) CancelOrder(ctx context.Context, orderID int) error {
	return c.doRequest(ctx, "DELETE", fmt.Sprintf("/orders/%d", orderID), nil, nil, "")
}

func (c *BrokerClient) UpdateOrder(ctx context.Context, orderID int, req UpdateOrderRequest) (*OrderResponse, error) {
	var res OrderResponse
	if err := c.doRequest(ctx, "PUT", fmt.Sprintf("/orders/%d", orderID), req, &res, ""); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *BrokerClient) GetPositions(ctx context.Context) ([]PositionResponse, error) {
	var res []PositionResponse
	if err := c.doRequest(ctx, "GET", "/positions", nil, &res, ""); err != nil {
		return nil, err
	}
	return res, nil
}

func (c *BrokerClient) GetAccountNAV(ctx context.Context) (*AccountNAVResponse, error) {
	var res AccountNAVResponse
	if err := c.doRequest(ctx, "GET", "/account/nav", nil, &res, ""); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *BrokerClient) GetOpenOrders(ctx context.Context) ([]OpenOrderResponse, error) {
	var res []OpenOrderResponse
	if err := c.doRequest(ctx, "GET", "/orders/open", nil, &res, ""); err != nil {
		return nil, err
	}
	return res, nil
}

func (c *BrokerClient) GetOHLC(ctx context.Context, symbols []string) ([]OHLCResponse, error) {
	var res []OHLCResponse
	symbolsStr := strings.Join(symbols, ",")
	path := fmt.Sprintf("/ohlc?symbols=%s", symbolsStr)
	if err := c.doRequest(ctx, "GET", path, nil, &res, ""); err != nil {
		return nil, err
	}
	return res, nil
}

type OrderRequest struct {
	Symbol        string  `json:"symbol"`
	Action        string  `json:"action"`
	TotalQuantity float64 `json:"totalQuantity"`
	OrderRef      string  `json:"orderRef,omitempty"`
	OutsideRth    bool    `json:"outsideRth"`
}

type LimitOrderRequest struct {
	OrderRequest
	LmtPrice float64 `json:"lmtPrice"`
}

type MarketOrderRequest struct {
	OrderRequest
}

type BracketOrderRequest struct {
	OrderRequest
	LmtPrice float64 `json:"lmtPrice"`
	TPPrice  float64 `json:"tpPrice"`
	SLPrice  float64 `json:"slPrice"`
}

type UpdateOrderRequest struct {
	LmtPrice      *float64 `json:"lmtPrice,omitempty"`
	AuxPrice      *float64 `json:"auxPrice,omitempty"`
	TotalQuantity *float64 `json:"totalQuantity,omitempty"`
}

type OrderResponse struct {
	PermId  int    `json:"permId"`
	OrderId int    `json:"orderId"`
	Status  string `json:"status"`
}

type BracketOrderResponse struct {
	ParentPermId  int    `json:"parentPermId"`
	ParentOrderId int    `json:"parentOrderId"`
	Status        string `json:"status"`
}

type PositionResponse struct {
	ConId       int     `json:"conId"`
	Symbol      string  `json:"symbol"`
	Quantity    float64 `json:"quantity"`
	AverageCost float64 `json:"averageCost"`
}

type AccountNAVResponse struct {
	Account  string  `json:"account"`
	NAV      float64 `json:"nav"`
	Currency string  `json:"currency"`
}

type OpenOrderResponse struct {
	PermId        int      `json:"permId"`
	OrderId       int      `json:"orderId"`
	Symbol        string   `json:"symbol"`
	Action        string   `json:"action"`
	TotalQuantity float64  `json:"totalQuantity"`
	LmtPrice      *float64 `json:"lmtPrice,omitempty"`
	AuxPrice      *float64 `json:"auxPrice,omitempty"`
	Status        string   `json:"status"`
	Remaining     float64  `json:"remaining"`
}

type OHLCResponse struct {
	Symbol string  `json:"symbol"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
	Date   string  `json:"date"`
}
