package shelff_test

import (
	"errors"
	"os"
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

func TestScanBooksInDirectoryRecursiveOnlyReturnsRequestedSubtree(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	selectedDir := filepath.Join(root, "selected")
	otherDir := filepath.Join(root, "other")
	deepDir := filepath.Join(selectedDir, "deep")
	configDir := filepath.Join(root, shelff.ConfigDir)
	mkdirAll(t, selectedDir, otherDir, deepDir, configDir)

	selectedPDF := writeTestPDF(t, selectedDir, "selected.pdf")
	deepPDF := writeTestPDF(t, deepDir, "deep.pdf")
	writeTestPDF(t, otherDir, "other.pdf")
	writeTestPDF(t, configDir, "ignored.pdf")

	books, err := library.ScanBooksInDirectory(selectedDir, true)
	if err != nil {
		t.Fatalf("ScanBooksInDirectory returned error: %v", err)
	}

	if len(books) != 2 {
		t.Fatalf("len(books) = %d, want 2", len(books))
	}
	if got := findBook(t, books, selectedPDF); got.PDFPath != selectedPDF {
		t.Fatalf("selected book = %#v, want %q", got, selectedPDF)
	}
	if got := findBook(t, books, deepPDF); got.PDFPath != deepPDF {
		t.Fatalf("deep book = %#v, want %q", got, deepPDF)
	}
}

func TestScanBooksInDirectoryAcceptsLibraryRootRelativePaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	selectedDir := filepath.Join(root, "selected")
	deepDir := filepath.Join(selectedDir, "deep")
	mkdirAll(t, selectedDir, deepDir)

	selectedPDF := writeTestPDF(t, selectedDir, "selected.pdf")
	deepPDF := writeTestPDF(t, deepDir, "deep.pdf")

	books, err := library.ScanBooksInDirectory("selected", true)
	if err != nil {
		t.Fatalf("ScanBooksInDirectory returned error: %v", err)
	}

	if len(books) != 2 {
		t.Fatalf("len(books) = %d, want 2", len(books))
	}
	if got := findBook(t, books, selectedPDF); got.PDFPath != selectedPDF {
		t.Fatalf("selected book = %#v, want %q", got, selectedPDF)
	}
	if got := findBook(t, books, deepPDF); got.PDFPath != deepPDF {
		t.Fatalf("deep book = %#v, want %q", got, deepPDF)
	}
}

func TestScanBooksInDirectoryFollowsSymlinkStartDirectoryWithinRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	selectedDir := filepath.Join(root, "selected")
	linkDir := filepath.Join(root, "selected-link")
	mkdirAll(t, selectedDir)

	selectedPDF := writeTestPDF(t, selectedDir, "selected.pdf")
	if err := os.Symlink(selectedDir, linkDir); err != nil {
		t.Skipf("os.Symlink unavailable: %v", err)
	}

	books, err := library.ScanBooksInDirectory(linkDir, true)
	if err != nil {
		t.Fatalf("ScanBooksInDirectory returned error: %v", err)
	}

	if len(books) != 1 || books[0].PDFPath != selectedPDF {
		t.Fatalf("books = %#v, want selected.pdf through symlink start directory", books)
	}
}

func TestScanBooksInDirectoryWithSymlinkRootNormalizesPathsAndSkipsConfigDir(t *testing.T) {
	t.Parallel()

	realRoot := t.TempDir()
	linkRoot := filepath.Join(t.TempDir(), "library-link")
	selectedPDF := writeTestPDF(t, realRoot, "selected.pdf")
	configDir := filepath.Join(realRoot, shelff.ConfigDir)
	mkdirAll(t, configDir)
	writeTestPDF(t, configDir, "ignored.pdf")
	orphanSidecar := shelff.SidecarPath(filepath.Join(realRoot, "orphan.pdf"))
	writeRawJSONFile(t, orphanSidecar, `{"schemaVersion":1,"metadata":{"dc:title":"orphan"}}`)

	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Skipf("os.Symlink unavailable: %v", err)
	}

	library := openTestLibrary(t, linkRoot)
	books, err := library.ScanBooks(true)
	if err != nil {
		t.Fatalf("ScanBooks returned error: %v", err)
	}

	wantPDF := filepath.Join(linkRoot, filepath.Base(selectedPDF))
	if len(books) != 1 || books[0].PDFPath != wantPDF {
		t.Fatalf("books = %#v, want only %q under symlink root", books, wantPDF)
	}

	orphaned, err := library.FindOrphanedSidecars()
	if err != nil {
		t.Fatalf("FindOrphanedSidecars returned error: %v", err)
	}
	wantSidecar := filepath.Join(linkRoot, filepath.Base(orphanSidecar))
	wantExpectedPDF := filepath.Join(linkRoot, "orphan.pdf")
	if len(orphaned) != 1 || orphaned[0].SidecarPath != wantSidecar || orphaned[0].ExpectedPDF != wantExpectedPDF {
		t.Fatalf("orphaned = %#v, want sidecar=%q expectedPDF=%q", orphaned, wantSidecar, wantExpectedPDF)
	}
}

