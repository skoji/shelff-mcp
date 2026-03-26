# shelff-go Design Document

> This document is the authoritative implementation specification for shelff-go.
> An implementer should be able to build the complete library and MCP server from this document alone,
> referencing only the shelff-schema repository for JSON Schema definitions.

## 1. Overview

shelff-go is a Go library and MCP server for managing shelff metadata — the sidecar JSON files that accompany PDFs in a shelff library.

### 1.1 Purpose

- Provide a Go implementation of shelff's sidecar JSON read/write operations
- Serve as the foundation for a Shelff MCP server
- **Prove the portability of shelff's sidecar JSON specification** by implementing it in a language other than Swift — surfacing any implicit Swift dependencies in the format

### 1.2 What shelff-go is NOT

- Not a PDF reader or renderer
- Not a replacement for the shelff iOS app
- Not responsible for iCloud sync, NSFileCoordinator, or UIDocument lifecycle

### 1.3 Repository

- **Repository**: `github.com/skoji/shelff-go`
- **Schema**: `github.com/skoji/shelff-schema` (included as a git submodule)
- **License**: TBD

## 2. Project Setup

### 2.1 Go Module

```
module github.com/skoji/shelff-go

go 1.23
```

Minimum Go version: 1.23 (for `slices`, `maps` packages and `slog` in standard library).

### 2.2 Directory Structure

```
shelff-go/
├── shelff-schema/              # git submodule → github.com/skoji/shelff-schema
│   ├── sidecar.schema.json
│   ├── categories.schema.json
│   ├── tags.schema.json
│   └── SPECIFICATION.md
├── shelff/                     # Main library package
│   ├── sidecar.go              # Sidecar CRUD
│   ├── sidecar_test.go
│   ├── book.go                 # Book (PDF+sidecar) operations
│   ├── book_test.go
│   ├── library.go              # Library-level operations (scan, categories, tags, stats)
│   ├── library_test.go
│   ├── types.go                # Data types
│   ├── errors.go               # Error types
│   └── validate.go             # Schema validation
├── cmd/
│   └── shelff-mcp/             # MCP server binary
│       └── main.go
├── go.mod
├── go.sum
├── DESIGN.md                   # This document
└── README.md
```

### 2.3 Dependencies

- **Required**: `github.com/google/jsonschema-go` for JSON Schema validation
- **Required**: `github.com/modelcontextprotocol/go-sdk` (official Go MCP SDK) for the MCP server
- **Principle**: Minimize dependencies. Standard library `encoding/json`, `os`, `path/filepath`, `time` should cover most needs.

## 3. Data Types

All types live in the `shelff` package. JSON field names use the exact names from the schemas.

### 3.1 Sidecar Metadata

```go
// SidecarMetadata represents the top-level structure of a *.pdf.meta.json file.
type SidecarMetadata struct {
    SchemaVersion int               `json:"schemaVersion"`
    Metadata      DublinCore        `json:"metadata"`
    Reading       *ReadingProgress  `json:"reading,omitempty"`
    Display       *DisplaySettings  `json:"display,omitempty"`
    Category      *string           `json:"category,omitempty"`
    Tags          []string          `json:"tags,omitempty"`

    // unexported: holds the raw JSON bytes read from disk, used for round-trip preservation.
    rawJSON []byte
}

// DublinCore holds Dublin Core metadata fields.
// Field names use the "dc:" prefix to match the JSON representation.
type DublinCore struct {
    Title      string   `json:"dc:title"`
    Creator    []string `json:"dc:creator,omitempty"`
    Date       *string  `json:"dc:date,omitempty"`
    Publisher  *string  `json:"dc:publisher,omitempty"`
    Language   *string  `json:"dc:language,omitempty"`
    Subject    []string `json:"dc:subject,omitempty"`
    Identifier *string  `json:"dc:identifier,omitempty"`
}

// ReadingProgress tracks how far a user has read.
type ReadingProgress struct {
    LastReadPage int        `json:"lastReadPage"`          // 1-indexed
    LastReadAt   time.Time  `json:"lastReadAt"`            // ISO 8601 UTC
    TotalPages   int        `json:"totalPages"`
    Status       *string    `json:"status,omitempty"`      // "unread" | "reading" | "finished"
    FinishedAt   *time.Time `json:"finishedAt,omitempty"`  // ISO 8601 UTC
}

// DisplaySettings controls PDF rendering preferences.
type DisplaySettings struct {
    Direction  string  `json:"direction"`             // "LTR" | "RTL"
    PageLayout *string `json:"pageLayout,omitempty"`  // "single" | "spread" | "spread-with-cover"
}
```

