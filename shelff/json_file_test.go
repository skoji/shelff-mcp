package shelff

import (
	"errors"
	"testing"
)

// These white-box tests intentionally use package shelff so they can exercise
// unexported low-level helpers directly without widening the production API.
func TestWriteAllHandlesShortWrites(t *testing.T) {
	t.Parallel()

	writer := &shortWriter{maxPerWrite: 2}
	if err := writeAll(writer, []byte("abcdef")); err != nil {
		t.Fatalf("writeAll returned error: %v", err)
	}
	if string(writer.data) != "abcdef" {
		t.Fatalf("written data = %q, want %q", string(writer.data), "abcdef")
	}
}

func TestWriteAllPropagatesWriterErrors(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom")
	writer := &shortWriter{errAfter: 1, err: wantErr, maxPerWrite: 2}
	if err := writeAll(writer, []byte("abcdef")); !errors.Is(err, wantErr) {
		t.Fatalf("writeAll error = %v, want %v", err, wantErr)
	}
}

func TestJSONBytesToMapRejectsTrailingJSON(t *testing.T) {
	t.Parallel()

	_, err := jsonBytesToMap([]byte(`{"a":1}{"b":2}`))
	if err == nil {
		t.Fatal("jsonBytesToMap error = nil, want trailing JSON error")
	}
}

type shortWriter struct {
	data        []byte
	maxPerWrite int
	errAfter    int
	err         error
	writes      int
}

func (w *shortWriter) Write(p []byte) (int, error) {
	if w.errAfter > 0 && w.writes >= w.errAfter {
		return 0, w.err
	}
	w.writes++

	n := len(p)
	if w.maxPerWrite > 0 && n > w.maxPerWrite {
		n = w.maxPerWrite
	}
	w.data = append(w.data, p[:n]...)
	return n, nil
}
