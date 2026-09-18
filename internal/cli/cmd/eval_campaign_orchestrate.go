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
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type campaignOperatorSessions struct {
	InferenceSessionID string
	DataSessionID      string
	DataOperatorID     string
}

type campaignExecuteOptions struct {
	RunID               string
	Publish             bool
	Daemon              bool
	Limit               uint32
	InferenceSessionID  string
	DataSessionID       string
	EnsembleURL         string
	OllamaEndpoint      string
	NoAutoRefresh       bool
	WaitForProviderIdle bool
	ProviderIdlePoll    time.Duration
	ProviderSettle      time.Duration
	JSONOutput          bool
}

func resolveCampaignOperatorSessions(
	cmd *cobra.Command,
	deps nativeEvalDeps,
	cfg *config.Config,
	inferenceSessionID string,
	dataSessionID string,
) (campaignOperatorSessions, error) {
	_, _, authContext, err := chatEvalEnvironment(cmd, chatEvalDeps{
		configLoader:         deps.configLoader,
		fileSvcFactory:       deps.fileSvcFactory,
		authLoader:           deps.authLoader,
		clientFactory:        deps.clientFactory,
		refreshClientFactory: defaultRefreshClientFactory,
		now:                  deps.now,
		newID:                deps.newID,
	})
	if err != nil {
		return campaignOperatorSessions{}, err
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
		return campaignOperatorSessions{}, err
	}
	inferenceOperator, err := evaluation.SelectInferenceOperator(operators, inferenceSessionID)
	if err != nil {
		return campaignOperatorSessions{}, err
	}
	dataOperator, err := evaluation.SelectCampaignDataOperator(operators, dataSessionID)
	if err != nil {
		return campaignOperatorSessions{}, err
	}
	return campaignOperatorSessions{
		InferenceSessionID: inferenceOperator.OperatorSessionID,
		DataSessionID:      dataOperator.OperatorSessionID,
		DataOperatorID:     dataOperator.OperatorID,
	}, nil
}

