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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func compileSchema(t *testing.T, src string) *Schema {
	t.Helper()
	c := NewCompiler()
	s, err := c.Compile([]byte(src))
	require.NoError(t, err, "failed to compile schema: %s", src)
	return s
}

func validateValue(t *testing.T, schema *Schema, instance string) Failures {
	t.Helper()
	v := NewValidator()
	var val any
	require.NoError(t, json.Unmarshal([]byte(instance), &val))
	return v.ValidateValue(schema, val)
}

func assertNoFailures(t *testing.T, fs Failures) {
	t.Helper()
	if len(fs) > 0 {
		t.Fatalf("expected no failures, got %d:\n%s", len(fs), fs.Error())
	}
}

func assertHasFailure(t *testing.T, fs Failures, reason ReasonCode) {
	t.Helper()
	if !fs.Has(reason) {
		t.Fatalf("expected failure with reason %s, got %d failures:\n%s", reason, len(fs), fs.Error())
	}
}

func TestValidator_TypeObject_Pass(t *testing.T) {
	s := compileSchema(t, `{"type":"object"}`)
	fs := validateValue(t, s, `{"a":1}`)
	assertNoFailures(t, fs)
}

func TestValidator_TypeObject_Fail(t *testing.T) {
	s := compileSchema(t, `{"type":"object"}`)
	fs := validateValue(t, s, `[1,2]`)
	assertHasFailure(t, fs, ReasonValueTypeMismatch)
}

func TestValidator_TypeString_Pass(t *testing.T) {
	s := compileSchema(t, `{"type":"string"}`)
	fs := validateValue(t, s, `"hello"`)
	assertNoFailures(t, fs)
}

func TestValidator_TypeInteger_Pass(t *testing.T) {
	s := compileSchema(t, `{"type":"integer"}`)
	fs := validateValue(t, s, `42`)
	assertNoFailures(t, fs)
}

func TestValidator_TypeInteger_FailFloat(t *testing.T) {
	s := compileSchema(t, `{"type":"integer"}`)
	fs := validateValue(t, s, `42.5`)
	assertHasFailure(t, fs, ReasonValueTypeMismatch)
}

func TestValidator_Enum_Pass(t *testing.T) {
	s := compileSchema(t, `{"enum":["a","b","c"]}`)
	fs := validateValue(t, s, `"b"`)
	assertNoFailures(t, fs)
}

func TestValidator_Enum_Fail(t *testing.T) {
	s := compileSchema(t, `{"enum":["a","b","c"]}`)
	fs := validateValue(t, s, `"d"`)
	assertHasFailure(t, fs, ReasonValueEnumMismatch)
}

func TestValidator_Const_Pass(t *testing.T) {
	s := compileSchema(t, `{"const":42}`)
	fs := validateValue(t, s, `42`)
	assertNoFailures(t, fs)
}

func TestValidator_Const_Fail(t *testing.T) {
	s := compileSchema(t, `{"const":42}`)
	fs := validateValue(t, s, `43`)
	assertHasFailure(t, fs, ReasonValueConstMismatch)
}

func TestValidator_Required_Pass(t *testing.T) {
	s := compileSchema(t, `{"type":"object","required":["a","b"],"properties":{"a":{"type":"string"},"b":{"type":"number"}}}`)
	fs := validateValue(t, s, `{"a":"x","b":1}`)
	assertNoFailures(t, fs)
}

func TestValidator_Required_FailMissing(t *testing.T) {
	s := compileSchema(t, `{"type":"object","required":["a","b"]}`)
	fs := validateValue(t, s, `{"a":"x"}`)
	assertHasFailure(t, fs, ReasonValueRequiredMissing)
}

func TestValidator_AdditionalPropertiesFalse_Fail(t *testing.T) {
	s := compileSchema(t, `{"type":"object","properties":{"a":{"type":"string"}},"additionalProperties":false}`)
	fs := validateValue(t, s, `{"a":"x","b":1}`)
	assertHasFailure(t, fs, ReasonValueAdditionalProp)
}

func TestValidator_AdditionalPropertiesFalse_Pass(t *testing.T) {
	s := compileSchema(t, `{"type":"object","properties":{"a":{"type":"string"}},"additionalProperties":false}`)
	fs := validateValue(t, s, `{"a":"x"}`)
	assertNoFailures(t, fs)
}

func TestValidator_MinProperties_Fail(t *testing.T) {
	s := compileSchema(t, `{"type":"object","minProperties":3}`)
	fs := validateValue(t, s, `{"a":1,"b":2}`)
	assertHasFailure(t, fs, ReasonValueMinProperties)
}

