package shelff_test

import (
	"testing"

	"github.com/skoji/shelff-go/shelff"
)

func TestSidecarPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pdfPath string
		want    string
	}{
		{
			name:    "simple pdf",
			pdfPath: "/library/book.pdf",
			want:    "/library/book.pdf.meta.json",
		},
		{
			name:    "spaces in filename",
			pdfPath: "/library/My Report.pdf",
			want:    "/library/My Report.pdf.meta.json",
		},
		{
			name:    "uppercase extension preserved",
			pdfPath: "/library/BOOK.PDF",
			want:    "/library/BOOK.PDF.meta.json",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := shelff.SidecarPath(tt.pdfPath); got != tt.want {
				t.Fatalf("SidecarPath(%q) = %q, want %q", tt.pdfPath, got, tt.want)
			}
		})
	}
}

func TestPDFPathFromSidecar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		sidecar string
		wantPDF string
		wantOK  bool
	}{
		{
			name:    "lowercase pdf sidecar",
			sidecar: "/library/book.pdf.meta.json",
			wantPDF: "/library/book.pdf",
			wantOK:  true,
		},
		{
			name:    "uppercase pdf extension sidecar",
			sidecar: "/library/BOOK.PDF.meta.json",
			wantPDF: "/library/BOOK.PDF",
			wantOK:  true,
		},
		{
			name:    "missing pdf extension before suffix",
			sidecar: "/library/book.meta.json",
			wantOK:  false,
		},
		{
			name:    "wrong suffix",
			sidecar: "/library/book.pdf.json",
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotPDF, gotOK := shelff.PDFPathFromSidecar(tt.sidecar)
			if gotOK != tt.wantOK {
				t.Fatalf("PDFPathFromSidecar(%q) ok = %v, want %v", tt.sidecar, gotOK, tt.wantOK)
			}
			if gotPDF != tt.wantPDF {
				t.Fatalf("PDFPathFromSidecar(%q) pdf = %q, want %q", tt.sidecar, gotPDF, tt.wantPDF)
			}
		})
	}
}

func TestIsSidecarPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		{path: "/library/book.pdf.meta.json", want: true},
		{path: "/library/BOOK.PDF.meta.json", want: true},
		{path: "/library/book.meta.json", want: false},
		{path: "/library/book.pdf", want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()

			if got := shelff.IsSidecarPath(tt.path); got != tt.want {
				t.Fatalf("IsSidecarPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
