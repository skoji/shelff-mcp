package shelff_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/skoji/shelff-mcp/shelff"
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

func TestReadMetadataReturnsCollection(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "book.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-"), 0o644); err != nil {
		t.Fatal(err)
	}

	const body = `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "Book"},
  "collection": {"title": "Monthly Swift", "position": 4}
}`
	if err := os.WriteFile(shelff.SidecarPath(pdfPath), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, err := shelff.ReadMetadata(pdfPath)
	if err != nil {
		t.Fatalf("ReadMetadata error = %v", err)
	}
	if meta.Collection == nil {
		t.Fatal("Collection = nil, want populated")
	}
	if meta.Collection.Title != "Monthly Swift" {
		t.Fatalf("Collection.Title = %q, want %q", meta.Collection.Title, "Monthly Swift")
	}
	if meta.Collection.Position == nil || *meta.Collection.Position != 4 {
		t.Fatalf("Collection.Position = %v, want 4", meta.Collection.Position)
	}
}

func TestWriteMetadataAddsCollection(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "book.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-"), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, err := shelff.WriteMetadata(pdfPath, map[string]any{
		"collection": map[string]any{
			"title":    "Monthly Swift",
			"position": 7,
		},
	})
	if err != nil {
		t.Fatalf("WriteMetadata error = %v", err)
	}
	if meta.Collection == nil {
		t.Fatal("Collection = nil, want populated")
	}
	if meta.Collection.Title != "Monthly Swift" {
		t.Fatalf("Collection.Title = %q, want %q", meta.Collection.Title, "Monthly Swift")
	}
	if meta.Collection.Position == nil || *meta.Collection.Position != 7 {
		t.Fatalf("Collection.Position = %v, want 7", meta.Collection.Position)
	}
}

