package tearsheet

import (
	"math"
	"time"
)

// tradingDays is how many trading days a year has. It is the divisor
// that turns a daily return into an annualised one.
const tradingDays = 252

// computeMetrics calculates the numbers a tear sheet leads with.
func computeMetrics(points, daily []Point, trades []Trade, fees float64, open int) Metrics {
	m := Metrics{
		TotalFees:     fees,
		OpenPositions: open,
		TotalTrades:   len(trades),
	}
	m.WinRate, m.ProfitFactor, m.AvgWin, m.AvgLoss, m.BestTrade, m.WorstTrade = tradeStats(trades)

	if len(points) == 0 {
		return m
	}

	m.StartingValue = points[0].Value
	m.EndingValue = points[len(points)-1].Value
	m.TotalReturn = ratio(m.EndingValue, m.StartingValue) - 1

	// The annualised return is compounded over the days the run
	// covered, so a run of a month is not annualised like a run of a
	// year.
	if len(daily) >= 2 {
		days := daily[len(daily)-1].Time.Sub(daily[0].Time).Hours() / 24
		if days > 0 {
			m.CAGR = math.Pow(m.EndingValue/m.StartingValue, 365.25/days) - 1
		}
	}

	returns := dailyReturns(daily)
	if len(returns) > 0 {
		mean := average(returns)
		m.Volatility = stdev(returns, mean) * math.Sqrt(tradingDays)
		if m.Volatility > 0 {
			m.Sharpe = (mean * tradingDays) / m.Volatility
		}
		if down := downsideDeviation(returns, mean); down > 0 {
			m.Sortino = (mean * tradingDays) / (down * math.Sqrt(tradingDays))
		}
	}

	m.MaxDrawdown, m.MaxDrawdownFrom, m.MaxDrawdownTo = maxDrawdown(daily)

	// Exposure is the share of the days that had a position open.
	if len(daily) > 0 {
		open := 0
		for _, p := range daily {
			if p.Open > 0 {
				open++
			}
		}
		m.Exposure = float64(open) / float64(len(daily))
	}

	return m
}

// tradeStats summarises the closed trades.
func tradeStats(trades []Trade) (winRate, profitFactor, avgWin, avgLoss, best, worst float64) {
	if len(trades) == 0 {
		return 0, 0, 0, 0, 0, 0
	}

	var wins, losses int
	var grossWin, grossLoss float64
	for _, t := range trades {
		switch {
		case t.PnL > 0:
			wins++
			grossWin += t.PnL
			if t.PnL > best {
				best = t.PnL
			}
		case t.PnL < 0:
			losses++
			grossLoss -= t.PnL
			if t.PnL < worst {
				worst = t.PnL
			}
		}
	}

	winRate = float64(wins) / float64(len(trades))
	if grossLoss > 0 {
		profitFactor = grossWin / grossLoss
	}
	if wins > 0 {
		avgWin = grossWin / float64(wins)
	}
	if losses > 0 {
		avgLoss = -grossLoss / float64(losses)
	}
	return winRate, profitFactor, avgWin, avgLoss, best, worst
}

// dailyCloses returns the last sample of each day.
func dailyCloses(points []Point) []Point {
	if len(points) == 0 {
		return nil
	}
	out := make([]Point, 0, len(points))
	for _, p := range points {
		if n := len(out); n > 0 && sameDay(out[n-1].Time, p.Time) {
			out[n-1] = p
			continue
		}
		out = append(out, p)
	}
	return out
}

// drawdown returns the fall from the running peak of the series.
//
// The result is never positive: a day at a new high draws down zero.
func drawdown(points []Point) []Point {
	out := make([]Point, 0, len(points))
	peak := 0.0
	for i, p := range points {
		if i == 0 || p.Value > peak {
			peak = p.Value
		}
		dd := 0.0
		if peak > 0 {
			dd = p.Value/peak - 1
		}
		out = append(out, Point{Time: p.Time, Value: dd, Open: p.Open})
	}
	return out
}

// maxDrawdown returns the deepest peak-to-trough fall of the series.
func maxDrawdown(points []Point) (depth float64, from, to time.Time) {
	if len(points) == 0 {
		return 0, time.Time{}, time.Time{}
	}

	peak := points[0].Value
	peakAt := points[0].Time
	worst := 0.0

	for _, p := range points {
		if p.Value > peak {
			peak = p.Value
			peakAt = p.Time
		}
		if peak <= 0 {
			continue
		}
		dd := 1 - p.Value/peak
		if dd > worst {
			worst = dd
			from = peakAt
			to = p.Time
		}
	}
	return worst, from, to
}

// monthlyReturns returns each month's compounded return.
//
// The first month is measured from the start of the run; every later
// month from the end of the one before it. The series is the whole
// one, not the daily closes, so that the first month has the run's
// starting value to measure from.
func monthlyReturns(points []Point) []Monthly {
	if len(points) == 0 {
		return nil
	}

	out := make([]Monthly, 0, len(points))
	base := points[0].Value
	cur := points[0]

	for _, p := range points[1:] {
		if p.Time.Format("2006-01") != cur.Time.Format("2006-01") {
			out = append(out, Monthly{Month: cur.Time.Format("2006-01"), Return: ratio(cur.Value, base) - 1})
			base = cur.Value
			cur = p
			continue
		}
		cur = p
	}
	out = append(out, Monthly{Month: cur.Time.Format("2006-01"), Return: ratio(cur.Value, base) - 1})
	return out
}

// dailyReturns returns the return of each day after the first.
func dailyReturns(daily []Point) []float64 {
	if len(daily) < 2 {
		return nil
	}
	out := make([]float64, 0, len(daily)-1)
	for i := 1; i < len(daily); i++ {
		out = append(out, ratio(daily[i].Value, daily[i-1].Value)-1)
	}
	return out
}

// average is the arithmetic mean.
func average(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

// stdev is the sample standard deviation.
func stdev(xs []float64, mean float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	sum := 0.0
	for _, x := range xs {
		d := x - mean
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(xs)-1))
}

// downsideDeviation is the standard deviation of the returns below
// zero. The upside does not count against it.
func downsideDeviation(xs []float64, mean float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range xs {
		d := math.Min(x-mean, 0)
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(xs)))
}

// ratio divides, and reports zero when it cannot.
func ratio(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

// sameDay reports whether two times fall on the same calendar day.
func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
