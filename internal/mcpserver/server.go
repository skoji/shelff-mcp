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

	"github.com/skoji/shelff-go/shelff"
)

var (
	ErrEmptyPath       = errors.New("path is empty")
	ErrAbsolutePath    = errors.New("path must be relative to the library root")
	ErrPathTraversal   = errors.New("path resolves outside the library root")
	ErrRootNotProvided = errors.New("library root must be provided via --root or SHELFF_ROOT")
)

type Server struct {
	library *shelff.Library
	server  *mcp.Server
}

type pdfPathInput struct {
	PDFPath string `json:"pdfPath" jsonschema:"Path to a PDF relative to the library root."`
}

type scanBooksInput struct {
	Recursive bool `json:"recursive" jsonschema:"Whether to scan subdirectories recursively."`
}

type readSidecarOutput struct {
	Exists  bool                    `json:"exists"`
	Sidecar *shelff.SidecarMetadata `json:"sidecar,omitempty"`
}

type scanBooksOutput struct {
	Books []bookEntryOutput `json:"books"`
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
		Name:        "scan_books",
		Description: "Scan the library for PDF files and whether they have sidecars.",
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
		Name:        "read_tag_order",
		Description: "Read the tag ordering configuration file.",
	}, s.readTagOrder)
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

func (s *Server) scanBooks(_ context.Context, _ *mcp.CallToolRequest, in scanBooksInput) (*mcp.CallToolResult, scanBooksOutput, error) {
	books, err := s.library.ScanBooks(in.Recursive)
	if err != nil {
		return nil, scanBooksOutput{}, err
	}

	out := scanBooksOutput{Books: make([]bookEntryOutput, 0, len(books))}
	for _, book := range books {
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

func (s *Server) readTagOrder(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, shelff.TagOrder, error) {
	tagOrder, err := s.library.ReadTagOrder()
	if err != nil {
		return nil, shelff.TagOrder{}, err
	}
	return nil, *tagOrder, nil
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

func toolNames() []string {
	names := []string{
		"read_sidecar",
		"scan_books",
		"find_orphaned_sidecars",
		"validate_sidecar",
		"library_stats",
		"collect_all_tags",
		"read_categories",
		"read_tag_order",
	}
	slices.Sort(names)
	return names
}
