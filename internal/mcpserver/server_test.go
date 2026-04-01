package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skoji/shelff-mcp/shelff"
)

func TestServerListsReadOnlyTools(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, t.TempDir())
	session := newClientSession(t, server)
	defer session.Close()

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools error = %v", err)
	}

	var got []string
	for _, tool := range result.Tools {
		got = append(got, tool.Name)
	}
	slices.Sort(got)
	if !slices.Equal(got, toolNames()) {
		t.Fatalf("tool names = %v, want %v", got, toolNames())
	}
}

func TestReadOnlyToolsReturnStructuredData(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library, err := shelff.OpenLibrary(root)
	if err != nil {
		t.Fatalf("OpenLibrary error = %v", err)
	}

	pdfWithSidecar := writeTestPDF(t, root, "book.pdf")
	sidecar, err := shelff.CreateSidecar(pdfWithSidecar)
	if err != nil {
		t.Fatalf("CreateSidecar error = %v", err)
	}
	sidecar.Metadata.Creator = []string{"Ada"}
	sidecar.Tags = []string{"golang", "reading"}
	category := "reference"
	sidecar.Category = &category
	status := shelff.StatusReading
	finishedAt := time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC)
	sidecar.Reading = &shelff.ReadingProgress{
		LastReadPage: 10,
		LastReadAt:   time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC),
		TotalPages:   100,
		Status:       &status,
		FinishedAt:   &finishedAt,
	}
	if err := shelff.WriteSidecar(pdfWithSidecar, sidecar); err != nil {
		t.Fatalf("WriteSidecar error = %v", err)
	}

	nestedDir := filepath.Join(root, "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	writeTestPDF(t, nestedDir, "draft.pdf")
	writeRawJSONFile(t, filepath.Join(nestedDir, "orphan.pdf.meta.json"), `{"schemaVersion":1,"metadata":{"dc:title":"orphan"}}`)

	if err := library.WriteCategories(&shelff.CategoryList{
		Version: 1,
		Categories: []shelff.CategoryItem{
			{Name: "reference", Order: 0},
			{Name: "tutorial", Order: 1},
		},
	}); err != nil {
		t.Fatalf("WriteCategories error = %v", err)
	}
	if err := library.WriteTagOrder(&shelff.TagOrder{
		Version:  1,
		TagOrder: []string{"reading", "golang"},
	}); err != nil {
		t.Fatalf("WriteTagOrder error = %v", err)
	}

	server := newTestServer(t, root)
	session := newClientSession(t, server)
	defer session.Close()

	readResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "read_metadata",
		Arguments: map[string]any{"pdfPath": "book.pdf"},
	})
	if err != nil {
		t.Fatalf("read_metadata error = %v", err)
	}
	var readOut readMetadataOutput
	decodeStructuredContent(t, readResult, &readOut)
	if !readOut.HasSidecar || readOut.Metadata == nil || readOut.Metadata.Metadata.Title != "book" {
		t.Fatalf("read_metadata output = %#v", readOut)
	}

	missingReadResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "read_metadata",
		Arguments: map[string]any{"pdfPath": "nested/draft.pdf"},
	})
	if err != nil {
		t.Fatalf("read_metadata missing error = %v", err)
	}
	var missingReadOut readMetadataOutput
	decodeStructuredContent(t, missingReadResult, &missingReadOut)
	if missingReadOut.HasSidecar {
		t.Fatalf("read_metadata missing: hasSidecar = true, want false")
	}
	if missingReadOut.Metadata == nil {
		t.Fatal("read_metadata missing: metadata is nil, want minimal metadata")
	}
	if missingReadOut.Metadata.Metadata.Title != "draft" {
		t.Fatalf("read_metadata missing: title = %q, want %q", missingReadOut.Metadata.Metadata.Title, "draft")
	}

	scanResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "scan_books",
		Arguments: map[string]any{"recursive": true},
	})
	if err != nil {
		t.Fatalf("scan_books error = %v", err)
	}
	var scanOut scanBooksOutput
	decodeStructuredContent(t, scanResult, &scanOut)
	if len(scanOut.Books) != 2 {
		t.Fatalf("scan_books count = %d, want 2", len(scanOut.Books))
	}
	if scanOut.Total != 2 || scanOut.Offset != 0 || scanOut.Limit != defaultScanBooksLimit || scanOut.HasMore {
		t.Fatalf("scan_books page info = %#v, want total=2 offset=0 limit=%d hasMore=false", scanOut, defaultScanBooksLimit)
	}
	if scanOut.Books[0].PDFPath != "book.pdf" || !scanOut.Books[0].HasSidecar {
		t.Fatalf("first scan_books entry = %#v", scanOut.Books[0])
	}
	if scanOut.Books[1].PDFPath != "nested/draft.pdf" || scanOut.Books[1].HasSidecar {
		t.Fatalf("second scan_books entry = %#v", scanOut.Books[1])
	}

	orphanedResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "find_orphaned_sidecars",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("find_orphaned_sidecars error = %v", err)
	}
	var orphanedOut orphanedSidecarsOutput
	decodeStructuredContent(t, orphanedResult, &orphanedOut)
	if len(orphanedOut.Sidecars) != 1 || orphanedOut.Sidecars[0].SidecarPath != "nested/orphan.pdf.meta.json" {
		t.Fatalf("orphaned sidecars = %#v", orphanedOut.Sidecars)
	}

	validateResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "validate_sidecar",
		Arguments: map[string]any{"pdfPath": "book.pdf"},
	})
	if err != nil {
		t.Fatalf("validate_sidecar error = %v", err)
	}
	var validateOut validateSidecarOutput
	decodeStructuredContent(t, validateResult, &validateOut)
	if len(validateOut.Errors) != 0 {
		t.Fatalf("validate_sidecar errors = %#v, want empty", validateOut.Errors)
	}

	statsResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "library_stats",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("library_stats error = %v", err)
	}
	var statsOut shelff.LibraryStats
	decodeStructuredContent(t, statsResult, &statsOut)
	if statsOut.TotalPDFs != 2 || statsOut.WithSidecar != 1 || statsOut.OrphanedSidecars != 1 {
		t.Fatalf("library_stats = %#v", statsOut)
	}

	tagsResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "collect_all_tags",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("collect_all_tags error = %v", err)
	}
	var tagsOut collectAllTagsOutput
	decodeStructuredContent(t, tagsResult, &tagsOut)
	if !slices.Equal(tagsOut.Tags, []string{"reading", "golang"}) {
		t.Fatalf("collect_all_tags = %#v", tagsOut.Tags)
	}

	categoriesResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "read_categories",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("read_categories error = %v", err)
	}
	var categoriesOut readCategoriesOutput
	decodeStructuredContent(t, categoriesResult, &categoriesOut)
	if !categoriesOut.Exists || categoriesOut.Categories == nil || categoriesOut.Categories.Version != 1 || len(categoriesOut.Categories.Categories) != 2 {
		t.Fatalf("read_categories = %#v", categoriesOut)
	}

	tagOrderResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "read_tag_order",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("read_tag_order error = %v", err)
	}
	var tagOrderOut readTagOrderOutput
	decodeStructuredContent(t, tagOrderResult, &tagOrderOut)
	if !tagOrderOut.Exists || tagOrderOut.TagOrder == nil || tagOrderOut.TagOrder.Version != 1 || !slices.Equal(tagOrderOut.TagOrder.TagOrder, []string{"reading", "golang"}) {
		t.Fatalf("read_tag_order = %#v", tagOrderOut)
	}
}

