package indicator

import (
	"container/ring"
)

type SMA struct {
	period int
	sum    float64
	count  int
	buffer *ring.Ring
	value  float64
}

func NewSMA(period int) *SMA {
	return &SMA{
		period: period,
		buffer: ring.New(period),
	}
}

func (s *SMA) Update(bar *Bar) float64 {
	val := bar.Close
	if s.count >= s.period {
		s.sum -= s.buffer.Value.(float64)
	} else {
		s.count++
	}

	s.sum += val
	s.buffer.Value = val
	s.buffer = s.buffer.Next()

	if s.count == s.period {
		s.value = s.sum / float64(s.period)
	} else {
		s.value = 0
	}
	return s.value
}

func (s *SMA) Value() float64 {
	return s.value
}

func (s *SMA) IsReady() bool {
	return s.count >= s.period
}
