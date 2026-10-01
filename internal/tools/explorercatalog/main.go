// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// explorercatalog generates the evaluation explorer's scenario catalog module
// from the Go scenario catalog, so the public task pages can never drift from
// the scenarios that are actually scored. Prompt-hint argument values are
// private and never emitted (INV-EVAL-EVID-04).
//
//	go run ./internal/tools/explorercatalog -write
//	go run ./internal/tools/explorercatalog -check
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const outputRelPath = "dashboard/g8e-adapter/evaluation-explorer/src/content/scenario-catalog.generated.ts"

type inlineContent struct {
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	Content string `json:"content"`
}

type workspaceFile struct {
	Label   string `json:"label"`
	RelPath string `json:"relPath"`
	Content string `json:"content"`
	Decoy   bool   `json:"decoy,omitempty"`
}

type hintArgument struct {
	ToolName     string `json:"toolName"`
	ArgumentName string `json:"argumentName"`
	Source       string `json:"source"`
}

type promptHint struct {
	HintedTools []string       `json:"hintedTools"`
	Arguments   []hintArgument `json:"arguments"`
}

type task struct {
	ID                string          `json:"id"`
	Category          string          `json:"category"`
	PublicDescription string          `json:"publicDescription"`
	GradingMethod     string          `json:"gradingMethod"`
	TrajectoryPolicy  string          `json:"trajectoryPolicy"`
	UserPrompt        string          `json:"userPrompt"`
	InlineContext     []inlineContent `json:"inlineContext,omitempty"`
	WorkspaceFiles    []workspaceFile `json:"workspaceFiles,omitempty"`
	ExpectedBehavior  string          `json:"expectedBehavior"`
	RequiredConcepts  []string        `json:"requiredConcepts"`
	AllowedTools      []string        `json:"allowedTools,omitempty"`
	ExpectedTools     []string        `json:"expectedTools,omitempty"`
	ForbiddenTools    []string        `json:"forbiddenTools,omitempty"`
	PromptHint        *promptHint     `json:"promptHint,omitempty"`
}

func main() {
	root, err := findRepoRoot()
	if err != nil {
		fatal("%v", err)
	}
	if err := run(os.Args[1:], root, os.Stdout, os.Stderr); err != nil {
		fatal("%v", err)
	}
}

// run parses args and either writes the generated module under root or
// verifies the committed module matches the Go catalog. -check is the default.
func run(args []string, root string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("explorercatalog", flag.ContinueOnError)
	flags.SetOutput(stderr)
	checkOnly := flags.Bool("check", false, "verify the generated catalog module matches the Go catalog")
	write := flags.Bool("write", false, "write the generated catalog module")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *checkOnly && *write {
		return errors.New("use only one of -check or -write")
	}
	generated, err := generate()
	if err != nil {
		return err
	}
	path := filepath.Join(root, outputRelPath)
	if *write {
		if err := os.WriteFile(path, generated, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", outputRelPath, err)
		}
		fmt.Fprintf(stdout, "generated %s\n", outputRelPath)
		return nil
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w (run: make explorer-catalog)", outputRelPath, err)
	}
	if !bytes.Equal(existing, generated) {
		return fmt.Errorf("%s is out of date (run: make explorer-catalog)", outputRelPath)
	}
	return nil
}

