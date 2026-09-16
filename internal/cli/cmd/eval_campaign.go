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
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/gateway"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
)

func campaignEvalCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "campaign",
		Short: "Initialize, schedule, and resume North Star model campaigns",
	}
	cmd.AddCommand(
		campaignEvalInitCmd(deps),
		campaignEvalScheduleCmd(deps),
		campaignEvalStacksGenerateCmd(deps),
		campaignEvalScheduleHeterogeneousCmd(deps),
		campaignEvalExecuteCmd(deps),
		campaignEvalPublishCmd(deps),
		campaignEvalVerifyCmd(deps),
		campaignEvalAccountCmd(deps),
		campaignEvalExportCmd(deps),
		campaignEvalRepairResultsCmd(deps),
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
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a North Star campaign run with frozen catalog and model registry",
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
			inventory, err := evaluation.LoadModelInventoryFreezeFile(inventoryFile)
			if err != nil {
				return err
			}
			if inventory.CampaignID != "" && inventory.CampaignID != campaignID {
				return fmt.Errorf("evaluation: campaign init: inventory campaign_id mismatch")
			}
			inventory.CampaignID = campaignID
			catalog, artifacts, err := evaluation.LoadNorthStarScenarioCatalog()
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
			if jsonOutput {
				payload, err := json.MarshalIndent(map[string]any{
					"campaign_id":           campaignID,
					"run_id":                runID,
					"catalog_digest":        catalog.GetCatalogDigest(),
					"model_registry_digest": inventory.RegistryDigest,
					"model_count":           len(inventory.Variants),
					"scenario_count":        len(catalog.GetScenarios()),
					"project_root":          cfg.ProjectRoot,
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
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON status")
	return cmd
}

func campaignEvalStacksGenerateCmd(deps nativeEvalDeps) *cobra.Command {
	var campaignID string
	var seed uint64
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "stacks-generate",
		Short: "Generate and persist the preregistered heterogeneous stack set for one campaign",
		RunE: func(cmd *cobra.Command, args []string) error {
			if campaignID == "" {
				return fmt.Errorf("evaluation: campaign stacks-generate: %w", constants.ErrMissingRequiredField)
			}
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			controller := evaluation.NewCampaignController(evaluation.NewStore(fileSvc), nil, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
			stackSet, err := controller.GenerateHeterogeneousStackSet(cmd.Context(), campaignID, seed)
			if err != nil {
				return fmt.Errorf("evaluation: campaign stacks-generate: %w", err)
			}
			if jsonOutput {
				payload, err := json.MarshalIndent(map[string]any{
					"campaign_id":            campaignID,
					"generation_rule":        stackSet.GenerationRule,
					"seed":                   stackSet.Seed,
					"set_digest":             stackSet.SetDigest,
					"stack_count":            len(stackSet.Stacks),
					"hypothesis_stack_count": stackSet.Coverage.HypothesisStackCount,
					"coverage_stack_count":   stackSet.Coverage.CoverageStackCount,
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
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON status")
	return cmd
}

func campaignEvalScheduleHeterogeneousCmd(deps nativeEvalDeps) *cobra.Command {
	var runID string
	var jsonOutput bool
	var publish bool
	cmd := &cobra.Command{
		Use:   "schedule-heterogeneous",
		Short: "Materialize and persist the heterogeneous system-lane assignment matrix for one run",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runID == "" {
				return fmt.Errorf("evaluation: campaign schedule-heterogeneous: %w", constants.ErrMissingRequiredField)
			}
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			controller := evaluation.NewCampaignController(evaluation.NewStore(fileSvc), nil, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
			if publish {
				publication, err := newCampaignPublicationCoordinator(cmd, fileSvc)
				if err != nil {
					return fmt.Errorf("evaluation: campaign schedule-heterogeneous: %w", err)
				}
				controller = controller.WithPublication(publication)
			}
			count, err := controller.ScheduleHeterogeneousRun(cmd.Context(), runID)
			if err != nil {
				return fmt.Errorf("evaluation: campaign schedule-heterogeneous: %w", err)
			}
			if jsonOutput {
				payload, err := json.MarshalIndent(map[string]any{"run_id": runID, "assignment_count": count}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Scheduled %d heterogeneous assignments for run %s\n", count, runID)
			return err
		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "Campaign run ID")
	cmd.Flags().BoolVar(&publish, "publish", false, "Publish queued assignment lifecycle projections to the public mirror")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON status")
	return cmd
}

func campaignEvalScheduleCmd(deps nativeEvalDeps) *cobra.Command {
	var runID string
	var jsonOutput bool
	var publish bool
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Materialize and persist the homogeneous assignment matrix for one run",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runID == "" {
				return fmt.Errorf("evaluation: campaign schedule: %w", constants.ErrMissingRequiredField)
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
			count, err := controller.ScheduleHomogeneousRun(cmd.Context(), runID)
			if err != nil {
				return fmt.Errorf("evaluation: campaign schedule: %w", err)
			}
			if jsonOutput {
				payload, err := json.MarshalIndent(map[string]any{"run_id": runID, "assignment_count": count}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Scheduled %d homogeneous assignments for run %s\n", count, runID)
			return err
		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "Campaign run ID")
	cmd.Flags().BoolVar(&publish, "publish", false, "Publish queued assignment lifecycle projections to the public mirror")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON status")
	return cmd
}

func campaignEvalExecuteCmd(deps nativeEvalDeps) *cobra.Command {
	var runID string
	var limit uint32
	var inferenceSessionID string
	var dataSessionID string
	var ensembleURL string
	var jsonOutput bool
	var noAutoRefresh bool
	var publish bool
	cmd := &cobra.Command{
		Use:   "execute",
		Short: "Execute one or more queued North Star campaign assignments through production POST /api/v1/chat",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runID == "" {
				return fmt.Errorf("evaluation: campaign execute: %w", constants.ErrMissingRequiredField)
			}
			if limit == 0 {
				limit = 1
			}
			cfg, fileSvc, authContext, err := chatEvalEnvironment(cmd, chatEvalDeps{
				configLoader:         deps.configLoader,
				fileSvcFactory:       deps.fileSvcFactory,
				authLoader:           deps.authLoader,
				clientFactory:        deps.clientFactory,
				refreshClientFactory: defaultRefreshClientFactory,
				now:                  deps.now,
				newID:                deps.newID,
			})
			if err != nil {
				return err
			}
			store := evaluation.NewStore(fileSvc)
			summary, err := evaluation.NewCampaignController(store, nil, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() }).RunSummary(cmd.Context(), runID)
			if err != nil {
				return fmt.Errorf("evaluation: campaign execute: %w", err)
			}
			binding := summary.Run.GetCampaignBinding()
			if binding == nil {
				return fmt.Errorf("evaluation: campaign execute: missing campaign binding")
			}
			if inferenceSessionID == "" {
				inferenceSessionID = binding.GetInferenceOperatorSessionId()
			}
			if dataSessionID == "" {
				dataSessionID = binding.GetDataOperatorSessionId()
			}
			operators, err := chatEvalListOperators(cmd, chatEvalDeps{
				configLoader:   deps.configLoader,
				fileSvcFactory: deps.fileSvcFactory,
				authLoader:     deps.authLoader,
				clientFactory:  deps.clientFactory,
				now:            deps.now,
				newID:          deps.newID,
			}, cfg, authContext)
			if err != nil {
				return err
			}
			if !noAutoRefresh {
				authContext, err = chatEvalEnsureOperatorBinding(cmd, chatEvalDeps{
					configLoader:         deps.configLoader,
					fileSvcFactory:       deps.fileSvcFactory,
					authLoader:           deps.authLoader,
					clientFactory:        deps.clientFactory,
					refreshClientFactory: defaultRefreshClientFactory,
					now:                  deps.now,
					newID:                deps.newID,
				}, cfg, fileSvc, authContext, operators, dataSessionID)
				if err != nil {
					return fmt.Errorf("evaluation: campaign execute: %w", err)
				}
			}
			selected, err := evaluation.SelectInferenceOperator(operators, inferenceSessionID)
			if err != nil {
				return err
			}
			dataOperator, err := chatEvalResolveDataOperator(operators, authContext, dataSessionID)
			if err != nil {
				return fmt.Errorf("evaluation: campaign execute: %w", err)
			}
			spec, err := store.LoadCampaignSpec(cmd.Context(), binding.GetCampaignId())
			if err != nil {
				return fmt.Errorf("evaluation: campaign execute: %w", err)
			}
			_, artifacts, err := evaluation.LoadNorthStarScenarioCatalog()
			if err != nil {
				return fmt.Errorf("evaluation: campaign execute: %w", err)
			}
			ensembleClient, err := chatEvalEnsembleClient(cfg, authContext, resolveChatEvalEnsembleURL(ensembleURL), chatEvalDeps{
				configLoader:   deps.configLoader,
				fileSvcFactory: deps.fileSvcFactory,
				authLoader:     deps.authLoader,
				clientFactory:  deps.clientFactory,
				now:            deps.now,
				newID:          deps.newID,
			})
			if err != nil {
				return err
			}
			persona := harnessclient.Persona{
				ID:                "g8e-campaign-controller",
				UserAgent:         "g8e-eval-campaign",
				UserID:            authContext.UserID,
				CLISessionID:      authContext.CLISessionID,
				OperatorID:        dataOperator.OperatorID,
				OperatorSessionID: dataOperator.OperatorSessionID,
			}
			executor := evaluation.NewCampaignChatExecutor(
				ensembleClient,
				persona,
				dataOperator.OperatorID,
				dataOperator.OperatorSessionID,
				store,
				func(ctx context.Context, fetch func(context.Context) (map[string]any, error)) (map[string]any, error) {
					return chatEvalWaitForTrace(ctx, fetch, newChatAcceptReporter(cmd.OutOrStdout(), jsonOutput))
				},
				deps.now,
				func(prefix string) string { return prefix + "-" + deps.newID() },
			)
			controller := evaluation.NewCampaignController(store, executor, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
			if publish {
				publication, err := newCampaignPublicationCoordinator(cmd, fileSvc)
				if err != nil {
					return fmt.Errorf("evaluation: campaign execute: %w", err)
				}
				controller = controller.WithPublication(publication)
			}
			executionBinding := evaluation.CampaignExecutionBinding{
				InferenceOperatorSessionID: selected.OperatorSessionID,
				DataOperatorID:             dataOperator.OperatorID,
				DataOperatorSessionID:      dataOperator.OperatorSessionID,
				ModelRegistryDigest:        spec.GetModelRegistryDigest(),
				ModelRegistry:              evaluation.InferenceVariantsFromEvalRegistry(spec.GetModelRegistry()),
			}
			results := make([]map[string]any, 0, limit)
			executed := 0
			for i := 0; i < int(limit); i++ {
				ctx, cancel := context.WithTimeout(cmd.Context(), 8*time.Minute)
				result, ok, err := controller.ExecuteNextAssignment(ctx, runID, executionBinding, artifacts)
				cancel()
				if err != nil {
					return fmt.Errorf("evaluation: campaign execute: %w", err)
				}
				if !ok {
					break
				}
				executed++
				entry := map[string]any{
					"assignment_id": result.GetAssignmentId(),
					"status":        result.GetLifecycleStatus().String(),
					"result_digest": result.GetResultDigest(),
				}
				results = append(results, entry)
				if !jsonOutput {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Executed %s: %s\n", result.GetAssignmentId(), result.GetLifecycleStatus().String())
				}
			}
			if jsonOutput {
				payload, err := json.MarshalIndent(map[string]any{
					"run_id":    runID,
					"executed":  executed,
					"remaining": int64(summary.ExpectedAssignment) - int64(summary.TerminalCount) - int64(executed),
					"results":   results,
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
	cmd.Flags().BoolVar(&noAutoRefresh, "no-auto-refresh", false, "Do not refresh stale CLI operator bindings before execution")
	cmd.Flags().BoolVar(&publish, "publish", false, "Publish assignment lifecycle and terminal result projections to the public mirror")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON status")
	return cmd
}

func campaignEvalPublishCmd(deps nativeEvalDeps) *cobra.Command {
	var runID string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish missing public lifecycle projections for one campaign run",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runID == "" {
				return fmt.Errorf("evaluation: campaign publish: %w", constants.ErrMissingRequiredField)
			}
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			publication, err := newCampaignPublicationCoordinator(cmd, fileSvc)
			if err != nil {
				return fmt.Errorf("evaluation: campaign publish: %w", err)
			}
			count, err := publication.PublishRunCatchUp(cmd.Context(), runID)
			if err != nil {
				return fmt.Errorf("evaluation: campaign publish: %w", err)
			}
			if jsonOutput {
				payload, err := json.MarshalIndent(map[string]any{"run_id": runID, "published_records": count}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Published %d public projection record(s) for run %s\n", count, runID)
			return err
		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "Campaign run ID")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON status")
	return cmd
}

type gatewayCampaignFeedExporter struct {
	publisher *gateway.PublicPublisherService
}

func (e *gatewayCampaignFeedExporter) HighWaterSequence(ctx context.Context) (int64, error) {
	snapshot, err := e.publisher.GetSnapshot(ctx)
	if err == nil {
		return snapshot.HighWaterSequence, nil
	}
	if errors.Is(err, constants.ErrPublicFeedSnapshotNotFound) {
		return 0, nil
	}
	return 0, err
}

func (e *gatewayCampaignFeedExporter) ExportBatch(ctx context.Context, records []evaluation.CampaignPublicFeedRecord) error {
	batch := make([]models.PublicFeedRecord, len(records))
	for index, record := range records {
		batch[index] = models.PublicFeedRecord{
			Sequence:    record.Sequence,
			RecordType:  models.PublicFeedRecordTypeProjection,
			RecordHash:  record.RecordHash,
			RecordBytes: record.RecordBytes,
		}
	}
	return e.publisher.ExportBatch(ctx, batch)
}

func newCampaignPublicationCoordinator(cmd *cobra.Command, fileSvc fs.RuntimeFileService) (*evaluation.CampaignPublicationCoordinator, error) {
	exportConfig, err := readPublicExportConfig(commandContext(cmd), fileSvc)
	if err != nil {
		return nil, err
	}
	if !exportConfig.Enabled {
		return nil, constants.ErrPublicFeedDisabled
	}
	publisher, err := newPublicPublisherForCommand(commandContext(cmd), fileSvc, exportConfig)
	if err != nil {
		return nil, err
	}
	if exportConfig.MirrorOrigin != "" {
		publisher.SetMirrorOrigin(exportConfig.MirrorOrigin)
	}
	return evaluation.NewCampaignPublicationCoordinator(
		evaluation.NewStore(fileSvc),
		fileSvc,
		&gatewayCampaignFeedExporter{publisher: publisher},
	), nil
}

func campaignEvalVerifyCmd(deps nativeEvalDeps) *cobra.Command {
	var runID string
	var jsonOutput bool
	var requireProviderObservation bool
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Independently verify persisted campaign assignment results for one run",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runID == "" {
				return fmt.Errorf("evaluation: campaign verify: %w", constants.ErrMissingRequiredField)
			}
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			store := evaluation.NewStore(fileSvc)
			run, err := store.LoadRun(cmd.Context(), runID)
			if err != nil {
				return fmt.Errorf("evaluation: campaign verify: %w", err)
			}
			catalog, err := store.LoadScenarioCatalog(cmd.Context(), run.GetCampaignBinding().GetCampaignId())
			if err != nil {
				return fmt.Errorf("evaluation: campaign verify: %w", err)
			}
			_, artifacts, err := evaluation.LoadNorthStarScenarioCatalog()
			if err != nil {
				return fmt.Errorf("evaluation: campaign verify: %w", err)
			}
			verifier := evaluation.NewCampaignRunVerifier(deps.now)
			observationReader, err := evaluation.NewCampaignProviderObservationReader(fileSvc)
			if err != nil {
				return fmt.Errorf("evaluation: campaign verify: %w", err)
			}
			policy := evaluation.ProviderObservationPolicyInterim
			if requireProviderObservation {
				policy = evaluation.ProviderObservationPolicyStrict
			}
			verifier = verifier.WithProviderObservationReader(observationReader, policy)
			report, err := verifier.VerifyRun(cmd.Context(), store, runID, catalog, artifacts)
			if err != nil {
				return fmt.Errorf("evaluation: campaign verify: %w", err)
			}
			if err := store.SaveCampaignVerification(cmd.Context(), runID, report); err != nil {
				return fmt.Errorf("evaluation: campaign verify: %w", err)
			}
			if jsonOutput {
				payload, err := json.MarshalIndent(map[string]any{
					"run_id":          runID,
					"status":          report.GetStatus().String(),
					"failure_count":   report.GetFailureCount(),
					"failure_reasons": report.GetFailureReasons(),
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
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON status")
	return cmd
}

func campaignEvalAccountCmd(deps nativeEvalDeps) *cobra.Command {
	var runID string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "account",
		Short: "Verify homogeneous matrix population accounting for one campaign run",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runID == "" {
				return fmt.Errorf("evaluation: campaign account: %w", constants.ErrMissingRequiredField)
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
			if jsonOutput {
				payload, err := json.MarshalIndent(map[string]any{
					"run_id":                  runID,
					"complete":                report.Complete,
					"expected_cells":          report.ExpectedCells,
					"scheduled_assignments":   report.ScheduledAssignments,
					"queued":                  report.QueuedCount,
					"running":                 report.RunningCount,
					"terminal":                report.TerminalCount,
					"stopped":                 report.StoppedCount,
					"disposition_counts":      report.DispositionCounts,
					"missing_cells":           report.MissingCells,
					"duplicate_identities":    report.DuplicateIdentities,
					"extra_assignments":       report.ExtraAssignments,
					"terminal_without_result": report.TerminalWithoutResult,
					"result_without_terminal": report.ResultWithoutTerminal,
					"failure_reasons":         report.FailureReasons,
					"accounted_at":            report.AccountedAt.Format(time.RFC3339),
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
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON status")
	return cmd
}

func campaignEvalExportCmd(deps nativeEvalDeps) *cobra.Command {
	var runID string
	var outputDir string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Generate disclosure-safe JSONL, CSV, and SQLite exports for one campaign run",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runID == "" {
				return fmt.Errorf("evaluation: campaign export: %w", constants.ErrMissingRequiredField)
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
			if jsonOutput {
				payload, err := json.MarshalIndent(map[string]any{
					"run_id":                report.RunID,
					"campaign_id":           report.CampaignID,
					"output_dir":            report.OutputDir,
					"exported_at":           report.ExportedAt.Format(time.RFC3339),
					"assignment_count":      report.AssignmentCount,
					"terminal_result_count": report.TerminalResultCount,
					"files":                 report.Files,
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
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON status")
	return cmd
}

func campaignEvalRepairResultsCmd(deps nativeEvalDeps) *cobra.Command {
	var runID string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "repair-results",
		Short: "Backfill persisted terminal results for assignments missing result records",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runID == "" {
				return fmt.Errorf("evaluation: campaign repair-results: %w", constants.ErrMissingRequiredField)
			}
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			_, artifacts, err := evaluation.LoadNorthStarScenarioCatalog()
			if err != nil {
				return fmt.Errorf("evaluation: campaign repair-results: %w", err)
			}
			controller := evaluation.NewCampaignController(evaluation.NewStore(fileSvc), nil, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
			repaired, err := controller.RepairAssignmentsWithoutResults(cmd.Context(), runID, artifacts)
			if err != nil {
				return fmt.Errorf("evaluation: campaign repair-results: %w", err)
			}
			if jsonOutput {
				payload, err := json.MarshalIndent(map[string]any{"run_id": runID, "repaired_results": repaired}, "", "  ")
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
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON status")
	return cmd
}

func campaignEvalStatusCmd(deps nativeEvalDeps) *cobra.Command {
	var runID string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Report resumable campaign run status from canonical assignment records",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runID == "" {
				return fmt.Errorf("evaluation: campaign status: %w", constants.ErrMissingRequiredField)
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
			if jsonOutput {
				payload, err := json.MarshalIndent(map[string]any{
					"run_id":               runID,
					"campaign_id":          summary.Run.GetCampaignBinding().GetCampaignId(),
					"expected_assignments": summary.ExpectedAssignment,
					"queued":               summary.QueuedCount,
					"running":              summary.RunningCount,
					"terminal":             summary.TerminalCount,
					"next_assignment_id":   summary.NextAssignmentID,
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
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON status")
	return cmd
}
