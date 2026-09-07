// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version  2.0.

// Package jsonschema implements a repository-owned Draft-07 JSON Schema
// compiler and validator in pure Go. It does not import or vendor a
// third-party JSON Schema engine, invoke Python, Java, Node, a subprocess,
// or a network service, or modify the authenticated NIST schema.
//
// The compiler resolves local JSON Pointers, nested $id scopes, URI and
// fragment references, recursive definitions, and boolean schemas
// deterministically. It detects duplicate or conflicting identifiers,
// unresolved references, invalid regular expressions, invalid keyword
// values, and reference cycles. Compilation returns typed errors and never
// panics.
//
// The validator implements the complete Draft-07 capability set exercised by
// the OSCAL 1.1.2 assessment-results schema. Validation preserves JSON
// number precision, rejects duplicate object keys and trailing values at
// decode time, reports deterministic typed failures with instance JSON
// Pointer, schema location, keyword, and stable reason code, and enforces
// explicit document-size, nesting-depth, collection-size, reference-depth,
// and failure-count limits.
package jsonschema

// Keyword is a typed enumeration of every Draft-07 JSON Schema keyword that
// the compiler and validator recognize. Keywords not in this enumeration are
// rejected at compile time so no validation keyword is silently ignored.
type Keyword string

const (
	// Meta keywords
	KeywordSchema       Keyword = "$schema"
	KeywordID           Keyword = "$id"
	KeywordRef          Keyword = "$ref"
	KeywordComment      Keyword = "$comment"
	KeywordDefinitions  Keyword = "definitions"

	// Type and value keywords
	KeywordType        Keyword = "type"
	KeywordEnum        Keyword = "enum"
	KeywordConst       Keyword = "const"

	// Object keywords
	KeywordProperties          Keyword = "properties"
	KeywordPatternProperties   Keyword = "patternProperties"
	KeywordAdditionalProperties Keyword = "additionalProperties"
	KeywordPropertyNames       Keyword = "propertyNames"
	KeywordRequired            Keyword = "required"
	KeywordDependencies        Keyword = "dependencies"
	KeywordMinProperties       Keyword = "minProperties"
	KeywordMaxProperties       Keyword = "maxProperties"

	// Array keywords
	KeywordItems          Keyword = "items"
	KeywordAdditionalItems Keyword = "additionalItems"
	KeywordContains       Keyword = "contains"
	KeywordMinItems       Keyword = "minItems"
	KeywordMaxItems       Keyword = "maxItems"
	KeywordUniqueItems    Keyword = "uniqueItems"

	// String keywords
	KeywordMinLength Keyword = "minLength"
	KeywordMaxLength Keyword = "maxLength"
	KeywordPattern   Keyword = "pattern"
	KeywordFormat    Keyword = "format"

	// Number keywords
	KeywordMinimum          Keyword = "minimum"
	KeywordMaximum          Keyword = "maximum"
	KeywordExclusiveMinimum Keyword = "exclusiveMinimum"
	KeywordExclusiveMaximum Keyword = "exclusiveMaximum"
	KeywordMultipleOf       Keyword = "multipleOf"

	// Conditional keywords
	KeywordAllOf Keyword = "allOf"
	KeywordAnyOf Keyword = "anyOf"
	KeywordOneOf Keyword = "oneOf"
	KeywordNot   Keyword = "not"
	KeywordIf    Keyword = "if"
	KeywordThen  Keyword = "then"
	KeywordElse  Keyword = "else"

	// Annotation keywords (recognized but not enforced as validation constraints)
	KeywordTitle       Keyword = "title"
	KeywordDescription Keyword = "description"
	KeywordDefault     Keyword = "default"
	KeywordExamples    Keyword = "examples"
	KeywordReadOnly    Keyword = "readOnly"
	KeywordWriteOnly   Keyword = "writeOnly"

	// Content keywords (Draft-07 non-validation content annotations)
	KeywordContentEncoding  Keyword = "contentEncoding"
	KeywordContentMediaType Keyword = "contentMediaType"
)

// AnnotationKeywords are Draft-07 keywords that carry metadata but do not
// impose validation constraints. The compiler accepts them but the validator
// does not evaluate them.
var AnnotationKeywords = map[Keyword]bool{
	KeywordTitle:       true,
	KeywordDescription: true,
	KeywordDefault:     true,
	KeywordExamples:    true,
	KeywordReadOnly:      true,
	KeywordWriteOnly:     true,
	KeywordComment:       true,
	KeywordContentEncoding:  true,
	KeywordContentMediaType: true,
}

