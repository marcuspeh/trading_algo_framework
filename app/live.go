package app

import (
	"context"
	"time"

	tf "github.com/marcuspeh/trading-framework"
	"github.com/marcuspeh/trading-framework/live"
)

// brokerAdapter converts the app.BrokerClient interface (which uses
// the framework's broker request/response shapes) into the
// live.BrokerClient interface expected by the live engine.
type brokerAdapter struct{ inner BrokerClient }

func (b *brokerAdapter) PlaceMarketOrder(ctx context.Context, req live.MarketOrderRequest, idem string) (*live.OrderResponse, error) {
	r, err := b.inner.PlaceMarketOrder(ctx, MarketOrderRequest{
		Symbol:        req.Symbol,
		Action:        req.Action,
		TotalQuantity: req.TotalQuantity,
		OrderRef:      req.OrderRef,
		OutsideRth:    req.OutsideRth,
	}, idem)
	if err != nil {
		return nil, err
	}
	return toLiveOrderResponse(r), nil
}

func (b *brokerAdapter) PlaceLimitOrder(ctx context.Context, req live.LimitOrderRequest, idem string) (*live.OrderResponse, error) {
	r, err := b.inner.PlaceLimitOrder(ctx, LimitOrderRequest{
		MarketOrderRequest: MarketOrderRequest{
			Symbol:        req.Symbol,
			Action:        req.Action,
			TotalQuantity: req.TotalQuantity,
			OrderRef:      req.OrderRef,
			OutsideRth:    req.OutsideRth,
		},
		LmtPrice: req.LmtPrice,
	}, idem)
	if err != nil {
		return nil, err
	}
	return toLiveOrderResponse(r), nil
}

func (b *brokerAdapter) CancelOrder(ctx context.Context, orderID int) error {
	return b.inner.CancelOrder(ctx, orderID)
}

func (b *brokerAdapter) GetPositions(ctx context.Context) ([]live.PositionResponse, error) {
	r, err := b.inner.GetPositions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]live.PositionResponse, len(r))
	for i, p := range r {
		out[i] = live.PositionResponse{
			ConID:       p.ConID,
			Symbol:      p.Symbol,
			Quantity:    p.Quantity,
			AverageCost: p.AverageCost,
		}
	}
	return out, nil
}

func (b *brokerAdapter) GetOpenOrders(ctx context.Context) ([]live.OpenOrderResponse, error) {
	r, err := b.inner.GetOpenOrders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]live.OpenOrderResponse, len(r))
	for i, o := range r {
		out[i] = live.OpenOrderResponse{
			PermID:        o.PermID,
			OrderID:       o.OrderID,
			Symbol:        o.Symbol,
			Action:        o.Action,
			TotalQuantity: o.TotalQuantity,
			LmtPrice:      o.LmtPrice,
			AuxPrice:      o.AuxPrice,
			Status:        o.Status,
			Remaining:     o.Remaining,
		}
	}
	return out, nil
}

func (b *brokerAdapter) GetAccountNAV(ctx context.Context) (*live.AccountNAVResponse, error) {
	r, err := b.inner.GetAccountNAV(ctx)
	if err != nil {
		return nil, err
	}
	return &live.AccountNAVResponse{
		Account:  r.Account,
		NAV:      r.NAV,
		Currency: r.Currency,
	}, nil
}

func toLiveOrderResponse(r *OrderResponse) *live.OrderResponse {
	if r == nil {
		return nil
	}
	return &live.OrderResponse{
		PermID:  r.PermID,
		OrderID: r.OrderID,
		Status:  r.Status,
	}
}

// newLiveEngine wires a live.Engine from the App's configured options.
func newLiveEngine(a *App) (*live.Engine, error) {
	clock := liveClockAdapter{inner: a.liveClock}
	market := marketAdapter{inner: a.market}
	constituents := constituentsAdapter{inner: a.constituents}
	broker := &brokerAdapter{inner: a.broker}

	return live.New(live.Config{
		Broker:       broker,
		Market:       market,
		Constituents: constituents,
		Config:       a.config,
		Logger:       a.logger,
		IsTradingDay: a.tradingDay,
		Risk:         a.risk,
		Clock:        clock,
	})
}

// liveClockAdapter bridges the App's Clock interface (Now) to the
// live package's Clock interface (Now).
type liveClockAdapter struct{ inner Clock }

func (l liveClockAdapter) Now() time.Time { return l.inner.Now() }

// marketAdapter bridges MarketDataClient to live.MarketDataClient.
type marketAdapter struct{ inner MarketDataClient }

func (m marketAdapter) Bars(ctx context.Context, symbol string, lookback int, asOf time.Time) ([]tf.Bar, error) {
	return m.inner.Bars(ctx, symbol, lookback, asOf)
}

// constituentsAdapter bridges ConstituentsClient to live.ConstituentsClient.
type constituentsAdapter struct{ inner ConstituentsClient }

func (c constituentsAdapter) Constituents(ctx context.Context, etf string, asOf time.Time) ([]string, error) {
	return c.inner.Constituents(ctx, etf, asOf)
}