func TestSidecarMutationTools(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	bootstrapPDFPath := writeTestPDF(t, root, "draft.pdf")

	server := newTestServer(t, root)
	session := newClientSession(t, server)
	defer session.Close()

	createResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "create_sidecar",
		Arguments: map[string]any{"pdfPath": "book.pdf"},
	})
	if err != nil {
		t.Fatalf("create_sidecar error = %v", err)
	}
	var createOut readMetadataOutput
	decodeStructuredContent(t, createResult, &createOut)
	if !createOut.HasSidecar || createOut.Metadata == nil || createOut.Metadata.Metadata.Title != "book" {
		t.Fatalf("create_sidecar output = %#v", createOut)
	}

	writeResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "write_metadata",
		Arguments: map[string]any{
			"pdfPath": "book.pdf",
			"metadata": map[string]any{
				"metadata": map[string]any{
					"dc:creator": []any{"Ada"},
				},
				"tags":     []any{"go", "mcp"},
				"category": "Reference",
				"reading": map[string]any{
					"lastReadPage": 10,
					"lastReadAt":   "2026-03-26T10:00:00Z",
					"totalPages":   100,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("write_metadata first error = %v", err)
	}
	var writeOut readMetadataOutput
	decodeStructuredContent(t, writeResult, &writeOut)
	if writeOut.Metadata == nil || len(writeOut.Metadata.Metadata.Creator) != 1 || writeOut.Metadata.Metadata.Creator[0] != "Ada" {
		t.Fatalf("write_metadata first output = %#v", writeOut)
	}

	writeRawJSONFile(t, shelff.SidecarPath(pdfPath), `{"schemaVersion":1,"metadata":{"dc:title":"book","dc:creator":["Ada"]},"category":"Reference","tags":["go","mcp"],"reading":{"lastReadPage":10,"lastReadAt":"2026-03-26T10:00:00Z","totalPages":100},"display":{"direction":"RTL"},"x-custom":9007199254740993}`)

	writeResult, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "write_metadata",
		Arguments: map[string]any{
			"pdfPath": "book.pdf",
			"metadata": map[string]any{
				"schemaVersion": nil,
				"metadata": map[string]any{
					"dc:title": nil,
				},
				"category": nil,
				"reading": map[string]any{
					"lastReadAt": nil,
				},
				"display": map[string]any{
					"direction": nil,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("write_metadata second error = %v", err)
	}
	writeOut = readMetadataOutput{}
	decodeStructuredContent(t, writeResult, &writeOut)
	if writeOut.Metadata == nil || writeOut.Metadata.Metadata.Title != "book" {
		t.Fatalf("write_metadata second output = %#v", writeOut)
	}
	if len(writeOut.Metadata.Metadata.Creator) != 1 || writeOut.Metadata.Metadata.Creator[0] != "Ada" {
		t.Fatalf("write_metadata creator = %#v, want preserved", writeOut.Metadata.Metadata.Creator)
	}
	if writeOut.Metadata.SchemaVersion != shelff.SchemaVersion {
		t.Fatalf("write_metadata schemaVersion = %d, want %d", writeOut.Metadata.SchemaVersion, shelff.SchemaVersion)
	}
	if writeOut.Metadata.Category != nil {
		t.Fatalf("write_metadata category = %#v, want nil", writeOut.Metadata.Category)
	}
	if writeOut.Metadata.Reading != nil {
		t.Fatalf("write_metadata reading = %#v, want nil", writeOut.Metadata.Reading)
	}
	if writeOut.Metadata.Display != nil {
		t.Fatalf("write_metadata display = %#v, want nil", writeOut.Metadata.Display)
	}

	rawSidecar := readJSONFileUseNumber(t, shelff.SidecarPath(pdfPath))
	if got := rawSidecar["x-custom"].(json.Number).String(); got != "9007199254740993" {
		t.Fatalf("raw sidecar x-custom = %#v, want exact large integer", rawSidecar["x-custom"])
	}
	if got := rawSidecar["schemaVersion"].(json.Number).String(); got != strconv.Itoa(shelff.SchemaVersion) {
		t.Fatalf("raw sidecar schemaVersion = %#v, want %d", rawSidecar["schemaVersion"], shelff.SchemaVersion)
	}

	writeResult, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "write_metadata",
		Arguments: map[string]any{
			"pdfPath": "draft.pdf",
			"metadata": map[string]any{
				"tags": []any{"bootstrap"},
			},
		},
	})
	if err != nil {
		t.Fatalf("write_metadata bootstrap error = %v", err)
	}
	writeOut = readMetadataOutput{}
	decodeStructuredContent(t, writeResult, &writeOut)
	if writeOut.Metadata == nil || writeOut.Metadata.Metadata.Title != "draft" {
		t.Fatalf("write_metadata bootstrap output = %#v", writeOut)
	}
	if !slices.Equal(writeOut.Metadata.Tags, []string{"bootstrap"}) {
		t.Fatalf("write_metadata bootstrap tags = %#v", writeOut.Metadata.Tags)
	}
	if _, err := os.Stat(shelff.SidecarPath(bootstrapPDFPath)); err != nil {
		t.Fatalf("bootstrap sidecar not written: %v", err)
	}

	deleteResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "delete_sidecar",
		Arguments: map[string]any{"pdfPath": "book.pdf"},
	})
	if err != nil {
		t.Fatalf("delete_sidecar error = %v", err)
	}
	var deleteOut deleteSidecarOutput
	decodeStructuredContent(t, deleteResult, &deleteOut)
	if !deleteOut.Deleted {
		t.Fatalf("delete_sidecar output = %#v, want deleted=true", deleteOut)
	}
	if _, err := os.Stat(shelff.SidecarPath(pdfPath)); !os.IsNotExist(err) {
		t.Fatalf("sidecar still exists after delete_sidecar: %v", err)
	}

	deleteResult, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "delete_sidecar",
		Arguments: map[string]any{"pdfPath": "book.pdf"},
	})
	if err != nil {
		t.Fatalf("delete_sidecar second error = %v", err)
	}
	deleteOut = deleteSidecarOutput{}
	decodeStructuredContent(t, deleteResult, &deleteOut)
	if deleteOut.Deleted {
		t.Fatalf("second delete_sidecar output = %#v, want deleted=false", deleteOut)
	}
}

