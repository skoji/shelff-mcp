package shelff_test

import (
	"errors"
	"testing"

	"github.com/skoji/shelff-go/shelff"
)

func TestRollbackErrorUnwrapIncludesOriginalAndRollbackErrors(t *testing.T) {
	t.Parallel()

	originalErr := errors.New("original failure")
	rollbackErr := errors.New("rollback failure")
	err := &shelff.RollbackError{
		OriginalError: originalErr,
		RollbackError: rollbackErr,
	}

	if !errors.Is(err, originalErr) {
		t.Fatalf("errors.Is(err, originalErr) = false, want true")
	}
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("errors.Is(err, rollbackErr) = false, want true")
	}
}

func TestRollbackErrorUnwrapOmitsNilMembers(t *testing.T) {
	t.Parallel()

	originalErr := errors.New("original failure")
	err := &shelff.RollbackError{
		OriginalError: originalErr,
	}

	if !errors.Is(err, originalErr) {
		t.Fatalf("errors.Is(err, originalErr) = false, want true")
	}

	otherErr := errors.New("other failure")
	if errors.Is(err, otherErr) {
		t.Fatalf("errors.Is(err, otherErr) = true, want false")
	}
}
