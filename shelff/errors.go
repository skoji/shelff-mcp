package shelff

import (
	"errors"
	"fmt"
)

var (
	ErrPDFNotFound           = errors.New("pdf file not found")
	ErrSidecarAlreadyExists  = errors.New("sidecar file already exists")
	ErrSidecarNotFound       = errors.New("sidecar file not found")
	ErrAlreadyExists         = errors.New("destination file already exists")
	ErrCategoryNotFound      = errors.New("category not found")
	ErrCategoryAlreadyExists = errors.New("category already exists")
	ErrTagAlreadyExists      = errors.New("tag already exists in order list")
	ErrEmptyName             = errors.New("name is empty after trimming")
	ErrCategoryMismatch      = errors.New("category names do not match existing set")
	ErrInvalidSchemaVersion  = errors.New("unsupported schema version")
	ErrLibraryNotFound       = errors.New("library root is missing or not a directory")
	ErrNilSidecarMetadata    = errors.New("sidecar metadata is nil")
	ErrInvalidFieldValue     = errors.New("invalid field value")

	errUnexpectedTrailingData = errors.New("invalid JSON: unexpected trailing data")
)

// RollbackError is returned when an operation and its rollback both fail.
type RollbackError struct {
	OriginalError error
	RollbackError error
}

func (e *RollbackError) Error() string {
	return fmt.Sprintf("operation failed: %v; rollback failed: %v", e.OriginalError, e.RollbackError)
}

func (e *RollbackError) Unwrap() []error {
	var errs []error
	if e.OriginalError != nil {
		errs = append(errs, e.OriginalError)
	}
	if e.RollbackError != nil {
		errs = append(errs, e.RollbackError)
	}
	return errs
}
