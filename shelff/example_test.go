package shelff_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/skoji/shelff-mcp/shelff"
)

func Example() {
	root, err := os.MkdirTemp("", "shelff-example-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(root)

	lib, err := shelff.OpenLibrary(root)
	if err != nil {
		panic(err)
	}

	pdfPath := filepath.Join(root, "Example Book.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.7\n"), 0o644); err != nil {
		panic(err)
	}

	meta, err := shelff.CreateSidecar(pdfPath)
	if err != nil {
		panic(err)
	}
	meta.Metadata.Creator = []string{"Example Author"}
	meta.Tags = []string{"example", "docs"}

	if err := lib.AddCategory("Examples"); err != nil {
		panic(err)
	}
	meta.Category = stringPtr("Examples")

	if err := shelff.WriteSidecar(pdfPath, meta); err != nil {
		panic(err)
	}

	books, err := lib.ScanBooks(true)
	if err != nil {
		panic(err)
	}

	fmt.Println("books:", len(books))
	// Output:
	// books: 1
}

func ExampleLibrary_Stats() {
	root, err := os.MkdirTemp("", "shelff-stats-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(root)

	lib, err := shelff.OpenLibrary(root)
	if err != nil {
		panic(err)
	}

	pdfPath := filepath.Join(root, "Report.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.7\n"), 0o644); err != nil {
		panic(err)
	}
	meta, err := shelff.CreateSidecar(pdfPath)
	if err != nil {
		panic(err)
	}
	meta.Tags = []string{"reporting", "stats"}
	if err := shelff.WriteSidecar(pdfPath, meta); err != nil {
		panic(err)
	}

	stats, err := lib.Stats()
	if err != nil {
		panic(err)
	}

	fmt.Println("total:", stats.TotalPDFs)
	fmt.Println("with sidecar:", stats.WithSidecar)
	// Output:
	// total: 1
	// with sidecar: 1
}

func ExampleLibrary_Validate() {
	root, err := os.MkdirTemp("", "shelff-validate-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(root)

	lib, err := shelff.OpenLibrary(root)
	if err != nil {
		panic(err)
	}

	pdfPath := filepath.Join(root, "Valid.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.7\n"), 0o644); err != nil {
		panic(err)
	}
	if _, err := shelff.CreateSidecar(pdfPath); err != nil {
		panic(err)
	}

	validationErrors, err := lib.Validate(pdfPath)
	if err != nil {
		panic(err)
	}

	fmt.Println("errors:", len(validationErrors))
	// Output:
	// errors: 0
}

func stringPtr(value string) *string {
	return &value
}