func initializeCampaignRun(
	cmd *cobra.Command,
	deps nativeEvalDeps,
	plan *evaluation.CampaignStartPlan,
	sessions campaignOperatorSessions,
) error {
	cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
	if err != nil {
		return err
	}
	authContext, err := deps.authLoader(fileSvc, cfg)
	if err != nil {
		return fmt.Errorf("evaluation: campaign init: load CLI identity: %w", err)
	}
	inventory, err := evaluation.LoadModelInventoryFreezeFile(plan.InventoryPath)
	if err != nil {
		return err
	}
	if inventory.CampaignID != "" && inventory.CampaignID != plan.CampaignID {
		return fmt.Errorf("evaluation: campaign init: inventory campaign_id mismatch")
	}
	inventory.CampaignID = plan.CampaignID
	catalog, artifacts, err := evaluation.LoadNorthStarScenarioCatalog()
	if err != nil {
		return fmt.Errorf("evaluation: campaign init: %w", err)
	}
	controller := evaluation.NewCampaignController(evaluation.NewStore(fileSvc), nil, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
	_, err = controller.InitializeCampaign(cmd.Context(), evaluation.CampaignInitRequest{
		CampaignID:                 plan.CampaignID,
		RunID:                      plan.RunID,
		Catalog:                    catalog,
		Inventory:                  inventory,
		ScenarioArtifacts:          artifacts,
		RepetitionCount:            1,
		InferenceOperatorSessionID: sessions.InferenceSessionID,
		DataOperatorSessionID:      sessions.DataSessionID,
		Deployment:                 nativeEvalDeployment(cfg, authContext, plan.RunID, ""),
	})
	if err != nil {
		return fmt.Errorf("evaluation: campaign init: %w", err)
	}
	return nil
}

func scheduleHomogeneousCampaignRun(cmd *cobra.Command, deps nativeEvalDeps, runID string, publish bool) (int, error) {
	_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
	if err != nil {
		return 0, err
	}
	controller := evaluation.NewCampaignController(evaluation.NewStore(fileSvc), nil, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
	if publish {
		publication, err := newCampaignPublicationCoordinator(cmd, fileSvc)
		if err != nil {
			return 0, fmt.Errorf("evaluation: campaign schedule: %w", err)
		}
		controller = controller.WithPublication(publication)
	}
	return controller.ScheduleHomogeneousRun(cmd.Context(), runID)
}

func runCampaignExecute(cmd *cobra.Command, deps nativeEvalDeps, opts campaignExecuteOptions) (int, error) {
	if opts.Daemon {
		opts.Limit = ^uint32(0)
	} else if opts.Limit == 0 {
		opts.Limit = 1
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
		return 0, err
	}
	store := evaluation.NewStore(fileSvc)
	controllerFactory := func(executor evaluation.CampaignAssignmentExecutor) *evaluation.CampaignController {
		return evaluation.NewCampaignController(store, executor, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
	}
	summary, err := controllerFactory(nil).RunSummary(cmd.Context(), opts.RunID)
	if err != nil {
		return 0, fmt.Errorf("evaluation: campaign execute: %w", err)
	}
	binding := summary.Run.GetCampaignBinding()
	if binding == nil {
		return 0, fmt.Errorf("evaluation: campaign execute: missing campaign binding")
	}
	if opts.InferenceSessionID == "" {
		opts.InferenceSessionID = binding.GetInferenceOperatorSessionId()
	}
	if opts.DataSessionID == "" {
		opts.DataSessionID = binding.GetDataOperatorSessionId()
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
		return 0, err
	}
	if !opts.NoAutoRefresh {
		authContext, err = chatEvalEnsureOperatorBinding(cmd, chatEvalDeps{
			configLoader:         deps.configLoader,
			fileSvcFactory:       deps.fileSvcFactory,
			authLoader:           deps.authLoader,
			clientFactory:        deps.clientFactory,
			refreshClientFactory: defaultRefreshClientFactory,
			now:                  deps.now,
			newID:                deps.newID,
		}, cfg, fileSvc, authContext, operators, opts.DataSessionID)
		if err != nil {
			return 0, fmt.Errorf("evaluation: campaign execute: %w", err)
		}
	}
	selected, err := evaluation.SelectInferenceOperator(operators, opts.InferenceSessionID)
	if err != nil {
		return 0, err
	}
	dataOperator, err := chatEvalResolveDataOperator(operators, authContext, opts.DataSessionID)
	if err != nil {
		return 0, fmt.Errorf("evaluation: campaign execute: %w", err)
	}
	spec, err := store.LoadCampaignSpec(cmd.Context(), binding.GetCampaignId())
	if err != nil {
		return 0, fmt.Errorf("evaluation: campaign execute: %w", err)
	}
	_, artifacts, err := evaluation.LoadNorthStarScenarioCatalog()
	if err != nil {
		return 0, fmt.Errorf("evaluation: campaign execute: %w", err)
	}
	ensembleClient, err := chatEvalEnsembleClient(cfg, authContext, resolveChatEvalEnsembleURL(opts.EnsembleURL), chatEvalDeps{
		configLoader:   deps.configLoader,
		fileSvcFactory: deps.fileSvcFactory,
		authLoader:     deps.authLoader,
		clientFactory:  deps.clientFactory,
		now:            deps.now,
		newID:          deps.newID,
	})
	if err != nil {
		return 0, err
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
			return chatEvalWaitForTrace(ctx, fetch, newChatAcceptReporter(cmd.OutOrStdout(), opts.JSONOutput))
		},
		deps.now,
		func(prefix string) string { return prefix + "-" + deps.newID() },
	)
	controller := controllerFactory(executor)
	var publication *evaluation.CampaignPublicationCoordinator
	if opts.Publish {
		publication, err = newCampaignPublicationCoordinator(cmd, fileSvc)
		if err != nil {
			return 0, fmt.Errorf("evaluation: campaign execute: %w", err)
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
	executed := 0
	resolvedOllamaEndpoint, err := resolveCampaignOllamaEndpoint(opts.OllamaEndpoint, operators, selected.OperatorSessionID)
	if err != nil {
		return executed, err
	}
	iterations := int(opts.Limit)
	if opts.Daemon {
		iterations = 1<<31 - 1
	}
	for i := 0; i < iterations; i++ {
		restartCtx, restartCancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
		err := restartOllamaViaObserverIfEnabled(restartCtx, operators, opts.RunID, dataOperator, cfg, authContext, chatEvalDeps{
			configLoader:   deps.configLoader,
			fileSvcFactory: deps.fileSvcFactory,
			authLoader:     deps.authLoader,
			clientFactory:  deps.clientFactory,
			now:            deps.now,
			newID:          deps.newID,
		}, func(prefix string) string { return prefix + "-" + deps.newID() })
		restartCancel()
		if err != nil {
			return executed, fmt.Errorf("evaluation: campaign execute: %w", err)
		}
		if opts.WaitForProviderIdle {
			idleCtx, idleCancel := context.WithTimeout(cmd.Context(), 15*time.Minute)
			err := inference.WaitForProviderIdle(idleCtx, inference.ProviderIdleOptions{
				Endpoint:       resolvedOllamaEndpoint,
				PollInterval:   opts.ProviderIdlePoll,
				SettleDuration: opts.ProviderSettle,
			})
			idleCancel()
			if err != nil {
				return executed, fmt.Errorf("evaluation: campaign execute: %w", err)
			}
			if !opts.JSONOutput {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Provider idle at %s\n", resolvedOllamaEndpoint)
			}
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 8*time.Minute)
		result, ok, err := controller.ExecuteNextAssignment(ctx, opts.RunID, executionBinding, artifacts)
		cancel()
		if err != nil {
			return executed, fmt.Errorf("evaluation: campaign execute: %w", err)
		}
		if !ok {
			if publication != nil {
				completionCount, publishErr := publication.PublishRunCompletion(cmd.Context(), opts.RunID, deps.now().UTC())
				if publishErr != nil {
					return executed, fmt.Errorf("evaluation: campaign execute: %w", publishErr)
				}
				if completionCount > 0 && !opts.JSONOutput {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Published %d completion projection record(s) for run %s\n", completionCount, opts.RunID)
				}
			}
			break
		}
		executed++
		if !opts.JSONOutput {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Executed %s: %s\n", result.GetAssignmentId(), result.GetLifecycleStatus().String())
		}
	}
	return executed, nil
}

func verifyCampaignRun(
	cmd *cobra.Command,
	deps nativeEvalDeps,
	runID string,
	requireProviderObservation bool,
	requireModelProvenance bool,
	jsonOutput bool,
) (*evalv1.EvaluationVerificationReport, error) {
	cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
	if err != nil {
		return nil, err
	}
	store := evaluation.NewStore(fileSvc)
	run, err := store.LoadRun(cmd.Context(), runID)
	if err != nil {
		return nil, fmt.Errorf("evaluation: campaign verify: %w", err)
	}
	catalog, err := store.LoadScenarioCatalog(cmd.Context(), run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		return nil, fmt.Errorf("evaluation: campaign verify: %w", err)
	}
	_, artifacts, err := evaluation.LoadNorthStarScenarioCatalog()
	if err != nil {
		return nil, fmt.Errorf("evaluation: campaign verify: %w", err)
	}
	verifier := evaluation.NewCampaignRunVerifier(deps.now)
	observationReader, err := newCampaignProviderObservationReader(fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("evaluation: campaign verify: %w", err)
	}
	policy := evaluation.ProviderObservationPolicyInterim
	if requireProviderObservation {
		policy = evaluation.ProviderObservationPolicyStrict
	}
	verifier = verifier.WithProviderObservationReader(observationReader, policy)
	provenanceReader, err := newCampaignModelProvenanceReader(fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("evaluation: campaign verify: %w", err)
	}
	provenancePolicy := evaluation.ModelProvenancePolicyInterim
	if requireModelProvenance {
		provenancePolicy = evaluation.ModelProvenancePolicyStrict
	}
	verifier = verifier.WithModelProvenanceReader(provenanceReader, provenancePolicy)
	report, err := verifier.VerifyRun(cmd.Context(), store, runID, catalog, artifacts)
	if err != nil {
		return nil, fmt.Errorf("evaluation: campaign verify: %w", err)
	}
	if err := store.SaveCampaignVerification(cmd.Context(), runID, report); err != nil {
		return nil, fmt.Errorf("evaluation: campaign verify: %w", err)
	}
	publication, pubErr := newCampaignPublicationCoordinator(cmd, fileSvc)
	if pubErr != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: campaign verify publication unavailable: %v\n", pubErr)
	} else if report.GetStatus() == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS {
		published, pubErr := publication.PublishRunVerification(cmd.Context(), runID, report)
		if pubErr != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: campaign verify publication: %v\n", pubErr)
		} else if published > 0 && !jsonOutput {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Published %d verification projection record(s) to public mirror\n", published)
		}
	}
	return report, nil
}

func writeCampaignStartPlan(out io.Writer, plan *evaluation.CampaignStartPlan, sessions campaignOperatorSessions, jsonOutput bool) error {
	if jsonOutput {
		payload, err := json.MarshalIndent(map[string]any{
			"campaign_id":            plan.CampaignID,
			"run_id":                 plan.RunID,
			"inventory_file":         plan.InventoryPath,
			"model_tags":             plan.ModelTags,
			"model_registry_digest":  plan.RegistryDigest,
			"homogeneous_cell_count": plan.HomogeneousCellCount,
			"inference_session":      sessions.InferenceSessionID,
			"data_session":           sessions.DataSessionID,
		}, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(payload))
		return err
	}
	_, err := fmt.Fprintf(out, "Campaign start plan\nCampaign: %s\nRun: %s\nModels: %s\nCells: %d\nInventory: %s\nRegistry digest: %s\nInference session: %s\nData session: %s\n",
		plan.CampaignID,
		plan.RunID,
		fmt.Sprint(plan.ModelTags),
		plan.HomogeneousCellCount,
		plan.InventoryPath,
		plan.RegistryDigest,
		sessions.InferenceSessionID,
		sessions.DataSessionID,
	)
	return err
}

func persistActiveCampaignRun(projectRoot string, plan *evaluation.CampaignStartPlan, startedAt time.Time) error {
	return evaluation.SaveActiveCampaignRun(projectRoot, evaluation.ActiveCampaignRun{
		RunID:         plan.RunID,
		CampaignID:    plan.CampaignID,
		InventoryFile: plan.InventoryPath,
		ModelTags:     plan.ModelTags,
		StartedAt:     startedAt,
	})
}

func resolveCampaignRunID(cmd *cobra.Command, command, flagValue string, args []string) (string, error) {
	if len(args) > 1 {
		return "", fmt.Errorf("evaluation: campaign %s: accepts at most one run ID argument", command)
	}
	if len(args) == 1 {
		if flagValue != "" && flagValue != args[0] {
			return "", fmt.Errorf("evaluation: campaign %s: conflicting run ID %q and positional argument %q", command, flagValue, args[0])
		}
		return args[0], nil
	}
	if flagValue != "" {
		return flagValue, nil
	}
	projectRoot, err := cmd.Flags().GetString("project-root")
	if err != nil {
		return "", fmt.Errorf("evaluation: campaign %s: read project root: %w", command, err)
	}
	cfg, err := loadConfig(projectRoot)
	if err == nil {
		if active, activeErr := evaluation.LoadActiveCampaignRun(cfg.ProjectRoot); activeErr == nil && active.RunID != "" {
			return active.RunID, nil
		}
	}
	return "", fmt.Errorf("evaluation: campaign %s: %w (pass --run-id or run `g8e eval campaign start` first)", command, constants.ErrMissingRequiredField)
}
