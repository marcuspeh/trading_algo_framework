package indicator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os/exec"
	"testing"
)

type Prices struct {
	Open   []float64 `json:"open"`
	High   []float64 `json:"high"`
	Low    []float64 `json:"low"`
	Close  []float64 `json:"close"`
	Volume []float64 `json:"volume"`
}

type ValidationRequest struct {
	Name   string                 `json:"name"`
	Prices Prices                 `json:"prices"`
	Params map[string]interface{} `json:"params"`
}

func getTalibReference(name string, bars []*Bar, params map[string]interface{}) (interface{}, error) {
	prices := Prices{
		Open:   make([]float64, len(bars)),
		High:   make([]float64, len(bars)),
		Low:    make([]float64, len(bars)),
		Close:  make([]float64, len(bars)),
		Volume: make([]float64, len(bars)),
	}
	for i, b := range bars {
		prices.Open[i] = b.Open
		prices.High[i] = b.High
		prices.Low[i] = b.Low
		prices.Close[i] = b.Close
		prices.Volume[i] = b.Volume
	}

	req := ValidationRequest{
		Name:   name,
		Prices: prices,
		Params: params,
	}

	input, _ := json.Marshal(req)
	cmd := exec.Command("python3", "talib_reference.py")
	cmd.Stdin = bytes.NewReader(input)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("error running python: %v, stderr: %s", err, stderr.String())
	}

	var result interface{}
	err = json.Unmarshal(out.Bytes(), &result)
	return result, err
}

func generateRandomBars(n int) []*Bar {
	bars := make([]*Bar, n)
	price := 100.0
	for i := 0; i < n; i++ {
		price += (rand.Float64() - 0.5) * 2
		bars[i] = &Bar{
			Open:   price,
			High:   price + rand.Float64(),
			Low:    price - rand.Float64(),
			Close:  price + (rand.Float64() - 0.5),
			Volume: rand.Float64() * 1000,
		}
	}
	return bars
}

// requireTalib skips the test when the Python TA-Lib reference is not
// available on this machine. The reference implementation needs python3
// with the pandas, numpy and ta-lib packages installed.
func requireTalib(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("skipping: python3 is not installed")
	}
	cmd := exec.Command("python3", "-c", "import talib, pandas, numpy")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("skipping: python TA-Lib reference unavailable: %v (%s)", err, out)
	}
}

