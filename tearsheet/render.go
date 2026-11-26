package tearsheet

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"math"
	"strings"
)

// Render writes the tear sheet as a single, self-contained HTML page.
//
// The page carries its own styles and its own script: nothing is
// fetched from the network, so it opens from a file:// URL and still
// works offline.
func Render(w io.Writer, rep Report) error {
	data, err := buildPage(rep)
	if err != nil {
		return err
	}
	return pageTemplate.Execute(w, data)
}

// pageData is what the template renders.
type pageData struct {
	Title     string
	Subtitle  string
	Mode      string
	Period    string
	Generated string

	HeroLabel string
	HeroValue string
	HeroNote  string

	Tiles []tile

	EquitySVG   template.HTML
	EquityData  template.JS
	DrawdownSVG template.HTML
	DrawData    template.JS
	MonthlySVG  template.HTML

	MonthlyHead []string
	MonthlyRows [][]string
	TradeHead   []string
	TradeRows   [][]string

	HasEquity  bool
	HasMonthly bool
	HasTrades  bool
}

// tile is one number in the row under the hero.
type tile struct {
	Label string
	Value string
	Note  string
}

// buildPage assembles everything the page shows.
func buildPage(rep Report) (*pageData, error) {
	cur := rep.Meta.Currency
	if cur == "" {
		cur = "USD"
	}

	d := &pageData{
		Mode:      strings.ToUpper(rep.Meta.Mode),
		Generated: rep.Meta.Generated.Format("2006-01-02 15:04:05 MST"),
	}
	if d.Mode == "" {
		d.Mode = "RUN"
	}

	d.Title = "Tear sheet"
	if !rep.Meta.Start.IsZero() && !rep.Meta.End.IsZero() {
		d.Period = fmt.Sprintf("%s → %s",
			rep.Meta.Start.Format("2006-01-02"),
			rep.Meta.End.Format("2006-01-02"),
		)
	} else if !rep.Meta.Start.IsZero() {
		d.Period = "since " + rep.Meta.Start.Format("2006-01-02")
	}
	d.Subtitle = strings.TrimSpace(strings.Join([]string{d.Mode, d.Period}, " · "))

	m := rep.Metrics
	d.HeroLabel = "Total return"
	d.HeroValue = signedPct(m.TotalReturn, 2)
	if m.StartingValue != 0 {
		d.HeroNote = fmt.Sprintf("%s → %s",
			money(m.StartingValue, cur), money(m.EndingValue, cur))
	}

	d.Tiles = []tile{
		{Label: "CAGR", Value: signedPct(m.CAGR, 2)},
		{Label: "Sharpe", Value: ratio2(m.Sharpe)},
		{Label: "Sortino", Value: ratio2(m.Sortino)},
		{Label: "Max drawdown", Value: pct(m.MaxDrawdown, 2)},
		{Label: "Volatility", Value: pct(m.Volatility, 1)},
		{Label: "Win rate", Value: pct(m.WinRate, 1)},
		{Label: "Profit factor", Value: ratio2(m.ProfitFactor)},
		{Label: "Trades", Value: fmt.Sprintf("%d", m.TotalTrades)},
		{Label: "Exposure", Value: pct(m.Exposure, 1)},
		{Label: "Fees", Value: money(m.TotalFees, cur)},
	}
	if m.AvgWin != 0 || m.AvgLoss != 0 {
		d.Tiles = append(d.Tiles,
			tile{Label: "Avg win", Value: money(m.AvgWin, cur)},
			tile{Label: "Avg loss", Value: money(m.AvgLoss, cur)},
		)
	}
	if m.BestTrade != 0 || m.WorstTrade != 0 {
		d.Tiles = append(d.Tiles,
			tile{Label: "Best trade", Value: money(m.BestTrade, cur)},
			tile{Label: "Worst trade", Value: money(m.WorstTrade, cur)},
		)
	}

	eq, eqData, err := equityChart(rep, cur)
	if err != nil {
		return nil, err
	}
	d.EquitySVG, d.EquityData = eq, eqData

	dd, ddData, err := drawdownChart(rep, cur)
	if err != nil {
		return nil, err
	}
	d.DrawdownSVG, d.DrawData = dd, ddData

	d.MonthlySVG, d.MonthlyHead, d.MonthlyRows = monthlyChart(rep, cur)

	d.TradeHead, d.TradeRows = tradeTable(rep, cur)

	d.HasEquity = len(rep.Daily) > 0
	d.HasMonthly = len(rep.Monthly) > 0
	d.HasTrades = len(rep.Trades) > 0
	return d, nil
}

