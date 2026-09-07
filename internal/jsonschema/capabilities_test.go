// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version  2.0.

package jsonschema

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	oscalschema "github.com/g8e-ai/g8e/v2/protocol/schemas/oscal"
)

// schemaWalker traverses a JSON Schema document structure, counting only
// actual JSON Schema keywords (not property names inside `properties` maps)
// and properly recursing into subschemas.
type schemaWalker struct {
	keywordCounts   map[string]int
	formatCounts    map[string]int
	refCounts       map[string]int
	patternCount    int
	totalRefs       int
	boolSchemaCount int
	maxDepth        int
}

func newSchemaWalker() *schemaWalker {
	return &schemaWalker{
		keywordCounts: make(map[string]int),
		formatCounts:  make(map[string]int),
		refCounts:     make(map[string]int),
	}
}

// walkSchema traverses a JSON Schema node, counting keywords and recursing
// into subschemas. It distinguishes between schema-valued keywords (whose
// values are schemas to recurse into) and non-schema-valued keywords (whose
// values are strings, arrays of strings, or arrays of values).
func (w *schemaWalker) walkSchema(node any, depth int) {
	if depth > w.maxDepth {
		w.maxDepth = depth
	}
	switch v := node.(type) {
	case bool:
		// Boolean schema (true = accept all, false = reject all)
		// Only count when it appears as a schema-valued keyword value
		return
	case map[string]any:
		for key, val := range v {
			w.keywordCounts[key]++
			w.walkKeywordValue(key, val, depth+1)
		}
	}
}

// walkKeywordValue dispatches based on the keyword and walks the value
// appropriately. Schema-valued keywords recurse into the value as a schema.
// Array-of-schema keywords recurse into each element. Map-of-schema keywords
// recurse into each value. Non-schema keywords do not recurse.
func (w *schemaWalker) walkKeywordValue(key string, val any, depth int) {
	switch key {
	// Schema-valued keywords
	case "additionalProperties", "additionalItems", "propertyNames",
		"items", "contains", "not", "if", "then", "else":
		if b, ok := val.(bool); ok {
			if !b {
				w.boolSchemaCount++
			}
		} else {
			w.walkSchema(val, depth)
		}

	// Array-of-schema keywords
	case "allOf", "anyOf", "oneOf":
		if arr, ok := val.([]any); ok {
			for _, item := range arr {
				w.walkSchema(item, depth)
			}
		}

	// Map-of-schema keywords (property name -> schema)
	case "properties", "patternProperties", "definitions":
		if m, ok := val.(map[string]any); ok {
			for _, sub := range m {
				w.walkSchema(sub, depth)
			}
		}

	// Dependencies: can be schema or array of property names
	case "dependencies":
		if m, ok := val.(map[string]any); ok {
			for _, sub := range m {
				switch s := sub.(type) {
				case map[string]any:
					w.walkSchema(s, depth)
				case []any:
					// array of property names, not schemas
				}
			}
		}

	// String-valued keywords with side effects
	case "$ref":
		if s, ok := val.(string); ok {
			w.totalRefs++
			switch {
			case s == "#":
				w.refCounts["root"]++
			case strings.HasPrefix(s, "#/definitions/"):
				w.refCounts["json-pointer"]++
			case strings.HasPrefix(s, "#"):
				w.refCounts["fragment"]++
			default:
				w.refCounts["other"]++
			}
		}

	case "format":
		if s, ok := val.(string); ok {
			w.formatCounts[s]++
		}

	case "pattern":
		w.patternCount++

	// Non-schema-valued keywords: do not recurse
	case "type", "enum", "const", "required", "minItems", "maxItems",
		"minProperties", "maxProperties", "minLength", "maxLength",
		"minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum",
		"multipleOf", "uniqueItems", "$schema", "$id", "$comment",
		"title", "description", "default", "examples", "readOnly",
		"writeOnly":
		// Values are not schemas; do not recurse

	default:
		// Unknown keyword: do not recurse (will be flagged by another test)
	}
}

func scanSchema(t *testing.T) *schemaWalker {
	t.Helper()
	schemaBytes, err := oscalschema.SchemaBytes()
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(schemaBytes, &raw))
	w := newSchemaWalker()
	w.walkSchema(raw, 0)
	return w
}

func TestOSCALProfile_KeywordCountsMatchEmbeddedSchema(t *testing.T) {
	w := scanSchema(t)
	// Only check keywords that are in our profile; the walker counts all
	// keyword occurrences including property names that happen to match
	// keyword names, so we check the ones we track.
	for kw, expected := range OSCALProfile.KeywordCounts {
		got := w.keywordCounts[string(kw)]
		assert.Equal(t, expected, got, "keyword %s count mismatch", kw)
	}
}

func TestOSCALProfile_FormatsMatchEmbeddedSchema(t *testing.T) {
	w := scanSchema(t)
	assert.Len(t, w.formatCounts, len(OSCALProfile.Formats))
	for _, f := range OSCALProfile.Formats {
		assert.Contains(t, w.formatCounts, string(f), "format %s not found in schema", f)
	}
}

func TestOSCALProfile_DefinitionCountMatchesEmbeddedSchema(t *testing.T) {
	schemaBytes, err := oscalschema.SchemaBytes()
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(schemaBytes, &raw))
	defs, ok := raw["definitions"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, OSCALProfile.DefinitionCount, len(defs))
}

func TestOSCALProfile_BooleanSchemaCountMatchesEmbeddedSchema(t *testing.T) {
	w := scanSchema(t)
	assert.Equal(t, OSCALProfile.BooleanSchemaCount, w.boolSchemaCount)
}

