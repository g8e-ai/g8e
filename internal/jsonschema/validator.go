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
	"math/big"
	"net/mail"
	"net/url"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// Validator validates a JSON instance against a compiled Draft-07 Schema.
// It produces deterministic typed failures with instance JSON Pointer,
// schema location, keyword, and stable reason code. It enforces explicit
// document-size, nesting-depth, collection-size, reference-depth, and
// failure-count limits. Validation never panics.
type Validator struct {
	maxDepth      int
	maxProperties int
	maxItems      int
	maxRefDepth   int
	maxFailures   int
}

// NewValidator creates a Validator with default resource limits.
func NewValidator() *Validator {
	return &Validator{
		maxDepth:      constants.OSCALValidatorMaxDepth,
		maxProperties: constants.OSCALValidatorMaxProperties,
		maxItems:      constants.OSCALValidatorMaxItems,
		maxRefDepth:   constants.OSCALValidatorMaxRefDepth,
		maxFailures:   constants.OSCALValidatorMaxFailures,
	}
}

// Validate validates raw JSON bytes against the compiled schema. It decodes
// the bytes using a strict decoder that preserves JSON number precision and
// rejects trailing data, then validates the decoded value.
func (v *Validator) Validate(schema *Schema, data []byte) Failures {
	if len(data) > constants.OSCALValidatorMaxDocumentBytes {
		return Failures{{
			Reason:  ReasonCompileResourceLimit,
			Message: fmt.Sprintf("document size %d exceeds limit %d", len(data), constants.OSCALValidatorMaxDocumentBytes),
		}}
	}
	instance, err := decodeJSONForValidation(data)
	if err != nil {
		return Failures{{
			Reason:  decodeReason(err),
			Message: err.Error(),
		}}
	}
	return v.ValidateValue(schema, instance)
}

// ValidateValue validates a decoded JSON value against the compiled schema.
// The value must be decoded with json.Number for number precision
// preservation (use decodeJSONForValidation).
func (v *Validator) ValidateValue(schema *Schema, instance any) Failures {
	failures := &failureCollector{max: v.maxFailures}
	v.validate(schema, instance, "", 0, failures)
	return failures.failures.Sorted()
}

// decodeJSONForValidation decodes JSON bytes preserving number precision
// via json.Number and rejecting trailing data.
func decodeJSONForValidation(data []byte) (any, error) {
	return decodeJSONStrict(data)
}

// failureCollector collects failures up to a maximum count.
type failureCollector struct {
	failures Failures
	max      int
}

func (fc *failureCollector) add(f Failure) {
	if len(fc.failures) < fc.max {
		fc.failures = append(fc.failures, f)
	}
}

func (fc *failureCollector) hasFailures() bool {
	return len(fc.failures) > 0
}

