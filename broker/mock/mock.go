// Package mock is an in-memory broker for backtests.
//
// It implements broker.Client, so a backtest runs the same order
// plumbing a live run does: the same executor translates a strategy's
// order into broker requests, and the same broker contract carries
// them. The only difference is where the requests go.
//
// The mock holds a portfolio.Backtest for the accounting and an order
// book for the resting orders. It never re-implements the portfolio's
// fill semantics - it calls ApplyFill, MarkPrice and ChargeFee - so
// the two cannot drift.
package mock

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/marcuspeh/trading-framework/broker"
	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/market"
	"github.com/marcuspeh/trading-framework/order"
	"github.com/marcuspeh/trading-framework/portfolio"
)

// Config configures a Mock.
type Config struct {
	// Portfolio holds the simulated positions and cash.
	Portfolio *portfolio.Backtest

	// Bars supplies the price a fill happens at.
	Bars market.BarProvider

	// Session resolves the session a timestamp belongs to.
	Session *clock.Session

	// SlippageBps is the adverse price movement applied to every fill,
	// in basis points of the fill price.
	SlippageBps float64

	// Commission is charged per fill, in account currency.
	Commission float64

	// CommissionPerShare is charged per unit of quantity.
	CommissionPerShare float64
}

// Mock is an in-memory broker.
//
// It is safe for concurrent use.
type Mock struct {
	mu        sync.Mutex
	portfolio *portfolio.Backtest
	bars      market.BarProvider
	session   *clock.Session

	slippageBps        float64
	commission         float64
	commissionPerShare float64

	nextID int
	orders map[int]*restingOrder

	// byKey maps an idempotency key to the order it produced, so a
	// repeated submission returns the first response - the same thing
	// a real broker does.
	byKey map[string]int

	// finalStatus records how an order that is no longer resting
	// ended, so a repeated idempotency key reports it accurately.
	finalStatus map[int]string

	// fillPrices records the price each order filled at.
	fillPrices map[int]float64
}

// Compile-time proof that the mock satisfies the broker interface.
var _ broker.Client = (*Mock)(nil)

// orderKind says how an order triggers against a bar.
//
// A take-profit and a stop-loss are both protective legs of a bracket,
// but they trigger in opposite directions: a take-profit on a long
// fires when the price rises to it, and a stop-loss fires when the
// price falls to it. Treating both as limit orders would make a
// stop-loss fire on the wrong side of the market.
type orderKind int

const (
	// kindEntry is a limit order: it fills when the market trades
	// through its price.
	kindEntry orderKind = iota

	// kindTakeProfit is a protective limit: it fills when the market
	// reaches the target in the trade's favour.
	kindTakeProfit

	// kindStopLoss is a protective stop: it fills when the market
	// reaches the stop against the trade.
	kindStopLoss
)

// restingOrder is an order waiting to fill.
type restingOrder struct {
	id       int
	order    order.Order
	limit    float64
	quantity int

	// side is the side the order trades on when it fills. An entry
	// trades on the order's own side; a protective leg trades on the
	// opposite side, because it closes a position rather than opening
	// one.
	side order.Side

	// evaluableFrom is the session whose close this order is first
	// evaluated against. It is the next session to close after the
	// order was submitted, so an order can never fill at a price
	// printed before it existed.
	evaluableFrom time.Time

	// lastEvaluated is the session this order was last evaluated
	// against, so it is evaluated once per session and never twice.
	lastEvaluated time.Time

	// takeProfit and stopLoss are the protective legs of a bracket.
	// They become live orders when the entry fills.
	takeProfit float64
	stopLoss   float64

	// group links the legs of a bracket together, so that when one
	// fills the others are cancelled - as a real bracket's are.
	group int

	// kind says how the order triggers.
	kind orderKind

	// filledAt is the price the order filled at, when it filled.
	filledAt float64
}

// New returns a Mock broker.
func New(cfg Config) *Mock {
	p := cfg.Portfolio
	if p == nil {
		p = portfolio.NewBacktest(0)
	}
	s := cfg.Session
	if s == nil {
		s = clock.NewSession(clock.NewBacktest(time.Now()), nil, nil)
	}
	return &Mock{
		portfolio:         p,
		bars:              cfg.Bars,
		session:           s,
		slippageBps:       cfg.SlippageBps,
		commission:        cfg.Commission,
		commissionPerShare: cfg.CommissionPerShare,
		nextID:            1000,
		orders:            map[int]*restingOrder{},
		byKey:             map[string]int{},
		finalStatus:       map[int]string{},
		fillPrices:        map[int]float64{},
	}
}

// Portfolio returns the simulated portfolio.
func (m *Mock) Portfolio() *portfolio.Backtest { return m.portfolio }

