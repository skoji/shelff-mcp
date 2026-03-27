package shelff

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
)

//go:generate cp ../shelff-schema/sidecar.schema.json schema/sidecar.schema.json

//go:embed schema/sidecar.schema.json
var embeddedSidecarSchema []byte

var loadSidecarSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	var schema jsonschema.Schema
	if err := json.Unmarshal(embeddedSidecarSchema, &schema); err != nil {
		return nil, err
	}
	if _, err := schema.Resolve(nil); err != nil {
		return nil, err
	}
	return &schema, nil
})

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
	value = normalizeJSONValue(value).(map[string]any)

	schema, err := loadSidecarSchema()
	if err != nil {
		return nil, err
	}

	var validationErrors []string
	seen := make(map[string]struct{})
	collectValidationErrors("$", value, schema, schema, &validationErrors, seen)
	return validationErrors, nil
}

func collectValidationErrors(path string, value any, schema *jsonschema.Schema, root *jsonschema.Schema, errs *[]string, seen map[string]struct{}) {
	schema = resolveSchema(schema, root)
	if schema == nil {
		return
	}

	if err := validateLocally(value, schema); err != nil {
		appendValidationError(errs, seen, fmt.Sprintf("%s: %v", path, err))
	}

	if schema.Format == "date-time" {
		if stringValue, ok := value.(string); ok {
			if _, err := time.Parse(time.RFC3339, stringValue); err != nil {
				appendValidationError(errs, seen, fmt.Sprintf("%s: invalid date-time %q", path, stringValue))
			}
		}
	}

	if isObjectSchema(schema) {
		object, ok := value.(map[string]any)
		if !ok {
			return
		}

		propertyNames := mapsKeys(schema.Properties)
		slices.Sort(propertyNames)
		for _, key := range propertyNames {
			childValue, ok := object[key]
			if !ok {
				continue
			}
			collectValidationErrors(joinJSONPath(path, key), childValue, schema.Properties[key], root, errs, seen)
		}
		return
	}

	if isArraySchema(schema) {
		array, ok := value.([]any)
		if !ok {
			return
		}
		if schema.Items == nil {
			return
		}
		for i, item := range array {
			collectValidationErrors(fmt.Sprintf("%s[%d]", path, i), item, schema.Items, root, errs, seen)
		}
	}
}

func validateLocally(value any, schema *jsonschema.Schema) error {
	localSchema := localValidationSchema(schema)
	resolved, err := localSchema.Resolve(nil)
	if err != nil {
		return err
	}
	return resolved.Validate(value)
}

func localValidationSchema(schema *jsonschema.Schema) *jsonschema.Schema {
	local := &jsonschema.Schema{
		Type:             schema.Type,
		Types:            slices.Clone(schema.Types),
		Enum:             slices.Clone(schema.Enum),
		Const:            schema.Const,
		MultipleOf:       schema.MultipleOf,
		Minimum:          schema.Minimum,
		Maximum:          schema.Maximum,
		ExclusiveMinimum: schema.ExclusiveMinimum,
		ExclusiveMaximum: schema.ExclusiveMaximum,
		MinLength:        schema.MinLength,
		MaxLength:        schema.MaxLength,
		Pattern:          schema.Pattern,
		MinItems:         schema.MinItems,
		MaxItems:         schema.MaxItems,
		UniqueItems:      schema.UniqueItems,
		MinProperties:    schema.MinProperties,
		MaxProperties:    schema.MaxProperties,
		Required:         slices.Clone(schema.Required),
	}

	if isObjectSchema(schema) {
		local.Properties = placeholderProperties(schema.Properties)
		local.AdditionalProperties = schema.AdditionalProperties
	}

	return local
}

func placeholderProperties(properties map[string]*jsonschema.Schema) map[string]*jsonschema.Schema {
	if len(properties) == 0 {
		return nil
	}

	result := make(map[string]*jsonschema.Schema, len(properties))
	for key := range properties {
		result[key] = &jsonschema.Schema{}
	}
	return result
}

func resolveSchema(schema *jsonschema.Schema, root *jsonschema.Schema) *jsonschema.Schema {
	if schema == nil {
		return nil
	}
	if schema.Ref == "" {
		return schema
	}

	const defsPrefix = "#/$defs/"
	if strings.HasPrefix(schema.Ref, defsPrefix) && root != nil {
		name := strings.TrimPrefix(schema.Ref, defsPrefix)
		if resolved, ok := root.Defs[name]; ok {
			return resolved
		}
	}

	return schema
}

func appendValidationError(errs *[]string, seen map[string]struct{}, err string) {
	if _, ok := seen[err]; ok {
		return
	}
	seen[err] = struct{}{}
	*errs = append(*errs, err)
}

func isObjectSchema(schema *jsonschema.Schema) bool {
	return schema.Type == "object" || slices.Contains(schema.Types, "object")
}

func isArraySchema(schema *jsonschema.Schema) bool {
	return schema.Type == "array" || slices.Contains(schema.Types, "array")
}

func mapsKeys[V any](m map[string]V) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

func normalizeJSONValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, child := range v {
			result[key] = normalizeJSONValue(child)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, child := range v {
			result[i] = normalizeJSONValue(child)
		}
		return result
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
		}
		if f, err := v.Float64(); err == nil {
			return f
		}
		return v.String()
	default:
		return value
	}
}

func joinJSONPath(base string, key string) string {
	if base == "" {
		return key
	}
	return base + "." + key
}
