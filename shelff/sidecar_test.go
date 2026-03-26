package shelff_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/skoji/shelff-go/shelff"
)

func TestReadSidecarReturnsNilWhenMissing(t *testing.T) {
	t.Parallel()

	pdfPath := filepath.Join(t.TempDir(), "book.pdf")

	meta, err := shelff.ReadSidecar(pdfPath)
	if err != nil {
		t.Fatalf("ReadSidecar returned error: %v", err)
	}
	if meta != nil {
		t.Fatalf("ReadSidecar() = %#v, want nil", meta)
	}
}

func TestCreateSidecarWritesInitialContent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "My Report.pdf")

	meta, err := shelff.CreateSidecar(pdfPath)
	if err != nil {
		t.Fatalf("CreateSidecar returned error: %v", err)
	}
	if meta.SchemaVersion != shelff.SchemaVersion {
		t.Fatalf("SchemaVersion = %d, want %d", meta.SchemaVersion, shelff.SchemaVersion)
	}
	if meta.Metadata.Title != "My Report" {
		t.Fatalf("Metadata.Title = %q, want %q", meta.Metadata.Title, "My Report")
	}

	decoded := decodeJSONFile(t, shelff.SidecarPath(pdfPath))
	if decoded["schemaVersion"].(json.Number).String() != "1" {
		t.Fatalf("schemaVersion = %#v, want %d", decoded["schemaVersion"], shelff.SchemaVersion)
	}

	metadata, ok := decoded["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata = %#v, want JSON object", decoded["metadata"])
	}
	if metadata["dc:title"] != "My Report" {
		t.Fatalf("metadata.dc:title = %#v, want %q", metadata["dc:title"], "My Report")
	}
}

func TestCreateSidecarReturnsErrSidecarAlreadyExists(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	if _, err := shelff.CreateSidecar(pdfPath); err != nil {
		t.Fatalf("first CreateSidecar returned error: %v", err)
	}

	_, err := shelff.CreateSidecar(pdfPath)
	if !errors.Is(err, shelff.ErrSidecarAlreadyExists) {
		t.Fatalf("CreateSidecar error = %v, want ErrSidecarAlreadyExists", err)
	}
}

func TestCreateSidecarReturnsErrPDFNotFound(t *testing.T) {
	t.Parallel()

	pdfPath := filepath.Join(t.TempDir(), "missing.pdf")
	_, err := shelff.CreateSidecar(pdfPath)
	if !errors.Is(err, shelff.ErrPDFNotFound) {
		t.Fatalf("CreateSidecar error = %v, want ErrPDFNotFound", err)
	}
}

func TestReadSidecarParsesExistingContent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := filepath.Join(root, "book.pdf")
	sidecarPath := shelff.SidecarPath(pdfPath)
	const body = `{
  "schemaVersion": 1,
  "metadata": {
    "dc:title": "Book",
    "dc:creator": ["Author"]
  },
  "tags": ["tag1", "tag2"]
}`
	writeFile(t, sidecarPath, []byte(body))

	meta, err := shelff.ReadSidecar(pdfPath)
	if err != nil {
		t.Fatalf("ReadSidecar returned error: %v", err)
	}
	if meta == nil {
		t.Fatal("ReadSidecar() returned nil metadata")
	}
	if meta.Metadata.Title != "Book" {
		t.Fatalf("Metadata.Title = %q, want %q", meta.Metadata.Title, "Book")
	}
	if len(meta.Metadata.Creator) != 1 || meta.Metadata.Creator[0] != "Author" {
		t.Fatalf("Metadata.Creator = %#v, want [\"Author\"]", meta.Metadata.Creator)
	}
	if len(meta.Tags) != 2 || meta.Tags[0] != "tag1" || meta.Tags[1] != "tag2" {
		t.Fatalf("Tags = %#v, want [\"tag1\", \"tag2\"]", meta.Tags)
	}
}

