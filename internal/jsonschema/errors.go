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
	"io"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// ReasonCode is a stable, machine-readable failure code. Every compile and
// validation failure carries one. Reason codes are part of the stable API
// and must not change between versions.
type ReasonCode string

// Compile-time reason codes.
const (
	ReasonCompileUnrecognizedKeyword ReasonCode = "compile.unrecognized_keyword"
	ReasonCompileUnresolvedRef       ReasonCode = "compile.unresolved_ref"
	ReasonCompileDuplicateID         ReasonCode = "compile.duplicate_id"
	ReasonCompileInvalidRegex        ReasonCode = "compile.invalid_regex"
	ReasonCompileInvalidKeywordValue ReasonCode = "compile.invalid_keyword_value"
	ReasonCompileUnsupportedFormat   ReasonCode = "compile.unsupported_format"
	ReasonCompileRefCycle            ReasonCode = "compile.ref_cycle"
	ReasonCompileInvalidJSON         ReasonCode = "compile.invalid_json"
	ReasonCompileDuplicateKey        ReasonCode = "compile.duplicate_key"
	ReasonCompileTrailingData        ReasonCode = "compile.trailing_data"
	ReasonCompileResourceLimit       ReasonCode = "compile.resource_limit"
	ReasonCompileInvalidType         ReasonCode = "compile.invalid_type"
)

// Validation reason codes.
const (
	ReasonValueTypeMismatch       ReasonCode = "validate.type_mismatch"
	ReasonValueEnumMismatch       ReasonCode = "validate.enum_mismatch"
	ReasonValueConstMismatch      ReasonCode = "validate.const_mismatch"
	ReasonValueRequiredMissing    ReasonCode = "validate.required_missing"
	ReasonValueAdditionalProp     ReasonCode = "validate.additional_property"
	ReasonValueMinItems           ReasonCode = "validate.min_items"
	ReasonValueMaxItems           ReasonCode = "validate.max_items"
	ReasonValueMinProperties      ReasonCode = "validate.min_properties"
	ReasonValueMaxProperties      ReasonCode = "validate.max_properties"
	ReasonValueMinLength          ReasonCode = "validate.min_length"
	ReasonValueMaxLength          ReasonCode = "validate.max_length"
	ReasonValuePatternMismatch    ReasonCode = "validate.pattern_mismatch"
	ReasonValueFormatMismatch     ReasonCode = "validate.format_mismatch"
	ReasonValueMinimum            ReasonCode = "validate.minimum"
	ReasonValueMaximum            ReasonCode = "validate.maximum"
	ReasonValueExclusiveMinimum   ReasonCode = "validate.exclusive_minimum"
	ReasonValueExclusiveMaximum   ReasonCode = "validate.exclusive_maximum"
	ReasonValueMultipleOf         ReasonCode = "validate.multiple_of"
	ReasonValueUniqueItems        ReasonCode = "validate.unique_items"
	ReasonValueAllOfFailed        ReasonCode = "validate.all_of_failed"
	ReasonValueAnyOfFailed        ReasonCode = "validate.any_of_failed"
	ReasonValueOneOfFailed        ReasonCode = "validate.one_of_failed"
	ReasonValueNotFailed          ReasonCode = "validate.not_failed"
	ReasonValueIfThenFailed       ReasonCode = "validate.if_then_failed"
	ReasonValueIfElseFailed       ReasonCode = "validate.if_else_failed"
	ReasonValueDependenciesFailed ReasonCode = "validate.dependencies_failed"
	ReasonValueContainsMissing    ReasonCode = "validate.contains_missing"
	ReasonValuePropertyNames      ReasonCode = "validate.property_names"
)

// Failure is a typed validation or compilation failure. It carries a stable
// reason code, a human-readable message, the instance JSON Pointer (for
// validation failures), the schema location, and the keyword that produced
// the failure.
type Failure struct {
	Reason      ReasonCode `json:"reason"`
	Message     string     `json:"message"`
	InstancePtr string     `json:"instance_ptr,omitempty"`
	SchemaPtr   string     `json:"schema_ptr,omitempty"`
	Keyword     string     `json:"keyword,omitempty"`
}

func (f Failure) Error() string {
	var b strings.Builder
	b.WriteString(string(f.Reason))
	if f.SchemaPtr != "" {
		fmt.Fprintf(&b, " schema=%s", f.SchemaPtr)
	}
	if f.InstancePtr != "" {
		fmt.Fprintf(&b, " instance=%s", f.InstancePtr)
	}
	if f.Keyword != "" {
		fmt.Fprintf(&b, " keyword=%s", f.Keyword)
	}
	if f.Message != "" {
		fmt.Fprintf(&b, ": %s", f.Message)
	}
	return b.String()
}

// Failures is a collection of typed failures with deterministic ordering.
// Failures are sorted by (InstancePtr, SchemaPtr, Reason, Keyword) to
// produce deterministic output across runs.
type Failures []Failure

