package shelff_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/skoji/shelff-mcp/shelff"
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

func TestWriteSidecarRejectsInvalidDirection(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdf := writeTestPDF(t, dir, "test.pdf")

	meta := &shelff.SidecarMetadata{
		SchemaVersion: shelff.SchemaVersion,
		Metadata:      shelff.DublinCore{Title: "test"},
		Display: &shelff.DisplaySettings{
			Direction: shelff.Direction("invalid"),
		},
	}
	err := shelff.WriteSidecar(pdf, meta)
	if !errors.Is(err, shelff.ErrInvalidFieldValue) {
		t.Fatalf("WriteSidecar with invalid direction: got %v, want ErrInvalidFieldValue", err)
	}
}

func TestWriteSidecarRejectsInvalidPageLayout(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdf := writeTestPDF(t, dir, "test.pdf")

	layout := shelff.PageLayout("bogus")
	meta := &shelff.SidecarMetadata{
		SchemaVersion: shelff.SchemaVersion,
		Metadata:      shelff.DublinCore{Title: "test"},
		Display: &shelff.DisplaySettings{
			Direction:  shelff.DirectionLTR,
			PageLayout: &layout,
		},
	}
	err := shelff.WriteSidecar(pdf, meta)
	if !errors.Is(err, shelff.ErrInvalidFieldValue) {
		t.Fatalf("WriteSidecar with invalid pageLayout: got %v, want ErrInvalidFieldValue", err)
	}
}

func TestWriteSidecarRejectsInvalidReadingStatus(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdf := writeTestPDF(t, dir, "test.pdf")

	status := shelff.ReadingStatus("bogus")
	meta := &shelff.SidecarMetadata{
		SchemaVersion: shelff.SchemaVersion,
		Metadata:      shelff.DublinCore{Title: "test"},
		Reading: &shelff.ReadingProgress{
			LastReadPage: 1,
			LastReadAt:   time.Now(),
			TotalPages:   100,
			Status:       &status,
		},
	}
	err := shelff.WriteSidecar(pdf, meta)
	if !errors.Is(err, shelff.ErrInvalidFieldValue) {
		t.Fatalf("WriteSidecar with invalid status: got %v, want ErrInvalidFieldValue", err)
	}
}

func TestWriteSidecarAcceptsNilOptionalFields(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdf := writeTestPDF(t, dir, "test.pdf")

	meta := &shelff.SidecarMetadata{
		SchemaVersion: shelff.SchemaVersion,
		Metadata:      shelff.DublinCore{Title: "test"},
	}
	if err := shelff.WriteSidecar(pdf, meta); err != nil {
		t.Fatalf("WriteSidecar with nil Display/Reading: %v", err)
	}
}

func TestReadSidecarAcceptsInvalidEnumValues(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdf := writeTestPDF(t, dir, "test.pdf")

	raw := `{
		"schemaVersion": 1,
		"metadata": {"dc:title": "test"},
		"display": {"direction": "DIAGONAL", "pageLayout": "triple"},
		"reading": {"lastReadPage": 1, "lastReadAt": "2026-01-01T00:00:00Z", "totalPages": 10, "status": "paused"}
	}`
	sidecarPath := pdf + shelff.SidecarSuffix
	writeFile(t, sidecarPath, []byte(raw))

	meta, err := shelff.ReadSidecar(pdf)
	if err != nil {
		t.Fatalf("ReadSidecar with invalid enum values: %v", err)
	}
	if meta.Display.Direction != shelff.Direction("DIAGONAL") {
		t.Fatalf("Direction = %q, want %q", meta.Display.Direction, "DIAGONAL")
	}
	if *meta.Display.PageLayout != shelff.PageLayout("triple") {
		t.Fatalf("PageLayout = %q, want %q", *meta.Display.PageLayout, "triple")
	}
	if *meta.Reading.Status != shelff.ReadingStatus("paused") {
		t.Fatalf("Status = %q, want %q", *meta.Reading.Status, "paused")
	}
}

