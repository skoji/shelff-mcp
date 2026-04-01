package shelff_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/skoji/shelff-mcp/shelff"
)

func TestMoveBookMovesPDFAndSidecar(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	destDir := filepath.Join(root, "dest")
	mkdirAll(t, sourceDir, destDir)

	pdfPath := writeTestPDF(t, sourceDir, "book.pdf")
	if _, err := shelff.CreateSidecar(pdfPath); err != nil {
		t.Fatalf("CreateSidecar returned error: %v", err)
	}

	newPDFPath, err := shelff.MoveBook(pdfPath, destDir)
	if err != nil {
		t.Fatalf("MoveBook returned error: %v", err)
	}

	wantPDFPath := filepath.Join(destDir, "book.pdf")
	if newPDFPath != wantPDFPath {
		t.Fatalf("newPDFPath = %q, want %q", newPDFPath, wantPDFPath)
	}
	assertPathExists(t, newPDFPath)
	assertPathExists(t, shelff.SidecarPath(newPDFPath))
	assertPathMissing(t, pdfPath)
	assertPathMissing(t, shelff.SidecarPath(pdfPath))
}

func TestMoveBookWithoutSidecarMovesOnlyPDF(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	destDir := filepath.Join(root, "dest")
	mkdirAll(t, sourceDir, destDir)

	pdfPath := writeTestPDF(t, sourceDir, "book.pdf")

	newPDFPath, err := shelff.MoveBook(pdfPath, destDir)
	if err != nil {
		t.Fatalf("MoveBook returned error: %v", err)
	}

	assertPathExists(t, newPDFPath)
	assertPathMissing(t, shelff.SidecarPath(newPDFPath))
	assertPathMissing(t, pdfPath)
}

func TestMoveBookReturnsErrAlreadyExistsWhenDestinationPDFExists(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	destDir := filepath.Join(root, "dest")
	mkdirAll(t, sourceDir, destDir)

	pdfPath := writeTestPDF(t, sourceDir, "book.pdf")
	writeTestPDF(t, destDir, "book.pdf")

	_, err := shelff.MoveBook(pdfPath, destDir)
	if !errors.Is(err, shelff.ErrAlreadyExists) {
		t.Fatalf("MoveBook error = %v, want ErrAlreadyExists", err)
	}

	assertPathExists(t, pdfPath)
}

func TestMoveBookReturnsErrPDFNotFoundWhenSourceIsMissing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	destDir := filepath.Join(root, "dest")
	mkdirAll(t, destDir)

	_, err := shelff.MoveBook(filepath.Join(root, "missing.pdf"), destDir)
	if !errors.Is(err, shelff.ErrPDFNotFound) {
		t.Fatalf("MoveBook error = %v, want ErrPDFNotFound", err)
	}
}

func TestMoveBookReturnsErrorWhenDestinationDirectoryIsMissing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	mkdirAll(t, sourceDir)

	pdfPath := writeTestPDF(t, sourceDir, "book.pdf")
	_, err := shelff.MoveBook(pdfPath, filepath.Join(root, "missing-dest"))
	if err == nil {
		t.Fatal("MoveBook error = nil, want error for missing destination directory")
	}

	assertPathExists(t, pdfPath)
}

func TestMoveBookRollsBackWhenSidecarMoveFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	destDir := filepath.Join(root, "dest")
	mkdirAll(t, sourceDir, destDir)

	pdfPath := writeTestPDF(t, sourceDir, "book.pdf")
	if _, err := shelff.CreateSidecar(pdfPath); err != nil {
		t.Fatalf("CreateSidecar returned error: %v", err)
	}
	if err := os.Mkdir(filepath.Join(destDir, "book.pdf.meta.json"), 0o755); err != nil {
		t.Fatalf("os.Mkdir: %v", err)
	}

	_, err := shelff.MoveBook(pdfPath, destDir)
	if !errors.Is(err, shelff.ErrAlreadyExists) {
		t.Fatalf("MoveBook error = %v, want ErrAlreadyExists", err)
	}

	assertPathExists(t, pdfPath)
	assertPathExists(t, shelff.SidecarPath(pdfPath))
	assertPathMissing(t, filepath.Join(destDir, "book.pdf"))
}

