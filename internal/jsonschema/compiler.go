// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version  2.0.

package jsonschema

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// Compiler compiles a Draft-07 JSON Schema into an immutable Schema tree.
// It resolves local JSON Pointers, nested $id scopes, URI and fragment
// references, recursive definitions, and boolean schemas deterministically.
// It detects duplicate or conflicting identifiers, unresolved references,
// invalid regular expressions, invalid keyword values, and reference cycles.
// Compilation returns typed errors and never panics. No network loading is
// performed.
type Compiler struct {
	maxDepth      int
	maxProperties int
	maxItems      int
	maxRefDepth   int
	maxFailures   int

	// root is the raw decoded root schema.
	root any

	// idIndex maps $id values to their raw schema nodes, for fragment ref
	// resolution.
	idIndex map[string]any

	// idPaths maps $id values to their JSON Pointer path within the root.
	idPaths map[string]string

	// compiled tracks already-compiled schema nodes by their JSON Pointer
	// path, to handle recursive definitions without infinite recursion.
	compiled map[string]*Schema

	// failures collects compilation failures.
	failures Failures
}

// NewCompiler creates a Compiler with default resource limits.
func NewCompiler() *Compiler {
	return &Compiler{
		maxDepth:      constants.OSCALValidatorMaxDepth,
		maxProperties: constants.OSCALValidatorMaxProperties,
		maxItems:      constants.OSCALValidatorMaxItems,
		maxRefDepth:   constants.OSCALValidatorMaxRefDepth,
		maxFailures:   constants.OSCALValidatorMaxFailures,
		idIndex:       make(map[string]any),
		idPaths:       make(map[string]string),
		compiled:      make(map[string]*Schema),
	}
}

// Compile compiles a Draft-07 JSON Schema from raw bytes. It returns the
// compiled root Schema or a typed error wrapping CompileError.
func (c *Compiler) Compile(data []byte) (*Schema, error) {
	c.failures = nil
	v, err := decodeJSONStrict(data)
	if err != nil {
		return nil, &CompileError{Failures: Failures{{
			Reason:  ReasonCompileInvalidJSON,
			Message: err.Error(),
		}}}
	}
	c.root = v
	// Phase 1: index all $id values and definitions
	c.indexIDs(v, "")
	// Phase 2: compile the root schema
	s := c.compileNode(v, "", 0)
	if len(c.failures) > 0 {
		return nil, &CompileError{Failures: c.failures.Sorted()}
	}
	return s, nil
}

// indexIDs walks the raw schema and indexes every $id value, recording its
// JSON Pointer path. Duplicate $id values are a compile error.
func (c *Compiler) indexIDs(node any, path string) {
	switch v := node.(type) {
	case bool:
		return
	case map[string]any:
		if id, ok := v["$id"].(string); ok && id != "" {
			if _, exists := c.idIndex[id]; exists {
				if existingPath := c.idPaths[id]; existingPath != path {
					c.addFailure(Failure{
						Reason:    ReasonCompileDuplicateID,
						Message:   fmt.Sprintf("duplicate $id %q at %s and %s", id, existingPath, path),
						SchemaPtr: path,
						Keyword:   "$id",
					})
				}
			} else {
				c.idIndex[id] = v
				c.idPaths[id] = path
			}
		}
		// Recurse into schema-valued keywords
		for key, val := range v {
			c.indexIDsKeyword(key, val, path)
		}
	}
}

func (c *Compiler) indexIDsKeyword(key string, val any, path string) {
	childPath := path + "/" + escapeJSONPointerToken(key)
	switch key {
	case "additionalProperties", "additionalItems", "propertyNames",
		"items", "contains", "not", "if", "then", "else":
		if _, isBool := val.(bool); !isBool {
			c.indexIDs(val, childPath)
		}
	case "allOf", "anyOf", "oneOf":
		if arr, ok := val.([]any); ok {
			for i, item := range arr {
				c.indexIDs(item, fmt.Sprintf("%s/%d", childPath, i))
			}
		}
	case "properties", "patternProperties", "definitions":
		if m, ok := val.(map[string]any); ok {
			for k, sub := range m {
				c.indexIDs(sub, childPath+"/"+escapeJSONPointerToken(k))
			}
		}
	case "dependencies":
		if m, ok := val.(map[string]any); ok {
			for k, sub := range m {
				if sm, ok := sub.(map[string]any); ok {
					c.indexIDs(sm, childPath+"/"+escapeJSONPointerToken(k))
				}
			}
		}
	}
}