func TestScanBooksInDirectoryNonRecursiveOnlyReturnsDirectChildren(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	selectedDir := filepath.Join(root, "selected")
	deepDir := filepath.Join(selectedDir, "deep")
	mkdirAll(t, selectedDir, deepDir)

	selectedPDF := writeTestPDF(t, selectedDir, "selected.pdf")
	writeTestPDF(t, deepDir, "deep.pdf")

	books, err := library.ScanBooksInDirectory(selectedDir, false)
	if err != nil {
		t.Fatalf("ScanBooksInDirectory returned error: %v", err)
	}

	if len(books) != 1 || books[0].PDFPath != selectedPDF {
		t.Fatalf("books = %#v, want only direct child PDF", books)
	}
}

func TestScanBooksInDirectoryRejectsOutsideRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)

	if _, err := library.ScanBooksInDirectory(t.TempDir(), true); err == nil {
		t.Fatal("ScanBooksInDirectory error = nil, want outside-root error")
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
	for _, needle := range []string{"const:", "missing properties", "invalid date-time", "enum:", "minimum:"} {
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

func TestCheckLibraryWithNoDotShelff(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)

	writeTestPDF(t, root, "a.pdf")
	writeTestPDF(t, root, "b.pdf")

	result, err := library.CheckLibrary()
	if err != nil {
		t.Fatalf("CheckLibrary returned error: %v", err)
	}

	if result.DotShelff.Exists {
		t.Fatalf("DotShelff.Exists = true, want false")
	}
	if result.DotShelff.CategoriesJSON {
		t.Fatalf("DotShelff.CategoriesJSON = true, want false")
	}
	if result.DotShelff.TagsJSON {
		t.Fatalf("DotShelff.TagsJSON = true, want false")
	}
	if len(result.Integrity.UndefinedCategories) != 0 {
		t.Fatalf("UndefinedCategories = %v, want empty", result.Integrity.UndefinedCategories)
	}
	if len(result.Integrity.UndefinedTags) != 0 {
		t.Fatalf("UndefinedTags = %v, want empty", result.Integrity.UndefinedTags)
	}
	if len(result.Integrity.UnusedCategories) != 0 {
		t.Fatalf("UnusedCategories = %v, want empty", result.Integrity.UnusedCategories)
	}
	if len(result.Integrity.UnusedTags) != 0 {
		t.Fatalf("UnusedTags = %v, want empty", result.Integrity.UnusedTags)
	}
	if len(result.OrphanedSidecars) != 0 {
		t.Fatalf("OrphanedSidecars = %v, want empty", result.OrphanedSidecars)
	}
	if result.Summary.TotalPDFs != 2 {
		t.Fatalf("TotalPDFs = %d, want 2", result.Summary.TotalPDFs)
	}
	if result.Summary.WithSidecar != 0 {
		t.Fatalf("WithSidecar = %d, want 0", result.Summary.WithSidecar)
	}
	if result.Summary.WithoutSidecar != 2 {
		t.Fatalf("WithoutSidecar = %d, want 2", result.Summary.WithoutSidecar)
	}
}

func TestCheckLibraryWithConfigAndBooks(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)

	// Create categories: 小説, 技術書 (技術書 will be unused)
	if err := library.WriteCategories(&shelff.CategoryList{
		Version: 1,
		Categories: []shelff.CategoryItem{
			{Name: "小説", Order: 0},
			{Name: "技術書", Order: 1},
		},
	}); err != nil {
		t.Fatalf("WriteCategories: %v", err)
	}

	// Create tags: Go, Rust (Rust will be unused)
	if err := library.WriteTagOrder(&shelff.TagOrder{
		Version:  1,
		TagOrder: []string{"Go", "Rust"},
	}); err != nil {
		t.Fatalf("WriteTagOrder: %v", err)
	}

	// book with sidecar: category=小説, tags=[Go, Swift]
	// Swift is undefined (not in tags.json)
	pdfPath := writeTestPDF(t, root, "book1.pdf")
	meta, err := shelff.CreateSidecar(pdfPath)
	if err != nil {
		t.Fatalf("CreateSidecar: %v", err)
	}
	cat := "小説"
	meta.Category = &cat
	meta.Tags = []string{"Go", "Swift"}
	if err := shelff.WriteSidecar(pdfPath, meta); err != nil {
		t.Fatalf("WriteSidecar: %v", err)
	}

	// book with sidecar: category=SF (undefined)
	pdfPath2 := writeTestPDF(t, root, "book2.pdf")
	meta2, err := shelff.CreateSidecar(pdfPath2)
	if err != nil {
		t.Fatalf("CreateSidecar: %v", err)
	}
	cat2 := "SF"
	meta2.Category = &cat2
	if err := shelff.WriteSidecar(pdfPath2, meta2); err != nil {
		t.Fatalf("WriteSidecar: %v", err)
	}

	// book without sidecar
	writeTestPDF(t, root, "book3.pdf")

	// orphaned sidecar
	orphanPath := filepath.Join(root, "gone.pdf.meta.json")
	writeRawJSONFile(t, orphanPath, `{"schemaVersion":1,"metadata":{"dc:title":"gone"}}`)

	result, err := library.CheckLibrary()
	if err != nil {
		t.Fatalf("CheckLibrary returned error: %v", err)
	}

	if !result.DotShelff.Exists {
		t.Fatalf("DotShelff.Exists = false, want true")
	}
	if !result.DotShelff.CategoriesJSON {
		t.Fatalf("DotShelff.CategoriesJSON = false, want true")
	}
	if !result.DotShelff.TagsJSON {
		t.Fatalf("DotShelff.TagsJSON = false, want true")
	}

	// undefinedCategories: SF (used in sidecar but not in categories.json)
	if !slices.Equal(result.Integrity.UndefinedCategories, []string{"SF"}) {
		t.Fatalf("UndefinedCategories = %v, want [SF]", result.Integrity.UndefinedCategories)
	}
	// undefinedTags: Swift (used in sidecar but not in tags.json)
	if !slices.Equal(result.Integrity.UndefinedTags, []string{"Swift"}) {
		t.Fatalf("UndefinedTags = %v, want [Swift]", result.Integrity.UndefinedTags)
	}
	// unusedCategories: 技術書 (defined but not used in any sidecar)
	if !slices.Equal(result.Integrity.UnusedCategories, []string{"技術書"}) {
		t.Fatalf("UnusedCategories = %v, want [技術書]", result.Integrity.UnusedCategories)
	}
	// unusedTags: Rust (defined but not used in any sidecar)
	if !slices.Equal(result.Integrity.UnusedTags, []string{"Rust"}) {
		t.Fatalf("UnusedTags = %v, want [Rust]", result.Integrity.UnusedTags)
	}

	if !slices.Equal(result.OrphanedSidecars, []string{"gone.pdf.meta.json"}) {
		t.Fatalf("OrphanedSidecars = %v, want [gone.pdf.meta.json]", result.OrphanedSidecars)
	}

	if result.Summary.TotalPDFs != 3 {
		t.Fatalf("TotalPDFs = %d, want 3", result.Summary.TotalPDFs)
	}
	if result.Summary.WithSidecar != 2 {
		t.Fatalf("WithSidecar = %d, want 2", result.Summary.WithSidecar)
	}
	if result.Summary.WithoutSidecar != 1 {
		t.Fatalf("WithoutSidecar = %d, want 1", result.Summary.WithoutSidecar)
	}
}