func TestWriteMetadataDeletesCollection(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "book.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-"), 0o644); err != nil {
		t.Fatal(err)
	}

	const body = `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "Book"},
  "collection": {"title": "Monthly Swift"}
}`
	if err := os.WriteFile(shelff.SidecarPath(pdfPath), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, err := shelff.WriteMetadata(pdfPath, map[string]any{
		"collection": nil,
	})
	if err != nil {
		t.Fatalf("WriteMetadata error = %v", err)
	}
	if meta.Collection != nil {
		t.Fatalf("Collection = %v, want nil", meta.Collection)
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

const cropSidecarBody = `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "Book"},
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

func TestReadMetadataReturnsDisplayCrop(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "book.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shelff.SidecarPath(pdfPath), []byte(cropSidecarBody), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, err := shelff.ReadMetadata(pdfPath)
	if err != nil {
		t.Fatalf("ReadMetadata error = %v", err)
	}
	if meta.Display == nil || meta.Display.Crop == nil {
		t.Fatalf("Display = %#v, want crop populated", meta.Display)
	}
	if meta.Display.Crop.Odd.Top != 0.05 || meta.Display.Crop.Even.Right != 0.03 {
		t.Fatalf("crop = %+v, want odd.top 0.05 and even.right 0.03", *meta.Display.Crop)
	}
}

func TestWriteMetadataPreservesDisplayCropWhenUntouched(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "book.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shelff.SidecarPath(pdfPath), []byte(cropSidecarBody), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, err := shelff.WriteMetadata(pdfPath, map[string]any{
		"metadata": map[string]any{
			"dc:title": "Renamed",
		},
	})
	if err != nil {
		t.Fatalf("WriteMetadata error = %v", err)
	}
	if meta.Metadata.Title != "Renamed" {
		t.Fatalf("title = %q, want %q", meta.Metadata.Title, "Renamed")
	}
	if meta.Display == nil || meta.Display.Crop == nil {
		t.Fatalf("Display = %#v, want crop preserved by an unrelated update", meta.Display)
	}
	crop := meta.Display.Crop
	wantOdd := shelff.CropInsets{Top: 0.05, Bottom: 0.04, Left: 0.03, Right: 0.02}
	if crop.Odd != wantOdd {
		t.Fatalf("crop.Odd = %+v, want %+v", crop.Odd, wantOdd)
	}
	wantEven := shelff.CropInsets{Top: 0.05, Bottom: 0.04, Left: 0.02, Right: 0.03}
	if crop.Even != wantEven {
		t.Fatalf("crop.Even = %+v, want %+v", crop.Even, wantEven)
	}
	if crop.ExcludeFirstPage == nil || !*crop.ExcludeFirstPage {
		t.Fatalf("crop.ExcludeFirstPage = %v, want true preserved", crop.ExcludeFirstPage)
	}

	data, err := os.ReadFile(shelff.SidecarPath(pdfPath))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	display, ok := raw["display"].(map[string]any)
	if !ok {
		t.Fatalf("raw display = %#v, want JSON object", raw["display"])
	}
	if _, present := display["crop"]; !present {
		t.Fatalf("raw display = %#v, want crop key on disk", display)
	}
}

func TestWriteMetadataAddsDisplayCropKeepingDirection(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "book.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-"), 0o644); err != nil {
		t.Fatal(err)
	}
	const body = `{
  "schemaVersion": 1,
  "metadata": {"dc:title": "Book"},
  "display": {"direction": "RTL"}
}`
	if err := os.WriteFile(shelff.SidecarPath(pdfPath), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, err := shelff.WriteMetadata(pdfPath, map[string]any{
		"display": map[string]any{
			"crop": map[string]any{
				"odd":  map[string]any{"top": 0.1, "bottom": 0.1, "left": 0.05, "right": 0.05},
				"even": map[string]any{"top": 0.1, "bottom": 0.1, "left": 0.05, "right": 0.05},
			},
		},
	})
	if err != nil {
		t.Fatalf("WriteMetadata error = %v", err)
	}
	if meta.Display == nil {
		t.Fatal("Display = nil, want preserved")
	}
	if meta.Display.Direction != shelff.DirectionRTL {
		t.Fatalf("direction = %q, want %q", meta.Display.Direction, shelff.DirectionRTL)
	}
	if meta.Display.Crop == nil {
		t.Fatal("Display.Crop = nil, want added")
	}
	if meta.Display.Crop.Odd.Top != 0.1 || meta.Display.Crop.Even.Left != 0.05 {
		t.Fatalf("crop = %+v, want odd.top 0.1 and even.left 0.05", *meta.Display.Crop)
	}
	if meta.Display.Crop.ExcludeFirstPage != nil {
		t.Fatalf("crop.ExcludeFirstPage = %v, want nil when not supplied", *meta.Display.Crop.ExcludeFirstPage)
	}
}

func TestWriteMetadataMergesIntoExistingDisplayCrop(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "book.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shelff.SidecarPath(pdfPath), []byte(cropSidecarBody), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, err := shelff.WriteMetadata(pdfPath, map[string]any{
		"display": map[string]any{
			"crop": map[string]any{
				"odd": map[string]any{"top": 0.2},
			},
		},
	})
	if err != nil {
		t.Fatalf("WriteMetadata error = %v", err)
	}
	if meta.Display == nil || meta.Display.Crop == nil {
		t.Fatalf("Display = %#v, want crop populated", meta.Display)
	}
	wantOdd := shelff.CropInsets{Top: 0.2, Bottom: 0.04, Left: 0.03, Right: 0.02}
	if meta.Display.Crop.Odd != wantOdd {
		t.Fatalf("crop.Odd = %+v, want %+v", meta.Display.Crop.Odd, wantOdd)
	}
	wantEven := shelff.CropInsets{Top: 0.05, Bottom: 0.04, Left: 0.02, Right: 0.03}
	if meta.Display.Crop.Even != wantEven {
		t.Fatalf("crop.Even = %+v, want %+v", meta.Display.Crop.Even, wantEven)
	}
}

func TestWriteMetadataDeletesDisplayCrop(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "book.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shelff.SidecarPath(pdfPath), []byte(cropSidecarBody), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, err := shelff.WriteMetadata(pdfPath, map[string]any{
		"display": map[string]any{
			"crop": nil,
		},
	})
	if err != nil {
		t.Fatalf("WriteMetadata error = %v", err)
	}
	if meta.Display == nil {
		t.Fatal("Display = nil, want kept with direction")
	}
	if meta.Display.Direction != shelff.DirectionRTL {
		t.Fatalf("direction = %q, want %q", meta.Display.Direction, shelff.DirectionRTL)
	}
	if meta.Display.Crop != nil {
		t.Fatalf("Display.Crop = %+v, want nil after null patch", *meta.Display.Crop)
	}
}

func TestWriteMetadataDropsDisplayWhenCropHasNoDirection(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "book.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-"), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, err := shelff.WriteMetadata(pdfPath, map[string]any{
		"display": map[string]any{
			"crop": map[string]any{
				"odd":  map[string]any{"top": 0.1, "bottom": 0, "left": 0, "right": 0},
				"even": map[string]any{"top": 0.1, "bottom": 0, "left": 0, "right": 0},
			},
		},
	})
	if err != nil {
		t.Fatalf("WriteMetadata error = %v", err)
	}
	if meta.Display != nil {
		t.Fatalf("Display = %#v, want dropped because direction is required", meta.Display)
	}
}

func TestWriteMetadataRejectsInvalidDisplayCrop(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		crop map[string]any
	}{
		{
			name: "negative inset",
			crop: map[string]any{
				"odd":  map[string]any{"top": -0.1, "bottom": 0, "left": 0, "right": 0},
				"even": map[string]any{"top": 0, "bottom": 0, "left": 0, "right": 0},
			},
		},
		{
			name: "vertical sum reaches one",
			crop: map[string]any{
				"odd":  map[string]any{"top": 0.5, "bottom": 0.5, "left": 0, "right": 0},
				"even": map[string]any{"top": 0, "bottom": 0, "left": 0, "right": 0},
			},
		},
		{
			name: "horizontal sum exceeds one",
			crop: map[string]any{
				"odd":  map[string]any{"top": 0, "bottom": 0, "left": 0, "right": 0},
				"even": map[string]any{"top": 0, "bottom": 0, "left": 0.7, "right": 0.4},
			},
		},
		{
			name: "inset above one",
			crop: map[string]any{
				"odd":  map[string]any{"top": 1.5, "bottom": 0, "left": 0, "right": 0},
				"even": map[string]any{"top": 0, "bottom": 0, "left": 0, "right": 0},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			pdfPath := filepath.Join(dir, "book.pdf")
			if err := os.WriteFile(pdfPath, []byte("%PDF-"), 0o644); err != nil {
				t.Fatal(err)
			}

			_, err := shelff.WriteMetadata(pdfPath, map[string]any{
				"display": map[string]any{
					"direction": "LTR",
					"crop":      tt.crop,
				},
			})
			if !errors.Is(err, shelff.ErrInvalidFieldValue) {
				t.Fatalf("WriteMetadata error = %v, want ErrInvalidFieldValue", err)
			}
		})
	}
}