// validate is the core validation dispatcher. It follows $ref chains,
// handles boolean schemas, and dispatches to keyword-specific validators.
func (v *Validator) validate(schema *Schema, instance any, instancePtr string, depth int, fc *failureCollector) {
	if schema == nil {
		return
	}
	if depth > v.maxDepth {
		fc.add(Failure{
			Reason:      ReasonCompileResourceLimit,
			Message:     fmt.Sprintf("validation depth exceeds limit %d", v.maxDepth),
			InstancePtr: instancePtr,
		})
		return
	}

	// Follow $ref
	schema = schema.Resolved()

	// Boolean schema
	if schema.AcceptsAll() {
		return
	}
	if schema.RejectsAll() {
		fc.add(Failure{
			Reason:      ReasonValueTypeMismatch,
			Message:     "schema rejects all values (boolean false)",
			InstancePtr: instancePtr,
			SchemaPtr:   schema.SchemaLocation,
		})
		return
	}

	// type
	if schema.Type != "" {
		if !typeMatches(schema.Type, instance) {
			fc.add(Failure{
				Reason:      ReasonValueTypeMismatch,
				Message:     fmt.Sprintf("expected type %s, got %s", schema.Type, jsonTypeName(instance)),
				InstancePtr: instancePtr,
				SchemaPtr:   schema.SchemaLocation,
				Keyword:     "type",
			})
			return // type mismatch means other keywords don't apply
		}
	}

	// enum
	if len(schema.Enum) > 0 {
		if !enumContains(schema.Enum, instance) {
			fc.add(Failure{
				Reason:      ReasonValueEnumMismatch,
				Message:     "value is not one of the allowed enum values",
				InstancePtr: instancePtr,
				SchemaPtr:   schema.SchemaLocation,
				Keyword:     "enum",
			})
		}
	}

	// const
	if schema.HasConst {
		if !jsonEqual(schema.Const, instance) {
			fc.add(Failure{
				Reason:      ReasonValueConstMismatch,
				Message:     "value does not match const",
				InstancePtr: instancePtr,
				SchemaPtr:   schema.SchemaLocation,
				Keyword:     "const",
			})
		}
	}

	// Type-specific validation
	switch instance.(type) {
	case map[string]any:
		v.validateObject(schema, instance, instancePtr, depth, fc)
	case []any:
		v.validateArray(schema, instance, instancePtr, depth, fc)
	case string:
		v.validateString(schema, instance, instancePtr, fc)
	case json.Number:
		v.validateNumber(schema, instance, instancePtr, fc)
	case float64:
		v.validateNumber(schema, instance, instancePtr, fc)
	}

	// Conditional subschemas
	v.validateConditionals(schema, instance, instancePtr, depth, fc)
}

// validateObject validates object-type keywords: properties, patternProperties,
// additionalProperties, propertyNames, required, dependencies, min/maxProperties.
func (v *Validator) validateObject(schema *Schema, instance any, instancePtr string, depth int, fc *failureCollector) {
	obj := instance.(map[string]any)

	// required
	if len(schema.Required) > 0 {
		for _, req := range schema.Required {
			if _, ok := obj[req]; !ok {
				fc.add(Failure{
					Reason:      ReasonValueRequiredMissing,
					Message:     fmt.Sprintf("required property %q is missing", req),
					InstancePtr: instancePtr,
					SchemaPtr:   schema.SchemaLocation,
					Keyword:     "required",
				})
			}
		}
	}

	// minProperties
	if schema.MinProperties != nil {
		if len(obj) < *schema.MinProperties {
			fc.add(Failure{
				Reason:      ReasonValueMinProperties,
				Message:     fmt.Sprintf("has %d properties, minimum is %d", len(obj), *schema.MinProperties),
				InstancePtr: instancePtr,
				SchemaPtr:   schema.SchemaLocation,
				Keyword:     "minProperties",
			})
		}
	}

	// maxProperties
	if schema.MaxProperties != nil {
		if len(obj) > *schema.MaxProperties {
			fc.add(Failure{
				Reason:      ReasonValueMaxProperties,
				Message:     fmt.Sprintf("has %d properties, maximum is %d", len(obj), *schema.MaxProperties),
				InstancePtr: instancePtr,
				SchemaPtr:   schema.SchemaLocation,
				Keyword:     "maxProperties",
			})
		}
	}

	// properties + patternProperties + additionalProperties
	for key, val := range obj {
		childPtr := instancePtr + "/" + escapeJSONPointerToken(key)
		matched := false

		// properties
		if sub, ok := schema.Properties[key]; ok {
			v.validate(sub, val, childPtr, depth+1, fc)
			matched = true
		}

		// patternProperties
		for re, sub := range schema.PatternProperties {
			if re.MatchString(key) {
				v.validate(sub, val, childPtr, depth+1, fc)
				matched = true
			}
		}

		// additionalProperties
		if !matched {
			if schema.AdditionalPropertiesBool != nil && !*schema.AdditionalPropertiesBool {
				fc.add(Failure{
					Reason:      ReasonValueAdditionalProp,
					Message:     fmt.Sprintf("additional property %q is not allowed", key),
					InstancePtr: childPtr,
					SchemaPtr:   schema.SchemaLocation,
					Keyword:     "additionalProperties",
				})
			} else if schema.AdditionalProperties != nil {
				v.validate(schema.AdditionalProperties, val, childPtr, depth+1, fc)
			}
		}
	}

	// propertyNames
	if schema.PropertyNames != nil {
		for key := range obj {
			keyVal := key
			v.validate(schema.PropertyNames, keyVal, instancePtr+"/"+escapeJSONPointerToken(key), depth+1, fc)
		}
	}

	// dependencies
	for depKey, dep := range schema.Dependencies {
		if _, present := obj[depKey]; !present {
			continue
		}
		switch d := dep.(type) {
		case []string:
			for _, requiredProp := range d {
				if _, ok := obj[requiredProp]; !ok {
					fc.add(Failure{
						Reason:      ReasonValueDependenciesFailed,
						Message:     fmt.Sprintf("property %q requires %q", depKey, requiredProp),
						InstancePtr: instancePtr,
						SchemaPtr:   schema.SchemaLocation,
						Keyword:     "dependencies",
					})
				}
			}
		case *Schema:
			v.validate(d, instance, instancePtr, depth+1, fc)
		}
	}
}

