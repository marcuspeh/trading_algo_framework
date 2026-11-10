// Package pipeline is the small glue between the scheduler and the
// Executor. It runs orders through RiskManager validation, then submits
// approved orders via the Executor.
package pipeline

import (
	"context"
	"time"

	"github.com/marcuspeh/trading-framework/execution"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
	"github.com/marcuspeh/trading-framework/risk"
	"github.com/marcuspeh/trading-framework/scheduler"
)

// Handler returns a scheduler.Handler that runs orders through risk and
// the executor.
func Handler(r risk.Manager, exec execution.Executor, log logger) scheduler.Handler {
	if r == nil {
		r = risk.NewBasic()
	}
	return func(ctx context.Context, jc scheduler.JobContext, j job.Job, date time.Time, orders []order.Order) error {
		if len(orders) == 0 {
			return nil
		}
		log.Info("orders generated",
			"job", j.Name,
			"count", len(orders),
			"date", date.Format("2006-01-02"),
		)

		approved, err := r.Validate(jc, orders)
		if reasons := risk.Rejected(err); len(reasons) > 0 {
			for _, rr := range reasons {
				log.Warn("order rejected",
					"job", j.Name,
					"symbol", rr.Order.Symbol,
					"side", rr.Order.Side.String(),
					"reason", rr.Reason,
				)
			}
		} else if err != nil {
			log.Error("risk validation error",
				"job", j.Name,
				"err", err.Error(),
			)
		}

		for _, o := range approved {
			log.Info("order approved",
				"job", j.Name,
				"symbol", o.Symbol,
				"side", o.Side.String(),
				"qty", o.Quantity,
				"type", o.Type.String(),
			)
		}

		for _, o := range approved {
			res, err := exec.Submit(ctx, o)
			if err != nil {
				log.Error("order submit failed",
					"job", j.Name,
					"symbol", o.Symbol,
					"side", o.Side.String(),
					"err", err.Error(),
				)
				continue
			}
			log.Info("order submitted",
				"job", j.Name,
				"symbol", o.Symbol,
				"side", o.Side.String(),
				"id", res.ID,
				"filled", res.Filled,
				"fill_price", res.FillPrice,
			)
		}
		return nil
	}
}

type logger interface {
	Info(msg string, fields ...any)
	Warn(msg string, fields ...any)
	Error(msg string, fields ...any)
}