func TestBookAndConfigMutationTools(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	incomingDir := filepath.Join(root, "incoming")
	destDir := filepath.Join(root, "shelf")
	if err := os.MkdirAll(incomingDir, 0o755); err != nil {
		t.Fatalf("MkdirAll incoming error = %v", err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatalf("MkdirAll shelf error = %v", err)
	}

	pdfPath := writeTestPDF(t, incomingDir, "book.pdf")
	rootMovePDFPath := writeTestPDF(t, incomingDir, "root-move.pdf")
	sidecar, err := shelff.CreateSidecar(pdfPath)
	if err != nil {
		t.Fatalf("CreateSidecar error = %v", err)
	}
	category := "Reference"
	sidecar.Category = &category
	sidecar.Tags = []string{"go", "mcp"}
	if err := shelff.WriteSidecar(pdfPath, sidecar); err != nil {
		t.Fatalf("WriteSidecar error = %v", err)
	}

	server := newTestServer(t, root)
	session := newClientSession(t, server)
	defer session.Close()

	moveResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "move_book",
		Arguments: map[string]any{
			"pdfPath": "incoming/book.pdf",
			"destDir": "shelf",
		},
	})
	if err != nil {
		t.Fatalf("move_book error = %v", err)
	}
	var bookOut bookPathOutput
	decodeStructuredContent(t, moveResult, &bookOut)
	if bookOut.PDFPath != "shelf/book.pdf" {
		t.Fatalf("move_book output = %#v, want shelf/book.pdf", bookOut)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(bookOut.PDFPath))); err != nil {
		t.Fatalf("moved PDF missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "incoming", "book.pdf")); !os.IsNotExist(err) {
		t.Fatalf("source PDF still exists after move_book: %v", err)
	}

	renameResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "rename_book",
		Arguments: map[string]any{
			"pdfPath": bookOut.PDFPath,
			"newName": "renamed.pdf",
		},
	})
	if err != nil {
		t.Fatalf("rename_book error = %v", err)
	}
	bookOut = bookPathOutput{}
	decodeStructuredContent(t, renameResult, &bookOut)
	if bookOut.PDFPath != "shelf/renamed.pdf" {
		t.Fatalf("rename_book output = %#v, want shelf/renamed.pdf", bookOut)
	}
	renamedPDFPath := filepath.Join(root, filepath.FromSlash(bookOut.PDFPath))
	if _, err := os.Stat(renamedPDFPath); err != nil {
		t.Fatalf("renamed PDF missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "shelf", "book.pdf")); !os.IsNotExist(err) {
		t.Fatalf("old PDF still exists after rename_book: %v", err)
	}
	if _, err := os.Stat(shelff.SidecarPath(renamedPDFPath)); err != nil {
		t.Fatalf("renamed sidecar missing: %v", err)
	}

	rootMoveResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "move_book",
		Arguments: map[string]any{
			"pdfPath": "incoming/root-move.pdf",
			"destDir": ".",
		},
	})
	if err != nil {
		t.Fatalf("move_book to root error = %v", err)
	}
	bookOut = bookPathOutput{}
	decodeStructuredContent(t, rootMoveResult, &bookOut)
	if bookOut.PDFPath != "root-move.pdf" {
		t.Fatalf("move_book to root output = %#v, want root-move.pdf", bookOut)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(bookOut.PDFPath))); err != nil {
		t.Fatalf("root-moved PDF missing: %v", err)
	}
	if _, err := os.Stat(rootMovePDFPath); !os.IsNotExist(err) {
		t.Fatalf("source PDF still exists after root move_book: %v", err)
	}

	addCategoryResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "add_category",
		Arguments: map[string]any{"name": " Reference "},
	})
	if err != nil {
		t.Fatalf("add_category error = %v", err)
	}
	var categoriesOut readCategoriesOutput
	decodeStructuredContent(t, addCategoryResult, &categoriesOut)
	if !categoriesOut.Exists || categoriesOut.Categories == nil || len(categoriesOut.Categories.Categories) != 1 || categoriesOut.Categories.Categories[0].Name != "Reference" {
		t.Fatalf("add_category output = %#v", categoriesOut)
	}

	renameCategoryResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "rename_category",
		Arguments: map[string]any{
			"oldName": "Reference",
			"newName": "Docs",
			"cascade": true,
		},
	})
	if err != nil {
		t.Fatalf("rename_category error = %v", err)
	}
	categoriesOut = readCategoriesOutput{}
	decodeStructuredContent(t, renameCategoryResult, &categoriesOut)
	if !categoriesOut.Exists || categoriesOut.Categories == nil || len(categoriesOut.Categories.Categories) != 1 || categoriesOut.Categories.Categories[0].Name != "Docs" {
		t.Fatalf("rename_category output = %#v", categoriesOut)
	}
	renamedSidecar, err := shelff.ReadSidecar(renamedPDFPath)
	if err != nil {
		t.Fatalf("ReadSidecar after rename_category error = %v", err)
	}
	if renamedSidecar == nil || renamedSidecar.Category == nil || *renamedSidecar.Category != "Docs" {
		t.Fatalf("renamed sidecar category = %#v, want Docs", renamedSidecar)
	}

	addCategoryResult, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "add_category",
		Arguments: map[string]any{"name": "Archive"},
	})
	if err != nil {
		t.Fatalf("second add_category error = %v", err)
	}
	decodeStructuredContent(t, addCategoryResult, &categoriesOut)
	if !categoriesOut.Exists || categoriesOut.Categories == nil || len(categoriesOut.Categories.Categories) != 2 {
		t.Fatalf("second add_category output = %#v", categoriesOut)
	}

	reorderCategoriesResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "reorder_categories",
		Arguments: map[string]any{"names": []any{"Archive", "Docs"}},
	})
	if err != nil {
		t.Fatalf("reorder_categories error = %v", err)
	}
	categoriesOut = readCategoriesOutput{}
	decodeStructuredContent(t, reorderCategoriesResult, &categoriesOut)
	if !categoriesOut.Exists || categoriesOut.Categories == nil || len(categoriesOut.Categories.Categories) != 2 || categoriesOut.Categories.Categories[0].Name != "Archive" || categoriesOut.Categories.Categories[1].Name != "Docs" {
		t.Fatalf("reorder_categories output = %#v", categoriesOut)
	}

	removeCategoryResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "remove_category",
		Arguments: map[string]any{
			"name":    "Docs",
			"cascade": true,
		},
	})
	if err != nil {
		t.Fatalf("remove_category error = %v", err)
	}
	categoriesOut = readCategoriesOutput{}
	decodeStructuredContent(t, removeCategoryResult, &categoriesOut)
	if !categoriesOut.Exists || categoriesOut.Categories == nil || len(categoriesOut.Categories.Categories) != 1 || categoriesOut.Categories.Categories[0].Name != "Archive" {
		t.Fatalf("remove_category output = %#v", categoriesOut)
	}
	renamedSidecar, err = shelff.ReadSidecar(renamedPDFPath)
	if err != nil {
		t.Fatalf("ReadSidecar after remove_category error = %v", err)
	}
	if renamedSidecar == nil || renamedSidecar.Category != nil {
		t.Fatalf("sidecar category after remove_category = %#v, want nil", renamedSidecar)
	}

	addTagResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "add_tag_to_order",
		Arguments: map[string]any{"name": " go "},
	})
	if err != nil {
		t.Fatalf("add_tag_to_order error = %v", err)
	}
	var tagOrderOut readTagOrderOutput
	decodeStructuredContent(t, addTagResult, &tagOrderOut)
	if !tagOrderOut.Exists || tagOrderOut.TagOrder == nil || !slices.Equal(tagOrderOut.TagOrder.TagOrder, []string{"go"}) {
		t.Fatalf("add_tag_to_order output = %#v", tagOrderOut)
	}

	addTagResult, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "add_tag_to_order",
		Arguments: map[string]any{"name": "mcp"},
	})
	if err != nil {
		t.Fatalf("second add_tag_to_order error = %v", err)
	}
	decodeStructuredContent(t, addTagResult, &tagOrderOut)
	if !tagOrderOut.Exists || tagOrderOut.TagOrder == nil || !slices.Equal(tagOrderOut.TagOrder.TagOrder, []string{"go", "mcp"}) {
		t.Fatalf("second add_tag_to_order output = %#v", tagOrderOut)
	}

	renameTagResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "rename_tag",
		Arguments: map[string]any{
			"oldName": "go",
			"newName": "golang",
			"cascade": true,
		},
	})
	if err != nil {
		t.Fatalf("rename_tag error = %v", err)
	}
	tagOrderOut = readTagOrderOutput{}
	decodeStructuredContent(t, renameTagResult, &tagOrderOut)
	if !tagOrderOut.Exists || tagOrderOut.TagOrder == nil || !slices.Equal(tagOrderOut.TagOrder.TagOrder, []string{"golang", "mcp"}) {
		t.Fatalf("rename_tag output = %#v", tagOrderOut)
	}
	renamedSidecar, err = shelff.ReadSidecar(renamedPDFPath)
	if err != nil {
		t.Fatalf("ReadSidecar after rename_tag error = %v", err)
	}
	if renamedSidecar == nil || !slices.Equal(renamedSidecar.Tags, []string{"golang", "mcp"}) {
		t.Fatalf("sidecar tags after rename_tag = %#v", renamedSidecar)
	}

	reorderTagsResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "reorder_tags",
		Arguments: map[string]any{"names": []any{"mcp", " golang ", "extra"}},
	})
	if err != nil {
		t.Fatalf("reorder_tags error = %v", err)
	}
	tagOrderOut = readTagOrderOutput{}
	decodeStructuredContent(t, reorderTagsResult, &tagOrderOut)
	if !tagOrderOut.Exists || tagOrderOut.TagOrder == nil || !slices.Equal(tagOrderOut.TagOrder.TagOrder, []string{"mcp", "golang", "extra"}) {
		t.Fatalf("reorder_tags output = %#v", tagOrderOut)
	}

	removeTagResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "remove_tag_from_order",
		Arguments: map[string]any{
			"name":    "mcp",
			"cascade": true,
		},
	})
	if err != nil {
		t.Fatalf("remove_tag_from_order error = %v", err)
	}
	tagOrderOut = readTagOrderOutput{}
	decodeStructuredContent(t, removeTagResult, &tagOrderOut)
	if !tagOrderOut.Exists || tagOrderOut.TagOrder == nil || !slices.Equal(tagOrderOut.TagOrder.TagOrder, []string{"golang", "extra"}) {
		t.Fatalf("remove_tag_from_order output = %#v", tagOrderOut)
	}
	renamedSidecar, err = shelff.ReadSidecar(renamedPDFPath)
	if err != nil {
		t.Fatalf("ReadSidecar after remove_tag_from_order error = %v", err)
	}
	if renamedSidecar == nil || !slices.Equal(renamedSidecar.Tags, []string{"golang"}) {
		t.Fatalf("sidecar tags after remove_tag_from_order = %#v", renamedSidecar)
	}
}