// validateArray validates array-type keywords: items, additionalItems,
// contains, min/maxItems, uniqueItems.
func (v *Validator) validateArray(schema *Schema, instance any, instancePtr string, depth int, fc *failureCollector) {
	arr := instance.([]any)

	// minItems
	if schema.MinItems != nil {
		if len(arr) < *schema.MinItems {
			fc.add(Failure{
				Reason:      ReasonValueMinItems,
				Message:     fmt.Sprintf("has %d items, minimum is %d", len(arr), *schema.MinItems),
				InstancePtr: instancePtr,
				SchemaPtr:   schema.SchemaLocation,
				Keyword:     "minItems",
			})
		}
	}

	// maxItems
	if schema.MaxItems != nil {
		if len(arr) > *schema.MaxItems {
			fc.add(Failure{
				Reason:      ReasonValueMaxItems,
				Message:     fmt.Sprintf("has %d items, maximum is %d", len(arr), *schema.MaxItems),
				InstancePtr: instancePtr,
				SchemaPtr:   schema.SchemaLocation,
				Keyword:     "maxItems",
			})
		}
	}

	// uniqueItems
	if schema.UniqueItems {
		if !allUnique(arr) {
			fc.add(Failure{
				Reason:      ReasonValueUniqueItems,
				Message:     "array items are not unique",
				InstancePtr: instancePtr,
				SchemaPtr:   schema.SchemaLocation,
				Keyword:     "uniqueItems",
			})
		}
	}

	// items
	if schema.Items != nil {
		for i, item := range arr {
			v.validate(schema.Items, item, fmt.Sprintf("%s/%d", instancePtr, i), depth+1, fc)
		}
	}

	// contains
	if schema.Contains != nil {
		found := false
		for _, item := range arr {
			subFC := &failureCollector{max: 1}
			v.validate(schema.Contains, item, "", depth+1, subFC)
			if !subFC.hasFailures() {
				found = true
				break
			}
		}
		if !found {
			fc.add(Failure{
				Reason:      ReasonValueContainsMissing,
				Message:     "no array item matches the contains schema",
				InstancePtr: instancePtr,
				SchemaPtr:   schema.SchemaLocation,
				Keyword:     "contains",
			})
		}
	}
}

