package execution

import (
	"github.com/marcuspeh/trading-framework/broker"
)

// BrokerClient is the minimal surface the live Executor needs from a
// broker client. It is an alias of broker.Client, so any client that
// implements the canonical broker interface - including
// *clients.BrokerClient - can be passed to app.WithLive.
type BrokerClient = broker.Client

// The broker request/response types are re-exported here for
// convenience, since the execution package is where strategy code
// interacts with them.
type (
	MarketOrderRequest = broker.MarketOrderRequest
	LimitOrderRequest  = broker.LimitOrderRequest
	OrderResponse      = broker.OrderResponse
	PositionResponse   = broker.PositionResponse
	OpenOrderResponse  = broker.OpenOrderResponse
	AccountNAVResponse = broker.AccountNAVResponse
)
