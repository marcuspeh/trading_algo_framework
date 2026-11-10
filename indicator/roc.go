package indicator

import (
	"container/ring"
)

type ROC struct {
	period int
	count  int
	buffer *ring.Ring
	value  float64
}

func NewROC(period int) *ROC {
	return &ROC{
		period: period,
		buffer: ring.New(period),
	}
}

func (r *ROC) Update(bar *Bar) float64 {
	val := bar.Close
	if r.count >= r.period {
		prevVal := r.buffer.Value.(float64)
		if prevVal != 0 {
			r.value = ((val - prevVal) / prevVal) * 100
		}
	} else {
		r.count++
	}

	r.buffer.Value = val
	r.buffer = r.buffer.Next()
	return r.value
}

func (r *ROC) Value() float64 {
	return r.value
}

func (r *ROC) IsReady() bool {
	return r.count >= r.period
}
