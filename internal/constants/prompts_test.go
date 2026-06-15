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

func TestAgentModeConstants(t *testing.T) {
	t.Run("agent mode g8e bound has correct value", func(t *testing.T) {
		assert.Equal(t, "g8e.bound", AgentModeG8eBound)
	})

	t.Run("agent mode g8e not bound has correct value", func(t *testing.T) {
		assert.Equal(t, "g8e.not.bound", AgentModeG8eNotBound)
	})

	t.Run("agent mode cloud operator bound has correct value", func(t *testing.T) {
		assert.Equal(t, "g8e.cloud.bound", AgentModeCloudOperatorBound)
	})

	t.Run("all agent mode constants are distinct", func(t *testing.T) {
		modes := []string{
			AgentModeG8eBound,
			AgentModeG8eNotBound,
			AgentModeCloudOperatorBound,
		}

		seen := make(map[string]bool)
		for _, mode := range modes {
			assert.False(t, seen[mode], "agent mode constant %s is duplicated", mode)
			seen[mode] = true
		}
	})

	t.Run("all agent mode constants have g8e prefix", func(t *testing.T) {
		modes := []string{
			AgentModeG8eBound,
			AgentModeG8eNotBound,
			AgentModeCloudOperatorBound,
		}

		for _, mode := range modes {
			assert.Contains(t, mode, "g8e.", "agent mode constant %s should have g8e prefix", mode)
		}
	})
}

func TestPromptSectionConstants(t *testing.T) {
	t.Run("prompt section identity has correct value", func(t *testing.T) {
		assert.Equal(t, "identity", PromptSectionIdentity)
	})

	t.Run("prompt section safety has correct value", func(t *testing.T) {
		assert.Equal(t, "safety", PromptSectionSafety)
	})

	t.Run("prompt section loyalty has correct value", func(t *testing.T) {
		assert.Equal(t, "loyalty", PromptSectionLoyalty)
	})

	t.Run("prompt section dissent has correct value", func(t *testing.T) {
		assert.Equal(t, "dissent", PromptSectionDissent)
	})

	t.Run("prompt section capabilities has correct value", func(t *testing.T) {
		assert.Equal(t, "capabilities", PromptSectionCapabilities)
	})

	t.Run("prompt section execution has correct value", func(t *testing.T) {
		assert.Equal(t, "execution", PromptSectionExecution)
	})

	t.Run("prompt section tools has correct value", func(t *testing.T) {
		assert.Equal(t, "tools", PromptSectionTools)
	})

	t.Run("prompt section docs has correct value", func(t *testing.T) {
		assert.Equal(t, "docs", PromptSectionDocs)
	})

	t.Run("prompt section system context has correct value", func(t *testing.T) {
		assert.Equal(t, "system_context", PromptSectionSystemContext)
	})

	t.Run("prompt section vault mode has correct value", func(t *testing.T) {
		assert.Equal(t, "sentinel_mode", PromptSectionVaultMode)
	})

	t.Run("prompt section triage context has correct value", func(t *testing.T) {
		assert.Equal(t, "triage_context", PromptSectionTriageContext)
	})

	t.Run("prompt section investigation context has correct value", func(t *testing.T) {
		assert.Equal(t, "investigation_context", PromptSectionInvestigationContext)
	})

	t.Run("prompt section response constraints has correct value", func(t *testing.T) {
		assert.Equal(t, "response_constraints", PromptSectionResponseConstraints)
	})

	t.Run("prompt section learned context has correct value", func(t *testing.T) {
		assert.Equal(t, "learned_context", PromptSectionLearnedContext)
	})

	t.Run("prompt section agent persona has correct value", func(t *testing.T) {
		assert.Equal(t, "agent_persona", PromptSectionAgentPersona)
	})

	t.Run("all prompt section constants are distinct", func(t *testing.T) {
		sections := []string{
			PromptSectionIdentity,
			PromptSectionSafety,
			PromptSectionLoyalty,
			PromptSectionDissent,
			PromptSectionCapabilities,
			PromptSectionExecution,
			PromptSectionTools,
			PromptSectionDocs,
			PromptSectionSystemContext,
			PromptSectionVaultMode,
			PromptSectionTriageContext,
			PromptSectionInvestigationContext,
			PromptSectionResponseConstraints,
			PromptSectionLearnedContext,
			PromptSectionAgentPersona,
		}

		seen := make(map[string]bool)
		for _, section := range sections {
			assert.False(t, seen[section], "prompt section constant %s is duplicated", section)
			seen[section] = true
		}
	})

	t.Run("all prompt section constants use underscores for spaces", func(t *testing.T) {
		sections := []string{
			PromptSectionSystemContext,
			PromptSectionTriageContext,
			PromptSectionInvestigationContext,
			PromptSectionResponseConstraints,
			PromptSectionLearnedContext,
			PromptSectionAgentPersona,
		}

		for _, section := range sections {
			assert.NotContains(t, section, " ", "prompt section constant %s should use underscores instead of spaces", section)
		}
	})
}

func TestPromptsConstantsContractRegression(t *testing.T) {
	t.Run("agent mode constants match protocol values", func(t *testing.T) {
		// These tests ensure the Go constants match the JSON SSOT in protocol/constants/prompts.json
		assert.Equal(t, "g8e.bound", AgentModeG8eBound)
		assert.Equal(t, "g8e.not.bound", AgentModeG8eNotBound)
		assert.Equal(t, "g8e.cloud.bound", AgentModeCloudOperatorBound)
	})

	t.Run("prompt section constants match protocol values", func(t *testing.T) {
		// These tests ensure the Go constants match the JSON SSOT in protocol/constants/prompts.json
		assert.Equal(t, "identity", PromptSectionIdentity)
		assert.Equal(t, "safety", PromptSectionSafety)
		assert.Equal(t, "loyalty", PromptSectionLoyalty)
		assert.Equal(t, "dissent", PromptSectionDissent)
		assert.Equal(t, "capabilities", PromptSectionCapabilities)
		assert.Equal(t, "execution", PromptSectionExecution)
		assert.Equal(t, "tools", PromptSectionTools)
		assert.Equal(t, "docs", PromptSectionDocs)
		assert.Equal(t, "system_context", PromptSectionSystemContext)
		assert.Equal(t, "sentinel_mode", PromptSectionVaultMode)
		assert.Equal(t, "triage_context", PromptSectionTriageContext)
		assert.Equal(t, "investigation_context", PromptSectionInvestigationContext)
		assert.Equal(t, "response_constraints", PromptSectionResponseConstraints)
		assert.Equal(t, "learned_context", PromptSectionLearnedContext)
		assert.Equal(t, "agent_persona", PromptSectionAgentPersona)
	})
}
