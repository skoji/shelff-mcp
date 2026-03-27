package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGetSpecificationDefaultReturnsOverview(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, t.TempDir())
	session := newClientSession(t, server)
	defer session.Close()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_specification",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool IsError = true")
	}

	var out getSpecificationOutput
	decodeStructuredContent(t, result, &out)

	if out.Topic != "overview" {
		t.Fatalf("topic = %q, want %q", out.Topic, "overview")
	}
	if !strings.Contains(out.Content, "shelff Metadata Specification") {
		t.Fatal("content should contain SPECIFICATION.md header")
	}
}

func TestGetSpecificationOverview(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, t.TempDir())
	session := newClientSession(t, server)
	defer session.Close()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_specification",
		Arguments: map[string]any{"topic": "overview"},
	})
	if err != nil {
		t.Fatalf("CallTool error = %v", err)
	}

	var out getSpecificationOutput
	decodeStructuredContent(t, result, &out)

	if out.Topic != "overview" {
		t.Fatalf("topic = %q, want %q", out.Topic, "overview")
	}
	if !strings.Contains(out.Content, "Filesystem Layout") {
		t.Fatal("content should contain Filesystem Layout section")
	}
}

func TestGetSpecificationSidecar(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, t.TempDir())
	session := newClientSession(t, server)
	defer session.Close()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_specification",
		Arguments: map[string]any{"topic": "sidecar"},
	})
	if err != nil {
		t.Fatalf("CallTool error = %v", err)
	}

	var out getSpecificationOutput
	decodeStructuredContent(t, result, &out)

	if out.Topic != "sidecar" {
		t.Fatalf("topic = %q, want %q", out.Topic, "sidecar")
	}
	if !strings.Contains(out.Content, "shelff Sidecar Metadata") {
		t.Fatal("content should contain sidecar schema title")
	}
	if !strings.Contains(out.Content, "DublinCoreMetadata") {
		t.Fatal("content should contain DublinCoreMetadata definition")
	}
}

func TestGetSpecificationCategories(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, t.TempDir())
	session := newClientSession(t, server)
	defer session.Close()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_specification",
		Arguments: map[string]any{"topic": "categories"},
	})
	if err != nil {
		t.Fatalf("CallTool error = %v", err)
	}

	var out getSpecificationOutput
	decodeStructuredContent(t, result, &out)

	if out.Topic != "categories" {
		t.Fatalf("topic = %q, want %q", out.Topic, "categories")
	}
	if !strings.Contains(out.Content, "shelff Category List") {
		t.Fatal("content should contain categories schema title")
	}
}

func TestGetSpecificationTags(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, t.TempDir())
	session := newClientSession(t, server)
	defer session.Close()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_specification",
		Arguments: map[string]any{"topic": "tags"},
	})
	if err != nil {
		t.Fatalf("CallTool error = %v", err)
	}

	var out getSpecificationOutput
	decodeStructuredContent(t, result, &out)

	if out.Topic != "tags" {
		t.Fatalf("topic = %q, want %q", out.Topic, "tags")
	}
	if !strings.Contains(out.Content, "shelff Tag Order") {
		t.Fatal("content should contain tags schema title")
	}
}

func TestGetSpecificationAll(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, t.TempDir())
	session := newClientSession(t, server)
	defer session.Close()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_specification",
		Arguments: map[string]any{"topic": "all"},
	})
	if err != nil {
		t.Fatalf("CallTool error = %v", err)
	}

	var out getSpecificationOutput
	decodeStructuredContent(t, result, &out)

	if out.Topic != "all" {
		t.Fatalf("topic = %q, want %q", out.Topic, "all")
	}
	// all should include everything
	if !strings.Contains(out.Content, "shelff Metadata Specification") {
		t.Fatal("all content should contain overview")
	}
	if !strings.Contains(out.Content, "DublinCoreMetadata") {
		t.Fatal("all content should contain sidecar schema")
	}
	if !strings.Contains(out.Content, "shelff Category List") {
		t.Fatal("all content should contain categories schema")
	}
	if !strings.Contains(out.Content, "shelff Tag Order") {
		t.Fatal("all content should contain tags schema")
	}
}

func TestGetSpecificationInvalidTopic(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, t.TempDir())
	session := newClientSession(t, server)
	defer session.Close()

	assertToolErrorContains(t, session, "get_specification", map[string]any{"topic": "nonexistent"}, "unknown topic")
}
