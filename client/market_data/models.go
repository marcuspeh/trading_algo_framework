package etf

type Constituent struct {
	Ticker string  `json:"ticker"`
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
}

type ConstituentResponse struct {
	Symbol       string        `json:"symbol"`
	Constituents []Constituent `json:"constituents"`
	Source       string        `json:"source"`
}
