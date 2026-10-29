package shelff

import (
	"fmt"
	"os"
	"slices"
)

// QueryBooksInDirectory returns summaries of books under dirPath whose sidecar
// matches filter. Books without a sidecar are excluded. Sidecars that cannot
// be parsed are skipped and reported in Warnings.
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

		data, err := os.ReadFile(sidecarPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		meta, err := ParseSidecarJSON(data)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("skipped unparsable sidecar %s: %v", l.relativeLibraryPath(sidecarPath), err))
			continue
		}

		if !filter.matches(meta) {
			continue
		}
		result.Books = append(result.Books, summarizeBook(book.PDFPath, sidecarPath, meta))
	}
	return result, nil
}

func (f BookFilter) validate() error {
	switch f.TagMatch {
	case "", TagMatchAny, TagMatchAll:
	default:
		return fmt.Errorf("%w: %q", ErrInvalidTagMatch, f.TagMatch)
	}
	if f.Status != nil && *f.Status != "" && !ReadingStatus(*f.Status).Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidStatusFilter, *f.Status)
	}
	return nil
}

func (f BookFilter) matches(meta *SidecarMetadata) bool {
	if f.Category != nil {
		if *f.Category == "" {
			if meta.Category != nil {
				return false
			}
		} else if meta.Category == nil || *meta.Category != *f.Category {
			return false
		}
	}

	if f.Collection != nil {
		if *f.Collection == "" {
			if meta.Collection != nil {
				return false
			}
		} else if meta.Collection == nil || meta.Collection.Title != *f.Collection {
			return false
		}
	}

	if f.Tags != nil && !f.matchesTags(meta.Tags) {
		return false
	}

	if f.Status != nil {
		var status *ReadingStatus
		if meta.Reading != nil {
			status = meta.Reading.Status
		}
		if *f.Status == "" {
			if status != nil {
				return false
			}
		} else if status == nil || string(*status) != *f.Status {
			return false
		}
	}

	if f.Archived != nil {
		archived := meta.Archive != nil && meta.Archive.IsArchived
		if archived != *f.Archived {
			return false
		}
	}

	return true
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
		Category:    meta.Category,
		Tags:        meta.Tags,
		Collection:  meta.Collection,
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
