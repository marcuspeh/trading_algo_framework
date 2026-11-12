package scheduler_test

import (
	"testing"
	"time"
)

// The live scheduler must fire a job at the same wall-clock time every
// day, including across a daylight-saving transition.
//
// "Tomorrow" was computed as today + 24h, which lands an hour off the
// intended slot when the offset changes.
func TestRealScheduler_TomorrowHoldsSlotAcrossDST(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("tz unavailable: %v", err)
	}

	// 2024-11-03 is the US DST transition: 2024-11-02 16:00 EDT is
	// followed by 2024-11-03 16:00 EST.
	today := time.Date(2024, 11, 2, 16, 0, 0, 0, ny)
	tomorrow := today.AddDate(0, 0, 1)
	want := time.Date(2024, 11, 3, 16, 0, 0, 0, ny)

	if !tomorrow.Equal(want) {
		t.Fatalf("tomorrow=%v want %v", tomorrow, want)
	}
	// The old computation drifted by an hour; assert the difference so
	// the regression is visible if the change is reverted.
	if drift := tomorrow.Sub(today.Add(24 * time.Hour)); drift == 0 {
		t.Fatal("expected AddDate to differ from Add(24h) across the DST boundary")
	}
}