func (m *Mock) Health(_ context.Context) (*broker.HealthResponse, error) {
	return &broker.HealthResponse{Status: "ok", IBKRConnected: true}, nil
}

// PlaceMarketOrder fills immediately at the anchor session's close -
// the price the strategy just read.
func (m *Mock) PlaceMarketOrder(ctx context.Context, req broker.MarketOrderRequest, key string) (*broker.OrderResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if id, ok := m.byKey[key]; ok && key != "" {
		return &broker.OrderResponse{OrderId: id, Status: "Filled"}, nil
	}

	session := m.session.Anchor(m.session.Clock().Now())
	price, ok := m.closePrice(req.Contract.Symbol, session)
	if !ok {
		return nil, fmt.Errorf("mock broker: no bar for %s on %s", req.Contract.Symbol, session.Format("2006-01-02"))
	}

	id := m.nextID
	m.nextID++
	if key != "" {
		m.byKey[key] = id
	}

	side := sideOf(req.Action)
	qty := float64(req.TotalQuantity)
	fillPrice := m.applySlippage(price, side)
	m.portfolio.ApplyFill(req.Contract.Symbol, side, qty, fillPrice)
	m.portfolio.MarkPrice(req.Contract.Symbol, price)
	m.chargeFee(qty)
	m.fillPrices[id] = fillPrice

	return &broker.OrderResponse{OrderId: id, Status: "Filled"}, nil
}

// PlaceLimitOrder fills if the session's bar trades through the limit,
// and otherwise rests until it does.
func (m *Mock) PlaceLimitOrder(ctx context.Context, req broker.LimitOrderRequest, key string) (*broker.OrderResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if id, ok := m.byKey[key]; ok && key != "" {
		return &broker.OrderResponse{OrderId: id, Status: m.statusOf(id)}, nil
	}

	id := m.nextID
	m.nextID++
	if key != "" {
		m.byKey[key] = id
	}

	now := m.session.Clock().Now()
	resting := &restingOrder{
		id:    id,
		order: order.Order{
			Symbol:   req.Contract.Symbol,
			Side:     sideOf(req.Action),
			Quantity: float64(req.TotalQuantity),
			Type:     order.Limit,
			TIF:      req.TIF,
		},
		limit:         req.LmtPrice,
		quantity:      req.TotalQuantity,
		side:          sideOf(req.Action),
		evaluableFrom: m.nextSessionToClose(now),
	}
	m.orders[id] = resting

	// A limit order is not evaluated against the session it was
	// submitted in: that session's bar may already be complete, and an
	// order must not fill at a price printed before it existed.
	return &broker.OrderResponse{OrderId: id, Status: "Submitted"}, nil
}

// PlaceBracketOrder submits a limit order with protective legs. The
// parent rests like any other limit order; the legs trigger against a
// later session's bar.
func (m *Mock) PlaceBracketOrder(ctx context.Context, req broker.BracketOrderRequest, key string) (*broker.BracketOrderResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if id, ok := m.byKey[key]; ok && key != "" {
		return &broker.BracketOrderResponse{ParentOrderId: id, Status: m.statusOf(id)}, nil
	}

	id := m.nextID
	m.nextID++
	if key != "" {
		m.byKey[key] = id
	}

	now := m.session.Clock().Now()
	m.orders[id] = &restingOrder{
		id:    id,
		order: order.Order{
			Symbol:   req.Contract.Symbol,
			Side:     sideOf(req.Action),
			Quantity: float64(req.TotalQuantity),
			Type:     order.Limit,
			TIF:      req.ParentTIF,
		},
		limit:         req.LmtPrice,
		quantity:      req.TotalQuantity,
		side:          sideOf(req.Action),
		evaluableFrom: m.nextSessionToClose(now),
		takeProfit:    req.TPPrice,
		stopLoss:      req.SLPrice,
		group:         id,
	}

	return &broker.BracketOrderResponse{ParentOrderId: id, Status: "Submitted"}, nil
}

// UpdateOrder changes the limit price of a resting order.
func (m *Mock) UpdateOrder(_ context.Context, orderID int, req broker.UpdateOrderRequest) (*broker.OrderResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	resting, ok := m.orders[orderID]
	if !ok {
		return nil, fmt.Errorf("mock broker: no open order with id %d", orderID)
	}
	if req.LmtPrice != nil {
		resting.limit = *req.LmtPrice
		resting.order.Limit = req.LmtPrice
	}
	if req.TotalQuantity != nil {
		resting.quantity = *req.TotalQuantity
	}
	return &broker.OrderResponse{OrderId: orderID, Status: "Submitted"}, nil
}

// CancelOrder removes a resting order.
func (m *Mock) CancelOrder(_ context.Context, orderID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.orders[orderID]; !ok {
		return fmt.Errorf("mock broker: no open order with id %d", orderID)
	}
	delete(m.orders, orderID)
	m.finalStatus[orderID] = "Cancelled"
	return nil
}