func TestValidator_MaxProperties_Fail(t *testing.T) {
	s := compileSchema(t, `{"type":"object","maxProperties":2}`)
	fs := validateValue(t, s, `{"a":1,"b":2,"c":3}`)
	assertHasFailure(t, fs, ReasonValueMaxProperties)
}

func TestValidator_Items_Pass(t *testing.T) {
	s := compileSchema(t, `{"type":"array","items":{"type":"number"},"minItems":1}`)
	fs := validateValue(t, s, `[1,2,3]`)
	assertNoFailures(t, fs)
}

func TestValidator_Items_Fail(t *testing.T) {
	s := compileSchema(t, `{"type":"array","items":{"type":"number"}}`)
	fs := validateValue(t, s, `[1,"x",3]`)
	assertHasFailure(t, fs, ReasonValueTypeMismatch)
}

func TestValidator_MinItems_Fail(t *testing.T) {
	s := compileSchema(t, `{"type":"array","items":{"type":"number"},"minItems":3}`)
	fs := validateValue(t, s, `[1,2]`)
	assertHasFailure(t, fs, ReasonValueMinItems)
}

func TestValidator_UniqueItems_Fail(t *testing.T) {
	s := compileSchema(t, `{"type":"array","uniqueItems":true}`)
	fs := validateValue(t, s, `[1,2,1]`)
	assertHasFailure(t, fs, ReasonValueUniqueItems)
}

func TestValidator_UniqueItems_Pass(t *testing.T) {
	s := compileSchema(t, `{"type":"array","uniqueItems":true}`)
	fs := validateValue(t, s, `[1,2,3]`)
	assertNoFailures(t, fs)
}

func TestValidator_MinLength_Fail(t *testing.T) {
	s := compileSchema(t, `{"type":"string","minLength":5}`)
	fs := validateValue(t, s, `"abc"`)
	assertHasFailure(t, fs, ReasonValueMinLength)
}

func TestValidator_MaxLength_Fail(t *testing.T) {
	s := compileSchema(t, `{"type":"string","maxLength":3}`)
	fs := validateValue(t, s, `"abcde"`)
	assertHasFailure(t, fs, ReasonValueMaxLength)
}

func TestValidator_Pattern_Pass(t *testing.T) {
	s := compileSchema(t, `{"type":"string","pattern":"^[0-9]+$"}`)
	fs := validateValue(t, s, `"12345"`)
	assertNoFailures(t, fs)
}

func TestValidator_Pattern_Fail(t *testing.T) {
	s := compileSchema(t, `{"type":"string","pattern":"^[0-9]+$"}`)
	fs := validateValue(t, s, `"12a45"`)
	assertHasFailure(t, fs, ReasonValuePatternMismatch)
}

func TestValidator_FormatDateTime_Pass(t *testing.T) {
	s := compileSchema(t, `{"type":"string","format":"date-time"}`)
	fs := validateValue(t, s, `"2026-09-07T12:00:00Z"`)
	assertNoFailures(t, fs)
}

func TestValidator_FormatDateTime_Fail(t *testing.T) {
	s := compileSchema(t, `{"type":"string","format":"date-time"}`)
	fs := validateValue(t, s, `"not-a-date"`)
	assertHasFailure(t, fs, ReasonValueFormatMismatch)
}

func TestValidator_FormatURI_Pass(t *testing.T) {
	s := compileSchema(t, `{"type":"string","format":"uri"}`)
	fs := validateValue(t, s, `"https://example.com/path"`)
	assertNoFailures(t, fs)
}

func TestValidator_FormatURI_Fail(t *testing.T) {
	s := compileSchema(t, `{"type":"string","format":"uri"}`)
	fs := validateValue(t, s, `"not-a-uri"`)
	assertHasFailure(t, fs, ReasonValueFormatMismatch)
}

func TestValidator_FormatEmail_Pass(t *testing.T) {
	s := compileSchema(t, `{"type":"string","format":"email"}`)
	fs := validateValue(t, s, `"user@example.com"`)
	assertNoFailures(t, fs)
}

func TestValidator_FormatEmail_Fail(t *testing.T) {
	s := compileSchema(t, `{"type":"string","format":"email"}`)
	fs := validateValue(t, s, `"not-an-email"`)
	assertHasFailure(t, fs, ReasonValueFormatMismatch)
}

func TestValidator_Minimum_Fail(t *testing.T) {
	s := compileSchema(t, `{"type":"number","minimum":10}`)
	fs := validateValue(t, s, `5`)
	assertHasFailure(t, fs, ReasonValueMinimum)
}

