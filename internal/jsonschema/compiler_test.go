// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version  2.0.

package jsonschema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	oscalschema "github.com/g8e-ai/g8e/v2/protocol/schemas/oscal"
)

func TestCompiler_CompilesOSCALSchema(t *testing.T) {
	schemaBytes, err := oscalschema.SchemaBytes()
	require.NoError(t, err)
	c := NewCompiler()
	s, err := c.Compile(schemaBytes)
	require.NoError(t, err, "compiling the official OSCAL 1.1.2 schema must succeed")
	assert.NotNil(t, s)
	assert.Equal(t, "object", s.Type)
	assert.NotNil(t, s.Properties)
}

func TestCompiler_CompilesBooleanTrueSchema(t *testing.T) {
	c := NewCompiler()
	s, err := c.Compile([]byte(`true`))
	require.NoError(t, err)
	assert.True(t, s.AcceptsAll())
}

func TestCompiler_CompilesBooleanFalseSchema(t *testing.T) {
	c := NewCompiler()
	s, err := c.Compile([]byte(`false`))
	require.NoError(t, err)
	assert.True(t, s.RejectsAll())
}

func TestCompiler_RejectsInvalidJSON(t *testing.T) {
	c := NewCompiler()
	_, err := c.Compile([]byte(`{invalid`))
	require.Error(t, err)
	ce, ok := err.(*CompileError)
	require.True(t, ok)
	assert.True(t, ce.Failures.Has(ReasonCompileInvalidJSON))
}

func TestCompiler_RejectsDuplicateObjectKeys(t *testing.T) {
	compiler := NewCompiler()
	_, err := compiler.Compile([]byte(`{"type":"object","type":"array"}`))
	require.Error(t, err)
	compileErr, ok := err.(*CompileError)
	require.True(t, ok)
	assert.True(t, compileErr.Failures.Has(ReasonCompileDuplicateKey))
}

func TestCompiler_RejectsTrailingData(t *testing.T) {
	c := NewCompiler()
	_, err := c.Compile([]byte(`{}` + "\n" + `{}`))
	require.Error(t, err)
}

func TestCompiler_RejectsUnresolvedRef(t *testing.T) {
	c := NewCompiler()
	_, err := c.Compile([]byte(`{"$ref": "#/definitions/NonExistent"}`))
	require.Error(t, err)
	ce, ok := err.(*CompileError)
	require.True(t, ok)
	assert.True(t, ce.Failures.Has(ReasonCompileUnresolvedRef))
}

func TestCompiler_RejectsDuplicateID(t *testing.T) {
	c := NewCompiler()
	_, err := c.Compile([]byte(`{
		"definitions": {
			"a": {"$id": "#dup", "type": "string"},
			"b": {"$id": "#dup", "type": "number"}
		}
	}`))
	require.Error(t, err)
	ce, ok := err.(*CompileError)
	require.True(t, ok)
	assert.True(t, ce.Failures.Has(ReasonCompileDuplicateID))
}

func TestCompiler_RejectsInvalidRegex(t *testing.T) {
	c := NewCompiler()
	_, err := c.Compile([]byte(`{"pattern": "[invalid"}`))
	require.Error(t, err)
	ce, ok := err.(*CompileError)
	require.True(t, ok)
	assert.True(t, ce.Failures.Has(ReasonCompileInvalidRegex))
}

func TestCompiler_RejectsUnsupportedFormat(t *testing.T) {
	c := NewCompiler()
	_, err := c.Compile([]byte(`{"type": "string", "format": "unknown-format"}`))
	require.Error(t, err)
	ce, ok := err.(*CompileError)
	require.True(t, ok)
	assert.True(t, ce.Failures.Has(ReasonCompileUnsupportedFormat))
}

func TestCompiler_ResolvesFragmentRef(t *testing.T) {
	c := NewCompiler()
	s, err := c.Compile([]byte(`{
		"$id": "http://example.com/root.json",
		"definitions": {
			"foo": {
				"$id": "#foo",
				"type": "string"
			}
		},
		"properties": {
			"x": {"$ref": "#foo"}
		}
	}`))
	require.NoError(t, err)
	propX := s.Properties["x"].Resolved()
	require.NotNil(t, propX)
	assert.Equal(t, "string", propX.Type)
}

func TestCompiler_ResolvesJSONPointerRef(t *testing.T) {
	c := NewCompiler()
	s, err := c.Compile([]byte(`{
		"definitions": {
			"foo": {"type": "integer"}
		},
		"properties": {
			"x": {"$ref": "#/definitions/foo"}
		}
	}`))
	require.NoError(t, err)
	propX := s.Properties["x"].Resolved()
	require.NotNil(t, propX)
	assert.Equal(t, "integer", propX.Type)
}

func TestCompiler_HandlesRecursiveDefinition(t *testing.T) {
	c := NewCompiler()
	s, err := c.Compile([]byte(`{
		"definitions": {
			"node": {
				"$id": "#node",
				"type": "object",
				"properties": {
					"children": {
						"type": "array",
						"items": {"$ref": "#node"}
					}
				}
			}
		},
		"properties": {
			"root": {"$ref": "#node"}
		}
	}`))
	require.NoError(t, err)
	rootSchema := s.Properties["root"].Resolved()
	require.NotNil(t, rootSchema)
	assert.Equal(t, "object", rootSchema.Type)
	// The recursive ref should resolve without infinite loop
	childrenItems := rootSchema.Properties["children"].Items.Resolved()
	require.NotNil(t, childrenItems)
	assert.Equal(t, "object", childrenItems.Type)
}

func TestCompiler_RefOverridesAllOtherKeywords(t *testing.T) {
	c := NewCompiler()
	s, err := c.Compile([]byte(`{
		"definitions": {
			"foo": {"type": "string"}
		},
		"properties": {
			"x": {"$ref": "#/definitions/foo", "type": "integer"}
		}
	}`))
	require.NoError(t, err)
	// Per Draft-07, $ref overrides all other keywords
	propX := s.Properties["x"]
	assert.True(t, propX.IsRef())
	resolved := propX.Resolved()
	assert.Equal(t, "string", resolved.Type)
}

func TestCompiler_NeverPanicsOnMalformedInput(t *testing.T) {
	// Various malformed inputs should return errors, not panic
	inputs := []string{
		``,
		`null`,
		`[]`,
		`{"type": 123}`,
		`{"enum": "not-an-array"}`,
		`{"properties": "not-an-object"}`,
		`{"required": "not-an-array"}`,
		`{"allOf": "not-an-array"}`,
		`{"$ref": 123}`,
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			c := NewCompiler()
			// Should not panic
			_, _ = c.Compile([]byte(input))
		})
	}
}

func TestCompiler_DoesNotPerformNetworkLoading(t *testing.T) {
	// An absolute URI ref must fail (no network loading)
	c := NewCompiler()
	_, err := c.Compile([]byte(`{"$ref": "http://example.com/schema.json"}`))
	require.Error(t, err)
	ce, ok := err.(*CompileError)
	require.True(t, ok)
	assert.True(t, ce.Failures.Has(ReasonCompileUnresolvedRef))
}