func TestReadCategoriesAndTagOrderExistsField(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library, err := shelff.OpenLibrary(root)
	if err != nil {
		t.Fatalf("OpenLibrary error = %v", err)
	}

	server := newTestServer(t, root)
	session := newClientSession(t, server)
	defer session.Close()

	// File missing: exists should be false
	catResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "read_categories",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("read_categories (missing) error = %v", err)
	}
	var catOut readCategoriesOutput
	decodeStructuredContent(t, catResult, &catOut)
	if catOut.Exists {
		t.Fatalf("read_categories (missing) exists = true, want false")
	}
	if catOut.Categories != nil {
		t.Fatalf("read_categories (missing) categories = %#v, want nil", catOut.Categories)
	}

	tagResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "read_tag_order",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("read_tag_order (missing) error = %v", err)
	}
	var tagOut readTagOrderOutput
	decodeStructuredContent(t, tagResult, &tagOut)
	if tagOut.Exists {
		t.Fatalf("read_tag_order (missing) exists = true, want false")
	}
	if tagOut.TagOrder != nil {
		t.Fatalf("read_tag_order (missing) tagOrder = %#v, want nil", tagOut.TagOrder)
	}

	// Create files, then exists should be true
	if err := library.WriteCategories(&shelff.CategoryList{
		Version:    1,
		Categories: []shelff.CategoryItem{{Name: "小説", Order: 0}},
	}); err != nil {
		t.Fatalf("WriteCategories error = %v", err)
	}
	if err := library.WriteTagOrder(&shelff.TagOrder{
		Version:  1,
		TagOrder: []string{"Go"},
	}); err != nil {
		t.Fatalf("WriteTagOrder error = %v", err)
	}

	catResult, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "read_categories",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("read_categories (exists) error = %v", err)
	}
	catOut = readCategoriesOutput{}
	decodeStructuredContent(t, catResult, &catOut)
	if !catOut.Exists {
		t.Fatalf("read_categories (exists) exists = false, want true")
	}
	if catOut.Categories == nil || len(catOut.Categories.Categories) != 1 {
		t.Fatalf("read_categories (exists) categories = %#v", catOut.Categories)
	}

	tagResult, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "read_tag_order",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("read_tag_order (exists) error = %v", err)
	}
	tagOut = readTagOrderOutput{}
	decodeStructuredContent(t, tagResult, &tagOut)
	if !tagOut.Exists {
		t.Fatalf("read_tag_order (exists) exists = false, want true")
	}
	if tagOut.TagOrder == nil || !slices.Equal(tagOut.TagOrder.TagOrder, []string{"Go"}) {
		t.Fatalf("read_tag_order (exists) tagOrder = %#v", tagOut.TagOrder)
	}
}

