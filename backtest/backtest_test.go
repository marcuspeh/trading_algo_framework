package backtest_test

import (
	"context"
	"testing"
	"time"

	tf "github.com/marcuspeh/trading-framework"
	"github.com/marcuspeh/trading-framework/backtest"
)

type fakeMarket struct {
	bars map[string][]tf.Bar
}

func (f *fakeMarket) Bars(_ context.Context, symbol string, lookback int, _ time.Time) ([]tf.Bar, error) {
	list := f.bars[symbol]
	if len(list) == 0 {
		return nil, tf.ErrNotFound
	}
	if lookback > len(list) {
		lookback = len(list)
	}
	out := make([]tf.Bar, lookback)
	copy(out, list[len(list)-lookback:])
	return out, nil
}

type fakeConstituents struct{}

func (fakeConstituents) Constituents(_ context.Context, _ string, _ time.Time) ([]string, error) {
	return nil, nil
}

type fakeBarSource struct {
	bars map[string]map[string]tf.Bar // symbol -> yyyy-mm-dd -> Bar
}

func (f *fakeBarSource) Bar(symbol string, date time.Time) (tf.Bar, bool) {
	key := date.Format("2006-01-02")
	if m, ok := f.bars[symbol]; ok {
		if b, ok := m[key]; ok {
			return b, true
		}
	}
	return tf.Bar{}, false
}

type fakeConfig struct{}

func (fakeConfig) Get(string) (string, error) { return "", nil }
func (fakeConfig) Watch(string, any, any) (tf.Watcher, error) {
	return nil, nil
}

type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Warn(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}

func TestClock_BasicOps(t *testing.T) {
	start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	c := backtest.NewClock(start)
	if !c.Date().Equal(start) {
		t.Fatalf("date: %v", c.Date())
	}
	if !c.Now().Equal(start) {
		t.Fatalf("now: %v", c.Now())
	}
	c.SetDate(start.AddDate(0, 0, 1))
	c.SetNow(start.AddDate(0, 0, 1).Add(2 * time.Hour))
	if !c.Date().Equal(start.AddDate(0, 0, 1)) {
		t.Fatalf("date after set: %v", c.Date())
	}
	if !c.Now().Equal(start.AddDate(0, 0, 1).Add(2 * time.Hour)) {
		t.Fatalf("now after set: %v", c.Now())
	}
}

func TestDefaultFill_MarketAtClose(t *testing.T) {
	day := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)
	bs := &fakeBarSource{bars: map[string]map[string]tf.Bar{
		"AAPL": {"2024-01-05": {Time: day, Open: 100, High: 110, Low: 90, Close: 105}},
	}}
	f := backtest.NewDefaultFill(bs)
	price, filled, ok := f.Fill(tf.Order{Symbol: "AAPL", Side: tf.Buy, Quantity: 1, Type: tf.Market}, day)
	if !ok || !filled || price != 105 {
		t.Fatalf("market fill: ok=%v filled=%v price=%v", ok, filled, price)
	}
}

func TestDefaultFill_LimitBuyTriggersAtLow(t *testing.T) {
	day := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)
	bs := &fakeBarSource{bars: map[string]map[string]tf.Bar{
		"AAPL": {"2024-01-05": {Time: day, Open: 100, High: 110, Low: 90, Close: 105}},
	}}
	f := backtest.NewDefaultFill(bs)
	limit := 95.0
	price, filled, ok := f.Fill(tf.Order{Symbol: "AAPL", Side: tf.Buy, Quantity: 1, Type: tf.Limit, Limit: &limit}, day)
	if !ok || !filled || price != 95 {
		t.Fatalf("limit buy fill: ok=%v filled=%v price=%v", ok, filled, price)
	}
}

func TestDefaultFill_LimitBuyDoesNotTrigger(t *testing.T) {
	day := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)
	bs := &fakeBarSource{bars: map[string]map[string]tf.Bar{
		"AAPL": {"2024-01-05": {Time: day, Open: 100, High: 110, Low: 95, Close: 105}},
	}}
	f := backtest.NewDefaultFill(bs)
	limit := 90.0
	_, filled, ok := f.Fill(tf.Order{Symbol: "AAPL", Side: tf.Buy, Quantity: 1, Type: tf.Limit, Limit: &limit}, day)
	if !ok || filled {
		t.Fatal("limit buy should not fill")
	}
}