func TestParseSidecarJSONPreservesUnknownTopLevelFieldsForWrite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	meta, err := shelff.ParseSidecarJSON([]byte(`{
  "schemaVersion": 1,
  "metadata": {
    "dc:title": "Book"
  },
  "x-custom": 42
}`))
	if err != nil {
		t.Fatalf("ParseSidecarJSON returned error: %v", err)
	}

	meta.Metadata.Title = "Updated"
	if err := shelff.WriteSidecar(pdfPath, meta); err != nil {
		t.Fatalf("WriteSidecar returned error: %v", err)
	}

	decoded := decodeJSONFile(t, shelff.SidecarPath(pdfPath))
	if got := decoded["x-custom"].(json.Number).String(); got != "42" {
		t.Fatalf("x-custom = %#v, want 42", decoded["x-custom"])
	}
}

func TestWriteSidecarThenReadBack(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	status := shelff.StatusReading
	layout := shelff.LayoutSpread
	category := "Category"
	meta := &shelff.SidecarMetadata{
		SchemaVersion: shelff.SchemaVersion,
		Metadata: shelff.DublinCore{
			Title:   "Book",
			Creator: []string{"Author"},
		},
		Reading: &shelff.ReadingProgress{
			LastReadPage: 10,
			LastReadAt:   time.Date(2026, 3, 20, 1, 30, 0, 0, time.UTC),
			TotalPages:   100,
			Status:       &status,
		},
		Display: &shelff.DisplaySettings{
			Direction:  shelff.DirectionLTR,
			PageLayout: &layout,
		},
		Category: &category,
		Tags:     []string{"go", "pdf"},
	}

	if err := shelff.WriteSidecar(pdfPath, meta); err != nil {
		t.Fatalf("WriteSidecar returned error: %v", err)
	}

	readBack, err := shelff.ReadSidecar(pdfPath)
	if err != nil {
		t.Fatalf("ReadSidecar returned error: %v", err)
	}
	if readBack == nil {
		t.Fatal("ReadSidecar() returned nil metadata")
	}

	if readBack.Metadata.Title != meta.Metadata.Title {
		t.Fatalf("Metadata.Title = %q, want %q", readBack.Metadata.Title, meta.Metadata.Title)
	}
	if readBack.Reading == nil || readBack.Reading.LastReadPage != 10 || readBack.Reading.TotalPages != 100 {
		t.Fatalf("Reading = %#v, want populated reading progress", readBack.Reading)
	}
	if readBack.Display == nil || readBack.Display.Direction != shelff.DirectionLTR {
		t.Fatalf("Display = %#v, want direction %q", readBack.Display, shelff.DirectionLTR)
	}
	if readBack.Category == nil || *readBack.Category != category {
		t.Fatalf("Category = %#v, want %q", readBack.Category, category)
	}
	if len(readBack.Tags) != 2 || readBack.Tags[0] != "go" || readBack.Tags[1] != "pdf" {
		t.Fatalf("Tags = %#v, want [\"go\", \"pdf\"]", readBack.Tags)
	}
}

func TestWriteSidecarPreservesUnknownTopLevelFields(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	const original = `{
  "metadata": {
    "dc:title": "Original"
  },
  "schemaVersion": 1,
  "x-calibre-id": 42
}`
	writeFile(t, shelff.SidecarPath(pdfPath), []byte(original))

	meta, err := shelff.ReadSidecar(pdfPath)
	if err != nil {
		t.Fatalf("ReadSidecar returned error: %v", err)
	}
	meta.Metadata.Title = "Updated"

	if err := shelff.WriteSidecar(pdfPath, meta); err != nil {
		t.Fatalf("WriteSidecar returned error: %v", err)
	}

	decoded := decodeJSONFile(t, shelff.SidecarPath(pdfPath))
	if decoded["x-calibre-id"].(json.Number).String() != "42" {
		t.Fatalf("x-calibre-id = %#v, want 42", decoded["x-calibre-id"])
	}

	metadata := decoded["metadata"].(map[string]any)
	if metadata["dc:title"] != "Updated" {
		t.Fatalf("metadata.dc:title = %#v, want %q", metadata["dc:title"], "Updated")
	}
}