func TestReadSidecarParsesCollection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	sidecarPath := shelff.SidecarPath(pdfPath)
	const body = `{
  "schemaVersion": 1,
  "metadata": {
    "dc:title": "Book"
  },
  "collection": {
    "title": "Monthly Swift",
    "position": 3.5
  }
}`
	writeFile(t, sidecarPath, []byte(body))

	meta, err := shelff.ReadSidecar(pdfPath)
	if err != nil {
		t.Fatalf("ReadSidecar returned error: %v", err)
	}
	if meta == nil {
		t.Fatal("ReadSidecar returned nil")
	}
	if meta.Collection == nil {
		t.Fatal("Collection = nil, want populated")
	}
	if meta.Collection.Title != "Monthly Swift" {
		t.Fatalf("Collection.Title = %q, want %q", meta.Collection.Title, "Monthly Swift")
	}
	if meta.Collection.Position == nil {
		t.Fatal("Collection.Position = nil, want 3.5")
	}
	if *meta.Collection.Position != 3.5 {
		t.Fatalf("Collection.Position = %v, want 3.5", *meta.Collection.Position)
	}
}

func TestReadSidecarParsesCollectionWithoutPosition(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	sidecarPath := shelff.SidecarPath(pdfPath)
	const body = `{
  "schemaVersion": 1,
  "metadata": {
    "dc:title": "Book"
  },
  "collection": {
    "title": "Quarterly Review"
  }
}`
	writeFile(t, sidecarPath, []byte(body))

	meta, err := shelff.ReadSidecar(pdfPath)
	if err != nil {
		t.Fatalf("ReadSidecar returned error: %v", err)
	}
	if meta.Collection == nil {
		t.Fatal("Collection = nil, want populated")
	}
	if meta.Collection.Title != "Quarterly Review" {
		t.Fatalf("Collection.Title = %q, want %q", meta.Collection.Title, "Quarterly Review")
	}
	if meta.Collection.Position != nil {
		t.Fatalf("Collection.Position = %v, want nil", *meta.Collection.Position)
	}
}

func TestWriteSidecarRoundTripsCollection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	position := 2.0
	meta := &shelff.SidecarMetadata{
		SchemaVersion: shelff.SchemaVersion,
		Metadata: shelff.DublinCore{
			Title: "Book",
		},
		Collection: &shelff.Collection{
			Title:    "Intro to Swift",
			Position: &position,
		},
	}

	if err := shelff.WriteSidecar(pdfPath, meta); err != nil {
		t.Fatalf("WriteSidecar returned error: %v", err)
	}

	decoded := decodeJSONFile(t, shelff.SidecarPath(pdfPath))
	collection, ok := decoded["collection"].(map[string]any)
	if !ok {
		t.Fatalf("collection = %#v, want JSON object", decoded["collection"])
	}
	if collection["title"] != "Intro to Swift" {
		t.Fatalf("collection.title = %#v, want %q", collection["title"], "Intro to Swift")
	}
	if got := collection["position"].(json.Number).String(); got != "2" {
		t.Fatalf("collection.position = %#v, want 2", collection["position"])
	}

	readBack, err := shelff.ReadSidecar(pdfPath)
	if err != nil {
		t.Fatalf("ReadSidecar returned error: %v", err)
	}
	if readBack.Collection == nil || readBack.Collection.Title != "Intro to Swift" {
		t.Fatalf("readBack.Collection = %#v, want title %q", readBack.Collection, "Intro to Swift")
	}
	if readBack.Collection.Position == nil || *readBack.Collection.Position != 2.0 {
		t.Fatalf("readBack.Collection.Position = %#v, want 2", readBack.Collection.Position)
	}
}

