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

// ReadingProgress tracks how far a user has read.
type ReadingProgress struct {
	LastReadPage int        `json:"lastReadPage"`
	LastReadAt   time.Time  `json:"lastReadAt"`
	TotalPages   int        `json:"totalPages"`
	Status       *string    `json:"status,omitempty"`
	FinishedAt   *time.Time `json:"finishedAt,omitempty"`
}

// DisplaySettings controls PDF rendering preferences.
type DisplaySettings struct {
	Direction  string  `json:"direction"`
	PageLayout *string `json:"pageLayout,omitempty"`
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

const (
	SidecarSuffix  = ".meta.json"
	ConfigDir      = ".shelff"
	CategoriesFile = "categories.json"
	TagsFile       = "tags.json"
	SchemaVersion  = 1

	StatusUnread   = "unread"
	StatusReading  = "reading"
	StatusFinished = "finished"

	DirectionLTR = "LTR"
	DirectionRTL = "RTL"

	LayoutSingle          = "single"
	LayoutSpread          = "spread"
	LayoutSpreadWithCover = "spread-with-cover"
)