### 3.2 Category List

```go
// CategoryList represents .shelff/categories.json.
type CategoryList struct {
    Version    int            `json:"version"`
    Categories []CategoryItem `json:"categories"`
}

type CategoryItem struct {
    Name  string `json:"name"`
    Order int    `json:"order"`
}
```

### 3.3 Tag Order

```go
// TagOrder represents .shelff/tags.json.
type TagOrder struct {
    Version  int      `json:"version"`
    TagOrder []string `json:"tagOrder"`
}
```

### 3.4 Query Result Types

```go
// BookEntry represents a PDF found during a directory scan.
type BookEntry struct {
    PDFPath     string  // Absolute path to the PDF file
    SidecarPath *string // Absolute path to sidecar JSON, nil if not present
    HasSidecar  bool
}

// OrphanedSidecar represents a sidecar JSON with no corresponding PDF.
type OrphanedSidecar struct {
    SidecarPath string // Absolute path to the orphaned sidecar
    ExpectedPDF string // The PDF path it would correspond to
}

// LibraryStats holds aggregate statistics about a shelff library.
type LibraryStats struct {
    TotalPDFs         int
    WithSidecar       int
    WithoutSidecar    int
    OrphanedSidecars  int
    CategoryCounts    map[string]int  // category name → count of PDFs
    TagCounts         map[string]int  // tag name → count of PDFs
    StatusCounts      map[string]int  // reading status → count of PDFs ("unread"/"reading"/"finished"/""(no reading data))
}
```

### 3.5 Constants

```go
const (
    SidecarSuffix     = ".meta.json"       // appended to PDF filename
    ConfigDir         = ".shelff"           // directory name under library root
    CategoriesFile    = "categories.json"   // in .shelff/
    TagsFile          = "tags.json"         // in .shelff/
    SchemaVersion     = 1

    StatusUnread   = "unread"
    StatusReading  = "reading"
    StatusFinished = "finished"

    DirectionLTR = "LTR"
    DirectionRTL = "RTL"

    LayoutSingle          = "single"
    LayoutSpread          = "spread"
    LayoutSpreadWithCover = "spread-with-cover"
)
```

## 4. API Design

### 4.1 Path Helpers (exported, stateless)

```go
// SidecarPath returns the sidecar JSON path for a given PDF path.
// Example: "/lib/book.pdf" → "/lib/book.pdf.meta.json"
func SidecarPath(pdfPath string) string

// PDFPathFromSidecar returns the PDF path for a given sidecar path.
// Returns ("", false) if the path doesn't match the sidecar naming convention.
// Example: "/lib/book.pdf.meta.json" → ("/lib/book.pdf", true)
func PDFPathFromSidecar(sidecarPath string) (string, bool)

// IsSidecarPath reports whether the given path looks like a shelff sidecar file.
func IsSidecarPath(path string) bool
```

### 4.2 Sidecar Operations (exported, stateless)

These functions operate on individual sidecar files. They do not require a Library instance.

