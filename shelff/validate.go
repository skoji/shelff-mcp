package shelff

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
)

//go:embed schema/sidecar.schema.json
var embeddedSidecarSchema []byte

type jsonSchema struct {
	Type                 string                 `json:"type"`
	Required             []string               `json:"required"`
	AdditionalProperties *bool                  `json:"additionalProperties,omitempty"`
	Properties           map[string]*jsonSchema `json:"properties,omitempty"`
	Items                *jsonSchema            `json:"items,omitempty"`
	Ref                  string                 `json:"$ref,omitempty"`
	Const                any                    `json:"const,omitempty"`
	Enum                 []string               `json:"enum,omitempty"`
	Minimum              *int64                 `json:"minimum,omitempty"`
	Format               string                 `json:"format,omitempty"`
	Defs                 map[string]*jsonSchema `json:"$defs,omitempty"`
}

// Validate validates a sidecar JSON file against the schema.
// Returns a list of validation errors (empty if valid).
func (l *Library) Validate(pdfPath string) ([]string, error) {
	data, err := os.ReadFile(SidecarPath(pdfPath))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrSidecarNotFound
		}
		return nil, err
	}

	value, err := jsonBytesToMap(data)
	if err != nil {
		return []string{fmt.Sprintf("invalid JSON: %v", err)}, nil
	}

	schema, err := loadSidecarSchema()
	if err != nil {
		return nil, err
	}

	var validationErrors []string
	validateJSONValue("$", value, schema, schema, &validationErrors)
	return validationErrors, nil
}

func loadSidecarSchema() (*jsonSchema, error) {
	var schema jsonSchema
	if err := json.Unmarshal(embeddedSidecarSchema, &schema); err != nil {
		return nil, err
	}
	return &schema, nil
}

func validateJSONValue(path string, value any, schema *jsonSchema, root *jsonSchema, errs *[]string) {
	schema = resolveSchema(schema, root)
	if schema == nil {
		return
	}

	if schema.Const != nil && !matchesConst(value, schema.Const) {
		*errs = append(*errs, fmt.Sprintf("%s: expected const %v, got %v", path, schema.Const, value))
	}

	switch schema.Type {
	case "object":
		validateJSONObject(path, value, schema, root, errs)
	case "array":
		validateJSONArray(path, value, schema, root, errs)
	case "string":
		validateJSONString(path, value, schema, errs)
	case "integer":
		validateJSONInteger(path, value, schema, errs)
	}
}

func validateJSONObject(path string, value any, schema *jsonSchema, root *jsonSchema, errs *[]string) {
	object, ok := value.(map[string]any)
	if !ok {
		*errs = append(*errs, fmt.Sprintf("%s: expected object", path))
		return
	}

	for _, required := range schema.Required {
		if _, ok := object[required]; !ok {
			*errs = append(*errs, fmt.Sprintf("%s: missing required property %q", path, required))
		}
	}

	for key, child := range schema.Properties {
		if childValue, ok := object[key]; ok {
			validateJSONValue(joinJSONPath(path, key), childValue, child, root, errs)
		}
	}

	if schema.AdditionalProperties != nil && !*schema.AdditionalProperties {
		for key := range object {
			if _, ok := schema.Properties[key]; !ok {
				*errs = append(*errs, fmt.Sprintf("%s: unexpected property %q", path, key))
			}
		}
	}
}

func validateJSONArray(path string, value any, schema *jsonSchema, root *jsonSchema, errs *[]string) {
	array, ok := value.([]any)
	if !ok {
		*errs = append(*errs, fmt.Sprintf("%s: expected array", path))
		return
	}
	if schema.Items == nil {
		return
	}
	for i, item := range array {
		validateJSONValue(fmt.Sprintf("%s[%d]", path, i), item, schema.Items, root, errs)
	}
}

func validateJSONString(path string, value any, schema *jsonSchema, errs *[]string) {
	stringValue, ok := value.(string)
	if !ok {
		*errs = append(*errs, fmt.Sprintf("%s: expected string", path))
		return
	}

	if len(schema.Enum) > 0 && !slices.Contains(schema.Enum, stringValue) {
		*errs = append(*errs, fmt.Sprintf("%s: expected one of %v, got %q", path, schema.Enum, stringValue))
	}
	if schema.Format == "date-time" {
		if _, err := time.Parse(time.RFC3339, stringValue); err != nil {
			*errs = append(*errs, fmt.Sprintf("%s: invalid date-time %q", path, stringValue))
		}
	}
}

func validateJSONInteger(path string, value any, schema *jsonSchema, errs *[]string) {
	integerValue, ok := asInt64(value)
	if !ok {
		*errs = append(*errs, fmt.Sprintf("%s: expected integer", path))
		return
	}

	if schema.Minimum != nil && integerValue < *schema.Minimum {
		*errs = append(*errs, fmt.Sprintf("%s: expected integer >= %d, got %d", path, *schema.Minimum, integerValue))
	}
}

func resolveSchema(schema *jsonSchema, root *jsonSchema) *jsonSchema {
	if schema == nil {
		return nil
	}
	if schema.Ref == "" {
		return schema
	}
	const prefix = "#/$defs/"
	if !strings.HasPrefix(schema.Ref, prefix) {
		return schema
	}
	name := strings.TrimPrefix(schema.Ref, prefix)
	if root == nil || root.Defs == nil {
		return schema
	}
	if resolved, ok := root.Defs[name]; ok {
		return resolved
	}
	return schema
}

func matchesConst(value any, want any) bool {
	gotInt, gotIsInt := asInt64(value)
	wantInt, wantIsInt := asInt64(want)
	if gotIsInt && wantIsInt {
		return gotInt == wantInt
	}
	return value == want
}

func asInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case json.Number:
		i, err := v.Int64()
		return i, err == nil
	case float64:
		i := int64(v)
		if float64(i) != v {
			return 0, false
		}
		return i, true
	case int:
		return int64(v), true
	case int64:
		return v, true
	}
	return 0, false
}

func joinJSONPath(base string, key string) string {
	if base == "" {
		return key
	}
	return base + "." + key
}
