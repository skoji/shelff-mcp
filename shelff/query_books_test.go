package shelff_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/skoji/shelff-mcp/shelff"
)

func setupQueryLibrary(t *testing.T) (*shelff.Library, string) {
	t.Helper()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	subDir := filepath.Join(root, "sub")
	mkdirAll(t, subDir)

	sidecars := map[string]string{
		filepath.Join(root, "a.pdf"): `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "Book A", "dc:creator": ["Ada", "Bob"]},
  "reading": {"lastReadPage": 1, "lastReadAt": "2026-01-01T00:00:00Z", "totalPages": 10, "status": "reading"},
  "category": "Fiction",
  "tags": ["fantasy", "history"],
  "collection": {"title": "Series", "position": 1},
  "archive": {"isArchived": true, "changedAt": "2026-01-02T00:00:00Z"}
}`,
		filepath.Join(root, "b.pdf"): `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "Book B"},
  "reading": {"lastReadPage": 0, "lastReadAt": "2026-01-01T00:00:00Z", "totalPages": 10, "status": "unread"},
  "category": "Tech",
  "tags": ["go"],
  "archive": {"isArchived": false, "changedAt": "2026-01-02T00:00:00Z"}
}`,
		filepath.Join(root, "c.pdf"): `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "Book C"},
  "reading": {"lastReadPage": 0, "lastReadAt": "2026-01-01T00:00:00Z", "totalPages": 10},
  "tags": []
}`,
		filepath.Join(root, "d.pdf"): `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "Book D"},
  "category": "Fiction",
  "collection": {"title": "Other"}
}`,
		filepath.Join(subDir, "f.pdf"): `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "Book F"},
  "reading": {"lastReadPage": 10, "lastReadAt": "2026-01-01T00:00:00Z", "totalPages": 10, "status": "finished"},
  "category": "Fiction",
  "tags": ["history"]
}`,
		filepath.Join(root, "broken.pdf"): `{not json`,
	}
	for pdfPath, contents := range sidecars {
		writeTestPDF(t, filepath.Dir(pdfPath), filepath.Base(pdfPath))
		writeRawJSONFile(t, shelff.SidecarPath(pdfPath), contents)
	}
	writeTestPDF(t, root, "e.pdf")

	return library, root
}

func queryBookNames(t *testing.T, root string, result *shelff.QueryBooksResult) []string {
	t.Helper()

	names := make([]string, 0, len(result.Books))
	for _, book := range result.Books {
		rel, err := filepath.Rel(root, book.PDFPath)
		if err != nil {
			t.Fatalf("filepath.Rel(%q): %v", book.PDFPath, err)
		}
		names = append(names, filepath.ToSlash(rel))
	}
	return names
}

func boolPtr(value bool) *bool {
	return &value
}

func statusPtr(value string) *shelff.ReadingStatus {
	status := shelff.ReadingStatus(value)
	return &status
}

func tagsPtr(values ...string) *[]string {
	tags := append([]string{}, values...)
	return &tags
}

func TestQueryBooksInDirectoryFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		filter shelff.BookFilter
		want   []string
	}{
		{name: "no filter excludes books without sidecar", filter: shelff.BookFilter{}, want: []string{"a.pdf", "b.pdf", "c.pdf", "d.pdf", "sub/f.pdf"}},
		{name: "category value", filter: shelff.BookFilter{Category: stringPtr("Fiction")}, want: []string{"a.pdf", "d.pdf", "sub/f.pdf"}},
		{name: "category is case sensitive", filter: shelff.BookFilter{Category: stringPtr("fiction")}, want: []string{}},
		{name: "empty category means unset", filter: shelff.BookFilter{Category: stringPtr("")}, want: []string{"c.pdf"}},
		{name: "collection value", filter: shelff.BookFilter{Collection: stringPtr("Series")}, want: []string{"a.pdf"}},
		{name: "empty collection means unset", filter: shelff.BookFilter{Collection: stringPtr("")}, want: []string{"b.pdf", "c.pdf", "sub/f.pdf"}},
		{name: "empty tags means missing or empty", filter: shelff.BookFilter{Tags: tagsPtr()}, want: []string{"c.pdf", "d.pdf"}},
		{name: "single tag", filter: shelff.BookFilter{Tags: tagsPtr("history")}, want: []string{"a.pdf", "sub/f.pdf"}},
		{name: "tags any by default", filter: shelff.BookFilter{Tags: tagsPtr("fantasy", "go")}, want: []string{"a.pdf", "b.pdf"}},
		{name: "tags any explicit", filter: shelff.BookFilter{Tags: tagsPtr("fantasy", "go"), TagMatch: shelff.TagMatchAny}, want: []string{"a.pdf", "b.pdf"}},
		{name: "tags all", filter: shelff.BookFilter{Tags: tagsPtr("fantasy", "history"), TagMatch: shelff.TagMatchAll}, want: []string{"a.pdf"}},
		{name: "tags all without match", filter: shelff.BookFilter{Tags: tagsPtr("history", "go"), TagMatch: shelff.TagMatchAll}, want: []string{}},
		{name: "empty status means unset including no reading block", filter: shelff.BookFilter{Status: statusPtr("")}, want: []string{"c.pdf", "d.pdf"}},
		{name: "unread does not include unset", filter: shelff.BookFilter{Status: statusPtr("unread")}, want: []string{"b.pdf"}},
		{name: "reading", filter: shelff.BookFilter{Status: statusPtr("reading")}, want: []string{"a.pdf"}},
		{name: "finished", filter: shelff.BookFilter{Status: statusPtr("finished")}, want: []string{"sub/f.pdf"}},
		{name: "archived true", filter: shelff.BookFilter{Archived: boolPtr(true)}, want: []string{"a.pdf"}},
		{name: "archived false includes missing archive", filter: shelff.BookFilter{Archived: boolPtr(false)}, want: []string{"b.pdf", "c.pdf", "d.pdf", "sub/f.pdf"}},
		{name: "and category with unset status", filter: shelff.BookFilter{Category: stringPtr("Fiction"), Status: statusPtr("")}, want: []string{"d.pdf"}},
		{name: "and category with tag", filter: shelff.BookFilter{Category: stringPtr("Fiction"), Tags: tagsPtr("history"), Archived: boolPtr(false)}, want: []string{"sub/f.pdf"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			library, root := setupQueryLibrary(t)
			result, err := library.QueryBooksInDirectory(root, true, tt.filter)
			if err != nil {
				t.Fatalf("QueryBooksInDirectory returned error: %v", err)
			}
			if got := queryBookNames(t, root, result); !slices.Equal(got, tt.want) {
				t.Fatalf("books = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestQueryBooksInDirectoryReportsUnparsableSidecars(t *testing.T) {
	t.Parallel()

	library, root := setupQueryLibrary(t)
	result, err := library.QueryBooksInDirectory(root, true, shelff.BookFilter{})
	if err != nil {
		t.Fatalf("QueryBooksInDirectory returned error: %v", err)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "broken.pdf.meta.json") {
		t.Fatalf("warnings = %v, want one warning mentioning broken.pdf.meta.json", result.Warnings)
	}
	if strings.Contains(result.Warnings[0], root) {
		t.Fatalf("warning %q should use a library-relative path", result.Warnings[0])
	}
}

func TestQueryBooksInDirectoryReturnsSummaries(t *testing.T) {
	t.Parallel()

	library, root := setupQueryLibrary(t)
	result, err := library.QueryBooksInDirectory(root, true, shelff.BookFilter{})
	if err != nil {
		t.Fatalf("QueryBooksInDirectory returned error: %v", err)
	}
	if len(result.Books) < 3 {
		t.Fatalf("books = %#v, want at least 3", result.Books)
	}

	a := result.Books[0]
	if a.PDFPath != filepath.Join(root, "a.pdf") || a.SidecarPath != shelff.SidecarPath(filepath.Join(root, "a.pdf")) {
		t.Fatalf("a paths = %q / %q", a.PDFPath, a.SidecarPath)
	}
	if a.Title != "Book A" || !slices.Equal(a.Creators, []string{"Ada", "Bob"}) {
		t.Fatalf("a title/creators = %q / %v", a.Title, a.Creators)
	}
	if a.Category == nil || *a.Category != "Fiction" || !slices.Equal(a.Tags, []string{"fantasy", "history"}) {
		t.Fatalf("a category/tags = %v / %v", a.Category, a.Tags)
	}
	if a.Status == nil || *a.Status != shelff.StatusReading {
		t.Fatalf("a status = %v, want reading", a.Status)
	}
	if a.Collection == nil || a.Collection.Title != "Series" || a.Collection.Position == nil || *a.Collection.Position != 1 {
		t.Fatalf("a collection = %#v", a.Collection)
	}
	if a.Archived == nil || !*a.Archived {
		t.Fatalf("a archived = %v, want true", a.Archived)
	}

	c := result.Books[2]
	if c.Title != "Book C" || c.Creators != nil || c.Category != nil || len(c.Tags) != 0 || c.Status != nil || c.Collection != nil || c.Archived != nil {
		t.Fatalf("c summary = %#v, want only title and paths", c)
	}
}

func TestQueryBooksInDirectoryHonorsDirectoryAndRecursive(t *testing.T) {
	t.Parallel()

	library, root := setupQueryLibrary(t)

	result, err := library.QueryBooksInDirectory(root, false, shelff.BookFilter{})
	if err != nil {
		t.Fatalf("QueryBooksInDirectory returned error: %v", err)
	}
	if got, want := queryBookNames(t, root, result), []string{"a.pdf", "b.pdf", "c.pdf", "d.pdf"}; !slices.Equal(got, want) {
		t.Fatalf("non-recursive books = %v, want %v", got, want)
	}

	result, err = library.QueryBooksInDirectory("sub", true, shelff.BookFilter{})
	if err != nil {
		t.Fatalf("QueryBooksInDirectory returned error: %v", err)
	}
	if got, want := queryBookNames(t, root, result), []string{"sub/f.pdf"}; !slices.Equal(got, want) {
		t.Fatalf("sub books = %v, want %v", got, want)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("sub warnings = %v, want none", result.Warnings)
	}
}

func TestQueryBooksInDirectoryRejectsInvalidFilter(t *testing.T) {
	t.Parallel()

	library, root := setupQueryLibrary(t)

	if _, err := library.QueryBooksInDirectory(root, true, shelff.BookFilter{TagMatch: "some"}); !errors.Is(err, shelff.ErrInvalidTagMatch) {
		t.Fatalf("invalid tagMatch err = %v, want ErrInvalidTagMatch", err)
	}
	if _, err := library.QueryBooksInDirectory(root, true, shelff.BookFilter{Status: statusPtr("done")}); !errors.Is(err, shelff.ErrInvalidStatusFilter) {
		t.Fatalf("invalid status err = %v, want ErrInvalidStatusFilter", err)
	}
}

func TestQueryBooksInDirectoryTreatsEmptyStringsAsUnset(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	writeRawJSONFile(t, shelff.SidecarPath(writeTestPDF(t, root, "empty.pdf")), `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "Empty"},
  "category": "",
  "collection": {"title": ""}
}`)
	writeRawJSONFile(t, shelff.SidecarPath(writeTestPDF(t, root, "missing.pdf")), `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "Missing"}
}`)
	writeRawJSONFile(t, shelff.SidecarPath(writeTestPDF(t, root, "set.pdf")), `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "Set"},
  "category": "X",
  "collection": {"title": "S"}
}`)

	tests := []struct {
		name   string
		filter shelff.BookFilter
		want   []string
	}{
		{name: "empty category filter matches empty and missing", filter: shelff.BookFilter{Category: stringPtr("")}, want: []string{"empty.pdf", "missing.pdf"}},
		{name: "category value does not match empty", filter: shelff.BookFilter{Category: stringPtr("X")}, want: []string{"set.pdf"}},
		{name: "empty collection filter matches empty title and missing", filter: shelff.BookFilter{Collection: stringPtr("")}, want: []string{"empty.pdf", "missing.pdf"}},
		{name: "collection value does not match empty title", filter: shelff.BookFilter{Collection: stringPtr("S")}, want: []string{"set.pdf"}},
	}
	for _, tt := range tests {
		result, err := library.QueryBooksInDirectory(root, true, tt.filter)
		if err != nil {
			t.Fatalf("%s: QueryBooksInDirectory returned error: %v", tt.name, err)
		}
		if got := queryBookNames(t, root, result); !slices.Equal(got, tt.want) {
			t.Fatalf("%s: books = %v, want %v", tt.name, got, tt.want)
		}
	}

	result, err := library.QueryBooksInDirectory(root, true, shelff.BookFilter{})
	if err != nil {
		t.Fatalf("QueryBooksInDirectory returned error: %v", err)
	}
	empty := result.Books[0]
	if empty.Title != "Empty" || empty.Category != nil || empty.Collection != nil {
		t.Fatalf("empty summary = %#v, want category and collection omitted", empty)
	}
}

func TestQueryBooksInDirectoryWarnsAboutSidecarsRemovedDuringQuery(t *testing.T) {
	root := t.TempDir()
	library := openTestLibrary(t, root)
	keptPDF := writeTestPDF(t, root, "kept.pdf")
	writeRawJSONFile(t, shelff.SidecarPath(keptPDF), `{"schemaVersion":1,"metadata":{"dc:title":"Kept"}}`)
	gonePDF := writeTestPDF(t, root, "gone.pdf")
	goneSidecar := shelff.SidecarPath(gonePDF)
	writeRawJSONFile(t, goneSidecar, `{"schemaVersion":1,"metadata":{"dc:title":"Gone"}}`)

	shelff.SetReadSidecarFileForTest(t, func(path string) ([]byte, error) {
		if path == goneSidecar {
			if err := os.Remove(path); err != nil {
				t.Fatalf("os.Remove(%q): %v", path, err)
			}
		}
		return os.ReadFile(path)
	})

	result, err := library.QueryBooksInDirectory(root, true, shelff.BookFilter{})
	if err != nil {
		t.Fatalf("QueryBooksInDirectory returned error: %v", err)
	}
	if got, want := queryBookNames(t, root, result), []string{"kept.pdf"}; !slices.Equal(got, want) {
		t.Fatalf("books = %v, want %v", got, want)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "gone.pdf.meta.json") || strings.Contains(result.Warnings[0], root) {
		t.Fatalf("warnings = %v, want one relative warning mentioning gone.pdf.meta.json", result.Warnings)
	}
}

func TestQueryBooksInDirectoryWithSymlinkRootNormalizesPaths(t *testing.T) {
	t.Parallel()

	realRoot := t.TempDir()
	linkRoot := filepath.Join(t.TempDir(), "library-link")
	subDir := filepath.Join(realRoot, "sub")
	mkdirAll(t, subDir)
	bookPDF := writeTestPDF(t, subDir, "book.pdf")
	writeRawJSONFile(t, shelff.SidecarPath(bookPDF), `{"schemaVersion":1,"metadata":{"dc:title":"Book"}}`)
	brokenPDF := writeTestPDF(t, subDir, "broken.pdf")
	writeRawJSONFile(t, shelff.SidecarPath(brokenPDF), `{not json`)

	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Skipf("os.Symlink unavailable: %v", err)
	}

	library := openTestLibrary(t, linkRoot)
	for _, dir := range []string{linkRoot, "sub"} {
		result, err := library.QueryBooksInDirectory(dir, true, shelff.BookFilter{})
		if err != nil {
			t.Fatalf("QueryBooksInDirectory(%q) returned error: %v", dir, err)
		}

		wantPDF := filepath.Join(linkRoot, "sub", "book.pdf")
		if len(result.Books) != 1 || result.Books[0].PDFPath != wantPDF || result.Books[0].SidecarPath != shelff.SidecarPath(wantPDF) {
			t.Fatalf("QueryBooksInDirectory(%q) books = %#v, want only %q under symlink root", dir, result.Books, wantPDF)
		}
		wantWarningPath := filepath.Join("sub", "broken.pdf.meta.json")
		if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], wantWarningPath) || strings.Contains(result.Warnings[0], "..") {
			t.Fatalf("QueryBooksInDirectory(%q) warnings = %v, want one warning with %q and no ..", dir, result.Warnings, wantWarningPath)
		}
	}
}