```go
// ReadSidecar reads and parses the sidecar JSON for the given PDF.
// Returns (nil, nil) if the sidecar file does not exist (this is a normal state).
// Returns an error if the file exists but cannot be read or parsed.
func ReadSidecar(pdfPath string) (*SidecarMetadata, error)

// CreateSidecar creates a new sidecar JSON for the given PDF.
// Sets dc:title to the PDF filename without the .pdf extension.
// Returns ErrSidecarAlreadyExists if the sidecar already exists.
// Returns ErrPDFNotFound if the PDF file does not exist.
func CreateSidecar(pdfPath string) (*SidecarMetadata, error)

// WriteSidecar writes the sidecar JSON for the given PDF.
// Preserves unknown top-level fields from the original JSON (round-trip preservation).
// Creates the file if it does not exist; overwrites if it does.
// The file is written atomically (write to temp + rename).
// Output is pretty-printed with sorted keys.
func WriteSidecar(pdfPath string, meta *SidecarMetadata) error

// DeleteSidecar deletes the sidecar JSON for the given PDF.
// Returns nil if the sidecar does not exist (idempotent).
func DeleteSidecar(pdfPath string) error
```

### 4.3 Book Operations (PDF + sidecar as a unit)

```go
// MoveBook moves a PDF and its sidecar (if present) to a destination directory.
// The destination directory must exist.
// If the sidecar does not exist, only the PDF is moved.
// Returns the new PDF path.
// If a file with the same name exists at the destination, returns ErrAlreadyExists.
func MoveBook(pdfPath string, destDir string) (newPDFPath string, err error)

// RenameBook renames a PDF and its sidecar (if present) within the same directory.
// newName must be a single base filename. Leading/trailing whitespace is trimmed,
// and a trailing ".pdf" extension is accepted and stripped before renaming.
// Returns the new PDF path.
// If a file with the new name exists, returns ErrAlreadyExists.
func RenameBook(pdfPath string, newName string) (newPDFPath string, err error)

// DeleteBook deletes a PDF and its sidecar (if present).
// NOTE: This is provided as a library function but is NOT exposed via MCP.
func DeleteBook(pdfPath string) error
```

**Atomicity for MoveBook/RenameBook**: If the PDF is moved/renamed successfully but the sidecar operation fails, the PDF operation is rolled back (moved/renamed back) and an error is returned. This matches the shelff Swift implementation's behavior.

**Consistency for DeleteBook**: DeleteBook should avoid leaving the library in a partial state. If sidecar deletion fails after the PDF has been staged for deletion, the PDF should be restored to its original path and the operation should return an error.

### 4.4 Library Type

```go
// Library represents a shelff documents directory.
type Library struct {
    root string // absolute path to the library root directory
}

// OpenLibrary creates a Library for the given root directory.
// The directory must exist.
// Does not create the .shelff/ config directory — it is created on first write.
func OpenLibrary(rootDir string) (*Library, error)

// Root returns the library root directory path.
func (l *Library) Root() string
```

### 4.5 Category Operations (on Library)

```go
// ReadCategories reads .shelff/categories.json.
// Returns an empty CategoryList if the file does not exist.
func (l *Library) ReadCategories() (*CategoryList, error)

// WriteCategories writes .shelff/categories.json.
// Creates .shelff/ directory if it does not exist.
// Normalizes order values to match array indices before writing.
func (l *Library) WriteCategories(cats *CategoryList) error

// AddCategory adds a category to the list.
// Returns ErrCategoryAlreadyExists if the name already exists (after trimming).
// Returns ErrEmptyName if the name is empty after trimming.
func (l *Library) AddCategory(name string) error

// RemoveCategory removes a category from the list.
// If cascade is true, clears the category field in all sidecars that reference it.
// Returns ErrCategoryNotFound if the category does not exist.
func (l *Library) RemoveCategory(name string, cascade bool) error

// RenameCategory renames a category.
// If cascade is true, updates the category field in all sidecars that reference the old name.
// Returns ErrCategoryNotFound if the old name does not exist.
// Returns ErrCategoryAlreadyExists if the new name already exists.
// Returns ErrEmptyName if the new name is empty after trimming.
func (l *Library) RenameCategory(oldName string, newName string, cascade bool) error

// ReorderCategories sets the category order.
// names must contain exactly the same set of category names (no additions or removals).
// Returns ErrCategoryMismatch if the names don't match.
func (l *Library) ReorderCategories(names []string) error
```

### 4.6 Tag Operations (on Library)

