package indicator

type RSI struct {
	period    int
	prevClose float64
	avgGain   float64
	avgLoss   float64
	count     int
	isReady   bool
	isInit    bool
	lastValue float64
}

func NewRSI(period int) *RSI {
	return &RSI{
		period: period,
	}
}

func (r *RSI) Update(bar *Bar) float64 {
	if !r.isInit {
		r.isInit = true
		r.prevClose = bar.Close
		return 0
	}

	diff := bar.Close - r.prevClose
	r.prevClose = bar.Close

	gain := 0.0
	loss := 0.0
	if diff > 0 {
		gain = diff
	} else {
		loss = -diff
	}

	r.count++
	if r.count <= r.period {
		r.avgGain += gain / float64(r.period)
		r.avgLoss += loss / float64(r.period)
		if r.count == r.period {
			r.isReady = true
		}
	} else {
		alpha := 1.0 / float64(r.period)
		r.avgGain = (gain * alpha) + (r.avgGain * (1.0 - alpha))
		r.avgLoss = (loss * alpha) + (r.avgLoss * (1.0 - alpha))
	}

	if r.isReady {
		if r.avgLoss == 0 {
			r.lastValue = 100
		} else {
			rs := r.avgGain / r.avgLoss
			r.lastValue = 100 - (100 / (1 + rs))
		}
	} else {
		r.lastValue = 0
	}

	return r.lastValue
}

func (r *RSI) Value() float64 {
	return r.lastValue
}

func (r *RSI) IsReady() bool {
	return r.isReady
}
