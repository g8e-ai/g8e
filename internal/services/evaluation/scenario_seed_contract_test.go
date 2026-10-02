// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
)

// What g8ee enforces on a seed. The same literals are pinned in
// ensemble/tests/unit/services/evaluation/test_investigation_seed_contract.py,
// so a change on either side fails a test on that side until both are updated.
const (
	g8eeSeedMaxTurns         = 16
	g8eeSeedMaxHistoryEvents = 16
	g8eeSeedMaxCaseTitle     = 200
)

var (
	g8eeSeedSenders = []string{"user", "primary", "assistant"}
	g8eeSeedActors  = []string{"g8eo", "system", "user"}
	// SEEDABLE_HISTORY_EVENTS in ensemble/app/services/evaluation/investigation_seed.py.
	g8eeSeedableHistoryEvents = []string{
		"g8e.v1.operator.command.approval.rejected",
		"g8e.v1.operator.command.execution.started",
		"g8e.v1.operator.command.failed",
		"g8e.v1.operator.file.edit.failed",
		"g8e.v1.operator.filesystem.grep.completed",
		"g8e.v1.operator.filesystem.grep.failed",
		"g8e.v1.operator.filesystem.read.completed",
		"g8e.v1.operator.filesystem.read.failed",
	}
	g8eeSeedEventTypePattern = regexp.MustCompile(`^g8e\.v1\.operator\.[a-z0-9_.]+$`)
)

func TestSeedContract_HarnessMirrorsG8eeTextBound(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 8000, harnessclient.EnsembleSeedMaxText)
	assert.Equal(t, g8eeSeedSenders, []string{
		harnessclient.EnsembleSeedSenderUser, harnessclient.EnsembleSeedSenderPrimary, harnessclient.EnsembleSeedSenderAssistant,
	})
}

// Every shipped seed must be one g8ee accepts, otherwise the request fails with
// HTTP 400 and the scenario reports an environment error on every attempt.
func TestSeedContract_EveryCatalogSeedSatisfiesWhatG8eeEnforces(t *testing.T) {
	t.Parallel()
	catalog, artifacts, err := BuildScenarioCatalog()
	require.NoError(t, err)
	bounded := func(t *testing.T, field, value string) {
		t.Helper()
		assert.LessOrEqual(t, utf8.RuneCountInString(value), harnessclient.EnsembleSeedMaxText, field)
	}
	for _, scenario := range catalog.GetScenarios() {
		t.Run(scenario.GetScenarioId(), func(t *testing.T) {
			t.Parallel()
			var input ScenarioInputFixture
			require.NoError(t, json.Unmarshal(artifacts[scenario.GetScenarioId()].Input.Body, &input))
			seed := input.Seed

			assert.NotEmpty(t, seed.CaseTitle)
			assert.LessOrEqual(t, utf8.RuneCountInString(seed.CaseTitle), g8eeSeedMaxCaseTitle)
			assert.NotContains(t, strings.ToLower(seed.CaseTitle), "eval", "the case title is model-visible (R4)")
			bounded(t, "case_description", seed.CaseDescription)

			assert.LessOrEqual(t, len(seed.Turns), g8eeSeedMaxTurns)
			for _, turn := range seed.Turns {
				assert.Contains(t, g8eeSeedSenders, turn.Sender)
				assert.NotEmpty(t, turn.Content)
				bounded(t, "turn content", turn.Content)
			}

			assert.LessOrEqual(t, len(seed.HistoryEvents), g8eeSeedMaxHistoryEvents)
			for _, event := range seed.HistoryEvents {
				assert.Regexp(t, g8eeSeedEventTypePattern, event.EventType)
				assert.True(t, slices.Contains(g8eeSeedableHistoryEvents, event.EventType), "%q is not a seedable event", event.EventType)
				assert.Contains(t, g8eeSeedActors, event.Actor)
				assert.NotEmpty(t, event.Summary)
				for field, value := range map[string]string{
					"summary": event.Summary, "arguments_json": event.ArgumentsJSON, "command": event.Command, "error": event.Error,
				} {
					bounded(t, field, value)
				}
				assert.LessOrEqual(t, len(event.ToolName), 128)
				assert.LessOrEqual(t, len(event.ExecutionID), 128)
				assert.LessOrEqual(t, len(event.ErrorType), 128)
			}

			if seed.CaseMemory != nil {
				memory := seed.CaseMemory
				for field, value := range map[string]string{
					"investigation_summary": memory.InvestigationSummary, "communication_preferences": memory.CommunicationPreferences,
					"technical_background": memory.TechnicalBackground, "response_style": memory.ResponseStyle,
					"problem_solving_approach": memory.ProblemSolvingApproach, "interaction_style": memory.InteractionStyle,
				} {
					bounded(t, field, value)
				}
			}
		})
	}
}
