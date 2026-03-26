package shelff_test

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/skoji/shelff-go/shelff"
)

func TestScanBooksRecursiveSkipsConfigDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	topPDF := writeTestPDF(t, root, "top.pdf")
	nestedDir := filepath.Join(root, "nested")
	configDir := filepath.Join(root, shelff.ConfigDir)
	mkdirAll(t, nestedDir, configDir)
	nestedPDF := writeTestPDF(t, nestedDir, "nested.PDF")
	writeTestPDF(t, configDir, "ignored.pdf")
	if _, err := shelff.CreateSidecar(nestedPDF); err != nil {
		t.Fatalf("CreateSidecar returned error: %v", err)
	}

	books, err := library.ScanBooks(true)
	if err != nil {
		t.Fatalf("ScanBooks returned error: %v", err)
	}

	if len(books) != 2 {
		t.Fatalf("len(books) = %d, want 2", len(books))
	}
	if books[0].PDFPath != nestedPDF && books[0].PDFPath != topPDF {
		t.Fatalf("unexpected PDFPath %q", books[0].PDFPath)
	}
	if got := findBook(t, books, nestedPDF); !got.HasSidecar || got.SidecarPath == nil || *got.SidecarPath != shelff.SidecarPath(nestedPDF) {
		t.Fatalf("nested book = %#v, want sidecar present", got)
	}
	if got := findBook(t, books, topPDF); got.HasSidecar || got.SidecarPath != nil {
		t.Fatalf("top book = %#v, want no sidecar", got)
	}
}

func TestScanBooksNonRecursiveOnlyReturnsTopLevelPDFs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	topPDF := writeTestPDF(t, root, "top.pdf")
	mkdirAll(t, filepath.Join(root, "nested"))
	writeTestPDF(t, filepath.Join(root, "nested"), "nested.pdf")

	books, err := library.ScanBooks(false)
	if err != nil {
		t.Fatalf("ScanBooks returned error: %v", err)
	}
	if len(books) != 1 || books[0].PDFPath != topPDF {
		t.Fatalf("books = %#v, want only top-level PDF", books)
	}
}

func TestFindOrphanedSidecarsFindsMissingPDFs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	validPDF := writeTestPDF(t, root, "valid.pdf")
	if _, err := shelff.CreateSidecar(validPDF); err != nil {
		t.Fatalf("CreateSidecar returned error: %v", err)
	}
	orphanPath := shelff.SidecarPath(filepath.Join(root, "orphan.pdf"))
	writeRawJSONFile(t, orphanPath, `{"schemaVersion":1,"metadata":{"dc:title":"orphan"}}`)

	orphaned, err := library.FindOrphanedSidecars()
	if err != nil {
		t.Fatalf("FindOrphanedSidecars returned error: %v", err)
	}
	if len(orphaned) != 1 {
		t.Fatalf("len(orphaned) = %d, want 1", len(orphaned))
	}
	if orphaned[0].SidecarPath != orphanPath {
		t.Fatalf("SidecarPath = %q, want %q", orphaned[0].SidecarPath, orphanPath)
	}
}

func TestCollectAllTagsUsesConfiguredOrderThenAlphabeticalRemainder(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	writeTagsSidecar(t, root, "one.pdf", []string{"mystery", "history"})
	writeTagsSidecar(t, root, "two.pdf", []string{"sci-fi"})
	if err := library.WriteTagOrder(&shelff.TagOrder{
		Version:  shelff.SchemaVersion,
		TagOrder: []string{"history", "unused"},
	}); err != nil {
		t.Fatalf("WriteTagOrder returned error: %v", err)
	}

	tags, err := library.CollectAllTags()
	if err != nil {
		t.Fatalf("CollectAllTags returned error: %v", err)
	}
	want := []string{"history", "mystery", "sci-fi"}
	if !slices.Equal(tags, want) {
		t.Fatalf("CollectAllTags = %#v, want %#v", tags, want)
	}
}

