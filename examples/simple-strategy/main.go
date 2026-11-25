// simple-strategy is a minimal example showing how a strategy uses
// the trading-framework package.
//
// It registers two Jobs (refresh-universe at 09:00, rebalance at 16:00)
// and runs a backtest over October 2026.
//
// Everything the strategy reads comes from the market-data-service: the
// SPY's constituents, the daily bars and the exchange calendar. The
// service has to be running:
//
//	docker compose -f ../market-data-service/docker-compose.yml up -d
//
// The rebalance strategy is a moving-average crossover over those
// constituents:
//
//   - long when the SMA50 is above the SMA200, short when it is below
//   - the SMA200 is the stop loss as well: a long is closed when the
//     close falls to it, a short when the close rises to it. It is
//     recomputed every day, so the stop follows the average
//   - a position is also closed when the crossover turns the other
//     way. The name is a candidate again from the next day on
//   - at most 10 positions are open at once; when more names want in
//     than there are slots for them, the ones with the lowest
//     volatility (the ATR as a percentage of the price) win them
//   - every position is sized at an equal share of the portfolio
//
// Run from the repo root:
//
//	go run ./examples/simple-strategy
package main

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	indicators "github.com/marcuspeh/indicators-go"
	marketdata "github.com/marcuspeh/market-data-service/sdk/go"
	"github.com/marcuspeh/trading-framework/app"
	"github.com/marcuspeh/trading-framework/calendar"
	"github.com/marcuspeh/trading-framework/clients"
	"github.com/marcuspeh/trading-framework/clock"
	"github.com/marcuspeh/trading-framework/config"
	"github.com/marcuspeh/trading-framework/job"
	"github.com/marcuspeh/trading-framework/order"
)

const (
	// universeETF is the ETF whose constituents the strategy trades.
	universeETF = "SPY"

	// fastPeriod and slowPeriod are the SMA windows of the crossover.
	// The slow one is also the stop loss.
	fastPeriod = 50
	slowPeriod = 200

	// atrPeriod is the window of the ATR that measures volatility.
	atrPeriod = 14

	// maxPositions is the number of positions the strategy may hold at
	// once. The config client can override it.
	maxPositions = 10

	// barWorkers is how many names' bars are fetched at once.
	//
	// The market-data-service takes a couple of seconds to answer a
	// bar request and the universe runs to hundreds of names, so
	// fetching them one at a time makes a backtest take days.
	barWorkers = 24
)

// setup is what the strategy knows about one symbol on one day.
type setup struct {
	symbol string
	// price is the last close.
	price float64
	// smaFast and smaSlow are the two moving averages.
	smaFast float64
	smaSlow float64
	// volatility is the ATR as a percentage of the price. It is the
	// risk the tiebreak ranks the candidates by.
	volatility float64
}

// long reports whether the crossover asks for a long position.
func (s setup) long() bool { return s.smaFast > s.smaSlow }

// short reports whether the crossover asks for a short position.
func (s setup) short() bool { return s.smaFast < s.smaSlow }

// computeSetup reads the crossover and the volatility off a symbol's
// bars.
//
// The indicator maths runs on the framework's bars. order.Bar is the
// library's bar type, so it feeds straight in.
func computeSetup(symbol string, bars []order.Bar) setup {
	fast := indicators.NewSMA(fastPeriod)
	slow := indicators.NewSMA(slowPeriod)
	atr := indicators.NewATR(atrPeriod)

	var f, s, a float64
	for i := range bars {
		f = fast.Update(&bars[i])
		s = slow.Update(&bars[i])
		a = atr.Update(&bars[i])
	}

	price := bars[len(bars)-1].Close
	return setup{
		symbol:     symbol,
		price:      price,
		smaFast:    f,
		smaSlow:    s,
		volatility: a / price,
	}
}

func refresher(ctx job.Context, date time.Time) ([]order.Order, error) {
	if !ctx.IsTradingDay(date) {
		return nil, nil
	}
	syms, err := ctx.Constituents(universeETF)
	if err != nil {
		return nil, err
	}
	ctx.Logger().Info("universe refreshed",
		"date", date.Format("2006-01-02"),
		"count", len(syms),
	)
	return nil, nil
}

