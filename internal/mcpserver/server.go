package mcpserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skoji/shelff-mcp/shelff"
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

type writeMetadataInput struct {
	PDFPath  string         `json:"pdfPath" jsonschema:"Path to a PDF relative to the library root."`
	Metadata map[string]any `json:"metadata" jsonschema:"Partial metadata object to merge into the existing metadata."`
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

type readMetadataOutput struct {
	HasSidecar bool                    `json:"hasSidecar"`
	Metadata   *shelff.SidecarMetadata `json:"metadata,omitempty"`
}

type readCategoriesOutput struct {
	Exists     bool                 `json:"exists"`
	Categories *shelff.CategoryList `json:"categories,omitempty"`
}

type readTagOrderOutput struct {
	Exists   bool             `json:"exists"`
	TagOrder *shelff.TagOrder `json:"tagOrder,omitempty"`
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

type listDirectoriesInput struct {
	Recursive bool    `json:"recursive" jsonschema:"Whether to list subdirectories recursively."`
	Directory *string `json:"directory,omitempty" jsonschema:"Optional directory relative to the library root. Defaults to the root directory."`
}

type listDirectoriesOutput struct {
	Directories []string `json:"directories"`
}

type createDirectoryInput struct {
	Path string `json:"path" jsonschema:"Directory path relative to the library root. Parent directories are created automatically."`
}

type createDirectoryOutput struct {
	Path string `json:"path"`
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
		Name:        "get_specification",
		Description: "Return shelff specification and JSON schemas. Call this to learn about sidecar JSON structure, field meanings, and file conventions before using other tools.",
	}, s.getSpecification)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "read_metadata",
		Description: "Read metadata for a PDF path relative to the library root. Returns minimal metadata (title from filename) even when no sidecar file exists.",
	}, s.readMetadata)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "create_sidecar",
		Description: "Create a new sidecar for a PDF that does not already have one.",
	}, s.createSidecar)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "write_metadata",
		Description: "Apply a partial metadata update for a PDF, creating a sidecar first if needed.",
	}, s.writeMetadata)
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
		Name:        "check_library",
		Description: "Run diagnostic checks: config file presence, category/tag integrity, orphaned sidecars, and book counts.",
	}, s.checkLibrary)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "collect_all_tags",
		Description: "Collect all tags in display order, then append uncategorized tags alphabetically.",
	}, s.collectAllTags)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_directories",
		Description: "List directories in the library, optionally filtered to a subdirectory.",
	}, s.listDirectories)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "create_directory",
		Description: "Create a directory (and parent directories) within the library.",
	}, s.createDirectory)
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

func (s *Server) readMetadata(_ context.Context, _ *mcp.CallToolRequest, in pdfPathInput) (*mcp.CallToolResult, readMetadataOutput, error) {
	pdfPath, err := s.resolvePath(in.PDFPath)
	if err != nil {
		return nil, readMetadataOutput{}, err
	}

	_, statErr := os.Stat(shelff.SidecarPath(pdfPath))
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, readMetadataOutput{}, statErr
	}
	hasSidecar := statErr == nil

	meta, err := shelff.ReadMetadata(pdfPath)
	if err != nil {
		return nil, readMetadataOutput{}, err
	}
	return nil, readMetadataOutput{
		HasSidecar: hasSidecar,
		Metadata: meta,
	}, nil
}

func (s *Server) createSidecar(_ context.Context, _ *mcp.CallToolRequest, in pdfPathInput) (*mcp.CallToolResult, readMetadataOutput, error) {
	pdfPath, err := s.resolvePath(in.PDFPath)
	if err != nil {
		return nil, readMetadataOutput{}, err
	}

	sidecar, err := shelff.CreateSidecar(pdfPath)
	if err != nil {
		return nil, readMetadataOutput{}, err
	}
	return nil, readMetadataOutput{
		HasSidecar: true,
		Metadata:   sidecar,
	}, nil
}

