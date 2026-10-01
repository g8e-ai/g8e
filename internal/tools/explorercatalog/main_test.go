// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// newCatalogRoot returns a temp directory laid out like a repo root, with the
// directory that holds the generated module already created.
func newCatalogRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, outputRelPath)), 0o755))
	return root
}

func TestGenerate_EmitsEveryCatalogScenarioWithoutHintValues(t *testing.T) {
	out, err := generate()
	require.NoError(t, err)
	text := string(out)

	assert.Contains(t, text, "DO NOT EDIT")
	assert.Equal(t, evaluation.DefaultSuiteScenarioCount, strings.Count(text, `"trajectoryPolicy":`))
	assert.Contains(t, text, `SCENARIO_CATALOG_ID = "`+evaluation.DefaultSuiteID+`@`+evaluation.DefaultSuiteVersion+`"`)
	assert.Contains(t, text, `"promptHint":`)
	// Hint arguments publish where a value comes from, never the value itself.
	assert.NotContains(t, text, `"value":`)
	assert.Contains(t, text, `"source": "workspace"`)
}

func TestGenerate_KeepsLicenseHeader(t *testing.T) {
	out, err := generate()
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(out), "// Copyright (c) 2026 Lateralus Labs, LLC.\n// Licensed under the Business Source License 1.1"))
}

func TestGenerate_IsDeterministic(t *testing.T) {
	first, err := generate()
	require.NoError(t, err)
	second, err := generate()
	require.NoError(t, err)
	assert.Equal(t, first, second)
}

func TestGenerate_UsesPublicToolArgumentsCategorySpelling(t *testing.T) {
	out, err := generate()
	require.NoError(t, err)
	assert.Contains(t, string(out), `"category": "tool_arguments"`)
	assert.NotContains(t, string(out), `"category": "tool_argument"`)
}

func TestEnumSuffix_StripsPrefixAndLowercases(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		prefix string
		want   string
	}{
		{name: "prefixed enum", value: "EVALUATION_GRADING_METHOD_SEMANTIC_JUDGE", prefix: "EVALUATION_GRADING_METHOD_", want: "semantic_judge"},
		{name: "prefix absent leaves value lowercased", value: "OTHER_VALUE", prefix: "EVALUATION_GRADING_METHOD_", want: "other_value"},
		{name: "empty value", value: "", prefix: "EVALUATION_GRADING_METHOD_", want: ""},
		{name: "empty prefix only lowercases", value: "MIXED_Case", prefix: "", want: "mixed_case"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, enumSuffix(tt.value, tt.prefix))
		})
	}
}

func TestNonEmpty_ReturnsNilForEmptyAndCopyForPopulated(t *testing.T) {
	assert.Nil(t, nonEmpty(nil))
	assert.Nil(t, nonEmpty([]string{}))

	source := []string{"a", "b"}
	got := nonEmpty(source)
	assert.Equal(t, []string{"a", "b"}, got)

	got[0] = "mutated"
	assert.Equal(t, "a", source[0], "result must not alias the caller's slice")
}

