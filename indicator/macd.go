package indicators

type MACD struct {
	fastEMA   *EMA
	slowEMA   *EMA
	signalEMA *EMA
	macd      float64
	signal    float64
	hist      float64
	isReady   bool
}

func NewMACD(fastPeriod, slowPeriod, signalPeriod int) *MACD {
	return &MACD{
		fastEMA:   NewEMA(fastPeriod),
		slowEMA:   NewEMA(slowPeriod),
		signalEMA: NewEMA(signalPeriod),
	}
}

func (m *MACD) Update(bar *Bar) float64 {
	fast := m.fastEMA.Update(bar)
	slow := m.slowEMA.Update(bar)

	m.macd = fast - slow
	if m.slowEMA.IsReady() {
		m.signal = m.signalEMA.Update(&Bar{Close: m.macd})
		if m.signalEMA.count >= m.signalEMA.period {
			m.hist = m.macd - m.signal
			m.isReady = true
		}
	}
	return m.macd
}

func (m *MACD) Value() float64 {
	return m.macd
}

func (m *MACD) Signal() float64 {
	return m.signal
}

func (m *MACD) Histogram() float64 {
	return m.hist
}

func (m *MACD) IsReady() bool {
	return m.isReady
}
