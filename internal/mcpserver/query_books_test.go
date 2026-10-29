package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skoji/shelff-mcp/shelff"
)

func setupQueryBooksRoot(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	subDir := filepath.Join(root, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", subDir, err)
	}

	writeRawJSONFile(t, shelff.SidecarPath(writeTestPDF(t, root, "a.pdf")), `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "Book A", "dc:creator": ["Ada"]},
  "reading": {"lastReadPage": 1, "lastReadAt": "2026-01-01T00:00:00Z", "totalPages": 10, "status": "reading"},
  "category": "Fiction",
  "tags": ["fantasy"],
  "collection": {"title": "Series", "position": 2},
  "archive": {"isArchived": false, "changedAt": "2026-01-02T00:00:00Z"}
}`)
	writeRawJSONFile(t, shelff.SidecarPath(writeTestPDF(t, root, "b.pdf")), `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "Book B"}
}`)
	writeRawJSONFile(t, shelff.SidecarPath(writeTestPDF(t, subDir, "c.pdf")), `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "Book C"},
  "category": "Fiction"
}`)
	writeRawJSONFile(t, shelff.SidecarPath(writeTestPDF(t, root, "broken.pdf")), `{not json`)
	writeTestPDF(t, root, "plain.pdf")
	return root
}

func callQueryBooks(t *testing.T, session *mcp.ClientSession, arguments map[string]any) map[string]any {
	t.Helper()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "query_books",
		Arguments: arguments,
	})
	if err != nil {
		t.Fatalf("query_books error = %v", err)
	}
	if result.IsError {
		t.Fatalf("query_books returned tool error: %#v", result.Content[0])
	}
	var out map[string]any
	decodeStructuredContent(t, result, &out)
	return out
}

func queryBooksPDFPaths(t *testing.T, out map[string]any) []string {
	t.Helper()

	books, ok := out["books"].([]any)
	if !ok {
		t.Fatalf("query_books books = %#v, want array", out["books"])
	}
	paths := make([]string, 0, len(books))
	for _, book := range books {
		paths = append(paths, book.(map[string]any)["pdfPath"].(string))
	}
	return paths
}

func TestQueryBooksIsReadOnlyTool(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, t.TempDir())
	session := newClientSession(t, server)
	defer session.Close()

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools error = %v", err)
	}
	for _, tool := range result.Tools {
		if tool.Name != "query_books" {
			continue
		}
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Fatalf("query_books annotations = %#v, want ReadOnlyHint", tool.Annotations)
		}
		return
	}
	t.Fatalf("query_books is not registered")
}

func TestQueryBooksReturnsSummaries(t *testing.T) {
	t.Parallel()

	root := setupQueryBooksRoot(t)
	server := newTestServer(t, root)
	session := newClientSession(t, server)
	defer session.Close()

	out := callQueryBooks(t, session, map[string]any{})

	if got, want := queryBooksPDFPaths(t, out), []string{"a.pdf", "b.pdf", "sub/c.pdf"}; !slices.Equal(got, want) {
		t.Fatalf("pdfPaths = %v, want %v (recursive defaults to true, books without sidecar excluded)", got, want)
	}
	if out["total"] != float64(3) || out["offset"] != float64(0) || out["limit"] != float64(defaultScanBooksLimit) || out["hasMore"] != false {
		t.Fatalf("page info = %#v", out)
	}
	warnings, ok := out["warnings"].([]any)
	if !ok || len(warnings) != 1 {
		t.Fatalf("warnings = %#v, want one warning", out["warnings"])
	}

	books := out["books"].([]any)
	a := books[0].(map[string]any)
	if a["sidecarPath"] != "a.pdf.meta.json" || a["title"] != "Book A" || a["category"] != "Fiction" || a["status"] != "reading" || a["archived"] != false {
		t.Fatalf("a = %#v", a)
	}
	if creators, ok := a["creators"].([]any); !ok || len(creators) != 1 || creators[0] != "Ada" {
		t.Fatalf("a creators = %#v", a["creators"])
	}
	if tags, ok := a["tags"].([]any); !ok || len(tags) != 1 || tags[0] != "fantasy" {
		t.Fatalf("a tags = %#v", a["tags"])
	}
	collection, ok := a["collection"].(map[string]any)
	if !ok || collection["title"] != "Series" || collection["position"] != float64(2) {
		t.Fatalf("a collection = %#v", a["collection"])
	}

	b := books[1].(map[string]any)
	for _, key := range []string{"creators", "category", "tags", "status", "collection", "archived"} {
		if _, present := b[key]; present {
			t.Fatalf("b has %q = %#v, want omitted", key, b[key])
		}
	}
	if b["pdfPath"] != "b.pdf" || b["sidecarPath"] != "b.pdf.meta.json" || b["title"] != "Book B" {
		t.Fatalf("b = %#v", b)
	}
}