func (fs Failures) Error() string {
	if len(fs) == 0 {
		return "no failures"
	}
	if len(fs) == 1 {
		return fs[0].Error()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d failures:", len(fs))
	for _, f := range fs {
		b.WriteString("\n  ")
		b.WriteString(f.Error())
	}
	return b.String()
}

// Sorted returns a copy of the failures sorted deterministically.
func (fs Failures) Sorted() Failures {
	out := make(Failures, len(fs))
	copy(out, fs)
	// Sort by instance ptr, then schema ptr, then reason, then keyword
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			if compareFailure(out[j], out[j-1]) < 0 {
				out[j], out[j-1] = out[j-1], out[j]
			}
		}
	}
	return out
}

func compareFailure(a, b Failure) int {
	if a.InstancePtr != b.InstancePtr {
		if a.InstancePtr < b.InstancePtr {
			return -1
		}
		return 1
	}
	if a.SchemaPtr != b.SchemaPtr {
		if a.SchemaPtr < b.SchemaPtr {
			return -1
		}
		return 1
	}
	if a.Reason != b.Reason {
		if a.Reason < b.Reason {
			return -1
		}
		return 1
	}
	if a.Keyword != b.Keyword {
		if a.Keyword < b.Keyword {
			return -1
		}
		return 1
	}
	return 0
}

// Has reports whether any failure matches the given reason code.
func (fs Failures) Has(reason ReasonCode) bool {
	for _, f := range fs {
		if f.Reason == reason {
			return true
		}
	}
	return false
}

// CompileError wraps one or more compilation failures.
type CompileError struct {
	Failures Failures
}

func (e *CompileError) Error() string {
	return e.Failures.Error()
}

func (e *CompileError) Unwrap() error {
	if len(e.Failures) == 1 {
		return nil
	}
	return nil
}

// ValidationError wraps one or more validation failures.
type ValidationError struct {
	Failures Failures
}

func (e *ValidationError) Error() string {
	return e.Failures.Error()
}

// decodeJSONStrict decodes JSON bytes and rejects duplicate object keys.
// It uses a custom decoder that tracks key uniqueness.
func decodeJSONStrict(data []byte) (any, error) {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	value, err := decodeJSONValue(dec, 0)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		if err != nil {
			return nil, newJSONDecodeError(ReasonCompileInvalidJSON, err.Error())
		}
		return nil, newJSONDecodeError(ReasonCompileTrailingData, "trailing data after JSON document")
	}
	return value, nil
}

type jsonDecodeError struct {
	reason  ReasonCode
	message string
}

func (e *jsonDecodeError) Error() string {
	return fmt.Sprintf("%s: %s", e.reason, e.message)
}

func newJSONDecodeError(reason ReasonCode, message string) error {
	return &jsonDecodeError{reason: reason, message: message}
}

func decodeReason(err error) ReasonCode {
	if decodeErr, ok := err.(*jsonDecodeError); ok {
		return decodeErr.reason
	}
	return ReasonCompileInvalidJSON
}

func decodeJSONValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > constants.OSCALValidatorMaxDepth {
		return nil, newJSONDecodeError(ReasonCompileResourceLimit, fmt.Sprintf("JSON nesting depth exceeds limit %d", constants.OSCALValidatorMaxDepth))
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, newJSONDecodeError(ReasonCompileInvalidJSON, err.Error())
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return token, nil
	}
	switch delimiter {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			if len(object) >= constants.OSCALValidatorMaxProperties {
				return nil, newJSONDecodeError(ReasonCompileResourceLimit, fmt.Sprintf("object property count exceeds limit %d", constants.OSCALValidatorMaxProperties))
			}
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, newJSONDecodeError(ReasonCompileInvalidJSON, err.Error())
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, newJSONDecodeError(ReasonCompileInvalidJSON, "object key is not a string")
			}
			if _, exists := object[key]; exists {
				return nil, newJSONDecodeError(ReasonCompileDuplicateKey, fmt.Sprintf("duplicate object key %q", key))
			}
			value, err := decodeJSONValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		if _, err := decoder.Token(); err != nil {
			return nil, newJSONDecodeError(ReasonCompileInvalidJSON, err.Error())
		}
		return object, nil
	case '[':
		array := make([]any, 0)
		for decoder.More() {
			if len(array) >= constants.OSCALValidatorMaxItems {
				return nil, newJSONDecodeError(ReasonCompileResourceLimit, fmt.Sprintf("array item count exceeds limit %d", constants.OSCALValidatorMaxItems))
			}
			value, err := decodeJSONValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, newJSONDecodeError(ReasonCompileInvalidJSON, err.Error())
		}
		return array, nil
	default:
		return nil, newJSONDecodeError(ReasonCompileInvalidJSON, fmt.Sprintf("unexpected JSON delimiter %q", delimiter))
	}
}

func checkDuplicateKeys(v any, path string) error {
	switch val := v.(type) {
	case map[string]any:
		// json.Decode already handles duplicate keys by overwriting,
		// so we need a different approach. We use a raw decode to detect
		// duplicates. However, since the standard library doesn't expose
		// this, we skip duplicate-key detection at decode time and
		// handle it in the validator's raw token stream.
		// For now, recurse into values.
		for k, sub := range val {
			if err := checkDuplicateKeys(sub, path+"/"+escapeJSONPointerToken(k)); err != nil {
				return err
			}
		}
	case []any:
		for i, item := range val {
			if err := checkDuplicateKeys(item, fmt.Sprintf("%s/%d", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}
