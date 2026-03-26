package shelff_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/skoji/shelff-go/shelff"
)

func TestReadCategoriesReturnsEmptyListWhenMissing(t *testing.T) {
	t.Parallel()

	library := openTestLibrary(t, t.TempDir())

	cats, err := library.ReadCategories()
	if err != nil {
		t.Fatalf("ReadCategories returned error: %v", err)
	}

	if cats.Version != shelff.SchemaVersion {
		t.Fatalf("Version = %d, want %d", cats.Version, shelff.SchemaVersion)
	}
	if len(cats.Categories) != 0 {
		t.Fatalf("len(Categories) = %d, want 0", len(cats.Categories))
	}
}

func TestWriteCategoriesCreatesConfigDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configDir := filepath.Join(root, shelff.ConfigDir)
	categoriesPath := filepath.Join(configDir, shelff.CategoriesFile)
	library := openTestLibrary(t, root)

	if err := library.WriteCategories(&shelff.CategoryList{
		Version: shelff.SchemaVersion,
		Categories: []shelff.CategoryItem{
			{Name: "Alpha", Order: 7},
		},
	}); err != nil {
		t.Fatalf("WriteCategories returned error: %v", err)
	}

	if info, err := os.Stat(configDir); err != nil || !info.IsDir() {
		t.Fatalf("config dir stat = (%v, %v), want existing directory", info, err)
	}
	if _, err := os.Stat(categoriesPath); err != nil {
		t.Fatalf("categories path stat err = %v, want file", err)
	}
}

func TestWriteCategoriesNormalizesOrderAndPreservesUnknownFields(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configDir := filepath.Join(root, shelff.ConfigDir)
	mkdirAll(t, configDir)
	writeRawJSONFile(t, filepath.Join(configDir, shelff.CategoriesFile), `{
  "version": 1,
  "categories": [
    {"name": "Alpha", "order": 99}
  ],
  "x-custom": 42
}`)

	library := openTestLibrary(t, root)
	cats, err := library.ReadCategories()
	if err != nil {
		t.Fatalf("ReadCategories returned error: %v", err)
	}
	cats.Categories = append(cats.Categories, shelff.CategoryItem{Name: " Beta ", Order: 77})

	if err := library.WriteCategories(cats); err != nil {
		t.Fatalf("WriteCategories returned error: %v", err)
	}

	got := readJSONMap(t, filepath.Join(configDir, shelff.CategoriesFile))
	if got["x-custom"] != json.Number("42") {
		t.Fatalf("x-custom = %#v, want 42", got["x-custom"])
	}

	categories := got["categories"].([]any)
	second := categories[1].(map[string]any)
	if second["name"] != "Beta" {
		t.Fatalf("second category name = %#v, want %q", second["name"], "Beta")
	}
	if second["order"] != json.Number("1") {
		t.Fatalf("second category order = %#v, want 1", second["order"])
	}
}

func TestAddCategoryTrimsNameAndRejectsDuplicates(t *testing.T) {
	t.Parallel()

	library := openTestLibrary(t, t.TempDir())
	if err := library.AddCategory("  Fiction  "); err != nil {
		t.Fatalf("AddCategory returned error: %v", err)
	}
	if err := library.AddCategory("Fiction"); !errors.Is(err, shelff.ErrCategoryAlreadyExists) {
		t.Fatalf("second AddCategory error = %v, want ErrCategoryAlreadyExists", err)
	}

	cats, err := library.ReadCategories()
	if err != nil {
		t.Fatalf("ReadCategories returned error: %v", err)
	}
	if len(cats.Categories) != 1 || cats.Categories[0].Name != "Fiction" || cats.Categories[0].Order != 0 {
		t.Fatalf("Categories = %#v, want single normalized Fiction entry", cats.Categories)
	}
}

func TestRemoveCategoryWithoutCascadeLeavesSidecarsUntouched(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	pdfPath := writeCategorySidecar(t, root, "book.pdf", "Fiction")
	if err := library.WriteCategories(&shelff.CategoryList{
		Version: shelff.SchemaVersion,
		Categories: []shelff.CategoryItem{
			{Name: "Fiction", Order: 0},
		},
	}); err != nil {
		t.Fatalf("WriteCategories returned error: %v", err)
	}

	if err := library.RemoveCategory("Fiction", false); err != nil {
		t.Fatalf("RemoveCategory returned error: %v", err)
	}

	meta := mustReadSidecar(t, pdfPath)
	if meta.Category == nil || *meta.Category != "Fiction" {
		t.Fatalf("Category after non-cascade remove = %#v, want Fiction", meta.Category)
	}
}