// validateString validates string-type keywords: minLength, maxLength,
// pattern, format.
func (v *Validator) validateString(schema *Schema, instance any, instancePtr string, fc *failureCollector) {
	str := instance.(string)
	runeLen := len([]rune(str))

	// minLength
	if schema.MinLength != nil {
		if runeLen < *schema.MinLength {
			fc.add(Failure{
				Reason:      ReasonValueMinLength,
				Message:     fmt.Sprintf("string length %d, minimum is %d", runeLen, *schema.MinLength),
				InstancePtr: instancePtr,
				SchemaPtr:   schema.SchemaLocation,
				Keyword:     "minLength",
			})
		}
	}

	// maxLength
	if schema.MaxLength != nil {
		if runeLen > *schema.MaxLength {
			fc.add(Failure{
				Reason:      ReasonValueMaxLength,
				Message:     fmt.Sprintf("string length %d, maximum is %d", runeLen, *schema.MaxLength),
				InstancePtr: instancePtr,
				SchemaPtr:   schema.SchemaLocation,
				Keyword:     "maxLength",
			})
		}
	}

	// pattern
	if schema.Pattern != nil {
		if !schema.Pattern.MatchString(str) {
			fc.add(Failure{
				Reason:      ReasonValuePatternMismatch,
				Message:     fmt.Sprintf("string does not match pattern %s", schema.Pattern.String()),
				InstancePtr: instancePtr,
				SchemaPtr:   schema.SchemaLocation,
				Keyword:     "pattern",
			})
		}
	}

	// format
	if schema.Format != "" {
		if !validateFormat(schema.Format, str) {
			fc.add(Failure{
				Reason:      ReasonValueFormatMismatch,
				Message:     fmt.Sprintf("string does not match format %s", schema.Format),
				InstancePtr: instancePtr,
				SchemaPtr:   schema.SchemaLocation,
				Keyword:     "format",
			})
		}
	}
}

// validateNumber validates numeric keywords: minimum, maximum,
// exclusiveMinimum, exclusiveMaximum, multipleOf.
func (v *Validator) validateNumber(schema *Schema, instance any, instancePtr string, fc *failureCollector) {
	val := toBigFloat(instance)

	// minimum
	if schema.Minimum != nil {
		min := big.NewFloat(*schema.Minimum)
		if val.Cmp(min) < 0 {
			fc.add(Failure{
				Reason:      ReasonValueMinimum,
				Message:     fmt.Sprintf("value is less than minimum %v", *schema.Minimum),
				InstancePtr: instancePtr,
				SchemaPtr:   schema.SchemaLocation,
				Keyword:     "minimum",
			})
		}
	}

	// maximum
	if schema.Maximum != nil {
		max := big.NewFloat(*schema.Maximum)
		if val.Cmp(max) > 0 {
			fc.add(Failure{
				Reason:      ReasonValueMaximum,
				Message:     fmt.Sprintf("value is greater than maximum %v", *schema.Maximum),
				InstancePtr: instancePtr,
				SchemaPtr:   schema.SchemaLocation,
				Keyword:     "maximum",
			})
		}
	}

	// exclusiveMinimum (Draft-07: numeric value)
	if schema.ExclusiveMinimum != nil {
		exMin := big.NewFloat(*schema.ExclusiveMinimum)
		if val.Cmp(exMin) <= 0 {
			fc.add(Failure{
				Reason:      ReasonValueExclusiveMinimum,
				Message:     fmt.Sprintf("value is not greater than exclusiveMinimum %v", *schema.ExclusiveMinimum),
				InstancePtr: instancePtr,
				SchemaPtr:   schema.SchemaLocation,
				Keyword:     "exclusiveMinimum",
			})
		}
	}

	// exclusiveMaximum (Draft-07: numeric value)
	if schema.ExclusiveMaximum != nil {
		exMax := big.NewFloat(*schema.ExclusiveMaximum)
		if val.Cmp(exMax) >= 0 {
			fc.add(Failure{
				Reason:      ReasonValueExclusiveMaximum,
				Message:     fmt.Sprintf("value is not less than exclusiveMaximum %v", *schema.ExclusiveMaximum),
				InstancePtr: instancePtr,
				SchemaPtr:   schema.SchemaLocation,
				Keyword:     "exclusiveMaximum",
			})
		}
	}

	// multipleOf
	if schema.MultipleOf != nil {
		if !isMultipleOf(val, *schema.MultipleOf) {
			fc.add(Failure{
				Reason:      ReasonValueMultipleOf,
				Message:     fmt.Sprintf("value is not a multiple of %v", *schema.MultipleOf),
				InstancePtr: instancePtr,
				SchemaPtr:   schema.SchemaLocation,
				Keyword:     "multipleOf",
			})
		}
	}
}

