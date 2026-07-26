// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mcp

import (
	"encoding/json"
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

// ParseFieldPath extracts a field value from a JSON document using dot notation
func ParseFieldPath(document json.RawMessage, fieldPath string) (FieldValue, error) {
	if document == nil {
		return FieldValue{}, constants.ErrFieldPathDocumentNil
	}

	if fieldPath == "" {
		return FieldValue{}, constants.ErrFieldPathEmpty
	}

	// Parse the document into a generic map
	var docMap map[string]interface{}
	if err := json.Unmarshal(document, &docMap); err != nil {
		return FieldValue{}, fmt.Errorf("field_parser: parse document JSON: %w", err)
	}

	// Navigate the path
	components := strings.Split(fieldPath, ".")
	var current interface{} = docMap

	for _, component := range components {
		switch v := current.(type) {
		case map[string]interface{}:
			var ok bool
			current, ok = v[component]
			if !ok {
				return FieldValue{}, fmt.Errorf("field_parser: %w: %q", constants.ErrFieldPathComponentNotFound, component)
			}
		default:
			return FieldValue{}, fmt.Errorf("field_parser: %w: %q", constants.ErrFieldPathNotObject, component)
		}
	}

	return convertToFieldValue(current), nil
}

// ConvertToFieldValue converts an interface{} value (e.g., from JSON unmarshal
// or DocumentStoreService.GetField) to a typed FieldValue. Exported so the
// document store layer can use it at the package boundary.
func ConvertToFieldValue(val interface{}) FieldValue {
	return convertToFieldValue(val)
}

// convertToFieldValue converts an interface{} value to a typed FieldValue.
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