func TestRemoveCategoryWithCascadeClearsMatchingSidecars(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	matchingPDF := writeCategorySidecar(t, root, "match.pdf", "Fiction")
	otherPDF := writeCategorySidecar(t, root, "other.pdf", "History")
	if err := library.WriteCategories(&shelff.CategoryList{
		Version: shelff.SchemaVersion,
		Categories: []shelff.CategoryItem{
			{Name: "Fiction", Order: 0},
			{Name: "History", Order: 1},
		},
	}); err != nil {
		t.Fatalf("WriteCategories returned error: %v", err)
	}

	if err := library.RemoveCategory("Fiction", true); err != nil {
		t.Fatalf("RemoveCategory returned error: %v", err)
	}

	if meta := mustReadSidecar(t, matchingPDF); meta.Category != nil {
		t.Fatalf("matching category = %#v, want nil", *meta.Category)
	}
	if meta := mustReadSidecar(t, otherPDF); meta.Category == nil || *meta.Category != "History" {
		t.Fatalf("other category = %#v, want History", meta.Category)
	}
}

func TestRenameCategoryWithCascadeUpdatesSidecars(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	pdfPath := writeCategorySidecar(t, root, "book.pdf", "Fiction")
	if err := library.WriteCategories(&shelff.CategoryList{
		Version: shelff.SchemaVersion,
		Categories: []shelff.CategoryItem{
			{Name: "Fiction", Order: 0},
		},
	}); err != nil {
		t.Fatalf("WriteCategories returned error: %v", err)
	}

	if err := library.RenameCategory("Fiction", "  Novels  ", true); err != nil {
		t.Fatalf("RenameCategory returned error: %v", err)
	}

	cats, err := library.ReadCategories()
	if err != nil {
		t.Fatalf("ReadCategories returned error: %v", err)
	}
	if len(cats.Categories) != 1 || cats.Categories[0].Name != "Novels" {
		t.Fatalf("Categories = %#v, want Novels", cats.Categories)
	}

	meta := mustReadSidecar(t, pdfPath)
	if meta.Category == nil || *meta.Category != "Novels" {
		t.Fatalf("Category = %#v, want Novels", meta.Category)
	}
}

func TestReorderCategoriesNormalizesOrderAndValidatesSet(t *testing.T) {
	t.Parallel()

	library := openTestLibrary(t, t.TempDir())
	if err := library.WriteCategories(&shelff.CategoryList{
		Version: shelff.SchemaVersion,
		Categories: []shelff.CategoryItem{
			{Name: "Fiction", Order: 9},
			{Name: "History", Order: 8},
		},
	}); err != nil {
		t.Fatalf("WriteCategories returned error: %v", err)
	}

	if err := library.ReorderCategories([]string{"History", "Fiction"}); err != nil {
		t.Fatalf("ReorderCategories returned error: %v", err)
	}
	if err := library.ReorderCategories([]string{"History"}); !errors.Is(err, shelff.ErrCategoryMismatch) {
		t.Fatalf("mismatched ReorderCategories error = %v, want ErrCategoryMismatch", err)
	}

	cats, err := library.ReadCategories()
	if err != nil {
		t.Fatalf("ReadCategories returned error: %v", err)
	}
	if cats.Categories[0].Name != "History" || cats.Categories[0].Order != 0 {
		t.Fatalf("first category = %#v, want History order 0", cats.Categories[0])
	}
	if cats.Categories[1].Name != "Fiction" || cats.Categories[1].Order != 1 {
		t.Fatalf("second category = %#v, want Fiction order 1", cats.Categories[1])
	}
}

func TestReadTagOrderReturnsEmptyListWhenMissing(t *testing.T) {
	t.Parallel()

	library := openTestLibrary(t, t.TempDir())

	tags, err := library.ReadTagOrder()
	if err != nil {
		t.Fatalf("ReadTagOrder returned error: %v", err)
	}

	if tags.Version != shelff.SchemaVersion {
		t.Fatalf("Version = %d, want %d", tags.Version, shelff.SchemaVersion)
	}
	if len(tags.TagOrder) != 0 {
		t.Fatalf("len(TagOrder) = %d, want 0", len(tags.TagOrder))
	}
}

func TestWriteTagOrderCreatesConfigDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configDir := filepath.Join(root, shelff.ConfigDir)
	tagsPath := filepath.Join(configDir, shelff.TagsFile)
	library := openTestLibrary(t, root)

	if err := library.WriteTagOrder(&shelff.TagOrder{
		Version:  shelff.SchemaVersion,
		TagOrder: []string{"history"},
	}); err != nil {
		t.Fatalf("WriteTagOrder returned error: %v", err)
	}

	if info, err := os.Stat(configDir); err != nil || !info.IsDir() {
		t.Fatalf("config dir stat = (%v, %v), want existing directory", info, err)
	}
	if _, err := os.Stat(tagsPath); err != nil {
		t.Fatalf("tags path stat err = %v, want file", err)
	}
}

func TestWriteTagOrderPreservesUnknownFields(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configDir := filepath.Join(root, shelff.ConfigDir)
	mkdirAll(t, configDir)
	writeRawJSONFile(t, filepath.Join(configDir, shelff.TagsFile), `{
  "version": 1,
  "tagOrder": ["old"],
  "x-custom": "kept"
}`)

	library := openTestLibrary(t, root)
	tags, err := library.ReadTagOrder()
	if err != nil {
		t.Fatalf("ReadTagOrder returned error: %v", err)
	}
	tags.TagOrder = []string{" sci-fi ", "history"}

	if err := library.WriteTagOrder(tags); err != nil {
		t.Fatalf("WriteTagOrder returned error: %v", err)
	}

	got := readJSONMap(t, filepath.Join(configDir, shelff.TagsFile))
	if got["x-custom"] != "kept" {
		t.Fatalf("x-custom = %#v, want %q", got["x-custom"], "kept")
	}
	tagOrder := got["tagOrder"].([]any)
	if tagOrder[0] != "sci-fi" || tagOrder[1] != "history" {
		t.Fatalf("tagOrder = %#v, want normalized values", tagOrder)
	}
}

func TestAddTagToOrderTrimsNameAndRejectsDuplicates(t *testing.T) {
	t.Parallel()

	library := openTestLibrary(t, t.TempDir())
	if err := library.AddTagToOrder("  sci-fi "); err != nil {
		t.Fatalf("AddTagToOrder returned error: %v", err)
	}
	if err := library.AddTagToOrder("sci-fi"); !errors.Is(err, shelff.ErrTagAlreadyExists) {
		t.Fatalf("second AddTagToOrder error = %v, want ErrTagAlreadyExists", err)
	}
}

func TestRemoveTagFromOrderWithAndWithoutCascade(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	untouchedPDF := writeTagsSidecar(t, root, "untouched.pdf", []string{"sci-fi", "history"})
	cascadedPDF := writeTagsSidecar(t, root, "cascaded.pdf", []string{"sci-fi", "history"})
	if err := library.WriteTagOrder(&shelff.TagOrder{
		Version:  shelff.SchemaVersion,
		TagOrder: []string{"sci-fi", "history"},
	}); err != nil {
		t.Fatalf("WriteTagOrder returned error: %v", err)
	}

	if err := library.RemoveTagFromOrder("sci-fi", false); err != nil {
		t.Fatalf("RemoveTagFromOrder returned error: %v", err)
	}
	if meta := mustReadSidecar(t, untouchedPDF); len(meta.Tags) != 2 || meta.Tags[0] != "sci-fi" {
		t.Fatalf("non-cascade tags = %#v, want unchanged", meta.Tags)
	}

	if err := library.AddTagToOrder("sci-fi"); err != nil {
		t.Fatalf("AddTagToOrder returned error: %v", err)
	}
	if err := library.RemoveTagFromOrder("sci-fi", true); err != nil {
		t.Fatalf("cascade RemoveTagFromOrder returned error: %v", err)
	}

	if meta := mustReadSidecar(t, cascadedPDF); len(meta.Tags) != 1 || meta.Tags[0] != "history" {
		t.Fatalf("cascade tags = %#v, want [history]", meta.Tags)
	}
}

