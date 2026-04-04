package shelff_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/skoji/shelff-mcp/shelff"
)

func TestMakeDirectory(t *testing.T) {
	t.Parallel()

	t.Run("creates a single directory", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		library := openTestLibrary(t, root)

		err := library.MakeDirectory("new-shelf")
		if err != nil {
			t.Fatalf("MakeDirectory returned error: %v", err)
		}

		info, err := os.Stat(filepath.Join(root, "new-shelf"))
		if err != nil {
			t.Fatalf("directory not created: %v", err)
		}
		if !info.IsDir() {
			t.Fatal("created path is not a directory")
		}
	})

	t.Run("creates nested directories", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		library := openTestLibrary(t, root)

		err := library.MakeDirectory("a/b/c")
		if err != nil {
			t.Fatalf("MakeDirectory returned error: %v", err)
		}

		info, err := os.Stat(filepath.Join(root, "a", "b", "c"))
		if err != nil {
			t.Fatalf("nested directory not created: %v", err)
		}
		if !info.IsDir() {
			t.Fatal("created path is not a directory")
		}
	})

	t.Run("idempotent for existing directory", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		library := openTestLibrary(t, root)
		mkdirAll(t, filepath.Join(root, "existing"))

		err := library.MakeDirectory("existing")
		if err != nil {
			t.Fatalf("MakeDirectory on existing dir returned error: %v", err)
		}
	})

	t.Run("rejects path traversal", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		library := openTestLibrary(t, root)

		err := library.MakeDirectory("../outside")
		if err == nil {
			t.Fatal("expected error for path traversal, got nil")
		}
	})

	t.Run("rejects empty path", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		library := openTestLibrary(t, root)

		err := library.MakeDirectory("")
		if err == nil {
			t.Fatal("expected error for empty path, got nil")
		}
	})

	t.Run("rejects config directory", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		library := openTestLibrary(t, root)

		err := library.MakeDirectory(shelff.ConfigDir)
		if err == nil {
			t.Fatal("expected error for config directory, got nil")
		}
	})
}

func TestListDirectories(t *testing.T) {
	t.Parallel()

	t.Run("lists top-level directories", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		library := openTestLibrary(t, root)
		mkdirAll(t, filepath.Join(root, "a"), filepath.Join(root, "b"))

		dirs, err := library.ListDirectories("", false)
		if err != nil {
			t.Fatalf("ListDirectories returned error: %v", err)
		}

		want := []string{"a", "b"}
		if len(dirs) != len(want) {
			t.Fatalf("dirs = %v, want %v", dirs, want)
		}
		for i, d := range dirs {
			if d != want[i] {
				t.Fatalf("dirs[%d] = %q, want %q", i, d, want[i])
			}
		}
	})

	t.Run("lists directories recursively", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		library := openTestLibrary(t, root)
		mkdirAll(t, filepath.Join(root, "a", "sub"), filepath.Join(root, "b"))

		dirs, err := library.ListDirectories("", true)
		if err != nil {
			t.Fatalf("ListDirectories returned error: %v", err)
		}

		want := []string{"a", "a/sub", "b"}
		if len(dirs) != len(want) {
			t.Fatalf("dirs = %v, want %v", dirs, want)
		}
		for i, d := range dirs {
			if d != want[i] {
				t.Fatalf("dirs[%d] = %q, want %q", i, d, want[i])
			}
		}
	})

	t.Run("excludes config directory", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		library := openTestLibrary(t, root)
		mkdirAll(t, filepath.Join(root, shelff.ConfigDir), filepath.Join(root, "a"))

		dirs, err := library.ListDirectories("", true)
		if err != nil {
			t.Fatalf("ListDirectories returned error: %v", err)
		}

		for _, d := range dirs {
			if d == shelff.ConfigDir {
				t.Fatalf("config directory should be excluded, got %v", dirs)
			}
		}
	})

	t.Run("lists from subdirectory", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		library := openTestLibrary(t, root)
		mkdirAll(t, filepath.Join(root, "a", "x"), filepath.Join(root, "a", "y"))

		dirs, err := library.ListDirectories("a", false)
		if err != nil {
			t.Fatalf("ListDirectories returned error: %v", err)
		}

		want := []string{"a/x", "a/y"}
		if len(dirs) != len(want) {
			t.Fatalf("dirs = %v, want %v", dirs, want)
		}
		for i, d := range dirs {
			if d != want[i] {
				t.Fatalf("dirs[%d] = %q, want %q", i, d, want[i])
			}
		}
	})

	t.Run("rejects path traversal", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		library := openTestLibrary(t, root)

		_, err := library.ListDirectories("../outside", false)
		if err == nil {
			t.Fatal("expected error for path traversal, got nil")
		}
	})

	t.Run("returns empty for empty library", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		library := openTestLibrary(t, root)

		dirs, err := library.ListDirectories("", false)
		if err != nil {
			t.Fatalf("ListDirectories returned error: %v", err)
		}
		if len(dirs) != 0 {
			t.Fatalf("dirs = %v, want empty", dirs)
		}
	})
}

func TestDeleteDirectory(t *testing.T) {
	t.Parallel()

	t.Run("deletes an empty directory", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		library := openTestLibrary(t, root)
		mkdirAll(t, filepath.Join(root, "empty"))

		err := library.DeleteDirectory("empty")
		if err != nil {
			t.Fatalf("DeleteDirectory returned error: %v", err)
		}

		_, err = os.Stat(filepath.Join(root, "empty"))
		if !os.IsNotExist(err) {
			t.Fatal("directory still exists after delete")
		}
	})

	t.Run("refuses to delete non-empty directory", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		library := openTestLibrary(t, root)
		mkdirAll(t, filepath.Join(root, "has-stuff"))
		writeTestPDF(t, filepath.Join(root, "has-stuff"), "book.pdf")

		err := library.DeleteDirectory("has-stuff")
		if err == nil {
			t.Fatal("expected error for non-empty directory, got nil")
		}
	})

	t.Run("rejects path traversal", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		library := openTestLibrary(t, root)

		err := library.DeleteDirectory("../outside")
		if err == nil {
			t.Fatal("expected error for path traversal, got nil")
		}
	})

	t.Run("rejects config directory", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		library := openTestLibrary(t, root)
		mkdirAll(t, filepath.Join(root, shelff.ConfigDir))

		err := library.DeleteDirectory(shelff.ConfigDir)
		if err == nil {
			t.Fatal("expected error for config directory, got nil")
		}
	})

	t.Run("rejects empty path", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		library := openTestLibrary(t, root)

		err := library.DeleteDirectory("")
		if err == nil {
			t.Fatal("expected error for empty path, got nil")
		}
	})

	t.Run("returns error for non-existent directory", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		library := openTestLibrary(t, root)

		err := library.DeleteDirectory("no-such-dir")
		if err == nil {
			t.Fatal("expected error for non-existent directory, got nil")
		}
	})
}
