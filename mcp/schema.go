package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var errInvalidHeaderDefinition = errors.New("invalid MCP tool header definition")

type headerBinding struct {
	name      string
	valueType string
	path      []string
}

func validateToolDefinition(definition *ToolDefinition) error {
	if !validToolName(definition.Name) {
		return fmt.Errorf("name must be 1-128 MCP name characters")
	}
	if strings.TrimSpace(definition.Description) == "" {
		return fmt.Errorf("description is required")
	}
	if len(definition.InputSchema) == 0 || !json.Valid(definition.InputSchema) {
		return fmt.Errorf("inputSchema must be valid JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(definition.InputSchema))
	decoder.UseNumber()
	var schema map[string]any
	if err := decoder.Decode(&schema); err != nil || schema == nil {
		return fmt.Errorf("inputSchema must be a JSON object")
	}
	bindings, err := scanHeaderBindings(schema, nil, true, make(map[string]struct{}))
	if err != nil {
		return err
	}
	definition.headers = bindings
	definition.InputSchema = append(json.RawMessage(nil), definition.InputSchema...)
	return nil
}

func validToolName(name string) bool {
	if len(name) < 1 || len(name) > 128 {
		return false
	}
	for _, character := range name {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '_' || character == '-' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func scanHeaderBindings(node any, path []string, reachable bool, names map[string]struct{}) ([]headerBinding, error) {
	object, isObject := node.(map[string]any)
	if !isObject {
		if list, ok := node.([]any); ok {
			for _, item := range list {
				if _, err := scanHeaderBindings(item, path, false, names); err != nil {
					return nil, err
				}
			}
		}
		return nil, nil
	}

	var bindings []headerBinding
	if rawName, exists := object["x-mcp-header"]; exists {
		name, ok := rawName.(string)
		if !reachable || len(path) == 0 || !ok || !validHeaderToken(name) {
			return nil, fmt.Errorf("%w: annotation at %q", errInvalidHeaderDefinition, strings.Join(path, "."))
		}
		valueType, ok := object["type"].(string)
		if !ok || (valueType != "string" && valueType != "integer" && valueType != "boolean") {
			return nil, fmt.Errorf("%w: x-mcp-header %q must annotate string, integer, or boolean", errInvalidHeaderDefinition, name)
		}
		folded := strings.ToLower(name)
		if _, exists := names[folded]; exists {
			return nil, fmt.Errorf("%w: duplicate name %q", errInvalidHeaderDefinition, name)
		}
		names[folded] = struct{}{}
		bindings = append(bindings, headerBinding{name: name, valueType: valueType, path: append([]string(nil), path...)})
	}

	for key, value := range object {
		if key == "x-mcp-header" {
			continue
		}
		if key == "properties" && reachable {
			properties, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("properties at %q must be an object", strings.Join(path, "."))
			}
			for property, schema := range properties {
				found, err := scanHeaderBindings(schema, appendPath(path, property), true, names)
				if err != nil {
					return nil, err
				}
				bindings = append(bindings, found...)
			}
			continue
		}
		found, err := scanHeaderBindings(value, path, false, names)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, found...)
	}
	return bindings, nil
}

func appendPath(path []string, segment string) []string {
	result := make([]string, len(path)+1)
	copy(result, path)
	result[len(path)] = segment
	return result
}

func validHeaderToken(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || strings.ContainsRune("!#$%&'*+-.^_`|~", character) {
			continue
		}
		return false
	}
	return true
}

func (definition ToolDefinition) callHeaders(arguments json.RawMessage) (http.Header, error) {
	decoder := json.NewDecoder(bytes.NewReader(arguments))
	decoder.UseNumber()
	var values map[string]any
	if err := decoder.Decode(&values); err != nil {
		return nil, fmt.Errorf("decode arguments: %w", err)
	}
	headers := make(http.Header, len(definition.headers))
	for _, binding := range definition.headers {
		value, exists := nestedValue(values, binding.path)
		if !exists {
			continue
		}
		encoded, err := bindingValue(binding.valueType, value)
		if err != nil {
			return nil, fmt.Errorf("header %q: %w", binding.name, err)
		}
		headers.Set("Mcp-Param-"+binding.name, encodeHeaderValue(encoded))
	}
	return headers, nil
}

func nestedValue(values map[string]any, path []string) (any, bool) {
	var current any = values
	for _, segment := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func bindingValue(valueType string, value any) (string, error) {
	switch valueType {
	case "string":
		text, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("expected string, got %T", value)
		}
		return text, nil
	case "integer":
		number, ok := value.(json.Number)
		if !ok {
			return "", fmt.Errorf("expected integer, got %T", value)
		}
		return safeInteger(number)
	case "boolean":
		boolean, ok := value.(bool)
		if !ok {
			return "", fmt.Errorf("expected boolean, got %T", value)
		}
		if boolean {
			return "true", nil
		}
		return "false", nil
	default:
		return "", fmt.Errorf("unsupported binding type %q", valueType)
	}
}
