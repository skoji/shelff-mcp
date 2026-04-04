package shelff

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MakeDirectory creates a directory (and parent directories) within the library.
// The path is relative to the library root. Forward slashes are accepted.
func (l *Library) MakeDirectory(relPath string) error {
	resolved, err := l.resolveDirectoryRelPath(relPath)
	if err != nil {
		return err
	}
	if l.isConfigPath(resolved) {
		return fmt.Errorf("cannot create directory within config directory: %s", relPath)
	}
	return os.MkdirAll(resolved, 0o755)
}

// ListDirectories lists directories within the library.
// If directory is empty, lists from the library root.
// If recursive is true, lists all nested directories.
// Returns paths relative to the library root using forward slashes.
// The .shelff config directory is excluded.
func (l *Library) ListDirectories(directory string, recursive bool) ([]string, error) {
	startDir := l.root
	if dir := strings.TrimSpace(directory); dir != "" {
		resolved, err := l.resolveDirectoryRelPath(dir)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("not a directory: %s", dir)
		}
		startDir = resolved
	}

	if l.isWithinConfigDir(startDir) {
		return []string{}, nil
	}

	return l.listDirsFrom(startDir, recursive)
}

// DeleteDirectory removes an empty directory within the library.
// Returns an error if the directory is not empty, does not exist, or is the config directory.
func (l *Library) DeleteDirectory(relPath string) error {
	resolved, err := l.resolveDirectoryRelPath(relPath)
	if err != nil {
		return err
	}
	if l.isConfigPath(resolved) {
		return fmt.Errorf("cannot delete config directory: %s", relPath)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", relPath)
	}

	entries, err := os.ReadDir(resolved)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("%w: %s", ErrDirectoryNotEmpty, relPath)
	}

	return os.Remove(resolved)
}

// isConfigPath checks whether the given absolute path is within the config directory
// without requiring the path to exist (no symlink resolution).
func (l *Library) isConfigPath(absPath string) bool {
	rel, err := filepath.Rel(l.root, absPath)
	if err != nil {
		return false
	}
	rel = filepath.Clean(rel)
	return rel == ConfigDir || strings.HasPrefix(rel, ConfigDir+string(filepath.Separator))
}

func (l *Library) resolveDirectoryRelPath(relPath string) (string, error) {
	trimmed := strings.TrimSpace(relPath)
	if trimmed == "" {
		return "", fmt.Errorf("directory path is empty")
	}
	clean := filepath.Clean(filepath.FromSlash(trimmed))
	abs := filepath.Join(l.root, clean)

	// Check the literal path first.
	rel, err := filepath.Rel(l.root, abs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %s", ErrPathOutsideRoot, relPath)
	}

	// Resolve symlinks for the existing portion and re-check.
	resolved, err := resolveExistingPrefix(abs)
	if err != nil {
		return "", err
	}
	resolvedRoot, err := filepath.EvalSymlinks(l.root)
	if err != nil {
		return "", err
	}
	relResolved, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrPathOutsideRoot, relPath)
	}
	if relResolved == ".." || strings.HasPrefix(relResolved, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %s", ErrPathOutsideRoot, relPath)
	}

	return abs, nil
}

// resolveExistingPrefix resolves symlinks for the longest existing prefix of
// the path, then appends any remaining (non-existent) segments.
func resolveExistingPrefix(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return path, nil
	}
	resolvedParent, err := resolveExistingPrefix(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolvedParent, filepath.Base(path)), nil
}

func (l *Library) listDirsFrom(dir string, recursive bool) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var dirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if entry.Name() == ConfigDir {
			continue
		}
		full := filepath.Join(dir, entry.Name())
		rel, err := filepath.Rel(l.root, full)
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, filepath.ToSlash(rel))
		if recursive {
			sub, err := l.listDirsFrom(full, true)
			if err != nil {
				return nil, err
			}
			dirs = append(dirs, sub...)
		}
	}

	sort.Strings(dirs)
	if dirs == nil {
		dirs = []string{}
	}
	return dirs, nil
}