func TestValidator_Maximum_Fail(t *testing.T) {
	s := compileSchema(t, `{"type":"number","maximum":10}`)
	fs := validateValue(t, s, `15`)
	assertHasFailure(t, fs, ReasonValueMaximum)
}

func TestValidator_ExclusiveMinimum_Fail(t *testing.T) {
	s := compileSchema(t, `{"type":"number","exclusiveMinimum":10}`)
	fs := validateValue(t, s, `10`)
	assertHasFailure(t, fs, ReasonValueExclusiveMinimum)
}

func TestValidator_MultipleOf_Pass(t *testing.T) {
	s := compileSchema(t, `{"type":"number","multipleOf":3}`)
	fs := validateValue(t, s, `9`)
	assertNoFailures(t, fs)
}

func TestValidator_MultipleOf_Fail(t *testing.T) {
	s := compileSchema(t, `{"type":"number","multipleOf":3}`)
	fs := validateValue(t, s, `10`)
	assertHasFailure(t, fs, ReasonValueMultipleOf)
}

func TestValidator_AllOf_Pass(t *testing.T) {
	s := compileSchema(t, `{"allOf":[{"type":"string"},{"minLength":3}]}`)
	fs := validateValue(t, s, `"hello"`)
	assertNoFailures(t, fs)
}

func TestValidator_AllOf_Fail(t *testing.T) {
	s := compileSchema(t, `{"allOf":[{"type":"string"},{"minLength":10}]}`)
	fs := validateValue(t, s, `"hello"`)
	assertHasFailure(t, fs, ReasonValueAllOfFailed)
}

func TestValidator_AnyOf_Pass(t *testing.T) {
	s := compileSchema(t, `{"anyOf":[{"type":"string"},{"type":"number"}]}`)
	fs := validateValue(t, s, `42`)
	assertNoFailures(t, fs)
}

func TestValidator_AnyOf_Fail(t *testing.T) {
	s := compileSchema(t, `{"anyOf":[{"type":"string"},{"type":"number"}]}`)
	fs := validateValue(t, s, `true`)
	assertHasFailure(t, fs, ReasonValueAnyOfFailed)
}

func TestValidator_OneOf_Pass(t *testing.T) {
	s := compileSchema(t, `{"oneOf":[{"type":"string","minLength":5},{"type":"number"}]}`)
	fs := validateValue(t, s, `42`)
	assertNoFailures(t, fs)
}

func TestValidator_OneOf_FailMultiple(t *testing.T) {
	s := compileSchema(t, `{"oneOf":[{"type":"string"},{"type":"string","minLength":1}]}`)
	fs := validateValue(t, s, `"hello"`)
	assertHasFailure(t, fs, ReasonValueOneOfFailed)
}

func TestValidator_Not_Fail(t *testing.T) {
	s := compileSchema(t, `{"not":{"type":"string"}}`)
	fs := validateValue(t, s, `"hello"`)
	assertHasFailure(t, fs, ReasonValueNotFailed)
}

func TestValidator_IfThenElse_ThenPass(t *testing.T) {
	s := compileSchema(t, `{"if":{"type":"string"},"then":{"minLength":3}}`)
	fs := validateValue(t, s, `"hello"`)
	assertNoFailures(t, fs)
}

func TestValidator_IfThenElse_ThenFail(t *testing.T) {
	s := compileSchema(t, `{"if":{"type":"string"},"then":{"minLength":10}}`)
	fs := validateValue(t, s, `"hello"`)
	assertHasFailure(t, fs, ReasonValueIfThenFailed)
}

func TestValidator_IfThenElse_ElsePass(t *testing.T) {
	s := compileSchema(t, `{"if":{"type":"string"},"then":{"minLength":10},"else":{"type":"number"}}`)
	fs := validateValue(t, s, `42`)
	assertNoFailures(t, fs)
}

func TestValidator_IfThenElse_ElseFail(t *testing.T) {
	s := compileSchema(t, `{"if":{"type":"string"},"then":{"minLength":10},"else":{"type":"number"}}`)
	fs := validateValue(t, s, `true`)
	assertHasFailure(t, fs, ReasonValueIfElseFailed)
}

func TestValidator_Dependencies_Array_Pass(t *testing.T) {
	s := compileSchema(t, `{"dependencies":{"a":["b"]}}`)
	fs := validateValue(t, s, `{"a":1,"b":2}`)
	assertNoFailures(t, fs)
}

func TestValidator_Dependencies_Array_Fail(t *testing.T) {
	s := compileSchema(t, `{"dependencies":{"a":["b"]}}`)
	fs := validateValue(t, s, `{"a":1}`)
	assertHasFailure(t, fs, ReasonValueDependenciesFailed)
}