func TestWriteSidecarDropsUnknownMetadataFields(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	const original = `{
  "metadata": {
    "dc:title": "Original",
    "dcterms:modified": "2025-01-01"
  },
  "schemaVersion": 1
}`
	writeFile(t, shelff.SidecarPath(pdfPath), []byte(original))

	meta, err := shelff.ReadSidecar(pdfPath)
	if err != nil {
		t.Fatalf("ReadSidecar returned error: %v", err)
	}
	meta.Metadata.Title = "Updated"

	if err := shelff.WriteSidecar(pdfPath, meta); err != nil {
		t.Fatalf("WriteSidecar returned error: %v", err)
	}

	decoded := decodeJSONFile(t, shelff.SidecarPath(pdfPath))
	metadata := decoded["metadata"].(map[string]any)
	if metadata["dc:title"] != "Updated" {
		t.Fatalf("metadata.dc:title = %#v, want %q", metadata["dc:title"], "Updated")
	}
	if _, ok := metadata["dcterms:modified"]; ok {
		t.Fatalf("expected metadata.dcterms:modified to be removed, got %#v", metadata["dcterms:modified"])
	}
}

func TestWriteSidecarPreservesLargeUnknownInteger(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	const original = `{
  "metadata": {
    "dc:title": "Original"
  },
  "schemaVersion": 1,
  "x-large-id": 9007199254740993
}`
	writeFile(t, shelff.SidecarPath(pdfPath), []byte(original))

	meta, err := shelff.ReadSidecar(pdfPath)
	if err != nil {
		t.Fatalf("ReadSidecar returned error: %v", err)
	}
	meta.Metadata.Title = "Updated"

	if err := shelff.WriteSidecar(pdfPath, meta); err != nil {
		t.Fatalf("WriteSidecar returned error: %v", err)
	}

	data := string(readFile(t, shelff.SidecarPath(pdfPath)))
	if !strings.Contains(data, `"x-large-id": 9007199254740993`) {
		t.Fatalf("expected preserved large integer in file, got %s", data)
	}
}

func TestWriteSidecarDoesNotResurrectRemovedOptionalFields(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	const original = `{
  "category": "Category",
  "display": {
    "direction": "LTR"
  },
  "metadata": {
    "dc:title": "Original"
  },
  "reading": {
    "lastReadPage": 5,
    "lastReadAt": "2026-03-20T10:30:00Z",
    "totalPages": 100
  },
  "schemaVersion": 1,
  "tags": ["go"]
}`
	writeFile(t, shelff.SidecarPath(pdfPath), []byte(original))

	meta, err := shelff.ReadSidecar(pdfPath)
	if err != nil {
		t.Fatalf("ReadSidecar returned error: %v", err)
	}
	meta.Reading = nil
	meta.Display = nil
	meta.Category = nil
	meta.Tags = nil

	if err := shelff.WriteSidecar(pdfPath, meta); err != nil {
		t.Fatalf("WriteSidecar returned error: %v", err)
	}

	data := string(readFile(t, shelff.SidecarPath(pdfPath)))
	for _, key := range []string{`"reading"`, `"display"`, `"category"`, `"tags"`} {
		if strings.Contains(data, key) {
			t.Fatalf("expected %s to be removed, but file was %s", key, data)
		}
	}
}

func TestWriteSidecarPreservesDCDatePrecision(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	const original = `{
  "metadata": {
    "dc:date": "2024-06",
    "dc:title": "Book"
  },
  "schemaVersion": 1
}`
	writeFile(t, shelff.SidecarPath(pdfPath), []byte(original))

	meta, err := shelff.ReadSidecar(pdfPath)
	if err != nil {
		t.Fatalf("ReadSidecar returned error: %v", err)
	}
	if err := shelff.WriteSidecar(pdfPath, meta); err != nil {
		t.Fatalf("WriteSidecar returned error: %v", err)
	}

	decoded := decodeJSONFile(t, shelff.SidecarPath(pdfPath))
	metadata := decoded["metadata"].(map[string]any)
	if metadata["dc:date"] != "2024-06" {
		t.Fatalf("metadata.dc:date = %#v, want %q", metadata["dc:date"], "2024-06")
	}
}

