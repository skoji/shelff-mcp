package shelff_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/skoji/shelff-mcp/shelff"
)

func writeArchiveTestBook(t *testing.T, sidecarBody string) string {
	t.Helper()

	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "book.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-"), 0o644); err != nil {
		t.Fatal(err)
	}
	if sidecarBody != "" {
		if err := os.WriteFile(shelff.SidecarPath(pdfPath), []byte(sidecarBody), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return pdfPath
}

const archivedSidecarBody = `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "Book"},
  "archive": {"isArchived": true, "changedAt": "2026-10-27T01:02:03Z"}
}`

func assertArchive(t *testing.T, got *shelff.ArchiveState, wantArchived bool, wantChangedAt time.Time) {
	t.Helper()

	if got == nil {
		t.Fatalf("Archive = nil, want isArchived=%v changedAt=%s", wantArchived, wantChangedAt)
	}
	if got.IsArchived != wantArchived || !got.ChangedAt.Equal(wantChangedAt) {
		t.Fatalf("Archive = {isArchived:%v changedAt:%s}, want {isArchived:%v changedAt:%s}",
			got.IsArchived, got.ChangedAt, wantArchived, wantChangedAt)
	}
}

func TestWriteMetadataPreservesArchiveWhenUntouched(t *testing.T) {
	t.Parallel()

	pdfPath := writeArchiveTestBook(t, archivedSidecarBody)

	meta, err := shelff.WriteMetadata(pdfPath, map[string]any{
		"tags": []any{"golang"},
	})
	if err != nil {
		t.Fatalf("WriteMetadata error = %v", err)
	}
	assertArchive(t, meta.Archive, true, time.Date(2026, 10, 27, 1, 2, 3, 0, time.UTC))
}