func rebalance(ctx job.Context, date time.Time) ([]order.Order, error) {
	if !ctx.IsTradingDay(date) {
		return nil, nil
	}

	syms, err := ctx.Constituents(universeETF)
	if err != nil {
		return nil, err
	}
	if len(syms) == 0 {
		return nil, nil
	}

	limit, err := positionLimit(ctx)
	if err != nil {
		return nil, err
	}

	// What the crossover says about every name in the universe.
	setups, noHistory := fetchSetups(ctx, syms)
	if len(noHistory) > 0 {
		ctx.Logger().Warn("symbols skipped",
			"count", len(noHistory),
			"reason", "not enough history",
			"examples", names(noHistory),
		)
	}

	// What the strategy holds today.
	held := map[string]float64{}
	for _, p := range ctx.Positions() {
		held[p.Symbol] = p.Quantity
	}

	var closes []order.Order
	var exited int

	for _, s := range setups {
		qty, ok := held[s.symbol]
		if !ok {
			continue
		}
		switch {
		case qty > 0 && (s.price <= s.smaSlow || s.short()):
			// The long is done: the SMA200 stop was hit, or the
			// crossover turned and says the other side. The slot is
			// freed.
			closes = append(closes, order.Order{
				Symbol: s.symbol, Side: order.Sell, Quantity: qty, Type: order.Market,
			})
			exited++
		case qty < 0 && (s.price >= s.smaSlow || s.long()):
			// The short is done, for the same two reasons.
			closes = append(closes, order.Order{
				Symbol: s.symbol, Side: order.Buy, Quantity: -qty, Type: order.Market,
			})
			exited++
		}
	}

	// The names that want a position and do not have one. A name that
	// was closed today is not one of them: it acts once a day.
	var candidates []setup
	for _, s := range setups {
		if _, ok := held[s.symbol]; ok {
			continue
		}
		if s.long() || s.short() {
			candidates = append(candidates, s)
		}
	}

	// The slots left over: the cap, less the positions that stay open.
	free := limit - (len(held) - exited)
	if free < 0 {
		free = 0
	}

	// More names want in than there are slots for them. The tiebreak
	// is the risk: the least volatile name wins the slot.
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].volatility < candidates[j].volatility
	})
	if free > len(candidates) {
		free = len(candidates)
	}
	opened := candidates[:free]
	skipped := candidates[free:]

	value := ctx.PortfolioValue()
	budget := value / float64(limit)
	cash := ctx.Cash()

	// The last close of every name, which is the price the day's
	// orders fill at.
	priceOf := make(map[string]float64, len(setups))
	for _, s := range setups {
		priceOf[s.symbol] = s.price
	}

	orders := make([]order.Order, 0, len(closes)+len(opened))
	orders = append(orders, closes...)

	// The closes come first: a short's cover frees cash before the
	// next entry is sized.
	for _, o := range closes {
		cash += cashChange(o, priceOf)
	}

	for _, s := range opened {
		side := order.Buy
		if s.short() {
			side = order.Sell
		}
		qty := math.Floor(budget / s.price)
		if qty < 1 {
			ctx.Logger().Warn("position too small to open",
				"symbol", s.symbol,
				"price", s.price,
				"budget", budget,
			)
			continue
		}
		if side == order.Buy && qty*s.price > cash {
			// The portfolio cannot fund the position at its equal
			// share, so it is sized at what the cash can buy.
			qty = math.Floor(cash / s.price)
			if qty < 1 {
				ctx.Logger().Warn("not enough cash to open",
					"symbol", s.symbol,
					"price", s.price,
					"cash", cash,
				)
				continue
			}
		}
		o := order.Order{Symbol: s.symbol, Side: side, Quantity: qty, Type: order.Market}
		cash += cashChange(o, priceOf)
		orders = append(orders, o)
	}

	if len(skipped) > 0 {
		// The universe is wider than the number of slots, so most days
		// leave names without one. The ones that missed by the least
		// are the interesting ones.
		missed := make([]string, 0, len(skipped))
		for _, s := range skipped {
			missed = append(missed, s.symbol)
		}
		ctx.Logger().Info("candidates skipped",
			"count", len(skipped),
			"reason", "no free slot",
			"closest", names(missed),
		)
	}

	ctx.Logger().Info("rebalance",
		"date", date.Format("2006-01-02"),
		"portfolio_value", value,
		"cash", ctx.Cash(),
		"positions", len(held),
		"candidates", len(candidates),
		"opened", len(opened),
		"exited", exited,
		"orders", len(orders),
	)

	return orders, nil
}

