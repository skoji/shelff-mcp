# shelff-go

`shelff-go` provides:

- a Go library for working with [shelff](https://skoji.dev/en/shelff/) libraries
- an MCP server, `shelff-mcp`, that exposes most of the library operations over stdio

## Installation

```bash
go get github.com/skoji/shelff-go/shelff
```

The repository currently requires Go 1.25 or later because the MCP server uses the official `github.com/modelcontextprotocol/go-sdk`.

To install the MCP server binary:

```bash
go install github.com/skoji/shelff-go/cmd/shelff-mcp@latest
```

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

	"github.com/skoji/shelff-go/shelff"
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
- `Validate(pdfPath)`

## MCP server

`shelff-mcp` runs as a stdio MCP server rooted at a single shelff library.

Start it with an explicit root:

```bash
shelff-mcp --root /path/to/library
```

Or use `SHELFF_ROOT`:

```bash
SHELFF_ROOT=/path/to/library shelff-mcp
```

### Root path rules

- the root must point at the shelff library directory itself
- tool paths are always relative to that root
- absolute paths are rejected
- path traversal outside the root is rejected, including symlink escapes

### macOS / iCloud note

On macOS, a shelff library stored in iCloud typically lives at:

```text
/Users/<user>/Library/Mobile Documents/iCloud~jp~skoji~shelff/Documents/
```

You can pass that directory directly as `--root` or `SHELFF_ROOT`.

Because the path contains a space in `Mobile Documents`, quote it in shell commands:

```bash
shelff-mcp --root "/Users/<user>/Library/Mobile Documents/iCloud~jp~skoji~shelff/Documents/"
SHELFF_ROOT="/Users/<user>/Library/Mobile Documents/iCloud~jp~skoji~shelff/Documents/" shelff-mcp
```

For large-scale edits, migrations, or batch mutations, it is safer to work on a copied library first and then sync the results back once you are satisfied.

### Exposed MCP tools

Read-only tools:

- `read_sidecar`
- `scan_books`
- `find_orphaned_sidecars`
- `validate_sidecar`
- `library_stats`
- `collect_all_tags`
- `read_categories`
- `read_tag_order`

Mutation tools:

- `create_sidecar`
- `write_sidecar`
- `delete_sidecar`
- `move_book`
- `rename_book`
- `add_category`
- `remove_category`
- `rename_category`
- `reorder_categories`
- `add_tag_to_order`
- `remove_tag_from_order`
- `rename_tag`
- `reorder_tags`

`DeleteBook` is intentionally not exposed via MCP, to reduce the risk of destructive PDF deletion from agent workflows.

### `scan_books` pagination and directory filtering

`scan_books` supports optional pagination and subtree filtering:

- `directory`: library-root-relative directory to scan from
- `limit`: maximum number of books to return, default `100`
- `offset`: number of filtered books to skip, default `0`

Filtering is applied first, then pagination. The response includes:

- `books`: the current page of results
- `total`: total number of books matching the filter before pagination
- `offset`: the applied offset
- `limit`: the applied limit
- `hasMore`: whether more matching books remain after this page

### `write_sidecar` semantics

`write_sidecar` is an MCP-layer convenience API that accepts a partial JSON object:

- in general, fields omitted from the input are left unchanged
- object fields are merged recursively
- arrays and scalar values replace the existing value
- in general, `null` deletes the targeted field when that is schema-safe
- `schemaVersion` is always normalized to the current `shelff.SchemaVersion`
- `metadata.dc:title` is preserved from the current sidecar when it is omitted from the patch or set to `null`
- if a patch removes a required key inside `reading` or `display`, normalization drops the entire `reading` or `display` object instead of leaving an invalid partial object behind
- if no sidecar exists yet, the tool creates one first and then applies the patch

The returned sidecar is the canonical persisted representation after normalization.

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
- `(*Library).FindOrphanedSidecars() ([]OrphanedSidecar, error)`
- `(*Library).Validate(pdfPath string) ([]string, error)`
- `(*Library).Stats() (*LibraryStats, error)`
- `(*Library).CollectAllTags() ([]string, error)`

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

- design document: [`design/shelff-go-design.md`](./design/shelff-go-design.md)
- schema source: [`shelff-schema/`](./shelff-schema/)