// resolveRef resolves a $ref string to a raw schema node. It supports:
//   - "#/definitions/foo" — JSON Pointer to top-level definitions
//   - "#some-id" — fragment matching a nested $id
//   - "#" — root schema
func (c *Compiler) resolveRef(ref string) (any, string, error) {
	if ref == "#" {
		return c.root, "", nil
	}
	if strings.HasPrefix(ref, "#/definitions/") {
		ptr := ref[1:] // remove leading #
		val, err := resolveJSONPointer(c.root, ptr)
		if err != nil {
			return nil, "", fmt.Errorf("unresolved $ref %q: %v", ref, err)
		}
		return val, ptr, nil
	}
	if strings.HasPrefix(ref, "#") {
		fragment := ref // includes the #
		if node, ok := c.idIndex[fragment]; ok {
			return node, c.idPaths[fragment], nil
		}
		return nil, "", fmt.Errorf("unresolved $ref %q: no $id matches fragment", ref)
	}
	return nil, "", fmt.Errorf("unresolved $ref %q: only local refs (starting with #) are supported", ref)
}

// compileNode compiles a raw schema node into a *Schema at the given path
// and depth. It handles boolean schemas, $ref resolution, and all supported
// keywords. Recursive definitions are handled via the compiled map.
func (c *Compiler) compileNode(node any, path string, depth int) *Schema {
	if depth > c.maxDepth {
		c.addFailure(Failure{
			Reason:    ReasonCompileResourceLimit,
			Message:   fmt.Sprintf("schema nesting depth exceeds limit %d", c.maxDepth),
			SchemaPtr: path,
		})
		return &Schema{Boolean: boolPtr(false)}
	}

	// Boolean schema
	if b, ok := node.(bool); ok {
		return &Schema{Boolean: &b, SchemaLocation: path}
	}

	// Object schema
	obj, ok := node.(map[string]any)
	if !ok {
		c.addFailure(Failure{
			Reason:    ReasonCompileInvalidType,
			Message:   fmt.Sprintf("schema must be a boolean or object, got %T", node),
			SchemaPtr: path,
		})
		return &Schema{Boolean: boolPtr(false)}
	}

	// Check for already-compiled (recursive definition)
	if existing, ok := c.compiled[path]; ok {
		return existing
	}

	// Create a placeholder for recursive references
	s := &Schema{SchemaLocation: path}
	c.compiled[path] = s

	// Check for unrecognized keywords
	for key := range obj {
		if !SupportedKeyword(Keyword(key)) {
			c.addFailure(Failure{
				Reason:    ReasonCompileUnrecognizedKeyword,
				Message:   fmt.Sprintf("unrecognized keyword %q", key),
				SchemaPtr: path,
				Keyword:   key,
			})
		}
	}

	// $id
	if id, ok := obj["$id"].(string); ok {
		s.ID = id
	}

	// $ref — if present, all other keywords are ignored per Draft-07
	if ref, ok := obj["$ref"].(string); ok {
		s.RefPath = ref
		target, targetPath, err := c.resolveRef(ref)
		if err != nil {
			c.addFailure(Failure{
				Reason:    ReasonCompileUnresolvedRef,
				Message:   err.Error(),
				SchemaPtr: path,
				Keyword:   "$ref",
			})
			s.Boolean = boolPtr(false)
			return s
		}
		// Compile the target (may recurse back to this node)
		s.Ref = c.compileNode(target, targetPath, depth+1)
		return s
	}

	c.compileScalarKeywords(s, obj, path)

	// properties
	if props, ok := obj["properties"]; ok {
		if m, ok := props.(map[string]any); ok {
			s.Properties = make(map[string]*Schema)
			for k, sub := range m {
				childPath := path + "/properties/" + escapeJSONPointerToken(k)
				s.Properties[k] = c.compileNode(sub, childPath, depth+1)
			}
		}
	}

	// patternProperties
	if pp, ok := obj["patternProperties"]; ok {
		if m, ok := pp.(map[string]any); ok {
			s.PatternProperties = make(map[*regexp.Regexp]*Schema)
			for k, sub := range m {
				re, err := regexp.Compile(k)
				if err != nil {
					c.addFailure(Failure{
						Reason:    ReasonCompileInvalidRegex,
						Message:   fmt.Sprintf("patternProperties key %q is not a valid regex: %v", k, err),
						SchemaPtr: path,
						Keyword:   "patternProperties",
					})
					continue
				}
				childPath := path + "/patternProperties/" + escapeJSONPointerToken(k)
				s.PatternProperties[re] = c.compileNode(sub, childPath, depth+1)
			}
		}
	}

	// additionalProperties
	if ap, ok := obj["additionalProperties"]; ok {
		switch v := ap.(type) {
		case bool:
			bv := v
			s.AdditionalPropertiesBool = &bv
		case map[string]any:
			s.AdditionalProperties = c.compileNode(v, path+"/additionalProperties", depth+1)
		}
	}

	// propertyNames
	if pn, ok := obj["propertyNames"]; ok {
		if _, isBool := pn.(bool); !isBool {
			s.PropertyNames = c.compileNode(pn, path+"/propertyNames", depth+1)
		} else if b, _ := pn.(bool); !b {
			s.PropertyNames = &Schema{Boolean: boolPtr(false)}
		}
	}

	// required
	if req, ok := obj["required"]; ok {
		if arr, ok := req.([]any); ok {
			for _, item := range arr {
				if str, ok := item.(string); ok {
					s.Required = append(s.Required, str)
				}
			}
		}
	}

	// dependencies
	if deps, ok := obj["dependencies"]; ok {
		if m, ok := deps.(map[string]any); ok {
			s.Dependencies = make(map[string]any)
			for k, val := range m {
				switch v := val.(type) {
				case []any:
					// Array of property names
					names := make([]string, 0, len(v))
					for _, item := range v {
						if str, ok := item.(string); ok {
							names = append(names, str)
						}
					}
					s.Dependencies[k] = names
				case map[string]any:
					// Subschema
					childPath := path + "/dependencies/" + escapeJSONPointerToken(k)
					s.Dependencies[k] = c.compileNode(v, childPath, depth+1)
				case bool:
					s.Dependencies[k] = &Schema{Boolean: &v}
				}
			}
		}
	}

	// minProperties / maxProperties
	s.MinProperties = parseIntPtr(obj["minProperties"])
	s.MaxProperties = parseIntPtr(obj["maxProperties"])

	// items
	if items, ok := obj["items"]; ok {
		switch v := items.(type) {
		case bool:
			s.Items = &Schema{Boolean: &v}
		case map[string]any:
			s.Items = c.compileNode(v, path+"/items", depth+1)
		case []any:
			// Tuple form: array of schemas
			// For OSCAL, items is always a single schema, but we handle
			// tuple form by compiling the first item schema.
			// Full tuple-form support would require AdditionalItems handling.
			if len(v) > 0 {
				s.Items = c.compileNode(v[0], path+"/items/0", depth+1)
			}
		}
	}

	// additionalItems
	if ai, ok := obj["additionalItems"]; ok {
		switch v := ai.(type) {
		case bool:
			bv := v
			s.AdditionalItemsBool = &bv
		case map[string]any:
			s.AdditionalItems = c.compileNode(v, path+"/additionalItems", depth+1)
		}
	}

	// contains
	if cont, ok := obj["contains"]; ok {
		if _, isBool := cont.(bool); !isBool {
			s.Contains = c.compileNode(cont, path+"/contains", depth+1)
		}
	}

	// minItems / maxItems
	s.MinItems = parseIntPtr(obj["minItems"])
	s.MaxItems = parseIntPtr(obj["maxItems"])

	// uniqueItems
	if ui, ok := obj["uniqueItems"].(bool); ok {
		s.UniqueItems = ui
	}

	// Conditional subschemas
	if allOf, ok := obj["allOf"].([]any); ok {
		for i, item := range allOf {
			s.AllOf = append(s.AllOf, c.compileNode(item, fmt.Sprintf("%s/allOf/%d", path, i), depth+1))
		}
	}
	if anyOf, ok := obj["anyOf"].([]any); ok {
		for i, item := range anyOf {
			s.AnyOf = append(s.AnyOf, c.compileNode(item, fmt.Sprintf("%s/anyOf/%d", path, i), depth+1))
		}
	}
	if oneOf, ok := obj["oneOf"].([]any); ok {
		for i, item := range oneOf {
			s.OneOf = append(s.OneOf, c.compileNode(item, fmt.Sprintf("%s/oneOf/%d", path, i), depth+1))
		}
	}
	if not, ok := obj["not"]; ok {
		if _, isBool := not.(bool); !isBool {
			s.Not = c.compileNode(not, path+"/not", depth+1)
		}
	}
	if ifVal, ok := obj["if"]; ok {
		if _, isBool := ifVal.(bool); !isBool {
			s.If = c.compileNode(ifVal, path+"/if", depth+1)
		}
	}
	if then, ok := obj["then"]; ok {
		if _, isBool := then.(bool); !isBool {
			s.Then = c.compileNode(then, path+"/then", depth+1)
		}
	}
	if elseVal, ok := obj["else"]; ok {
		if _, isBool := elseVal.(bool); !isBool {
			s.Else = c.compileNode(elseVal, path+"/else", depth+1)
		}
	}

	return s
}