func TestRemoveTagFromOrderMissingTagIsNoOp(t *testing.T) {
	t.Parallel()

	library := openTestLibrary(t, t.TempDir())
	if err := library.WriteTagOrder(&shelff.TagOrder{
		Version:  shelff.SchemaVersion,
		TagOrder: []string{"history"},
	}); err != nil {
		t.Fatalf("WriteTagOrder returned error: %v", err)
	}

	if err := library.RemoveTagFromOrder("missing", false); err != nil {
		t.Fatalf("RemoveTagFromOrder returned error: %v", err)
	}

	tags, err := library.ReadTagOrder()
	if err != nil {
		t.Fatalf("ReadTagOrder returned error: %v", err)
	}
	if len(tags.TagOrder) != 1 || tags.TagOrder[0] != "history" {
		t.Fatalf("TagOrder = %#v, want unchanged [history]", tags.TagOrder)
	}
}

func TestRenameTagWithCascadeUpdatesSidecars(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	pdfPath := writeTagsSidecar(t, root, "book.pdf", []string{"sci-fi", "mystery", "mystery"})
	if err := library.WriteTagOrder(&shelff.TagOrder{
		Version:  shelff.SchemaVersion,
		TagOrder: []string{"sci-fi", "history"},
	}); err != nil {
		t.Fatalf("WriteTagOrder returned error: %v", err)
	}

	if err := library.RenameTag("sci-fi", " history ", true); !errors.Is(err, shelff.ErrTagAlreadyExists) {
		t.Fatalf("RenameTag duplicate error = %v, want ErrTagAlreadyExists", err)
	}
	if err := library.RenameTag("sci-fi", "fantasy", true); err != nil {
		t.Fatalf("RenameTag returned error: %v", err)
	}

	tags, err := library.ReadTagOrder()
	if err != nil {
		t.Fatalf("ReadTagOrder returned error: %v", err)
	}
	if len(tags.TagOrder) != 2 || tags.TagOrder[0] != "fantasy" || tags.TagOrder[1] != "history" {
		t.Fatalf("TagOrder = %#v, want [fantasy history]", tags.TagOrder)
	}

	meta := mustReadSidecar(t, pdfPath)
	if len(meta.Tags) != 3 || meta.Tags[0] != "fantasy" || meta.Tags[1] != "mystery" || meta.Tags[2] != "mystery" {
		t.Fatalf("Tags = %#v, want [fantasy mystery mystery]", meta.Tags)
	}
}

func TestRenameTagMissingInOrderStillCascadesSidecars(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	pdfPath := writeTagsSidecar(t, root, "book.pdf", []string{"sci-fi", "history"})
	if err := library.WriteTagOrder(&shelff.TagOrder{
		Version:  shelff.SchemaVersion,
		TagOrder: []string{"history"},
	}); err != nil {
		t.Fatalf("WriteTagOrder returned error: %v", err)
	}

	if err := library.RenameTag("sci-fi", "fantasy", true); err != nil {
		t.Fatalf("RenameTag returned error: %v", err)
	}

	tags, err := library.ReadTagOrder()
	if err != nil {
		t.Fatalf("ReadTagOrder returned error: %v", err)
	}
	if len(tags.TagOrder) != 1 || tags.TagOrder[0] != "history" {
		t.Fatalf("TagOrder = %#v, want unchanged [history]", tags.TagOrder)
	}

	meta := mustReadSidecar(t, pdfPath)
	if len(meta.Tags) != 2 || meta.Tags[0] != "fantasy" || meta.Tags[1] != "history" {
		t.Fatalf("Tags = %#v, want [fantasy history]", meta.Tags)
	}
}

func TestRenameTagCascadeDeduplicatesResultingTagCollision(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := openTestLibrary(t, root)
	pdfPath := writeTagsSidecar(t, root, "book.pdf", []string{"sci-fi", "history"})
	if err := library.WriteTagOrder(&shelff.TagOrder{
		Version:  shelff.SchemaVersion,
		TagOrder: []string{"sci-fi"},
	}); err != nil {
		t.Fatalf("WriteTagOrder returned error: %v", err)
	}

	if err := library.RenameTag("sci-fi", "history", true); err != nil {
		t.Fatalf("RenameTag returned error: %v", err)
	}

	tags, err := library.ReadTagOrder()
	if err != nil {
		t.Fatalf("ReadTagOrder returned error: %v", err)
	}
	if len(tags.TagOrder) != 1 || tags.TagOrder[0] != "history" {
		t.Fatalf("TagOrder = %#v, want [history]", tags.TagOrder)
	}

	meta := mustReadSidecar(t, pdfPath)
	if len(meta.Tags) != 1 || meta.Tags[0] != "history" {
		t.Fatalf("Tags = %#v, want [history]", meta.Tags)
	}
}

