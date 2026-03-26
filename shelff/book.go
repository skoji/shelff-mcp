package shelff

import (
	"fmt"
	"os"
	"path/filepath"
)

// MoveBook moves a PDF and its sidecar (if present) to a destination directory.
func MoveBook(pdfPath string, destDir string) (newPDFPath string, err error) {
	if err := ensurePDFFile(pdfPath); err != nil {
		return "", err
	}
	if err := ensureDirectory(destDir); err != nil {
		return "", err
	}

	newPDFPath = filepath.Join(destDir, filepath.Base(pdfPath))
	if err := ensurePathDoesNotExist(newPDFPath); err != nil {
		return "", err
	}
	if err := os.Rename(pdfPath, newPDFPath); err != nil {
		return "", mapAlreadyExistsError(newPDFPath, err)
	}

	if err := moveSidecarWithRollback(pdfPath, newPDFPath); err != nil {
		return "", err
	}

	return newPDFPath, nil
}

// RenameBook renames a PDF and its sidecar (if present) within the same directory.
func RenameBook(pdfPath string, newName string) (newPDFPath string, err error) {
	if err := ensurePDFFile(pdfPath); err != nil {
		return "", err
	}

	newPDFPath = filepath.Join(filepath.Dir(pdfPath), newName+".pdf")
	if err := ensurePathDoesNotExist(newPDFPath); err != nil {
		return "", err
	}
	if err := os.Rename(pdfPath, newPDFPath); err != nil {
		return "", mapAlreadyExistsError(newPDFPath, err)
	}

	if err := moveSidecarWithRollback(pdfPath, newPDFPath); err != nil {
		return "", err
	}

	return newPDFPath, nil
}

// DeleteBook deletes a PDF and its sidecar (if present).
func DeleteBook(pdfPath string) error {
	if err := ensurePDFFile(pdfPath); err != nil {
		return err
	}
	if err := os.Remove(pdfPath); err != nil {
		return err
	}
	return DeleteSidecar(pdfPath)
}

func moveSidecarWithRollback(oldPDFPath string, newPDFPath string) error {
	oldSidecarPath := SidecarPath(oldPDFPath)
	newSidecarPath := SidecarPath(newPDFPath)

	info, err := os.Stat(oldSidecarPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return rollbackBookMove(newPDFPath, oldPDFPath, err)
	}
	if info.IsDir() {
		return rollbackBookMove(newPDFPath, oldPDFPath, fmt.Errorf("sidecar path is a directory: %s", oldSidecarPath))
	}

	if err := ensurePathDoesNotExist(newSidecarPath); err != nil {
		return rollbackBookMove(newPDFPath, oldPDFPath, err)
	}
	if err := os.Rename(oldSidecarPath, newSidecarPath); err != nil {
		return rollbackBookMove(newPDFPath, oldPDFPath, mapAlreadyExistsError(newSidecarPath, err))
	}

	return nil
}

func rollbackBookMove(currentPDFPath string, originalPDFPath string, originalErr error) error {
	rollbackErr := os.Rename(currentPDFPath, originalPDFPath)
	if rollbackErr != nil {
		return &RollbackError{
			OriginalError: originalErr,
			RollbackError: rollbackErr,
		}
	}
	return originalErr
}

func ensurePDFFile(pdfPath string) error {
	info, err := os.Stat(pdfPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrPDFNotFound
		}
		return err
	}
	if info.IsDir() {
		return ErrPDFNotFound
	}
	return nil
}

func ensureDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}
	return nil
}

func ensurePathDoesNotExist(path string) error {
	_, err := os.Stat(path)
	if err == nil {
		return ErrAlreadyExists
	}
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func mapAlreadyExistsError(path string, err error) error {
	if err == nil {
		return nil
	}
	if existsErr := ensurePathDoesNotExist(path); existsErr == ErrAlreadyExists {
		return ErrAlreadyExists
	}
	return err
}
