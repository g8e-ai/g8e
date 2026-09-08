// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version  2.0.

package jsonschema

import (
	"regexp"
)

// Schema is an immutable compiled Draft-07 schema node. It is produced by
// the Compiler and consumed by the Validator. A Schema is either a boolean
// schema (true = accept all, false = reject all) or an object schema with
// typed fields for each supported keyword.
type Schema struct {
	// Boolean is set when the schema is a boolean schema. True means
	// "accept any value" and false means "reject any value".
	Boolean *bool

	// ID is the $id of this schema, if any. It establishes a new reference
	// scope for $ref resolution.
	ID string

	// Type is the asserted JSON type, if any. Empty means no type assertion.
	Type string

	// Enum is the set of allowed values, if any.
	Enum []any

	// Const is the required constant value, if any.
	Const    any
	HasConst bool

	// Properties maps property names to their subschemas.
	Properties map[string]*Schema

	// PatternProperties maps regex patterns to subschemas.
	PatternProperties map[*regexp.Regexp]*Schema

	// AdditionalProperties is the schema for properties not in Properties
	// or PatternProperties. nil means no constraint. A boolean false means
	// reject. A schema means validate against that schema.
	AdditionalProperties     *Schema
	AdditionalPropertiesBool *bool

	// PropertyNames is the schema that every property name must validate
	// against, if any.
	PropertyNames *Schema

	// Required is the list of required property names.
	Required []string

	// Dependencies maps property names to either a list of required
	// properties (if the named property is present) or a subschema.
	Dependencies map[string]any

	// MinProperties / MaxProperties
	MinProperties *int
	MaxProperties *int

	// Items is the schema for array items (single schema form).
	Items *Schema

	// AdditionalItems is the schema for items beyond those covered by
	// tuple-form Items. nil means no constraint.
	AdditionalItems     *Schema
	AdditionalItemsBool *bool

	// Contains is the schema that at least one array item must match.
	Contains *Schema

	// MinItems / MaxItems
	MinItems *int
	MaxItems *int

	// UniqueItems asserts that array items must be unique.
	UniqueItems bool

	// MinLength / MaxLength for strings
	MinLength *int
	MaxLength *int

	// Pattern is the compiled regular expression for string validation.
	Pattern *regexp.Regexp

	// Format is the string format assertion, if any.
	Format Format

	// Numeric constraints
	Minimum          *float64
	Maximum          *float64
	ExclusiveMinimum *float64
	ExclusiveMaximum *float64
	MultipleOf       *float64

	// Conditional subschemas
	AllOf []*Schema
	AnyOf []*Schema
	OneOf []*Schema
	Not   *Schema
	If    *Schema
	Then  *Schema
	Else  *Schema

	// Ref is the resolved reference target, if this schema is a $ref.
	// A schema with a $ref is equivalent to its target; all other keywords
	// in the same object are ignored per Draft-07.
	Ref *Schema

	// RefPath is the original $ref string, for error reporting.
	RefPath string

	// SchemaLocation is the JSON Pointer to this schema within the root
	// document, for error reporting.
	SchemaLocation string
}

// IsBoolean reports whether this schema is a boolean schema.
func (s *Schema) IsBoolean() bool {
	return s != nil && s.Boolean != nil
}

// AcceptsAll reports whether this schema accepts any value (boolean true).
func (s *Schema) AcceptsAll() bool {
	return s != nil && s.Boolean != nil && *s.Boolean
}

// RejectsAll reports whether this schema rejects any value (boolean false).
func (s *Schema) RejectsAll() bool {
	return s != nil && s.Boolean != nil && !*s.Boolean
}

// IsRef reports whether this schema is a $ref schema.
func (s *Schema) IsRef() bool {
	return s != nil && s.Ref != nil
}

// Resolved returns the effective schema, following $ref chains.
func (s *Schema) Resolved() *Schema {
	if s == nil {
		return nil
	}
	depth := 0
	for s.IsRef() && depth < 64 {
		s = s.Ref
		depth++
	}
	return s
}
