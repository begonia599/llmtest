package converter

import (
	"fmt"
	"strings"
)

// CleanSchemaForGemini recursively cleans a JSON Schema for Gemini compatibility.
func CleanSchemaForGemini(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}
	visited := make(map[string]bool)
	return cleanSchemaRecursive(schema, visited)
}

func cleanSchemaRecursive(schema map[string]any, visited map[string]bool) map[string]any {
	if schema == nil {
		return nil
	}

	// Detect circular references
	id := fmt.Sprintf("%p", schema)
	if visited[id] {
		return schema
	}
	visited[id] = true

	result := make(map[string]any)

	// Copy and transform fields
	for k, v := range schema {
		switch k {
		case "type":
			result["type"] = convertType(v)
		case "properties":
			if props, ok := v.(map[string]any); ok {
				cleaned := make(map[string]any)
				for pk, pv := range props {
					if pm, ok := pv.(map[string]any); ok {
						cleaned[pk] = cleanSchemaRecursive(pm, visited)
					} else {
						cleaned[pk] = pv
					}
				}
				result["properties"] = cleaned
			}
		case "items":
			if items, ok := v.(map[string]any); ok {
				result["items"] = cleanSchemaRecursive(items, visited)
			}
		case "allOf":
			// Merge all schemas in allOf
			if arr, ok := v.([]any); ok {
				merged := mergeAllOf(arr, visited)
				for mk, mv := range merged {
					result[mk] = mv
				}
			}
		case "anyOf":
			// Convert anyOf to enum if all items have const
			if arr, ok := v.([]any); ok {
				if enums := extractEnumFromAnyOf(arr); enums != nil {
					result["enum"] = enums
				}
			}
		case "default":
			// Move default to description
			if desc, ok := result["description"].(string); ok {
				result["description"] = fmt.Sprintf("%s (Default: %v)", desc, v)
			} else {
				result["description"] = fmt.Sprintf("(Default: %v)", v)
			}
		case "required", "description", "enum", "format", "nullable":
			result[k] = v
		case "$defs", "definitions", "$schema", "$id", "const", "oneOf", "strict":
			// Remove unsupported fields
		default:
			result[k] = v
		}
	}

	return result
}

func convertType(v any) string {
	switch t := v.(type) {
	case string:
		return typeToGemini(t)
	case []any:
		// Extract non-null type from type array
		for _, item := range t {
			if s, ok := item.(string); ok && s != "null" {
				return typeToGemini(s)
			}
		}
		return "STRING"
	default:
		return "STRING"
	}
}

func typeToGemini(t string) string {
	switch strings.ToLower(t) {
	case "string":
		return "STRING"
	case "number":
		return "NUMBER"
	case "integer":
		return "INTEGER"
	case "boolean":
		return "BOOLEAN"
	case "array":
		return "ARRAY"
	case "object":
		return "OBJECT"
	default:
		return "STRING"
	}
}

func mergeAllOf(arr []any, visited map[string]bool) map[string]any {
	merged := make(map[string]any)
	mergedProps := make(map[string]any)
	var mergedRequired []any

	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			cleaned := cleanSchemaRecursive(m, visited)
			for k, v := range cleaned {
				switch k {
				case "properties":
					if props, ok := v.(map[string]any); ok {
						for pk, pv := range props {
							mergedProps[pk] = pv
						}
					}
				case "required":
					if req, ok := v.([]any); ok {
						mergedRequired = append(mergedRequired, req...)
					}
				default:
					merged[k] = v
				}
			}
		}
	}

	if len(mergedProps) > 0 {
		merged["properties"] = mergedProps
	}
	if len(mergedRequired) > 0 {
		merged["required"] = mergedRequired
	}

	return merged
}

func extractEnumFromAnyOf(arr []any) []any {
	var enums []any
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			if c, ok := m["const"]; ok {
				enums = append(enums, c)
			} else {
				return nil // Not all items have const
			}
		}
	}
	return enums
}
