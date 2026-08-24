package indicators

import (
	"container/ring"
)

type MinMax struct {
	period int
	count  int
	buffer *ring.Ring
	min    float64
	max    float64
}

func NewMinMax(period int) *MinMax {
	return &MinMax{
		period: period,
		buffer: ring.New(period),
	}
}

func (m *MinMax) Update(bar *Bar) (float64, float64) {
	val := bar.Close
	m.buffer.Value = val
	m.buffer = m.buffer.Next()

	if m.count < m.period {
		m.count++
	}

	if m.count == m.period {
		min := val
		max := val
		m.buffer.Do(func(p interface{}) {
			if p != nil {
				v := p.(float64)
				if v < min {
					min = v
				}
				if v > max {
					max = v
				}
			}
		})
		m.min = min
		m.max = max
	}
	return m.min, m.max
}

func (m *MinMax) Min() float64 {
	return m.min
}

func (m *MinMax) Max() float64 {
	return m.max
}

func (m *MinMax) IsReady() bool {
	return m.count >= m.period
}

func (m *MinMax) UpdateIndicator(bar *Bar) float64 {
	min, _ := m.Update(bar)
	return min
}

func (m *MinMax) Value() float64 {
	return m.min
}
