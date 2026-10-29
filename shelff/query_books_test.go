package shelff_test

import (
	"errors"
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
		{name: "empty status means unset including no reading block", filter: shelff.BookFilter{Status: stringPtr("")}, want: []string{"c.pdf", "d.pdf"}},
		{name: "unread does not include unset", filter: shelff.BookFilter{Status: stringPtr("unread")}, want: []string{"b.pdf"}},
		{name: "reading", filter: shelff.BookFilter{Status: stringPtr("reading")}, want: []string{"a.pdf"}},
		{name: "finished", filter: shelff.BookFilter{Status: stringPtr("finished")}, want: []string{"sub/f.pdf"}},
		{name: "archived true", filter: shelff.BookFilter{Archived: boolPtr(true)}, want: []string{"a.pdf"}},
		{name: "archived false includes missing archive", filter: shelff.BookFilter{Archived: boolPtr(false)}, want: []string{"b.pdf", "c.pdf", "d.pdf", "sub/f.pdf"}},
		{name: "and category with unset status", filter: shelff.BookFilter{Category: stringPtr("Fiction"), Status: stringPtr("")}, want: []string{"d.pdf"}},
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
	if _, err := library.QueryBooksInDirectory(root, true, shelff.BookFilter{Status: stringPtr("done")}); !errors.Is(err, shelff.ErrInvalidStatusFilter) {
		t.Fatalf("invalid status err = %v, want ErrInvalidStatusFilter", err)
	}
}
