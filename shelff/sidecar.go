package shelff

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

var (
	knownSidecarTopLevelKeys = map[string]struct{}{
		"schemaVersion": {},
		"metadata":      {},
		"reading":       {},
		"display":       {},
		"category":      {},
		"tags":          {},
	}
)

// ReadSidecar reads and parses the sidecar JSON for the given PDF.
func ReadSidecar(pdfPath string) (*SidecarMetadata, error) {
	data, err := os.ReadFile(SidecarPath(pdfPath))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var meta SidecarMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	meta.rawJSON = append([]byte(nil), data...)
	return &meta, nil
}

// CreateSidecar creates a new sidecar JSON for the given PDF.
func CreateSidecar(pdfPath string) (*SidecarMetadata, error) {
	info, err := os.Stat(pdfPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrPDFNotFound
		}
		return nil, err
	}
	if info.IsDir() {
		return nil, ErrPDFNotFound
	}

	if _, err := os.Stat(SidecarPath(pdfPath)); err == nil {
		return nil, ErrSidecarAlreadyExists
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	meta := &SidecarMetadata{
		SchemaVersion: SchemaVersion,
		Metadata: DublinCore{
			Title: pdfTitleFromPath(pdfPath),
		},
	}

	if err := WriteSidecar(pdfPath, meta); err != nil {
		return nil, err
	}

	return meta, nil
}

// WriteSidecar writes the sidecar JSON for the given PDF.
// It preserves unknown top-level fields present in meta.rawJSON (typically when
// meta originated from ReadSidecar), but does not otherwise read or merge from
// any on-disk sidecar JSON.
// It does not verify that the PDF file itself currently exists.
// Atomic replacement relies on the host platform's rename semantics.
func WriteSidecar(pdfPath string, meta *SidecarMetadata) error {
	if meta == nil {
		return ErrNilSidecarMetadata
	}

	normalized := normalizeSidecarMetadata(meta)
	data, err := writeMergedJSONFile(SidecarPath(pdfPath), normalized, meta.rawJSON, knownSidecarTopLevelKeys)
	if err != nil {
		return err
	}

	meta.rawJSON = append([]byte(nil), data...)
	return nil
}

// DeleteSidecar deletes the sidecar JSON for the given PDF.
func DeleteSidecar(pdfPath string) error {
	if err := os.Remove(SidecarPath(pdfPath)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func pdfTitleFromPath(pdfPath string) string {
	base := filepath.Base(pdfPath)
	ext := filepath.Ext(base)
	if strings.EqualFold(ext, ".pdf") {
		return strings.TrimSuffix(base, ext)
	}
	return base
}

func normalizeSidecarMetadata(meta *SidecarMetadata) *SidecarMetadata {
	copyMeta := *meta
	if meta.Reading != nil {
		readingCopy := *meta.Reading
		readingCopy.LastReadAt = readingCopy.LastReadAt.UTC()
		if readingCopy.FinishedAt != nil {
			finishedAt := readingCopy.FinishedAt.UTC()
			readingCopy.FinishedAt = &finishedAt
		}
		copyMeta.Reading = &readingCopy
	}
	return &copyMeta
}