func TestCheckLibraryTool(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library, err := shelff.OpenLibrary(root)
	if err != nil {
		t.Fatalf("OpenLibrary error = %v", err)
	}

	// Setup: categories + tags + books + orphan
	if err := library.WriteCategories(&shelff.CategoryList{
		Version:    1,
		Categories: []shelff.CategoryItem{{Name: "小説", Order: 0}},
	}); err != nil {
		t.Fatalf("WriteCategories error = %v", err)
	}
	if err := library.WriteTagOrder(&shelff.TagOrder{
		Version:  1,
		TagOrder: []string{"Go"},
	}); err != nil {
		t.Fatalf("WriteTagOrder error = %v", err)
	}

	pdfPath := writeTestPDF(t, root, "book.pdf")
	meta, err := shelff.CreateSidecar(pdfPath)
	if err != nil {
		t.Fatalf("CreateSidecar error = %v", err)
	}
	cat := "SF"
	meta.Category = &cat
	meta.Tags = []string{"Go", "Swift"}
	if err := shelff.WriteSidecar(pdfPath, meta); err != nil {
		t.Fatalf("WriteSidecar error = %v", err)
	}

	writeTestPDF(t, root, "nosidecar.pdf")

	server := newTestServer(t, root)
	session := newClientSession(t, server)
	defer session.Close()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "check_library",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("check_library error = %v", err)
	}

	var out shelff.CheckLibraryResult
	decodeStructuredContent(t, result, &out)

	if !out.DotShelff.Exists || !out.DotShelff.CategoriesJSON || !out.DotShelff.TagsJSON {
		t.Fatalf("dotShelff = %#v, want all true", out.DotShelff)
	}
	if !slices.Equal(out.Integrity.UndefinedCategories, []string{"SF"}) {
		t.Fatalf("undefinedCategories = %v, want [SF]", out.Integrity.UndefinedCategories)
	}
	if !slices.Equal(out.Integrity.UndefinedTags, []string{"Swift"}) {
		t.Fatalf("undefinedTags = %v, want [Swift]", out.Integrity.UndefinedTags)
	}
	if !slices.Equal(out.Integrity.UnusedCategories, []string{"小説"}) {
		t.Fatalf("unusedCategories = %v, want [小説]", out.Integrity.UnusedCategories)
	}
	if out.Summary.TotalPDFs != 2 || out.Summary.WithSidecar != 1 || out.Summary.WithoutSidecar != 1 {
		t.Fatalf("summary = %#v", out.Summary)
	}
}

