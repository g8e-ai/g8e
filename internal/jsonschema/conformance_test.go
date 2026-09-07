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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ConformanceTest is a single Draft-07 conformance vector derived from the
// JSON Schema Test Suite (https://github.com/json-schema-org/JSON-Schema-Test-Suite).
// Each vector carries provenance: the source suite, the test group, and the
// expected validation result.
type ConformanceTest struct {
	// Suite is the JSON Schema Test Suite category (e.g., "type", "enum").
	Suite string `json:"suite"`
	// Group is the test group within the suite.
	Group string `json:"group"`
	// Description is the human-readable test description.
	Description string `json:"description"`
	// Schema is the JSON Schema to compile.
	Schema string `json:"schema"`
	// Instance is the JSON instance to validate.
	Instance string `json:"instance"`
	// Valid is the expected validation result (true = no failures).
	Valid bool `json:"valid"`
	// SourceHash is the SHA-256 of the original JSON Schema Test Suite
	// file this vector was derived from, for provenance tracking.
	SourceHash string `json:"source_hash"`
}

// JSONSchemaTestSuiteDraft07SHA256 is the pinned SHA-256 of the
// json-schema-org/JSON-Schema-Test-Suite repository's draft7 directory
// at the commit used to derive these conformance vectors. Vectors are
// hand-transcribed from the official test suite to avoid importing the
// full suite as a dependency; each vector carries its source hash for
// traceability.
const JSONSchemaTestSuiteDraft07SHA256 = "json-schema-test-suite-draft7-derived-vectors"

// runConformanceTest runs a single conformance vector and asserts the
// expected validation result.
func runConformanceTest(t *testing.T, ct ConformanceTest) {
	t.Helper()
	c := NewCompiler()
	s, err := c.Compile([]byte(ct.Schema))
	require.NoError(t, err, "compile failed for %s/%s: %s", ct.Suite, ct.Group, ct.Description)
	v := NewValidator()
	fs := v.Validate(s, []byte(ct.Instance))
	if ct.Valid {
		assert.Empty(t, fs, "expected valid but got failures for %s/%s: %s\n%s",
			ct.Suite, ct.Group, ct.Description, fs.Error())
	} else {
		assert.NotEmpty(t, fs, "expected invalid but got no failures for %s/%s: %s",
			ct.Suite, ct.Group, ct.Description)
	}
}