// curvePoint is one point of a chart's data.
//
// The label is formatted here rather than in the script, so the tooltip
// and the axes agree on how a number reads.
type curvePoint struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Date  string  `json:"date"`
	Value string  `json:"value"`
}

// equityChart draws the portfolio's value over the run.
func equityChart(rep Report, cur string) (template.HTML, template.JS, error) {
	if len(rep.Daily) == 0 {
		return "", "", nil
	}

	const (
		w = 960.0
		h = 300.0
	)
	ml, mr, mt, mb := 68.0, 18.0, 14.0, 30.0

	values := make([]float64, len(rep.Daily))
	for i, p := range rep.Daily {
		values[i] = p.Value
	}
	lo, hi := minMax(values)
	if lo > rep.Metrics.StartingValue && rep.Metrics.StartingValue != 0 {
		lo = rep.Metrics.StartingValue
	}
	lo, hi = pad(lo, hi, 0.06)

	px := linspace(ml, w-mr, len(values))
	py := make([]float64, len(values))
	for i, v := range values {
		py[i] = scaleY(v, lo, hi, mt, h-mb)
	}

	var b strings.Builder
	gridY(tickValues(lo, hi, 5), lo, hi, mt, h-mb, ml, w-mr, func(v float64) string {
		return compactMoney(v, cur)
	}, &b)

	b.WriteString(fmt.Sprintf(`<path class="eq-line" d="%s"/>`,
		linePath(px, py)))

	// The last value carries an end marker and a direct label: it is
	// the number the chart is about.
	b.WriteString(fmt.Sprintf(`<circle class="eq-dot" cx="%.2f" cy="%.2f" r="4.5"/>`,
		px[len(px)-1], py[len(py)-1]))
	b.WriteString(fmt.Sprintf(`<text class="eq-end" x="%.2f" y="%.2f" text-anchor="end">%s</text>`,
		px[len(px)-1]-8, py[len(py)-1]-18, money(values[len(values)-1], cur)))

	// The hover layer: a hairline that snaps to the nearest sample and
	// a marker on it.
	b.WriteString(fmt.Sprintf(`<line class="hair" x1="0" y1="%.2f" x2="0" y2="%.2f" style="display:none"/>`,
		mt, h-mb))
	b.WriteString(`<circle class="hair-dot" r="4.5" style="display:none"/>`)

	axisLabels(&b, rep.Daily, ml, w-mr, h-mb)

	data := make([]curvePoint, len(rep.Daily))
	for i, p := range rep.Daily {
		data[i] = curvePoint{
			X:     px[i],
			Y:     py[i],
			Date:  p.Time.Format("2006-01-02"),
			Value: money(p.Value, cur),
		}
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return "", "", err
	}

	svg := fmt.Sprintf(`<svg viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Portfolio value over the run">%s</svg>`, w, h, b.String())
	return template.HTML(svg), template.JS(raw), nil
}

