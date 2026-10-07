package export

import "testing"

// TestRealBundle is T15: the bundle of the weather station through the tablo
// source and the templates' adapters holds the golden file list and passes the
// walk, the scan, the determinism and the base-path tests; this project's plan
// gives 93 files; the test logs both sizes. T14 runs again here, where tablo
// and git do the reading.
func TestRealBundle(t *testing.T) {
	t.Skip("waits at the integrate gate for tablo's Source, which tablo's release brings, and for the templates' adapters")
}

// TestSourceLine is T16: each page's context line holds the ref, the
// abbreviated commit and the date, and reads "viewer: an observer"; no page
// holds the day of the run.
func TestSourceLine(t *testing.T) {
	t.Skip("waits at the integrate gate for tablo's Source and the templates' adapters, which draw the pages of a real run")
}
