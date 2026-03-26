package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skoji/shelff-go/shelff"
)

var (
	ErrEmptyPath       = errors.New("path is empty")
	ErrAbsolutePath    = errors.New("path must be relative to the library root")
	ErrPathTraversal   = errors.New("path resolves outside the library root")
	ErrRootNotProvided = errors.New("library root must be provided via --root or SHELFF_ROOT")
	ErrInvalidLimit    = errors.New("limit must be greater than 0")
	ErrInvalidOffset   = errors.New("offset must be greater than or equal to 0")
)

const defaultScanBooksLimit = 100

type Server struct {
	library *shelff.Library
	server  *mcp.Server
}

type pdfPathInput struct {
	PDFPath string `json:"pdfPath" jsonschema:"Path to a PDF relative to the library root."`
}

type scanBooksInput struct {
	Recursive bool    `json:"recursive" jsonschema:"Whether to scan subdirectories recursively."`
	Directory *string `json:"directory,omitempty" jsonschema:"Optional directory relative to the library root. Defaults to the root directory."`
	Limit     *int    `json:"limit,omitempty" jsonschema:"Maximum number of books to return. Defaults to 100."`
	Offset    *int    `json:"offset,omitempty" jsonschema:"Number of filtered books to skip before returning results. Defaults to 0."`
}

type writeSidecarInput struct {
	PDFPath string         `json:"pdfPath" jsonschema:"Path to a PDF relative to the library root."`
	Sidecar map[string]any `json:"sidecar" jsonschema:"Partial sidecar object to merge into the existing sidecar."`
}

type moveBookInput struct {
	PDFPath string `json:"pdfPath" jsonschema:"Path to a PDF relative to the library root."`
	DestDir string `json:"destDir" jsonschema:"Destination directory relative to the library root."`
}

type renameBookInput struct {
	PDFPath string `json:"pdfPath" jsonschema:"Path to a PDF relative to the library root."`
	NewName string `json:"newName" jsonschema:"New PDF base name, with or without the .pdf suffix."`
}

type nameInput struct {
	Name string `json:"name" jsonschema:"Name to add or remove."`
}

type cascadeNameInput struct {
	Name    string `json:"name" jsonschema:"Name to remove."`
	Cascade bool   `json:"cascade" jsonschema:"Whether to update matching sidecars too."`
}

type renameConfigInput struct {
	OldName string `json:"oldName" jsonschema:"Existing name."`
	NewName string `json:"newName" jsonschema:"Replacement name."`
	Cascade bool   `json:"cascade" jsonschema:"Whether to update matching sidecars too."`
}

type reorderNamesInput struct {
	Names []string `json:"names" jsonschema:"Replacement ordered list of names."`
}

type readSidecarOutput struct {
	Exists  bool                    `json:"exists"`
	Sidecar *shelff.SidecarMetadata `json:"sidecar,omitempty"`
}

type bookPathOutput struct {
	PDFPath string `json:"pdfPath"`
}

type scanBooksOutput struct {
	Books   []bookEntryOutput `json:"books"`
	Total   int               `json:"total"`
	Offset  int               `json:"offset"`
	Limit   int               `json:"limit"`
	HasMore bool              `json:"hasMore"`
}

type bookEntryOutput struct {
	PDFPath     string  `json:"pdfPath"`
	SidecarPath *string `json:"sidecarPath,omitempty"`
	HasSidecar  bool    `json:"hasSidecar"`
}

type orphanedSidecarsOutput struct {
	Sidecars []orphanedSidecarOutput `json:"sidecars"`
}

type orphanedSidecarOutput struct {
	SidecarPath string `json:"sidecarPath"`
	ExpectedPDF string `json:"expectedPDF"`
}

type validateSidecarOutput struct {
	Errors []string `json:"errors"`
}

type deleteSidecarOutput struct {
	Deleted bool `json:"deleted"`
}

type collectAllTagsOutput struct {
	Tags []string `json:"tags"`
}

func New(root string) (*Server, error) {
	library, err := openCanonicalLibrary(root)
	if err != nil {
		return nil, err
	}

	s := &Server{
		library: library,
		server: mcp.NewServer(&mcp.Implementation{
			Name:    "shelff-mcp",
			Title:   "shelff MCP",
			Version: buildVersion(),
		}, nil),
	}
	s.registerTools()
	return s, nil
}

