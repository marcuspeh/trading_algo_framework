package broker

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
