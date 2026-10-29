package shelff_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/skoji/shelff-mcp/shelff"
)

const badLastReadAtSidecar = `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "bad"},
  "category": "Skipped",
  "tags": ["skipped"],
  "reading": {"lastReadPage": 1, "lastReadAt": "yesterday", "totalPages": 10, "status": "reading"}
}`

func assertOneUnreadable(t *testing.T, got []shelff.UnreadableSidecar, wantPath string) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("UnreadableSidecars = %+v, want 1 entry", got)
	}
	if got[0].SidecarPath != wantPath {
		t.Fatalf("UnreadableSidecars[0].SidecarPath = %q, want %q", got[0].SidecarPath, wantPath)
	}
	if got[0].Error == "" {
		t.Fatal("UnreadableSidecars[0].Error is empty")
	}
}

func TestStatsReportsUnreadableSidecarAndContinues(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	writeTagsSidecar(t, root, "good.pdf", []string{"golang"})
	bad := writeTestPDF(t, root, "bad.pdf")
	writeRawJSONFile(t, shelff.SidecarPath(bad), badLastReadAtSidecar)

	stats, err := library.Stats()
	if err != nil {
		t.Fatalf("Stats returned error: %v", err)
	}
	if stats.TotalPDFs != 2 || stats.WithSidecar != 2 || stats.WithoutSidecar != 0 {
		t.Fatalf("stats = %+v, want 2 PDFs with sidecar", stats)
	}
	if stats.TagCounts["golang"] != 1 || len(stats.TagCounts) != 1 {
		t.Fatalf("TagCounts = %v, want only golang", stats.TagCounts)
	}
	if len(stats.CategoryCounts) != 0 {
		t.Fatalf("CategoryCounts = %v, want unreadable sidecar skipped", stats.CategoryCounts)
	}
	assertOneUnreadable(t, stats.UnreadableSidecars, "bad.pdf.meta.json")
}

func TestStatsReportsNoUnreadableSidecarsAsEmptyList(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	writeTagsSidecar(t, root, "good.pdf", []string{"golang"})

	stats, err := library.Stats()
	if err != nil {
		t.Fatalf("Stats returned error: %v", err)
	}
	if stats.UnreadableSidecars == nil || len(stats.UnreadableSidecars) != 0 {
		t.Fatalf("UnreadableSidecars = %#v, want empty non-nil slice", stats.UnreadableSidecars)
	}
	data, err := json.Marshal(stats)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"UnreadableSidecars":[]`) {
		t.Fatalf("JSON = %s, want UnreadableSidecars:[]", data)
	}
}

func TestCollectAllTagsReportsUnreadableSidecarAndContinues(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	writeTagsSidecar(t, root, "good.pdf", []string{"golang"})
	bad := writeTestPDF(t, root, "bad.pdf")
	writeRawJSONFile(t, shelff.SidecarPath(bad), badLastReadAtSidecar)

	result, err := library.CollectAllTags()
	if err != nil {
		t.Fatalf("CollectAllTags returned error: %v", err)
	}
	if !slices.Equal(result.Tags, []string{"golang"}) {
		t.Fatalf("Tags = %v, want [golang]", result.Tags)
	}
	assertOneUnreadable(t, result.UnreadableSidecars, "bad.pdf.meta.json")
}

func TestCollectAllTagsReportsNoUnreadableSidecarsAsEmptyList(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	writeTagsSidecar(t, root, "good.pdf", []string{"golang"})

	result, err := library.CollectAllTags()
	if err != nil {
		t.Fatalf("CollectAllTags returned error: %v", err)
	}
	if result.UnreadableSidecars == nil || len(result.UnreadableSidecars) != 0 {
		t.Fatalf("UnreadableSidecars = %#v, want empty non-nil slice", result.UnreadableSidecars)
	}
}

func TestCheckLibraryReportsUnreadableSidecarForNonArchiveCause(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	writeTagsSidecar(t, root, "good.pdf", []string{"golang"})
	bad := writeTestPDF(t, root, "bad.pdf")
	writeRawJSONFile(t, shelff.SidecarPath(bad), badLastReadAtSidecar)

	result, err := library.CheckLibrary()
	if err != nil {
		t.Fatalf("CheckLibrary returned error: %v", err)
	}
	assertOneUnreadable(t, result.UnreadableSidecars, "bad.pdf.meta.json")
	if len(result.InvalidArchives) != 0 {
		t.Fatalf("InvalidArchives = %+v, want none", result.InvalidArchives)
	}
	if len(result.Integrity.UndefinedCategories) != 0 || !slices.Equal(result.Integrity.UndefinedTags, []string{"golang"}) {
		t.Fatalf("Integrity = %+v, want unreadable sidecar excluded", result.Integrity)
	}
	if result.Summary.TotalPDFs != 2 || result.Summary.WithSidecar != 2 {
		t.Fatalf("Summary = %+v, want 2/2", result.Summary)
	}
}

func TestCheckLibraryReportsMalformedJSONAsUnreadable(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	bad := writeTestPDF(t, root, "bad.pdf")
	writeRawJSONFile(t, shelff.SidecarPath(bad), `{not json`)

	result, err := library.CheckLibrary()
	if err != nil {
		t.Fatalf("CheckLibrary returned error: %v", err)
	}
	assertOneUnreadable(t, result.UnreadableSidecars, "bad.pdf.meta.json")
	if len(result.InvalidArchives) != 0 {
		t.Fatalf("InvalidArchives = %+v, want none", result.InvalidArchives)
	}
	if result.Summary.WithSidecar != 1 {
		t.Fatalf("Summary = %+v, want WithSidecar 1", result.Summary)
	}
}

func TestCheckLibraryReportsNoUnreadableSidecarsAsEmptyList(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	writeTagsSidecar(t, root, "plain.pdf", []string{"golang"})

	result, err := library.CheckLibrary()
	if err != nil {
		t.Fatalf("CheckLibrary returned error: %v", err)
	}
	if result.UnreadableSidecars == nil || len(result.UnreadableSidecars) != 0 {
		t.Fatalf("UnreadableSidecars = %#v, want empty non-nil slice", result.UnreadableSidecars)
	}
}