func TestDefaultFill_NoBar(t *testing.T) {
	day := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)
	f := backtest.NewDefaultFill(&fakeBarSource{})
	_, _, ok := f.Fill(tf.Order{Symbol: "AAPL", Side: tf.Buy, Quantity: 1, Type: tf.Market}, day)
	if ok {
		t.Fatal("expected ok=false with no bar")
	}
}

func TestBacktestEngine_RunsJobAndFillsOrder(t *testing.T) {
	start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC) // Tue
	end := start.AddDate(0, 0, 2)                        // Thu

	market := &fakeMarket{bars: map[string][]tf.Bar{
		"AAPL": {
			{Time: start, Close: 100},
			{Time: start.AddDate(0, 0, 1), Close: 105},
		},
	}}
	bs := &fakeBarSource{bars: map[string]map[string]tf.Bar{
		"AAPL": {
			"2024-01-02": {Time: start, Close: 100},
			"2024-01-03": {Time: start.AddDate(0, 0, 1), Close: 105},
		},
	}}

	eng, err := backtest.New(backtest.Config{
		Start:        start,
		End:          end,
		StartingCash: 10_000,
		Market:       market,
		Constituents: fakeConstituents{},
		Config:       fakeConfig{},
		Logger:       nopLogger{},
		BarSource:    bs,
	})
	if err != nil {
		t.Fatal(err)
	}

	called := 0
	job := tf.Job{
		Name: "demo",
		Time: tf.Time(16, 0),
		Handler: func(_ tf.Context, _ time.Time) ([]tf.Order, error) {
			called++
			return []tf.Order{{Symbol: "AAPL", Side: tf.Buy, Quantity: 10, Type: tf.Market}}, nil
		},
	}
	if err := eng.RegisterJob(job); err != nil {
		t.Fatal(err)
	}
	if err := eng.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Range includes Jan 2 (Tue), Jan 3 (Wed), Jan 4 (Thu) = 3 days.
	if called != 3 {
		t.Fatalf("expected 3 invocations, got %d", called)
	}
	positions := eng.Portfolio().Positions()
	// Two of the three days have bars, so two fills happen. The third
	// day has no bar so the market order sits unfilled.
	if len(positions) != 1 || positions[0].Symbol != "AAPL" || positions[0].Quantity != 20 {
		t.Fatalf("positions: %+v", positions)
	}
}

func TestBacktestEngine_NoJobsRejected(t *testing.T) {
	eng, err := backtest.New(backtest.Config{
		Start:        time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		End:          time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
		StartingCash: 1000,
		Market:       &fakeMarket{},
		Constituents: fakeConstituents{},
		Config:       fakeConfig{},
		Logger:       nopLogger{},
		BarSource:    &fakeBarSource{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Run(context.Background()); err == nil {
		t.Fatal("expected error when no jobs registered")
	}
}

func TestBacktestEngine_RequiresValidRange(t *testing.T) {
	_, err := backtest.New(backtest.Config{
		Start:        time.Time{},
		End:          time.Time{},
		StartingCash: 1000,
		Market:       &fakeMarket{},
		Constituents: fakeConstituents{},
		Config:       fakeConfig{},
		Logger:       nopLogger{},
		BarSource:    &fakeBarSource{},
	})
	if err == nil {
		t.Fatal("expected error for missing range")
	}
}

func TestExecutor_RejectsBadOrder(t *testing.T) {
	eng, err := backtest.New(backtest.Config{
		Start:        time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		End:          time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
		StartingCash: 1000,
		Market:       &fakeMarket{},
		Constituents: fakeConstituents{},
		Config:       fakeConfig{},
		Logger:       nopLogger{},
		BarSource:    &fakeBarSource{},
	})
	if err != nil {
		t.Fatal(err)
	}
	exec := eng.Executor()
	if _, err := exec.Submit(context.Background(), tf.Order{Symbol: "", Side: tf.Buy, Quantity: 1, Type: tf.Market}); err == nil {
		t.Fatal("expected symbol error")
	}
	if _, err := exec.Submit(context.Background(), tf.Order{Symbol: "AAPL", Side: tf.Buy, Quantity: 0, Type: tf.Market}); err == nil {
		t.Fatal("expected qty error")
	}
}
