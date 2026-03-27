package mcpserver

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//go:generate mkdir -p specs
//go:generate cp ../../shelff-schema/SPECIFICATION.md specs/SPECIFICATION.md
//go:generate cp ../../shelff-schema/sidecar.schema.json specs/sidecar.schema.json
//go:generate cp ../../shelff-schema/categories.schema.json specs/categories.schema.json
//go:generate cp ../../shelff-schema/tags.schema.json specs/tags.schema.json

//go:embed specs/SPECIFICATION.md
var specOverview string

//go:embed specs/sidecar.schema.json
var specSidecarSchema string

//go:embed specs/categories.schema.json
var specCategoriesSchema string

//go:embed specs/tags.schema.json
var specTagsSchema string

func unknownTopicError(topic string) error {
	return fmt.Errorf("unknown topic %q: valid topics are overview, sidecar, categories, tags, all", topic)
}

type getSpecificationInput struct {
	Topic string `json:"topic,omitempty" jsonschema:"Topic to retrieve: overview (default), sidecar, categories, tags, or all."`
}

type getSpecificationOutput struct {
	Topic   string `json:"topic"`
	Content string `json:"content"`
}

func (s *Server) getSpecification(_ context.Context, _ *mcp.CallToolRequest, in getSpecificationInput) (*mcp.CallToolResult, getSpecificationOutput, error) {
	topic := in.Topic
	if topic == "" {
		topic = "overview"
	}

	content, err := specificationContent(topic)
	if err != nil {
		return nil, getSpecificationOutput{}, err
	}

	return nil, getSpecificationOutput{
		Topic:   topic,
		Content: content,
	}, nil
}

func specificationContent(topic string) (string, error) {
	switch topic {
	case "overview":
		return specOverview, nil
	case "sidecar":
		return specSidecarSchema, nil
	case "categories":
		return specCategoriesSchema, nil
	case "tags":
		return specTagsSchema, nil
	case "all":
		var b strings.Builder
		b.WriteString(specOverview)
		b.WriteString("\n\n# Sidecar Schema\n\n")
		b.WriteString(specSidecarSchema)
		b.WriteString("\n\n# Categories Schema\n\n")
		b.WriteString(specCategoriesSchema)
		b.WriteString("\n\n# Tags Schema\n\n")
		b.WriteString(specTagsSchema)
		return b.String(), nil
	default:
		return "", unknownTopicError(topic)
	}
}
