package shelff

import (
	"fmt"
	"time"
)

var timeNow = time.Now

type archivePatch struct {
	isArchived   bool
	changedAt    time.Time
	hasChangedAt bool
}

func parseArchivePatch(raw any) (archivePatch, error) {
	object, ok := raw.(map[string]any)
	if !ok {
		return archivePatch{}, fmt.Errorf("%w: archive must be an object", ErrInvalidFieldValue)
	}
	for key := range object {
		if key != "isArchived" && key != "changedAt" {
			return archivePatch{}, fmt.Errorf("%w: archive has unknown property %q", ErrInvalidFieldValue, key)
		}
	}

	var patch archivePatch
	rawIsArchived, present := object["isArchived"]
	if !present {
		return archivePatch{}, fmt.Errorf("%w: archive.isArchived is required", ErrInvalidFieldValue)
	}
	patch.isArchived, ok = rawIsArchived.(bool)
	if !ok {
		return archivePatch{}, fmt.Errorf("%w: archive.isArchived must be a boolean", ErrInvalidFieldValue)
	}

	rawChangedAt, present := object["changedAt"]
	if !present {
		return patch, nil
	}
	value, ok := rawChangedAt.(string)
	if !ok {
		return archivePatch{}, fmt.Errorf("%w: archive.changedAt must be a date-time string", ErrInvalidFieldValue)
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return archivePatch{}, fmt.Errorf("%w: archive.changedAt %q is not an RFC 3339 date-time", ErrInvalidFieldValue, value)
	}
	patch.changedAt = parsed
	patch.hasChangedAt = true

	return patch, nil
}

// resolve returns the archive object to store, or nil when the existing state
// should be kept as is.
func (p archivePatch) resolve(existing *ArchiveState) map[string]any {
	var changedAt time.Time
	if p.hasChangedAt {
		if existing != nil && p.changedAt.Before(existing.ChangedAt) {
			return nil
		}
		changedAt = p.changedAt
	} else {
		if existing != nil && existing.IsArchived == p.isArchived && !existing.ChangedAt.IsZero() {
			return nil
		}
		// Second precision keeps the timestamp parseable by ISO 8601 readers
		// that reject fractional seconds.
		changedAt = timeNow().Truncate(time.Second)
	}

	return map[string]any{
		"isArchived": p.isArchived,
		"changedAt":  changedAt.UTC().Format(time.RFC3339Nano),
	}
}