// drawdownChart draws the fall from the running peak.
func drawdownChart(rep Report, cur string) (template.HTML, template.JS, error) {
	if len(rep.Drawdown) == 0 {
		return "", "", nil
	}

	const (
		w = 960.0
		h = 150.0
	)
	ml, mr, mt, mb := 68.0, 18.0, 12.0, 26.0

	values := make([]float64, len(rep.Drawdown))
	for i, p := range rep.Drawdown {
		values[i] = p.Value
	}
	lo, hi := minMax(values)
	if hi > 0 {
		hi = 0
	}
	lo, hi = pad(lo, hi, 0.08)

	px := linspace(ml, w-mr, len(values))
	py := make([]float64, len(values))
	for i, v := range values {
		py[i] = scaleY(v, lo, hi, mt, h-mb)
	}

	var b strings.Builder
	gridY(tickValues(lo, hi, 3), lo, hi, mt, h-mb, ml, w-mr, func(v float64) string {
		return signedPct(v, 1)
	}, &b)

	// The zero line is the baseline of the chart: everything below it
	// is the drawdown.
	b.WriteString(fmt.Sprintf(`<line class="base" x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f"/>`,
		ml, scaleY(0, lo, hi, mt, h-mb), w-mr, scaleY(0, lo, hi, mt, h-mb)))
	b.WriteString(fmt.Sprintf(`<path class="dd-area" d="%s"/>`,
		areaPath(px, py, 0, lo, mt, h-mb)))
	b.WriteString(fmt.Sprintf(`<path class="dd-line" d="%s"/>`,
		linePath(px, py)))

	b.WriteString(fmt.Sprintf(`<line class="hair" x1="0" y1="%.2f" x2="0" y2="%.2f" style="display:none"/>`,
		mt, h-mb))
	b.WriteString(`<circle class="hair-dot dd-dot" r="4.5" style="display:none"/>`)

	axisLabels(&b, rep.Daily, ml, w-mr, h-mb)

	data := make([]curvePoint, len(rep.Drawdown))
	for i, p := range rep.Drawdown {
		data[i] = curvePoint{
			X:     px[i],
			Y:     py[i],
			Date:  p.Time.Format("2006-01-02"),
			Value: signedPct(p.Value, 2),
		}
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return "", "", err
	}

	svg := fmt.Sprintf(`<svg viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Drawdown from the running peak">%s</svg>`, w, h, b.String())
	return template.HTML(svg), template.JS(raw), nil
}

// monthlyChart draws the return of each month.
func monthlyChart(rep Report, cur string) (template.HTML, []string, [][]string) {
	head := []string{"Month", "Return"}
	rows := make([][]string, 0, len(rep.Monthly))
	if len(rep.Monthly) == 0 {
		return "", head, rows
	}

	const (
		w = 960.0
		h = 190.0
	)
	ml, mr, mt, mb := 68.0, 18.0, 14.0, 30.0

	values := make([]float64, len(rep.Monthly))
	for i, m := range rep.Monthly {
		values[i] = m.Return
	}
	lo, hi := minMax(values)
	if lo > 0 {
		lo = 0
	}
	if hi < 0 {
		hi = 0
	}
	lo, hi = pad(lo, hi, 0.12)

	plotW := w - ml - mr
	band := plotW / float64(len(values))
	barW := math.Min(24, band-8)
	if barW < 2 {
		barW = math.Max(1, band-2)
	}

	zero := scaleY(0, lo, hi, mt, h-mb)

	var b strings.Builder
	gridY(tickValues(lo, hi, 4), lo, hi, mt, h-mb, ml, w-mr, func(v float64) string {
		return signedPct(v, 1)
	}, &b)

	for i, m := range rep.Monthly {
		x := ml + band*float64(i) + (band-barW)/2
		y := scaleY(m.Return, lo, hi, mt, h-mb)
		class := "bar pos"
		if m.Return < 0 {
			class = "bar neg"
		}
		top := math.Min(y, zero)
		height := math.Abs(zero - y)
		b.WriteString(fmt.Sprintf(
			`<path class="%s" d="%s" tabindex="0" data-label="%s" data-value="%s" `+
				`data-x="%.2f" data-y="%.2f" data-w="%.2f"/>`,
			class, columnPath(x, top, barW, height, 4, m.Return >= 0),
			m.Month, signedPct(m.Return, 2),
			x, top, barW))
		rows = append(rows, []string{m.Month, signedPct(m.Return, 2)})
	}

	b.WriteString(fmt.Sprintf(`<line class="base" x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f"/>`,
		ml, zero, w-mr, zero))

	// The first and the last month are labelled; the rest are carried
	// by the tooltip and the table.
	for _, i := range []int{0, len(rep.Monthly) - 1} {
		if i < 0 || i >= len(rep.Monthly) {
			continue
		}
		x := ml + band*float64(i) + band/2
		anchor := "middle"
		if i == 0 {
			anchor = "start"
		}
		if i == len(rep.Monthly)-1 {
			anchor = "end"
		}
		b.WriteString(fmt.Sprintf(`<text class="tick" x="%.2f" y="%.2f" text-anchor="%s">%s</text>`,
			x, h-mb+18, anchor, rep.Monthly[i].Month))
	}

	svg := fmt.Sprintf(`<svg viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Return of each calendar month">%s</svg>`, w, h, b.String())
	return template.HTML(svg), head, rows
}

