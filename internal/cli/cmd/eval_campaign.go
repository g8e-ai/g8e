// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type campaignInitOutput struct {
	CampaignID          string `json:"campaign_id"`
	RunID               string `json:"run_id"`
	CatalogDigest       string `json:"catalog_digest"`
	ModelRegistryDigest string `json:"model_registry_digest"`
	ModelCount          int    `json:"model_count"`
	ScenarioCount       int    `json:"scenario_count"`
	ProjectRoot         string `json:"project_root"`
}

type campaignListOutput struct {
	Campaigns []campaignListCampaign `json:"campaigns"`
}

type campaignListCampaign struct {
	CampaignID             string            `json:"campaign_id"`
	ModelCount             int               `json:"model_count"`
	ScenarioCount          uint32            `json:"scenario_count"`
	RepetitionCount        uint32            `json:"repetition_count"`
	ModelRegistryDigest    string            `json:"model_registry_digest"`
	CatalogDigest          string            `json:"catalog_digest"`
	HasHeterogeneousStacks bool              `json:"has_heterogeneous_stacks"`
	Runs                   []campaignListRun `json:"runs"`
}

type campaignListRun struct {
	RunID               string `json:"run_id"`
	Status              string `json:"status"`
	ExpectedAssignments uint64 `json:"expected_assignments"`
	Queued              uint32 `json:"queued"`
	Running             uint32 `json:"running"`
	Terminal            uint32 `json:"terminal"`
}

type campaignStacksOutput struct {
	CampaignID           string `json:"campaign_id"`
	GenerationRule       string `json:"generation_rule"`
	Seed                 uint64 `json:"seed"`
	SetDigest            string `json:"set_digest"`
	StackCount           int    `json:"stack_count"`
	HypothesisStackCount uint32 `json:"hypothesis_stack_count"`
	CoverageStackCount   uint32 `json:"coverage_stack_count"`
}

type campaignScheduleOutput struct {
	RunID           string `json:"run_id"`
	AssignmentCount int    `json:"assignment_count"`
	Heterogeneous   bool   `json:"heterogeneous"`
}

type campaignExecuteResult struct {
	AssignmentID string `json:"assignment_id"`
	Status       string `json:"status"`
	ResultDigest string `json:"result_digest"`
}

type campaignExecuteOutput struct {
	RunID     string                  `json:"run_id"`
	Executed  int                     `json:"executed"`
	Remaining int64                   `json:"remaining"`
	Results   []campaignExecuteResult `json:"results"`
}

type campaignPublishOutput struct {
	RunID            string `json:"run_id"`
	PublishedRecords int    `json:"published_records"`
	Force            bool   `json:"force"`
}

type campaignVerifyOutput struct {
	RunID          string   `json:"run_id"`
	Status         string   `json:"status"`
	FailureCount   uint32   `json:"failure_count"`
	FailureReasons []string `json:"failure_reasons"`
}

type campaignAccountOutput struct {
	RunID                 string            `json:"run_id"`
	Complete              bool              `json:"complete"`
	ExpectedCells         uint64            `json:"expected_cells"`
	ScheduledAssignments  uint32            `json:"scheduled_assignments"`
	Queued                uint32            `json:"queued"`
	Running               uint32            `json:"running"`
	Terminal              uint32            `json:"terminal"`
	Stopped               uint32            `json:"stopped"`
	DispositionCounts     map[string]uint32 `json:"disposition_counts"`
	MissingCells          []string          `json:"missing_cells"`
	DuplicateIdentities   []string          `json:"duplicate_identities"`
	ExtraAssignments      []string          `json:"extra_assignments"`
	TerminalWithoutResult []string          `json:"terminal_without_result"`
	ResultWithoutTerminal []string          `json:"result_without_terminal"`
	FailureReasons        []string          `json:"failure_reasons"`
	AccountedAt           string            `json:"accounted_at"`
}

type campaignRepairOutput struct {
	RunID   string `json:"run_id"`
	Count   int    `json:"repaired_trace_digests,omitempty"`
	Results int    `json:"repaired_results,omitempty"`
}

type campaignExportOutput struct {
	RunID               string                          `json:"run_id"`
	CampaignID          string                          `json:"campaign_id"`
	OutputDir           string                          `json:"output_dir"`
	ExportedAt          string                          `json:"exported_at"`
	AssignmentCount     uint32                          `json:"assignment_count"`
	TerminalResultCount uint32                          `json:"terminal_result_count"`
	Files               []evaluation.CampaignExportFile `json:"files"`
}

