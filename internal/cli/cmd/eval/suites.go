// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

const suiteStdinArg = "-"

func suitesEvalCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "suites",
		Short: "Create, inspect, edit, and delete evaluation suites",
		Long: `A suite is a named, versioned set of scenarios a campaign scores. Two suites are
built in and read-only: default-suite (the full scenario set) and smoke-suite
(its five-scenario screening subset). Custom suites are authored as JSON files
and stored under the runtime data directory.

A campaign copies its suite when it is created (g8e eval campaigns create
--suite <id>), so editing or deleting a suite never changes a campaign that
already used it.

Start a custom suite from a built-in one:
  g8e eval suites export default-suite > my-suite.json
  (edit id, version, and scenarios)
  g8e eval suites create my-suite.json`,
	}
	addLeaves(cmd,
		suitesListCmd(deps),
		suitesShowCmd(deps),
		suitesCreateCmd(deps),
		suitesUpdateCmd(deps),
		suitesDeleteCmd(deps),
	)
	cmd.AddCommand(noJSON(suitesExportCmd(deps)))
	return cmd
}

type suiteListJSON struct {
	Suites []evaluation.SuiteSummary `json:"suites"`
}

type suiteScenarioRowJSON struct {
	ScenarioID       string   `json:"scenario_id"`
	Category         string   `json:"category"`
	GradingMethod    string   `json:"grading_method"`
	TrajectoryPolicy string   `json:"trajectory_policy"`
	EligibleRoles    []string `json:"eligible_roles"`
}

type suiteShowJSON struct {
	evaluation.SuiteSummary
	CatalogDigest string                 `json:"catalog_digest"`
	Scenarios     []suiteScenarioRowJSON `json:"scenarios"`
}

type suiteChangeJSON struct {
	ID            string `json:"id"`
	Version       string `json:"version"`
	ScenarioCount int    `json:"scenario_count"`
	CatalogDigest string `json:"catalog_digest"`
}

func suitesListCmd(deps nativeEvalDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List built-in and custom suites",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			suites, err := evaluation.NewStore(fileSvc).ListSuites(cmd.Context())
			if err != nil {
				return err
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), suiteListJSON{Suites: suites})
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "SUITE\tVERSION\tSCENARIOS\tSOURCE")
			for _, suite := range suites {
				source := "custom"
				if suite.Builtin {
					source = "built-in"
				}
				_, _ = fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", suite.ID, suite.Version, suite.ScenarioCount, source)
			}
			return w.Flush()
		},
	}
}

func suitesShowCmd(deps nativeEvalDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "show <suite>",
		Short: "Show one suite and its scenarios",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			store := evaluation.NewStore(fileSvc)
			def, builtin, err := store.ResolveSuite(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			catalog, _, err := evaluation.MaterializeSuite(def)
			if err != nil {
				return err
			}
			payload := suiteShowJSON{SuiteSummary: def.Summary(builtin), CatalogDigest: catalog.GetCatalogDigest()}
			for _, scenario := range def.Scenarios {
				payload.Scenarios = append(payload.Scenarios, suiteScenarioRowJSON{
					ScenarioID: scenario.ScenarioID, Category: scenario.Category, GradingMethod: scenario.GradingMethod,
					TrajectoryPolicy: scenario.TrajectoryPolicy, EligibleRoles: scenario.EligibleRoles,
				})
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), payload)
			}
			out := cmd.OutOrStdout()
			source := "custom"
			if builtin {
				source = "built-in"
			}
			_, _ = fmt.Fprintf(out, "Suite: %s\nVersion: %s\nSource: %s\nScenarios: %d\nCatalog digest: %s\n", def.ID, def.Version, source, len(def.Scenarios), payload.CatalogDigest)
			if def.Description != "" {
				_, _ = fmt.Fprintf(out, "Description: %s\n", def.Description)
			}
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "\nSCENARIO\tCATEGORY\tGRADING\tPOLICY\tROLES")
			for _, row := range payload.Scenarios {
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%v\n", row.ScenarioID, row.Category, row.GradingMethod, row.TrajectoryPolicy, row.EligibleRoles)
			}
			return w.Flush()
		},
	}
}