func (s *Server) writeMetadata(_ context.Context, _ *mcp.CallToolRequest, in writeMetadataInput) (*mcp.CallToolResult, readMetadataOutput, error) {
	pdfPath, err := s.resolvePath(in.PDFPath)
	if err != nil {
		return nil, readMetadataOutput{}, err
	}

	written, err := shelff.WriteMetadata(pdfPath, in.Metadata)
	if err != nil {
		return nil, readMetadataOutput{}, err
	}

	return nil, readMetadataOutput{
		HasSidecar: true,
		Metadata:   written,
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

func (s *Server) listDirectories(_ context.Context, _ *mcp.CallToolRequest, in listDirectoriesInput) (*mcp.CallToolResult, listDirectoriesOutput, error) {
	var directory string
	if in.Directory != nil {
		directory = *in.Directory
	}
	dirs, err := s.library.ListDirectories(directory, in.Recursive)
	if err != nil {
		return nil, listDirectoriesOutput{}, mapPathOutsideRoot(err)
	}
	return nil, listDirectoriesOutput{Directories: dirs}, nil
}

func (s *Server) createDirectory(_ context.Context, _ *mcp.CallToolRequest, in createDirectoryInput) (*mcp.CallToolResult, createDirectoryOutput, error) {
	if err := s.library.MakeDirectory(in.Path); err != nil {
		return nil, createDirectoryOutput{}, mapPathOutsideRoot(err)
	}
	return nil, createDirectoryOutput{Path: in.Path}, nil
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

func (s *Server) checkLibrary(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, shelff.CheckLibraryResult, error) {
	result, err := s.library.CheckLibrary()
	if err != nil {
		return nil, shelff.CheckLibraryResult{}, err
	}
	return nil, *result, nil
}

func (s *Server) collectAllTags(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, collectAllTagsOutput, error) {
	tags, err := s.library.CollectAllTags()
	if err != nil {
		return nil, collectAllTagsOutput{}, err
	}
	return nil, collectAllTagsOutput{Tags: tags}, nil
}

func (s *Server) readCategories(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, readCategoriesOutput, error) {
	categories, err := s.library.ReadCategories()
	if err != nil {
		return nil, readCategoriesOutput{}, err
	}
	return nil, readCategoriesOutput{
		Exists:     categories != nil,
		Categories: categories,
	}, nil
}

func (s *Server) addCategory(ctx context.Context, _ *mcp.CallToolRequest, in nameInput) (*mcp.CallToolResult, readCategoriesOutput, error) {
	if err := s.library.AddCategory(in.Name); err != nil {
		return nil, readCategoriesOutput{}, err
	}
	return s.readCategories(ctx, nil, struct{}{})
}

func (s *Server) removeCategory(ctx context.Context, _ *mcp.CallToolRequest, in cascadeNameInput) (*mcp.CallToolResult, readCategoriesOutput, error) {
	if err := s.library.RemoveCategory(in.Name, in.Cascade); err != nil {
		return nil, readCategoriesOutput{}, err
	}
	return s.readCategories(ctx, nil, struct{}{})
}

func (s *Server) renameCategory(ctx context.Context, _ *mcp.CallToolRequest, in renameConfigInput) (*mcp.CallToolResult, readCategoriesOutput, error) {
	if err := s.library.RenameCategory(in.OldName, in.NewName, in.Cascade); err != nil {
		return nil, readCategoriesOutput{}, err
	}
	return s.readCategories(ctx, nil, struct{}{})
}

func (s *Server) reorderCategories(ctx context.Context, _ *mcp.CallToolRequest, in reorderNamesInput) (*mcp.CallToolResult, readCategoriesOutput, error) {
	if err := s.library.ReorderCategories(in.Names); err != nil {
		return nil, readCategoriesOutput{}, err
	}
	return s.readCategories(ctx, nil, struct{}{})
}

func (s *Server) readTagOrder(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, readTagOrderOutput, error) {
	tagOrder, err := s.library.ReadTagOrder()
	if err != nil {
		return nil, readTagOrderOutput{}, err
	}
	return nil, readTagOrderOutput{
		Exists:   tagOrder != nil,
		TagOrder: tagOrder,
	}, nil
}

func (s *Server) addTagToOrder(ctx context.Context, _ *mcp.CallToolRequest, in nameInput) (*mcp.CallToolResult, readTagOrderOutput, error) {
	if err := s.library.AddTagToOrder(in.Name); err != nil {
		return nil, readTagOrderOutput{}, err
	}
	return s.readTagOrder(ctx, nil, struct{}{})
}

func (s *Server) removeTagFromOrder(ctx context.Context, _ *mcp.CallToolRequest, in cascadeNameInput) (*mcp.CallToolResult, readTagOrderOutput, error) {
	if err := s.library.RemoveTagFromOrder(in.Name, in.Cascade); err != nil {
		return nil, readTagOrderOutput{}, err
	}
	return s.readTagOrder(ctx, nil, struct{}{})
}

func (s *Server) renameTag(ctx context.Context, _ *mcp.CallToolRequest, in renameConfigInput) (*mcp.CallToolResult, readTagOrderOutput, error) {
	if err := s.library.RenameTag(in.OldName, in.NewName, in.Cascade); err != nil {
		return nil, readTagOrderOutput{}, err
	}
	return s.readTagOrder(ctx, nil, struct{}{})
}

func (s *Server) reorderTags(ctx context.Context, _ *mcp.CallToolRequest, in reorderNamesInput) (*mcp.CallToolResult, readTagOrderOutput, error) {
	if err := s.library.ReorderTags(in.Names); err != nil {
		return nil, readTagOrderOutput{}, err
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

func mapPathOutsideRoot(err error) error {
	if errors.Is(err, shelff.ErrPathOutsideRoot) {
		return ErrPathTraversal
	}
	return err
}

func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return "dev"
	}
	return info.Main.Version
}

func toolNames() []string {
	names := []string{
		"add_category",
		"add_tag_to_order",
		"check_library",
		"create_directory",
		"get_specification",
		"read_metadata",
		"create_sidecar",
		"write_metadata",
		"delete_sidecar",
		"move_book",
		"rename_book",
		"scan_books",
		"find_orphaned_sidecars",
		"validate_sidecar",
		"list_directories",
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
