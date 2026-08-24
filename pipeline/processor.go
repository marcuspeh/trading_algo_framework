// Package pipeline wires the framework order pipeline:
// job -> []Order -> log -> risk -> executor.
package pipeline

import (
	"context"
	"time"

	tf "github.com/marcuspeh/trading-framework"
	"github.com/marcuspeh/trading-framework/risk"
)

type Executor interface {
	Submit(ctx context.Context, order tf.Order) (tf.OrderResult, error)
}

type Processor struct {
	risk   tf.RiskManager
	exec   Executor
	logger tf.Logger
}

func New(r tf.RiskManager, e Executor, log tf.Logger) *Processor {
	if r == nil {
		r = risk.NewBasic()
	}
	if log == nil {
		log = nopLogger{}
	}
	return &Processor{risk: r, exec: e, logger: log}
}

// ProcessJob logs the orders produced by a Job, runs them through the
// RiskManager, and forwards the survivors to the Executor. Submit
// failures are logged but do not abort the run.
func (p *Processor) ProcessJob(ctx tf.Context, job tf.Job, date time.Time, orders []tf.Order) error {
	if len(orders) == 0 {
		return nil
	}

	p.logger.Info("orders generated",
		"job", job.Name,
		"count", len(orders),
		"date", date.Format("2006-01-02"),
	)

	approved, err := p.risk.Validate(ctx, orders)
	if err != nil {
		if reasons, ok := err.(risk.Reasons); ok {
			for _, r := range reasons {
				p.logger.Warn("order rejected",
					"job", job.Name,
					"symbol", r.Order.Symbol,
					"side", r.Order.Side.String(),
					"reason", r.Reason,
				)
			}
		} else {
			p.logger.Error("risk validation error",
				"job", job.Name,
				"err", err.Error(),
			)
		}
	}

	for _, o := range approved {
		p.logger.Info("order approved",
			"job", job.Name,
			"symbol", o.Symbol,
			"side", o.Side.String(),
			"qty", o.Quantity,
			"type", o.Type.String(),
		)
	}

	if p.exec == nil {
		return nil
	}

	for _, o := range approved {
		res, err := p.exec.Submit(context.Background(), o)
		if err != nil {
			p.logger.Error("order submit failed",
				"job", job.Name,
				"symbol", o.Symbol,
				"side", o.Side.String(),
				"err", err.Error(),
			)
			continue
		}
		if res.Error != nil {
			p.logger.Error("order submit returned error",
				"job", job.Name,
				"symbol", o.Symbol,
				"err", res.Error.Error(),
			)
			continue
		}
		p.logger.Info("order submitted",
			"job", job.Name,
			"symbol", o.Symbol,
			"side", o.Side.String(),
			"id", res.ID,
			"filled", res.Filled,
			"fill_price", res.FillPrice,
		)
	}

	return nil
}

type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Warn(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}