func suitesExportCmd(deps nativeEvalDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "export <suite>",
		Short: "Write one suite's definition file to stdout",
		Long: `Write a suite's full definition (every prompt, fixture, and grading criterion) as
JSON on stdout, in the format 'g8e eval suites create' reads. Redirect it to a
file to use a built-in suite as the template for a custom one.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			def, _, err := evaluation.NewStore(fileSvc).ResolveSuite(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			body, err := evaluation.EncodeScenarioSuite(def)
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(body)
			return err
		},
	}
}

func suitesCreateCmd(deps nativeEvalDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "create <definition>",
		Short: "Create a custom suite from a definition file",
		Long: `Create a custom suite from a JSON definition file ("-" reads stdin). The suite
is validated against the full scenario contract (tool registry, trajectory
policy, argument validators, prompt hints, workspace fixtures) before anything is
stored, and its id must not already exist. Built-in suite ids are reserved.

Examples:
  g8e eval suites export default-suite > my-suite.json
  g8e eval suites create my-suite.json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return writeSuite(cmd, deps, args[0], func(store *evaluation.Store, def evaluation.ScenarioSuite) error {
				return store.CreateSuite(cmd.Context(), def)
			}, "created")
		},
	}
}

func suitesUpdateCmd(deps nativeEvalDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "update <definition>",
		Short: "Replace a custom suite with a new definition",
		Long: `Replace an existing custom suite with a new definition file ("-" reads stdin).
The definition's id selects the suite. Changing a suite's content requires a new
version, so one id@version never names two different suites; resubmitting
identical content changes nothing. Built-in suites cannot be updated.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return writeSuite(cmd, deps, args[0], func(store *evaluation.Store, def evaluation.ScenarioSuite) error {
				return store.UpdateSuite(cmd.Context(), def)
			}, "updated")
		},
	}
}

func suitesDeleteCmd(deps nativeEvalDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <suite>",
		Short: "Delete a custom suite",
		Long: `Delete a custom suite. Campaigns keep the copy of the suite they froze, so
existing campaigns, runs, and their verification are unaffected. Built-in suites
cannot be deleted.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			if err := evaluation.NewStore(fileSvc).DeleteSuite(cmd.Context(), args[0]); err != nil {
				return err
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), map[string]any{"id": args[0], "deleted": true})
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Suite %s deleted\n", args[0])
			return err
		},
	}
}

// writeSuite reads a definition file, applies one store write to it, and
// reports the resulting suite.
func writeSuite(cmd *cobra.Command, deps nativeEvalDeps, source string, apply func(*evaluation.Store, evaluation.ScenarioSuite) error, verb string) error {
	body, err := readSuiteSource(cmd, source)
	if err != nil {
		return err
	}
	def, err := evaluation.DecodeScenarioSuite(body)
	if err != nil {
		return err
	}
	catalog, _, err := evaluation.MaterializeSuite(def)
	if err != nil {
		return err
	}
	_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
	if err != nil {
		return err
	}
	if err := apply(evaluation.NewStore(fileSvc), def); err != nil {
		return err
	}
	payload := suiteChangeJSON{ID: def.ID, Version: def.Version, ScenarioCount: len(def.Scenarios), CatalogDigest: catalog.GetCatalogDigest()}
	if output.JSONEnabled(cmd) {
		return output.WriteJSON(cmd.OutOrStdout(), payload)
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Suite %s@%s %s (%d scenarios)\nCatalog digest: %s\n\nNext: g8e eval campaigns create <campaign> <model> --suite %s\n",
		payload.ID, payload.Version, verb, payload.ScenarioCount, payload.CatalogDigest, payload.ID)
	return err
}

// readSuiteSource reads a user-supplied definition file, or stdin for "-". The
// path is operator input outside the runtime directory, so it is read directly.
func readSuiteSource(cmd *cobra.Command, source string) ([]byte, error) {
	if source == suiteStdinArg {
		body, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return nil, fmt.Errorf("evaluation: read suite definition from stdin: %w", err)
		}
		return body, nil
	}
	body, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("evaluation: read suite definition: %w", err)
	}
	return body, nil
}
