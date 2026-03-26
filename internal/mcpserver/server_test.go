package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skoji/shelff-go/shelff"
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
		Name:      "read_sidecar",
		Arguments: map[string]any{"pdfPath": "book.pdf"},
	})
	if err != nil {
		t.Fatalf("read_sidecar error = %v", err)
	}
	var readOut readSidecarOutput
	decodeStructuredContent(t, readResult, &readOut)
	if !readOut.Exists || readOut.Sidecar == nil || readOut.Sidecar.Metadata.Title != "book" {
		t.Fatalf("read_sidecar output = %#v", readOut)
	}

	missingReadResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "read_sidecar",
		Arguments: map[string]any{"pdfPath": "nested/draft.pdf"},
	})
	if err != nil {
		t.Fatalf("read_sidecar missing error = %v", err)
	}
	var missingReadOut readSidecarOutput
	decodeStructuredContent(t, missingReadResult, &missingReadOut)
	if missingReadOut.Exists || missingReadOut.Sidecar != nil {
		t.Fatalf("missing read_sidecar output = %#v, want exists=false sidecar=nil", missingReadOut)
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
	var categoriesOut shelff.CategoryList
	decodeStructuredContent(t, categoriesResult, &categoriesOut)
	if categoriesOut.Version != 1 || len(categoriesOut.Categories) != 2 {
		t.Fatalf("read_categories = %#v", categoriesOut)
	}

	tagOrderResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "read_tag_order",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("read_tag_order error = %v", err)
	}
	var tagOrderOut shelff.TagOrder
	decodeStructuredContent(t, tagOrderResult, &tagOrderOut)
	if tagOrderOut.Version != 1 || !slices.Equal(tagOrderOut.TagOrder, []string{"reading", "golang"}) {
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
	var createOut readSidecarOutput
	decodeStructuredContent(t, createResult, &createOut)
	if !createOut.Exists || createOut.Sidecar == nil || createOut.Sidecar.Metadata.Title != "book" {
		t.Fatalf("create_sidecar output = %#v", createOut)
	}

	writeResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "write_sidecar",
		Arguments: map[string]any{
			"pdfPath": "book.pdf",
			"sidecar": map[string]any{
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
		t.Fatalf("write_sidecar first error = %v", err)
	}
	var writeOut readSidecarOutput
	decodeStructuredContent(t, writeResult, &writeOut)
	if writeOut.Sidecar == nil || len(writeOut.Sidecar.Metadata.Creator) != 1 || writeOut.Sidecar.Metadata.Creator[0] != "Ada" {
		t.Fatalf("write_sidecar first output = %#v", writeOut)
	}

	writeRawJSONFile(t, shelff.SidecarPath(pdfPath), `{"schemaVersion":1,"metadata":{"dc:title":"book","dc:creator":["Ada"]},"tags":["go","mcp"],"x-custom":9007199254740993}`)

	writeResult, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "write_sidecar",
		Arguments: map[string]any{
			"pdfPath": "book.pdf",
			"sidecar": map[string]any{
				"schemaVersion": nil,
				"metadata": map[string]any{
					"dc:title": nil,
				},
				"category": nil,
			},
		},
	})
	if err != nil {
		t.Fatalf("write_sidecar second error = %v", err)
	}
	writeOut = readSidecarOutput{}
	decodeStructuredContent(t, writeResult, &writeOut)
	if writeOut.Sidecar == nil || writeOut.Sidecar.Metadata.Title != "book" {
		t.Fatalf("write_sidecar second output = %#v", writeOut)
	}
	if len(writeOut.Sidecar.Metadata.Creator) != 1 || writeOut.Sidecar.Metadata.Creator[0] != "Ada" {
		t.Fatalf("write_sidecar creator = %#v, want preserved", writeOut.Sidecar.Metadata.Creator)
	}
	if writeOut.Sidecar.SchemaVersion != shelff.SchemaVersion {
		t.Fatalf("write_sidecar schemaVersion = %d, want %d", writeOut.Sidecar.SchemaVersion, shelff.SchemaVersion)
	}
	if writeOut.Sidecar.Category != nil {
		t.Fatalf("write_sidecar category = %#v, want nil", writeOut.Sidecar.Category)
	}

	rawSidecar := readJSONFileUseNumber(t, shelff.SidecarPath(pdfPath))
	if got := rawSidecar["x-custom"].(json.Number).String(); got != "9007199254740993" {
		t.Fatalf("raw sidecar x-custom = %#v, want exact large integer", rawSidecar["x-custom"])
	}
	if got := rawSidecar["schemaVersion"].(json.Number).String(); got != "1" {
		t.Fatalf("raw sidecar schemaVersion = %#v, want 1", rawSidecar["schemaVersion"])
	}

	writeResult, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "write_sidecar",
		Arguments: map[string]any{
			"pdfPath": "draft.pdf",
			"sidecar": map[string]any{
				"tags": []any{"bootstrap"},
			},
		},
	})
	if err != nil {
		t.Fatalf("write_sidecar bootstrap error = %v", err)
	}
	writeOut = readSidecarOutput{}
	decodeStructuredContent(t, writeResult, &writeOut)
	if writeOut.Sidecar == nil || writeOut.Sidecar.Metadata.Title != "draft" {
		t.Fatalf("write_sidecar bootstrap output = %#v", writeOut)
	}
	if !slices.Equal(writeOut.Sidecar.Tags, []string{"bootstrap"}) {
		t.Fatalf("write_sidecar bootstrap tags = %#v", writeOut.Sidecar.Tags)
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

func TestReadOnlyToolsRejectPathTraversal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("os.Symlink unavailable: %v", err)
	}

	server := newTestServer(t, root)
	session := newClientSession(t, server)
	defer session.Close()

	for _, pdfPath := range []string{"../outside.pdf", "escape/outside.pdf"} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name:      "read_sidecar",
			Arguments: map[string]any{"pdfPath": pdfPath},
		})
		if err != nil {
			t.Fatalf("CallTool(%q) error = %v", pdfPath, err)
		}
		if !result.IsError {
			t.Fatalf("CallTool(%q) IsError = false, want true", pdfPath)
		}
		if len(result.Content) == 0 {
			t.Fatalf("CallTool(%q) returned no error content", pdfPath)
		}
		text, ok := result.Content[0].(*mcp.TextContent)
		if !ok || !strings.Contains(text.Text, ErrPathTraversal.Error()) {
			t.Fatalf("CallTool(%q) content = %#v, want traversal error", pdfPath, result.Content[0])
		}
	}
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
