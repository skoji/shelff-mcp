package shelff

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var (
	knownCategoryTopLevelKeys = map[string]struct{}{
		"version":    {},
		"categories": {},
	}
	knownTagOrderTopLevelKeys = map[string]struct{}{
		"version":  {},
		"tagOrder": {},
	}
)

// ReadCategories reads .shelff/categories.json.
// Returns an empty CategoryList if the file does not exist.
func (l *Library) ReadCategories() (*CategoryList, error) {
	data, err := os.ReadFile(l.categoriesPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &CategoryList{
				Version:    SchemaVersion,
				Categories: []CategoryItem{},
			}, nil
		}
		return nil, err
	}

	var cats CategoryList
	if err := json.Unmarshal(data, &cats); err != nil {
		return nil, err
	}

	cats.rawJSON = append([]byte(nil), data...)
	if cats.Categories == nil {
		cats.Categories = []CategoryItem{}
	}
	if cats.Version == 0 {
		cats.Version = SchemaVersion
	}
	return &cats, nil
}

// WriteCategories writes .shelff/categories.json.
// Creates .shelff/ directory if it does not exist.
// Normalizes order values to match array indices before writing.
// It preserves unknown top-level fields present in cats.rawJSON (typically when
// cats originated from ReadCategories), but does not otherwise read or merge
// from any on-disk categories.json.
func (l *Library) WriteCategories(cats *CategoryList) error {
	normalized, err := normalizeCategoryList(cats)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(l.configDirPath(), 0o755); err != nil {
		return err
	}

	data, err := writeMergedJSONFile(l.categoriesPath(), normalized, cats.rawJSON, knownCategoryTopLevelKeys)
	if err != nil {
		return err
	}
	cats.rawJSON = append([]byte(nil), data...)
	cats.Version = normalized.Version
	cats.Categories = append([]CategoryItem(nil), normalized.Categories...)
	return nil
}

// AddCategory adds a category to the list.
// Returns ErrCategoryAlreadyExists if the name already exists (after trimming).
// Returns ErrEmptyName if the name is empty after trimming.
func (l *Library) AddCategory(name string) error {
	normalizedName, err := normalizeListName(name)
	if err != nil {
		return err
	}

	cats, err := l.ReadCategories()
	if err != nil {
		return err
	}
	for _, cat := range cats.Categories {
		if cat.Name == normalizedName {
			return ErrCategoryAlreadyExists
		}
	}

	cats.Categories = append(cats.Categories, CategoryItem{Name: normalizedName, Order: len(cats.Categories)})
	return l.WriteCategories(cats)
}

// RemoveCategory removes a category from the list.
// If cascade is true, clears the category field in all sidecars that reference it.
// Cascade updates are applied sequentially; if a sidecar update fails, already-written
// sidecars are not rolled back.
// Returns ErrCategoryNotFound if the category does not exist.
func (l *Library) RemoveCategory(name string, cascade bool) error {
	normalizedName, err := normalizeListName(name)
	if err != nil {
		return err
	}

	cats, err := l.ReadCategories()
	if err != nil {
		return err
	}

	index := slices.IndexFunc(cats.Categories, func(cat CategoryItem) bool {
		return cat.Name == normalizedName
	})
	if index == -1 {
		return ErrCategoryNotFound
	}

	cats.Categories = append(cats.Categories[:index], cats.Categories[index+1:]...)
	if err := l.WriteCategories(cats); err != nil {
		return err
	}
	if !cascade {
		return nil
	}

	return l.updateSidecars(func(meta *SidecarMetadata) bool {
		if meta.Category == nil || *meta.Category != normalizedName {
			return false
		}
		meta.Category = nil
		return true
	})
}

// RenameCategory renames a category.
// If cascade is true, updates the category field in all sidecars that reference the old name.
// Cascade updates are applied sequentially; if a sidecar update fails, already-written
// sidecars are not rolled back.
// Returns ErrCategoryNotFound if the old name does not exist.
// Returns ErrCategoryAlreadyExists if the new name already exists.
// Returns ErrEmptyName if the new name is empty after trimming.
func (l *Library) RenameCategory(oldName string, newName string, cascade bool) error {
	normalizedOld, err := normalizeListName(oldName)
	if err != nil {
		return err
	}
	normalizedNew, err := normalizeListName(newName)
	if err != nil {
		return err
	}

	cats, err := l.ReadCategories()
	if err != nil {
		return err
	}

	index := slices.IndexFunc(cats.Categories, func(cat CategoryItem) bool {
		return cat.Name == normalizedOld
	})
	if index == -1 {
		return ErrCategoryNotFound
	}

	if normalizedOld != normalizedNew {
		for _, cat := range cats.Categories {
			if cat.Name == normalizedNew {
				return ErrCategoryAlreadyExists
			}
		}
	}

	cats.Categories[index].Name = normalizedNew
	if err := l.WriteCategories(cats); err != nil {
		return err
	}
	if !cascade || normalizedOld == normalizedNew {
		return nil
	}

	return l.updateSidecars(func(meta *SidecarMetadata) bool {
		if meta.Category == nil || *meta.Category != normalizedOld {
			return false
		}
		meta.Category = stringPtr(normalizedNew)
		return true
	})
}