func TestStatsCountsBooksTagsCategoriesStatusesAndOrphans(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)

	readingPDF := writeCategorySidecar(t, root, "reading.pdf", "Fiction")
	readingMeta := mustReadSidecar(t, readingPDF)
	readingMeta.Tags = []string{"fantasy", "fantasy", "history"}
	readingStatus := shelff.StatusReading
	readingMeta.Reading = &shelff.ReadingProgress{
		LastReadPage: 10,
		LastReadAt:   mustParseTime(t, "2026-03-26T09:00:00Z"),
		TotalPages:   100,
		Status:       &readingStatus,
	}
	if err := shelff.WriteSidecar(readingPDF, readingMeta); err != nil {
		t.Fatalf("WriteSidecar returned error: %v", err)
	}

	noReadingPDF := writeTagsSidecar(t, root, "noreading.pdf", []string{"history"})
	writeTestPDF(t, root, "plain.pdf")
	writeRawJSONFile(t, shelff.SidecarPath(filepath.Join(root, "orphan.pdf")), `{"schemaVersion":1,"metadata":{"dc:title":"orphan"}}`)

	stats, err := library.Stats()
	if err != nil {
		t.Fatalf("Stats returned error: %v", err)
	}
	if stats.TotalPDFs != 3 || stats.WithSidecar != 2 || stats.WithoutSidecar != 1 || stats.OrphanedSidecars != 1 {
		t.Fatalf("stats counts = %#v, want totals 3/2/1/1", stats)
	}
	if stats.CategoryCounts["Fiction"] != 1 {
		t.Fatalf("CategoryCounts = %#v, want Fiction=1", stats.CategoryCounts)
	}
	if stats.TagCounts["fantasy"] != 1 || stats.TagCounts["history"] != 2 {
		t.Fatalf("TagCounts = %#v, want fantasy=1 history=2", stats.TagCounts)
	}
	if stats.StatusCounts[shelff.StatusReading] != 1 || stats.StatusCounts[""] != 2 {
		t.Fatalf("StatusCounts = %#v, want reading=1 empty=2", stats.StatusCounts)
	}
	if _, err := library.Validate(noReadingPDF); err != nil && !errors.Is(err, shelff.ErrSidecarNotFound) {
		t.Fatalf("Validate on valid sidecar err = %v", err)
	}
}

func TestValidateReturnsErrorsForInvalidSidecar(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	pdfPath := writeTestPDF(t, root, "book.pdf")
	writeRawJSONFile(t, shelff.SidecarPath(pdfPath), `{
  "schemaVersion": 2,
  "metadata": {},
  "reading": {
    "lastReadPage": 0,
    "lastReadAt": "not-a-time",
    "totalPages": 0,
    "status": "bogus"
  }
}`)

	errs, err := library.Validate(pdfPath)
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("Validate errors = nil, want validation errors")
	}

	joined := strings.Join(errs, "\n")
	for _, needle := range []string{"const 1", "missing required property \"dc:title\"", "invalid date-time", "expected one of", ">= 1"} {
		if !strings.Contains(joined, needle) {
			t.Fatalf("validation errors %q do not contain %q", joined, needle)
		}
	}
}

func TestValidateReturnsErrSidecarNotFoundWhenMissing(t *testing.T) {
	t.Parallel()

	library := openTestLibrary(t, t.TempDir())
	_, err := library.Validate(filepath.Join(t.TempDir(), "missing.pdf"))
	if !errors.Is(err, shelff.ErrSidecarNotFound) {
		t.Fatalf("Validate error = %v, want ErrSidecarNotFound", err)
	}
}

func TestValidateRejectsTrailingJSONContent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	pdfPath := writeTestPDF(t, root, "book.pdf")
	writeRawJSONFile(t, shelff.SidecarPath(pdfPath), `{"schemaVersion":1,"metadata":{"dc:title":"ok"}}{"extra":true}`)

	errs, err := library.Validate(pdfPath)
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if len(errs) != 1 || !strings.Contains(errs[0], "invalid JSON") {
		t.Fatalf("Validate errors = %#v, want invalid JSON error", errs)
	}
}

func findBook(t *testing.T, books []shelff.BookEntry, pdfPath string) shelff.BookEntry {
	t.Helper()

	for _, book := range books {
		if book.PDFPath == pdfPath {
			return book
		}
	}
	t.Fatalf("book with PDFPath %q not found", pdfPath)
	return shelff.BookEntry{}
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("time.Parse(%q): %v", value, err)
	}
	return parsed
}
