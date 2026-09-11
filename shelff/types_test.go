package shelff_test

import (
	"math"
	"testing"

	"github.com/skoji/shelff-mcp/shelff"
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

func TestCropInsetsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value shelff.CropInsets
		want  bool
	}{
		{"all zero", shelff.CropInsets{}, true},
		{"typical", shelff.CropInsets{Top: 0.05, Bottom: 0.04, Left: 0.03, Right: 0.03}, true},
		{"just under the vertical limit", shelff.CropInsets{Top: 0.5, Bottom: 0.49}, true},
		{"negative top", shelff.CropInsets{Top: -0.01}, false},
		{"negative right", shelff.CropInsets{Right: -0.5}, false},
		{"vertical sum equals one", shelff.CropInsets{Top: 0.5, Bottom: 0.5}, false},
		{"horizontal sum exceeds one", shelff.CropInsets{Left: 0.6, Right: 0.5}, false},
		{"single side above one", shelff.CropInsets{Top: 1.5}, false},
		{"NaN", shelff.CropInsets{Top: math.NaN()}, false},
		{"positive infinity", shelff.CropInsets{Left: math.Inf(1)}, false},
		{"negative infinity", shelff.CropInsets{Bottom: math.Inf(-1)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.value.Valid(); got != tt.want {
				t.Fatalf("CropInsets(%+v).Valid() = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