// ReorderCategories sets the category order.
// names must contain exactly the same set of category names (no additions or removals).
// Returns ErrCategoryMismatch if the names don't match.
func (l *Library) ReorderCategories(names []string) error {
	cats, err := l.ReadCategories()
	if err != nil {
		return err
	}

	normalizedNames := make([]string, len(names))
	for i, name := range names {
		normalizedNames[i], err = normalizeListName(name)
		if err != nil {
			return err
		}
	}

	currentByName := make(map[string]CategoryItem, len(cats.Categories))
	for _, cat := range cats.Categories {
		if _, exists := currentByName[cat.Name]; exists {
			return ErrCategoryAlreadyExists
		}
		currentByName[cat.Name] = cat
	}
	if len(currentByName) != len(normalizedNames) {
		return ErrCategoryMismatch
	}

	reordered := make([]CategoryItem, len(normalizedNames))
	for i, name := range normalizedNames {
		cat, ok := currentByName[name]
		if !ok {
			return ErrCategoryMismatch
		}
		reordered[i] = CategoryItem{Name: cat.Name, Order: i}
		delete(currentByName, name)
	}
	if len(currentByName) != 0 {
		return ErrCategoryMismatch
	}

	cats.Categories = reordered
	return l.WriteCategories(cats)
}

// ReadTagOrder reads .shelff/tags.json.
// Returns an empty TagOrder if the file does not exist.
func (l *Library) ReadTagOrder() (*TagOrder, error) {
	data, err := os.ReadFile(l.tagsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &TagOrder{
				Version:  SchemaVersion,
				TagOrder: []string{},
			}, nil
		}
		return nil, err
	}

	var tags TagOrder
	if err := json.Unmarshal(data, &tags); err != nil {
		return nil, err
	}

	tags.rawJSON = append([]byte(nil), data...)
	if tags.TagOrder == nil {
		tags.TagOrder = []string{}
	}
	if tags.Version == 0 {
		tags.Version = SchemaVersion
	}
	return &tags, nil
}

// WriteTagOrder writes .shelff/tags.json.
// Creates .shelff/ directory if it does not exist.
// It preserves unknown top-level fields present in tags.rawJSON (typically when
// tags originated from ReadTagOrder), but does not otherwise read or merge
// from any on-disk tags.json.
func (l *Library) WriteTagOrder(tags *TagOrder) error {
	normalized, err := normalizeTagOrder(tags)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(l.configDirPath(), 0o755); err != nil {
		return err
	}

	data, err := writeMergedJSONFile(l.tagsPath(), normalized, tags.rawJSON, knownTagOrderTopLevelKeys)
	if err != nil {
		return err
	}
	tags.rawJSON = append([]byte(nil), data...)
	tags.Version = normalized.Version
	tags.TagOrder = append([]string(nil), normalized.TagOrder...)
	return nil
}

// AddTagToOrder adds a tag to the display order list.
// This only affects display ordering — tags are primarily defined by their presence in sidecars.
// Returns ErrTagAlreadyExists if already in the order list.
func (l *Library) AddTagToOrder(name string) error {
	normalizedName, err := normalizeListName(name)
	if err != nil {
		return err
	}

	tags, err := l.ReadTagOrder()
	if err != nil {
		return err
	}
	for _, existing := range tags.TagOrder {
		if existing == normalizedName {
			return ErrTagAlreadyExists
		}
	}

	tags.TagOrder = append(tags.TagOrder, normalizedName)
	return l.WriteTagOrder(tags)
}

// RemoveTagFromOrder removes a tag from the display order list.
// If cascade is true, also removes the tag from all sidecars that reference it.
// Note: without cascade, this only removes the display ordering entry.
// If the tag is not present in tags.json, this is a no-op.
// Cascade updates are applied sequentially; if a sidecar update fails, already-written
// sidecars are not rolled back.
func (l *Library) RemoveTagFromOrder(name string, cascade bool) error {
	normalizedName, err := normalizeListName(name)
	if err != nil {
		return err
	}

	tags, err := l.ReadTagOrder()
	if err != nil {
		return err
	}

	filtered := make([]string, 0, len(tags.TagOrder))
	for _, tag := range tags.TagOrder {
		if tag != normalizedName {
			filtered = append(filtered, tag)
		}
	}
	if len(filtered) != len(tags.TagOrder) {
		tags.TagOrder = append([]string(nil), filtered...)
		if err := l.WriteTagOrder(tags); err != nil {
			return err
		}
	}
	if !cascade {
		return nil
	}

	return l.updateSidecars(func(meta *SidecarMetadata) bool {
		updatedTags, changed := removeTag(meta.Tags, normalizedName)
		if !changed {
			return false
		}
		meta.Tags = updatedTags
		return true
	})
}

