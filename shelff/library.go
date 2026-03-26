package shelff

import (
	"fmt"
	"os"
	"path/filepath"
)

// Library represents a shelff documents directory.
type Library struct {
	root string
}

// OpenLibrary creates a Library for the given root directory.
func OpenLibrary(rootDir string) (*Library, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(absRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrLibraryNotFound, absRoot)
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s", ErrLibraryNotFound, absRoot)
	}

	return &Library{root: absRoot}, nil
}

// Root returns the library root directory path.
func (l *Library) Root() string {
	if l == nil {
		return ""
	}
	return l.root
}