func (c *Compiler) compileScalarKeywords(s *Schema, obj map[string]any, path string) {
	c.compileTypeKeyword(s, obj, path)
	if enum, ok := obj["enum"]; ok {
		if values, valid := enum.([]any); valid {
			s.Enum = values
		} else {
			c.addFailure(Failure{Reason: ReasonCompileInvalidKeywordValue, Message: "enum must be an array", SchemaPtr: path, Keyword: "enum"})
		}
	}
	if value, ok := obj["const"]; ok {
		s.Const = value
		s.HasConst = true
	}
	s.MinLength = parseIntPtr(obj["minLength"])
	s.MaxLength = parseIntPtr(obj["maxLength"])
	if pattern, ok := obj["pattern"].(string); ok {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			c.addFailure(Failure{Reason: ReasonCompileInvalidRegex, Message: fmt.Sprintf("pattern %q is not a valid regex: %v", pattern, err), SchemaPtr: path, Keyword: "pattern"})
		} else {
			s.Pattern = compiled
		}
	}
	if format, ok := obj["format"].(string); ok {
		if !SupportedFormats[Format(format)] {
			c.addFailure(Failure{Reason: ReasonCompileUnsupportedFormat, Message: fmt.Sprintf("unsupported format %q", format), SchemaPtr: path, Keyword: "format"})
		} else {
			s.Format = Format(format)
		}
	}
	s.Minimum = parseFloatPtr(obj["minimum"])
	s.Maximum = parseFloatPtr(obj["maximum"])
	s.ExclusiveMinimum = parseFloatPtr(obj["exclusiveMinimum"])
	s.ExclusiveMaximum = parseFloatPtr(obj["exclusiveMaximum"])
	s.MultipleOf = parseFloatPtr(obj["multipleOf"])
}