type campaignShowOutput struct {
	RunID                  string `json:"run_id"`
	CampaignID             string `json:"campaign_id"`
	StartedAt              string `json:"started_at"`
	Lane                   string `json:"lane"`
	ModelCount             int    `json:"model_count"`
	ScenarioCount          uint32 `json:"scenario_count"`
	RepetitionCount        uint32 `json:"repetition_count"`
	ModelRegistryDigest    string `json:"model_registry_digest"`
	CatalogDigest          string `json:"catalog_digest"`
	HasHeterogeneousStacks bool   `json:"has_heterogeneous_stacks"`
	ExpectedAssignments    uint64 `json:"expected_assignments"`
	Queued                 uint32 `json:"queued"`
	Running                uint32 `json:"running"`
	Terminal               uint32 `json:"terminal"`
	NextAssignmentID       string `json:"next_assignment_id"`
}

type campaignStatusOutput struct {
	RunID               string `json:"run_id"`
	CampaignID          string `json:"campaign_id"`
	ExpectedAssignments uint64 `json:"expected_assignments"`
	Queued              uint32 `json:"queued"`
	Running             uint32 `json:"running"`
	Terminal            uint32 `json:"terminal"`
	NextAssignmentID    string `json:"next_assignment_id"`
}

func campaignEvalCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "campaign",
		Short: "Initialize, schedule, and resume evaluation model campaigns",
	}
	cmd.AddCommand(
		campaignEvalStartCmd(deps),
		campaignEvalInitCmd(deps),
		campaignEvalListCmd(deps),
		campaignEvalScheduleCmd(deps),
		campaignEvalStacksCmd(deps),
		campaignEvalExecuteCmd(deps),
		campaignEvalPublishCmd(deps),
		campaignEvalMirrorCmd(deps),
		campaignEvalVerifyCmd(deps),
		campaignEvalAccountCmd(deps),
		campaignEvalExportCmd(deps),
		campaignEvalRepairCmd(deps),
		campaignEvalShowCmd(deps),
		campaignEvalStatusCmd(deps),
	)
	return cmd
}

func campaignEvalInitCmd(deps nativeEvalDeps) *cobra.Command {
	var campaignID string
	var runID string
	var inventoryFile string
	var inferenceSessionID string
	var dataSessionID string
	var repetitionCount uint32
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a campaign run with frozen catalog and model registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			if campaignID == "" || runID == "" {
				return fmt.Errorf("evaluation: campaign init: %w", constants.ErrMissingRequiredField)
			}
			if inventoryFile == "" {
				return fmt.Errorf("evaluation: campaign init: --inventory-file is required")
			}
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			authContext, err := deps.authLoader(fileSvc, cfg)
			if err != nil {
				return fmt.Errorf("evaluation: campaign init: load CLI identity: %w", err)
			}
			inventory, err := loadEvaluationInventoryFreeze(cmd.Context(), fileSvc, cfg.ProjectRoot, inventoryFile)
			if err != nil {
				return fmt.Errorf("evaluation: campaign init: load inventory: %w", err)
			}
			if inventory.CampaignID != "" && inventory.CampaignID != campaignID {
				return fmt.Errorf("evaluation: campaign init: inventory campaign_id mismatch")
			}
			inventory.CampaignID = campaignID
			catalog, artifacts, err := evaluation.LoadScenarioCatalog()
			if err != nil {
				return fmt.Errorf("evaluation: campaign init: %w", err)
			}
			controller := evaluation.NewCampaignController(evaluation.NewStore(fileSvc), nil, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
			run, err := controller.InitializeCampaign(cmd.Context(), evaluation.CampaignInitRequest{
				CampaignID:                 campaignID,
				RunID:                      runID,
				Catalog:                    catalog,
				Inventory:                  inventory,
				ScenarioArtifacts:          artifacts,
				RepetitionCount:            repetitionCount,
				InferenceOperatorSessionID: inferenceSessionID,
				DataOperatorSessionID:      dataSessionID,
				Deployment:                 nativeEvalDeployment(cfg, authContext, runID, ""),
			})
			if err != nil {
				return fmt.Errorf("evaluation: campaign init: %w", err)
			}
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(campaignInitOutput{
					CampaignID:          campaignID,
					RunID:               runID,
					CatalogDigest:       catalog.GetCatalogDigest(),
					ModelRegistryDigest: inventory.RegistryDigest,
					ModelCount:          len(inventory.Variants),
					ScenarioCount:       len(catalog.GetScenarios()),
					ProjectRoot:         cfg.ProjectRoot,
				}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Campaign initialized\nCampaign: %s\nRun: %s\nCatalog digest: %s\nModel registry digest: %s\nModels: %d\nScenarios: %d\nStarted: %s\n",
				campaignID,
				run.GetRunId(),
				catalog.GetCatalogDigest(),
				inventory.RegistryDigest,
				len(inventory.Variants),
				len(catalog.GetScenarios()),
				run.GetStartedAt().AsTime().Format("2006-01-02T15:04:05Z"),
			)
			return err
		},
	}
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "Frozen evaluation campaign ID")
	cmd.Flags().StringVar(&runID, "run-id", "", "Campaign run ID")
	cmd.Flags().StringVar(&inventoryFile, "inventory-file", "", "Phase 3 inventory freeze JSON path")
	cmd.Flags().StringVar(&inferenceSessionID, "inference-session", "", "Exact inference Operator session ID")
	cmd.Flags().StringVar(&dataSessionID, "data-session", "", "Exact data Operator session ID")
	cmd.Flags().Uint32Var(&repetitionCount, "repetition-count", 1, "Homogeneous repetition count")
	return cmd
}

func campaignEvalListCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List persisted evaluation campaigns",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			store := evaluation.NewStore(fileSvc)
			campaigns, err := store.ListCampaigns(cmd.Context())
			if err != nil {
				return fmt.Errorf("evaluation: campaign list: %w", err)
			}
			controller := evaluation.NewCampaignController(store, nil, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
			rows, err := collectCampaignListRows(cmd.Context(), controller, store, campaigns)
			if err != nil {
				return fmt.Errorf("evaluation: campaign list: %w", err)
			}
			if output.JSONEnabled(cmd) {
				entries := make([]campaignListCampaign, 0, len(campaigns))
				for _, campaign := range campaigns {
					runEntries := make([]campaignListRun, 0, len(campaign.RunIDs))
					for _, row := range rows {
						if row.CampaignID != campaign.CampaignID {
							continue
						}
						runEntries = append(runEntries, campaignListRun{
							RunID:               row.RunID,
							Status:              row.Status,
							ExpectedAssignments: row.ExpectedAssignments,
							Queued:              row.Queued,
							Running:             row.Running,
							Terminal:            row.Terminal,
						})
					}
					entries = append(entries, campaignListCampaign{
						CampaignID:             campaign.CampaignID,
						ModelCount:             campaign.ModelCount,
						ScenarioCount:          campaign.ScenarioCount,
						RepetitionCount:        campaign.RepetitionCount,
						ModelRegistryDigest:    campaign.ModelRegistryDigest,
						CatalogDigest:          campaign.CatalogDigest,
						HasHeterogeneousStacks: campaign.HasHeterogeneousStacks,
						Runs:                   runEntries,
					})
				}
				payload, err := json.MarshalIndent(campaignListOutput{Campaigns: entries}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			if len(rows) == 0 {
				cmd.Println("No campaigns found")
				return nil
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Campaigns (%d campaigns, %d runs)\n", len(campaigns), len(rows))
			if err != nil {
				return err
			}
			cmd.Println(strings.Repeat("=", 132))
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "  %-24s  %-38s  %-14s  %-10s  %-6s  %-9s  %-4s\n",
				"CAMPAIGN", "RUN", "STATUS", "PROGRESS", "MODELS", "SCENARIOS", "REPS")
			if err != nil {
				return err
			}
			cmd.Println(strings.Repeat("-", 132))
			for _, row := range rows {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "  %-24s  %-38s  %-14s  %3d/%-6d  %-6d  %-9d  %-4d\n",
					row.CampaignID,
					row.RunID,
					row.Status,
					row.Terminal,
					row.ExpectedAssignments,
					row.ModelCount,
					row.ScenarioCount,
					row.RepetitionCount,
				)
				if err != nil {
					return err
				}
			}
			return nil
		},
	}
	return cmd
}

type campaignListRow struct {
	CampaignID          string
	RunID               string
	Status              string
	ExpectedAssignments uint64
	Queued              uint32
	Running             uint32
	Terminal            uint32
	ModelCount          int
	ScenarioCount       uint32
	RepetitionCount     uint32
}

func collectCampaignListRows(
	ctx context.Context,
	controller *evaluation.CampaignController,
	store *evaluation.Store,
	campaigns []evaluation.CampaignListEntry,
) ([]campaignListRow, error) {
	rows := make([]campaignListRow, 0)
	for _, campaign := range campaigns {
		for _, runID := range campaign.RunIDs {
			summary, err := controller.RunSummary(ctx, runID)
			if err != nil {
				return nil, err
			}
			verification, err := store.LoadCampaignVerification(ctx, runID)
			if err != nil {
				verification = nil
			}
			rows = append(rows, campaignListRow{
				CampaignID:          campaign.CampaignID,
				RunID:               runID,
				Status:              evaluation.CampaignRunStatus(summary, verification),
				ExpectedAssignments: summary.ExpectedAssignment,
				Queued:              summary.QueuedCount,
				Running:             summary.RunningCount,
				Terminal:            summary.TerminalCount,
				ModelCount:          campaign.ModelCount,
				ScenarioCount:       campaign.ScenarioCount,
				RepetitionCount:     campaign.RepetitionCount,
			})
		}
	}
	return rows, nil
}

func campaignEvalStacksCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stacks",
		Short: "Heterogeneous stack generation for one campaign",
	}
	cmd.AddCommand(campaignEvalStacksGenerateCmd(deps))
	return cmd
}

func campaignEvalStacksGenerateCmd(deps nativeEvalDeps) *cobra.Command {
	var campaignID string
	var seed uint64
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate and persist the preregistered heterogeneous stack set for one campaign",
		RunE: func(cmd *cobra.Command, args []string) error {
			if campaignID == "" {
				return fmt.Errorf("evaluation: campaign stacks generate: %w", constants.ErrMissingRequiredField)
			}
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			controller := evaluation.NewCampaignController(evaluation.NewStore(fileSvc), nil, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
			stackSet, err := controller.GenerateHeterogeneousStackSet(cmd.Context(), campaignID, seed)
			if err != nil {
				return fmt.Errorf("evaluation: campaign stacks generate: %w", err)
			}
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(campaignStacksOutput{
					CampaignID:           campaignID,
					GenerationRule:       stackSet.GenerationRule,
					Seed:                 stackSet.Seed,
					SetDigest:            stackSet.SetDigest,
					StackCount:           len(stackSet.Stacks),
					HypothesisStackCount: stackSet.Coverage.HypothesisStackCount,
					CoverageStackCount:   stackSet.Coverage.CoverageStackCount,
				}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Generated %d heterogeneous stacks for campaign %s\nGeneration rule: %s\nSeed: %d\nSet digest: %s\nHypothesis stacks: %d\nCoverage stacks: %d\n",
				len(stackSet.Stacks),
				campaignID,
				stackSet.GenerationRule,
				stackSet.Seed,
				stackSet.SetDigest,
				stackSet.Coverage.HypothesisStackCount,
				stackSet.Coverage.CoverageStackCount,
			)
			return err
		},
	}
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "Frozen evaluation campaign ID")
	cmd.Flags().Uint64Var(&seed, "seed", 0, "Deterministic heterogeneous stack generation seed")
	return cmd
}

