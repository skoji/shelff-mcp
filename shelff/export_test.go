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

// SetReadSidecarFileForTest replaces how QueryBooksInDirectory reads sidecar
// files until the test ends. Tests that use this helper must not call
// t.Parallel, because the reader is package-global.
func SetReadSidecarFileForTest(t testing.TB, read func(path string) ([]byte, error)) {
	t.Helper()

	previous := readSidecarFile
	readSidecarFile = read
	t.Cleanup(func() { readSidecarFile = previous })
}