func TestValidator_Dependencies_Schema_Pass(t *testing.T) {
	s := compileSchema(t, `{"dependencies":{"a":{"type":"object","required":["b"]}}}`)
	fs := validateValue(t, s, `{"a":1,"b":2}`)
	assertNoFailures(t, fs)
}

func TestValidator_Dependencies_Schema_Fail(t *testing.T) {
	s := compileSchema(t, `{"dependencies":{"a":{"type":"object","required":["b"]}}}`)
	fs := validateValue(t, s, `{"a":1}`)
	assertHasFailure(t, fs, ReasonValueRequiredMissing)
}

func TestValidator_Contains_Pass(t *testing.T) {
	s := compileSchema(t, `{"type":"array","contains":{"type":"number","minimum":5}}`)
	fs := validateValue(t, s, `[1,2,5,3]`)
	assertNoFailures(t, fs)
}

func TestValidator_Contains_Fail(t *testing.T) {
	s := compileSchema(t, `{"type":"array","contains":{"type":"number","minimum":10}}`)
	fs := validateValue(t, s, `[1,2,3]`)
	assertHasFailure(t, fs, ReasonValueContainsMissing)
}

func TestValidator_BooleanTrueSchema_AcceptsAll(t *testing.T) {
	s := compileSchema(t, `true`)
	fs := validateValue(t, s, `"anything"`)
	assertNoFailures(t, fs)
}

func TestValidator_BooleanFalseSchema_RejectsAll(t *testing.T) {
	s := compileSchema(t, `false`)
	fs := validateValue(t, s, `"anything"`)
	assertHasFailure(t, fs, ReasonValueTypeMismatch)
}

func TestValidator_RefResolution(t *testing.T) {
	s := compileSchema(t, `{
		"definitions": {"pos": {"type":"integer","minimum":0}},
		"properties": {"x": {"$ref": "#/definitions/pos"}}
	}`)
	fs := validateValue(t, s, `{"x": -1}`)
	assertHasFailure(t, fs, ReasonValueMinimum)
}

func TestValidator_NestedObjectValidation(t *testing.T) {
	s := compileSchema(t, `{
		"type":"object",
		"properties": {
			"nested": {
				"type":"object",
				"properties": {"a": {"type":"string"}},
				"required": ["a"]
			}
		},
		"required": ["nested"]
	}`)
	fs := validateValue(t, s, `{"nested": {}}`)
	assertHasFailure(t, fs, ReasonValueRequiredMissing)
}

func TestValidator_FailureDeterministicOrdering(t *testing.T) {
	s := compileSchema(t, `{"type":"object","required":["a","b","c"]}`)
	fs1 := validateValue(t, s, `{}`)
	fs2 := validateValue(t, s, `{}`)
	assert.Equal(t, len(fs1), len(fs2))
	for i := range fs1 {
		assert.Equal(t, fs1[i].Reason, fs2[i].Reason)
		assert.Equal(t, fs1[i].InstancePtr, fs2[i].InstancePtr)
	}
}

func TestValidator_PreservesNumberPrecision(t *testing.T) {
	// Large integer that would lose precision as float64
	s := compileSchema(t, `{"type":"integer"}`)
	v := NewValidator()
	var val any
	require.NoError(t, json.Unmarshal([]byte(`9007199254740993`), &val))
	fs := v.ValidateValue(s, val)
	assertNoFailures(t, fs)
}

func TestValidator_RejectsTrailingData(t *testing.T) {
	s := compileSchema(t, `{"type":"string"}`)
	v := NewValidator()
	fs := v.Validate(s, []byte(`"hello" "extra"`))
	assert.True(t, len(fs) > 0)
}

func TestValidator_RejectsInvalidJSON(t *testing.T) {
	s := compileSchema(t, `{"type":"string"}`)
	v := NewValidator()
	fs := v.Validate(s, []byte(`{invalid`))
	assert.True(t, len(fs) > 0)
}

func TestValidator_NeverPanics(t *testing.T) {
	s := compileSchema(t, `{"type":"object","properties":{"a":{"type":"string"}}}`)
	v := NewValidator()
	// Various inputs that should not cause panics
	inputs := []string{
		`null`,
		`true`,
		`false`,
		`0`,
		`""`,
		`[]`,
		`{}`,
		`{"a":null}`,
		`{"a":true}`,
		`{"a":[]}`,
		`[null,true,false,0,"",{},[]]`,
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			var val any
			require.NoError(t, json.Unmarshal([]byte(input), &val))
			_ = v.ValidateValue(s, val)
		})
	}
}
