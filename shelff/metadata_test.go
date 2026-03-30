package shelff

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadMetadataReturnsSidecarWhenExists(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "book.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a sidecar first
	created, err := CreateSidecar(pdfPath)
	if err != nil {
		t.Fatal(err)
	}

	meta, err := ReadMetadata(pdfPath)
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
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "my-book.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-"), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, err := ReadMetadata(pdfPath)
	if err != nil {
		t.Fatalf("ReadMetadata error = %v", err)
	}
	if meta == nil {
		t.Fatal("ReadMetadata returned nil, want minimal metadata")
	}
	if meta.Metadata.Title != "my-book" {
		t.Fatalf("title = %q, want %q", meta.Metadata.Title, "my-book")
	}
	if meta.SchemaVersion != SchemaVersion {
		t.Fatalf("schemaVersion = %d, want %d", meta.SchemaVersion, SchemaVersion)
	}
}

func TestReadMetadataReturnsErrPDFNotFound(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "nonexistent.pdf")

	_, err := ReadMetadata(pdfPath)
	if !errors.Is(err, ErrPDFNotFound) {
		t.Fatalf("error = %v, want ErrPDFNotFound", err)
	}
}