// generate renders the TypeScript module for the current Go catalog.
func generate() ([]byte, error) {
	catalog, artifacts, err := evaluation.BuildScenarioCatalog()
	if err != nil {
		return nil, fmt.Errorf("build scenario catalog: %w", err)
	}
	tasks := make([]task, 0, len(catalog.GetScenarios()))
	for _, scenario := range catalog.GetScenarios() {
		pair, ok := artifacts[scenario.GetScenarioId()]
		if !ok {
			return nil, fmt.Errorf("scenario %s has no fixture artifacts", scenario.GetScenarioId())
		}
		var input evaluation.ScenarioInputFixture
		if err := json.Unmarshal(pair.Input.Body, &input); err != nil {
			return nil, fmt.Errorf("decode input fixture for %s: %w", scenario.GetScenarioId(), err)
		}
		var gold evaluation.ScenarioGoldCriteria
		if err := json.Unmarshal(pair.Gold.Body, &gold); err != nil {
			return nil, fmt.Errorf("decode gold criteria for %s: %w", scenario.GetScenarioId(), err)
		}
		tasks = append(tasks, buildTask(scenario, input, gold))
	}
	body, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal tasks: %w", err)
	}
	var out bytes.Buffer
	fmt.Fprintf(&out, `// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Code generated by internal/tools/explorercatalog; DO NOT EDIT.
// Regenerate with: make explorer-catalog
//
// Source of truth: internal/services/evaluation/scenario_catalog_definitions.go.
// Prompt-hint argument values are private and are never emitted here.

import type { ScenarioTaskDefinition } from './scenario-task';

export const SCENARIO_CATALOG_VERSION = %q;
export const SCENARIO_CATALOG_ID = %q;

export const SCENARIO_TASKS: readonly ScenarioTaskDefinition[] = %s;
`, evaluation.DefaultSuiteVersion, evaluation.DefaultSuiteID+"@"+evaluation.DefaultSuiteVersion, body)
	return out.Bytes(), nil
}

func buildTask(scenario *evalv1.EvaluationScenarioDefinition, input evaluation.ScenarioInputFixture, gold evaluation.ScenarioGoldCriteria) task {
	out := task{
		ID:                scenario.GetScenarioId(),
		Category:          enumSuffix(scenario.GetCategory().String(), "EVALUATION_SCENARIO_CATEGORY_"),
		PublicDescription: scenario.GetPublicDescription(),
		GradingMethod:     enumSuffix(scenario.GetGradingMethod().String(), "EVALUATION_GRADING_METHOD_"),
		TrajectoryPolicy:  enumSuffix(scenario.GetTrajectoryPolicy().String(), "EVALUATION_TRAJECTORY_POLICY_"),
		UserPrompt:        input.UserPrompt,
		ExpectedBehavior:  gold.ExpectedBehavior,
		RequiredConcepts:  append([]string{}, scenario.GetRequiredConcepts()...),
		AllowedTools:      nonEmpty(scenario.GetAllowedTools()),
		ExpectedTools:     nonEmpty(scenario.GetExpectedTools()),
		ForbiddenTools:    nonEmpty(scenario.GetForbiddenTools()),
	}
	// The explorer's category ids are the lowercase enum suffixes, except the
	// proto singular TOOL_ARGUMENT, which the public contract spells plural.
	if out.Category == "tool_argument" {
		out.Category = "tool_arguments"
	}
	for _, block := range input.InlineContext {
		out.InlineContext = append(out.InlineContext, inlineContent{Kind: block.Kind, Label: block.Label, Content: block.Content})
	}
	for _, file := range input.WorkspaceFiles {
		out.WorkspaceFiles = append(out.WorkspaceFiles, workspaceFile{Label: file.Label, RelPath: file.RelPath, Content: file.Content, Decoy: file.Decoy})
	}
	if hint := scenario.GetPromptHint(); hint != nil {
		published := &promptHint{HintedTools: append([]string{}, hint.GetHintedTools()...), Arguments: []hintArgument{}}
		for _, argument := range hint.GetArguments() {
			published.Arguments = append(published.Arguments, hintArgument{
				ToolName:     argument.GetToolName(),
				ArgumentName: argument.GetArgumentName(),
				Source:       enumSuffix(argument.GetSource().String(), "EVALUATION_HINT_ARGUMENT_SOURCE_"),
			})
		}
		out.PromptHint = published
	}
	return out
}

func enumSuffix(value, prefix string) string {
	return strings.ToLower(strings.TrimPrefix(value, prefix))
}

func nonEmpty(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string{}, values...)
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above working directory")
		}
		dir = parent
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "explorercatalog: "+format+"\n", args...)
	os.Exit(1)
}
