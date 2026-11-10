package indicator

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

// Update feeds a bar into the tracker and returns the minimum value.
// Use UpdateMinMax to read both the minimum and the maximum.
func (m *MinMax) Update(bar *Bar) float64 {
	min, _ := m.UpdateMinMax(bar)
	return min
}

// UpdateMinMax feeds a bar into the tracker and returns the minimum and
// maximum values.
func (m *MinMax) UpdateMinMax(bar *Bar) (float64, float64) {
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

// UpdateBands feeds a bar into the tracker and returns the minimum and
// maximum values, so MinMax can be used with FeedBars.
func (m *MinMax) UpdateBands(bar *Bar) (float64, float64, float64) {
	min, max := m.UpdateMinMax(bar)
	return min, max, max
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

func (m *MinMax) Value() float64 {
	return m.min
}