func TestCheckLibraryIntegrityWithMissingConfigFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)

	// Sidecar with category and tags but no config files
	pdfPath := writeTestPDF(t, root, "book.pdf")
	meta, err := shelff.CreateSidecar(pdfPath)
	if err != nil {
		t.Fatalf("CreateSidecar: %v", err)
	}
	cat := "技術雑誌"
	meta.Category = &cat
	meta.Tags = []string{"Go", "Swift"}
	if err := shelff.WriteSidecar(pdfPath, meta); err != nil {
		t.Fatalf("WriteSidecar: %v", err)
	}

	result, err := library.CheckLibrary()
	if err != nil {
		t.Fatalf("CheckLibrary returned error: %v", err)
	}

	// No config files → all used categories/tags are undefined
	if !slices.Equal(result.Integrity.UndefinedCategories, []string{"技術雑誌"}) {
		t.Fatalf("UndefinedCategories = %v, want [技術雑誌]", result.Integrity.UndefinedCategories)
	}
	if !slices.Equal(result.Integrity.UndefinedTags, []string{"Go", "Swift"}) {
		t.Fatalf("UndefinedTags = %v, want [Go Swift]", result.Integrity.UndefinedTags)
	}
	// No config files → no unused
	if len(result.Integrity.UnusedCategories) != 0 {
		t.Fatalf("UnusedCategories = %v, want empty", result.Integrity.UnusedCategories)
	}
	if len(result.Integrity.UnusedTags) != 0 {
		t.Fatalf("UnusedTags = %v, want empty", result.Integrity.UnusedTags)
	}
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("time.Parse(%q): %v", value, err)
	}
	return parsed
}
