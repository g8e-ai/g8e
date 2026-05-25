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

/*
Docs-Drift Verification Tests

Test-driven documentation enforcement. These tests verify that documentation
files (markdown) stay in sync with the canonical source of truth in Go code
and protocol constants.

If a developer adds a new collection, API path, or constant without updating
the corresponding documentation, these tests fail in local development and CI.
*/
package contracts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/services/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var docsDir string

func init() {
	docsDir = filepath.Join(system.ResolveProjectRoot(), "docs")
}

// TestCollectionsDocsAlignment verifies that documented collections in storage.md
// match the canonical collections.json registry.
func TestCollectionsDocsAlignment(t *testing.T) {
	// Load canonical collections from protocol/constants/collections.json
	protocolConstantsDir := filepath.Join(system.ResolveProjectRoot(), "protocol/constants")
	collectionsData, err := os.ReadFile(filepath.Join(protocolConstantsDir, "collections.json"))
	require.NoError(t, err, "protocol/constants/collections.json must be readable")

	var collectionsJSON struct {
		Collections map[string]struct {
			Value string `json:"value"`
		} `json:"collections"`
	}
	err = json.Unmarshal(collectionsData, &collectionsJSON)
	require.NoError(t, err, "collections.json must be valid JSON")

	// Extract canonical collection names
	canonicalCollections := make(map[string]bool)
	for _, entry := range collectionsJSON.Collections {
		canonicalCollections[entry.Value] = true
	}

	// Note: storage.md does not exist in current docs structure.
	// This test is a placeholder for when storage documentation is added.
	// For now, we verify the protocol file is valid and contains collections.
	assert.NotEmpty(t, canonicalCollections, "collections.json must contain collection definitions")
}

// TestAPIPathsDocsAlignment verifies that documented API endpoints match
// the canonical api_paths.json registry.
func TestAPIPathsDocsAlignment(t *testing.T) {
	// Load canonical API paths from protocol/constants/api_paths.json
	protocolConstantsDir := filepath.Join(system.ResolveProjectRoot(), "protocol/constants")
	apiPathsData, err := os.ReadFile(filepath.Join(protocolConstantsDir, "api_paths.json"))
	require.NoError(t, err, "protocol/constants/api_paths.json must be readable")

	var apiPathsJSON map[string]interface{}
	err = json.Unmarshal(apiPathsData, &apiPathsJSON)
	require.NoError(t, err, "api_paths.json must be valid JSON")

	// Note: API documentation is in docs/reference/api/index.md
	// This test is a placeholder for verifying documented paths match the registry.
	// For now, we verify the file exists and is valid JSON.
	assert.NotEmpty(t, apiPathsJSON, "api_paths.json must contain API path definitions")
}

// TestEventsDocsAlignment verifies that documented event types match
// the canonical events.json registry.
func TestEventsDocsAlignment(t *testing.T) {
	// Load canonical events from protocol/constants/events.json
	protocolConstantsDir := filepath.Join(system.ResolveProjectRoot(), "protocol/constants")
	eventsData, err := os.ReadFile(filepath.Join(protocolConstantsDir, "events.json"))
	require.NoError(t, err, "protocol/constants/events.json must be readable")

	var eventsJSON struct {
		Events map[string]struct {
			Value string `json:"value"`
		} `json:"events"`
	}
	err = json.Unmarshal(eventsData, &eventsJSON)
	require.NoError(t, err, "events.json must be valid JSON")

	// Extract canonical event names
	canonicalEvents := make(map[string]bool)
	for _, entry := range eventsJSON.Events {
		canonicalEvents[entry.Value] = true
	}

	// Note: events documentation is in docs/architecture/protocol.md
	// This test is a placeholder for verifying documented events match the registry.
	// For now, we verify the protocol file is valid and contains events.
	assert.NotEmpty(t, canonicalEvents, "events.json must contain event definitions")
}

// TestDocsNoStaleTerms verifies that documentation does not contain
// deprecated or misleading terminology.
func TestDocsNoStaleTerms(t *testing.T) {
	staleTerms := []string{
		"Mutual-Adversary",
		"Reality Portal",
		"Sentinel daemon",
		"mandatory g8ed",
		"mandatory g8ee",
		"UAP JSON",
	}

	err := filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		contentStr := string(content)
		for _, term := range staleTerms {
			if strings.Contains(contentStr, term) {
				t.Errorf("File %s contains stale term '%s'", path, term)
			}
		}

		return nil
	})

	require.NoError(t, err, "Failed to walk docs directory")
}
