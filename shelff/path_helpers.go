package shelff

import (
	"path/filepath"
	"strings"
)

// SidecarPath returns the sidecar JSON path for a given PDF path.
func SidecarPath(pdfPath string) string {
	return pdfPath + SidecarSuffix
}

// PDFPathFromSidecar returns the PDF path for a given sidecar path.
func PDFPathFromSidecar(sidecarPath string) (string, bool) {
	if !strings.HasSuffix(sidecarPath, SidecarSuffix) {
		return "", false
	}

	pdfPath := strings.TrimSuffix(sidecarPath, SidecarSuffix)
	if !strings.EqualFold(filepath.Ext(pdfPath), ".pdf") {
		return "", false
	}

	return pdfPath, true
}

// IsSidecarPath reports whether the given path looks like a shelff sidecar file.
func IsSidecarPath(path string) bool {
	_, ok := PDFPathFromSidecar(path)
	return ok
}
