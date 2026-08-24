package indicators

import (
	"math"
)

type BollingerBands struct {
	period int
	sma    *SMA
	stdDev float64
	upper  float64
	middle float64
	lower  float64
}

func NewBollingerBands(period int, stdDev float64) *BollingerBands {
	return &BollingerBands{
		period: period,
		sma:    NewSMA(period),
		stdDev: stdDev,
	}
}

func (b *BollingerBands) Update(bar *Bar) (float64, float64, float64) {
	b.middle = b.sma.Update(bar)
	if b.sma.IsReady() {
		sumSq := 0.0
		b.sma.buffer.Do(func(p interface{}) {
			if p != nil {
				v := p.(float64)
				sumSq += math.Pow(v-b.middle, 2)
			}
		})
		std := math.Sqrt(sumSq / float64(b.period))
		b.upper = b.middle + (std * b.stdDev)
		b.lower = b.middle - (std * b.stdDev)
	}
	return b.upper, b.middle, b.lower
}

func (b *BollingerBands) Upper() float64 {
	return b.upper
}

func (b *BollingerBands) Middle() float64 {
	return b.middle
}

func (b *BollingerBands) Lower() float64 {
	return b.lower
}

func (b *BollingerBands) IsReady() bool {
	return b.sma.IsReady()
}

func (b *BollingerBands) UpdateIndicator(bar *Bar) float64 {
	_, middle, _ := b.Update(bar)
	return middle
}

func (b *BollingerBands) Value() float64 {
	return b.middle
}