func TestReadOnlyToolsRejectPathTraversal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	writeTestPDF(t, root, "inside.pdf")
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("os.Symlink unavailable: %v", err)
	}

	server := newTestServer(t, root)
	session := newClientSession(t, server)
	defer session.Close()

	assertToolTraversalError(t, session, "read_metadata", map[string]any{"pdfPath": "../outside.pdf"})
	assertToolTraversalError(t, session, "read_metadata", map[string]any{"pdfPath": "escape/outside.pdf"})
	assertToolTraversalError(t, session, "scan_books", map[string]any{"recursive": true, "directory": "../outside"})
	assertToolTraversalError(t, session, "scan_books", map[string]any{"recursive": true, "directory": "escape"})
	assertToolTraversalError(t, session, "rename_book", map[string]any{"pdfPath": "../outside.pdf", "newName": "renamed"})
	assertToolTraversalError(t, session, "rename_book", map[string]any{"pdfPath": "escape/outside.pdf", "newName": "renamed"})
	assertToolTraversalError(t, session, "move_book", map[string]any{"pdfPath": "inside.pdf", "destDir": "../outside"})
	assertToolTraversalError(t, session, "move_book", map[string]any{"pdfPath": "inside.pdf", "destDir": "escape"})
}

func TestScanBooksSupportsDirectoryFilteringAndPagination(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	nestedDir := filepath.Join(root, "nested")
	deepDir := filepath.Join(nestedDir, "deep")
	otherDir := filepath.Join(root, "other")
	for _, dir := range []string{nestedDir, deepDir, otherDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", dir, err)
		}
	}

	writeTestPDF(t, root, "top.pdf")
	writeTestPDF(t, nestedDir, "a.pdf")
	writeTestPDF(t, nestedDir, "b.pdf")
	writeTestPDF(t, deepDir, "c.pdf")
	writeTestPDF(t, otherDir, "outside.pdf")

	server := newTestServer(t, root)
	session := newClientSession(t, server)
	defer session.Close()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "scan_books",
		Arguments: map[string]any{
			"recursive": true,
			"directory": "nested",
			"offset":    1,
			"limit":     1,
		},
	})
	if err != nil {
		t.Fatalf("scan_books with directory/pagination error = %v", err)
	}

	var out scanBooksOutput
	decodeStructuredContent(t, result, &out)
	if out.Total != 3 || out.Offset != 1 || out.Limit != 1 || !out.HasMore {
		t.Fatalf("scan_books page info = %#v, want total=3 offset=1 limit=1 hasMore=true", out)
	}
	if len(out.Books) != 1 || out.Books[0].PDFPath != "nested/b.pdf" {
		t.Fatalf("scan_books books = %#v, want nested/b.pdf only", out.Books)
	}
}

