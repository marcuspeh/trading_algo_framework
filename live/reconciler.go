package live

import (
	"context"
	"fmt"

	"github.com/marcuspeh/go-tools/goroutine"
	tf "github.com/marcuspeh/trading-framework"
	"github.com/marcuspeh/trading-framework/portfolio"
)

// Reconciler overwrites the local MirrorPortfolio with broker state.
// IBKR is the source of truth; the framework's local view is replaced
// on every reconciliation.
type Reconciler struct {
	broker BrokerClient
	logger tf.Logger
	mirror *portfolio.MirrorPortfolio
}

// NewReconciler constructs a Reconciler.
func NewReconciler(b BrokerClient, m *portfolio.MirrorPortfolio, log tf.Logger) *Reconciler {
	if log == nil {
		log = nopLogger{}
	}
	return &Reconciler{broker: b, mirror: m, logger: log}
}

// Mirror returns the portfolio the Reconciler writes to.
func (r *Reconciler) Mirror() *portfolio.MirrorPortfolio { return r.mirror }

// Reconcile fetches broker positions, open orders, and NAV and overwrites
// the MirrorPortfolio. The returned counts are useful for tests and
// startup logging.
//
// The three broker fetches run concurrently via goroutine.ErrGroup so
// total latency is max(t1, t2, t3) rather than t1 + t2 + t3. NAV
// failure is non-fatal; positions/open orders failures abort.
type Result struct {
	Positions  int
	OpenOrders int
	HasNAV     bool
	NAV        float64
}

// Reconcile fetches broker truth and pushes it into the mirror.
func (r *Reconciler) Reconcile(ctx context.Context) (Result, error) {
	if r.broker == nil {
		return Result{}, fmt.Errorf("live reconciler: no broker client")
	}
	if r.mirror == nil {
		return Result{}, fmt.Errorf("live reconciler: no mirror portfolio")
	}

	var (
		bps []PositionResponse
		bos []OpenOrderResponse
		nav *AccountNAVResponse
	)
	navErrCh := make(chan error, 1)

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
		var err error
		nav, err = r.broker.GetAccountNAV(ctx)
		// NAV is best-effort; capture but don't fail the group.
		navErrCh <- err
		return nil
	})
	if err := g.Wait(); err != nil {
		return Result{}, fmt.Errorf("live reconciler: %w", err)
	}

	hasNav := false
	navVal := 0.0
	select {
	case navErr := <-navErrCh:
		if navErr == nil && nav != nil {
			hasNav = true
			navVal = nav.NAV
		} else if navErr != nil {
			r.logger.Warn("reconcile: account NAV unavailable", "err", navErr.Error())
		}
	default:
	}

	positions := make([]tf.Position, 0, len(bps))
	for _, bp := range bps {
		positions = append(positions, toFrameworkPosition(bp))
	}

	r.mirror.Sync(positions, navVal, hasNav)

	r.logger.Info("reconcile complete",
		"positions", len(positions),
		"open_orders", len(bos),
		"has_nav", hasNav,
		"nav", navVal,
	)

	return Result{
		Positions:  len(positions),
		OpenOrders: len(bos),
		HasNAV:     hasNav,
		NAV:        navVal,
	}, nil
}

type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Warn(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}