func campaignEvalScheduleCmd(deps nativeEvalDeps) *cobra.Command {
	var runID string
	var publish bool
	var heterogeneous bool
	cmd := &cobra.Command{
		Use:   "schedule [run-id]",
		Short: "Materialize and persist the assignment matrix for one run",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			runID, err = resolveCampaignRunID(cmd, deps, "schedule", runID, args)
			if err != nil {
				return err
			}
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			controller := evaluation.NewCampaignController(evaluation.NewStore(fileSvc), nil, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
			if publish {
				publication, err := newCampaignPublicationCoordinator(cmd, fileSvc)
				if err != nil {
					return fmt.Errorf("evaluation: campaign schedule: %w", err)
				}
				controller = controller.WithPublication(publication)
			}
			var count int
			if heterogeneous {
				count, err = controller.ScheduleHeterogeneousRun(cmd.Context(), runID)
			} else {
				count, err = controller.ScheduleHomogeneousRun(cmd.Context(), runID)
			}
			if err != nil {
				return fmt.Errorf("evaluation: campaign schedule: %w", err)
			}
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(campaignScheduleOutput{RunID: runID, AssignmentCount: count, Heterogeneous: heterogeneous}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			lane := "homogeneous"
			if heterogeneous {
				lane = "heterogeneous"
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Scheduled %d %s assignments for run %s\n", count, lane, runID)
			return err
		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "Campaign run ID")
	cmd.Flags().BoolVar(&publish, "publish", false, "Publish queued assignment lifecycle projections to the public mirror")
	cmd.Flags().BoolVar(&heterogeneous, "heterogeneous", false, "Materialize the heterogeneous system-lane assignment matrix")
	return cmd
}

func campaignEvalExecuteCmd(deps nativeEvalDeps) *cobra.Command {
	var runID string
	var limit uint32
	var inferenceSessionID string
	var dataSessionID string
	var ensembleURL string
	var ollamaEndpoint string
	var noAutoRefresh bool
	var publish bool
	var daemon bool
	cmd := &cobra.Command{
		Use:   "execute [run-id]",
		Short: "Execute one or more queued campaign assignments through production POST /api/v1/chat",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			runID, err = resolveCampaignRunID(cmd, deps, "execute", runID, args)
			if err != nil {
				return err
			}
			results := make([]campaignExecuteResult, 0)
			executed, err := runCampaignExecute(cmd, deps, campaignExecuteOptions{
				RunID:              runID,
				Publish:            publish,
				Daemon:             daemon,
				Limit:              limit,
				InferenceSessionID: inferenceSessionID,
				DataSessionID:      dataSessionID,
				EnsembleURL:        ensembleURL,
				OllamaEndpoint:     ollamaEndpoint,
				NoAutoRefresh:      noAutoRefresh,
				JSONOutput:         output.JSONEnabled(cmd),
				ResultOutput: func(result *evalv1.EvaluationAssignmentResult) {
					if output.JSONEnabled(cmd) && !daemon {
						results = append(results, campaignExecuteResult{
							AssignmentID: result.GetAssignmentId(),
							Status:       result.GetLifecycleStatus().String(),
							ResultDigest: result.GetResultDigest(),
						})
					}
				},
			})
			if err != nil {
				return err
			}
			if output.JSONEnabled(cmd) {
				_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
				if err != nil {
					return err
				}
				finalSummary, err := evaluation.NewCampaignController(evaluation.NewStore(fileSvc), nil, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() }).RunSummary(cmd.Context(), runID)
				if err != nil {
					return fmt.Errorf("evaluation: campaign execute: %w", err)
				}
				remaining := int64(finalSummary.ExpectedAssignment) - int64(finalSummary.TerminalCount)
				if remaining < 0 {
					remaining = 0
				}
				payload, err := json.MarshalIndent(campaignExecuteOutput{
					RunID:     runID,
					Executed:  executed,
					Remaining: remaining,
					Results:   results,
				}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Executed %d assignment(s) for run %s\n", executed, runID)
			return err

		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "Campaign run ID")
	cmd.Flags().Uint32Var(&limit, "limit", 1, "Maximum queued assignments to execute in this invocation")
	cmd.Flags().StringVar(&inferenceSessionID, "inference-session", "", "Exact inference Operator session ID")
	cmd.Flags().StringVar(&dataSessionID, "data-session", "", "Exact data Operator session ID")
	cmd.Flags().StringVar(&ensembleURL, "ensemble-url", "", "g8ee HTTP surface (default: http://localhost:8000)")
	cmd.Flags().StringVar(&ollamaEndpoint, "ollama-endpoint", "", "Approved remote Ollama endpoint for model maintenance")
	cmd.Flags().BoolVar(&daemon, "daemon", false, "Run continuously until the queued matrix is exhausted")
	cmd.Flags().BoolVar(&noAutoRefresh, "no-auto-refresh", false, "Do not refresh stale CLI operator bindings before execution")
	cmd.Flags().BoolVar(&publish, "publish", false, "Publish assignment lifecycle and terminal result projections to the public mirror")
	return cmd
}

func campaignEvalPublishCmd(deps nativeEvalDeps) *cobra.Command {
	var runID string
	var force bool
	cmd := &cobra.Command{
		Use:   "publish [run-id]",
		Short: "Publish missing public lifecycle projections for one campaign run",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			runID, err = resolveCampaignRunID(cmd, deps, "publish", runID, args)
			if err != nil {
				return err
			}
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			publication, err := newCampaignPublicationCoordinator(cmd, fileSvc)
			if err != nil {
				return fmt.Errorf("evaluation: campaign publish: %w", err)
			}
			store := evaluation.NewStore(fileSvc)
			report, reportErr := store.LoadCampaignVerification(cmd.Context(), runID)
			if reportErr != nil && !errors.Is(reportErr, constants.ErrNotFound) {
				return fmt.Errorf("evaluation: campaign publish: load verification report: %w", reportErr)
			}
			if force {
				if err := publication.ResetPublicationIdempotency(cmd.Context(), runID); err != nil {
					return fmt.Errorf("evaluation: campaign publish: %w", err)
				}
			}
			count, err := publication.PublishRunCatchUpWithVerification(cmd.Context(), runID, report)
			if err != nil {
				return fmt.Errorf("evaluation: campaign publish: %w", err)
			}
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(campaignPublishOutput{
					RunID:            runID,
					PublishedRecords: count,
					Force:            force,
				}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			if force {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Published %d public projection record(s) for run %s (forced republish)\n", count, runID)
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Published %d public projection record(s) for run %s\n", count, runID)
			return err
		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "Campaign run ID")
	cmd.Flags().BoolVar(&force, "force", false, "Clear host publication idempotency and republish all projections (use after gateway mirror volume wipe)")
	return cmd
}

func newCampaignPublicationCoordinator(cmd *cobra.Command, fileSvc fs.RuntimeFileService) (*evaluation.CampaignPublicationCoordinator, error) {
	projectRoot, err := cmd.Flags().GetString("project-root")
	if err != nil {
		return nil, fmt.Errorf("campaign publication: read project root: %w", err)
	}
	cfg, err := loadConfig(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("campaign publication: load config: %w", err)
	}
	exporter, err := newCampaignFeedExporter(commandContext(cmd), fileSvc, cfg)
	if err != nil {
		return nil, err
	}
	remote, err := newProviderObservationRemote(fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("campaign publication: provider observation remote: %w", err)
	}
	publicationState, err := newGatewayCampaignPublicationStateStoreFromConfig(fileSvc, cfg)
	if err != nil {
		return nil, err
	}
	coordinator := evaluation.NewCampaignPublicationCoordinator(
		evaluation.NewStore(fileSvc),
		fileSvc,
		publicationState,
		exporter,
		remote,
	)
	if isGatewayHealthy() {
		coordinator.WithMirrorProbe(newHTTPCampaignMirrorProbe(commandContext(cmd)))
	}
	return coordinator, nil
}

func campaignEvalVerifyCmd(deps nativeEvalDeps) *cobra.Command {
	var runID string
	var requireProviderObservation bool
	var requireModelProvenance bool
	cmd := &cobra.Command{
		Use:   "verify [run-id]",
		Short: "Independently verify persisted campaign assignment results for one run",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			runID, err = resolveCampaignRunID(cmd, deps, "verify", runID, args)
			if err != nil {
				return err
			}
			report, err := verifyCampaignRun(cmd, deps, runID, requireProviderObservation, requireModelProvenance, output.JSONEnabled(cmd))
			if err != nil {
				return err
			}
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(campaignVerifyOutput{
					RunID:          runID,
					Status:         report.GetStatus().String(),
					FailureCount:   report.GetFailureCount(),
					FailureReasons: report.GetFailureReasons(),
				}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Run %s verification: %s (%d failure(s))\n", runID, report.GetStatus().String(), report.GetFailureCount())
			if report.GetFailureCount() > 0 {
				for _, reason := range report.GetFailureReasons() {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", reason)
				}
				return constants.ErrEvalRunVerificationFailed
			}
			return err
		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "Campaign run ID")
	cmd.Flags().BoolVar(&requireProviderObservation, "require-provider-observation", false, "Fail verification when provider-boundary observation windows are missing or incomplete")
	cmd.Flags().BoolVar(&requireModelProvenance, "require-model-provenance", false, "Fail verification when model provenance attestation windows are missing or digest_match is false")
	return cmd
}

func campaignEvalAccountCmd(deps nativeEvalDeps) *cobra.Command {
	var runID string
	cmd := &cobra.Command{
		Use:   "account [run-id]",
		Short: "Verify matrix population coverage for one campaign run",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			runID, err = resolveCampaignRunID(cmd, deps, "account", runID, args)
			if err != nil {
				return err
			}
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			store := evaluation.NewStore(fileSvc)
			run, err := store.LoadRun(cmd.Context(), runID)
			if err != nil {
				return fmt.Errorf("evaluation: campaign account: %w", err)
			}
			catalog, err := store.LoadScenarioCatalog(cmd.Context(), run.GetCampaignBinding().GetCampaignId())
			if err != nil {
				return fmt.Errorf("evaluation: campaign account: %w", err)
			}
			report, err := evaluation.NewCampaignPopulationAccountant(deps.now).AccountRun(cmd.Context(), store, runID, catalog)
			if err != nil {
				return fmt.Errorf("evaluation: campaign account: %w", err)
			}
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(campaignAccountOutput{
					RunID:                 runID,
					Complete:              report.Complete,
					ExpectedCells:         report.ExpectedCells,
					ScheduledAssignments:  report.ScheduledAssignments,
					Queued:                report.QueuedCount,
					Running:               report.RunningCount,
					Terminal:              report.TerminalCount,
					Stopped:               report.StoppedCount,
					DispositionCounts:     report.DispositionCounts,
					MissingCells:          report.MissingCells,
					DuplicateIdentities:   report.DuplicateIdentities,
					ExtraAssignments:      report.ExtraAssignments,
					TerminalWithoutResult: report.TerminalWithoutResult,
					ResultWithoutTerminal: report.ResultWithoutTerminal,
					FailureReasons:        report.FailureReasons,
					AccountedAt:           report.AccountedAt.Format(time.RFC3339),
				}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Run %s population accounting: %s\nExpected cells: %d\nScheduled: %d\nQueued: %d\nRunning: %d\nTerminal: %d\nStopped: %d\n",
				runID,
				map[bool]string{true: "complete", false: "incomplete"}[report.Complete],
				report.ExpectedCells,
				report.ScheduledAssignments,
				report.QueuedCount,
				report.RunningCount,
				report.TerminalCount,
				report.StoppedCount,
			)
			if len(report.FailureReasons) > 0 {
				for _, reason := range report.FailureReasons {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", reason)
				}
			}
			if !report.Complete {
				return constants.ErrEvalRunVerificationFailed
			}
			return err
		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "Campaign run ID")
	return cmd
}

func campaignEvalExportCmd(deps nativeEvalDeps) *cobra.Command {
	var runID string
	var outputDir string
	cmd := &cobra.Command{
		Use:   "export [run-id]",
		Short: "Generate disclosure-safe JSONL, CSV, and SQLite exports for one campaign run",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			runID, err = resolveCampaignRunID(cmd, deps, "export", runID, args)
			if err != nil {
				return err
			}
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			store := evaluation.NewStore(fileSvc)
			report, err := evaluation.NewCampaignExporter(deps.now).ExportRun(cmd.Context(), store, fileSvc, runID, outputDir)
			if err != nil {
				return fmt.Errorf("evaluation: campaign export: %w", err)
			}
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(campaignExportOutput{
					RunID:               report.RunID,
					CampaignID:          report.CampaignID,
					OutputDir:           report.OutputDir,
					ExportedAt:          report.ExportedAt.Format(time.RFC3339),
					AssignmentCount:     report.AssignmentCount,
					TerminalResultCount: report.TerminalResultCount,
					Files:               report.Files,
				}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Exported run %s to %s\nAssignments: %d scheduled, %d terminal results\n",
				report.RunID,
				report.OutputDir,
				report.AssignmentCount,
				report.TerminalResultCount,
			)
			if err != nil {
				return err
			}
			for _, file := range report.Files {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "- %s (%s, %d record(s))\n", file.Name, file.Format, file.RecordCount)
				if err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "Campaign run ID")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Export output directory (default: .g8e/data/eval/runs/<run-id>/export)")
	return cmd
}

func campaignEvalRepairCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repair",
		Short: "Backfill or recompute persisted campaign assignment records",
	}
	cmd.AddCommand(
		campaignEvalRepairResultsCmd(deps),
		campaignEvalRepairTraceDigestsCmd(deps),
	)
	return cmd
}

func campaignEvalRepairTraceDigestsCmd(deps nativeEvalDeps) *cobra.Command {
	var runID string
	cmd := &cobra.Command{
		Use:   "trace-digests [run-id]",
		Short: "Recompute persisted chat-probe trace digests using authoritative g8e canonical JSON",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			runID, err = resolveCampaignRunID(cmd, deps, "repair trace-digests", runID, args)
			if err != nil {
				return err
			}
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			controller := evaluation.NewCampaignController(evaluation.NewStore(fileSvc), nil, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
			repaired, err := controller.RepairAssignmentTraceDigests(cmd.Context(), runID)
			if err != nil {
				return fmt.Errorf("evaluation: campaign repair trace-digests: %w", err)
			}
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(campaignRepairOutput{RunID: runID, Count: repaired}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Repaired %d assignment trace digest(s) for run %s\n", repaired, runID)
			return err
		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "Campaign run ID")
	return cmd
}

func campaignEvalRepairResultsCmd(deps nativeEvalDeps) *cobra.Command {
	var runID string
	cmd := &cobra.Command{
		Use:   "results [run-id]",
		Short: "Backfill persisted terminal results for assignments missing result records",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			runID, err = resolveCampaignRunID(cmd, deps, "repair results", runID, args)
			if err != nil {
				return err
			}
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			_, artifacts, err := evaluation.LoadScenarioCatalog()
			if err != nil {
				return fmt.Errorf("evaluation: campaign repair results: %w", err)
			}
			controller := evaluation.NewCampaignController(evaluation.NewStore(fileSvc), nil, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
			repaired, err := controller.RepairAssignmentsWithoutResults(cmd.Context(), runID, artifacts)
			if err != nil {
				return fmt.Errorf("evaluation: campaign repair results: %w", err)
			}
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(campaignRepairOutput{RunID: runID, Results: repaired}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Repaired %d terminal assignment result(s) for run %s\n", repaired, runID)
			return err
		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "Campaign run ID")
	return cmd
}

func campaignEvalShowCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <run-id>",
		Short: "Show one persisted campaign run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runID := args[0]
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			store := evaluation.NewStore(fileSvc)
			controller := evaluation.NewCampaignController(store, nil, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
			summary, err := controller.RunSummary(cmd.Context(), runID)
			if err != nil {
				return fmt.Errorf("evaluation: campaign show: %w", err)
			}
			campaignID := summary.Run.GetCampaignBinding().GetCampaignId()
			spec, err := store.LoadCampaignSpec(cmd.Context(), campaignID)
			if err != nil {
				return fmt.Errorf("evaluation: campaign show: %w", err)
			}
			hasStacks, err := store.LoadHeterogeneousStackSet(cmd.Context(), campaignID)
			hasHeterogeneousStacks := err == nil && hasStacks != nil
			startedAt := ""
			if summary.Run.GetStartedAt() != nil {
				startedAt = summary.Run.GetStartedAt().AsTime().Format(time.RFC3339)
			}
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(campaignShowOutput{
					RunID:                  runID,
					CampaignID:             campaignID,
					StartedAt:              startedAt,
					Lane:                   summary.Run.GetLane().String(),
					ModelCount:             len(spec.GetModelRegistry()),
					ScenarioCount:          spec.GetScenarioCount(),
					RepetitionCount:        spec.GetRepetitionCount(),
					ModelRegistryDigest:    spec.GetModelRegistryDigest(),
					CatalogDigest:          spec.GetCatalogDigest(),
					HasHeterogeneousStacks: hasHeterogeneousStacks,
					ExpectedAssignments:    summary.ExpectedAssignment,
					Queued:                 summary.QueuedCount,
					Running:                summary.RunningCount,
					Terminal:               summary.TerminalCount,
					NextAssignmentID:       summary.NextAssignmentID,
				}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Run: %s\nCampaign: %s\nStarted: %s\nLane: %s\nModels: %d\nScenarios: %d\nRepetitions: %d\nModel registry digest: %s\nCatalog digest: %s\nHeterogeneous stacks: %t\nExpected assignments: %d\nQueued: %d\nRunning: %d\nTerminal: %d\nNext assignment: %s\n",
				runID,
				campaignID,
				startedAt,
				summary.Run.GetLane().String(),
				len(spec.GetModelRegistry()),
				spec.GetScenarioCount(),
				spec.GetRepetitionCount(),
				spec.GetModelRegistryDigest(),
				spec.GetCatalogDigest(),
				hasHeterogeneousStacks,
				summary.ExpectedAssignment,
				summary.QueuedCount,
				summary.RunningCount,
				summary.TerminalCount,
				summary.NextAssignmentID,
			)
			return err
		},
	}
	return cmd
}

func campaignEvalStatusCmd(deps nativeEvalDeps) *cobra.Command {
	var runID string
	cmd := &cobra.Command{
		Use:   "status [run-id]",
		Short: "Report resumable campaign run status from canonical assignment records",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			runID, err = resolveCampaignRunID(cmd, deps, "status", runID, args)
			if err != nil {
				return err
			}
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			controller := evaluation.NewCampaignController(evaluation.NewStore(fileSvc), nil, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
			summary, err := controller.RunSummary(cmd.Context(), runID)
			if err != nil {
				return fmt.Errorf("evaluation: campaign status: %w", err)
			}
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(campaignStatusOutput{
					RunID:               runID,
					CampaignID:          summary.Run.GetCampaignBinding().GetCampaignId(),
					ExpectedAssignments: summary.ExpectedAssignment,
					Queued:              summary.QueuedCount,
					Running:             summary.RunningCount,
					Terminal:            summary.TerminalCount,
					NextAssignmentID:    summary.NextAssignmentID,
				}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Run: %s\nCampaign: %s\nExpected assignments: %d\nQueued: %d\nRunning: %d\nTerminal: %d\nNext assignment: %s\n",
				runID,
				summary.Run.GetCampaignBinding().GetCampaignId(),
				summary.ExpectedAssignment,
				summary.QueuedCount,
				summary.RunningCount,
				summary.TerminalCount,
				summary.NextAssignmentID,
			)
			return err
		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "Campaign run ID")
	return cmd
}

func resolveCampaignOllamaEndpoint(flag string, operators []models.OperatorDocumentGo, inferenceSessionID string) (string, error) {
	endpoint, err := evaluation.ResolveInferenceOllamaEndpoint(flag, operators, inferenceSessionID)
	if err != nil {
		return "", fmt.Errorf("evaluation: campaign execute: %w", err)
	}
	return endpoint, nil
}