func TestQueryBooksFiltersAndPaginates(t *testing.T) {
	t.Parallel()

	root := setupQueryBooksRoot(t)
	server := newTestServer(t, root)
	session := newClientSession(t, server)
	defer session.Close()

	out := callQueryBooks(t, session, map[string]any{"category": "Fiction", "limit": 1})
	if got, want := queryBooksPDFPaths(t, out), []string{"a.pdf"}; !slices.Equal(got, want) {
		t.Fatalf("pdfPaths = %v, want %v", got, want)
	}
	if out["total"] != float64(2) || out["hasMore"] != true || out["limit"] != float64(1) {
		t.Fatalf("page info = %#v, want total=2 hasMore=true limit=1", out)
	}

	out = callQueryBooks(t, session, map[string]any{"category": "Fiction", "offset": 1})
	if got, want := queryBooksPDFPaths(t, out), []string{"sub/c.pdf"}; !slices.Equal(got, want) {
		t.Fatalf("offset pdfPaths = %v, want %v", got, want)
	}

	out = callQueryBooks(t, session, map[string]any{"category": "", "status": "", "tags": []string{}})
	if got, want := queryBooksPDFPaths(t, out), []string{"b.pdf"}; !slices.Equal(got, want) {
		t.Fatalf("unset filter pdfPaths = %v, want %v", got, want)
	}

	out = callQueryBooks(t, session, map[string]any{"tags": []string{"fantasy", "missing"}, "tagMatch": "all"})
	if got := queryBooksPDFPaths(t, out); len(got) != 0 {
		t.Fatalf("tagMatch all pdfPaths = %v, want none", got)
	}
	if warnings, ok := out["warnings"].([]any); !ok || len(warnings) != 1 {
		t.Fatalf("warnings = %#v, want one warning even when nothing matches", out["warnings"])
	}

	out = callQueryBooks(t, session, map[string]any{"collection": "Series", "archived": false})
	if got, want := queryBooksPDFPaths(t, out), []string{"a.pdf"}; !slices.Equal(got, want) {
		t.Fatalf("collection pdfPaths = %v, want %v", got, want)
	}

	out = callQueryBooks(t, session, map[string]any{"recursive": false})
	if got, want := queryBooksPDFPaths(t, out), []string{"a.pdf", "b.pdf"}; !slices.Equal(got, want) {
		t.Fatalf("non-recursive pdfPaths = %v, want %v", got, want)
	}

	out = callQueryBooks(t, session, map[string]any{"directory": "sub"})
	if got, want := queryBooksPDFPaths(t, out), []string{"sub/c.pdf"}; !slices.Equal(got, want) {
		t.Fatalf("directory pdfPaths = %v, want %v", got, want)
	}
	if warnings, ok := out["warnings"].([]any); !ok || len(warnings) != 0 {
		t.Fatalf("directory warnings = %#v, want [] rather than null", out["warnings"])
	}
}

func TestQueryBooksTreatsEmptyStringsAsUnset(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRawJSONFile(t, shelff.SidecarPath(writeTestPDF(t, root, "empty.pdf")), `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "Empty"},
  "category": "",
  "collection": {"title": ""}
}`)
	writeRawJSONFile(t, shelff.SidecarPath(writeTestPDF(t, root, "set.pdf")), `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "Set"},
  "category": "X",
  "collection": {"title": "S"}
}`)

	server := newTestServer(t, root)
	session := newClientSession(t, server)
	defer session.Close()

	out := callQueryBooks(t, session, map[string]any{"category": "", "collection": ""})
	if got, want := queryBooksPDFPaths(t, out), []string{"empty.pdf"}; !slices.Equal(got, want) {
		t.Fatalf("pdfPaths = %v, want %v", got, want)
	}
	book := out["books"].([]any)[0].(map[string]any)
	for _, key := range []string{"category", "collection"} {
		if _, present := book[key]; present {
			t.Fatalf("empty.pdf has %q = %#v, want omitted", key, book[key])
		}
	}

	out = callQueryBooks(t, session, map[string]any{"category": "X", "collection": "S"})
	if got, want := queryBooksPDFPaths(t, out), []string{"set.pdf"}; !slices.Equal(got, want) {
		t.Fatalf("value filter pdfPaths = %v, want %v", got, want)
	}
}