func TestWriteSidecarNormalizesReadingTimesToUTC(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	status := shelff.StatusFinished
	finishedAt := time.Date(2026, 3, 20, 16, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	meta := &shelff.SidecarMetadata{
		SchemaVersion: shelff.SchemaVersion,
		Metadata: shelff.DublinCore{
			Title: "Book",
		},
		Reading: &shelff.ReadingProgress{
			LastReadPage: 100,
			LastReadAt:   time.Date(2026, 3, 20, 10, 30, 0, 0, time.FixedZone("JST", 9*60*60)),
			TotalPages:   100,
			Status:       &status,
			FinishedAt:   &finishedAt,
		},
	}

	if err := shelff.WriteSidecar(pdfPath, meta); err != nil {
		t.Fatalf("WriteSidecar returned error: %v", err)
	}

	data := string(readFile(t, shelff.SidecarPath(pdfPath)))
	if !strings.Contains(data, `"lastReadAt": "2026-03-20T01:30:00Z"`) {
		t.Fatalf("expected UTC lastReadAt in file, got %s", data)
	}
	if !strings.Contains(data, `"finishedAt": "2026-03-20T07:00:00Z"`) {
		t.Fatalf("expected UTC finishedAt in file, got %s", data)
	}
}

func TestDeleteSidecarIsIdempotent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	if _, err := shelff.CreateSidecar(pdfPath); err != nil {
		t.Fatalf("CreateSidecar returned error: %v", err)
	}

	if err := shelff.DeleteSidecar(pdfPath); err != nil {
		t.Fatalf("DeleteSidecar returned error: %v", err)
	}
	if _, err := os.Stat(shelff.SidecarPath(pdfPath)); !os.IsNotExist(err) {
		t.Fatalf("sidecar still exists after delete, stat err = %v", err)
	}
	if err := shelff.DeleteSidecar(pdfPath); err != nil {
		t.Fatalf("second DeleteSidecar returned error: %v", err)
	}
}

func TestWriteSidecarReturnsErrNilSidecarMetadata(t *testing.T) {
	t.Parallel()

	pdfPath := filepath.Join(t.TempDir(), "book.pdf")
	err := shelff.WriteSidecar(pdfPath, nil)
	if !errors.Is(err, shelff.ErrNilSidecarMetadata) {
		t.Fatalf("WriteSidecar error = %v, want ErrNilSidecarMetadata", err)
	}
}

func TestWriteSidecarPreservesExistingFileMode(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("permission mode assertions are not portable on Windows")
	}

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	sidecarPath := shelff.SidecarPath(pdfPath)
	writeFile(t, sidecarPath, []byte("{\n  \"metadata\": {\n    \"dc:title\": \"Original\"\n  },\n  \"schemaVersion\": 1\n}"))
	if err := os.Chmod(sidecarPath, 0o600); err != nil {
		t.Fatalf("os.Chmod: %v", err)
	}

	meta, err := shelff.ReadSidecar(pdfPath)
	if err != nil {
		t.Fatalf("ReadSidecar returned error: %v", err)
	}
	meta.Metadata.Title = "Updated"

	if err := shelff.WriteSidecar(pdfPath, meta); err != nil {
		t.Fatalf("WriteSidecar returned error: %v", err)
	}

	info, err := os.Stat(sidecarPath)
	if err != nil {
		t.Fatalf("os.Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("sidecar mode = %#o, want %#o", got, 0o600)
	}
}

func writeTestPDF(t *testing.T, dir string, name string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	writeFile(t, path, []byte("%PDF-1.7\n"))
	return path
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()

	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q): %v", path, err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%q): %v", path, err)
	}
	return data
}

func decodeJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()

	data := readFile(t, path)
	var decoded map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatalf("json.Unmarshal(%q): %v", path, err)
	}
	return decoded
}
