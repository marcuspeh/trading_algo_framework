package tradingframework_test

import (
	"errors"
	"testing"
	"time"

	tf "github.com/marcuspeh/trading-framework"
)

func TestTimeOfDayValidate(t *testing.T) {
	cases := []struct {
		name string
		t    tf.TimeOfDay
		ok   bool
	}{
		{"zero", tf.TimeOfDay{}, true},
		{"midday", tf.Time(12, 30), true},
		{"end-of-day", tf.Time(23, 59), true},
		{"hour too high", tf.Time(24, 0), false},
		{"hour negative", tf.Time(-1, 0), false},
		{"minute too high", tf.Time(0, 60), false},
		{"minute negative", tf.Time(0, -1), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.t.Validate()
			if (err == nil) != c.ok {
				t.Fatalf("Validate(%v) ok=%v err=%v", c.t, c.ok, err)
			}
		})
	}
}

func TestTimeOfDayEqual(t *testing.T) {
	if !tf.Time(9, 0).Equal(tf.Time(9, 0)) {
		t.Fatal("expected equality")
	}
	if tf.Time(9, 0).Equal(tf.Time(9, 1)) {
		t.Fatal("expected inequality")
	}
}

func TestTimeOfDayString(t *testing.T) {
	if got := tf.Time(9, 5).String(); got != "09:05" {
		t.Fatalf("got %q", got)
	}
}

func TestJobValidate(t *testing.T) {
	dummy := func(tf.Context, time.Time) ([]tf.Order, error) { return nil, nil }

	cases := []struct {
		name string
		job  tf.Job
		ok   bool
	}{
		{"good", tf.Job{Name: "x", Time: tf.Time(9, 0), Handler: dummy}, true},
		{"missing name", tf.Job{Time: tf.Time(9, 0), Handler: dummy}, false},
		{"missing handler", tf.Job{Name: "x", Time: tf.Time(9, 0)}, false},
		{"bad time", tf.Job{Name: "x", Time: tf.Time(25, 0), Handler: dummy}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.job.Validate()
			if (err == nil) != c.ok {
				t.Fatalf("Validate ok=%v err=%v", c.ok, err)
			}
			if err != nil {
				var ij tf.ErrInvalidJob
				if !errors.As(err, &ij) {
					t.Fatalf("expected ErrInvalidJob, got %T", err)
				}
			}
		})
	}
}

func TestSideString(t *testing.T) {
	if tf.Buy.String() != "BUY" {
		t.Fatal("Buy should be BUY")
	}
	if tf.Sell.String() != "SELL" {
		t.Fatal("Sell should be SELL")
	}
}

func TestOrderTypeString(t *testing.T) {
	if tf.Market.String() != "MARKET" {
		t.Fatal("Market should be MARKET")
	}
	if tf.Limit.String() != "LIMIT" {
		t.Fatal("Limit should be LIMIT")
	}
}

func TestModeString(t *testing.T) {
	if tf.Backtest.String() != "backtest" {
		t.Fatal("Backtest should be backtest")
	}
	if tf.Live.String() != "live" {
		t.Fatal("Live should be live")
	}
}

func TestErrNotFoundSentinel(t *testing.T) {
	if tf.ErrNotFound == nil {
		t.Fatal("ErrNotFound should be non-nil")
	}
	if tf.ErrNotFound.Error() == "" {
		t.Fatal("ErrNotFound must have a message")
	}
}
