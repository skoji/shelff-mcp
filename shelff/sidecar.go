package shelff

import (
	"bytes"
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
// It preserves unknown top-level fields from the existing sidecar JSON.
// It does not verify that the PDF file itself currently exists.
// Atomic replacement relies on the host platform's rename semantics.
func WriteSidecar(pdfPath string, meta *SidecarMetadata) error {
	if meta == nil {
		return ErrNilSidecarMetadata
	}

	normalized := normalizeSidecarMetadata(meta)

	currentMap, err := sidecarMetadataToMap(normalized)
	if err != nil {
		return err
	}

	if len(meta.rawJSON) > 0 {
		originalMap, err := jsonBytesToMap(meta.rawJSON)
		if err != nil {
			return err
		}

		mergeUnknownKeys(currentMap, originalMap, knownSidecarTopLevelKeys)
	}

	data, err := json.MarshalIndent(currentMap, "", "  ")
	if err != nil {
		return err
	}

	if err := writeFileAtomically(SidecarPath(pdfPath), data); err != nil {
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

func sidecarMetadataToMap(meta *SidecarMetadata) (map[string]any, error) {
	data, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	return jsonBytesToMap(data)
}

func jsonBytesToMap(data []byte) (map[string]any, error) {
	var result map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		return nil, err
	}
	if result == nil {
		result = map[string]any{}
	}
	return result, nil
}

func mergeUnknownKeys(dst map[string]any, src map[string]any, knownKeys map[string]struct{}) {
	for key, value := range src {
		if _, known := knownKeys[key]; known {
			continue
		}
		dst[key] = value
	}
}

func writeFileAtomically(path string, data []byte) (err error) {
	mode, err := fileModeForAtomicWrite(path)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}

	tmpPath := tmpFile.Name()
	defer func() {
		if err != nil {
			_ = tmpFile.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	if err = tmpFile.Chmod(mode); err != nil {
		return err
	}
	if _, err = tmpFile.Write(data); err != nil {
		return err
	}
	if err = tmpFile.Sync(); err != nil {
		return err
	}
	if err = tmpFile.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmpPath, path); err != nil {
		return err
	}

	return nil
}

func fileModeForAtomicWrite(path string) (os.FileMode, error) {
	info, err := os.Stat(path)
	if err == nil {
		return info.Mode().Perm(), nil
	}
	if os.IsNotExist(err) {
		return 0o644, nil
	}
	return 0, err
}
