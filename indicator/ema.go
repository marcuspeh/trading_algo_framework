package indicator

type EMA struct {
	period  int
	alpha   float64
	value   float64
	isReady bool
	count   int
}

func NewEMA(period int) *EMA {
	return &EMA{
		period: period,
		alpha:  2.0 / (float64(period) + 1.0),
	}
}

func (e *EMA) Update(bar *Bar) float64 {
	val := bar.Close
	if !e.isReady {
		e.count++
		if e.count == 1 {
			e.value = val
		} else if e.count < e.period {
			e.value = (e.value*float64(e.count-1) + val) / float64(e.count)
		} else if e.count == e.period {
			e.value = (e.value*float64(e.count-1) + val) / float64(e.count)
			e.isReady = true
		}
	} else {
		e.value = (val * e.alpha) + (e.value * (1.0 - e.alpha))
	}
	return e.value
}

func (e *EMA) Value() float64 {
	return e.value
}

func (e *EMA) IsReady() bool {
	return e.isReady
}
