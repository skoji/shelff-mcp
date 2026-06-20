# shelff Metadata Specification v1

## Overview

shelff stores all metadata as plain JSON files alongside PDFs in the filesystem. This design ensures:

- **Portability**: Metadata travels with the files. Copy a PDF and its sidecar to another device or app, and the metadata is preserved.
- **No lock-in**: Any tool that can read JSON can read shelff metadata.
- **Transparency**: Users can inspect and edit metadata with any text editor.

## Filesystem Layout

```
Documents/                          # shelff documents root (iCloud Documents container)
├── .shelff/                        # shelff configuration directory
│   ├── categories.json             # Category list and display order
│   └── tags.json              # Tag display order
├── book.pdf                        # A PDF file
├── book.pdf.meta.json              # Sidecar metadata for book.pdf
├── subfolder/                      # User-created folder
│   ├── another.pdf
│   └── another.pdf.meta.json
└── ...
```

## File Specifications

### Sidecar Metadata (`*.pdf.meta.json`)

Each PDF may have an accompanying sidecar file that stores its metadata. The sidecar filename is formed by appending `.meta.json` to the full PDF filename (e.g., `book.pdf` → `book.pdf.meta.json`). The sidecar resides in the same directory as the PDF.

If no sidecar file exists, the PDF has no shelff metadata (title falls back to the filename).

**Schema**: [sidecar.schema.json](./sidecar.schema.json)

#### Bookmarks

The `bookmarks` field stores user-defined page bookmarks (digital "sticky notes"). Each entry has a `page` (1-indexed integer) and an optional `label` string. Bookmarks are distinct from the PDF's built-in document outline; they are added by the reader to mark pages of interest.

```json
"bookmarks": [
  { "page": 3, "label": "Important diagram" },
  { "page": 15 },
  { "page": 42 }
]
```

When `label` is absent, the reader UI may render a fallback such as the page number.

#### Collection

The `collection` field identifies the series or magazine this book belongs to. A book can belong to at most one collection. The field is optional; when absent, the book is treated as not belonging to any collection.

Magazine (ordered by date, no position):

```json
"collection": {
  "title": "Monthly Swift"
}
```

Series (ordered by volume number):

```json
"collection": {
  "title": "Introduction to Swift",
  "position": 1
}
```

Half-volume or special edition:

```json
"collection": {
  "title": "Introduction to Swift",
  "position": 0.5
}
```

Fields:
- `title` (string, required): The name of the collection or series.
- `position` (number, optional): The position within the collection. Fractional values are allowed (e.g. `0.5` for a special half-volume). Omit for collections ordered by other means (e.g. magazines ordered by publication date).

This design is inspired by EPUB's `belongs-to-collection` / `group-position` metadata, simplified to a single collection without a type distinction (`series` vs `set`).

### Category List (`.shelff/categories.json`)

An ordered list of categories. Each PDF can belong to at most one category, specified by the `category` field in its sidecar. Categories must be defined in this file to appear in the UI, but a sidecar may reference a category name not yet listed here (it will be treated as uncategorized until the category is created).

**Schema**: [categories.schema.json](./categories.schema.json)

### Tag Order (`.shelff/tags.json`)

Defines the display order of tags. Unlike categories, **there is no master tag list** — the canonical set of tags is derived by scanning all sidecar files. This file only controls the order in which known tags are displayed. Tags found in sidecars but not in `tagOrder` are appended in alphabetical order.

**Schema**: [tags.schema.json](./tags.schema.json)

## Extension Conventions

All three schemas allow additional properties at the top level (`additionalProperties: true`). This permits third-party tools and user extensions to store custom data without conflicting with shelff's own fields.

### Rules for Extension Fields

- **Use the `x-` prefix** for third-party or user-defined fields (e.g., `x-calibre-id`, `x-my-custom-field`).
- **Top-level fields without `x-` prefix** may be introduced in future versions of this specification. Third-party tools should avoid unprefixed top-level fields to prevent conflicts.
- **The `metadata` (Dublin Core) object** also allows additional properties. Standard Dublin Core extensions (e.g., `dcterms:` namespace) are welcome. Custom fields within `metadata` should also use the `x-` prefix.
- **`reading` and `display` objects** do not allow additional properties in this version.

### Round-trip Preservation

Implementations that read and write sidecar files **must preserve unknown fields**. Specifically:

- When reading a sidecar, unknown fields should be retained in memory.
- When writing a sidecar, unknown fields from the original file must be written back.
- This applies to top-level fields and fields within `metadata`.
- Implementations must not discard data they do not understand.

## Encoding

- **Character encoding**: UTF-8
- **Date-time fields**: ISO 8601 format in UTC (e.g., `2026-03-20T10:30:00Z`)
- **Date fields** (e.g., `dc:date`): ISO 8601 date format (e.g., `2024-01-15`), though not strictly validated
- **JSON formatting**: Pretty-printed with sorted keys is recommended for readability and deterministic diffs, but not required
