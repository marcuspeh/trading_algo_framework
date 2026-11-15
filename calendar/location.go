package calendar

import (
	"time"

	// Embed the IANA database so the NYSE location resolves even on
	// hosts without a system zoneinfo directory (scratch containers,
	// minimal CI images).
	_ "time/tzdata"
)

// NYSE is the location every session time is expressed in.
var NYSE = mustLoadNYSE()

func mustLoadNYSE() *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		// time/tzdata is embedded above, so this is unreachable.
		panic("calendar: America/New_York unavailable: " + err.Error())
	}
	return loc
}