func (s *Server) Run(ctx context.Context) error {
	return s.server.Run(ctx, &mcp.StdioTransport{})
}

func (s *Server) MCP() *mcp.Server {
	return s.server
}

func (s *Server) Root() string {
	return s.library.Root()
}

func (s *Server) registerTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "read_sidecar",
		Description: "Read sidecar metadata for a PDF path relative to the library root.",
	}, s.readSidecar)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "create_sidecar",
		Description: "Create a new sidecar for a PDF that does not already have one.",
	}, s.createSidecar)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "write_sidecar",
		Description: "Apply a partial sidecar update for a PDF, creating a sidecar first if needed.",
	}, s.writeSidecar)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "delete_sidecar",
		Description: "Delete the sidecar for a PDF if it exists.",
	}, s.deleteSidecar)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "move_book",
		Description: "Move a PDF and its sidecar to a different directory within the library.",
	}, s.moveBook)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "rename_book",
		Description: "Rename a PDF and its sidecar within the same directory.",
	}, s.renameBook)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "scan_books",
		Description: "Scan the library for PDF files and whether they have sidecars, with optional directory filtering and pagination.",
	}, s.scanBooks)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "find_orphaned_sidecars",
		Description: "List sidecar files whose corresponding PDF is missing.",
	}, s.findOrphanedSidecars)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "validate_sidecar",
		Description: "Validate a sidecar file against the shelff schema.",
	}, s.validateSidecar)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "library_stats",
		Description: "Compute aggregate statistics for the library.",
	}, s.libraryStats)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "collect_all_tags",
		Description: "Collect all tags in display order, then append uncategorized tags alphabetically.",
	}, s.collectAllTags)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "read_categories",
		Description: "Read the category configuration file.",
	}, s.readCategories)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "add_category",
		Description: "Add a category to the configuration list.",
	}, s.addCategory)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "remove_category",
		Description: "Remove a category from the configuration list, optionally cascading to sidecars.",
	}, s.removeCategory)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "rename_category",
		Description: "Rename a category in the configuration list, optionally cascading to sidecars.",
	}, s.renameCategory)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "reorder_categories",
		Description: "Replace the category order with the provided ordered name list.",
	}, s.reorderCategories)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "read_tag_order",
		Description: "Read the tag ordering configuration file.",
	}, s.readTagOrder)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "add_tag_to_order",
		Description: "Add a tag to the display order list.",
	}, s.addTagToOrder)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "remove_tag_from_order",
		Description: "Remove a tag from the display order list, optionally cascading to sidecars.",
	}, s.removeTagFromOrder)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "rename_tag",
		Description: "Rename a tag in the display order list, optionally cascading to sidecars.",
	}, s.renameTag)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "reorder_tags",
		Description: "Replace the tag display order list.",
	}, s.reorderTags)
}

func (s *Server) readSidecar(_ context.Context, _ *mcp.CallToolRequest, in pdfPathInput) (*mcp.CallToolResult, readSidecarOutput, error) {
	pdfPath, err := s.resolvePath(in.PDFPath)
	if err != nil {
		return nil, readSidecarOutput{}, err
	}

	sidecar, err := shelff.ReadSidecar(pdfPath)
	if err != nil {
		return nil, readSidecarOutput{}, err
	}
	return nil, readSidecarOutput{
		Exists:  sidecar != nil,
		Sidecar: sidecar,
	}, nil
}

func (s *Server) createSidecar(_ context.Context, _ *mcp.CallToolRequest, in pdfPathInput) (*mcp.CallToolResult, readSidecarOutput, error) {
	pdfPath, err := s.resolvePath(in.PDFPath)
	if err != nil {
		return nil, readSidecarOutput{}, err
	}

	sidecar, err := shelff.CreateSidecar(pdfPath)
	if err != nil {
		return nil, readSidecarOutput{}, err
	}
	return nil, readSidecarOutput{
		Exists:  true,
		Sidecar: sidecar,
	}, nil
}

