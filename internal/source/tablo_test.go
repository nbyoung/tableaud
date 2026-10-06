package source_test

import "testing"

// TestTabloAdapter is T25: the adapter's run of the 200 and 404 cases of the
// server's table against the built weather-station entry of the corpus. It
// waits for a tablo release with the views, which brings internal/source/tablo.go
// and the require line of go.mod.
func TestTabloAdapter(t *testing.T) {
	t.Skip("tablo has no release with views yet; the adapter and this test land with step 5 of design 49ce")
}