// GetPositions returns the simulated positions.
func (m *Mock) GetPositions(_ context.Context) ([]broker.PositionResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	positions := m.portfolio.Positions()
	out := make([]broker.PositionResponse, 0, len(positions))
	for _, p := range positions {
		out = append(out, broker.PositionResponse{
			Symbol:      p.Symbol,
			Quantity:    p.Quantity,
			AverageCost: p.AvgPrice,
		})
	}
	return out, nil
}

// GetOpenOrders returns the resting orders.
func (m *Mock) GetOpenOrders(_ context.Context) ([]broker.OpenOrderResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]broker.OpenOrderResponse, 0, len(m.orders))
	for _, o := range m.orders {
		resp := broker.OpenOrderResponse{
			OrderId:       o.id,
			Symbol:        o.order.Symbol,
			Action:        o.side.String(),
			TotalQuantity: float64(o.quantity),
			Status:        "Submitted",
			Remaining:     float64(o.quantity),
		}
		limit := o.limit
		resp.LmtPrice = &limit
		out = append(out, resp)
	}
	return out, nil
}

// GetAccountNAV returns the simulated account value.
func (m *Mock) GetAccountNAV(_ context.Context) (*broker.AccountNAVResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return &broker.AccountNAVResponse{Account: "mock", NAV: m.portfolio.Value()}, nil
}

// AdvanceSession evaluates the resting orders against a session's bar.
//
// The engine calls it once per session, after that session has closed.
// An order is only evaluated against a session at or after the one it
// was submitted for, and only once per session.
func (m *Mock) AdvanceSession(session time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, o := range m.orders {
		if session.Before(o.evaluableFrom) {
			continue
		}
		if !o.lastEvaluated.IsZero() && !session.After(o.lastEvaluated) {
			continue
		}
		o.lastEvaluated = session

		// A day order expires at the close of the session it was
		// submitted for, whether or not the market has a bar for it.
		// The expiry must not wait on the market data.
		if o.order.TimeInForce() == "DAY" && !session.After(o.evaluableFrom) {
			delete(m.orders, id)
			m.finalStatus[id] = "Cancelled"
			continue
		}

		bar, ok := m.bar(o.order.Symbol, session)
		if !ok {
			continue
		}

		if o.fillAgainst(bar) {
			m.applyFill(o, bar)
			delete(m.orders, id)
			m.finalStatus[id] = "Filled"
			// A bracket's legs are one-cancels-the-other: when one
			// fills, the other is cancelled.
			m.cancelGroup(o.group, id)
			// An entry that filled brings its protective legs to life.
			m.armProtectiveLegs(o, session)
			continue
		}
	}
}

// applyFill books a resting order's fill into the portfolio.
//
// The fill happens at the limit price for a limit order, and at the
// stop or target for a protective leg. The position is then marked at
// the session's close, not at the fill price: an order that fills at
// 100 in a session that closed at 205 must not leave the position
// valued at 100.
func (m *Mock) applyFill(o *restingOrder, bar order.Bar) {
	price := o.fillPrice(bar)
	qty := float64(o.quantity)

	m.portfolio.ApplyFill(o.order.Symbol, o.side, qty, price)
	m.portfolio.MarkPrice(o.order.Symbol, bar.Close)
	m.chargeFee(qty)
	m.fillPrices[o.id] = price
}

// FillPrice returns the price an order filled at.
func (m *Mock) FillPrice(orderID int) (float64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	price, ok := m.fillPrices[orderID]
	return price, ok
}

// cancelGroup cancels the other orders in a bracket's group, leaving
// the one that filled.
func (m *Mock) cancelGroup(group, keep int) {
	if group == 0 {
		return
	}
	for id, o := range m.orders {
		if id == keep || o.group != group {
			continue
		}
		delete(m.orders, id)
	}
}

// armProtectiveLegs brings a bracket's take-profit and stop-loss to
// life once its entry has filled.
//
// The legs are separate resting orders, linked to each other: when one
// fills the other is cancelled, the way a real bracket's legs are.
func (m *Mock) armProtectiveLegs(parent *restingOrder, session time.Time) {
	// A leg of zero means there is no leg.
	if parent.takeProfit <= 0 && parent.stopLoss <= 0 {
		return
	}

	// The legs are first evaluated against the session that closes
	// after the entry filled.
	evaluableFrom := m.nextSessionToClose(m.session.Close(session))

	arm := func(limit float64, kind orderKind) {
		if limit <= 0 {
			return
		}
		id := m.nextID
		m.nextID++
		m.orders[id] = &restingOrder{
			id:    id,
			order: order.Order{
				Symbol:   parent.order.Symbol,
				Side:     opposite(parent.side),
				Quantity: float64(parent.quantity),
				Type:     order.Limit,
				TIF:      "GTC",
			},
			limit:         limit,
			quantity:      parent.quantity,
			side:          opposite(parent.side),
			evaluableFrom: evaluableFrom,
			group:         parent.group,
			kind:          kind,
		}
	}
	arm(parent.takeProfit, kindTakeProfit)
	arm(parent.stopLoss, kindStopLoss)
}