func TestBuildTask_MapsScenarioFixtureAndGoldIntoPublicTask(t *testing.T) {
	scenario := &evalv1.EvaluationScenarioDefinition{
		ScenarioId:        "scn-001",
		Category:          evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_TOOL_SELECTION,
		PublicDescription: "Pick the right tool.",
		GradingMethod:     evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
		TrajectoryPolicy:  evalv1.EvaluationTrajectoryPolicy_EVALUATION_TRAJECTORY_POLICY_GUIDED,
		RequiredConcepts:  []string{"concept-a"},
		AllowedTools:      []string{"fs_read", "fs_list"},
		ExpectedTools:     []string{"fs_read"},
		ForbiddenTools:    []string{"run_commands"},
	}
	input := evaluation.ScenarioInputFixture{
		UserPrompt: "read the file",
		InlineContext: []evaluation.ScenarioInlineContent{
			{Kind: "log", Label: "Log excerpt", Content: "line one"},
		},
		WorkspaceFiles: []evaluation.ScenarioWorkspaceFile{
			{Label: "Config", RelPath: "etc/app.conf", Content: "a=b"},
			{Label: "Decoy", RelPath: "etc/old.conf", Content: "a=c", Decoy: true},
		},
	}
	gold := evaluation.ScenarioGoldCriteria{ExpectedBehavior: "reads etc/app.conf"}

	got := buildTask(scenario, input, gold)

	assert.Equal(t, "scn-001", got.ID)
	assert.Equal(t, "tool_selection", got.Category)
	assert.Equal(t, "deterministic", got.GradingMethod)
	assert.Equal(t, "guided", got.TrajectoryPolicy)
	assert.Equal(t, "Pick the right tool.", got.PublicDescription)
	assert.Equal(t, "read the file", got.UserPrompt)
	assert.Equal(t, "reads etc/app.conf", got.ExpectedBehavior)
	assert.Equal(t, []string{"concept-a"}, got.RequiredConcepts)
	assert.Equal(t, []string{"fs_read", "fs_list"}, got.AllowedTools)
	assert.Equal(t, []string{"fs_read"}, got.ExpectedTools)
	assert.Equal(t, []string{"run_commands"}, got.ForbiddenTools)
	assert.Equal(t, []inlineContent{{Kind: "log", Label: "Log excerpt", Content: "line one"}}, got.InlineContext)
	assert.Equal(t, []workspaceFile{
		{Label: "Config", RelPath: "etc/app.conf", Content: "a=b"},
		{Label: "Decoy", RelPath: "etc/old.conf", Content: "a=c", Decoy: true},
	}, got.WorkspaceFiles)
	assert.Nil(t, got.PromptHint, "a scenario without a hint must not publish one")
}

func TestBuildTask_CategoryNamesUsePublicSpelling(t *testing.T) {
	tests := []struct {
		name     string
		category evalv1.EvaluationScenarioCategory
		want     string
	}{
		{name: "tool argument is published plural", category: evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_TOOL_ARGUMENT, want: "tool_arguments"},
		{name: "tool selection is unchanged", category: evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_TOOL_SELECTION, want: "tool_selection"},
		{name: "multi-word category is lowercased", category: evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_ROUTING_DELEGATION, want: "routing_delegation"},
		{name: "final response", category: evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_FINAL_RESPONSE, want: "final_response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildTask(&evalv1.EvaluationScenarioDefinition{Category: tt.category}, evaluation.ScenarioInputFixture{}, evaluation.ScenarioGoldCriteria{})
			assert.Equal(t, tt.want, got.Category)
		})
	}
}

func TestBuildTask_OmitsEmptyOptionalCollections(t *testing.T) {
	got := buildTask(&evalv1.EvaluationScenarioDefinition{ScenarioId: "scn-empty"}, evaluation.ScenarioInputFixture{}, evaluation.ScenarioGoldCriteria{})

	assert.Nil(t, got.AllowedTools)
	assert.Nil(t, got.ExpectedTools)
	assert.Nil(t, got.ForbiddenTools)
	assert.Nil(t, got.InlineContext)
	assert.Nil(t, got.WorkspaceFiles)
	assert.NotNil(t, got.RequiredConcepts, "requiredConcepts is always emitted as an array")
	assert.Empty(t, got.RequiredConcepts)
}

