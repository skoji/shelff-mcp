package shelff

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
)

// ReadMetadata returns metadata for a PDF.
// If a sidecar exists, returns its content.
// If no sidecar exists, returns a minimal SidecarMetadata with
// dc:title set to the PDF filename (without .pdf extension).
// Unlike ReadSidecar, this never returns nil for an existing PDF.
// Returns ErrPDFNotFound if the PDF does not exist.
func ReadMetadata(pdfPath string) (*SidecarMetadata, error) {
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

	sidecar, err := ReadSidecar(pdfPath)
	if err != nil {
		return nil, err
	}
	if sidecar != nil {
		return sidecar, nil
	}

	return &SidecarMetadata{
		SchemaVersion: SchemaVersion,
		Metadata: DublinCore{
			Title: pdfTitleFromPath(pdfPath),
		},
	}, nil
}

// WriteMetadata applies a partial update to the metadata for pdfPath.
// If no sidecar exists, one is created first.
// The partial map is merged into the existing (or newly created) metadata.
// Returns the written metadata after round-trip through disk.
func WriteMetadata(pdfPath string, partial map[string]any) (*SidecarMetadata, error) {
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

	if partial == nil {
		partial = map[string]any{}
	}

	existing, err := ReadSidecar(pdfPath)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		existing, err = CreateSidecar(pdfPath)
		if err != nil {
			return nil, err
		}
	}

	currentMap, err := sidecarToMap(existing)
	if err != nil {
		return nil, err
	}
	merged := mergeJSONObject(currentMap, partial)
	merged = normalizeMergedSidecar(currentMap, merged)

	next, err := mapToSidecar(merged)
	if err != nil {
		return nil, err
	}
	if err := WriteSidecar(pdfPath, next); err != nil {
		return nil, err
	}

	return ReadSidecar(pdfPath)
}

// sidecarToMap converts a SidecarMetadata to a map[string]any,
// preserving raw JSON (including json.Number) for round-trip fidelity.
func sidecarToMap(meta *SidecarMetadata) (map[string]any, error) {
	data := meta.rawJSON
	if len(data) == 0 {
		var err error
		data, err = json.Marshal(meta)
		if err != nil {
			return nil, err
		}
	}

	var decoded map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if err == nil {
			return nil, errUnexpectedTrailingData
		}
		return nil, err
	}
	if decoded == nil {
		decoded = map[string]any{}
	}
	return decoded, nil
}

func mapToSidecar(value map[string]any) (*SidecarMetadata, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return ParseSidecarJSON(data)
}

func mergeJSONObject(current map[string]any, patch map[string]any) map[string]any {
	if current == nil {
		current = map[string]any{}
	}

	merged := cloneJSONObject(current)
	for key, patchValue := range patch {
		if patchValue == nil {
			delete(merged, key)
			continue
		}

		patchObject, patchIsObject := patchValue.(map[string]any)
		currentObject, currentIsObject := merged[key].(map[string]any)
		if patchIsObject && currentIsObject {
			merged[key] = mergeJSONObject(currentObject, patchObject)
			continue
		}
		merged[key] = cloneJSONValue(patchValue)
	}
	return merged
}

func normalizeMergedSidecar(current map[string]any, merged map[string]any) map[string]any {
	merged["schemaVersion"] = SchemaVersion

	currentMetadata, _ := current["metadata"].(map[string]any)
	mergedMetadata, ok := merged["metadata"].(map[string]any)
	if !ok || mergedMetadata == nil {
		mergedMetadata = cloneJSONObject(currentMetadata)
	}
	if currentMetadata != nil {
		if title, ok := currentMetadata["dc:title"]; ok {
			if _, present := mergedMetadata["dc:title"]; !present || mergedMetadata["dc:title"] == nil {
				mergedMetadata["dc:title"] = title
			}
		}
	}
	merged["metadata"] = mergedMetadata
	normalizeRequiredObject(merged, "reading", "lastReadAt", "lastReadPage", "totalPages")
	normalizeRequiredObject(merged, "display", "direction")

	return merged
}

func normalizeRequiredObject(merged map[string]any, key string, requiredKeys ...string) {
	raw, ok := merged[key]
	if !ok {
		return
	}

	object, ok := raw.(map[string]any)
	if !ok || object == nil {
		delete(merged, key)
		return
	}

	for _, requiredKey := range requiredKeys {
		if _, present := object[requiredKey]; !present || object[requiredKey] == nil {
			delete(merged, key)
			return
		}
	}
}

func cloneJSONObject(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	cloned := make(map[string]any, len(value))
	for key, child := range value {
		cloned[key] = cloneJSONValue(child)
	}
	return cloned
}

func cloneJSONValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return cloneJSONObject(v)
	case []any:
		cloned := make([]any, len(v))
		for i, child := range v {
			cloned[i] = cloneJSONValue(child)
		}
		return cloned
	default:
		return value
	}
}