// fetchSetups reads the crossover and the volatility for every symbol.
//
// The bars are fetched concurrently: the service takes a couple of
// seconds to answer a request, and the universe has hundreds of names,
// so one at a time would make a backtest take days. The results keep
// the order of syms, so a run comes out the same however the requests
// were scheduled.
//
// A name without enough history has no crossover yet. It is skipped,
// not fatal: the universe is wider than the history the service has
// collected.
func fetchSetups(ctx job.Context, syms []string) ([]setup, []string) {
	setups := make([]setup, len(syms))
	fetched := make([]bool, len(syms))

	var wg sync.WaitGroup
	sem := make(chan struct{}, barWorkers)
	for i, sym := range syms {
		wg.Add(1)
		go func(i int, sym string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			bars, err := ctx.Bars(sym, slowPeriod)
			if err != nil {
				return
			}
			setups[i] = computeSetup(sym, bars)
			fetched[i] = true
		}(i, sym)
	}
	wg.Wait()

	kept := make([]setup, 0, len(syms))
	var noHistory []string
	for i, sym := range syms {
		if fetched[i] {
			kept = append(kept, setups[i])
			continue
		}
		noHistory = append(noHistory, sym)
	}
	return kept, noHistory
}

// names returns up to five names, for a log line that names a handful
// of a much longer list.
func names(syms []string) []string {
	if len(syms) > 5 {
		return syms[:5]
	}
	return syms
}

// positionLimit reads the number of positions the strategy may hold.
//
// The config client may narrow it; the default is maxPositions.
func positionLimit(ctx job.Context) (int, error) {
	raw, err := ctx.Config().Get("max_positions")
	if err != nil {
		if err == job.ErrNotFound {
			return maxPositions, nil
		}
		return 0, err
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("max_positions: %w", err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("max_positions: %d is not a position count", n)
	}
	return n, nil
}

// cashChange is what an order does to the cash, at the last close.
//
// A buy spends cash and a sell raises it. It is an estimate: the fill
// carries slippage and a commission, so the real cash differs a little.
// It is enough to keep the strategy from spending money it does not
// have.
func cashChange(o order.Order, priceOf map[string]float64) float64 {
	notional := o.Quantity * priceOf[o.Symbol]
	if o.Side == order.Buy {
		return -notional
	}
	return notional
}

// marketDataURL is where the market-data-service runs.
//
// It is the same variable the reference algo reads, so both talk to the
// same service. An empty value leaves the SDK on its own default.
func marketDataURL() string {
	return os.Getenv("MARKET_DATA_URL")
}

// backtestWindow is the window the example runs over: January 2025.
//
// The strategy needs 200 sessions of history before it can read a
// crossover, so the window is where the service's history is, not
// where the strategy's is. Change the dates to move it.
func backtestWindow() (start, end time.Time) {
	start = time.Date(2025, 1, 1, 0, 0, 0, 0, clock.NYSE)
	end = time.Date(2025, 2, 1, 0, 0, 0, 0, clock.NYSE)
	return start, end
}

func main() {
	// The market-data-service is the source of everything the strategy
	// reads: the constituents, the daily bars and the exchange
	// calendar.
	md := marketdata.NewClient(marketDataURL())
	cal := calendar.NewService(md)

	// The market adapter serves the strategy and the fill model from
	// the same cached series, so the two can never see different
	// prices.
	market := clients.NewMarket(md, cal)
	universe := clients.NewConstituents(md)

	cfg := config.NewMap(map[string]string{
		"strategy":      "simple",
		"max_positions": "10",
	})

	start, end := backtestWindow()

	a := app.NewApp(
		app.WithCalendarProvider(cal),
		app.WithBacktest(app.BacktestConfig{
			Start:        start,
			End:          end,
			StartingCash: 10_000,

			SlippageBps: 5,
			Commission:  1,
		}),
		app.WithMarketClient(market),
		app.WithUniverseClient(universe),
		app.WithConfigClient(cfg),
		app.WithStdoutLogger(),
	)

	if err := a.RegisterJob(job.Job{
		Name:    "refresh-universe",
		Time:    job.Time(9, 0),
		Handler: refresher,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "register refresh-universe:", err)
		os.Exit(1)
	}
	if err := a.RegisterJob(job.Job{
		Name:    "rebalance",
		Time:    job.Time(16, 0),
		Handler: rebalance,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "register rebalance:", err)
		os.Exit(1)
	}

	if err := a.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "app:", err)
		os.Exit(1)
	}
}
