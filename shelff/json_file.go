package shelff

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

func structToMap(v any) (map[string]any, error) {
	data, err := json.Marshal(v)
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

func writeMergedJSONFile(path string, value any, rawJSON []byte, knownKeys map[string]struct{}) ([]byte, error) {
	currentMap, err := structToMap(value)
	if err != nil {
		return nil, err
	}

	if len(rawJSON) > 0 {
		originalMap, err := jsonBytesToMap(rawJSON)
		if err != nil {
			return nil, err
		}
		mergeUnknownKeys(currentMap, originalMap, knownKeys)
	}

	data, err := json.MarshalIndent(currentMap, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := writeFileAtomically(path, data); err != nil {
		return nil, err
	}

	return data, nil
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
