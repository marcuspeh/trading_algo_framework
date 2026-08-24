package indicators

// Indicator defines the interface for a technical analysis indicator.
type Indicator interface {
	// Update adds a new price bar to the indicator and returns its latest calculated value.
	Update(bar *Bar) float64
	// Value returns the most recently calculated value of the indicator.
	Value() float64
	// IsReady checks if the indicator has enough historical data to produce a valid value.
	IsReady() bool
}
