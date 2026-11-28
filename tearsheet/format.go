package tearsheet

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// money formats an amount in the account's currency.
func money(v float64, cur string) string {
	sym := currencySymbol(cur)
	sign := ""
	if v < 0 {
		sign = "-"
	}
	return sign + sym + group(abs(v), 2)
}

// compactMoney formats an amount for an axis, where the digits matter
// less than the shape.
func compactMoney(v float64, cur string) string {
	sym := currencySymbol(cur)
	sign := ""
	if v < 0 {
		sign = "-"
	}
	a := abs(v)
	switch {
	case a >= 1_000_000_000:
		return sign + sym + trimZeros(strconv.FormatFloat(a/1_000_000_000, 'f', 1, 64)) + "B"
	case a >= 1_000_000:
		return sign + sym + trimZeros(strconv.FormatFloat(a/1_000_000, 'f', 1, 64)) + "M"
	case a >= 1_000:
		return sign + sym + trimZeros(strconv.FormatFloat(a/1_000, 'f', 1, 64)) + "K"
	default:
		return sign + sym + group(a, 0)
	}
}

// pct formats a fraction as a percentage.
func pct(v float64, digits int) string {
	return strconv.FormatFloat(v*100, 'f', digits, 64) + "%"
}

// signedPct formats a fraction as a percentage, with its sign.
func signedPct(v float64, digits int) string {
	if v == 0 {
		return strconv.FormatFloat(0, 'f', digits, 64) + "%"
	}
	s := strconv.FormatFloat(v*100, 'f', digits, 64)
	if v > 0 {
		return "+" + s + "%"
	}
	return s + "%"
}

// ratio2 formats a ratio, with a dash when there is nothing to show.
func ratio2(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "—"
	}
	return strconv.FormatFloat(v, 'f', 2, 64)
}

// trimNum formats a number without noise: no trailing zeros, no
// exponent.
func trimNum(v float64) string {
	return trimZeros(strconv.FormatFloat(v, 'f', -1, 64))
}

// trimZeros drops a trailing ".0" and a trailing ".".
func trimZeros(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// group formats a number with thousands separators.
func group(v float64, digits int) string {
	s := strconv.FormatFloat(v, 'f', digits, 64)
	dot := strings.IndexByte(s, '.')
	intPart, frac := s, ""
	if dot >= 0 {
		intPart, frac = s[:dot], s[dot:]
	}

	neg := strings.HasPrefix(intPart, "-")
	intPart = strings.TrimPrefix(intPart, "-")

	var b strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	out := b.String()
	if neg {
		out = "-" + out
	}
	return out + frac
}

// abs is the absolute value.
func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// currencySymbol is the sign a currency is written with.
func currencySymbol(cur string) string {
	switch strings.ToUpper(cur) {
	case "USD", "$":
		return "$"
	case "EUR", "€":
		return "€"
	case "GBP", "£":
		return "£"
	case "JPY", "¥":
		return "¥"
	case "":
		return "$"
	default:
		return cur + " "
	}
}

// humanDuration writes a duration in the largest unit that fits.
func humanDuration(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
