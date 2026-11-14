package execution

import (
	"context"
	"fmt"
	"time"

	"github.com/marcuspeh/go-tools/goroutine"
	"github.com/marcuspeh/trading-framework/broker"
	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/order"
	"github.com/marcuspeh/trading-framework/portfolio"
)

// PriceLookup supplies the price a position is marked at.
//
// The market client satisfies it. The reconciler uses it to derive the
// account's cash, which the broker does not report.
type PriceLookup interface {
	Bars(ctx context.Context, symbol string, lookback int, asOf time.Time) ([]order.Bar, error)
}

// Reconciler overwrites a portfolio.Live with broker state. The broker
// is the source of truth.
type Reconciler struct {
	broker Client
	logger Logger
	mirror *portfolio.Live
	prices PriceLookup
	session *clock.Session
}

type Logger interface {
	Info(msg string, fields ...any)
	Warn(msg string, fields ...any)
	Error(msg string, fields ...any)
}

// NewReconciler returns a Reconciler.
//
// The price lookup and session are optional. Without them the cash
// balance cannot be derived, and the mirror reports that its cash is
// unknown rather than reporting zero.
func NewReconciler(b Client, m *portfolio.Live, log Logger) *Reconciler {
	if log == nil {
		log = nopLogger{}
	}
	return &Reconciler{broker: b, mirror: m, logger: log}
}

// WithPrices supplies the price lookup and session the reconciler
// derives cash from.
func (r *Reconciler) WithPrices(p PriceLookup, session *clock.Session) *Reconciler {
	r.prices = p
	r.session = session
	return r
}

// Result is the broker-truth snapshot written into the mirror.
type Result struct {
	Positions  int
	OpenOrders int
	HasNAV     bool
	NAV        float64
	HasCash    bool
	Cash       float64
}

// Reconcile fetches broker positions, open orders, and NAV concurrently
// and writes them into the mirror. NAV failure is non-fatal.
//
// The cash balance is derived from the NAV and the position marks:
// cash = NAV - the market value of the positions. A position with no
// mark leaves the cash unknown, because a cash figure computed from a
// missing price would be wrong rather than merely imprecise.
func (r *Reconciler) Reconcile(ctx context.Context) (Result, error) {
	if r.broker == nil {
		return Result{}, fmt.Errorf("live reconciler: no broker client")
	}
	if r.mirror == nil {
		return Result{}, fmt.Errorf("live reconciler: no mirror portfolio")
	}

	var (
		bps    []broker.PositionResponse
		bos    []broker.OpenOrderResponse
		nav    *broker.AccountNAVResponse
		navErr error
	)

	g := goroutine.NewErrGroup()
	g.Run(ctx, func() error {
		var err error
		bps, err = r.broker.GetPositions(ctx)
		return err
	})
	g.Run(ctx, func() error {
		var err error
		bos, err = r.broker.GetOpenOrders(ctx)
		return err
	})
	g.Run(ctx, func() error {
		nav, navErr = r.broker.GetAccountNAV(ctx)
		return nil
	})
	if err := g.Wait(); err != nil {
		return Result{}, fmt.Errorf("live reconciler: %w", err)
	}

	hasNav := false
	navVal := 0.0
	if navErr != nil {
		r.logger.Warn("reconcile: account NAV unavailable", "err", navErr.Error())
	} else if nav != nil {
		hasNav = true
		navVal = nav.NAV
	}

	positions := make([]order.Position, 0, len(bps))
	for _, bp := range bps {
		positions = append(positions, toFrameworkPosition(bp))
	}

	cash, hasCash := r.deriveCash(ctx, navVal, hasNav, positions)

	r.mirror.Sync(portfolio.LiveSnapshot{
		Positions: positions,
		NAV:       navVal,
		HasNAV:    hasNav,
		Cash:      cash,
		HasCash:   hasCash,
	})

	r.logger.Info("reconcile complete",
		"positions", len(positions),
		"open_orders", len(bos),
		"has_nav", hasNav,
		"nav", navVal,
		"has_cash", hasCash,
	)

	return Result{
		Positions:  len(positions),
		OpenOrders: len(bos),
		HasNAV:     hasNav,
		NAV:        navVal,
		HasCash:    hasCash,
		Cash:       cash,
	}, nil
}

// deriveCash computes the account's cash from the NAV and the market
// value of the positions.
//
// The marks come from the same session the strategy sees, so the
// derived cash and the strategy's view of the account agree.
func (r *Reconciler) deriveCash(ctx context.Context, nav float64, hasNav bool, positions []order.Position) (float64, bool) {
	if !hasNav || r.prices == nil || r.session == nil {
		return 0, false
	}

	asOf := r.session.Anchor(r.session.Clock().Now())
	marketValue := 0.0
	for _, pos := range positions {
		if pos.Quantity == 0 {
			continue
		}
		bars, err := r.prices.Bars(ctx, pos.Symbol, 1, asOf)
		if err != nil || len(bars) == 0 {
			// A position with no mark leaves the cash unknown: a
			// cash figure built on a missing price would be wrong,
			// not merely imprecise.
			r.logger.Warn("reconcile: no mark for a position",
				"symbol", pos.Symbol,
				"as_of", asOf.Format("2006-01-02"),
			)
			return 0, false
		}
		marketValue += pos.Quantity * bars[len(bars)-1].Close
	}
	return nav - marketValue, true
}

type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Warn(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}