// statusOf reports the status of an order.
//
// An order that is no longer resting has either filled, expired or
// been cancelled, and the difference matters to a caller repeating an
// idempotent submission.
func (m *Mock) statusOf(id int) string {
	if _, ok := m.orders[id]; ok {
		return "Submitted"
	}
	if status, ok := m.finalStatus[id]; ok {
		return status
	}
	return "Filled"
}

// nextSessionToClose returns the first session whose close is after the
// given time.
//
// An order submitted at 09:00 is first evaluated against the session
// that closes that afternoon; an order submitted at 16:00, after the
// close, is first evaluated against the next session.
func (m *Mock) nextSessionToClose(now time.Time) time.Time {
	anchor := m.session.Anchor(now)
	// If the anchor is today's session and the close has not yet
	// passed, today's session is the one that closes next.
	if closeOf := m.session.Close(anchor); closeOf.After(now) {
		return anchor
	}
	// The session's own calendar decides what the next session is,
	// not the built-in one: a strategy may run on a different
	// exchange's days.
	return m.session.NextTradingDay(anchor)
}

// fillAgainst fills the order if the bar trades through its limit.
//
// It reports whether the order was filled.
func (o *restingOrder) fillAgainst(bar order.Bar) bool {
	if !o.limitTriggered(bar) {
		return false
	}
	o.filledAt = o.fillPriceForLimit(bar)
	return true
}

// fillPriceForLimit returns the price a limit order fills at.
//
// A limit order never fills at a worse price than its limit: a buy at
// 102 that finds the session opened at 90 fills at 90.
//
// A stop is the other way round: a stop at 95 that finds the session
// opened at 90 fills at 90, because the market gapped through it.
func (o *restingOrder) fillPriceForLimit(bar order.Bar) float64 {
	if o.kind == kindStopLoss {
		if o.side == order.Buy {
			if bar.Open > 0 && bar.Open > o.limit {
				return bar.Open
			}
			return o.limit
		}
		if bar.Open > 0 && bar.Open < o.limit {
			return bar.Open
		}
		return o.limit
	}
	if o.side == order.Buy {
		if bar.Open > 0 && bar.Open < o.limit {
			return bar.Open
		}
		return o.limit
	}
	if bar.Open > 0 && bar.Open > o.limit {
		return bar.Open
	}
	return o.limit
}

// fillPrice returns the price the order filled at.
func (o *restingOrder) fillPrice(bar order.Bar) float64 {
	if o.filledAt != 0 {
		return o.filledAt
	}
	return bar.Close
}

// opposite returns the other side.
func opposite(s order.Side) order.Side {
	if s == order.Buy {
		return order.Sell
	}
	return order.Buy
}

// limitTriggered reports whether the bar triggered the order.
func (o *restingOrder) limitTriggered(bar order.Bar) bool {
	if o.kind == kindStopLoss {
		// A stop is triggered by the market moving against the
		// trade: a long's stop fires when the price falls to it.
		if o.side == order.Buy {
			return bar.High >= o.limit
		}
		return bar.Low <= o.limit
	}
	if o.side == order.Buy {
		return bar.Low <= o.limit
	}
	return bar.High >= o.limit
}

// closePrice returns the close of a symbol on a session.
func (m *Mock) closePrice(symbol string, session time.Time) (float64, bool) {
	bar, ok := m.bar(symbol, session)
	if !ok {
		return 0, false
	}
	return bar.Close, true
}

// bar returns the bar for a symbol on a session.
func (m *Mock) bar(symbol string, session time.Time) (order.Bar, bool) {
	if m.bars == nil {
		return order.Bar{}, false
	}
	return m.bars.Bar(symbol, session)
}

// applySlippage moves the reference price against the trader.
func (m *Mock) applySlippage(price float64, side order.Side) float64 {
	if m.slippageBps == 0 || price == 0 {
		return price
	}
	adj := price * m.slippageBps / 10_000
	if side == order.Buy {
		return price + adj
	}
	return price - adj
}

// chargeFee books the commission on a fill.
func (m *Mock) chargeFee(quantity float64) {
	fee := m.commission + m.commissionPerShare*quantity
	if fee > 0 {
		m.portfolio.ChargeFee(fee)
	}
}

// sideOf maps a broker action to a framework side.
func sideOf(action string) order.Side {
	if action == "SELL" {
		return order.Sell
	}
	return order.Buy
}