```go
// ReadTagOrder reads .shelff/tags.json.
// Returns an empty TagOrder if the file does not exist.
func (l *Library) ReadTagOrder() (*TagOrder, error)

// WriteTagOrder writes .shelff/tags.json.
// Creates .shelff/ directory if it does not exist.
func (l *Library) WriteTagOrder(tags *TagOrder) error

// AddTagToOrder adds a tag to the display order list.
// This only affects display ordering — tags are primarily defined by their presence in sidecars.
// Returns ErrTagAlreadyExists if already in the order list.
func (l *Library) AddTagToOrder(name string) error

// RemoveTagFromOrder removes a tag from the display order list.
// If cascade is true, also removes the tag from all sidecars that reference it.
// Note: without cascade, this only removes the display ordering entry.
func (l *Library) RemoveTagFromOrder(name string, cascade bool) error

// RenameTag renames a tag in the display order list.
// If cascade is true, updates the tag in all sidecars that reference the old name.
// If a cascade rename would create duplicate occurrences of the new tag within a
// sidecar's tags array, the resulting tags array is de-duplicated while preserving order.
func (l *Library) RenameTag(oldName string, newName string, cascade bool) error

// ReorderTags sets the tag display order.
func (l *Library) ReorderTags(names []string) error
```

### 4.7 Query Operations (on Library)

```go
// ScanBooks scans the library for PDF files and their sidecar status.
// If recursive is true, scans subdirectories (default behavior).
// Excludes the .shelff/ config directory from results.
func (l *Library) ScanBooks(recursive bool) ([]BookEntry, error)

// FindOrphanedSidecars finds sidecar JSON files that have no corresponding PDF.
// Scans recursively.
func (l *Library) FindOrphanedSidecars() ([]OrphanedSidecar, error)

// Validate validates a sidecar JSON file against the schema.
// Returns a list of validation errors (empty if valid).
func (l *Library) Validate(pdfPath string) ([]string, error)

// Stats computes aggregate statistics about the library.
// Scans all PDFs and sidecars recursively.
func (l *Library) Stats() (*LibraryStats, error)

// CollectAllTags scans all sidecar files and returns the union of all tags.
// This is the canonical tag set — tags.json only defines display order.
func (l *Library) CollectAllTags() ([]string, error)
```

## 5. Behavior Specifications

### 5.1 Sidecar File Naming

- Sidecar path = PDF path + ".meta.json"
  - `book.pdf` → `book.pdf.meta.json`
  - `my report.pdf` → `my report.pdf.meta.json`
- Sidecar resides in the same directory as its PDF
- The `.pdf` extension is part of the base name for sidecar purposes

### 5.2 CreateSidecar — Initial Content

When creating a sidecar for a PDF that has none:

```json
{
  "schemaVersion": 1,
  "metadata": {
    "dc:title": "book"
  }
}
```

Where `"book"` is the PDF filename with the `.pdf` extension removed. For `"My Report.pdf"`, the title would be `"My Report"`.

### 5.3 Round-trip Preservation

This is a **critical requirement**. Implementations must not discard unknown **top-level** fields when reading and writing sidecar files.

**Strategy in Go**:

1. On read: `json.Unmarshal` into `SidecarMetadata` for typed access. Also store the raw `[]byte` in the unexported `rawJSON` field.
2. On write:
   a. Marshal the typed `SidecarMetadata` to `map[string]any` via `json.Marshal` → `json.Unmarshal`.
   b. If `rawJSON` is non-nil, unmarshal it to `map[string]any` (the original).
   c. Copy unknown top-level keys from the original into the new map.
   d. Do **not** preserve unknown keys within `metadata`; when `metadata` is rewritten, only the known Dublin Core keys represented by `DublinCore` are emitted.
   e. For optional top-level keys (`reading`, `display`, `category`, `tags`): if the typed struct has them as nil/zero, do NOT copy old values from the original. Explicit nil means "removed".
   f. Serialize the merged map with `json.MarshalIndent` (pretty-printed, sorted keys).

**Known top-level keys** (anything else is unknown and must be preserved):
`schemaVersion`, `metadata`, `reading`, `display`, `category`, `tags`