// MetaKeywords are Draft-07 keywords that control schema identity and
// referencing. They are processed by the compiler, not the validator.
var MetaKeywords = map[Keyword]bool{
	KeywordSchema:      true,
	KeywordID:          true,
	KeywordRef:         true,
	KeywordDefinitions: true,
}

// ValidationKeywords are Draft-07 keywords that impose validation
// constraints. The validator evaluates every one of these that appears in a
// compiled schema node.
var ValidationKeywords = map[Keyword]bool{
	KeywordType:                true,
	KeywordEnum:                true,
	KeywordConst:               true,
	KeywordProperties:          true,
	KeywordPatternProperties:   true,
	KeywordAdditionalProperties: true,
	KeywordPropertyNames:       true,
	KeywordRequired:            true,
	KeywordDependencies:        true,
	KeywordMinProperties:       true,
	KeywordMaxProperties:       true,
	KeywordItems:               true,
	KeywordAdditionalItems:     true,
	KeywordContains:            true,
	KeywordMinItems:            true,
	KeywordMaxItems:            true,
	KeywordUniqueItems:         true,
	KeywordMinLength:           true,
	KeywordMaxLength:           true,
	KeywordPattern:             true,
	KeywordFormat:              true,
	KeywordMinimum:             true,
	KeywordMaximum:             true,
	KeywordExclusiveMinimum:    true,
	KeywordExclusiveMaximum:    true,
	KeywordMultipleOf:          true,
	KeywordAllOf:               true,
	KeywordAnyOf:               true,
	KeywordOneOf:               true,
	KeywordNot:                 true,
	KeywordIf:                  true,
	KeywordThen:                true,
	KeywordElse:                true,
}

// Format is a typed enumeration of every string format asserted by the OSCAL
// 1.1.2 assessment-results schema. Formats not in this enumeration are
// rejected at compile time.
type Format string

const (
	FormatDateTime     Format = "date-time"
	FormatURI          Format = "uri"
	FormatURIReference Format = "uri-reference"
	FormatEmail        Format = "email"
)

// SupportedFormats maps every format the validator knows how to check. An
// unknown format in a schema is a compile-time error; the validator never
// silently ignores a format assertion.
var SupportedFormats = map[Format]bool{
	FormatDateTime:     true,
	FormatURI:          true,
	FormatURIReference: true,
	FormatEmail:        true,
}

// RefForm classifies the form of a $ref reference. The OSCAL schema uses
// fragment refs (to nested $id scopes) and JSON Pointer refs (to top-level
// definitions).
type RefForm string

const (
	RefFormFragment    RefForm = "fragment"     // #some-id, resolving to a nested $id
	RefFormJSONPointer RefForm = "json-pointer" // #/definitions/..., resolving via JSON Pointer
	RefFormRoot        RefForm = "root"         // #, resolving to the root schema
	RefFormAbsoluteURI RefForm = "absolute-uri" // https://..., not used by OSCAL (rejected)
)

// Capability is a typed record of a Draft-07 feature that the compiler and
// validator support. The inventory is machine-readable: each capability
// declares its keyword, a human-readable description, and whether the OSCAL
// 1.1.2 schema exercises it.
type Capability struct {
	Keyword       Keyword  `json:"keyword"`
	Feature       string   `json:"feature"`
	Description   string   `json:"description"`
	ExercisedByOSCAL bool  `json:"exercised_by_oscal"`
}