func TestQueryBooksInputSchemaListsEnums(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, t.TempDir())
	session := newClientSession(t, server)
	defer session.Close()

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools error = %v", err)
	}
	var schema struct {
		Properties map[string]struct {
			Enum []any `json:"enum"`
		} `json:"properties"`
	}
	for _, tool := range result.Tools {
		if tool.Name != "query_books" {
			continue
		}
		data, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("Marshal input schema error = %v", err)
		}
		if err := json.Unmarshal(data, &schema); err != nil {
			t.Fatalf("Unmarshal input schema error = %v", err)
		}
	}

	if got, want := schema.Properties["status"].Enum, []any{"unread", "reading", "finished", ""}; !slices.Equal(got, want) {
		t.Fatalf("status enum = %#v, want %#v", got, want)
	}
	if got, want := schema.Properties["tagMatch"].Enum, []any{"any", "all"}; !slices.Equal(got, want) {
		t.Fatalf("tagMatch enum = %#v, want %#v", got, want)
	}
}

func TestQueryBooksHandlerRejectsInvalidFilterValues(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, t.TempDir())

	_, _, err := server.queryBooks(context.Background(), nil, queryBooksInput{TagMatch: "some"})
	if !errors.Is(err, shelff.ErrInvalidTagMatch) {
		t.Fatalf("tagMatch err = %v, want ErrInvalidTagMatch", err)
	}
	status := shelff.ReadingStatus("done")
	_, _, err = server.queryBooks(context.Background(), nil, queryBooksInput{Status: &status})
	if !errors.Is(err, shelff.ErrInvalidStatusFilter) {
		t.Fatalf("status err = %v, want ErrInvalidStatusFilter", err)
	}
}

func TestQueryBooksHandlesHugeLimit(t *testing.T) {
	t.Parallel()

	root := setupQueryBooksRoot(t)
	server := newTestServer(t, root)

	offset := 1
	limit := math.MaxInt
	_, out, err := server.queryBooks(context.Background(), nil, queryBooksInput{Offset: &offset, Limit: &limit})
	if err != nil {
		t.Fatalf("queryBooks error = %v", err)
	}
	paths := make([]string, 0, len(out.Books))
	for _, book := range out.Books {
		paths = append(paths, book.PDFPath)
	}
	if want := []string{"b.pdf", "sub/c.pdf"}; !slices.Equal(paths, want) {
		t.Fatalf("pdfPaths = %v, want %v", paths, want)
	}
	if out.Total != 3 || out.HasMore {
		t.Fatalf("page info = %#v, want total=3 hasMore=false", out)
	}
}

func TestQueryBooksRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	writeTestPDF(t, root, "book.pdf")
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("os.Symlink unavailable: %v", err)
	}

	server := newTestServer(t, root)
	session := newClientSession(t, server)
	defer session.Close()

	assertToolErrorContains(t, session, "query_books", map[string]any{"limit": 0}, ErrInvalidLimit.Error())
	assertToolErrorContains(t, session, "query_books", map[string]any{"offset": -1}, ErrInvalidOffset.Error())
	assertToolErrorContains(t, session, "query_books", map[string]any{"tagMatch": "some"}, shelff.ErrInvalidTagMatch.Error())
	assertToolErrorContains(t, session, "query_books", map[string]any{"status": "done"}, shelff.ErrInvalidStatusFilter.Error())
	assertToolTraversalError(t, session, "query_books", map[string]any{"directory": "../outside"})
	assertToolTraversalError(t, session, "query_books", map[string]any{"directory": "escape"})
}
