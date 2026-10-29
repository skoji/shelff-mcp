package shelff

import (
	"testing"
	"time"
)

// SetTimeNowForTest replaces the package clock until the test ends.
// Tests that use this helper must not call t.Parallel, because the clock is
// package-global.
func SetTimeNowForTest(t testing.TB, now time.Time) {
	t.Helper()

	previous := timeNow
	timeNow = func() time.Time { return now }
	t.Cleanup(func() { timeNow = previous })
}