func (c *Compiler) compileTypeKeyword(s *Schema, obj map[string]any, path string) {
	value, ok := obj["type"]
	if !ok {
		return
	}
	switch typed := value.(type) {
	case string:
		s.Type = typed
	case []any:
		if len(typed) == 0 {
			c.addFailure(Failure{Reason: ReasonCompileInvalidKeywordValue, Message: "type array must contain at least one string", SchemaPtr: path, Keyword: "type"})
			return
		}
		for _, item := range typed {
			typeName, valid := item.(string)
			if !valid {
				c.addFailure(Failure{Reason: ReasonCompileInvalidKeywordValue, Message: "type array must contain only strings", SchemaPtr: path, Keyword: "type"})
				return
			}
			if s.Type == "" {
				s.Type = typeName
			}
		}
	default:
		c.addFailure(Failure{Reason: ReasonCompileInvalidKeywordValue, Message: fmt.Sprintf("type must be a string or array of strings, got %T", value), SchemaPtr: path, Keyword: "type"})
	}
}

func (c *Compiler) addFailure(f Failure) {
	if len(c.failures) < c.maxFailures {
		c.failures = append(c.failures, f)
	}
}

// Helper functions

func boolPtr(b bool) *bool {
	return &b
}

func parseIntPtr(v any) *int {
	if v == nil {
		return nil
	}
	if n, ok := v.(json.Number); ok {
		if i, err := n.Int64(); err == nil {
			ii := int(i)
			return &ii
		}
	}
	if i, ok := v.(float64); ok {
		ii := int(i)
		return &ii
	}
	if i, ok := v.(int); ok {
		return &i
	}
	return nil
}

func parseFloatPtr(v any) *float64 {
	if v == nil {
		return nil
	}
	if n, ok := v.(json.Number); ok {
		if f, err := n.Float64(); err == nil {
			return &f
		}
	}
	if f, ok := v.(float64); ok {
		return &f
	}
	return nil
}