func TestMoveBookMovesBrokenSymlinkSidecar(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	destDir := filepath.Join(root, "dest")
	mkdirAll(t, sourceDir, destDir)

	pdfPath := writeTestPDF(t, sourceDir, "book.pdf")
	sidecarPath := shelff.SidecarPath(pdfPath)
	if err := os.Symlink(filepath.Join(root, "missing-sidecar-target"), sidecarPath); err != nil {
		t.Skipf("os.Symlink unavailable: %v", err)
	}

	newPDFPath, err := shelff.MoveBook(pdfPath, destDir)
	if err != nil {
		t.Fatalf("MoveBook returned error: %v", err)
	}

	newSidecarPath := shelff.SidecarPath(newPDFPath)
	assertPathExists(t, newPDFPath)
	assertPathExistsWithLstat(t, newSidecarPath)
	assertPathMissingWithLstat(t, sidecarPath)

	target, err := os.Readlink(newSidecarPath)
	if err != nil {
		t.Fatalf("os.Readlink(%q): %v", newSidecarPath, err)
	}
	if target != filepath.Join(root, "missing-sidecar-target") {
		t.Fatalf("symlink target = %q, want %q", target, filepath.Join(root, "missing-sidecar-target"))
	}
}

func TestRenameBookRenamesPDFAndSidecar(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	if _, err := shelff.CreateSidecar(pdfPath); err != nil {
		t.Fatalf("CreateSidecar returned error: %v", err)
	}

	newPDFPath, err := shelff.RenameBook(pdfPath, "renamed")
	if err != nil {
		t.Fatalf("RenameBook returned error: %v", err)
	}

	wantPDFPath := filepath.Join(root, "renamed.pdf")
	if newPDFPath != wantPDFPath {
		t.Fatalf("newPDFPath = %q, want %q", newPDFPath, wantPDFPath)
	}
	assertPathExists(t, newPDFPath)
	assertPathExists(t, shelff.SidecarPath(newPDFPath))
	assertPathMissing(t, pdfPath)
	assertPathMissing(t, shelff.SidecarPath(pdfPath))
}

func TestRenameBookAcceptsOptionalPDFExtension(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	if _, err := shelff.CreateSidecar(pdfPath); err != nil {
		t.Fatalf("CreateSidecar returned error: %v", err)
	}

	newPDFPath, err := shelff.RenameBook(pdfPath, "renamed.pdf")
	if err != nil {
		t.Fatalf("RenameBook returned error: %v", err)
	}

	wantPDFPath := filepath.Join(root, "renamed.pdf")
	if newPDFPath != wantPDFPath {
		t.Fatalf("newPDFPath = %q, want %q", newPDFPath, wantPDFPath)
	}
	assertPathExists(t, newPDFPath)
	assertPathExists(t, shelff.SidecarPath(newPDFPath))
	assertPathMissing(t, pdfPath)
}

func TestRenameBookRollsBackWhenSidecarRenameFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	if _, err := shelff.CreateSidecar(pdfPath); err != nil {
		t.Fatalf("CreateSidecar returned error: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "renamed.pdf.meta.json"), 0o755); err != nil {
		t.Fatalf("os.Mkdir: %v", err)
	}

	_, err := shelff.RenameBook(pdfPath, "renamed")
	if !errors.Is(err, shelff.ErrAlreadyExists) {
		t.Fatalf("RenameBook error = %v, want ErrAlreadyExists", err)
	}

	assertPathExists(t, pdfPath)
	assertPathExists(t, shelff.SidecarPath(pdfPath))
	assertPathMissing(t, filepath.Join(root, "renamed.pdf"))
}

func TestRenameBookReturnsErrPDFNotFoundWhenSourceIsMissing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	_, err := shelff.RenameBook(filepath.Join(root, "missing.pdf"), "renamed")
	if !errors.Is(err, shelff.ErrPDFNotFound) {
		t.Fatalf("RenameBook error = %v, want ErrPDFNotFound", err)
	}
}

func TestRenameBookReturnsErrAlreadyExistsWhenTargetPDFExists(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	writeTestPDF(t, root, "renamed.pdf")

	_, err := shelff.RenameBook(pdfPath, "renamed")
	if !errors.Is(err, shelff.ErrAlreadyExists) {
		t.Fatalf("RenameBook error = %v, want ErrAlreadyExists", err)
	}

	assertPathExists(t, pdfPath)
}

