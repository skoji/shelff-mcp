package shelff

import "time"

// SidecarMetadata represents the top-level structure of a *.pdf.meta.json file.
type SidecarMetadata struct {
	SchemaVersion int              `json:"schemaVersion"`
	Metadata      DublinCore       `json:"metadata"`
	Reading       *ReadingProgress `json:"reading,omitempty"`
	Display       *DisplaySettings `json:"display,omitempty"`
	Category      *string          `json:"category,omitempty"`
	Tags          []string         `json:"tags,omitempty"`

	rawJSON []byte
}

// DublinCore holds Dublin Core metadata fields.
type DublinCore struct {
	Title      string   `json:"dc:title"`
	Creator    []string `json:"dc:creator,omitempty"`
	Date       *string  `json:"dc:date,omitempty"`
	Publisher  *string  `json:"dc:publisher,omitempty"`
	Language   *string  `json:"dc:language,omitempty"`
	Subject    []string `json:"dc:subject,omitempty"`
	Identifier *string  `json:"dc:identifier,omitempty"`
}

// Direction represents the reading direction of a PDF.
type Direction string

// Valid reports whether d is a recognised direction value.
func (d Direction) Valid() bool {
	switch d {
	case DirectionLTR, DirectionRTL:
		return true
	}
	return false
}

// PageLayout represents the page layout mode for a PDF.
type PageLayout string

// Valid reports whether p is a recognised page layout value.
func (p PageLayout) Valid() bool {
	switch p {
	case LayoutSingle, LayoutSpread, LayoutSpreadWithCover:
		return true
	}
	return false
}

// ReadingStatus represents the reading status of a book.
type ReadingStatus string

// Valid reports whether s is a recognised reading status value.
func (s ReadingStatus) Valid() bool {
	switch s {
	case StatusUnread, StatusReading, StatusFinished:
		return true
	}
	return false
}

// ReadingProgress tracks how far a user has read.
type ReadingProgress struct {
	LastReadPage int            `json:"lastReadPage"`
	LastReadAt   time.Time      `json:"lastReadAt"`
	TotalPages   int            `json:"totalPages"`
	Status       *ReadingStatus `json:"status,omitempty"`
	FinishedAt   *time.Time     `json:"finishedAt,omitempty"`
}

// DisplaySettings controls PDF rendering preferences.
type DisplaySettings struct {
	Direction  Direction   `json:"direction"`
	PageLayout *PageLayout `json:"pageLayout,omitempty"`
}

// CategoryList represents .shelff/categories.json.
type CategoryList struct {
	Version    int            `json:"version"`
	Categories []CategoryItem `json:"categories"`

	rawJSON []byte
}

// CategoryItem is a single category entry in .shelff/categories.json.
type CategoryItem struct {
	Name  string `json:"name"`
	Order int    `json:"order"`
}

// TagOrder represents .shelff/tags.json.
type TagOrder struct {
	Version  int      `json:"version"`
	TagOrder []string `json:"tagOrder"`

	rawJSON []byte
}

// BookEntry represents a PDF found during a directory scan.
type BookEntry struct {
	PDFPath     string
	SidecarPath *string
	HasSidecar  bool
}

// OrphanedSidecar represents a sidecar JSON with no corresponding PDF.
type OrphanedSidecar struct {
	SidecarPath string
	ExpectedPDF string
}

// LibraryStats holds aggregate statistics about a shelff library.
type LibraryStats struct {
	TotalPDFs        int
	WithSidecar      int
	WithoutSidecar   int
	OrphanedSidecars int
	CategoryCounts   map[string]int
	TagCounts        map[string]int
	StatusCounts     map[string]int
}

// CheckLibraryResult holds the result of a library diagnostic check.
type CheckLibraryResult struct {
	DotShelff        DotShelffStatus `json:"dotShelff"`
	Integrity        IntegrityReport `json:"integrity"`
	OrphanedSidecars []string        `json:"orphanedSidecars"`
	Summary          LibrarySummary  `json:"summary"`
}

// DotShelffStatus reports existence of the .shelff directory and config files.
type DotShelffStatus struct {
	Exists         bool `json:"exists"`
	CategoriesJSON bool `json:"categoriesJson"`
	TagsJSON       bool `json:"tagsJson"`
}

// IntegrityReport reports category/tag integrity issues.
type IntegrityReport struct {
	UndefinedCategories []string `json:"undefinedCategories"`
	UndefinedTags       []string `json:"undefinedTags"`
	UnusedCategories    []string `json:"unusedCategories"`
	UnusedTags          []string `json:"unusedTags"`
}

// LibrarySummary holds basic book counts.
type LibrarySummary struct {
	TotalPDFs      int `json:"totalPDFs"`
	WithSidecar    int `json:"withSidecar"`
	WithoutSidecar int `json:"withoutSidecar"`
}

const (
	SidecarSuffix  = ".meta.json"
	ConfigDir      = ".shelff"
	CategoriesFile = "categories.json"
	TagsFile       = "tags.json"
	SchemaVersion  = 1

	StatusUnread   ReadingStatus = "unread"
	StatusReading  ReadingStatus = "reading"
	StatusFinished ReadingStatus = "finished"

	DirectionLTR Direction = "LTR"
	DirectionRTL Direction = "RTL"

	LayoutSingle          PageLayout = "single"
	LayoutSpread          PageLayout = "spread"
	LayoutSpreadWithCover PageLayout = "spread-with-cover"
)