func TestReorderTagsWritesNormalizedOrder(t *testing.T) {
	t.Parallel()

	library := openTestLibrary(t, t.TempDir())
	if err := library.ReorderTags([]string{" history ", "sci-fi"}); err != nil {
		t.Fatalf("ReorderTags returned error: %v", err)
	}

	tags, err := library.ReadTagOrder()
	if err != nil {
		t.Fatalf("ReadTagOrder returned error: %v", err)
	}
	if len(tags.TagOrder) != 2 || tags.TagOrder[0] != "history" || tags.TagOrder[1] != "sci-fi" {
		t.Fatalf("TagOrder = %#v, want normalized order", tags.TagOrder)
	}
}

func TestReorderTagsDoesNotRequireExistingSetMatch(t *testing.T) {
	t.Parallel()

	library := openTestLibrary(t, t.TempDir())
	if err := library.WriteTagOrder(&shelff.TagOrder{
		Version:  shelff.SchemaVersion,
		TagOrder: []string{"history"},
	}); err != nil {
		t.Fatalf("WriteTagOrder returned error: %v", err)
	}

	if err := library.ReorderTags([]string{"new-tag", " history "}); err != nil {
		t.Fatalf("ReorderTags returned error: %v", err)
	}

	tags, err := library.ReadTagOrder()
	if err != nil {
		t.Fatalf("ReadTagOrder returned error: %v", err)
	}
	if len(tags.TagOrder) != 2 || tags.TagOrder[0] != "new-tag" || tags.TagOrder[1] != "history" {
		t.Fatalf("TagOrder = %#v, want [new-tag history]", tags.TagOrder)
	}
}

func openTestLibrary(t *testing.T, root string) *shelff.Library {
	t.Helper()

	library, err := shelff.OpenLibrary(root)
	if err != nil {
		t.Fatalf("OpenLibrary(%q): %v", root, err)
	}
	return library
}

func writeCategorySidecar(t *testing.T, dir string, fileName string, category string) string {
	t.Helper()

	pdfPath := writeTestPDF(t, dir, fileName)
	meta, err := shelff.CreateSidecar(pdfPath)
	if err != nil {
		t.Fatalf("CreateSidecar(%q): %v", pdfPath, err)
	}
	meta.Category = &category
	if err := shelff.WriteSidecar(pdfPath, meta); err != nil {
		t.Fatalf("WriteSidecar(%q): %v", pdfPath, err)
	}
	return pdfPath
}

func writeTagsSidecar(t *testing.T, dir string, fileName string, tags []string) string {
	t.Helper()

	pdfPath := writeTestPDF(t, dir, fileName)
	meta, err := shelff.CreateSidecar(pdfPath)
	if err != nil {
		t.Fatalf("CreateSidecar(%q): %v", pdfPath, err)
	}
	meta.Tags = append([]string(nil), tags...)
	if err := shelff.WriteSidecar(pdfPath, meta); err != nil {
		t.Fatalf("WriteSidecar(%q): %v", pdfPath, err)
	}
	return pdfPath
}

func mustReadSidecar(t *testing.T, pdfPath string) *shelff.SidecarMetadata {
	t.Helper()

	meta, err := shelff.ReadSidecar(pdfPath)
	if err != nil {
		t.Fatalf("ReadSidecar(%q): %v", pdfPath, err)
	}
	if meta == nil {
		t.Fatalf("ReadSidecar(%q) returned nil metadata", pdfPath)
	}
	return meta
}

func writeRawJSONFile(t *testing.T, path string, contents string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q): %v", path, err)
	}
}

func readJSONMap(t *testing.T, path string) map[string]any {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%q): %v", path, err)
	}

	var result map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("json decode %q: %v", path, err)
	}
	return result
}