func (s *Server) writeSidecar(_ context.Context, _ *mcp.CallToolRequest, in writeSidecarInput) (*mcp.CallToolResult, readSidecarOutput, error) {
	pdfPath, err := s.resolvePath(in.PDFPath)
	if err != nil {
		return nil, readSidecarOutput{}, err
	}
	if in.Sidecar == nil {
		in.Sidecar = map[string]any{}
	}

	existing, err := shelff.ReadSidecar(pdfPath)
	if err != nil {
		return nil, readSidecarOutput{}, err
	}
	if existing == nil {
		existing, err = shelff.CreateSidecar(pdfPath)
		if err != nil {
			return nil, readSidecarOutput{}, err
		}
	}

	currentMap, err := sidecarToMap(pdfPath, existing)
	if err != nil {
		return nil, readSidecarOutput{}, err
	}
	merged := mergeJSONObject(currentMap, in.Sidecar)
	merged = normalizeMergedSidecar(currentMap, merged)

	next, err := mapToSidecar(merged)
	if err != nil {
		return nil, readSidecarOutput{}, err
	}
	if err := shelff.WriteSidecar(pdfPath, next); err != nil {
		return nil, readSidecarOutput{}, err
	}
	written, err := shelff.ReadSidecar(pdfPath)
	if err != nil {
		return nil, readSidecarOutput{}, err
	}

	return nil, readSidecarOutput{
		Exists:  true,
		Sidecar: written,
	}, nil
}

func (s *Server) deleteSidecar(_ context.Context, _ *mcp.CallToolRequest, in pdfPathInput) (*mcp.CallToolResult, deleteSidecarOutput, error) {
	pdfPath, err := s.resolvePath(in.PDFPath)
	if err != nil {
		return nil, deleteSidecarOutput{}, err
	}

	err = os.Remove(shelff.SidecarPath(pdfPath))
	switch {
	case err == nil:
		return nil, deleteSidecarOutput{Deleted: true}, nil
	case os.IsNotExist(err):
		return nil, deleteSidecarOutput{Deleted: false}, nil
	default:
		return nil, deleteSidecarOutput{}, err
	}
}

func (s *Server) moveBook(_ context.Context, _ *mcp.CallToolRequest, in moveBookInput) (*mcp.CallToolResult, bookPathOutput, error) {
	pdfPath, err := s.resolvePath(in.PDFPath)
	if err != nil {
		return nil, bookPathOutput{}, err
	}
	destDir, err := s.resolveDirectoryPath(in.DestDir)
	if err != nil {
		return nil, bookPathOutput{}, err
	}

	newPDFPath, err := shelff.MoveBook(pdfPath, destDir)
	if err != nil {
		return nil, bookPathOutput{}, err
	}
	relative, err := s.relativePath(newPDFPath)
	if err != nil {
		return nil, bookPathOutput{}, err
	}
	return nil, bookPathOutput{PDFPath: relative}, nil
}

func (s *Server) renameBook(_ context.Context, _ *mcp.CallToolRequest, in renameBookInput) (*mcp.CallToolResult, bookPathOutput, error) {
	pdfPath, err := s.resolvePath(in.PDFPath)
	if err != nil {
		return nil, bookPathOutput{}, err
	}

	newPDFPath, err := shelff.RenameBook(pdfPath, in.NewName)
	if err != nil {
		return nil, bookPathOutput{}, err
	}
	relative, err := s.relativePath(newPDFPath)
	if err != nil {
		return nil, bookPathOutput{}, err
	}
	return nil, bookPathOutput{PDFPath: relative}, nil
}