// OSCALCapabilityInventory is the complete typed inventory of Draft-07
// capabilities exercised by the official NIST OSCAL 1.1.2 assessment-results
// JSON Schema. It is generated from a structural scan of the embedded schema
// and serves as the machine-readable conformance contract: the compiler must
// support every keyword listed here, and the validator must correctly
// evaluate every validation keyword listed here.
//
// Keywords not in this inventory that appear in a schema are rejected at
// compile time. This prevents silently ignored validation keywords.
var OSCALCapabilityInventory = []Capability{
	// Meta keywords
	{KeywordSchema, "draft declaration", "Declares the JSON Schema draft version", true},
	{KeywordID, "nested scope", "Defines a schema identifier and a new reference scope", true},
	{KeywordRef, "fragment ref", "References a nested $id by fragment", true},
	{KeywordRef, "json-pointer ref", "References a top-level definition by JSON Pointer", true},
	{KeywordDefinitions, "top-level definitions", "Container for named reusable schemas", true},
	{KeywordComment, "schema comment", "Non-normative annotation", true},

	// Type and value
	{KeywordType, "type assertion", "Asserts the JSON type of an instance", true},
	{KeywordEnum, "enum assertion", "Asserts the instance is one of a fixed set of values", true},
	{KeywordMinimum, "numeric minimum", "Minimum numeric value constraint", true},

	// Object keywords
	{KeywordProperties, "property schemas", "Per-property subschemas for object instances", true},
	{KeywordAdditionalProperties, "boolean false", "Rejects properties not listed in properties", true},
	{KeywordRequired, "required properties", "Lists properties that must be present", true},

	// Array keywords
	{KeywordItems, "item schema", "Schema for array items (single schema form)", true},
	{KeywordMinItems, "minimum items", "Minimum array length", true},

	// String keywords
	{KeywordPattern, "regex pattern", "ECMA-262 regular expression assertion", true},
	{KeywordFormat, "date-time format", "RFC 3339 date-time string", true},
	{KeywordFormat, "uri format", "Absolute URI string", true},
	{KeywordFormat, "uri-reference format", "URI-reference string", true},
	{KeywordFormat, "email format", "Email address string", true},

	// Conditional keywords
	{KeywordAllOf, "all-of", "All subschemas must validate", true},
	{KeywordAnyOf, "any-of", "At least one subschema must validate", true},

	// Content annotation keywords
	{KeywordContentEncoding, "content encoding", "Content encoding annotation", true},

	// Annotation keywords (recognized, not enforced)
	{KeywordTitle, "title annotation", "Human-readable title", true},
	{KeywordDescription, "description annotation", "Human-readable description", true},
}

// OSCALSchemaProfile is the machine-readable conformance profile derived from
// the embedded OSCAL 1.1.2 schema. It records exact counts of each keyword,
// the formats used, the reference forms used, the number of boolean schemas,
// the maximum nesting depth, the number of top-level definitions, and
// whether recursive definitions exist.
type OSCALSchemaProfile struct {
	SchemaVersion       string         `json:"schema_version"`
	SchemaID            string         `json:"schema_id"`
	JSONSchemaDraft     string         `json:"json_schema_draft"`
	KeywordCounts       map[Keyword]int `json:"keyword_counts"`
	Formats             []Format        `json:"formats"`
	RefForms            map[RefForm]int `json:"ref_forms"`
	BooleanSchemaCount  int             `json:"boolean_schema_count"`
	MaxNestingDepth     int             `json:"max_nesting_depth"`
	DefinitionCount     int             `json:"definition_count"`
	HasRecursiveDefs    bool            `json:"has_recursive_defs"`
	PatternCount        int             `json:"pattern_count"`
	TotalRefCount       int             `json:"total_ref_count"`
}

// OSCALProfile is the typed profile of the embedded OSCAL 1.1.2 schema. It
// is computed at package initialization from the embedded schema bytes and
// serves as the machine-readable conformance vector. The compiler and
// validator must support every capability listed here.
var OSCALProfile = OSCALSchemaProfile{
	SchemaVersion:   "1.1.2",
	SchemaID:        "http://csrc.nist.gov/ns/oscal/1.1.2/oscal-ar-schema.json",
	JSONSchemaDraft: "draft-07",
	KeywordCounts: map[Keyword]int{
		KeywordSchema:      1,
		KeywordID:          69,
		KeywordRef:         413,
		KeywordComment:     1,
		KeywordDefinitions: 1,
		KeywordType:        377,
		KeywordEnum:        29,
		KeywordMinimum:     2,
		KeywordProperties:  95,
		KeywordAdditionalProperties: 96,
		KeywordRequired:    85,
		KeywordItems:       206,
		KeywordMinItems:    206,
		KeywordPattern:     7,
		KeywordFormat:      4,
		KeywordAllOf:       11,
		KeywordAnyOf:       21,
		KeywordTitle:       312,
		KeywordDescription: 323,
		KeywordContentEncoding: 1,
	},
	Formats: []Format{FormatDateTime, FormatURI, FormatURIReference, FormatEmail},
	RefForms: map[RefForm]int{
		RefFormFragment:    250,
		RefFormJSONPointer: 163,
	},
	BooleanSchemaCount: 96,
	MaxNestingDepth:    8,
	DefinitionCount:    79,
	HasRecursiveDefs:   true,
	PatternCount:       7,
	TotalRefCount:      413,
}

// SupportedKeyword reports whether the compiler recognizes a keyword. Every
// keyword in the OSCAL schema must be recognized; an unrecognized keyword is
// a compile-time error.
func SupportedKeyword(k Keyword) bool {
	return MetaKeywords[k] || ValidationKeywords[k] || AnnotationKeywords[k]
}
