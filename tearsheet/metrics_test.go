package tearsheet

import (
	"math"
	"testing"
	"time"
)

func TestMetrics_TotalReturnAndEndingValue(t *testing.T) {
	points := []Point{
		{Time: day(1), Value: 10_000},
		{Time: day(2), Value: 10_500},
		{Time: day(3), Value: 11_000},
	}

	m := computeMetrics(points, dailyCloses(points), nil, 0, 0)
	if m.StartingValue != 10_000 {
		t.Errorf("expected a starting value of 10000, got %v", m.StartingValue)
	}
	if m.EndingValue != 11_000 {
		t.Errorf("expected an ending value of 11000, got %v", m.EndingValue)
	}
	if math.Abs(m.TotalReturn-0.1) > 1e-9 {
		t.Errorf("expected a return of 0.1, got %v", m.TotalReturn)
	}
}

func TestMetrics_MaxDrawdown(t *testing.T) {
	points := []Point{
		{Time: day(1), Value: 10_000},
		{Time: day(2), Value: 12_000},
		{Time: day(3), Value: 9_000},
		{Time: day(4), Value: 11_000},
	}

	m := computeMetrics(points, dailyCloses(points), nil, 0, 0)
	// From 12000 down to 9000 is a fall of 25%.
	if math.Abs(m.MaxDrawdown-0.25) > 1e-9 {
		t.Errorf("expected a drawdown of 0.25, got %v", m.MaxDrawdown)
	}
	if !m.MaxDrawdownFrom.Equal(day(2)) {
		t.Errorf("expected the drawdown to start on day 2, got %v", m.MaxDrawdownFrom)
	}
	if !m.MaxDrawdownTo.Equal(day(3)) {
		t.Errorf("expected the drawdown to end on day 3, got %v", m.MaxDrawdownTo)
	}
}

func TestMetrics_SharpeAndVolatility(t *testing.T) {
	// A steady 1% a day.
	points := []Point{
		{Time: day(1), Value: 10_000},
		{Time: day(2), Value: 10_100},
		{Time: day(3), Value: 10_201},
		{Time: day(4), Value: 10_303.01},
	}

	m := computeMetrics(points, dailyCloses(points), nil, 0, 0)
	if m.Volatility != 0 {
		t.Errorf("expected no volatility from a flat series, got %v", m.Volatility)
	}
	if m.Sharpe != 0 {
		t.Errorf("expected no Sharpe from a flat series, got %v", m.Sharpe)
	}
}

func TestMetrics_WinRateAndProfitFactor(t *testing.T) {
	trades := []Trade{
		{PnL: 100},
		{PnL: 50},
		{PnL: -30},
		{PnL: -20},
	}

	m := computeMetrics(nil, nil, trades, 0, 0)
	if math.Abs(m.WinRate-0.5) > 1e-9 {
		t.Errorf("expected a win rate of 0.5, got %v", m.WinRate)
	}
	// 150 of profit over 50 of loss.
	if math.Abs(m.ProfitFactor-3) > 1e-9 {
		t.Errorf("expected a profit factor of 3, got %v", m.ProfitFactor)
	}
	if m.AvgWin != 75 {
		t.Errorf("expected an average win of 75, got %v", m.AvgWin)
	}
	if m.AvgLoss != -25 {
		t.Errorf("expected an average loss of -25, got %v", m.AvgLoss)
	}
	if m.BestTrade != 100 || m.WorstTrade != -30 {
		t.Errorf("expected a best of 100 and a worst of -30, got %v and %v", m.BestTrade, m.WorstTrade)
	}
}

func TestMetrics_CAGRCompoundsOverTheRun(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(2, 0, 0)

	points := []Point{
		{Time: start, Value: 10_000},
		{Time: end, Value: 12_000},
	}

	m := computeMetrics(points, dailyCloses(points), nil, 0, 0)
	// Two years from 10000 to 12000 is about 9.54% a year.
	if m.CAGR < 0.095 || m.CAGR > 0.096 {
		t.Errorf("expected a CAGR of about 0.0954, got %v", m.CAGR)
	}
}

func TestMonthlyReturns_CompoundsWithinAndAcrossMonths(t *testing.T) {
	points := []Point{
		{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Value: 10_000},
		{Time: time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC), Value: 11_000},
		{Time: time.Date(2025, 2, 28, 0, 0, 0, 0, time.UTC), Value: 12_100},
	}

	months := monthlyReturns(points)
	if len(months) != 2 {
		t.Fatalf("expected 2 months, got %d", len(months))
	}
	// January is measured from the start of the run.
	if math.Abs(months[0].Return-0.1) > 1e-9 {
		t.Errorf("expected January to return 0.1, got %v", months[0].Return)
	}
	// February is measured from the end of January.
	if math.Abs(months[1].Return-0.1) > 1e-9 {
		t.Errorf("expected February to return 0.1, got %v", months[1].Return)
	}
}

func TestDailyCloses_TakesTheLastSampleOfEachDay(t *testing.T) {
	points := []Point{
		{Time: time.Date(2025, 1, 2, 9, 0, 0, 0, time.UTC), Value: 10_000},
		{Time: time.Date(2025, 1, 2, 16, 0, 0, 0, time.UTC), Value: 10_500},
		{Time: time.Date(2025, 1, 3, 16, 0, 0, 0, time.UTC), Value: 10_600},
	}

	daily := dailyCloses(points)
	if len(daily) != 2 {
		t.Fatalf("expected 2 days, got %d", len(daily))
	}
	if daily[0].Value != 10_500 {
		t.Errorf("expected the close of 10500, got %v", daily[0].Value)
	}
}