func TestWriteSidecarOmitsCollectionWhenNil(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	meta := &shelff.SidecarMetadata{
		SchemaVersion: shelff.SchemaVersion,
		Metadata:      shelff.DublinCore{Title: "Book"},
	}
	if err := shelff.WriteSidecar(pdfPath, meta); err != nil {
		t.Fatalf("WriteSidecar returned error: %v", err)
	}
	data := string(readFile(t, shelff.SidecarPath(pdfPath)))
	if strings.Contains(data, `"collection"`) {
		t.Fatalf("expected no collection key, got %s", data)
	}
}

func TestWriteSidecarRejectsCollectionWithoutTitle(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	meta := &shelff.SidecarMetadata{
		SchemaVersion: shelff.SchemaVersion,
		Metadata:      shelff.DublinCore{Title: "Book"},
		Collection:    &shelff.Collection{},
	}
	err := shelff.WriteSidecar(pdfPath, meta)
	if !errors.Is(err, shelff.ErrInvalidFieldValue) {
		t.Fatalf("WriteSidecar with empty collection title: got %v, want ErrInvalidFieldValue", err)
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

func TestReadSidecarParsesDisplayCrop(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	const body = `{
  "schemaVersion": 1,
  "metadata": {
    "dc:title": "Book"
  },
  "display": {
    "direction": "RTL",
    "pageLayout": "spread-with-cover",
    "crop": {
      "excludeFirstPage": true,
      "odd": {"top": 0.05, "bottom": 0.04, "left": 0.03, "right": 0.02},
      "even": {"top": 0.05, "bottom": 0.04, "left": 0.02, "right": 0.03}
    }
  }
}`
	writeFile(t, shelff.SidecarPath(pdfPath), []byte(body))

	meta, err := shelff.ReadSidecar(pdfPath)
	if err != nil {
		t.Fatalf("ReadSidecar returned error: %v", err)
	}
	if meta.Display == nil || meta.Display.Crop == nil {
		t.Fatalf("Display = %#v, want crop populated", meta.Display)
	}
	crop := meta.Display.Crop
	if crop.ExcludeFirstPage == nil || !*crop.ExcludeFirstPage {
		t.Fatalf("crop.ExcludeFirstPage = %v, want true", crop.ExcludeFirstPage)
	}
	wantOdd := shelff.CropInsets{Top: 0.05, Bottom: 0.04, Left: 0.03, Right: 0.02}
	if crop.Odd != wantOdd {
		t.Fatalf("crop.Odd = %+v, want %+v", crop.Odd, wantOdd)
	}
	wantEven := shelff.CropInsets{Top: 0.05, Bottom: 0.04, Left: 0.02, Right: 0.03}
	if crop.Even != wantEven {
		t.Fatalf("crop.Even = %+v, want %+v", crop.Even, wantEven)
	}
}

func TestReadSidecarParsesDisplayCropWithoutExcludeFirstPage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	const body = `{
  "schemaVersion": 1,
  "metadata": {
    "dc:title": "Book"
  },
  "display": {
    "direction": "LTR",
    "crop": {
      "odd": {"top": 0, "bottom": 0, "left": 0.1, "right": 0},
      "even": {"top": 0, "bottom": 0, "left": 0, "right": 0.1}
    }
  }
}`
	writeFile(t, shelff.SidecarPath(pdfPath), []byte(body))

	meta, err := shelff.ReadSidecar(pdfPath)
	if err != nil {
		t.Fatalf("ReadSidecar returned error: %v", err)
	}
	if meta.Display == nil || meta.Display.Crop == nil {
		t.Fatalf("Display = %#v, want crop populated", meta.Display)
	}
	if meta.Display.Crop.ExcludeFirstPage != nil {
		t.Fatalf("crop.ExcludeFirstPage = %v, want nil when omitted", *meta.Display.Crop.ExcludeFirstPage)
	}
}

func TestReadSidecarParsesNullDisplayCrop(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	const body = `{
  "schemaVersion": 1,
  "metadata": {
    "dc:title": "Book"
  },
  "display": {
    "direction": "LTR",
    "crop": null
  }
}`
	writeFile(t, shelff.SidecarPath(pdfPath), []byte(body))

	meta, err := shelff.ReadSidecar(pdfPath)
	if err != nil {
		t.Fatalf("ReadSidecar returned error: %v", err)
	}
	if meta.Display == nil {
		t.Fatal("Display = nil, want populated")
	}
	if meta.Display.Crop != nil {
		t.Fatalf("Display.Crop = %+v, want nil", meta.Display.Crop)
	}
}

func TestWriteSidecarRoundTripsDisplayCrop(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	meta := &shelff.SidecarMetadata{
		SchemaVersion: shelff.SchemaVersion,
		Metadata:      shelff.DublinCore{Title: "Book"},
		Display: &shelff.DisplaySettings{
			Direction: shelff.DirectionRTL,
			Crop: &shelff.CropSettings{
				Odd:  shelff.CropInsets{Top: 0.05, Bottom: 0.04, Left: 0.03, Right: 0.02},
				Even: shelff.CropInsets{Top: 0.05, Bottom: 0.04, Left: 0.02, Right: 0.03},
			},
		},
	}
	if err := shelff.WriteSidecar(pdfPath, meta); err != nil {
		t.Fatalf("WriteSidecar returned error: %v", err)
	}

	decoded := decodeJSONFile(t, shelff.SidecarPath(pdfPath))
	display, ok := decoded["display"].(map[string]any)
	if !ok {
		t.Fatalf("display = %#v, want JSON object", decoded["display"])
	}
	crop, ok := display["crop"].(map[string]any)
	if !ok {
		t.Fatalf("display.crop = %#v, want JSON object", display["crop"])
	}
	if _, present := crop["excludeFirstPage"]; present {
		t.Fatalf("display.crop = %#v, want no excludeFirstPage key when nil", crop)
	}
	odd, ok := crop["odd"].(map[string]any)
	if !ok {
		t.Fatalf("display.crop.odd = %#v, want JSON object", crop["odd"])
	}
	for _, side := range []string{"top", "bottom", "left", "right"} {
		if _, present := odd[side]; !present {
			t.Fatalf("display.crop.odd = %#v, want %q always written", odd, side)
		}
	}

	readBack, err := shelff.ReadSidecar(pdfPath)
	if err != nil {
		t.Fatalf("ReadSidecar returned error: %v", err)
	}
	if readBack.Display == nil || readBack.Display.Crop == nil {
		t.Fatalf("readBack.Display = %#v, want crop populated", readBack.Display)
	}
	if readBack.Display.Crop.Odd != meta.Display.Crop.Odd {
		t.Fatalf("readBack crop.Odd = %+v, want %+v", readBack.Display.Crop.Odd, meta.Display.Crop.Odd)
	}
	if readBack.Display.Crop.Even != meta.Display.Crop.Even {
		t.Fatalf("readBack crop.Even = %+v, want %+v", readBack.Display.Crop.Even, meta.Display.Crop.Even)
	}
}

func TestWriteSidecarWritesExplicitExcludeFirstPageFalse(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	excludeFirstPage := false
	meta := &shelff.SidecarMetadata{
		SchemaVersion: shelff.SchemaVersion,
		Metadata:      shelff.DublinCore{Title: "Book"},
		Display: &shelff.DisplaySettings{
			Direction: shelff.DirectionLTR,
			Crop: &shelff.CropSettings{
				Odd:              shelff.CropInsets{Left: 0.1},
				Even:             shelff.CropInsets{Right: 0.1},
				ExcludeFirstPage: &excludeFirstPage,
			},
		},
	}
	if err := shelff.WriteSidecar(pdfPath, meta); err != nil {
		t.Fatalf("WriteSidecar returned error: %v", err)
	}

	readBack, err := shelff.ReadSidecar(pdfPath)
	if err != nil {
		t.Fatalf("ReadSidecar returned error: %v", err)
	}
	if readBack.Display == nil || readBack.Display.Crop == nil {
		t.Fatalf("readBack.Display = %#v, want crop populated", readBack.Display)
	}
	if readBack.Display.Crop.ExcludeFirstPage == nil || *readBack.Display.Crop.ExcludeFirstPage {
		t.Fatalf("readBack crop.ExcludeFirstPage = %v, want explicit false", readBack.Display.Crop.ExcludeFirstPage)
	}
}

func TestWriteSidecarOmitsCropWhenNil(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	meta := &shelff.SidecarMetadata{
		SchemaVersion: shelff.SchemaVersion,
		Metadata:      shelff.DublinCore{Title: "Book"},
		Display:       &shelff.DisplaySettings{Direction: shelff.DirectionLTR},
	}
	if err := shelff.WriteSidecar(pdfPath, meta); err != nil {
		t.Fatalf("WriteSidecar returned error: %v", err)
	}
	if data := string(readFile(t, shelff.SidecarPath(pdfPath))); strings.Contains(data, `"crop"`) {
		t.Fatalf("expected no crop key, got %s", data)
	}
}

func TestWriteSidecarRejectsInvalidCropInsets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		crop shelff.CropSettings
	}{
		{
			name: "negative odd inset",
			crop: shelff.CropSettings{Odd: shelff.CropInsets{Top: -0.1}},
		},
		{
			name: "negative even inset",
			crop: shelff.CropSettings{Even: shelff.CropInsets{Left: -0.1}},
		},
		{
			name: "odd vertical sum reaches one",
			crop: shelff.CropSettings{Odd: shelff.CropInsets{Top: 0.6, Bottom: 0.4}},
		},
		{
			name: "even horizontal sum reaches one",
			crop: shelff.CropSettings{Even: shelff.CropInsets{Left: 0.5, Right: 0.5}},
		},
		{
			name: "inset above one",
			crop: shelff.CropSettings{Odd: shelff.CropInsets{Right: 1.5}},
		},
		{
			name: "not a number",
			crop: shelff.CropSettings{Odd: shelff.CropInsets{Top: math.NaN()}},
		},
		{
			name: "infinite",
			crop: shelff.CropSettings{Even: shelff.CropInsets{Bottom: math.Inf(1)}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pdfPath := writeTestPDF(t, t.TempDir(), "book.pdf")
			crop := tt.crop
			meta := &shelff.SidecarMetadata{
				SchemaVersion: shelff.SchemaVersion,
				Metadata:      shelff.DublinCore{Title: "Book"},
				Display: &shelff.DisplaySettings{
					Direction: shelff.DirectionLTR,
					Crop:      &crop,
				},
			}
			err := shelff.WriteSidecar(pdfPath, meta)
			if !errors.Is(err, shelff.ErrInvalidFieldValue) {
				t.Fatalf("WriteSidecar error = %v, want ErrInvalidFieldValue", err)
			}
			if _, statErr := os.Stat(shelff.SidecarPath(pdfPath)); !os.IsNotExist(statErr) {
				t.Fatalf("sidecar was written despite invalid crop: %v", statErr)
			}
		})
	}
}

func TestReadSidecarAcceptsOutOfRangeCrop(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	const body = `{
  "schemaVersion": 1,
  "metadata": {
    "dc:title": "Book"
  },
  "display": {
    "direction": "LTR",
    "crop": {
      "odd": {"top": 0.9, "bottom": 0.9, "left": 0, "right": 0},
      "even": {"top": -1, "bottom": 0, "left": 0, "right": 0}
    }
  }
}`
	writeFile(t, shelff.SidecarPath(pdfPath), []byte(body))

	meta, err := shelff.ReadSidecar(pdfPath)
	if err != nil {
		t.Fatalf("ReadSidecar returned error: %v", err)
	}
	if meta.Display == nil || meta.Display.Crop == nil {
		t.Fatalf("Display = %#v, want crop populated even when out of range", meta.Display)
	}
	if meta.Display.Crop.Odd.Top != 0.9 {
		t.Fatalf("crop.Odd.Top = %v, want 0.9 passed through", meta.Display.Crop.Odd.Top)
	}
}
