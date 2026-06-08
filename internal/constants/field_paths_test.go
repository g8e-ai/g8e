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

package constants

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFieldPathConstants(t *testing.T) {
	t.Run("constants are defined", func(t *testing.T) {
		assert.Equal(t, "investigations", FieldPathInvestigations)
		assert.Equal(t, "memories", FieldPathMemories)
		assert.Equal(t, "cases", FieldPathCases)
	})
}

func TestGetFieldPaths(t *testing.T) {
	t.Run("returns non-nil map", func(t *testing.T) {
		result := GetFieldPaths()
		assert.NotNil(t, result)
	})

	t.Run("contains all expected collections", func(t *testing.T) {
		result := GetFieldPaths()
		assert.Contains(t, result, FieldPathInvestigations)
		assert.Contains(t, result, FieldPathMemories)
		assert.Contains(t, result, FieldPathCases)
	})

	t.Run("investigations config is correct", func(t *testing.T) {
		result := GetFieldPaths()
		config := result[FieldPathInvestigations]

		assert.Contains(t, config.AllowedPaths, "suspect_ip_addresses")
		assert.Contains(t, config.AllowedPaths, "status")
		assert.Contains(t, config.AllowedPaths, "priority")
		assert.Contains(t, config.ForbiddenPaths, "credentials")
		assert.Contains(t, config.ForbiddenPaths, "api_keys")
		assert.Contains(t, config.ForbiddenPaths, "secrets")
	})

	t.Run("memories config is correct", func(t *testing.T) {
		result := GetFieldPaths()
		config := result[FieldPathMemories]

		assert.Contains(t, config.AllowedPaths, "content")
		assert.Contains(t, config.AllowedPaths, "tags")
		assert.Contains(t, config.AllowedPaths, "created_at")
		assert.Contains(t, config.ForbiddenPaths, "passwords")
		assert.Contains(t, config.ForbiddenPaths, "tokens")
		assert.Contains(t, config.ForbiddenPaths, "private_keys")
	})

	t.Run("cases config is correct", func(t *testing.T) {
		result := GetFieldPaths()
		config := result[FieldPathCases]

		assert.Contains(t, config.AllowedPaths, "title")
		assert.Contains(t, config.AllowedPaths, "description")
		assert.Contains(t, config.AllowedPaths, "resolution_summary")
		assert.Contains(t, config.ForbiddenPaths, "credentials")
		assert.Contains(t, config.ForbiddenPaths, "api_keys")
		assert.Contains(t, config.ForbiddenPaths, "secrets")
	})

	t.Run("returns copy to prevent mutation", func(t *testing.T) {
		result1 := GetFieldPaths()
		result2 := GetFieldPaths()

		// Modify the first result by replacing the entire config
		result1[FieldPathInvestigations] = FieldPathConfig{
			AllowedPaths:   []string{"modified"},
			ForbiddenPaths: []string{},
		}

		// Second result should not be affected
		assert.NotEqual(t, result1[FieldPathInvestigations].AllowedPaths, result2[FieldPathInvestigations].AllowedPaths)
		assert.Contains(t, result2[FieldPathInvestigations].AllowedPaths, "suspect_ip_addresses")
	})
}

func TestFieldPathConfig(t *testing.T) {
	t.Run("struct fields are exported", func(t *testing.T) {
		config := FieldPathConfig{
			AllowedPaths:   []string{"test"},
			ForbiddenPaths: []string{"secret"},
		}
		assert.Equal(t, []string{"test"}, config.AllowedPaths)
		assert.Equal(t, []string{"secret"}, config.ForbiddenPaths)
	})
}

func TestFieldPathContractRegression(t *testing.T) {
	t.Run("investigations allowed paths match expected", func(t *testing.T) {
		result := GetFieldPaths()
		config := result[FieldPathInvestigations]

		expectedAllowed := []string{
			"suspect_ip_addresses",
			"suspect_hostnames",
			"suspect_domains",
			"malware_hashes",
			"ioc_sources",
			"attack_patterns",
			"timeline_events",
			"evidence_summary",
			"status",
			"priority",
			"assigned_analyst",
			"created_at",
			"updated_at",
			"metadata",
		}
		assert.Equal(t, expectedAllowed, config.AllowedPaths)
	})

	t.Run("investigations forbidden paths match expected", func(t *testing.T) {
		result := GetFieldPaths()
		config := result[FieldPathInvestigations]

		expectedForbidden := []string{
			"credentials",
			"api_keys",
			"passwords",
			"tokens",
			"private_keys",
			"secrets",
		}
		assert.Equal(t, expectedForbidden, config.ForbiddenPaths)
	})

	t.Run("memories allowed paths match expected", func(t *testing.T) {
		result := GetFieldPaths()
		config := result[FieldPathMemories]

		expectedAllowed := []string{
			"content",
			"summary",
			"tags",
			"source",
			"context",
			"created_at",
			"updated_at",
		}
		assert.Equal(t, expectedAllowed, config.AllowedPaths)
	})

	t.Run("memories forbidden paths match expected", func(t *testing.T) {
		result := GetFieldPaths()
		config := result[FieldPathMemories]

		expectedForbidden := []string{
			"credentials",
			"api_keys",
			"passwords",
			"tokens",
			"private_keys",
			"secrets",
		}
		assert.Equal(t, expectedForbidden, config.ForbiddenPaths)
	})

	t.Run("cases allowed paths match expected", func(t *testing.T) {
		result := GetFieldPaths()
		config := result[FieldPathCases]

		expectedAllowed := []string{
			"title",
			"description",
			"status",
			"priority",
			"assigned_to",
			"created_at",
			"updated_at",
			"resolution_summary",
		}
		assert.Equal(t, expectedAllowed, config.AllowedPaths)
	})

	t.Run("cases forbidden paths match expected", func(t *testing.T) {
		result := GetFieldPaths()
		config := result[FieldPathCases]

		expectedForbidden := []string{
			"credentials",
			"api_keys",
			"passwords",
			"tokens",
			"private_keys",
			"secrets",
		}
		assert.Equal(t, expectedForbidden, config.ForbiddenPaths)
	})
}