**Known metadata keys** (anything else within `metadata` is rewritten away):
`dc:title`, `dc:creator`, `dc:date`, `dc:publisher`, `dc:language`, `dc:subject`, `dc:identifier`

**Example**: A sidecar contains `"x-calibre-id": 42` at the top level and `"dcterms:modified": "2025-01-01"` inside `metadata`. After shelff-go reads, modifies `dc:title`, and writes back, `x-calibre-id` must still be present, but `dcterms:modified` is not preserved.

### 5.4 JSON Output Format

- Encoding: UTF-8
- Pretty-printed with 2-space indentation
- Keys sorted alphabetically (for deterministic diffs)
- Date-time fields: ISO 8601 in UTC (e.g., `"2026-03-20T10:30:00Z"`)
- `dc:date`: stored as-is (string, not parsed or normalized). Typical formats: `"2024"`, `"2024-01"`, `"2024-01-15"`, `"2024-01-15T00:00:00Z"`. shelff-go must not alter the precision.
- Atomic writes: write to a temporary file in the same directory, then `os.Rename`

### 5.5 Category and Tag Naming

- Names are trimmed of leading/trailing whitespace before use
- Empty names (after trimming) are rejected
- Category names are unique within the category list
- Comparison is exact (case-sensitive)

### 5.6 Category Normalization

When writing categories.json, the `order` field of each CategoryItem must be normalized to match its array index (0-based). This ensures consistency regardless of how categories were manipulated in memory.

### 5.7 Tag Semantics

- The canonical set of tags comes from scanning all sidecar files (union of all `tags` arrays)
- `tags.json` only defines display order — it is not the master tag list
- Tags in `tags.json` that appear in no sidecar are silently excluded from `CollectAllTags` results (but remain in tags.json until explicitly removed)
- Tags in sidecars but absent from `tags.json` are appended alphabetically when computing display order

### 5.8 Cascade Operations

When `cascade` is true for category/tag rename/remove:

1. Scan all sidecar files recursively
2. For each sidecar that references the affected name:
   a. Read the sidecar (with round-trip preservation)
   b. Update the relevant field (category or tags)
   c. Write the sidecar back
3. Report the count of modified sidecars

For rename cascade:
- Category: replace `category` field value if it equals the old name
- Tag: replace the old name with the new name in the `tags` array; if that creates duplicate
  occurrences of the new tag, de-duplicate the resulting array while preserving order

For remove cascade:
- Category: set `category` to nil if it equals the removed name
- Tag: remove the tag from the `tags` array

### 5.9 MoveBook / RenameBook Atomicity

1. Move/rename the PDF file
2. Attempt to move/rename the sidecar (if it exists)
3. If step 2 fails: roll back step 1 (move/rename the PDF back)
4. If rollback also fails: return a `RollbackError` wrapping both errors

### 5.10 Directory Scanning

- Traverse the directory tree starting from the library root
- Skip the `.shelff/` directory (config, not content)
- A file is considered a PDF if its extension is `.pdf` (case-insensitive)
- A sidecar is identified by the `.pdf.meta.json` suffix
- Do not follow symbolic links (avoid cycles)

### 5.11 PDFs Without Sidecars

A PDF without a sidecar is a **normal, valid state**. It means the PDF has no shelff metadata. `ScanBooks` returns such PDFs with `HasSidecar: false`. The caller (e.g., MCP server) can create a sidecar on demand using `CreateSidecar`.

### 5.12 Schema Validation

Use the JSON Schema files from the `shelff-schema` submodule. Validation returns a list of human-readable error strings. An empty list means the document is valid.

Validate checks:
- Required fields present
- Field types correct
- Enum values valid
- `schemaVersion` / `version` equals 1

Validation does NOT reject unknown fields (schemas use `additionalProperties: true` at the top level and in metadata), even though unknown keys within `metadata` are not preserved by shelff-go when rewriting.

## 6. Error Types

