// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version  2.0.

package jsonschema

import (
	"encoding/json"
	"testing"
)

// FuzzSchemaCompilation fuzzes the compiler with random byte sequences.
// The compiler must return an error or a valid schema, never panic.
func FuzzSchemaCompilation(f *testing.F) {
	// Seed with valid and invalid schemas
	f.Add([]byte(`{"type":"object"}`))
	f.Add([]byte(`true`))
	f.Add([]byte(`false`))
	f.Add([]byte(`{"$ref":"#/definitions/foo","definitions":{"foo":{"type":"string"}}}`))
	f.Add([]byte(`{invalid`))
	f.Add([]byte(``))
	f.Add([]byte(`null`))

	f.Fuzz(func(t *testing.T, data []byte) {
		c := NewCompiler()
		s, err := c.Compile(data)
		if err != nil {
			return // compilation failure is expected for fuzz input
		}
		// If compilation succeeded, the schema should be usable
		v := NewValidator()
		_ = v.Validate(s, []byte(`{}`))
	})
}

// FuzzJSONDecoding fuzzes the JSON decoder used by the validator.
func FuzzJSONDecoding(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`"hello"`))
	f.Add([]byte(`42`))
	f.Add([]byte(`true`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{invalid`))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, err := decodeJSONForValidation(data)
		if err != nil {
			return // decode failure is expected for fuzz input
		}
	})
}

// FuzzDocumentValidation fuzzes the validator with random JSON instances
// against a fixed schema.
func FuzzDocumentValidation(f *testing.F) {
	// Compile a representative schema
	c := NewCompiler()
	schema, err := c.Compile([]byte(`{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"value": {"type": "integer", "minimum": 0},
			"items": {
				"type": "array",
				"items": {"type": "string"}
			}
		},
		"required": ["name"],
		"additionalProperties": false
	}`))
	if err != nil {
		f.Fatal(err)
	}

	f.Add([]byte(`{"name":"test"}`))
	f.Add([]byte(`{"name":"test","value":42}`))
	f.Add([]byte(`{"name":"test","value":-1}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`{"extra":"field"}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		v := NewValidator()
		// Should not panic
		_ = v.Validate(schema, data)
	})
}

// FuzzReferenceResolution fuzzes JSON Pointer resolution.
func FuzzReferenceResolution(f *testing.F) {
	root := map[string]any{
		"definitions": map[string]any{
			"foo": map[string]any{"type": "string"},
		},
		"properties": map[string]any{
			"a": map[string]any{"$ref": "#/definitions/foo"},
		},
	}
	f.Add("/definitions/foo")
	f.Add("/properties/a")
	f.Add("")
	f.Add("/nonexistent")
	f.Add("/definitions/foo/extra")

	f.Fuzz(func(t *testing.T, ptr string) {
		// Should not panic
		_, _ = resolveJSONPointer(root, ptr)
	})
}

// Ensure json import is used
var _ = json.Unmarshal
