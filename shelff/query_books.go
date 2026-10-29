package shelff

import (
	"fmt"
	"os"
	"slices"
)

var readSidecarFile = os.ReadFile

// QueryBooksInDirectory returns summaries of books under dirPath whose sidecar
// matches filter. Books without a sidecar are excluded. Sidecars that cannot
// be parsed, or that disappear between the scan and the read, are skipped and
// reported in Warnings. An empty category or collection title in a sidecar is
// treated as unset.
// If recursive is true, searches subdirectories below dirPath.
func (l *Library) QueryBooksInDirectory(dirPath string, recursive bool, filter BookFilter) (*QueryBooksResult, error) {
	if err := filter.validate(); err != nil {
		return nil, err
	}

	books, err := l.ScanBooksInDirectory(dirPath, recursive)
	if err != nil {
		return nil, err
	}

	result := &QueryBooksResult{
		Books:    make([]BookSummary, 0),
		Warnings: make([]string, 0),
	}
	for _, book := range books {
		if !book.HasSidecar || book.SidecarPath == nil {
			continue
		}
		sidecarPath := *book.SidecarPath

		data, err := readSidecarFile(sidecarPath)
		if err != nil {
			if os.IsNotExist(err) {
				result.Warnings = append(result.Warnings, fmt.Sprintf("skipped sidecar removed during query: %s", l.relativeLibraryPath(sidecarPath)))
				continue
			}
			return nil, err
		}
		meta, err := ParseSidecarJSON(data)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("skipped unparsable sidecar %s: %v", l.relativeLibraryPath(sidecarPath), err))
			continue
		}

		summary := summarizeBook(book.PDFPath, sidecarPath, meta)
		if filter.matches(summary) {
			result.Books = append(result.Books, summary)
		}
	}
	return result, nil
}

func (f BookFilter) validate() error {
	switch f.TagMatch {
	case "", TagMatchAny, TagMatchAll:
	default:
		return fmt.Errorf("%w: %q", ErrInvalidTagMatch, f.TagMatch)
	}
	if f.Status != nil && *f.Status != "" && !f.Status.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidStatusFilter, *f.Status)
	}
	return nil
}

func (f BookFilter) matches(book BookSummary) bool {
	if f.Category != nil && !matchesOptionalString(*f.Category, book.Category) {
		return false
	}

	if f.Collection != nil {
		var title *string
		if book.Collection != nil {
			title = &book.Collection.Title
		}
		if !matchesOptionalString(*f.Collection, title) {
			return false
		}
	}

	if f.Tags != nil && !f.matchesTags(book.Tags) {
		return false
	}

	if f.Status != nil {
		if *f.Status == "" {
			if book.Status != nil {
				return false
			}
		} else if book.Status == nil || *book.Status != *f.Status {
			return false
		}
	}

	if f.Archived != nil {
		archived := book.Archived != nil && *book.Archived
		if archived != *f.Archived {
			return false
		}
	}

	return true
}

// matchesOptionalString treats an empty want as "unset" and matches a nil value.
func matchesOptionalString(want string, value *string) bool {
	if want == "" {
		return value == nil
	}
	return value != nil && *value == want
}

func (f BookFilter) matchesTags(bookTags []string) bool {
	wanted := *f.Tags
	if len(wanted) == 0 {
		return len(bookTags) == 0
	}
	if f.TagMatch == TagMatchAll {
		for _, tag := range wanted {
			if !slices.Contains(bookTags, tag) {
				return false
			}
		}
		return true
	}
	for _, tag := range wanted {
		if slices.Contains(bookTags, tag) {
			return true
		}
	}
	return false
}

func summarizeBook(pdfPath string, sidecarPath string, meta *SidecarMetadata) BookSummary {
	summary := BookSummary{
		PDFPath:     pdfPath,
		SidecarPath: sidecarPath,
		Title:       meta.Metadata.Title,
		Creators:    meta.Metadata.Creator,
		Tags:        meta.Tags,
	}
	if meta.Category != nil && *meta.Category != "" {
		summary.Category = meta.Category
	}
	if meta.Collection != nil && meta.Collection.Title != "" {
		summary.Collection = meta.Collection
	}
	if meta.Reading != nil {
		summary.Status = meta.Reading.Status
	}
	if meta.Archive != nil {
		archived := meta.Archive.IsArchived
		summary.Archived = &archived
	}
	return summary
}