func (s *Server) scanBooks(_ context.Context, _ *mcp.CallToolRequest, in scanBooksInput) (*mcp.CallToolResult, scanBooksOutput, error) {
	limit := defaultScanBooksLimit
	if in.Limit != nil {
		limit = *in.Limit
	}
	if limit <= 0 {
		return nil, scanBooksOutput{}, ErrInvalidLimit
	}

	offset := 0
	if in.Offset != nil {
		offset = *in.Offset
	}
	if offset < 0 {
		return nil, scanBooksOutput{}, ErrInvalidOffset
	}

	directory := s.library.Root()
	if in.Directory != nil && strings.TrimSpace(*in.Directory) != "" {
		var err error
		directory, err = s.resolveDirectoryPath(*in.Directory)
		if err != nil {
			return nil, scanBooksOutput{}, err
		}
	}

	books, err := s.library.ScanBooksInDirectory(directory, in.Recursive)
	if err != nil {
		return nil, scanBooksOutput{}, err
	}

	total := len(books)
	start := min(offset, total)
	end := min(start+limit, total)
	page := books[start:end]

	out := scanBooksOutput{
		Books:   make([]bookEntryOutput, 0, len(page)),
		Total:   total,
		Offset:  offset,
		Limit:   limit,
		HasMore: end < total,
	}
	for _, book := range page {
		pdfPath, err := s.relativePath(book.PDFPath)
		if err != nil {
			return nil, scanBooksOutput{}, err
		}
		item := bookEntryOutput{
			PDFPath:    pdfPath,
			HasSidecar: book.HasSidecar,
		}
		if book.SidecarPath != nil {
			sidecarPath, err := s.relativePath(*book.SidecarPath)
			if err != nil {
				return nil, scanBooksOutput{}, err
			}
			item.SidecarPath = &sidecarPath
		}
		out.Books = append(out.Books, item)
	}
	return nil, out, nil
}

func (s *Server) findOrphanedSidecars(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, orphanedSidecarsOutput, error) {
	orphaned, err := s.library.FindOrphanedSidecars()
	if err != nil {
		return nil, orphanedSidecarsOutput{}, err
	}

	out := orphanedSidecarsOutput{Sidecars: make([]orphanedSidecarOutput, 0, len(orphaned))}
	for _, sidecar := range orphaned {
		sidecarPath, err := s.relativePath(sidecar.SidecarPath)
		if err != nil {
			return nil, orphanedSidecarsOutput{}, err
		}
		expectedPDF, err := s.relativePath(sidecar.ExpectedPDF)
		if err != nil {
			return nil, orphanedSidecarsOutput{}, err
		}
		out.Sidecars = append(out.Sidecars, orphanedSidecarOutput{
			SidecarPath: sidecarPath,
			ExpectedPDF: expectedPDF,
		})
	}
	return nil, out, nil
}

func (s *Server) validateSidecar(_ context.Context, _ *mcp.CallToolRequest, in pdfPathInput) (*mcp.CallToolResult, validateSidecarOutput, error) {
	pdfPath, err := s.resolvePath(in.PDFPath)
	if err != nil {
		return nil, validateSidecarOutput{}, err
	}

	validationErrors, err := s.library.Validate(pdfPath)
	if err != nil {
		return nil, validateSidecarOutput{}, err
	}
	return nil, validateSidecarOutput{Errors: validationErrors}, nil
}

func (s *Server) libraryStats(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, shelff.LibraryStats, error) {
	stats, err := s.library.Stats()
	if err != nil {
		return nil, shelff.LibraryStats{}, err
	}
	return nil, *stats, nil
}

func (s *Server) collectAllTags(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, collectAllTagsOutput, error) {
	tags, err := s.library.CollectAllTags()
	if err != nil {
		return nil, collectAllTagsOutput{}, err
	}
	return nil, collectAllTagsOutput{Tags: tags}, nil
}

func (s *Server) readCategories(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, shelff.CategoryList, error) {
	categories, err := s.library.ReadCategories()
	if err != nil {
		return nil, shelff.CategoryList{}, err
	}
	return nil, *categories, nil
}

func (s *Server) addCategory(ctx context.Context, _ *mcp.CallToolRequest, in nameInput) (*mcp.CallToolResult, shelff.CategoryList, error) {
	if err := s.library.AddCategory(in.Name); err != nil {
		return nil, shelff.CategoryList{}, err
	}
	return s.readCategories(ctx, nil, struct{}{})
}

func (s *Server) removeCategory(ctx context.Context, _ *mcp.CallToolRequest, in cascadeNameInput) (*mcp.CallToolResult, shelff.CategoryList, error) {
	if err := s.library.RemoveCategory(in.Name, in.Cascade); err != nil {
		return nil, shelff.CategoryList{}, err
	}
	return s.readCategories(ctx, nil, struct{}{})
}