```go
var (
    ErrPDFNotFound          = errors.New("pdf file not found")
    ErrSidecarAlreadyExists = errors.New("sidecar file already exists")
    ErrSidecarNotFound      = errors.New("sidecar file not found")
    ErrAlreadyExists        = errors.New("destination file already exists")
    ErrCategoryNotFound     = errors.New("category not found")
    ErrCategoryAlreadyExists = errors.New("category already exists")
    ErrTagAlreadyExists     = errors.New("tag already exists in order list")
    ErrEmptyName            = errors.New("name is empty after trimming")
    ErrCategoryMismatch     = errors.New("category names do not match existing set")
    ErrInvalidSchemaVersion = errors.New("unsupported schema version")
    ErrLibraryNotFound      = errors.New("library directory does not exist")
)

// RollbackError is returned when a multi-step operation fails
// and the rollback also fails, leaving the filesystem in a potentially
// inconsistent state.
type RollbackError struct {
    OriginalError error
    RollbackError error
}
```

## 7. MCP Server Specification

The MCP server (`cmd/shelff-mcp/`) exposes shelff-go library functions as MCP tools. It uses stdio transport.

### 7.1 Server Initialization

The server accepts the library root path as a command-line argument or environment variable:

```
shelff-mcp --root /path/to/shelff/Documents
# or
SHELFF_ROOT=/path/to/shelff/Documents shelff-mcp
```

### 7.2 Exposed Tools

| Tool Name | Library Function | Notes |
|---|---|---|
| `read_sidecar` | `ReadSidecar` | |
| `create_sidecar` | `CreateSidecar` | |
| `write_sidecar` | `WriteSidecar` | Accepts partial updates (merge with existing) |
| `delete_sidecar` | `DeleteSidecar` | |
| `move_book` | `MoveBook` | |
| `rename_book` | `RenameBook` | |
| `scan_books` | `Library.ScanBooks` | |
| `find_orphaned_sidecars` | `Library.FindOrphanedSidecars` | |
| `validate_sidecar` | `Library.Validate` | |
| `library_stats` | `Library.Stats` | |
| `collect_all_tags` | `Library.CollectAllTags` | |
| `read_categories` | `Library.ReadCategories` | |
| `add_category` | `Library.AddCategory` | |
| `remove_category` | `Library.RemoveCategory` | cascade parameter exposed |
| `rename_category` | `Library.RenameCategory` | cascade parameter exposed |
| `reorder_categories` | `Library.ReorderCategories` | |
| `read_tag_order` | `Library.ReadTagOrder` | |
| `add_tag_to_order` | `Library.AddTagToOrder` | |
| `remove_tag_from_order` | `Library.RemoveTagFromOrder` | cascade parameter exposed |
| `rename_tag` | `Library.RenameTag` | cascade parameter exposed |
| `reorder_tags` | `Library.ReorderTags` | |

### 7.3 NOT Exposed via MCP

| Library Function | Reason |
|---|---|
| `DeleteBook` | Accident prevention — deleting PDFs is destructive and irreversible |

### 7.4 Tool: `write_sidecar` — Partial Update Semantics

The MCP `write_sidecar` tool accepts a **partial JSON object**. Only the fields present in the input are updated; absent fields are left unchanged. This is an MCP-layer convenience — the library's `WriteSidecar` always writes the full struct.

Implementation in the MCP layer:
1. `ReadSidecar` (or `CreateSidecar` if no sidecar exists)
2. Merge the input fields into the existing metadata
3. `WriteSidecar` the merged result

### 7.5 Tool Input/Output Format

- Inputs and outputs use JSON
- PDF paths in tool inputs/outputs are relative to the library root (the MCP server resolves to absolute paths internally)
- The MCP server validates that all paths resolve within the library root (path traversal prevention)

### 7.6 Path Security

The MCP server must reject any path that resolves outside the library root after canonicalization. This prevents `../` traversal attacks.

```go
func (s *Server) resolvePath(relative string) (string, error) {
    abs := filepath.Join(s.library.Root(), relative)
    resolved, err := filepath.EvalSymlinks(abs) // or filepath.Abs
    if err != nil {
        return "", err
    }
    if !strings.HasPrefix(resolved, s.library.Root()) {
        return "", ErrPathTraversal
    }
    return resolved, nil
}
```

## 8. Testing Strategy

### 8.1 Test Structure

