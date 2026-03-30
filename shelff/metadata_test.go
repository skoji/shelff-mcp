package shelff_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/skoji/shelff-go/shelff"
)

func TestReadMetadataReturnsSidecarWhenExists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "book.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a sidecar first
	created, err := shelff.CreateSidecar(pdfPath)
	if err != nil {
		t.Fatal(err)
	}

	meta, err := shelff.ReadMetadata(pdfPath)
	if err != nil {
		t.Fatalf("ReadMetadata error = %v", err)
	}
	if meta == nil {
		t.Fatal("ReadMetadata returned nil")
	}
	if meta.Metadata.Title != created.Metadata.Title {
		t.Fatalf("title = %q, want %q", meta.Metadata.Title, created.Metadata.Title)
	}
	if meta.SchemaVersion != created.SchemaVersion {
		t.Fatalf("schemaVersion = %d, want %d", meta.SchemaVersion, created.SchemaVersion)
	}
}

func TestReadMetadataReturnsMinimalWhenNoSidecar(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "my-book.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-"), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, err := shelff.ReadMetadata(pdfPath)
	if err != nil {
		t.Fatalf("ReadMetadata error = %v", err)
	}
	if meta == nil {
		t.Fatal("ReadMetadata returned nil, want minimal metadata")
	}
	if meta.Metadata.Title != "my-book" {
		t.Fatalf("title = %q, want %q", meta.Metadata.Title, "my-book")
	}
	if meta.SchemaVersion != shelff.SchemaVersion {
		t.Fatalf("schemaVersion = %d, want %d", meta.SchemaVersion, shelff.SchemaVersion)
	}
}

func TestReadMetadataReturnsErrPDFNotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "nonexistent.pdf")

	_, err := shelff.ReadMetadata(pdfPath)
	if !errors.Is(err, shelff.ErrPDFNotFound) {
		t.Fatalf("error = %v, want ErrPDFNotFound", err)
	}
}

func TestReadMetadataReturnsErrPDFNotFoundForDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_, err := shelff.ReadMetadata(dir)
	if !errors.Is(err, shelff.ErrPDFNotFound) {
		t.Fatalf("error = %v, want ErrPDFNotFound", err)
	}
}

func TestWriteMetadataReturnsErrPDFNotFoundForDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_, err := shelff.WriteMetadata(dir, map[string]any{})
	if !errors.Is(err, shelff.ErrPDFNotFound) {
		t.Fatalf("error = %v, want ErrPDFNotFound", err)
	}
}

func TestWriteMetadataCreatesWhenNoSidecar(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "new-book.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-"), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, err := shelff.WriteMetadata(pdfPath, map[string]any{
		"tags": []any{"go"},
	})
	if err != nil {
		t.Fatalf("WriteMetadata error = %v", err)
	}
	if meta.Metadata.Title != "new-book" {
		t.Fatalf("title = %q, want %q", meta.Metadata.Title, "new-book")
	}
	if len(meta.Tags) != 1 || meta.Tags[0] != "go" {
		t.Fatalf("tags = %v, want [go]", meta.Tags)
	}
	if meta.SchemaVersion != shelff.SchemaVersion {
		t.Fatalf("schemaVersion = %d, want %d", meta.SchemaVersion, shelff.SchemaVersion)
	}
}

func TestWriteMetadataMergesIntoExisting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "book.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := shelff.CreateSidecar(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	// Write initial creator
	existing, err := shelff.ReadSidecar(pdfPath)
	if err != nil {
		t.Fatalf("ReadSidecar error = %v", err)
	}
	if existing == nil {
		t.Fatal("ReadSidecar returned nil")
	}
	existing.Metadata.Creator = []string{"Ada"}
	if err := shelff.WriteSidecar(pdfPath, existing); err != nil {
		t.Fatal(err)
	}

	meta, err := shelff.WriteMetadata(pdfPath, map[string]any{
		"metadata": map[string]any{
			"dc:creator": []any{"Bob"},
		},
	})
	if err != nil {
		t.Fatalf("WriteMetadata error = %v", err)
	}
	if meta.Metadata.Title != "book" {
		t.Fatalf("title = %q, want preserved as %q", meta.Metadata.Title, "book")
	}
	if len(meta.Metadata.Creator) != 1 || meta.Metadata.Creator[0] != "Bob" {
		t.Fatalf("creator = %v, want [Bob]", meta.Metadata.Creator)
	}
}

func TestWriteMetadataNilPatchDeletesField(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "book.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-"), 0o644); err != nil {
		t.Fatal(err)
	}

	cat := "ref"
	created, err := shelff.CreateSidecar(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	created.Category = &cat
	if err := shelff.WriteSidecar(pdfPath, created); err != nil {
		t.Fatal(err)
	}

	meta, err := shelff.WriteMetadata(pdfPath, map[string]any{
		"category": nil,
	})
	if err != nil {
		t.Fatalf("WriteMetadata error = %v", err)
	}
	if meta.Category != nil {
		t.Fatalf("category = %v, want nil", meta.Category)
	}
}

func TestWriteMetadataForcesSchemaVersion(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "book.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-"), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, err := shelff.WriteMetadata(pdfPath, map[string]any{
		"schemaVersion": nil,
	})
	if err != nil {
		t.Fatalf("WriteMetadata error = %v", err)
	}
	if meta.SchemaVersion != shelff.SchemaVersion {
		t.Fatalf("schemaVersion = %d, want %d", meta.SchemaVersion, shelff.SchemaVersion)
	}
}

func TestWriteMetadataPreservesTitle(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "book.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := shelff.CreateSidecar(pdfPath); err != nil {
		t.Fatal(err)
	}

	meta, err := shelff.WriteMetadata(pdfPath, map[string]any{
		"tags": []any{"test"},
	})
	if err != nil {
		t.Fatalf("WriteMetadata error = %v", err)
	}
	if meta.Metadata.Title != "book" {
		t.Fatalf("title = %q, want %q", meta.Metadata.Title, "book")
	}
}

func TestWriteMetadataPreservesUnknownTopLevelFields(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "book.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write sidecar with unknown field
	sidecarJSON := `{"schemaVersion":1,"metadata":{"dc:title":"book"},"x-custom":42}`
	if err := os.WriteFile(shelff.SidecarPath(pdfPath), []byte(sidecarJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := shelff.WriteMetadata(pdfPath, map[string]any{
		"tags": []any{"go"},
	})
	if err != nil {
		t.Fatalf("WriteMetadata error = %v", err)
	}

	// Read raw file to check x-custom preserved
	data, err := os.ReadFile(shelff.SidecarPath(pdfPath))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["x-custom"] == nil {
		t.Fatal("x-custom field was not preserved")
	}
}

func TestWriteMetadataReturnsErrPDFNotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "nonexistent.pdf")

	_, err := shelff.WriteMetadata(pdfPath, map[string]any{})
	if !errors.Is(err, shelff.ErrPDFNotFound) {
		t.Fatalf("error = %v, want ErrPDFNotFound", err)
	}
}

func TestWriteMetadataEmptyPatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "book.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-"), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, err := shelff.WriteMetadata(pdfPath, nil)
	if err != nil {
		t.Fatalf("WriteMetadata error = %v", err)
	}
	if meta == nil {
		t.Fatal("WriteMetadata returned nil")
	}
	if meta.Metadata.Title != "book" {
		t.Fatalf("title = %q, want %q", meta.Metadata.Title, "book")
	}
}