// RenameTag renames a tag in the display order list.
// If cascade is true, updates the tag in all sidecars that reference the old name.
// If the old tag is absent from tags.json, only the cascade sidecar update is attempted.
// Cascade updates are applied sequentially; if a sidecar update fails, already-written
// sidecars are not rolled back.
func (l *Library) RenameTag(oldName string, newName string, cascade bool) error {
	normalizedOld, err := normalizeListName(oldName)
	if err != nil {
		return err
	}
	normalizedNew, err := normalizeListName(newName)
	if err != nil {
		return err
	}

	tags, err := l.ReadTagOrder()
	if err != nil {
		return err
	}

	index := slices.Index(tags.TagOrder, normalizedOld)
	if index != -1 && normalizedOld != normalizedNew && slices.Contains(tags.TagOrder, normalizedNew) {
		return ErrTagAlreadyExists
	}
	if index != -1 {
		tags.TagOrder[index] = normalizedNew
		if err := l.WriteTagOrder(tags); err != nil {
			return err
		}
	}
	if !cascade || normalizedOld == normalizedNew {
		return nil
	}

	return l.updateSidecars(func(meta *SidecarMetadata) bool {
		updatedTags, changed := renameTag(meta.Tags, normalizedOld, normalizedNew)
		if !changed {
			return false
		}
		meta.Tags = updatedTags
		return true
	})
}

// ReorderTags replaces the tag display order list.
// Unlike ReorderCategories, names do not need to match the existing set because
// tags.json only controls display order, not the canonical set of tags.
func (l *Library) ReorderTags(names []string) error {
	tags, err := l.ReadTagOrder()
	if err != nil {
		return err
	}

	tags.TagOrder = append(tags.TagOrder[:0], names...)
	return l.WriteTagOrder(tags)
}

func (l *Library) configDirPath() string {
	return filepath.Join(l.root, ConfigDir)
}

func (l *Library) categoriesPath() string {
	return filepath.Join(l.configDirPath(), CategoriesFile)
}

func (l *Library) tagsPath() string {
	return filepath.Join(l.configDirPath(), TagsFile)
}

func normalizeCategoryList(cats *CategoryList) (*CategoryList, error) {
	if cats == nil {
		return nil, errors.New("category list is nil")
	}

	normalized := *cats
	if normalized.Version == 0 {
		normalized.Version = SchemaVersion
	}
	normalized.Categories = make([]CategoryItem, len(cats.Categories))

	seen := make(map[string]struct{}, len(cats.Categories))
	for i, cat := range cats.Categories {
		name, err := normalizeListName(cat.Name)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[name]; exists {
			return nil, ErrCategoryAlreadyExists
		}
		seen[name] = struct{}{}
		normalized.Categories[i] = CategoryItem{
			Name:  name,
			Order: i,
		}
	}

	return &normalized, nil
}

func normalizeTagOrder(tags *TagOrder) (*TagOrder, error) {
	if tags == nil {
		return nil, errors.New("tag order is nil")
	}

	normalized := *tags
	if normalized.Version == 0 {
		normalized.Version = SchemaVersion
	}
	normalized.TagOrder = make([]string, 0, len(tags.TagOrder))

	seen := make(map[string]struct{}, len(tags.TagOrder))
	for _, tag := range tags.TagOrder {
		name, err := normalizeListName(tag)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[name]; exists {
			return nil, ErrTagAlreadyExists
		}
		seen[name] = struct{}{}
		normalized.TagOrder = append(normalized.TagOrder, name)
	}

	return &normalized, nil
}

func normalizeListName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", ErrEmptyName
	}
	return trimmed, nil
}

func removeTag(tags []string, target string) ([]string, bool) {
	if len(tags) == 0 {
		return tags, false
	}

	filtered := make([]string, 0, len(tags))
	changed := false
	for _, tag := range tags {
		if tag == target {
			changed = true
			continue
		}
		filtered = append(filtered, tag)
	}
	return filtered, changed
}

func renameTag(tags []string, oldName string, newName string) ([]string, bool) {
	if len(tags) == 0 {
		return tags, false
	}

	updated := make([]string, len(tags))
	changed := false
	for i, tag := range tags {
		if tag == oldName {
			tag = newName
			changed = true
		}
		updated[i] = tag
	}
	return updated, changed
}

// updateSidecars walks sidecars sequentially and writes back only modified entries.
// If one write fails, the walk stops and prior sidecar updates are left in place.
func (l *Library) updateSidecars(update func(meta *SidecarMetadata) bool) error {
	return filepath.WalkDir(l.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if path != l.root && d.Name() == ConfigDir {
				return filepath.SkipDir
			}
			return nil
		}

		if d.Type()&os.ModeSymlink != 0 || !d.Type().IsRegular() || !IsSidecarPath(path) {
			return nil
		}

		pdfPath, ok := PDFPathFromSidecar(path)
		if !ok {
			return nil
		}

		meta, err := ReadSidecar(pdfPath)
		if err != nil {
			return err
		}
		if meta == nil || !update(meta) {
			return nil
		}
		return WriteSidecar(pdfPath, meta)
	})
}

func stringPtr(value string) *string {
	return &value
}
