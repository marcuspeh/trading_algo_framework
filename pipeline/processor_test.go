package pipeline_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	tf "github.com/marcuspeh/trading-framework"
	"github.com/marcuspeh/trading-framework/pipeline"
	"github.com/marcuspeh/trading-framework/risk"
)

type captureExecutor struct {
	mu     sync.Mutex
	orders []tf.Order
	failOn string
}

func (c *captureExecutor) Submit(_ context.Context, o tf.Order) (tf.OrderResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failOn == o.Symbol {
		return tf.OrderResult{}, errors.New("submit fail")
	}
	c.orders = append(c.orders, o)
	return tf.OrderResult{ID: "x", Filled: true, FillPrice: 100}, nil
}
func (c *captureExecutor) Positions(context.Context) ([]tf.Position, error) { return nil, nil }
func (c *captureExecutor) OpenOrders(context.Context) ([]tf.Order, error)   { return nil, nil }
func (c *captureExecutor) Cancel(context.Context, string) error             { return nil }

type recordingLogger struct {
	mu      sync.Mutex
	warnMsg []string
}

func (r *recordingLogger) Info(string, ...any) {}
func (r *recordingLogger) Warn(msg string, _ ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.warnMsg = append(r.warnMsg, msg)
}
func (r *recordingLogger) Error(string, ...any) {}

func TestProcessor_RunsThroughPipeline(t *testing.T) {
	exec := &captureExecutor{}
	log := &recordingLogger{}
	p := pipeline.New(risk.NewBasic(), exec, log)

	date := time.Now()
	job := tf.Job{Name: "demo", Time: tf.Time(9, 0)}
	orders := []tf.Order{
		{Symbol: "AAPL", Side: tf.Buy, Quantity: 10, Type: tf.Market},
		{Symbol: "", Side: tf.Buy, Quantity: 10, Type: tf.Market}, // rejected
	}
	if err := p.ProcessJob(tf.Context(nil), job, date, orders); err != nil {
		t.Fatal(err)
	}
	if len(exec.orders) != 1 {
		t.Fatalf("expected 1 submitted order, got %d", len(exec.orders))
	}
	if len(log.warnMsg) != 1 {
		t.Fatalf("expected 1 warn, got %d", len(log.warnMsg))
	}
}

func TestProcessor_ContinuesOnSubmitFailure(t *testing.T) {
	exec := &captureExecutor{failOn: "AAPL"}
	p := pipeline.New(risk.NewBasic(), exec, &recordingLogger{})

	orders := []tf.Order{
		{Symbol: "AAPL", Side: tf.Buy, Quantity: 1, Type: tf.Market},
		{Symbol: "MSFT", Side: tf.Buy, Quantity: 1, Type: tf.Market},
	}
	if err := p.ProcessJob(tf.Context(nil), tf.Job{Name: "x", Time: tf.Time(9, 0)}, time.Now(), orders); err != nil {
		t.Fatal(err)
	}
	if len(exec.orders) != 1 || exec.orders[0].Symbol != "MSFT" {
		t.Fatalf("expected MSFT to succeed, got %+v", exec.orders)
	}
}

func TestProcessor_NoOrdersIsNoop(t *testing.T) {
	exec := &captureExecutor{}
	p := pipeline.New(risk.NewBasic(), exec, &recordingLogger{})
	if err := p.ProcessJob(tf.Context(nil), tf.Job{Name: "x", Time: tf.Time(9, 0)}, time.Now(), nil); err != nil {
		t.Fatal(err)
	}
	if len(exec.orders) != 0 {
		t.Fatalf("expected no submissions")
	}
}