func (s *Server) renameCategory(ctx context.Context, _ *mcp.CallToolRequest, in renameConfigInput) (*mcp.CallToolResult, shelff.CategoryList, error) {
	if err := s.library.RenameCategory(in.OldName, in.NewName, in.Cascade); err != nil {
		return nil, shelff.CategoryList{}, err
	}
	return s.readCategories(ctx, nil, struct{}{})
}

func (s *Server) reorderCategories(ctx context.Context, _ *mcp.CallToolRequest, in reorderNamesInput) (*mcp.CallToolResult, shelff.CategoryList, error) {
	if err := s.library.ReorderCategories(in.Names); err != nil {
		return nil, shelff.CategoryList{}, err
	}
	return s.readCategories(ctx, nil, struct{}{})
}

func (s *Server) readTagOrder(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, shelff.TagOrder, error) {
	tagOrder, err := s.library.ReadTagOrder()
	if err != nil {
		return nil, shelff.TagOrder{}, err
	}
	return nil, *tagOrder, nil
}

func (s *Server) addTagToOrder(ctx context.Context, _ *mcp.CallToolRequest, in nameInput) (*mcp.CallToolResult, shelff.TagOrder, error) {
	if err := s.library.AddTagToOrder(in.Name); err != nil {
		return nil, shelff.TagOrder{}, err
	}
	return s.readTagOrder(ctx, nil, struct{}{})
}

func (s *Server) removeTagFromOrder(ctx context.Context, _ *mcp.CallToolRequest, in cascadeNameInput) (*mcp.CallToolResult, shelff.TagOrder, error) {
	if err := s.library.RemoveTagFromOrder(in.Name, in.Cascade); err != nil {
		return nil, shelff.TagOrder{}, err
	}
	return s.readTagOrder(ctx, nil, struct{}{})
}

func (s *Server) renameTag(ctx context.Context, _ *mcp.CallToolRequest, in renameConfigInput) (*mcp.CallToolResult, shelff.TagOrder, error) {
	if err := s.library.RenameTag(in.OldName, in.NewName, in.Cascade); err != nil {
		return nil, shelff.TagOrder{}, err
	}
	return s.readTagOrder(ctx, nil, struct{}{})
}

func (s *Server) reorderTags(ctx context.Context, _ *mcp.CallToolRequest, in reorderNamesInput) (*mcp.CallToolResult, shelff.TagOrder, error) {
	if err := s.library.ReorderTags(in.Names); err != nil {
		return nil, shelff.TagOrder{}, err
	}
	return s.readTagOrder(ctx, nil, struct{}{})
}

func openCanonicalLibrary(root string) (*shelff.Library, error) {
	if strings.TrimSpace(root) == "" {
		return nil, ErrRootNotProvided
	}

	library, err := shelff.OpenLibrary(root)
	if err != nil {
		return nil, err
	}

	resolvedRoot, err := filepath.EvalSymlinks(library.Root())
	if err != nil {
		return nil, err
	}
	if resolvedRoot == library.Root() {
		return library, nil
	}
	return shelff.OpenLibrary(resolvedRoot)
}

func (s *Server) resolvePath(relative string) (string, error) {
	if strings.TrimSpace(relative) == "" {
		return "", ErrEmptyPath
	}

	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." {
		return "", ErrEmptyPath
	}
	if filepath.IsAbs(clean) {
		return "", ErrAbsolutePath
	}

	joined := filepath.Join(s.library.Root(), clean)
	absolute, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	if err := s.ensureWithinRoot(absolute); err != nil {
		return "", err
	}

	resolved, err := resolveExistingPath(absolute)
	if err != nil {
		return "", err
	}
	if err := s.ensureWithinRoot(resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

func (s *Server) resolveDirectoryPath(relative string) (string, error) {
	if strings.TrimSpace(relative) == "" {
		return "", ErrEmptyPath
	}
	if filepath.Clean(filepath.FromSlash(relative)) == "." {
		return s.library.Root(), nil
	}
	return s.resolvePath(relative)
}

func resolveExistingPath(path string) (string, error) {
	current := path
	var suffix []string

	for {
		_, err := os.Lstat(current)
		switch {
		case err == nil:
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", err
			}
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return resolved, nil
		case !os.IsNotExist(err):
			return "", err
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", os.ErrNotExist
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func (s *Server) ensureWithinRoot(path string) error {
	rel, err := filepath.Rel(s.library.Root(), path)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ErrPathTraversal
	}
	return nil
}

func (s *Server) relativePath(path string) (string, error) {
	rel, err := filepath.Rel(s.library.Root(), path)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrPathTraversal
	}
	return filepath.ToSlash(rel), nil
}

func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return "dev"
	}
	return info.Main.Version
}

