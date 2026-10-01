// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package agenttoolsconstants

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentToolRegistryJSON_ReturnsNonEmptyDocumentThatIsStableAcrossCalls(t *testing.T) {
	first := AgentToolRegistryJSON()
	second := AgentToolRegistryJSON()

	require.NotEmpty(t, first)
	assert.Equal(t, first, second)
}

func TestAgentToolRegistryJSON_ReturnsDefensiveCopy(t *testing.T) {
	first := AgentToolRegistryJSON()
	second := AgentToolRegistryJSON()

	first[0] ^= 0xff

	assert.NotEqual(t, first, second, "mutating one result must not alter another")
	assert.Equal(t, second, AgentToolRegistryJSON(), "mutating a result must not alter the embedded document")
}

func TestAgentToolRegistryJSON_IsAnObjectWithEachGeneratedSectionPopulated(t *testing.T) {
	var document map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(AgentToolRegistryJSON(), &document))

	var schemaVersion string
	require.Contains(t, document, "schema_version")
	require.NoError(t, json.Unmarshal(document["schema_version"], &schemaVersion))
	assert.NotEmpty(t, schemaVersion)

	for _, section := range []string{"tools", "guidance_vectors"} {
		raw, ok := document[section]
		require.True(t, ok, "registry is missing section %q", section)
		var entries []json.RawMessage
		require.NoError(t, json.Unmarshal(raw, &entries), "section %q must be a JSON array", section)
		assert.NotEmpty(t, entries, "section %q must not be empty", section)
	}
}

func TestAgentToolRegistryJSON_EveryToolDeclaresModesAndItsRequiredArgumentsAreDeclared(t *testing.T) {
	var document struct {
		Tools []struct {
			Name              string   `json:"name"`
			AgentModes        []string `json:"agent_modes"`
			Arguments         []string `json:"arguments"`
			RequiredArguments []string `json:"required_arguments"`
		} `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(AgentToolRegistryJSON(), &document))

	for _, tool := range document.Tools {
		assert.NotEmpty(t, tool.AgentModes, "tool %q declares no agent mode", tool.Name)
		declared := make(map[string]bool, len(tool.Arguments))
		for _, argument := range tool.Arguments {
			declared[argument] = true
		}
		for _, required := range tool.RequiredArguments {
			assert.True(t, declared[required], "tool %q requires undeclared argument %q", tool.Name, required)
		}
	}
}

func TestAgentToolRegistryJSON_EveryToolAndGuidanceVectorIsNamed(t *testing.T) {
	var document struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
		GuidanceVectors []struct {
			VectorID string `json:"vector_id"`
			ToolName string `json:"tool_name"`
		} `json:"guidance_vectors"`
	}
	require.NoError(t, json.Unmarshal(AgentToolRegistryJSON(), &document))

	toolNames := make(map[string]bool, len(document.Tools))
	for i, tool := range document.Tools {
		assert.NotEmpty(t, tool.Name, "tool at index %d has no name", i)
		assert.False(t, toolNames[tool.Name], "duplicate tool name %q", tool.Name)
		toolNames[tool.Name] = true
	}
	vectorIDs := make(map[string]bool, len(document.GuidanceVectors))
	for i, vector := range document.GuidanceVectors {
		assert.NotEmpty(t, vector.VectorID, "guidance vector at index %d has no vector_id", i)
		assert.False(t, vectorIDs[vector.VectorID], "duplicate guidance vector %q", vector.VectorID)
		vectorIDs[vector.VectorID] = true
		assert.True(t, toolNames[vector.ToolName], "guidance vector %q references unknown tool %q", vector.VectorID, vector.ToolName)
	}
}