func TestRenameBookReturnsErrEmptyNameForBlankTarget(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")

	_, err := shelff.RenameBook(pdfPath, "   ")
	if !errors.Is(err, shelff.ErrEmptyName) {
		t.Fatalf("RenameBook error = %v, want ErrEmptyName", err)
	}

	assertPathExists(t, pdfPath)
}

func TestRenameBookRejectsNonBaseNames(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")

	testCases := []string{
		"nested/renamed",
		`nested\renamed`,
		".",
		"..",
	}

	for _, newName := range testCases {
		_, err := shelff.RenameBook(pdfPath, newName)
		if err == nil {
			t.Fatalf("RenameBook(%q) error = nil, want error", newName)
		}
		assertPathExists(t, pdfPath)
	}
}

func TestDeleteBookDeletesPDFAndSidecar(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	if _, err := shelff.CreateSidecar(pdfPath); err != nil {
		t.Fatalf("CreateSidecar returned error: %v", err)
	}

	if err := shelff.DeleteBook(pdfPath); err != nil {
		t.Fatalf("DeleteBook returned error: %v", err)
	}

	assertPathMissing(t, pdfPath)
	assertPathMissing(t, shelff.SidecarPath(pdfPath))
}

func TestDeleteBookRollsBackPDFWhenSidecarDeleteFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")
	sidecarPath := shelff.SidecarPath(pdfPath)
	if _, err := shelff.CreateSidecar(pdfPath); err != nil {
		t.Fatalf("CreateSidecar returned error: %v", err)
	}
	if err := os.Remove(sidecarPath); err != nil {
		t.Fatalf("os.Remove(%q): %v", sidecarPath, err)
	}
	if err := os.Mkdir(sidecarPath, 0o755); err != nil {
		t.Fatalf("os.Mkdir(%q): %v", sidecarPath, err)
	}
	if err := os.WriteFile(filepath.Join(sidecarPath, "nested"), []byte("x"), 0o644); err != nil {
		t.Fatalf("os.WriteFile nested file: %v", err)
	}

	err := shelff.DeleteBook(pdfPath)
	if err == nil {
		t.Fatal("DeleteBook error = nil, want error")
	}

	assertPathExists(t, pdfPath)
	assertPathExistsWithLstat(t, sidecarPath)
	assertPathMissing(t, filepath.Join(root, "book.pdf.deleting"))
}

func TestDeleteBookReturnsErrPDFNotFoundWhenSourceIsMissing(t *testing.T) {
	t.Parallel()

	err := shelff.DeleteBook(filepath.Join(t.TempDir(), "missing.pdf"))
	if !errors.Is(err, shelff.ErrPDFNotFound) {
		t.Fatalf("DeleteBook error = %v, want ErrPDFNotFound", err)
	}
}

func TestDeleteBookWithoutSidecarDeletesOnlyPDF(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := writeTestPDF(t, root, "book.pdf")

	if err := shelff.DeleteBook(pdfPath); err != nil {
		t.Fatalf("DeleteBook returned error: %v", err)
	}

	assertPathMissing(t, pdfPath)
	assertPathMissing(t, shelff.SidecarPath(pdfPath))
}

func TestMoveBookTreatsBrokenSymlinkDestinationAsExisting(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	destDir := filepath.Join(root, "dest")
	mkdirAll(t, sourceDir, destDir)

	pdfPath := writeTestPDF(t, sourceDir, "book.pdf")
	destPDFPath := filepath.Join(destDir, "book.pdf")
	if err := os.Symlink(filepath.Join(root, "missing-target"), destPDFPath); err != nil {
		t.Skipf("os.Symlink unavailable: %v", err)
	}

	_, err := shelff.MoveBook(pdfPath, destDir)
	if !errors.Is(err, shelff.ErrAlreadyExists) {
		t.Fatalf("MoveBook error = %v, want ErrAlreadyExists", err)
	}

	assertPathExists(t, pdfPath)
	assertPathExistsWithLstat(t, destPDFPath)
}

func mkdirAll(t *testing.T, paths ...string) {
	t.Helper()

	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("os.MkdirAll(%q): %v", path, err)
		}
	}
}

func assertPathExists(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected path %q to exist, stat err = %v", path, err)
	}
}

func assertPathExistsWithLstat(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("expected path %q to exist, lstat err = %v", path, err)
	}
}

func assertPathMissing(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected path %q to be missing, stat err = %v", path, err)
	}
}

func assertPathMissingWithLstat(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("expected path %q to be missing, lstat err = %v", path, err)
	}
}
