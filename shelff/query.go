package shelff

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// ScanBooks scans the library for PDF files and their sidecar status.
// If recursive is true, scans subdirectories.
// Excludes the .shelff/ config directory from results.
func (l *Library) ScanBooks(recursive bool) ([]BookEntry, error) {
	return l.scanBooksFrom(l.root, recursive)
}

// ScanBooksInDirectory scans a specific directory within the library for PDF
// files and their sidecar status.
// If recursive is true, scans subdirectories below dirPath.
// Excludes the .shelff/ config directory from results.
func (l *Library) ScanBooksInDirectory(dirPath string, recursive bool) ([]BookEntry, error) {
	startDir, err := l.resolveScanDirectory(dirPath)
	if err != nil {
		return nil, err
	}
	return l.scanBooksFrom(startDir, recursive)
}

func (l *Library) scanBooksFrom(startDir string, recursive bool) ([]BookEntry, error) {
	if l.isWithinConfigDir(startDir) {
		return []BookEntry{}, nil
	}

	resolvedRoot, err := filepath.EvalSymlinks(l.root)
	if err != nil {
		return nil, err
	}

	entries := make([]BookEntry, 0)
	err = l.walkLibraryFilesFrom(startDir, recursive, func(path string, d fs.DirEntry) error {
		if !isPDFPath(path) {
			return nil
		}

		hasSidecar, err := isRegularFile(SidecarPath(path))
		if err != nil {
			return err
		}

		var sidecarPath *string
		if hasSidecar {
			value := l.normalizeLibraryPath(SidecarPath(path), resolvedRoot)
			sidecarPath = &value
		}

		entries = append(entries, BookEntry{
			PDFPath:     l.normalizeLibraryPath(path, resolvedRoot),
			SidecarPath: sidecarPath,
			HasSidecar:  hasSidecar,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	slices.SortFunc(entries, func(a BookEntry, b BookEntry) int {
		return strings.Compare(a.PDFPath, b.PDFPath)
	})
	return entries, nil
}

func (l *Library) normalizeLibraryPath(path string, resolvedRoot string) string {
	if resolvedRoot == "" || resolvedRoot == l.root {
		return path
	}

	rel, err := filepath.Rel(resolvedRoot, path)
	if err != nil {
		return path
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return filepath.Join(l.root, rel)
}

// FindOrphanedSidecars finds sidecar JSON files that have no corresponding PDF.
// Scans recursively.
func (l *Library) FindOrphanedSidecars() ([]OrphanedSidecar, error) {
	resolvedRoot, err := filepath.EvalSymlinks(l.root)
	if err != nil {
		return nil, err
	}

	orphaned := make([]OrphanedSidecar, 0)
	err = l.walkLibraryFiles(true, func(path string, d fs.DirEntry) error {
		if !IsSidecarPath(path) {
			return nil
		}

		expectedPDF, ok := PDFPathFromSidecar(path)
		if !ok {
			return nil
		}

		hasPDF, err := isRegularFile(expectedPDF)
		if err != nil {
			return err
		}
		if hasPDF {
			return nil
		}

		orphaned = append(orphaned, OrphanedSidecar{
			SidecarPath: l.normalizeLibraryPath(path, resolvedRoot),
			ExpectedPDF: l.normalizeLibraryPath(expectedPDF, resolvedRoot),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	slices.SortFunc(orphaned, func(a OrphanedSidecar, b OrphanedSidecar) int {
		return strings.Compare(a.SidecarPath, b.SidecarPath)
	})
	return orphaned, nil
}

// Stats computes aggregate statistics about the library.
// Scans all PDFs and sidecars recursively.
func (l *Library) Stats() (*LibraryStats, error) {
	books, err := l.ScanBooks(true)
	if err != nil {
		return nil, err
	}
	orphaned, err := l.FindOrphanedSidecars()
	if err != nil {
		return nil, err
	}

	stats := &LibraryStats{
		CategoryCounts:   make(map[string]int),
		TagCounts:        make(map[string]int),
		StatusCounts:     make(map[string]int),
		OrphanedSidecars: len(orphaned),
		TotalPDFs:        len(books),
	}

	for _, book := range books {
		if !book.HasSidecar {
			stats.WithoutSidecar++
			stats.StatusCounts[""]++
			continue
		}

		stats.WithSidecar++

		meta, err := ReadSidecar(book.PDFPath)
		if err != nil {
			return nil, err
		}
		if meta == nil {
			stats.WithoutSidecar++
			stats.WithSidecar--
			stats.StatusCounts[""]++
			continue
		}

		if meta.Category != nil {
			stats.CategoryCounts[*meta.Category]++
		}

		perBookTags := make(map[string]struct{}, len(meta.Tags))
		for _, tag := range meta.Tags {
			if _, exists := perBookTags[tag]; exists {
				continue
			}
			perBookTags[tag] = struct{}{}
			stats.TagCounts[tag]++
		}

		status := ""
		if meta.Reading != nil && meta.Reading.Status != nil {
			status = *meta.Reading.Status
		}
		stats.StatusCounts[status]++
	}

	return stats, nil
}

// CollectAllTags scans all sidecar files and returns the union of all tags.
// This is the canonical tag set — tags.json only defines display order.
func (l *Library) CollectAllTags() ([]string, error) {
	books, err := l.ScanBooks(true)
	if err != nil {
		return nil, err
	}

	tagSet := make(map[string]struct{})
	for _, book := range books {
		if !book.HasSidecar {
			continue
		}

		meta, err := ReadSidecar(book.PDFPath)
		if err != nil {
			return nil, err
		}
		if meta == nil {
			continue
		}

		for _, tag := range meta.Tags {
			tagSet[tag] = struct{}{}
		}
	}

	tagsConfig, err := l.ReadTagOrder()
	if err != nil {
		return nil, err
	}

	result := make([]string, 0, len(tagSet))
	seen := make(map[string]struct{}, len(tagSet))
	for _, tag := range tagsConfig.TagOrder {
		if _, ok := tagSet[tag]; !ok {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}

	extras := make([]string, 0, len(tagSet)-len(result))
	for tag := range tagSet {
		if _, ok := seen[tag]; ok {
			continue
		}
		extras = append(extras, tag)
	}
	sort.Strings(extras)
	result = append(result, extras...)

	return result, nil
}

func (l *Library) walkLibraryFiles(recursive bool, visit func(path string, d fs.DirEntry) error) error {
	return l.walkLibraryFilesFrom(l.root, recursive, visit)
}

func (l *Library) walkLibraryFilesFrom(startDir string, recursive bool, visit func(path string, d fs.DirEntry) error) error {
	walkStart := startDir
	startInfo, err := os.Lstat(startDir)
	if err != nil {
		return err
	}
	if startInfo.Mode()&os.ModeSymlink != 0 {
		walkStart, err = filepath.EvalSymlinks(startDir)
		if err != nil {
			return err
		}
	}

	return filepath.WalkDir(walkStart, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == walkStart {
			return nil
		}

		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			if d.Name() == ConfigDir {
				return filepath.SkipDir
			}
			if !recursive {
				return filepath.SkipDir
			}
			return nil
		}

		if !d.Type().IsRegular() {
			return nil
		}

		return visit(path, d)
	})
}

func (l *Library) resolveScanDirectory(dirPath string) (string, error) {
	if strings.TrimSpace(dirPath) == "" {
		return "", fmt.Errorf("scan directory is empty")
	}

	cleanDir := filepath.Clean(filepath.FromSlash(dirPath))
	baseDir := cleanDir
	if !filepath.IsAbs(baseDir) {
		baseDir = filepath.Join(l.root, baseDir)
	}

	absDir, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(absDir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("scan directory is not a directory: %s", absDir)
	}

	resolvedRoot, err := filepath.EvalSymlinks(l.root)
	if err != nil {
		return "", err
	}
	resolvedDir, err := filepath.EvalSymlinks(absDir)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(resolvedRoot, resolvedDir)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("scan directory %q is outside library root %q", absDir, l.root)
	}

	startInfo, err := os.Lstat(absDir)
	if err != nil {
		return "", err
	}
	if startInfo.Mode()&os.ModeSymlink != 0 {
		return resolvedDir, nil
	}
	return absDir, nil
}

func (l *Library) isWithinConfigDir(path string) bool {
	rel, err := l.relativeToLibraryRoot(path)
	if err != nil {
		return false
	}
	rel = filepath.Clean(rel)
	return rel == ConfigDir || strings.HasPrefix(rel, ConfigDir+string(filepath.Separator))
}

func (l *Library) relativeToLibraryRoot(path string) (string, error) {
	resolvedRoot, err := filepath.EvalSymlinks(l.root)
	if err != nil {
		return "", err
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.Rel(resolvedRoot, resolvedPath)
}

func isPDFPath(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".pdf")
}

func isRegularFile(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.Mode().IsRegular(), nil
}