func sidecarToMap(pdfPath string, meta *shelff.SidecarMetadata) (map[string]any, error) {
	data, err := os.ReadFile(shelff.SidecarPath(pdfPath))
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		data, err = json.Marshal(meta)
		if err != nil {
			return nil, err
		}
	}

	var decoded map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if err == nil {
			return nil, errors.New("invalid JSON: unexpected trailing data")
		}
		return nil, err
	}
	if decoded == nil {
		decoded = map[string]any{}
	}
	return decoded, nil
}

func mapToSidecar(value map[string]any) (*shelff.SidecarMetadata, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return shelff.ParseSidecarJSON(data)
}

func mergeJSONObject(current map[string]any, patch map[string]any) map[string]any {
	if current == nil {
		current = map[string]any{}
	}

	merged := cloneJSONObject(current)
	for key, patchValue := range patch {
		if patchValue == nil {
			delete(merged, key)
			continue
		}

		patchObject, patchIsObject := patchValue.(map[string]any)
		currentObject, currentIsObject := merged[key].(map[string]any)
		if patchIsObject && currentIsObject {
			merged[key] = mergeJSONObject(currentObject, patchObject)
			continue
		}
		merged[key] = cloneJSONValue(patchValue)
	}
	return merged
}

func normalizeMergedSidecar(current map[string]any, merged map[string]any) map[string]any {
	merged["schemaVersion"] = shelff.SchemaVersion

	currentMetadata, _ := current["metadata"].(map[string]any)
	mergedMetadata, ok := merged["metadata"].(map[string]any)
	if !ok || mergedMetadata == nil {
		mergedMetadata = cloneJSONObject(currentMetadata)
	}
	if currentMetadata != nil {
		if title, ok := currentMetadata["dc:title"]; ok {
			if _, present := mergedMetadata["dc:title"]; !present || mergedMetadata["dc:title"] == nil {
				mergedMetadata["dc:title"] = title
			}
		}
	}
	merged["metadata"] = mergedMetadata
	normalizeRequiredObject(merged, "reading", "lastReadAt", "lastReadPage", "totalPages")
	normalizeRequiredObject(merged, "display", "direction")

	return merged
}

func normalizeRequiredObject(merged map[string]any, key string, requiredKeys ...string) {
	raw, ok := merged[key]
	if !ok {
		return
	}

	object, ok := raw.(map[string]any)
	if !ok || object == nil {
		delete(merged, key)
		return
	}

	for _, requiredKey := range requiredKeys {
		if _, present := object[requiredKey]; !present || object[requiredKey] == nil {
			delete(merged, key)
			return
		}
	}
}

func cloneJSONObject(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	cloned := make(map[string]any, len(value))
	for key, child := range value {
		cloned[key] = cloneJSONValue(child)
	}
	return cloned
}

func cloneJSONValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return cloneJSONObject(v)
	case []any:
		cloned := make([]any, len(v))
		for i, child := range v {
			cloned[i] = cloneJSONValue(child)
		}
		return cloned
	default:
		return value
	}
}

func toolNames() []string {
	names := []string{
		"add_category",
		"add_tag_to_order",
		"read_sidecar",
		"create_sidecar",
		"write_sidecar",
		"delete_sidecar",
		"move_book",
		"rename_book",
		"scan_books",
		"find_orphaned_sidecars",
		"validate_sidecar",
		"library_stats",
		"collect_all_tags",
		"read_categories",
		"remove_category",
		"rename_category",
		"reorder_categories",
		"read_tag_order",
		"remove_tag_from_order",
		"rename_tag",
		"reorder_tags",
	}
	slices.Sort(names)
	return names
}