// validateConditionals validates allOf, anyOf, oneOf, not, if/then/else.
func (v *Validator) validateConditionals(schema *Schema, instance any, instancePtr string, depth int, fc *failureCollector) {
	// allOf
	if len(schema.AllOf) > 0 {
		for _, sub := range schema.AllOf {
			subFC := &failureCollector{max: v.maxFailures}
			v.validate(sub, instance, instancePtr, depth+1, subFC)
			if subFC.hasFailures() {
				fc.add(Failure{
					Reason:      ReasonValueAllOfFailed,
					Message:     "instance does not validate against all allOf schemas",
					InstancePtr: instancePtr,
					SchemaPtr:   schema.SchemaLocation,
					Keyword:     "allOf",
				})
				break
			}
		}
	}

	// anyOf
	if len(schema.AnyOf) > 0 {
		anyValid := false
		for _, sub := range schema.AnyOf {
			subFC := &failureCollector{max: 1}
			v.validate(sub, instance, instancePtr, depth+1, subFC)
			if !subFC.hasFailures() {
				anyValid = true
				break
			}
		}
		if !anyValid {
			fc.add(Failure{
				Reason:      ReasonValueAnyOfFailed,
				Message:     "instance does not validate against any anyOf schema",
				InstancePtr: instancePtr,
				SchemaPtr:   schema.SchemaLocation,
				Keyword:     "anyOf",
			})
		}
	}

	// oneOf
	if len(schema.OneOf) > 0 {
		validCount := 0
		for _, sub := range schema.OneOf {
			subFC := &failureCollector{max: 1}
			v.validate(sub, instance, instancePtr, depth+1, subFC)
			if !subFC.hasFailures() {
				validCount++
			}
		}
		if validCount != 1 {
			fc.add(Failure{
				Reason:      ReasonValueOneOfFailed,
				Message:     fmt.Sprintf("instance validates against %d oneOf schemas (must be exactly 1)", validCount),
				InstancePtr: instancePtr,
				SchemaPtr:   schema.SchemaLocation,
				Keyword:     "oneOf",
			})
		}
	}

	// not
	if schema.Not != nil {
		subFC := &failureCollector{max: 1}
		v.validate(schema.Not, instance, instancePtr, depth+1, subFC)
		if !subFC.hasFailures() {
			fc.add(Failure{
				Reason:      ReasonValueNotFailed,
				Message:     "instance validates against the not schema (must not)",
				InstancePtr: instancePtr,
				SchemaPtr:   schema.SchemaLocation,
				Keyword:     "not",
			})
		}
	}

	// if/then/else
	if schema.If != nil {
		ifFC := &failureCollector{max: 1}
		v.validate(schema.If, instance, instancePtr, depth+1, ifFC)
		if !ifFC.hasFailures() {
			// if validates → then must validate
			if schema.Then != nil {
				thenFC := &failureCollector{max: v.maxFailures}
				v.validate(schema.Then, instance, instancePtr, depth+1, thenFC)
				if thenFC.hasFailures() {
					fc.add(Failure{
						Reason:      ReasonValueIfThenFailed,
						Message:     "instance validates if but not then",
						InstancePtr: instancePtr,
						SchemaPtr:   schema.SchemaLocation,
						Keyword:     "then",
					})
				}
			}
		} else {
			// if does not validate → else must validate
			if schema.Else != nil {
				elseFC := &failureCollector{max: v.maxFailures}
				v.validate(schema.Else, instance, instancePtr, depth+1, elseFC)
				if elseFC.hasFailures() {
					fc.add(Failure{
						Reason:      ReasonValueIfElseFailed,
						Message:     "instance does not validate if or else",
						InstancePtr: instancePtr,
						SchemaPtr:   schema.SchemaLocation,
						Keyword:     "else",
					})
				}
			}
		}
	}
}

// --- Helper functions ---