func TestOSCALProfile_HasRecursiveDefs(t *testing.T) {
	schemaBytes, err := oscalschema.SchemaBytes()
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(schemaBytes, &raw))
	// Find self-refs: a $ref whose value equals an $id in the same subtree
	foundRecursive := false
	var checkRecursive func(n any, currentID string) bool
	checkRecursive = func(n any, currentID string) bool {
		switch v := n.(type) {
		case map[string]any:
			id := currentID
			if newID, ok := v["$id"].(string); ok {
				id = newID
			}
			if ref, ok := v["$ref"].(string); ok {
				if ref == id && id != "" {
					return true
				}
			}
			for _, val := range v {
				if checkRecursive(val, id) {
					return true
				}
			}
		case []any:
			for _, item := range v {
				if checkRecursive(item, currentID) {
					return true
				}
			}
		}
		return false
	}
	foundRecursive = checkRecursive(raw, "")
	assert.True(t, OSCALProfile.HasRecursiveDefs, "profile claims recursive defs")
	assert.True(t, foundRecursive, "schema must contain at least one recursive definition")
}

func TestOSCALCapabilityInventory_EveryKeywordIsSupported(t *testing.T) {
	for _, cap := range OSCALCapabilityInventory {
		assert.True(t, SupportedKeyword(cap.Keyword),
			"inventory keyword %s is not supported by the compiler", cap.Keyword)
	}
}

func TestOSCALCapabilityInventory_EveryExercisedKeywordAppearsInSchema(t *testing.T) {
	w := scanSchema(t)
	for _, cap := range OSCALCapabilityInventory {
		if cap.ExercisedByOSCAL {
			_, present := w.keywordCounts[string(cap.Keyword)]
			assert.True(t, present,
				"inventory claims keyword %s is exercised by OSCAL but it does not appear in the schema",
				cap.Keyword)
		}
	}
}

func TestOSCALProfile_RefFormsMatchEmbeddedSchema(t *testing.T) {
	w := scanSchema(t)
	for form, expected := range OSCALProfile.RefForms {
		assert.Equal(t, expected, w.refCounts[string(form)],
			"ref form %s count mismatch", form)
	}
	assert.Equal(t, 0, w.refCounts["other"], "no absolute-URI or unknown refs expected")
	assert.Equal(t, 0, w.refCounts["root"], "no root refs expected")
}

func TestOSCALProfile_TotalRefCount(t *testing.T) {
	w := scanSchema(t)
	assert.Equal(t, OSCALProfile.TotalRefCount, w.totalRefs)
}

func TestOSCALProfile_PatternCount(t *testing.T) {
	w := scanSchema(t)
	assert.Equal(t, OSCALProfile.PatternCount, w.patternCount)
}

func TestSupportedFormats_CoversAllOSCALFormats(t *testing.T) {
	for _, f := range OSCALProfile.Formats {
		assert.True(t, SupportedFormats[f], "format %s is not supported", f)
	}
}

func TestOSCALProfile_CanBeMarshaled(t *testing.T) {
	data, err := json.Marshal(OSCALProfile)
	require.NoError(t, err)
	assert.NotEmpty(t, data)
	var roundtrip OSCALSchemaProfile
	require.NoError(t, json.Unmarshal(data, &roundtrip))
	assert.Equal(t, OSCALProfile.SchemaVersion, roundtrip.SchemaVersion)
	assert.Equal(t, OSCALProfile.DefinitionCount, roundtrip.DefinitionCount)
}

func TestAllValidationKeywordsAreSupported(t *testing.T) {
	for kw := range ValidationKeywords {
		assert.True(t, SupportedKeyword(kw), "validation keyword %s not supported", kw)
	}
}

func TestNoUnrecognizedKeywordsInOSCALSchema(t *testing.T) {
	schemaBytes, err := oscalschema.SchemaBytes()
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(schemaBytes, &raw))
	// Walk the schema properly, collecting only keywords that appear in
	// schema positions (not property names inside `properties` maps)
	unrecognized := make(map[string]int)
	var walkSchemaNode func(n any)
	walkSchemaNode = func(n any) {
		switch v := n.(type) {
		case bool:
			return
		case map[string]any:
			for key, val := range v {
				if !SupportedKeyword(Keyword(key)) {
					unrecognized[key]++
				}
				// Recurse into schema-valued keywords only
				switch key {
				case "additionalProperties", "additionalItems", "propertyNames",
					"items", "contains", "not", "if", "then", "else":
					if _, isBool := val.(bool); !isBool {
						walkSchemaNode(val)
					}
				case "allOf", "anyOf", "oneOf":
					if arr, ok := val.([]any); ok {
						for _, item := range arr {
							walkSchemaNode(item)
						}
					}
				case "properties", "patternProperties", "definitions":
					if m, ok := val.(map[string]any); ok {
						for _, sub := range m {
							walkSchemaNode(sub)
						}
					}
				case "dependencies":
					if m, ok := val.(map[string]any); ok {
						for _, sub := range m {
							if sm, ok := sub.(map[string]any); ok {
								walkSchemaNode(sm)
							}
						}
					}
				}
			}
		}
	}
	walkSchemaNode(raw)
	if len(unrecognized) > 0 {
		var keys []string
		for k := range unrecognized {
			keys = append(keys, k)
		}
		t.Fatalf("unrecognized keywords in OSCAL schema: %s", strings.Join(keys, ", "))
	}
}

func TestOSCALProfile_MaxNestingDepth(t *testing.T) {
	w := scanSchema(t)
	// The walker counts schema-node depth (each schema-valued keyword
	// increments depth). The profile value is computed from the same
	// traversal.
	assert.Equal(t, OSCALProfile.MaxNestingDepth, w.maxDepth,
		"schema nesting depth mismatch")
}
