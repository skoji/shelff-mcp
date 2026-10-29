package shelff_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

var archiveTestNow = time.Date(2026, 10, 29, 12, 34, 56, 789, time.UTC)

// The tests below swap the package clock and therefore do not call t.Parallel.

func TestWriteMetadataArchiveWithoutChangedAtStampsCurrentTime(t *testing.T) {
	shelff.SetTimeNowForTest(t, archiveTestNow)

	pdfPath := writeArchiveTestBook(t, "")

	meta, err := shelff.WriteMetadata(pdfPath, map[string]any{
		"archive": map[string]any{"isArchived": true},
	})
	if err != nil {
		t.Fatalf("WriteMetadata error = %v", err)
	}
	assertArchive(t, meta.Archive, true, archiveTestNow.Truncate(time.Second))

	data, err := os.ReadFile(shelff.SidecarPath(pdfPath))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"changedAt": "2026-10-29T12:34:56Z"`) {
		t.Fatalf("expected second-precision UTC changedAt on disk, got %s", data)
	}
}

func TestWriteMetadataUnarchiveKeepsObjectAndAdvancesChangedAt(t *testing.T) {
	shelff.SetTimeNowForTest(t, archiveTestNow)

	pdfPath := writeArchiveTestBook(t, archivedSidecarBody)

	meta, err := shelff.WriteMetadata(pdfPath, map[string]any{
		"archive": map[string]any{"isArchived": false},
	})
	if err != nil {
		t.Fatalf("WriteMetadata error = %v", err)
	}
	assertArchive(t, meta.Archive, false, archiveTestNow.Truncate(time.Second))
}

func TestWriteMetadataArchiveSameStateKeepsChangedAt(t *testing.T) {
	shelff.SetTimeNowForTest(t, archiveTestNow)

	pdfPath := writeArchiveTestBook(t, archivedSidecarBody)

	meta, err := shelff.WriteMetadata(pdfPath, map[string]any{
		"archive": map[string]any{"isArchived": true},
	})
	if err != nil {
		t.Fatalf("WriteMetadata error = %v", err)
	}
	assertArchive(t, meta.Archive, true, time.Date(2026, 10, 27, 1, 2, 3, 0, time.UTC))
}

func TestWriteMetadataArchiveWithNewerChangedAtReplacesExisting(t *testing.T) {
	shelff.SetTimeNowForTest(t, archiveTestNow)

	pdfPath := writeArchiveTestBook(t, archivedSidecarBody)

	meta, err := shelff.WriteMetadata(pdfPath, map[string]any{
		"archive": map[string]any{"isArchived": false, "changedAt": "2026-10-28T09:00:00+09:00"},
	})
	if err != nil {
		t.Fatalf("WriteMetadata error = %v", err)
	}
	assertArchive(t, meta.Archive, false, time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
}

func TestWriteMetadataArchiveWithOlderChangedAtKeepsExisting(t *testing.T) {
	shelff.SetTimeNowForTest(t, archiveTestNow)

	pdfPath := writeArchiveTestBook(t, archivedSidecarBody)

	meta, err := shelff.WriteMetadata(pdfPath, map[string]any{
		"archive": map[string]any{"isArchived": false, "changedAt": "2026-10-26T00:00:00Z"},
	})
	if err != nil {
		t.Fatalf("WriteMetadata error = %v", err)
	}
	assertArchive(t, meta.Archive, true, time.Date(2026, 10, 27, 1, 2, 3, 0, time.UTC))
}

func TestWriteMetadataArchiveWithChangedAtAndNoExistingIsAdopted(t *testing.T) {
	shelff.SetTimeNowForTest(t, archiveTestNow)

	pdfPath := writeArchiveTestBook(t, "")

	meta, err := shelff.WriteMetadata(pdfPath, map[string]any{
		"archive": map[string]any{"isArchived": false, "changedAt": "2020-01-01T00:00:00Z"},
	})
	if err != nil {
		t.Fatalf("WriteMetadata error = %v", err)
	}
	assertArchive(t, meta.Archive, false, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
}

func TestWriteMetadataArchiveNullRemovesObject(t *testing.T) {
	shelff.SetTimeNowForTest(t, archiveTestNow)

	pdfPath := writeArchiveTestBook(t, archivedSidecarBody)

	meta, err := shelff.WriteMetadata(pdfPath, map[string]any{"archive": nil})
	if err != nil {
		t.Fatalf("WriteMetadata error = %v", err)
	}
	if meta.Archive != nil {
		t.Fatalf("Archive = %+v, want nil after null patch", *meta.Archive)
	}
}

func TestWriteMetadataRejectsInvalidArchive(t *testing.T) {
	shelff.SetTimeNowForTest(t, archiveTestNow)

	tests := []struct {
		name    string
		archive any
	}{
		{name: "not an object", archive: true},
		{name: "missing isArchived", archive: map[string]any{"changedAt": "2026-10-28T00:00:00Z"}},
		{name: "non-boolean isArchived", archive: map[string]any{"isArchived": "yes"}},
		{name: "invalid changedAt", archive: map[string]any{"isArchived": true, "changedAt": "yesterday"}},
		{name: "non-string changedAt", archive: map[string]any{"isArchived": true, "changedAt": 12345}},
		{name: "unknown property", archive: map[string]any{"isArchived": true, "archivedAt": "2026-10-28T00:00:00Z"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdfPath := writeArchiveTestBook(t, archivedSidecarBody)
			before, err := os.ReadFile(shelff.SidecarPath(pdfPath))
			if err != nil {
				t.Fatal(err)
			}

			_, err = shelff.WriteMetadata(pdfPath, map[string]any{"archive": tt.archive})
			if !errors.Is(err, shelff.ErrInvalidFieldValue) {
				t.Fatalf("WriteMetadata error = %v, want ErrInvalidFieldValue", err)
			}

			after, err := os.ReadFile(shelff.SidecarPath(pdfPath))
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatalf("sidecar changed on rejected write:\nbefore %s\nafter  %s", before, after)
			}
		})
	}
}

func TestStatsCountsArchivedBooks(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)

	archivedPDF := writeTestPDF(t, root, "archived.pdf")
	writeRawJSONFile(t, shelff.SidecarPath(archivedPDF), `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "archived"},
  "reading": {"lastReadPage": 1, "lastReadAt": "2026-10-01T00:00:00Z", "totalPages": 10, "status": "reading"},
  "archive": {"isArchived": true, "changedAt": "2026-10-27T01:02:03Z"}
}`)
	unarchivedPDF := writeTestPDF(t, root, "unarchived.pdf")
	writeRawJSONFile(t, shelff.SidecarPath(unarchivedPDF), `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "unarchived"},
  "archive": {"isArchived": false, "changedAt": "2026-10-28T00:00:00Z"}
}`)
	writeTagsSidecar(t, root, "plain.pdf", []string{"golang"})
	writeTestPDF(t, root, "nosidecar.pdf")

	stats, err := library.Stats()
	if err != nil {
		t.Fatalf("Stats returned error: %v", err)
	}
	if stats.Archived != 1 {
		t.Fatalf("Archived = %d, want 1", stats.Archived)
	}
	if stats.StatusCounts[string(shelff.StatusReading)] != 1 {
		t.Fatalf("StatusCounts = %#v, want archived book still counted as reading", stats.StatusCounts)
	}
}

func TestCheckLibraryReportsInvalidArchives(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	nested := filepath.Join(root, "nested")
	mkdirAll(t, nested)

	validPDF := writeTestPDF(t, root, "valid.pdf")
	writeRawJSONFile(t, shelff.SidecarPath(validPDF), `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "valid"},
  "category": "Fiction",
  "archive": {"isArchived": true, "changedAt": "2026-10-27T01:02:03Z"}
}`)
	// Unparseable changedAt makes the typed read fail; check must still go on.
	badTimePDF := writeTestPDF(t, nested, "badtime.pdf")
	writeRawJSONFile(t, shelff.SidecarPath(badTimePDF), `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "badtime"},
  "category": "Skipped",
  "archive": {"isArchived": true, "changedAt": "yesterday"}
}`)
	// Structurally invalid but still readable: the rest of the sidecar counts.
	missingPDF := writeTestPDF(t, root, "missing.pdf")
	writeRawJSONFile(t, shelff.SidecarPath(missingPDF), `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "missing"},
  "tags": ["kept"],
  "archive": {"isArchived": true, "archivedAt": "2026-10-27T01:02:03Z"}
}`)
	// Problems outside archive are not archive issues.
	otherPDF := writeTestPDF(t, root, "other.pdf")
	writeRawJSONFile(t, shelff.SidecarPath(otherPDF), `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "other"},
  "reading": {"lastReadPage": 0, "lastReadAt": "2026-10-01T00:00:00Z", "totalPages": 10, "status": "bogus"}
}`)

	result, err := library.CheckLibrary()
	if err != nil {
		t.Fatalf("CheckLibrary returned error: %v", err)
	}

	if len(result.InvalidArchives) != 2 {
		t.Fatalf("InvalidArchives = %+v, want 2 entries", result.InvalidArchives)
	}
	missing := result.InvalidArchives[0]
	if missing.SidecarPath != "missing.pdf.meta.json" {
		t.Fatalf("InvalidArchives[0].SidecarPath = %q, want missing.pdf.meta.json", missing.SidecarPath)
	}
	if joined := strings.Join(missing.Errors, "\n"); !strings.Contains(joined, "changedAt") || !strings.Contains(joined, "archivedAt") {
		t.Fatalf("InvalidArchives[0].Errors = %q, want missing changedAt and unknown archivedAt", missing.Errors)
	}
	badTime := result.InvalidArchives[1]
	if badTime.SidecarPath != filepath.Join("nested", "badtime.pdf.meta.json") {
		t.Fatalf("InvalidArchives[1].SidecarPath = %q, want nested/badtime.pdf.meta.json", badTime.SidecarPath)
	}
	if joined := strings.Join(badTime.Errors, "\n"); !strings.Contains(joined, "$.archive.changedAt") {
		t.Fatalf("InvalidArchives[1].Errors = %q, want $.archive.changedAt error", badTime.Errors)
	}
	for _, issue := range result.InvalidArchives {
		for _, msg := range issue.Errors {
			if !strings.HasPrefix(msg, "$.archive") {
				t.Fatalf("InvalidArchives contains non-archive error %q", msg)
			}
		}
	}

	if !slices.Equal(result.Integrity.UndefinedCategories, []string{"Fiction"}) {
		t.Fatalf("UndefinedCategories = %v, want [Fiction] with the unreadable sidecar skipped", result.Integrity.UndefinedCategories)
	}
	if !slices.Equal(result.Integrity.UndefinedTags, []string{"kept"}) {
		t.Fatalf("UndefinedTags = %v, want [kept]", result.Integrity.UndefinedTags)
	}
	if result.Summary.TotalPDFs != 4 || result.Summary.WithSidecar != 4 || result.Summary.WithoutSidecar != 0 {
		t.Fatalf("Summary = %+v, want 4/4/0", result.Summary)
	}
}

func TestCheckLibraryReportsNoInvalidArchivesAsEmptyList(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	writeTagsSidecar(t, root, "plain.pdf", []string{"golang"})

	result, err := library.CheckLibrary()
	if err != nil {
		t.Fatalf("CheckLibrary returned error: %v", err)
	}
	if result.InvalidArchives == nil || len(result.InvalidArchives) != 0 {
		t.Fatalf("InvalidArchives = %#v, want empty non-nil slice", result.InvalidArchives)
	}
}