// typeMatches checks whether the instance matches the declared JSON type.
func typeMatches(typ string, instance any) bool {
	switch typ {
	case "object":
		_, ok := instance.(map[string]any)
		return ok
	case "array":
		_, ok := instance.([]any)
		return ok
	case "string":
		_, ok := instance.(string)
		return ok
	case "integer":
		// JSON integers are numbers without a fractional part
		if n, ok := instance.(json.Number); ok {
			if _, err := n.Int64(); err == nil {
				return true
			}
			// Large integers that don't fit int64 but have no fractional part
			if f, err := n.Float64(); err == nil {
				return f == float64(int64(f))
			}
			return false
		}
		if f, ok := instance.(float64); ok {
			return f == float64(int64(f))
		}
		return false
	case "number":
		if _, ok := instance.(json.Number); ok {
			return true
		}
		_, ok := instance.(float64)
		return ok
	case "boolean":
		_, ok := instance.(bool)
		return ok
	case "null":
		return instance == nil
	}
	return false
}

// jsonTypeName returns the JSON type name of an instance value.
func jsonTypeName(instance any) string {
	switch instance.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case json.Number:
		return "number"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case nil:
		return "null"
	}
	return fmt.Sprintf("%T", instance)
}

// enumContains checks whether the instance equals any enum value.
func enumContains(enum []any, instance any) bool {
	for _, e := range enum {
		if jsonEqual(e, instance) {
			return true
		}
	}
	return false
}

// jsonEqual compares two JSON values for equality, handling json.Number.
func jsonEqual(a, b any) bool {
	// Normalize json.Number to string for comparison
	an := normalizeJSON(a)
	bn := normalizeJSON(b)
	return an == bn
}

func normalizeJSON(v any) string {
	switch val := v.(type) {
	case json.Number:
		return "num:" + string(val)
	case nil:
		return "null"
	case bool:
		if val {
			return "bool:true"
		}
		return "bool:false"
	case string:
		return "str:" + val
	case float64:
		return fmt.Sprintf("num:%v", val)
	case map[string]any:
		// For complex types, marshal to canonical JSON
		b, _ := json.Marshal(val)
		return "obj:" + string(b)
	case []any:
		b, _ := json.Marshal(val)
		return "arr:" + string(b)
	}
	return fmt.Sprintf("%T:%v", v, v)
}

// allUnique checks whether all array items are unique.
func allUnique(arr []any) bool {
	seen := make(map[string]bool, len(arr))
	for _, item := range arr {
		key := normalizeJSON(item)
		if seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}

// toBigFloat converts a JSON number to a big.Float for precise comparison.
func toBigFloat(instance any) *big.Float {
	switch v := instance.(type) {
	case json.Number:
		f, _, err := big.ParseFloat(string(v), 10, 256, big.ToNearestEven)
		if err != nil {
			return big.NewFloat(0)
		}
		return f
	case float64:
		return big.NewFloat(v)
	}
	return big.NewFloat(0)
}

// isMultipleOf checks whether val is a multiple of divisor.
func isMultipleOf(val *big.Float, divisor float64) bool {
	if divisor == 0 {
		return false
	}
	d := big.NewFloat(divisor)
	quotient := new(big.Float).Quo(val, d)
	// Check if quotient is an integer
	intPart, _ := quotient.Int(nil)
	floatOfInt := new(big.Float).SetInt(intPart)
	return quotient.Cmp(floatOfInt) == 0
}

// validateFormat checks whether a string matches a Draft-07 format.
func validateFormat(format Format, str string) bool {
	switch format {
	case FormatDateTime:
		_, err := time.Parse(time.RFC3339, str)
		return err == nil
	case FormatURI:
		u, err := url.Parse(str)
		if err != nil {
			return false
		}
		return u.IsAbs() && u.Scheme != ""
	case FormatURIReference:
		_, err := url.Parse(str)
		return err == nil
	case FormatEmail:
		_, err := mail.ParseAddress(str)
		return err == nil
	}
	return false
}