func TestBuildTask_PublishesHintSourcesButNeverArgumentValues(t *testing.T) {
	scenario := &evalv1.EvaluationScenarioDefinition{
		PromptHint: &evalv1.PromptHint{
			HintedTools: []string{"fs_read"},
			Arguments: []*evalv1.PromptHintArgument{
				{ToolName: "fs_read", ArgumentName: "path", Source: evalv1.EvaluationHintArgumentSource_EVALUATION_HINT_ARGUMENT_SOURCE_WORKSPACE},
				{ToolName: "fs_read", ArgumentName: "limit", Source: evalv1.EvaluationHintArgumentSource_EVALUATION_HINT_ARGUMENT_SOURCE_MODEL_AUTHORED},
			},
		},
	}

	got := buildTask(scenario, evaluation.ScenarioInputFixture{}, evaluation.ScenarioGoldCriteria{})

	require.NotNil(t, got.PromptHint)
	assert.Equal(t, []string{"fs_read"}, got.PromptHint.HintedTools)
	assert.Equal(t, []hintArgument{
		{ToolName: "fs_read", ArgumentName: "path", Source: "workspace"},
		{ToolName: "fs_read", ArgumentName: "limit", Source: "model_authored"},
	}, got.PromptHint.Arguments)
}

func TestBuildTask_HintWithoutArgumentsEmitsEmptyArgumentList(t *testing.T) {
	scenario := &evalv1.EvaluationScenarioDefinition{
		PromptHint: &evalv1.PromptHint{HintedTools: []string{"fs_list"}},
	}

	got := buildTask(scenario, evaluation.ScenarioInputFixture{}, evaluation.ScenarioGoldCriteria{})

	require.NotNil(t, got.PromptHint)
	assert.NotNil(t, got.PromptHint.Arguments, "arguments must serialize as [] rather than null")
	assert.Empty(t, got.PromptHint.Arguments)
}

func TestFindRepoRoot_LocatesDirectoryContainingGoMod(t *testing.T) {
	root, err := findRepoRoot()
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(root, "go.mod"))
	assert.NoError(t, err)
}

func TestRun_WriteCreatesModuleAndCheckAcceptsIt(t *testing.T) {
	root := newCatalogRoot(t)
	var stdout, stderr bytes.Buffer

	require.NoError(t, run([]string{"-write"}, root, &stdout, &stderr))
	assert.Equal(t, "generated "+outputRelPath+"\n", stdout.String())

	written, err := os.ReadFile(filepath.Join(root, outputRelPath))
	require.NoError(t, err)
	expected, err := generate()
	require.NoError(t, err)
	assert.Equal(t, expected, written)

	require.NoError(t, run([]string{"-check"}, root, &stdout, &stderr))
}

func TestRun_DefaultsToCheckMode(t *testing.T) {
	root := newCatalogRoot(t)
	generated, err := generate()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, outputRelPath), generated, 0o644))

	require.NoError(t, run(nil, root, &bytes.Buffer{}, &bytes.Buffer{}))
}

func TestRun_CheckRejectsStaleModule(t *testing.T) {
	root := newCatalogRoot(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, outputRelPath), []byte("// stale\n"), 0o644))

	err := run([]string{"-check"}, root, &bytes.Buffer{}, &bytes.Buffer{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of date")
	assert.Contains(t, err.Error(), "make explorer-catalog")
}

func TestRun_CheckReportsMissingModule(t *testing.T) {
	root := newCatalogRoot(t)

	err := run([]string{"-check"}, root, &bytes.Buffer{}, &bytes.Buffer{})

	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.Contains(t, err.Error(), "make explorer-catalog")
}

func TestRun_WriteFailsWhenOutputDirectoryIsMissing(t *testing.T) {
	err := run([]string{"-write"}, t.TempDir(), &bytes.Buffer{}, &bytes.Buffer{})

	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.Contains(t, err.Error(), "write "+outputRelPath)
}

func TestRun_RejectsConflictingAndUnknownFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "check and write together", args: []string{"-check", "-write"}, want: "use only one of -check or -write"},
		{name: "unknown flag", args: []string{"-bogus"}, want: "flag provided but not defined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newCatalogRoot(t)

			err := run(tt.args, root, &bytes.Buffer{}, &bytes.Buffer{})

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			_, statErr := os.Stat(filepath.Join(root, outputRelPath))
			assert.ErrorIs(t, statErr, os.ErrNotExist, "a rejected invocation must not write the module")
		})
	}
}
