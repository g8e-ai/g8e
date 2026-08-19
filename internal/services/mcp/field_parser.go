// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
)

// FieldPathRegistry holds the schema registry for allowed field paths per collection
type FieldPathRegistry struct {
	logger   *slog.Logger
	registry map[string]CollectionFieldPaths
}

// CollectionFieldPaths defines allowed and forbidden paths for a collection
type CollectionFieldPaths struct {
	AllowedPaths   []string `json:"allowed_paths"`
	ForbiddenPaths []string `json:"forbidden_paths"`
}

// NewFieldPathRegistry loads the field path schema from the constants package
func NewFieldPathRegistry(logger *slog.Logger) (*FieldPathRegistry, error) {
	registry := &FieldPathRegistry{
		logger:   logger,
		registry: make(map[string]CollectionFieldPaths),
	}

	for collection, config := range getFieldPaths() {
		registry.registry[collection] = config
	}

	logger.Info("loaded field path registry from constants", "collections", len(registry.registry))
	return registry, nil
}

// ValidateFieldPath checks if a field path is allowed for a given collection
func (r *FieldPathRegistry) ValidateFieldPath(collection string, fieldPath string) error {
	if collection == "" {
		return constants.ErrFieldPathEmptyCollection
	}

	if fieldPath == "" {
		return constants.ErrFieldPathEmpty
	}

	// Check if collection exists in registry
	collectionPaths, exists := r.registry[collection]
	if !exists {
		return constants.ErrFieldPathInvalidCollection
	}

	// Parse the field path into components
	components := strings.Split(fieldPath, ".")
	if len(components) == 0 {
		return constants.ErrFieldPathSyntax
	}

	// Check each component for forbidden patterns
	for _, component := range components {
		if component == "" {
			return constants.ErrFieldPathSyntax
		}
	}

	// Check if any component or prefix is in the forbidden list
	for _, forbidden := range collectionPaths.ForbiddenPaths {
		for _, component := range components {
			if component == forbidden {
				return constants.ErrFieldPathForbidden
			}
		}
		if strings.HasPrefix(fieldPath, forbidden+".") || fieldPath == forbidden {
			return constants.ErrFieldPathForbidden
		}
	}

	// Check if the field path is in the allowed list
	allowed := false
	for _, allowedPath := range collectionPaths.AllowedPaths {
		if fieldPath == allowedPath || strings.HasPrefix(fieldPath, allowedPath+".") {
			allowed = true
			break
		}
	}

	if !allowed {
		return constants.ErrFieldPathInvalid
	}

	return nil
}

// ConvertToFieldValue converts an interface{} value (e.g., from JSON unmarshal
// or DocumentStoreService.GetField) to a typed FieldValue. Exported so the
// document store layer can use it at the package boundary.
func ConvertToFieldValue(val interface{}) FieldValue {
	return convertToFieldValue(val)
}

// convertToFieldValue converts an interface{} value to a typed FieldValue.
// The []interface{} and map[string]interface{} type-switch cases walk arbitrary
// JSON decoded via json.Unmarshal from external sources (document store fields,
// cloud metadata, command outputs) where the schema is unknown by design. This
// is the same schema-less passthrough exception as ScrubMap/scrubSlice/
// RehydratePayload in internal/services/scrubbing/boundary.go: a typed model
// cannot represent an externally-defined JSON shape, so rule 3's "no
// map[string]interface{} for known shapes" does not apply here. The output is
// the typed FieldValue, so callers receive a typed model regardless of input.
func convertToFieldValue(val interface{}) FieldValue {
	if val == nil {
		return FieldValue{Null: true}
	}

	switch v := val.(type) {
	case string:
		return FieldValue{Str: &v}
	case float64:
		return FieldValue{Float64: &v}
	case bool:
		return FieldValue{Bool: &v}
	case []interface{}:
		arr := make([]FieldValue, len(v))
		for i, item := range v {
			arr[i] = convertToFieldValue(item)
		}
		return FieldValue{Array: arr}
	case map[string]interface{}:
		obj := make(map[string]FieldValue)
		for key, item := range v {
			obj[key] = convertToFieldValue(item)
		}
		return FieldValue{Object: obj}
	default:
		// Fallback to string representation for unknown types
		s := fmt.Sprintf("%v", v)
		return FieldValue{Str: &s}
	}
}