// tradeTable lists the closed trades.
func tradeTable(rep Report, cur string) ([]string, [][]string) {
	head := []string{"Symbol", "Side", "Qty", "Entry", "Exit", "PnL", "Return", "Held"}
	rows := make([][]string, 0, len(rep.Trades))
	for _, t := range rep.Trades {
		rows = append(rows, []string{
			t.Symbol,
			t.Side,
			trimNum(t.Quantity),
			t.EntryTime.Format("2006-01-02") + " @ " + group(t.EntryPrice, 2),
			t.ExitTime.Format("2006-01-02") + " @ " + group(t.ExitPrice, 2),
			money(t.PnL, cur),
			signedPct(t.ReturnPct/100, 2),
			humanDuration(t.Duration),
		})
	}
	return head, rows
}

// gridY draws the horizontal gridlines and their labels.
//
// The label function decides how a value reads: an account's value is
// money, a return is a percentage.
func gridY(ticks []float64, lo, hi, top, bottom, left, right float64, label func(float64) string, b *strings.Builder) {
	for _, t := range ticks {
		y := scaleY(t, lo, hi, top, bottom)
		b.WriteString(fmt.Sprintf(`<line class="grid" x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f"/>`,
			left, y, right, y))
		b.WriteString(fmt.Sprintf(`<text class="tick" x="%.2f" y="%.2f" text-anchor="end">%s</text>`,
			left-10, y+4, label(t)))
	}
}

// columnPath builds the path of a column.
//
// The data-end is rounded and the baseline end is square: a column
// grows from the baseline, and only the end that carries the value is
// softened.
func columnPath(x, y, w, h, r float64, up bool) string {
	if h <= 0 || w <= 0 {
		return ""
	}
	if r > h/2 {
		r = h / 2
	}
	if r > w/2 {
		r = w / 2
	}

	if up {
		// The column grows up from the baseline at y+h.
		return fmt.Sprintf("M%.2f %.2f L%.2f %.2f Q%.2f %.2f %.2f %.2f "+
			"L%.2f %.2f Q%.2f %.2f %.2f %.2f L%.2f %.2f Z",
			x, y+h, x, y+r, x, y, x+r, y,
			x+w-r, y, x+w, y, x+w, y+r,
			x+w, y+h)
	}
	// The column grows down from the baseline at y.
	return fmt.Sprintf("M%.2f %.2f L%.2f %.2f Q%.2f %.2f %.2f %.2f "+
		"L%.2f %.2f Q%.2f %.2f %.2f %.2f L%.2f %.2f Z",
		x, y, x, y+h-r, x, y+h, x+r, y+h,
		x+w-r, y+h, x+w, y+h, x+w, y+h-r,
		x+w, y)
}

