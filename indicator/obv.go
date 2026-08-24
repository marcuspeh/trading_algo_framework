package indicators

type OBV struct {
	prevClose float64
	value     float64
	isInit    bool
}

func NewOBV() *OBV {
	return &OBV{}
}

func (o *OBV) Update(bar *Bar) float64 {
	if !o.isInit {
		o.isInit = true
		o.prevClose = bar.Close
		o.value = bar.Volume
		return o.value
	}

	if bar.Close > o.prevClose {
		o.value += bar.Volume
	} else if bar.Close < o.prevClose {
		o.value -= bar.Volume
	}

	o.prevClose = bar.Close
	return o.value
}

func (o *OBV) Value() float64 {
	return o.value
}

func (o *OBV) IsReady() bool {
	return o.isInit
}