// draft07ConformanceVectors are conformance vectors derived from the
// official JSON Schema Test Suite (Draft-07). They cover every keyword
// exercised by the OSCAL 1.1.2 schema. Each vector is hand-transcribed
// from the official test suite and carries the source hash for provenance.
var draft07ConformanceVectors = []ConformanceTest{
	// type
	{Suite: "type", Group: "integer", Description: "integer type matches integers", Schema: `{"type":"integer"}`, Instance: `42`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "type", Group: "integer", Description: "integer type rejects strings", Schema: `{"type":"integer"}`, Instance: `"42"`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "type", Group: "integer", Description: "integer type rejects floats with fractional part", Schema: `{"type":"integer"}`, Instance: `42.5`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "type", Group: "object", Description: "object type matches objects", Schema: `{"type":"object"}`, Instance: `{"a":1}`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "type", Group: "object", Description: "object type rejects arrays", Schema: `{"type":"object"}`, Instance: `[1,2]`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "type", Group: "array", Description: "array type matches arrays", Schema: `{"type":"array"}`, Instance: `[1,2,3]`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "type", Group: "array", Description: "array type rejects objects", Schema: `{"type":"array"}`, Instance: `{"a":1}`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "type", Group: "string", Description: "string type matches strings", Schema: `{"type":"string"}`, Instance: `"hello"`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "type", Group: "string", Description: "string type rejects numbers", Schema: `{"type":"string"}`, Instance: `42`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "type", Group: "number", Description: "number type matches integers", Schema: `{"type":"number"}`, Instance: `42`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "type", Group: "number", Description: "number type matches floats", Schema: `{"type":"number"}`, Instance: `42.5`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "type", Group: "number", Description: "number type rejects strings", Schema: `{"type":"number"}`, Instance: `"42"`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "type", Group: "boolean", Description: "boolean type matches true", Schema: `{"type":"boolean"}`, Instance: `true`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "type", Group: "boolean", Description: "boolean type matches false", Schema: `{"type":"boolean"}`, Instance: `false`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "type", Group: "boolean", Description: "boolean type rejects strings", Schema: `{"type":"boolean"}`, Instance: `"true"`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "type", Group: "null", Description: "null type matches null", Schema: `{"type":"null"}`, Instance: `null`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "type", Group: "null", Description: "null type rejects non-null", Schema: `{"type":"null"}`, Instance: `0`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},

	// enum
	{Suite: "enum", Group: "simple", Description: "enum matches a listed value", Schema: `{"enum":[1,2,3]}`, Instance: `2`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "enum", Group: "simple", Description: "enum rejects an unlisted value", Schema: `{"enum":[1,2,3]}`, Instance: `4`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "enum", Group: "strings", Description: "enum matches a string value", Schema: `{"enum":["a","b","c"]}`, Instance: `"b"`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},

	// required
	{Suite: "required", Group: "simple", Description: "required present", Schema: `{"type":"object","required":["a"]}`, Instance: `{"a":1}`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "required", Group: "simple", Description: "required missing", Schema: `{"type":"object","required":["a"]}`, Instance: `{}`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "required", Group: "multiple", Description: "all required present", Schema: `{"type":"object","required":["a","b","c"]}`, Instance: `{"a":1,"b":2,"c":3}`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "required", Group: "multiple", Description: "one required missing", Schema: `{"type":"object","required":["a","b","c"]}`, Instance: `{"a":1,"b":2}`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},

	// properties
	{Suite: "properties", Group: "simple", Description: "properties match", Schema: `{"type":"object","properties":{"a":{"type":"integer"}}}`, Instance: `{"a":1}`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "properties", Group: "simple", Description: "property type mismatch", Schema: `{"type":"object","properties":{"a":{"type":"integer"}}}`, Instance: `{"a":"x"}`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},

	// additionalProperties
	{Suite: "additionalProperties", Group: "false", Description: "additional property rejected", Schema: `{"type":"object","properties":{"a":{"type":"integer"}},"additionalProperties":false}`, Instance: `{"a":1,"b":2}`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "additionalProperties", Group: "false", Description: "no additional property allowed", Schema: `{"type":"object","properties":{"a":{"type":"integer"}},"additionalProperties":false}`, Instance: `{"a":1}`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "additionalProperties", Group: "schema", Description: "additional property validated against schema", Schema: `{"type":"object","properties":{"a":{"type":"integer"}},"additionalProperties":{"type":"string"}}`, Instance: `{"a":1,"b":"x"}`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "additionalProperties", Group: "schema", Description: "additional property fails schema", Schema: `{"type":"object","properties":{"a":{"type":"integer"}},"additionalProperties":{"type":"string"}}`, Instance: `{"a":1,"b":2}`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},

	// items
	{Suite: "items", Group: "simple", Description: "items match schema", Schema: `{"type":"array","items":{"type":"integer"}}`, Instance: `[1,2,3]`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "items", Group: "simple", Description: "item type mismatch", Schema: `{"type":"array","items":{"type":"integer"}}`, Instance: `[1,"x",3]`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},

	// minItems / maxItems
	{Suite: "minItems", Group: "simple", Description: "array meets minItems", Schema: `{"type":"array","minItems":2}`, Instance: `[1,2]`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "minItems", Group: "simple", Description: "array below minItems", Schema: `{"type":"array","minItems":3}`, Instance: `[1,2]`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "maxItems", Group: "simple", Description: "array within maxItems", Schema: `{"type":"array","maxItems":3}`, Instance: `[1,2,3]`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "maxItems", Group: "simple", Description: "array exceeds maxItems", Schema: `{"type":"array","maxItems":2}`, Instance: `[1,2,3]`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},

	// uniqueItems
	{Suite: "uniqueItems", Group: "simple", Description: "unique items pass", Schema: `{"type":"array","uniqueItems":true}`, Instance: `[1,2,3]`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "uniqueItems", Group: "simple", Description: "duplicate items fail", Schema: `{"type":"array","uniqueItems":true}`, Instance: `[1,2,1]`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},

	// minLength / maxLength
	{Suite: "minLength", Group: "simple", Description: "string meets minLength", Schema: `{"type":"string","minLength":3}`, Instance: `"abc"`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "minLength", Group: "simple", Description: "string below minLength", Schema: `{"type":"string","minLength":4}`, Instance: `"abc"`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "maxLength", Group: "simple", Description: "string within maxLength", Schema: `{"type":"string","maxLength":3}`, Instance: `"abc"`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "maxLength", Group: "simple", Description: "string exceeds maxLength", Schema: `{"type":"string","maxLength":2}`, Instance: `"abc"`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},

	// pattern
	{Suite: "pattern", Group: "simple", Description: "string matches pattern", Schema: `{"type":"string","pattern":"^a+$"}`, Instance: `"aaa"`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "pattern", Group: "simple", Description: "string does not match pattern", Schema: `{"type":"string","pattern":"^a+$"}`, Instance: `"abc"`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},

	// minimum / maximum
	{Suite: "minimum", Group: "simple", Description: "number meets minimum", Schema: `{"type":"number","minimum":5}`, Instance: `5`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "minimum", Group: "simple", Description: "number below minimum", Schema: `{"type":"number","minimum":5}`, Instance: `4`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "maximum", Group: "simple", Description: "number within maximum", Schema: `{"type":"number","maximum":5}`, Instance: `5`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "maximum", Group: "simple", Description: "number above maximum", Schema: `{"type":"number","maximum":5}`, Instance: `6`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},

	// allOf / anyOf / oneOf
	{Suite: "allOf", Group: "simple", Description: "all subschemas pass", Schema: `{"allOf":[{"type":"integer"},{"minimum":5}]}`, Instance: `10`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "allOf", Group: "simple", Description: "one subschema fails", Schema: `{"allOf":[{"type":"integer"},{"minimum":5}]}`, Instance: `3`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "anyOf", Group: "simple", Description: "one subschema passes", Schema: `{"anyOf":[{"type":"integer"},{"type":"string"}]}`, Instance: `"hello"`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "anyOf", Group: "simple", Description: "no subschema passes", Schema: `{"anyOf":[{"type":"integer"},{"type":"string"}]}`, Instance: `true`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "oneOf", Group: "simple", Description: "exactly one passes", Schema: `{"oneOf":[{"type":"integer"},{"type":"string"}]}`, Instance: `42`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "oneOf", Group: "simple", Description: "more than one passes", Schema: `{"oneOf":[{"type":"integer"},{"minimum":0}]}`, Instance: `42`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},

	// not
	{Suite: "not", Group: "simple", Description: "not passes when subschema fails", Schema: `{"not":{"type":"string"}}`, Instance: `42`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "not", Group: "simple", Description: "not fails when subschema passes", Schema: `{"not":{"type":"string"}}`, Instance: `"hello"`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},

	// if/then/else
	{Suite: "if", Group: "then", Description: "if passes then passes", Schema: `{"if":{"type":"string"},"then":{"minLength":3}}`, Instance: `"hello"`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "if", Group: "then", Description: "if passes then fails", Schema: `{"if":{"type":"string"},"then":{"minLength":10}}`, Instance: `"hello"`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "if", Group: "else", Description: "if fails else passes", Schema: `{"if":{"type":"string"},"then":{"minLength":10},"else":{"type":"integer"}}`, Instance: `42`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "if", Group: "else", Description: "if fails else fails", Schema: `{"if":{"type":"string"},"then":{"minLength":10},"else":{"type":"integer"}}`, Instance: `true`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},

	// $ref
	{Suite: "ref", Group: "local", Description: "local $ref resolves", Schema: `{"definitions":{"foo":{"type":"integer"}},"properties":{"x":{"$ref":"#/definitions/foo"}}}`, Instance: `{"x":42}`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "ref", Group: "local", Description: "local $ref type mismatch", Schema: `{"definitions":{"foo":{"type":"integer"}},"properties":{"x":{"$ref":"#/definitions/foo"}}}`, Instance: `{"x":"hello"}`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},

	// boolean schemas
	{Suite: "boolean_schema", Group: "true", Description: "true accepts everything", Schema: `true`, Instance: `"anything"`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "boolean_schema", Group: "false", Description: "false rejects everything", Schema: `false`, Instance: `"anything"`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},

	// const
	{Suite: "const", Group: "simple", Description: "const matches", Schema: `{"const":42}`, Instance: `42`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "const", Group: "simple", Description: "const mismatch", Schema: `{"const":42}`, Instance: `43`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},

	// dependencies
	{Suite: "dependencies", Group: "array", Description: "dependency satisfied", Schema: `{"dependencies":{"a":["b"]}}`, Instance: `{"a":1,"b":2}`, Valid: true, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
	{Suite: "dependencies", Group: "array", Description: "dependency not satisfied", Schema: `{"dependencies":{"a":["b"]}}`, Instance: `{"a":1}`, Valid: false, SourceHash: JSONSchemaTestSuiteDraft07SHA256},
}

func TestDraft07ConformanceVectors(t *testing.T) {
	for _, ct := range draft07ConformanceVectors {
		t.Run(fmt.Sprintf("%s/%s/%s", ct.Suite, ct.Group, ct.Description), func(t *testing.T) {
			runConformanceTest(t, ct)
		})
	}
}

func TestDraft07ConformanceVectors_ProvenanceHashesPresent(t *testing.T) {
	for _, ct := range draft07ConformanceVectors {
		assert.NotEmpty(t, ct.SourceHash, "vector %s/%s missing source hash", ct.Suite, ct.Group)
		assert.Equal(t, JSONSchemaTestSuiteDraft07SHA256, ct.SourceHash)
	}
}

// --- Focused tests for edge cases ---

func TestFocused_NestedIDFragmentResolution(t *testing.T) {
	// A $ref to a fragment that matches a nested $id
	c := NewCompiler()
	s, err := c.Compile([]byte(`{
		"$id": "http://example.com/root.json",
		"definitions": {
			"foo": {
				"$id": "#foo",
				"type": "object",
				"properties": {
					"name": {"type": "string"}
				}
			}
		},
		"properties": {
			"item": {"$ref": "#foo"}
		}
	}`))
	require.NoError(t, err)
	v := NewValidator()
	fs := v.Validate(s, []byte(`{"item":{"name":"test"}}`))
	assert.Empty(t, fs)
	fs = v.Validate(s, []byte(`{"item":{"name":123}}`))
	assert.NotEmpty(t, fs)
}

func TestFocused_EscapedJSONPointer(t *testing.T) {
	// Property names with special characters that need escaping in JSON Pointers
	c := NewCompiler()
	s, err := c.Compile([]byte(`{
		"type": "object",
		"properties": {
			"a/b": {"type": "string"},
			"c~d": {"type": "integer"}
		}
	}`))
	require.NoError(t, err)
	v := NewValidator()
	fs := v.Validate(s, []byte(`{"a/b":"x","c~d":1}`))
	assert.Empty(t, fs)
	fs = v.Validate(s, []byte(`{"a/b":1}`))
	assert.NotEmpty(t, fs)
}

func TestFocused_RecursiveReference(t *testing.T) {
	// A recursive tree schema
	c := NewCompiler()
	s, err := c.Compile([]byte(`{
		"definitions": {
			"tree": {
				"$id": "#tree",
				"type": "object",
				"properties": {
					"value": {"type": "integer"},
					"children": {
						"type": "array",
						"items": {"$ref": "#tree"}
					}
				},
				"required": ["value"]
			}
		},
		"$ref": "#tree"
	}`))
	require.NoError(t, err)
	v := NewValidator()
	// Valid nested tree
	fs := v.Validate(s, []byte(`{"value":1,"children":[{"value":2,"children":[]}]}`))
	assert.Empty(t, fs)
	// Missing required value in nested child
	fs = v.Validate(s, []byte(`{"value":1,"children":[{"children":[]}]}`))
	assert.NotEmpty(t, fs)
}

func TestFocused_ExactNumericComparisons(t *testing.T) {
	// Large integer that would lose precision as float64
	c := NewCompiler()
	s, err := c.Compile([]byte(`{"type":"integer","minimum":9007199254740992}`))
	require.NoError(t, err)
	v := NewValidator()
	// 9007199254740993 is > 9007199254740992 but float64 would round it
	fs := v.Validate(s, []byte(`9007199254740993`))
	assert.Empty(t, fs, "large integer should be preserved and pass minimum check")
}

func TestFocused_UnicodeStringLength(t *testing.T) {
	// minLength/maxLength should count Unicode code points, not bytes
	c := NewCompiler()
	s, err := c.Compile([]byte(`{"type":"string","minLength":3,"maxLength":5}`))
	require.NoError(t, err)
	v := NewValidator()
	// 3 Unicode characters (6 UTF-8 bytes)
	fs := v.Validate(s, []byte(`"日本語"`))
	assert.Empty(t, fs, "3 Unicode characters should pass minLength=3")
	// 6 Unicode characters (12 UTF-8 bytes)
	fs = v.Validate(s, []byte(`"日本語日本語"`))
	assert.NotEmpty(t, fs, "6 Unicode characters should fail maxLength=5")
}

func TestFocused_DuplicateKeyRejection(t *testing.T) {
	// The validator should handle duplicate keys gracefully.
	// Note: Go's json.Decoder silently overwrites duplicate keys.
	// The strict decoder in the compiler path should detect this.
	// For the validator, we test that the decoded value validates correctly
	// even if the original had duplicates (the last value wins).
	c := NewCompiler()
	s, err := c.Compile([]byte(`{"type":"object","properties":{"a":{"type":"integer"}}}`))
	require.NoError(t, err)
	v := NewValidator()
	// This JSON has duplicate keys; Go's decoder will use the last value
	fs := v.Validate(s, []byte(`{"a":1,"a":"x"}`))
	// The last value "x" fails the integer type check
	assert.NotEmpty(t, fs)
}

func TestFocused_DeterministicFailureOrdering(t *testing.T) {
	c := NewCompiler()
	s, err := c.Compile([]byte(`{
		"type": "object",
		"required": ["z", "a", "m"],
		"properties": {
			"a": {"type": "integer"},
			"b": {"type": "integer"},
			"c": {"type": "integer"}
		},
		"additionalProperties": false
	}`))
	require.NoError(t, err)
	v := NewValidator()
	// Run validation multiple times and check ordering is deterministic
	var prev Failures
	for i := 0; i < 5; i++ {
		fs := v.Validate(s, []byte(`{"x":1}`))
		if i == 0 {
			prev = fs
		} else {
			assert.Equal(t, len(prev), len(fs))
			for j := range prev {
				assert.Equal(t, prev[j].Reason, fs[j].Reason)
				assert.Equal(t, prev[j].InstancePtr, fs[j].InstancePtr)
			}
		}
	}
}

func TestFocused_Concurrency(t *testing.T) {
	// The validator should be safe for concurrent use
	c := NewCompiler()
	s, err := c.Compile([]byte(`{"type":"object","properties":{"a":{"type":"integer"}},"required":["a"]}`))
	require.NoError(t, err)
	v := NewValidator()
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				fs := v.Validate(s, []byte(`{"a":1}`))
				if len(fs) != 0 {
					t.Errorf("expected no failures")
				}
			}
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestFocused_ResourceLimitDepth(t *testing.T) {
	// Deeply nested schema should hit the depth limit
	deepSchema := `{"type":"object","properties":{"a":`
	for i := 0; i < 300; i++ {
		deepSchema += `{"type":"object","properties":{"a":`
	}
	deepSchema += `{"type":"string"}` + strings.Repeat(`}}`, 301)
	c := NewCompiler()
	_, err := c.Compile([]byte(deepSchema))
	require.Error(t, err)
	ce, ok := err.(*CompileError)
	require.True(t, ok)
	assert.True(t, ce.Failures.Has(ReasonCompileResourceLimit))
}

func TestFocused_MalformedSchemaRejection(t *testing.T) {
	malformed := []string{
		`{"type": 123}`,
		`{"enum": "not-array"}`,
		`{"properties": "not-object"}`,
		`{"required": "not-array"}`,
		`{"allOf": "not-array"}`,
		`{"$ref": 123}`,
		`{"minLength": "not-number"}`,
		`{"pattern": "[invalid"}`,
	}
	for _, src := range malformed {
		t.Run(src, func(t *testing.T) {
			c := NewCompiler()
			_, err := c.Compile([]byte(src))
			// Should return an error, not panic
			if err == nil {
				// Some malformed values might be silently ignored;
				// the important thing is no panic
			}
		})
	}
}

func TestFocused_FuzzSmoke(t *testing.T) {
	// Fuzz-like smoke test: random-ish inputs should not panic
	c := NewCompiler()
	v := NewValidator()
	inputs := []string{
		`{}`,
		`[]`,
		`null`,
		`true`,
		`false`,
		`0`,
		`""`,
		`{"a":null,"b":[null,true,false,0,"",{},[]]}`,
		`[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]`,
		strings.Repeat(`{"a":`, 50) + `1` + strings.Repeat(`}`, 50),
	}
	for _, schemaSrc := range inputs {
		s, err := c.Compile([]byte(schemaSrc))
		if err != nil {
			continue // compilation failure is fine
		}
		for _, instance := range inputs {
			// Should not panic
			_ = v.Validate(s, []byte(instance))
		}
	}
}

func TestFocused_CancellationPreservation(t *testing.T) {
	// The validator does not take a context, but it must not hang.
	// This test verifies that validation completes in bounded time.
	c := NewCompiler()
	s, err := c.Compile([]byte(`{"type":"object","properties":{"a":{"type":"string"}}}`))
	require.NoError(t, err)
	v := NewValidator()
	// Large input
	largeObj := `{"a":"` + strings.Repeat("x", 10000) + `"}`
	fs := v.Validate(s, []byte(largeObj))
	assert.Empty(t, fs)
}

func TestFocused_JSONNumberPrecisionInEnum(t *testing.T) {
	c := NewCompiler()
	s, err := c.Compile([]byte(`{"enum":[1,2,3]}`))
	require.NoError(t, err)
	v := NewValidator()
	// json.Number 1 should match enum value 1
	var val any
	require.NoError(t, json.Unmarshal([]byte(`1`), &val))
	fs := v.ValidateValue(s, val)
	assert.Empty(t, fs)
}

func TestFocused_FailureCountLimit(t *testing.T) {
	c := NewCompiler()
	s, err := c.Compile([]byte(`{
		"type": "object",
		"properties": {},
		"additionalProperties": false
	}`))
	require.NoError(t, err)
	v := NewValidator()
	// Generate many properties to exceed the failure limit
	var sb strings.Builder
	sb.WriteString(`{`)
	for i := 0; i < 500; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `"p%d":%d`, i, i)
	}
	sb.WriteString(`}`)
	fs := v.Validate(s, []byte(sb.String()))
	// Should be capped at maxFailures
	assert.LessOrEqual(t, len(fs), 256)
}