func TestIndicators(t *testing.T) {
	numIterations := 5
	n := 200
	tolerance := 1e-6

	requireTalib(t)

	for iteration := 0; iteration < numIterations; iteration++ {
		t.Run(fmt.Sprintf("Iteration_%d", iteration), func(t *testing.T) {
			bars := generateRandomBars(n)

			t.Run("SMA", func(t *testing.T) {
				period := 14
				sma := NewSMA(period)
				ref, err := getTalibReference("SMA", bars, map[string]interface{}{"period": period})
				if err != nil {
					t.Fatalf("Failed to get SMA reference: %v", err)
				}
				refList, ok := ref.([]interface{})
				if !ok {
					t.Fatalf("SMA reference is not a list: %T", ref)
				}

				for i := range bars {
					b := bars[i]
					val := sma.Update(b)
					if i >= period-1 {
						expected := refList[i].(float64)
						if math.Abs(val-expected) > tolerance {
							t.Errorf("SMA mismatch at %d: got %f, want %f", i, val, expected)
						}
					}
				}
			})

			t.Run("EMA", func(t *testing.T) {
				period := 14
				ema := NewEMA(period)
				ref, err := getTalibReference("EMA", bars, map[string]interface{}{"period": period})
				if err != nil {
					t.Fatalf("Failed to get EMA reference: %v", err)
				}
				refList, ok := ref.([]interface{})
				if !ok {
					t.Fatalf("EMA reference is not a list: %T", ref)
				}

				for i := range bars {
					b := bars[i]
					val := ema.Update(b)
					if i >= period-1 {
						expected := refList[i].(float64)
						if math.Abs(val-expected) > tolerance {
							if math.Abs(val-expected) > 1e-2 {
								t.Errorf("EMA mismatch at %d: got %f, want %f", i, val, expected)
							}
						}
					}
				}
			})

			t.Run("ROC", func(t *testing.T) {
				period := 10
				roc := NewROC(period)
				ref, err := getTalibReference("ROC", bars, map[string]interface{}{"period": period})
				if err != nil {
					t.Fatalf("Failed to get ROC reference: %v", err)
				}
				refList, ok := ref.([]interface{})
				if !ok {
					t.Fatalf("ROC reference is not a list: %T", ref)
				}

				for i := range bars {
					b := bars[i]
					val := roc.Update(b)
					if i >= period {
						expected := refList[i].(float64)
						if math.Abs(val-expected) > tolerance {
							t.Errorf("ROC mismatch at %d: got %f, want %f", i, val, expected)
						}
					}
				}
			})

			t.Run("RSI", func(t *testing.T) {
				period := 14
				rsi := NewRSI(period)
				ref, err := getTalibReference("RSI", bars, map[string]interface{}{"period": period})
				if err != nil {
					t.Fatalf("Failed to get RSI reference: %v", err)
				}
				refList, ok := ref.([]interface{})
				if !ok {
					t.Fatalf("RSI reference is not a list: %T", ref)
				}

				for i := range bars {
					b := bars[i]
					val := rsi.Update(b)
					if i >= period {
						expected := refList[i].(float64)
						if math.Abs(val-expected) > 0.05 {
							t.Errorf("RSI mismatch at %d: got %f, want %f", i, val, expected)
						}
					}
				}
			})

			t.Run("ATR", func(t *testing.T) {
				period := 14
				atr := NewATR(period)
				ref, err := getTalibReference("ATR", bars, map[string]interface{}{"period": period})
				if err != nil {
					t.Fatalf("Failed to get ATR reference: %v", err)
				}
				refList, ok := ref.([]interface{})
				if !ok {
					t.Fatalf("ATR reference is not a list: %T", ref)
				}

				for i := range bars {
					b := bars[i]
					val := atr.Update(b)
					if i >= period {
						expected := refList[i].(float64)
						if math.Abs(val-expected) > 0.05 {
							t.Errorf("ATR mismatch at %d: got %f, want %f", i, val, expected)
						}
					}
				}
			})

			t.Run("MACD", func(t *testing.T) {
				fast, slow, signal := 12, 26, 9
				macd := NewMACD(fast, slow, signal)
				ref, err := getTalibReference("MACD", bars, map[string]interface{}{"fast": fast, "slow": slow, "signal": signal})
				if err != nil {
					t.Fatalf("Failed to get MACD reference: %v", err)
				}
				refMap, ok := ref.(map[string]interface{})
				if !ok {
					t.Fatalf("MACD reference is not a map: %T", ref)
				}

				refMACD := refMap["macd"].([]interface{})
				refSignal := refMap["signal"].([]interface{})
				refHist := refMap["hist"].([]interface{})

				for i := range bars {
					b := bars[i]
					val := macd.Update(b)
					if i >= slow+signal+10 {
						expectedMACD := refMACD[i].(float64)
						expectedSignal := refSignal[i].(float64)
						expectedHist := refHist[i].(float64)

						if math.Abs(val-expectedMACD) > 0.2 {
							t.Errorf("MACD line mismatch at %d: got %f, want %f", i, val, expectedMACD)
						}
						if math.Abs(macd.Signal()-expectedSignal) > 0.2 {
							t.Errorf("MACD signal mismatch at %d: got %f, want %f", i, macd.Signal(), expectedSignal)
						}
						if math.Abs(macd.Histogram()-expectedHist) > 0.2 {
							t.Errorf("MACD hist mismatch at %d: got %f, want %f", i, macd.Histogram(), expectedHist)
						}
					}
				}
			})

			t.Run("BollingerBands", func(t *testing.T) {
				period := 20
				std := 2.0
				bb := NewBollingerBands(period, std)
				ref, err := getTalibReference("BOL", bars, map[string]interface{}{"period": period, "std": std})
				if err != nil {
					t.Fatalf("Failed to get BOL reference: %v", err)
				}
				refMap, ok := ref.(map[string]interface{})
				if !ok {
					t.Fatalf("BOL reference is not a map: %T", ref)
				}

				refUpper := refMap["upper"].([]interface{})
				refMiddle := refMap["middle"].([]interface{})
				refLower := refMap["lower"].([]interface{})

				for i := range bars {
					b := bars[i]
					up, mid, low := bb.UpdateBands(b)
					if i >= period-1 {
						if math.Abs(up-refUpper[i].(float64)) > tolerance {
							t.Errorf("BOL upper mismatch at %d: got %f, want %f", i, up, refUpper[i])
						}
						if math.Abs(mid-refMiddle[i].(float64)) > tolerance {
							t.Errorf("BOL middle mismatch at %d: got %f, want %f", i, mid, refMiddle[i])
						}
						if math.Abs(low-refLower[i].(float64)) > tolerance {
							t.Errorf("BOL lower mismatch at %d: got %f, want %f", i, low, refLower[i])
						}
					}
				}
			})

			t.Run("MINMAX", func(t *testing.T) {
				period := 14
				mm := NewMinMax(period)
				refMin, _ := getTalibReference("MIN", bars, map[string]interface{}{"period": period})
				refMax, _ := getTalibReference("MAX", bars, map[string]interface{}{"period": period})

				refMinList := refMin.([]interface{})
				refMaxList := refMax.([]interface{})

				for i := range bars {
					b := bars[i]
					min, max := mm.UpdateMinMax(b)
					if i >= period-1 {
						if math.Abs(min-refMinList[i].(float64)) > tolerance {
							t.Errorf("MIN mismatch at %d: got %f, want %f", i, min, refMinList[i])
						}
						if math.Abs(max-refMaxList[i].(float64)) > tolerance {
							t.Errorf("MAX mismatch at %d: got %f, want %f", i, max, refMaxList[i])
						}
					}
				}
			})

			t.Run("OBV", func(t *testing.T) {
				obv := NewOBV()
				ref, err := getTalibReference("OBV", bars, map[string]interface{}{})
				if err != nil {
					t.Fatalf("Failed to get OBV reference: %v", err)
				}
				refList, ok := ref.([]interface{})
				if !ok {
					t.Fatalf("OBV reference is not a list: %T", ref)
				}

				for i := range bars {
					b := bars[i]
					val := obv.Update(b)
					expected := refList[i].(float64)
					if math.Abs(val-expected) > tolerance {
						t.Errorf("OBV mismatch at %d: got %f, want %f", i, val, expected)
					}
				}
			})
		})
	}
}