Each test creates a temporary directory (`t.TempDir()`) as a library root. Tests are fully self-contained and require no external resources.

### 8.2 Key Test Scenarios

**Sidecar CRUD**:
- Create sidecar → verify dc:title matches filename, schemaVersion = 1
- Read nonexistent sidecar → returns (nil, nil)
- Read existing sidecar → all fields parsed correctly
- Write then read → round-trip identity
- Delete existing sidecar → file gone
- Delete nonexistent sidecar → no error (idempotent)
- Create when already exists → ErrSidecarAlreadyExists
- Create when PDF doesn't exist → ErrPDFNotFound

**Round-trip preservation**:
- Write a sidecar with unknown top-level field `"x-custom": 42` → read, modify dc:title, write back → `"x-custom"` still present
- Unknown field inside `metadata` (e.g., `"dcterms:modified": "..."`) is dropped on rewrite
- Optional fields set to nil are not resurrected from original JSON

**dc:date precision**:
- Read `"2024"` → write back → still `"2024"` (not `"2024-01-01"`)
- Read `"2024-06"` → write back → still `"2024-06"`

**Book operations**:
- MoveBook with sidecar → both files moved
- MoveBook without sidecar → only PDF moved, no error
- MoveBook to directory with existing file → ErrAlreadyExists
- RenameBook → both PDF and sidecar renamed
- RenameBook with blank or non-base target name → error, original files unchanged
- RenameBook with trailing `.pdf` in newName → renamed once, not `.pdf.pdf`
- RenameBook sidecar failure → PDF rolled back to original name
- DeleteBook → both files deleted
- DeleteBook without sidecar → only PDF deleted
- DeleteBook sidecar failure → PDF restored to original path

**Category operations**:
- Add → appears in list with correct order
- Add duplicate → ErrCategoryAlreadyExists
- Remove with cascade → category cleared from all sidecars
- Remove without cascade → sidecars untouched
- Rename with cascade → category updated in all sidecars
- Reorder → order values normalized

**Tag operations**:
- CollectAllTags → union of all sidecar tags
- Rename with cascade → tag updated in all sidecars
- Rename with cascade causing tag collision → resulting sidecar tags de-duplicated
- Remove with cascade → tag removed from all sidecars
- tags.json ordering applied correctly, unregistered tags appended alphabetically

**Query operations**:
- ScanBooks → finds PDFs recursively, skips .shelff/
- FindOrphanedSidecars → finds sidecars with no corresponding PDF
- Stats → correct counts

**Edge cases**:
- PDF filename with spaces and special characters
- Empty library (no PDFs, no .shelff/)
- Library with .shelff/ but no categories.json or tags.json
- Sidecar with only required fields (schemaVersion + metadata.dc:title)
- Very large library scan (performance)
- Concurrent access is out of scope (single-threaded library)

## 9. Notes

### 9.1 File Format Discrepancy

The `tags.schema.json` description mentions `tag-order.json` as the filename, but both SPECIFICATION.md and the shelff Swift implementation use `tags.json`. This library uses **`tags.json`** as the authoritative filename.

### 9.2 No iCloud / NSFileCoordinator

shelff-go operates on a plain filesystem. It does not use iCloud APIs or NSFileCoordinator. When used to operate on an iCloud Drive directory from macOS, file coordination is the caller's responsibility. In practice, for MCP use cases (single-process, sequential access), this is not an issue.

### 9.3 categories.json and tags.json Round-trip Preservation

Like sidecars, `categories.json` and `tags.json` use `additionalProperties: true`. The same round-trip preservation strategy should be applied to these files — unknown top-level fields must survive read/write cycles.

### 9.4 Relationship to shelff App

shelff-go and the shelff iPad app operate on the same file format but are independent implementations. They do not communicate directly. If both modify the same files (e.g., shelff-go via MCP on a Mac, shelff app on iPad), iCloud handles sync and conflict resolution at the file level. Sidecar-level conflict resolution (e.g., "newer lastReadAt wins") is handled by the shelff app via `SidecarDocument`; shelff-go does not implement conflict resolution.
