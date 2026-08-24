package indicators

import (
	"math"
)

type ATR struct {
	period    int
	prevClose float64
	ema       *EMA
	isInit    bool
	isReady   bool
	count     int
	lastValue float64
}

func NewATR(period int) *ATR {
	return &ATR{
		period: period,
		ema:    NewEMA(period),
	}
}

func (a *ATR) Update(bar *Bar) float64 {
	if !a.isInit {
		a.isInit = true
		a.prevClose = bar.Close
		return 0
	}

	tr := math.Max(bar.High-bar.Low, math.Max(math.Abs(bar.High-a.prevClose), math.Abs(bar.Low-a.prevClose)))
	a.prevClose = bar.Close

	a.count++
	if a.count <= a.period {
		a.lastValue += tr / float64(a.period)
		if a.count == a.period {
			a.isReady = true
		}
	} else {
		alpha := 1.0 / float64(a.period)
		a.lastValue = (tr * alpha) + (a.lastValue * (1.0 - alpha))
	}

	return a.lastValue
}

func (a *ATR) Value() float64 {
	return a.lastValue
}

func (a *ATR) IsReady() bool {
	return a.isReady
}
