package shelff_test

import (
	"testing"

	"github.com/skoji/shelff-go/shelff"
)

func TestDirectionValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value shelff.Direction
		want  bool
	}{
		{"LTR", shelff.DirectionLTR, true},
		{"RTL", shelff.DirectionRTL, true},
		{"empty", shelff.Direction(""), false},
		{"invalid", shelff.Direction("DIAGONAL"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.value.Valid(); got != tt.want {
				t.Fatalf("Direction(%q).Valid() = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestPageLayoutValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value shelff.PageLayout
		want  bool
	}{
		{"single", shelff.LayoutSingle, true},
		{"spread", shelff.LayoutSpread, true},
		{"spread-with-cover", shelff.LayoutSpreadWithCover, true},
		{"empty", shelff.PageLayout(""), false},
		{"invalid", shelff.PageLayout("double"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.value.Valid(); got != tt.want {
				t.Fatalf("PageLayout(%q).Valid() = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestReadingStatusValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value shelff.ReadingStatus
		want  bool
	}{
		{"unread", shelff.StatusUnread, true},
		{"reading", shelff.StatusReading, true},
		{"finished", shelff.StatusFinished, true},
		{"empty", shelff.ReadingStatus(""), false},
		{"invalid", shelff.ReadingStatus("paused"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.value.Valid(); got != tt.want {
				t.Fatalf("ReadingStatus(%q).Valid() = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
