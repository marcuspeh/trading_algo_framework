package calendar_test

import (
	"errors"
	"testing"
	"time"

	"github.com/marcuspeh/trading-framework/calendar"
)

// failingProvider is a provider that cannot answer.
type failingProvider struct{}

func (failingProvider) TradingDay(time.Time) (bool, error) {
	return false, errors.New("provider is down")
}

func (failingProvider) EarlyClose(time.Time) (bool, error) {
	return false, errors.New("provider is down")
}

func (failingProvider) SessionOpen(time.Time) (time.Time, error) {
	return time.Time{}, errors.New("provider is down")
}

func (failingProvider) SessionClose(time.Time) (time.Time, error) {
	return time.Time{}, errors.New("provider is down")
}

// A provider failure inside the error-free predicate the framework
// uses is fatal, and it says so.
func TestNewTradingDayFunc_ProviderFailureIsFatal(t *testing.T) {
	fn := calendar.NewTradingDayFunc(failingProvider{})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected a panic when the provider fails")
		}
		var perr *calendar.ProviderError
		if !errors.As(asError(r), &perr) {
			t.Fatalf("panic value = %v, want a *calendar.ProviderError", r)
		}
	}()
	fn(time.Date(2024, time.January, 3, 0, 0, 0, 0, time.UTC))
}

func TestNewCloseFunc_ProviderFailureIsFatal(t *testing.T) {
	fn := calendar.NewCloseFunc(failingProvider{})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected a panic when the provider fails")
		}
		var perr *calendar.ProviderError
		if !errors.As(asError(r), &perr) {
			t.Fatalf("panic value = %v, want a *calendar.ProviderError", r)
		}
	}()
	fn(time.Date(2024, time.January, 3, 0, 0, 0, 0, time.UTC))
}

// A close override replaces the provider's answer and leaves the rest
// of it alone.
func TestWithCloseFunc_OverridesTheClose(t *testing.T) {
	base := &stubProvider{}
	overridden := calendar.WithCloseFunc(base, func(d time.Time) time.Time {
		return time.Date(d.Year(), d.Month(), d.Day(), 12, 0, 0, 0, time.UTC)
	})

	close, err := overridden.SessionClose(time.Date(2024, time.January, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("SessionClose: %v", err)
	}
	if close.Hour() != 12 {
		t.Fatalf("close=%v want the override's 12:00", close)
	}

	trading, err := overridden.TradingDay(time.Date(2024, time.January, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("TradingDay: %v", err)
	}
	if !trading {
		t.Fatal("the override should not change the trading-day answer")
	}
}

// asError turns a recovered panic value into an error so errors.As can
// inspect it.
func asError(r any) error {
	if err, ok := r.(error); ok {
		return err
	}
	return errors.New("panic value is not an error")
}

// A provider with no session for a date says so with ErrNoSession,
// which is distinct from a failure.
func TestErrNoSession_DistinguishesAMissingSessionFromAFailure(t *testing.T) {
	_, err := noSessionProvider{}.SessionClose(
		time.Date(2024, time.January, 6, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected an error for a day with no session")
	}
	if !errors.Is(err, calendar.ErrNoSession) {
		t.Fatalf("error = %v want it to wrap ErrNoSession", err)
	}

	boom := errors.New("provider is down")
	_, err = failingProviderWith{err: boom}.SessionClose(
		time.Date(2024, time.January, 3, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected an error when the provider fails")
	}
	if errors.Is(err, calendar.ErrNoSession) {
		t.Fatal("a provider failure must not look like a missing session")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v want it to wrap %v", err, boom)
	}
}

// failingProviderWith fails with a specific error.
type failingProviderWith struct {
	err error
}

// TradingDay fails.
func (f failingProviderWith) TradingDay(time.Time) (bool, error) { return false, f.err }

// EarlyClose fails.
func (f failingProviderWith) EarlyClose(time.Time) (bool, error) { return false, f.err }

// SessionOpen fails.
func (f failingProviderWith) SessionOpen(time.Time) (time.Time, error) { return time.Time{}, f.err }

// SessionClose fails.
func (f failingProviderWith) SessionClose(time.Time) (time.Time, error) { return time.Time{}, f.err }

// The standard session stands in on the days a provider has no session
// for, so a strategy that trades on its own days can still resolve a
// close-relative slot.
func TestWithSessionFallback_AnswersTheStandardSession(t *testing.T) {
	base := &noSessionProvider{}
	p := calendar.WithSessionFallback(base)

	d := time.Date(2024, time.January, 6, 0, 0, 0, 0, time.UTC)

	open, err := p.SessionOpen(d)
	if err != nil {
		t.Fatalf("SessionOpen: %v", err)
	}
	if open.Hour() != 9 || open.Minute() != 30 {
		t.Errorf("open = %v want 09:30", open)
	}

	close, err := p.SessionClose(d)
	if err != nil {
		t.Fatalf("SessionClose: %v", err)
	}
	if close.Hour() != 16 || close.Minute() != 0 {
		t.Errorf("close = %v want 16:00", close)
	}
}

// A provider failure is still fatal: the fallback only covers a date
// the provider had no session for.
func TestWithSessionFallback_DoesNotHideAFailure(t *testing.T) {
	p := calendar.WithSessionFallback(failingProvider{})

	if _, err := p.SessionClose(time.Date(2024, time.January, 3, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("expected the provider's error to come through")
	}
}

// noSessionProvider has no session on any date.
type noSessionProvider struct{}

// TradingDay reports every day as a trading day.
func (noSessionProvider) TradingDay(time.Time) (bool, error) { return true, nil }

// EarlyClose reports no early close.
func (noSessionProvider) EarlyClose(time.Time) (bool, error) { return false, nil }

// SessionOpen reports no session.
func (noSessionProvider) SessionOpen(time.Time) (time.Time, error) {
	return time.Time{}, calendar.ErrNoSession
}

// SessionClose reports no session.
func (noSessionProvider) SessionClose(time.Time) (time.Time, error) {
	return time.Time{}, calendar.ErrNoSession
}
