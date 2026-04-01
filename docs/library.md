# shelff Go Library

The `shelff` package is a Go library for working with [shelff](https://skoji.dev/en/shelff/) libraries.

```bash
go get github.com/skoji/shelff-mcp/shelff
```

The library currently requires Go 1.25 or later.

## What the library manages

A shelff library consists of:

- PDF files anywhere under a library root
- sidecar files next to PDFs, using `*.pdf.meta.json`
- `.shelff/categories.json` for category definitions
- `.shelff/tags.json` for tag display order

The library provides APIs for:

- reading, creating, updating, and deleting sidecar metadata
- moving, renaming, and deleting PDF + sidecar pairs
- managing categories and tag order
- scanning a library for books and orphaned sidecars
- validating sidecars against the embedded schema
- computing library statistics and collecting tags

## Quick start

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/skoji/shelff-mcp/shelff"
)

func main() {
	root := "/path/to/library"

	lib, err := shelff.OpenLibrary(root)
	if err != nil {
		panic(err)
	}

	pdfPath := filepath.Join(root, "Go in Action.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.7\n"), 0o644); err != nil {
		panic(err)
	}

	meta, err := shelff.CreateSidecar(pdfPath)
	if err != nil {
		panic(err)
	}

	creator := []string{"William Kennedy", "Brian Ketelsen", "Erik St. Martin"}
	meta.Metadata.Creator = creator
	meta.Tags = []string{"go", "programming"}

	if err := lib.AddCategory("Programming"); err != nil {
		panic(err)
	}
	category := "Programming"
	meta.Category = &category

	if err := shelff.WriteSidecar(pdfPath, meta); err != nil {
		panic(err)
	}

	books, err := lib.ScanBooks(true)
	if err != nil {
		panic(err)
	}

	fmt.Printf("books: %d\n", len(books))
}
```

## Core concepts

### Sidecars

Every PDF can have a sidecar JSON file at:

```text
book.pdf.meta.json
```

Use these APIs for sidecar lifecycle:

- `shelff.ReadSidecar(pdfPath)`
- `shelff.CreateSidecar(pdfPath)`
- `shelff.WriteSidecar(pdfPath, meta)`
- `shelff.DeleteSidecar(pdfPath)`

`ReadSidecar` returns `(*SidecarMetadata, nil)` when the sidecar exists, and `(nil, nil)` when it does not.

`CreateSidecar` writes a minimal sidecar to disk immediately, then returns the created `*SidecarMetadata`. If you want to add more metadata, update the returned object and call `WriteSidecar` again.

### Categories and tags

`ReadCategories` returns `(*CategoryList, nil)` when `.shelff/categories.json` exists, and `(nil, nil)` when it does not.

`ReadTagOrder` returns `(*TagOrder, nil)` when `.shelff/tags.json` exists, and `(nil, nil)` when it does not.

Categories and tags are managed differently:

- categories are defined in `.shelff/categories.json`
- tags are discovered from sidecars; `.shelff/tags.json` only controls display order

Use these APIs:

- categories: `ReadCategories`, `WriteCategories`, `AddCategory`, `RemoveCategory`, `RenameCategory`, `ReorderCategories`
- tags: `ReadTagOrder`, `WriteTagOrder`, `AddTagToOrder`, `RemoveTagFromOrder`, `RenameTag`, `ReorderTags`

### Query APIs

Once a library is open, you can inspect it with:

- `ScanBooks(recursive bool)`
- `ScanBooksInDirectory(directoryPath string, recursive bool)`
- `FindOrphanedSidecars()`
- `Stats()`
- `CollectAllTags()`
- `CheckLibrary()`
- `Validate(pdfPath)`

## Common usage patterns

### Read metadata for existing PDFs

```go
books, err := lib.ScanBooks(true)
if err != nil {
	panic(err)
}

for _, book := range books {
	if !book.HasSidecar {
		continue
	}

	meta, err := shelff.ReadSidecar(book.PDFPath)
	if err != nil {
		panic(err)
	}
	fmt.Println(meta.Metadata.Title, meta.Tags)
}
```

### Rename a category and cascade updates into sidecars

```go
if err := lib.RenameCategory("Tech", "Technology", true); err != nil {
	panic(err)
}
```

### Rename a tag and deduplicate on collision

If a rename would produce duplicates inside a single sidecar, the resulting tag list is deduplicated.

```go
if err := lib.RenameTag("golang", "go", true); err != nil {
	panic(err)
}
```

### Validate a sidecar before import or sync

```go
validationErrors, err := lib.Validate("/path/to/book.pdf")
if err != nil {
	panic(err)
}
if len(validationErrors) > 0 {
	fmt.Println("sidecar is invalid:")
	for _, msg := range validationErrors {
		fmt.Println(" -", msg)
	}
}
```

### Generate library statistics

```go
stats, err := lib.Stats()
if err != nil {
	panic(err)
}