// axisLabels draws the first, the middle and the last date.
func axisLabels(b *strings.Builder, points []Point, left, right, bottom float64) {
	if len(points) == 0 {
		return
	}
	n := len(points)
	marks := []int{0, n / 2, n - 1}
	if n < 3 {
		marks = []int{0, n - 1}
	}
	for _, i := range marks {
		if i < 0 || i >= n {
			continue
		}
		x := left + (right-left)*float64(i)/float64(n-1)
		anchor := "middle"
		if i == 0 {
			anchor = "start"
		}
		if i == n-1 {
			anchor = "end"
		}
		b.WriteString(fmt.Sprintf(`<text class="tick" x="%.2f" y="%.2f" text-anchor="%s">%s</text>`,
			x, bottom+18, anchor, points[i].Time.Format("2006-01-02")))
	}
}

// linePath builds the path of a line.
func linePath(xs, ys []float64) string {
	var b strings.Builder
	for i := range xs {
		if i == 0 {
			fmt.Fprintf(&b, "M%.2f %.2f", xs[i], ys[i])
			continue
		}
		fmt.Fprintf(&b, " L%.2f %.2f", xs[i], ys[i])
	}
	return b.String()
}

// areaPath builds the path of the wash under a line.
//
// The wash is closed on the baseline the line grows from, so a value
// above the baseline fills upward and a value below it fills downward.
func areaPath(xs, ys []float64, baseLo, baseHi, top, bottom float64) string {
	if len(xs) == 0 {
		return ""
	}
	base := scaleY(baseHi, baseLo, baseHi, top, bottom)
	if baseLo == baseHi {
		base = bottom
	}
	var b strings.Builder
	fmt.Fprintf(&b, "M%.2f %.2f", xs[0], ys[0])
	for i := 1; i < len(xs); i++ {
		fmt.Fprintf(&b, " L%.2f %.2f", xs[i], ys[i])
	}
	fmt.Fprintf(&b, " L%.2f %.2f L%.2f %.2f Z", xs[len(xs)-1], base, xs[0], base)
	return b.String()
}

// scaleY maps a value onto the plot area.
func scaleY(v, lo, hi, top, bottom float64) float64 {
	if hi == lo {
		return (top + bottom) / 2
	}
	return bottom - (v-lo)/(hi-lo)*(bottom-top)
}

// linspace spreads n points between left and right.
func linspace(left, right float64, n int) []float64 {
	if n <= 1 {
		return []float64{left}
	}
	out := make([]float64, n)
	for i := range out {
		out[i] = left + (right-left)*float64(i)/float64(n-1)
	}
	return out
}

// minMax returns the smallest and the largest value.
func minMax(xs []float64) (float64, float64) {
	if len(xs) == 0 {
		return 0, 0
	}
	lo, hi := xs[0], xs[0]
	for _, x := range xs {
		if x < lo {
			lo = x
		}
		if x > hi {
			hi = x
		}
	}
	return lo, hi
}

// pad gives a range some air above and below.
func pad(lo, hi, frac float64) (float64, float64) {
	if hi == lo {
		d := math.Abs(hi) * frac
		if d == 0 {
			d = 1
		}
		return lo - d, hi + d
	}
	d := (hi - lo) * frac
	return lo - d, hi + d
}

// tickValues returns the round numbers an axis is labelled with.
func tickValues(lo, hi float64, count int) []float64 {
	if count < 2 {
		count = 2
	}
	step := niceStep((hi - lo) / float64(count))
	if step <= 0 {
		return nil
	}
	start := math.Ceil(lo/step) * step
	var out []float64
	// The ticks are counted from the start rather than accumulated, so
	// that the one at zero is zero and not a rounding error below it.
	for i := 0; ; i++ {
		v := start + float64(i)*step
		if v > hi+step/1000 {
			break
		}
		out = append(out, v)
	}
	return out
}

// niceStep rounds a raw step to a number a reader recognises.
func niceStep(raw float64) float64 {
	if raw <= 0 {
		return 0
	}
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	switch n := raw / mag; {
	case n <= 1:
		return mag
	case n <= 2:
		return 2 * mag
	case n <= 5:
		return 5 * mag
	default:
		return 10 * mag
	}
}
