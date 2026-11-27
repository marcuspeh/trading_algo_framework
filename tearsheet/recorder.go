package tearsheet

import (
	"sync"
	"time"

	"github.com/marcuspeh/trading-framework/order"
)

// lot is an open position: the part of a fill that has not been closed
// yet.
type lot struct {
	side       order.Side
	quantity   float64
	price      float64
	at         time.Time
	commission float64
}

// Recorder collects the events of a run.
//
// It is safe for concurrent use: the engines report the portfolio's
// value from the run's own goroutine, and the brokers report their
// fills from a webhook handler.
type Recorder struct {
	mu     sync.Mutex
	points []Point
	fills  []order.Fill
	trades []Trade
	lots   map[string][]lot
	fees   float64
}

// NewRecorder returns an empty Recorder.
func NewRecorder() *Recorder {
	return &Recorder{lots: map[string][]lot{}}
}

// OnEquity records the portfolio's value.
func (r *Recorder) OnEquity(at time.Time, value float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.points = append(r.points, Point{Time: at, Value: value, Open: r.openLots()})
}

// OnFill records a fill and matches it against the positions it
// closes.
//
// The matching is first in, first out: the oldest open lot is closed
// first, which is the order a broker reports its fills in and the one
// a tax lot accounting expects.
func (r *Recorder) OnFill(f order.Fill) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.fills = append(r.fills, f)
	r.match(f)
}

// Events returns the recorded events of the run: the portfolio's value
// at each sample, and the fills that moved it.
//
// The closed trades are not part of it: they are derived from the
// fills, so a run that is loaded back is matched the same way the run
// that recorded it was.
func (r *Recorder) Events() Events {
	r.mu.Lock()
	defer r.mu.Unlock()

	points := make([]Point, len(r.points))
	copy(points, r.points)
	fills := make([]order.Fill, len(r.fills))
	copy(fills, r.fills)
	return Events{Equity: points, Fills: fills}
}

// Restore puts a run's events back into the recorder.
//
// The equity samples are taken as they are. The fills are replayed, so
// the closed trades and the open lots they leave behind are matched
// the same way they were when they were first recorded.
//
// It is what a live service does after it restarts: it loads its
// history back and carries on where it left off.
func (r *Recorder) Restore(evts Events) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.points = append(r.points, evts.Equity...)
	for _, f := range evts.Fills {
		r.match(f)
	}
}

// match books a fill against the open lots. It is called with the lock
// held.
func (r *Recorder) match(f order.Fill) {
	r.fees += f.Commission
	if f.Quantity <= 0 {
		return
	}

	remaining := f.Quantity
	remainingCommission := f.Commission
	open := r.lots[f.Symbol]

	for remaining > 0 && len(open) > 0 {
		l := open[0]
		if l.side == f.Side {
			// A fill on the same side as the open lot opens more of
			// it rather than closing it.
			break
		}

		closed := remaining
		if closed > l.quantity {
			closed = l.quantity
		}

		// The commission is split between the part of the fill that
		// closes the lot and the part that opens a new one.
		openCommission := l.commission * closed / l.quantity
		closeCommission := remainingCommission * closed / remaining

		pnl := (f.Price - l.price) * closed
		if l.side == order.Sell {
			// A short profits when the price falls.
			pnl = -pnl
		}
		net := pnl - openCommission - closeCommission

		capital := l.price * closed
		returnPct := 0.0
		if capital != 0 {
			returnPct = net / capital * 100
		}

		r.trades = append(r.trades, Trade{
			Symbol:     f.Symbol,
			Side:       sideLabel(l.side),
			Quantity:   closed,
			EntryTime:  l.at,
			ExitTime:   f.Time,
			EntryPrice: l.price,
			ExitPrice:  f.Price,
			PnL:        net,
			ReturnPct:  returnPct,
			Duration:   f.Time.Sub(l.at),
		})

		remaining -= closed
		remainingCommission -= closeCommission
		l.quantity -= closed
		l.commission -= openCommission

		if l.quantity <= 0 {
			open = open[1:]
			continue
		}
		open[0] = l
	}

	if remaining > 0 {
		open = append(open, lot{
			side:       f.Side,
			quantity:   remaining,
			price:      f.Price,
			at:         f.Time,
			commission: remainingCommission,
		})
	}

	if len(open) == 0 {
		delete(r.lots, f.Symbol)
		return
	}
	r.lots[f.Symbol] = open
}

// Report builds the tear sheet from what the recorder has collected.
func (r *Recorder) Report(meta Meta) Report {
	r.mu.Lock()
	defer r.mu.Unlock()

	points := make([]Point, len(r.points))
	copy(points, r.points)
	trades := make([]Trade, len(r.trades))
	copy(trades, r.trades)

	report := Report{
		Meta:   meta,
		Equity: points,
		Trades: trades,
	}
	report.Daily = dailyCloses(points)
	report.Drawdown = drawdown(report.Daily)
	report.Metrics = computeMetrics(points, report.Daily, trades, r.fees, r.openLots())
	report.Monthly = monthlyReturns(points)
	return report
}

// openLots is how many open lots the recorder holds. It is called with
// the lock held.
func (r *Recorder) openLots() int {
	n := 0
	for _, lots := range r.lots {
		n += len(lots)
	}
	return n
}

// sideLabel names the direction of a trade.
func sideLabel(s order.Side) string {
	if s == order.Sell {
		return "SHORT"
	}
	return "LONG"
}
