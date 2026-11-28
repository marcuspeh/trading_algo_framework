package fills_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/marcuspeh/trading-framework/fills"
	"github.com/marcuspeh/trading-framework/order"
)

// capture keeps the lines the receiver logged.
type capture struct {
	mu    sync.Mutex
	lines []string
}

func (c *capture) Info(msg string, fields ...any)  { c.add("INFO", msg, fields) }
func (c *capture) Warn(msg string, fields ...any)  { c.add("WARN", msg, fields) }
func (c *capture) Error(msg string, fields ...any) { c.add("ERROR", msg, fields) }

func (c *capture) add(level, msg string, fields []any) {
	var b strings.Builder
	b.WriteString(level)
	b.WriteString(" ")
	b.WriteString(msg)
	for i := 0; i+1 < len(fields); i += 2 {
		fmt.Fprintf(&b, " %v=%v", fields[i], fields[i+1])
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lines = append(c.lines, b.String())
}

func (c *capture) all() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.lines...)
}

// post sends a fill report to the receiver.
func post(t *testing.T, url, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// A fill for an order the framework placed is logged against that
// order: the symbol and the side come from the submission.
func TestHandler_LogsAFillAgainstItsOrder(t *testing.T) {
	reg := fills.NewRegistry()
	reg.Record(4242, order.Order{ID: "abc", Symbol: "AAPL", Side: order.Buy, Quantity: 10})

	log := &capture{}
	srv := httptest.NewServer(reg.Handler(log))
	defer srv.Close()

	resp := post(t, srv.URL, `{"permId":4242,"execId":"0001","fillPrice":150.25,"fillQty":10,"commission":1.5}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", resp.StatusCode)
	}

	lines := log.all()
	if len(lines) != 1 {
		t.Fatalf("log lines=%d want 1: %v", len(lines), lines)
	}
	for _, want := range []string{
		"INFO order filled",
		"id=4242",
		"exec_id=0001",
		"symbol=AAPL",
		"side=BUY",
		"qty=10",
		"price=150.25",
		"commission=1.5",
	} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("fill log = %q want it to contain %q", lines[0], want)
		}
	}
}

// A bracket's exit leg is reported by the reference its entry carried,
// not by an id the framework knows. The leg closes the trade, so it
// trades the opposite side.
func TestHandler_LogsABracketExitLegAsTheOppositeSide(t *testing.T) {
	reg := fills.NewRegistry()
	reg.Record(111, order.Order{ID: "bracket-ref", Symbol: "AAPL", Side: order.Buy, Quantity: 10})

	log := &capture{}
	srv := httptest.NewServer(reg.Handler(log))
	defer srv.Close()

	resp := post(t, srv.URL, `{"permId":999,"orderRef":"bracket-ref","fillPrice":120,"fillQty":10}`)
	defer resp.Body.Close()

	lines := log.all()
	if len(lines) != 1 {
		t.Fatalf("log lines=%d want 1: %v", len(lines), lines)
	}
	for _, want := range []string{"INFO order filled", "symbol=AAPL", "side=SELL", "price=120"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("fill log = %q want it to contain %q", lines[0], want)
		}
	}
}

// A fill for an order the framework did not place is worth a warning:
// the price is known, but not what it was.
func TestHandler_WarnsAboutAFillForAnUnknownOrder(t *testing.T) {
	log := &capture{}
	srv := httptest.NewServer(fills.NewRegistry().Handler(log))
	defer srv.Close()

	resp := post(t, srv.URL, `{"permId":777,"execId":"0009","fillPrice":10,"fillQty":5}`)
	defer resp.Body.Close()

	lines := log.all()
	if len(lines) != 1 {
		t.Fatalf("log lines=%d want 1: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "WARN") ||
		!strings.Contains(lines[0], "fill for an order the framework did not place") {
		t.Errorf("fill log = %q want a warning about an unknown order", lines[0])
	}
	if !strings.Contains(lines[0], "price=10") {
		t.Errorf("fill log = %q want it to carry the price", lines[0])
	}
}

// A payload that is not a fill report is rejected rather than logged
// as one.
func TestHandler_RejectsAMalformedPayload(t *testing.T) {
	log := &capture{}
	srv := httptest.NewServer(fills.NewRegistry().Handler(log))
	defer srv.Close()

	resp := post(t, srv.URL, `{"permId":`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", resp.StatusCode)
	}
	for _, line := range log.all() {
		if strings.Contains(line, "order filled") {
			t.Errorf("a malformed payload was logged as a fill: %q", line)
		}
	}
}

// The server takes the fill reports the broker posts.
func TestServer_ReceivesAFill(t *testing.T) {
	reg := fills.NewRegistry()
	reg.Record(77, order.Order{Symbol: "MSFT", Side: order.Sell, Quantity: 3})

	log := &capture{}
	srv := fills.NewServer("127.0.0.1:0", reg, log)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := srv.Stop(context.Background()); err != nil {
			t.Error(err)
		}
	}()

	// The proxy posts to {WEBHOOK_URL}/fills, and its default webhook
	// URL puts the fill webhook at /webhook/fills.
	url := "http://" + srv.Addr() + "/webhook/fills"
	resp := post(t, url, `{"permId":77,"execId":"0002","fillPrice":300,"fillQty":3}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", resp.StatusCode)
	}

	lines := log.all()
	if len(lines) != 1 {
		t.Fatalf("log lines=%d want 1: %v", len(lines), lines)
	}
	for _, want := range []string{"order filled", "symbol=MSFT", "side=SELL", "price=300"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("fill log = %q want it to contain %q", lines[0], want)
		}
	}
}