func TestScanBooksRejectsInvalidPagination(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestPDF(t, root, "book.pdf")

	server := newTestServer(t, root)
	session := newClientSession(t, server)
	defer session.Close()

	assertToolErrorContains(t, session, "scan_books", map[string]any{"recursive": true, "limit": 0}, ErrInvalidLimit.Error())
	assertToolErrorContains(t, session, "scan_books", map[string]any{"recursive": true, "offset": -1}, ErrInvalidOffset.Error())
}

func TestListDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	// Create directory structure:
	//   root/
	//     a/
	//       nested/
	//     b/
	//     .shelff/   (config dir, should be excluded)
	for _, dir := range []string{"a", "a/nested", "b", ".shelff"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("MkdirAll error = %v", err)
		}
	}
	// Also place a file to ensure files are not listed
	writeTestPDF(t, root, "book.pdf")

	server := newTestServer(t, root)
	session := newClientSession(t, server)
	defer session.Close()

	// Non-recursive: only top-level directories
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_directories",
		Arguments: map[string]any{"recursive": false},
	})
	if err != nil {
		t.Fatalf("list_directories error = %v", err)
	}
	var out listDirectoriesOutput
	decodeStructuredContent(t, result, &out)
	slices.Sort(out.Directories)
	if want := []string{"a", "b"}; !slices.Equal(out.Directories, want) {
		t.Fatalf("list_directories non-recursive = %v, want %v", out.Directories, want)
	}

	// Recursive: all directories including nested
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_directories",
		Arguments: map[string]any{"recursive": true},
	})
	if err != nil {
		t.Fatalf("list_directories recursive error = %v", err)
	}
	out = listDirectoriesOutput{}
	decodeStructuredContent(t, result, &out)
	slices.Sort(out.Directories)
	if want := []string{"a", "a/nested", "b"}; !slices.Equal(out.Directories, want) {
		t.Fatalf("list_directories recursive = %v, want %v", out.Directories, want)
	}

	// directory: "" should behave like listing from the root
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_directories",
		Arguments: map[string]any{"recursive": false, "directory": ""},
	})
	if err != nil {
		t.Fatalf("list_directories with empty directory error = %v", err)
	}
	out = listDirectoriesOutput{}
	decodeStructuredContent(t, result, &out)
	slices.Sort(out.Directories)
	if want := []string{"a", "b"}; !slices.Equal(out.Directories, want) {
		t.Fatalf("list_directories with empty directory = %v, want %v", out.Directories, want)
	}

	// directory: "a" should return only entries under "a"
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_directories",
		Arguments: map[string]any{"recursive": true, "directory": "a"},
	})
	if err != nil {
		t.Fatalf("list_directories with directory=a error = %v", err)
	}
	out = listDirectoriesOutput{}
	decodeStructuredContent(t, result, &out)
	if want := []string{"a/nested"}; !slices.Equal(out.Directories, want) {
		t.Fatalf("list_directories with directory=a = %v, want %v", out.Directories, want)
	}

	// directory: ".shelff" should return empty list
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_directories",
		Arguments: map[string]any{"recursive": true, "directory": ".shelff"},
	})
	if err != nil {
		t.Fatalf("list_directories with directory=.shelff error = %v", err)
	}
	out = listDirectoriesOutput{}
	decodeStructuredContent(t, result, &out)
	if len(out.Directories) != 0 {
		t.Fatalf("list_directories with directory=.shelff = %v, want empty", out.Directories)
	}
}

func TestCreateDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	server := newTestServer(t, root)
	session := newClientSession(t, server)
	defer session.Close()

	// Create a simple directory
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "create_directory",
		Arguments: map[string]any{"path": "shelf-a"},
	})
	if err != nil {
		t.Fatalf("create_directory error = %v", err)
	}
	var out createDirectoryOutput
	decodeStructuredContent(t, result, &out)
	if out.Path != "shelf-a" {
		t.Fatalf("create_directory output = %#v, want shelf-a", out)
	}
	info, err := os.Stat(filepath.Join(root, "shelf-a"))
	if err != nil || !info.IsDir() {
		t.Fatalf("directory not created: %v", err)
	}

	// Create nested directories
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "create_directory",
		Arguments: map[string]any{"path": "shelf-b/sub"},
	})
	if err != nil {
		t.Fatalf("create_directory nested error = %v", err)
	}
	out = createDirectoryOutput{}
	decodeStructuredContent(t, result, &out)
	if out.Path != "shelf-b/sub" {
		t.Fatalf("create_directory nested output = %#v, want shelf-b/sub", out)
	}
	info, err = os.Stat(filepath.Join(root, "shelf-b", "sub"))
	if err != nil || !info.IsDir() {
		t.Fatalf("nested directory not created: %v", err)
	}

	// Idempotent: creating an existing directory succeeds
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "create_directory",
		Arguments: map[string]any{"path": "shelf-a"},
	})
	if err != nil {
		t.Fatalf("create_directory idempotent error = %v", err)
	}
	if result.IsError {
		t.Fatalf("create_directory idempotent IsError = true")
	}
}

func TestCreateDirectoryRejectsPathTraversal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("os.Symlink unavailable: %v", err)
	}

	server := newTestServer(t, root)
	session := newClientSession(t, server)
	defer session.Close()

	assertToolTraversalError(t, session, "create_directory", map[string]any{"path": "../outside"})
	assertToolTraversalError(t, session, "create_directory", map[string]any{"path": "escape/sub"})
}

func TestListDirectoriesRejectsPathTraversal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("os.Symlink unavailable: %v", err)
	}

	server := newTestServer(t, root)
	session := newClientSession(t, server)
	defer session.Close()

	assertToolTraversalError(t, session, "list_directories", map[string]any{"recursive": true, "directory": "../outside"})
	assertToolTraversalError(t, session, "list_directories", map[string]any{"recursive": true, "directory": "escape"})
}

func TestNewRejectsMissingRoot(t *testing.T) {
	t.Parallel()

	_, err := New("")
	if !errors.Is(err, ErrRootNotProvided) {
		t.Fatalf("New(\"\") error = %v, want ErrRootNotProvided", err)
	}
}

func newTestServer(t *testing.T, root string) *Server {
	t.Helper()

	server, err := New(root)
	if err != nil {
		t.Fatalf("New(%q) error = %v", root, err)
	}
	return server
}

func newClientSession(t *testing.T, server *Server) *mcp.ClientSession {
	t.Helper()

	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.MCP().Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server Connect error = %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client Connect error = %v", err)
	}
	return session
}

func decodeStructuredContent(t *testing.T, result *mcp.CallToolResult, out any) {
	t.Helper()

	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("Marshal structured content error = %v", err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("Unmarshal structured content error = %v", err)
	}
}

func assertToolTraversalError(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) {
	t.Helper()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      name,
		Arguments: arguments,
	})
	if err != nil {
		t.Fatalf("CallTool(%q) error = %v", name, err)
	}
	if !result.IsError {
		t.Fatalf("CallTool(%q) IsError = false, want true", name)
	}
	if len(result.Content) == 0 {
		t.Fatalf("CallTool(%q) returned no error content", name)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(text.Text, ErrPathTraversal.Error()) {
		t.Fatalf("CallTool(%q) content = %#v, want traversal error", name, result.Content[0])
	}
}

func assertToolErrorContains(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any, want string) {
	t.Helper()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      name,
		Arguments: arguments,
	})
	if err != nil {
		t.Fatalf("CallTool(%q) error = %v", name, err)
	}
	if !result.IsError {
		t.Fatalf("CallTool(%q) IsError = false, want true", name)
	}
	if len(result.Content) == 0 {
		t.Fatalf("CallTool(%q) returned no error content", name)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(text.Text, want) {
		t.Fatalf("CallTool(%q) content = %#v, want %q", name, result.Content[0], want)
	}
}

func readJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", path, err)
	}
	return decoded
}

func readJSONFileUseNumber(t *testing.T, path string) map[string]any {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	var decoded map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatalf("Decode(%q) error = %v", path, err)
	}
	return decoded
}

func writeTestPDF(t *testing.T, dir string, name string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("%PDF-1.7\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
	return path
}

func writeRawJSONFile(t *testing.T, path string, contents string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
