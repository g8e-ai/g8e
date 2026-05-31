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
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
)

var (
	ErrInvalidCollection  = errors.New("FIELD_PATH_INVALID_COLLECTION: collection not in schema registry")
	ErrInvalidFieldPath   = errors.New("FIELD_PATH_INVALID: field path not in allowlist")
	ErrForbiddenFieldPath = errors.New("FIELD_PATH_FORBIDDEN: field path in denylist")
	ErrInvalidPathSyntax  = errors.New("FIELD_PATH_SYNTAX: invalid dot-notation syntax")
	ErrEmptyFieldPath     = errors.New("FIELD_PATH_EMPTY: field path cannot be empty")
	ErrEmptyCollection    = errors.New("FIELD_PATH_EMPTY_COLLECTION: collection cannot be empty")
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

// NewFieldPathRegistry loads the field path schema from the embedded JSON
func NewFieldPathRegistry(logger *slog.Logger) (*FieldPathRegistry, error) {
	// Resolve paths without modifying global state to avoid data races
	projectRoot := constants.ResolveProjectRoot()
	protocolConstantsDir := filepath.Join(projectRoot, "protocol", "constants")

	registry := &FieldPathRegistry{
		logger:   logger,
		registry: make(map[string]CollectionFieldPaths),
	}

	// Load from protocol constants
	if err := registry.loadFromConstants(protocolConstantsDir); err != nil {
		return nil, fmt.Errorf("failed to load field path registry: %w", err)
	}

	return registry, nil
}

// loadFromConstants loads field paths from the protocol constants JSON
func (r *FieldPathRegistry) loadFromConstants(protocolConstantsDir string) error {
	fieldPathsPath := filepath.Join(protocolConstantsDir, "field_paths.json")
	data, err := os.ReadFile(fieldPathsPath)
	if err != nil {
		return fmt.Errorf("failed to read field_paths.json from %s: %w", fieldPathsPath, err)
	}

	var rawJSON map[string]interface{}
	if err := json.Unmarshal(data, &rawJSON); err != nil {
		return fmt.Errorf("failed to parse field_paths.json: %w", err)
	}

	fieldPaths, ok := rawJSON["field_paths"].(map[string]interface{})
	if !ok {
		return errors.New("field_paths.json missing 'field_paths' key or invalid type")
	}

	for collection, pathsData := range fieldPaths {
		collectionMap, ok := pathsData.(map[string]interface{})
		if !ok {
			r.logger.Warn("invalid field path data for collection", "collection", collection)
			continue
		}

		var fieldPaths CollectionFieldPaths

		if allowedPaths, ok := collectionMap["allowed_paths"].([]interface{}); ok {
			for _, path := range allowedPaths {
				if pathStr, ok := path.(string); ok {
					fieldPaths.AllowedPaths = append(fieldPaths.AllowedPaths, pathStr)
				}
			}
		}

		if forbiddenPaths, ok := collectionMap["forbidden_paths"].([]interface{}); ok {
			for _, path := range forbiddenPaths {
				if pathStr, ok := path.(string); ok {
					fieldPaths.ForbiddenPaths = append(fieldPaths.ForbiddenPaths, pathStr)
				}
			}
		}

		r.registry[collection] = fieldPaths
	}

	r.logger.Info("loaded field path registry from protocol constants", "collections", len(r.registry))
	return nil
}

// ValidateFieldPath checks if a field path is allowed for a given collection
func (r *FieldPathRegistry) ValidateFieldPath(collection string, fieldPath string) error {
	if collection == "" {
		return ErrEmptyCollection
	}

	if fieldPath == "" {
		return ErrEmptyFieldPath
	}

	// Check if collection exists in registry
	collectionPaths, exists := r.registry[collection]
	if !exists {
		return ErrInvalidCollection
	}

	// Parse the field path into components
	components := strings.Split(fieldPath, ".")
	if len(components) == 0 {
		return ErrInvalidPathSyntax
	}

	// Check each component for forbidden patterns
	for _, component := range components {
		if component == "" {
			return ErrInvalidPathSyntax
		}
	}

	// Check if any component or prefix is in the forbidden list
	for _, forbidden := range collectionPaths.ForbiddenPaths {
		for _, component := range components {
			if component == forbidden {
				return ErrForbiddenFieldPath
			}
		}
		if strings.HasPrefix(fieldPath, forbidden+".") || fieldPath == forbidden {
			return ErrForbiddenFieldPath
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
		return ErrInvalidFieldPath
	}

	return nil
}

// ParseFieldPath extracts a field value from a JSON document using dot notation
func ParseFieldPath(document json.RawMessage, fieldPath string) (interface{}, error) {
	if document == nil {
		return nil, errors.New("document is nil")
	}

	if fieldPath == "" {
		return nil, ErrEmptyFieldPath
	}

	// Parse the document into a generic map
	var docMap map[string]interface{}
	if err := json.Unmarshal(document, &docMap); err != nil {
		return nil, fmt.Errorf("failed to parse document JSON: %w", err)
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
				return nil, fmt.Errorf("field path component '%s' not found", component)
			}
		default:
			return nil, fmt.Errorf("cannot access field '%s' on non-object type", component)
		}
	}

	return current, nil
}
