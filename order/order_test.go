package order_test

import (
	"testing"

	"github.com/marcuspeh/trading-framework/order"
)

func TestSideString(t *testing.T) {
	if order.Buy.String() != "BUY" {
		t.Fatalf("Buy=%q", order.Buy.String())
	}
	if order.Sell.String() != "SELL" {
		t.Fatalf("Sell=%q", order.Sell.String())
	}
}

func TestOrderTypeString(t *testing.T) {
	if order.Market.String() != "MARKET" {
		t.Fatalf("Market=%q", order.Market.String())
	}
	if order.Limit.String() != "LIMIT" {
		t.Fatalf("Limit=%q", order.Limit.String())
	}
}
