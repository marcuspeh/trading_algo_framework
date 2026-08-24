package live

import (
	"context"
	"fmt"

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
	return &Reconciler{broker: b, logger: log}
}

// Mirror returns the portfolio the Reconciler writes to.
func (r *Reconciler) Mirror() *portfolio.MirrorPortfolio { return r.mirror }

// Reconcile fetches broker positions, open orders, and NAV and overwrites
// the MirrorPortfolio. The returned counts are useful for tests and
// startup logging.
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

	bps, err := r.broker.GetPositions(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("live reconciler: positions: %w", err)
	}
	positions := make([]tf.Position, 0, len(bps))
	for _, bp := range bps {
		positions = append(positions, toFrameworkPosition(bp))
	}

	bos, err := r.broker.GetOpenOrders(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("live reconciler: open orders: %w", err)
	}

	navResp, navErr := r.broker.GetAccountNAV(ctx)
	hasNav := navErr == nil && navResp != nil
	nav := 0.0
	if hasNav {
		nav = navResp.NAV
	} else if navErr != nil {
		r.logger.Warn("reconcile: account NAV unavailable", "err", navErr.Error())
	}

	r.mirror.Sync(positions, nav, hasNav)

	r.logger.Info("reconcile complete",
		"positions", len(positions),
		"open_orders", len(bos),
		"has_nav", hasNav,
		"nav", nav,
	)

	return Result{
		Positions:  len(positions),
		OpenOrders: len(bos),
		HasNAV:     hasNav,
		NAV:        nav,
	}, nil
}

type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Warn(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}