fmt.Println("total PDFs:", stats.TotalPDFs)
fmt.Println("with sidecar:", stats.WithSidecar)
fmt.Println("orphaned sidecars:", stats.OrphanedSidecars)
fmt.Println("tag counts:", stats.TagCounts)
```

## Important behaviors

### Unknown JSON fields are preserved

When a sidecar, category list, or tag order file is read and then written back through the library, unknown top-level JSON fields from the original document are preserved.

This is important for forward compatibility with other shelff producers.

### Writes are atomic

JSON writes use atomic replacement. Existing file permissions are preserved, and new files default to `0644`.

### Book operations protect against half-moves

`MoveBook` and `RenameBook` move the PDF first and then the sidecar. If the sidecar move fails, the PDF move is rolled back. If rollback also fails, the library returns a `*RollbackError`.

### Cascade updates are not transactional

Category/tag cascade operations update sidecars sequentially. If one sidecar write fails, already-written sidecars are not rolled back.

### Scan rules

- `ScanBooks(false)` scans only the library root
- `ScanBooks(true)` scans recursively
- `.shelff/` is always excluded from scanning
- symlinks are skipped and not followed

### Normalization rules

- category and tag names are trimmed before validation
- `RenameBook` accepts names with or without a `.pdf` suffix
- sidecar reading timestamps are normalized to UTC on write

## API overview

### Package-level helpers

- `OpenLibrary(rootDir string) (*Library, error)`
- `SidecarPath(pdfPath string) string`
- `PDFPathFromSidecar(sidecarPath string) (string, bool)`
- `IsSidecarPath(path string) bool`
- `ReadSidecar(pdfPath string) (*SidecarMetadata, error)`
- `CreateSidecar(pdfPath string) (*SidecarMetadata, error)`
- `WriteSidecar(pdfPath string, meta *SidecarMetadata) error`
- `DeleteSidecar(pdfPath string) error`
- `MoveBook(pdfPath string, destDir string) (string, error)`
- `RenameBook(pdfPath string, newName string) (string, error)`
- `DeleteBook(pdfPath string) error`

### Library methods

- `(*Library).Root() string`
- `(*Library).ReadCategories() (*CategoryList, error)`
- `(*Library).WriteCategories(cats *CategoryList) error`
- `(*Library).AddCategory(name string) error`
- `(*Library).RemoveCategory(name string, cascade bool) error`
- `(*Library).RenameCategory(oldName, newName string, cascade bool) error`
- `(*Library).ReorderCategories(names []string) error`
- `(*Library).ReadTagOrder() (*TagOrder, error)`
- `(*Library).WriteTagOrder(tags *TagOrder) error`
- `(*Library).AddTagToOrder(name string) error`
- `(*Library).RemoveTagFromOrder(name string, cascade bool) error`
- `(*Library).RenameTag(oldName, newName string, cascade bool) error`
- `(*Library).ReorderTags(names []string) error`
- `(*Library).ScanBooks(recursive bool) ([]BookEntry, error)`
- `(*Library).ScanBooksInDirectory(directoryPath string, recursive bool) ([]BookEntry, error)`
- `(*Library).FindOrphanedSidecars() ([]OrphanedSidecar, error)`
- `(*Library).Validate(pdfPath string) ([]string, error)`
- `(*Library).Stats() (*LibraryStats, error)`
- `(*Library).CollectAllTags() ([]string, error)`
- `(*Library).CheckLibrary() (*CheckLibraryResult, error)`

## Exported data types

The main exported structs are:

- `SidecarMetadata`
- `DublinCore`
- `ReadingProgress`
- `DisplaySettings`
- `CategoryList`
- `CategoryItem`
- `TagOrder`
- `BookEntry`
- `OrphanedSidecar`
- `LibraryStats`
- `CheckLibraryResult`
- `DotShelffStatus`
- `IntegrityReport`
- `LibrarySummary`

Useful exported constants include:

- `SchemaVersion`
- reading status values: `StatusUnread`, `StatusReading`, `StatusFinished`
- display direction values: `DirectionLTR`, `DirectionRTL`
- page layout values: `LayoutSingle`, `LayoutSpread`, `LayoutSpreadWithCover`

## Error handling

Some commonly returned errors:

- `ErrLibraryNotFound`
- `ErrPDFNotFound`
- `ErrSidecarNotFound`
- `ErrSidecarAlreadyExists`
- `ErrAlreadyExists`
- `ErrCategoryNotFound`
- `ErrCategoryAlreadyExists`
- `ErrTagAlreadyExists`
- `ErrCategoryMismatch`
- `ErrEmptyName`
- `ErrNilSidecarMetadata`

`RollbackError` indicates that both the original operation and the rollback failed.

## See also

- [shelff specification](https://github.com/skoji/shelff-schema/blob/main/SPECIFICATION.md)
- [shelff iOS/iPadOS app](https://skoji.dev/en/shelff/)
