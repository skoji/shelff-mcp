package shelff_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/skoji/shelff-go/shelff"
)

func TestOpenLibraryReturnsAbsoluteRoot(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}

	root, err := os.MkdirTemp(wd, ".tmp-open-library-*")
	if err != nil {
		t.Fatalf("os.MkdirTemp: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Fatalf("os.RemoveAll: %v", err)
		}
	})

	relRoot, err := filepath.Rel(wd, root)
	if err != nil {
		t.Fatalf("filepath.Rel: %v", err)
	}

	library, err := shelff.OpenLibrary(relRoot)
	if err != nil {
		t.Fatalf("OpenLibrary returned error: %v", err)
	}

	wantRoot, err := filepath.Abs(relRoot)
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}

	if library.Root() != wantRoot {
		t.Fatalf("Root() = %q, want %q", library.Root(), wantRoot)
	}
}

func TestOpenLibraryReturnsErrorForMissingDirectory(t *testing.T) {
	t.Parallel()

	missingRoot := filepath.Join(t.TempDir(), "missing")

	_, err := shelff.OpenLibrary(missingRoot)
	if !errors.Is(err, shelff.ErrLibraryNotFound) {
		t.Fatalf("OpenLibrary(%q) error = %v, want ErrLibraryNotFound", missingRoot, err)
	}
}

func TestOpenLibraryReturnsErrorForFilePath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	filePath := filepath.Join(root, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("content"), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	_, err := shelff.OpenLibrary(filePath)
	if !errors.Is(err, shelff.ErrLibraryNotFound) {
		t.Fatalf("OpenLibrary(%q) error = %v, want ErrLibraryNotFound", filePath, err)
	}
}

func TestNilLibraryRootIsEmpty(t *testing.T) {
	t.Parallel()

	var library *shelff.Library
	if got := library.Root(); got != "" {
		t.Fatalf("nil Root() = %q, want empty string", got)
	}
}
