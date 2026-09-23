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
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
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
	RunID                    string
	Publish                  bool
	Daemon                   bool
	Limit                    uint32
	InferenceSessionID       string
	DataSessionID            string
	EnsembleURL              string
	OllamaEndpoint           string
	EnforceProviderResidency bool
	NoAutoRefresh            bool
	JSONOutput               bool
	ResultOutput             func(*evalv1.EvaluationAssignmentResult)
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
	inventory, err := loadEvaluationInventoryFreeze(cmd.Context(), fileSvc, cfg.ProjectRoot, plan.InventoryPath)
	if err != nil {
		return fmt.Errorf("evaluation: campaign init: load inventory: %w", err)
	}
	if inventory.CampaignID != "" && inventory.CampaignID != plan.CampaignID {
		return fmt.Errorf("evaluation: campaign init: inventory campaign_id mismatch")
	}
	inventory.CampaignID = plan.CampaignID
	catalog, artifacts, err := evaluation.LoadScenarioCatalog()
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

func runCampaignExecute(cmd *cobra.Command, deps nativeEvalDeps, opts campaignExecuteOptions) (executed int, runErr error) {
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
	_, artifacts, err := evaluation.LoadScenarioCatalog()
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
		func(ctx context.Context, fetch func(context.Context) (evaluation.EvaluationTrace, error)) (evaluation.EvaluationTrace, error) {
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
	if err := preflightProviderObservationDelivery(fileSvc, cfg); err != nil {
		return executed, fmt.Errorf("evaluation: campaign execute: %w", err)
	}
	if opts.EnforceProviderResidency {
		defer func() {
			if runErr == nil {
				return
			}
			cleanupErr := releaseCampaignModels(cmd, deps, opts, cfg, authContext, dataOperator, operators, selected.OperatorSessionID, spec)
			if cleanupErr == nil {
				return
			}
			cleanupErr = fmt.Errorf("evaluation: campaign execute: release provider models: %w", cleanupErr)
			runErr = errors.Join(runErr, cleanupErr)
		}()
		endpoint, err := resolveCampaignOllamaEndpoint(opts.OllamaEndpoint, operators, selected.OperatorSessionID)
		if err != nil {
			return executed, fmt.Errorf("evaluation: campaign execute: %w", err)
		}
		if err := rejectResidentProviderModels(cmd.Context(), endpoint); err != nil {
			return executed, fmt.Errorf("evaluation: campaign execute: %w", err)
		}
	}
	modelBindings, err := evaluation.CampaignModelBindingsFromSpec(spec)
	if err != nil {
		return executed, fmt.Errorf("evaluation: campaign execute: %w", err)
	}
	if err := preflightCampaignModelProvenance(fileSvc, cfg, modelBindings); err != nil {
		return executed, fmt.Errorf("evaluation: campaign execute: %w", err)
	}
	iterations := int(opts.Limit)
	if opts.Daemon {
		iterations = 1<<31 - 1
	}
	for i := 0; i < iterations; i++ {
		ctx, cancel := context.WithTimeout(cmd.Context(), 8*time.Minute)
		result, ok, err := controller.ExecuteNextAssignment(ctx, opts.RunID, executionBinding, artifacts)
		cancel()
		if err != nil {
			return executed, fmt.Errorf("evaluation: campaign execute: %w", err)
		}
		if !ok {
			break
		}
		executed++
		if opts.ResultOutput != nil {
			opts.ResultOutput(result)
		}
		if !opts.JSONOutput {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Executed %s: %s\n", result.GetAssignmentId(), result.GetLifecycleStatus().String())
		}
	}

	summary, err = controller.RunSummary(cmd.Context(), opts.RunID)
	if err != nil {
		return executed, fmt.Errorf("evaluation: campaign execute: %w", err)
	}
	if summary.QueuedCount == 0 && summary.RunningCount == 0 {
		if err := releaseCampaignModels(cmd, deps, opts, cfg, authContext, dataOperator, operators, selected.OperatorSessionID, spec); err != nil {
			return executed, fmt.Errorf("evaluation: campaign execute: %w", err)
		}
		if publication != nil {
			completionCount, publishErr := publication.PublishRunCompletion(cmd.Context(), opts.RunID, deps.now().UTC())
			if publishErr != nil {
				return executed, fmt.Errorf("evaluation: campaign execute: %w", publishErr)
			}
			if completionCount > 0 && !opts.JSONOutput {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Published %d completion projection record(s) for run %s\n", completionCount, opts.RunID)
			}
		}
	}
	return executed, nil
}

func rejectResidentProviderModels(ctx context.Context, endpoint string) error {
	residency, err := inference.ReadProviderResidency(ctx, inference.ProviderResidencyOptions{Endpoint: endpoint})
	if err != nil {
		return fmt.Errorf("evaluation: read provider residency: %w", err)
	}
	if len(residency.Models) == 0 {
		return nil
	}
	residentTags := make([]string, 0, len(residency.Models))
	for _, model := range residency.Models {
		residentTags = append(residentTags, model.Name)
	}
	return fmt.Errorf("%w: %s", constants.ErrEvaluationProviderModelsResident, strings.Join(residentTags, ", "))
}

func releaseCampaignModels(
	cmd *cobra.Command,
	deps nativeEvalDeps,
	opts campaignExecuteOptions,
	cfg *config.Config,
	authContext *auth.ClientAuthContext,
	dataOperator *evaluation.DataOperatorStatus,
	operators []models.OperatorDocumentGo,
	inferenceSessionID string,
	spec *evalv1.EvaluationCampaignSpec,
) error {
	if inferenceSessionID == "" || spec == nil || len(spec.GetModelRegistry()) == 0 {
		return nil
	}
	endpoint, err := resolveCampaignOllamaEndpoint(opts.OllamaEndpoint, operators, inferenceSessionID)
	if err != nil {
		return err
	}
	modelTags := make([]string, 0, len(spec.GetModelRegistry()))
	for _, variant := range spec.GetModelRegistry() {
		if variant != nil && variant.GetServedModelTag() != "" {
			modelTags = append(modelTags, variant.GetServedModelTag())
		}
	}
	residency, err := inference.ReadProviderResidency(cmd.Context(), inference.ProviderResidencyOptions{Endpoint: endpoint})
	if err != nil {
		return err
	}
	residentTags := make([]string, 0, len(modelTags))
	for _, tag := range modelTags {
		for _, resident := range residency.Models {
			if resident.Name == tag {
				residentTags = append(residentTags, tag)
				break
			}
		}
	}
	if len(residentTags) == 0 {
		return nil
	}
	dispatcher, err := newHarnessOllamaModelCommandDispatcher(cfg, authContext, dataOperator, chatEvalDeps{
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
	if err := evaluation.ReleaseOllamaModels(cmd.Context(), dispatcher, inferenceSessionID, opts.RunID, residentTags, modelCommandEnvironment(endpoint), func(prefix string) string { return prefix + "-" + deps.newID() }); err != nil {
		return err
	}
	waitCtx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
	defer cancel()
	if err := inference.WaitForProviderModelsAbsent(waitCtx, inference.ProviderResidencyOptions{Endpoint: endpoint}, residentTags); err != nil {
		return err
	}
	if !opts.JSONOutput {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Released %d campaign model(s) at %s\n", len(residentTags), endpoint)
	}
	return nil
}

type campaignVerificationPublication interface {
	PublishRunCompletion(context.Context, string, time.Time) (int, error)
	PublishRunVerification(context.Context, string, *evalv1.EvaluationVerificationReport) (int, error)
}

type campaignVerificationPublicationFactory func(*cobra.Command, fs.RuntimeFileService) (campaignVerificationPublication, error)

func verifyCampaignRun(
	cmd *cobra.Command,
	deps nativeEvalDeps,
	runID string,
	requireProviderObservation bool,
	requireModelProvenance bool,
	jsonOutput bool,
) (*evalv1.EvaluationVerificationReport, error) {
	return verifyCampaignRunWithPublication(cmd, deps, runID, requireProviderObservation, requireModelProvenance, true, jsonOutput)
}

func verifyCampaignRunWithPublication(
	cmd *cobra.Command,
	deps nativeEvalDeps,
	runID string,
	requireProviderObservation bool,
	requireModelProvenance bool,
	publish bool,
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
	_, artifacts, err := evaluation.LoadScenarioCatalog()
	if err != nil {
		return nil, fmt.Errorf("evaluation: campaign verify: %w", err)
	}
	verifier := evaluation.NewCampaignRunVerifier(deps.now)
	observationReader, err := newCampaignProviderObservationReader(fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("evaluation: campaign verify: %w", err)
	}
	providerPolicy := evaluation.ProviderObservationPolicyInterim
	if requireProviderObservation {
		providerPolicy = evaluation.ProviderObservationPolicyStrict
	}
	verifier = verifier.WithProviderObservationReader(observationReader, providerPolicy)
	provenanceReader, err := newCampaignModelProvenanceReader(fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("evaluation: campaign verify: %w", err)
	}
	provenancePolicy := evaluation.ModelProvenancePolicyInterim
	if requireModelProvenance {
		provenancePolicy = evaluation.ModelProvenancePolicyStrict
	}
	verifier = verifier.WithModelProvenanceReader(provenanceReader, provenancePolicy)
	if err := evaluation.CaptureCampaignRunWitnessEvidence(cmd.Context(), store, runID, observationReader, provenanceReader); err != nil {
		return nil, fmt.Errorf("evaluation: campaign verify: %w", err)
	}
	report, err := verifier.VerifyRun(cmd.Context(), store, runID, catalog, artifacts)
	if err != nil {
		return nil, fmt.Errorf("evaluation: campaign verify: %w", err)
	}
	report, _, err = evaluation.BindStoredCampaignVerificationReport(cmd.Context(), store, report, evaluation.CampaignVerificationPolicy{
		VerifierReleaseVersion: constants.EvaluationSourceVersion,
		ProviderObservation:    providerPolicy,
		ModelProvenance:        provenancePolicy,
	})
	if err != nil {
		return nil, fmt.Errorf("evaluation: campaign verify: %w", err)
	}
	if err := store.SaveCampaignVerification(cmd.Context(), runID, report); err != nil {
		return nil, fmt.Errorf("evaluation: campaign verify: %w", err)
	}
	if !publish {
		return report, nil
	}
	publicationFactory := deps.campaignPublicationFactory
	if publicationFactory == nil {
		publicationFactory = func(cmd *cobra.Command, fileSvc fs.RuntimeFileService) (campaignVerificationPublication, error) {
			return newCampaignPublicationCoordinator(cmd, fileSvc)
		}
	}
	publication, pubErr := publicationFactory(cmd, fileSvc)
	if pubErr != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: campaign verify publication unavailable: %v\n", pubErr)
	} else {
		completionCount, pubErr := publication.PublishRunCompletion(cmd.Context(), runID, deps.now().UTC())
		if pubErr != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: campaign verify completion publication: %v\n", pubErr)
		} else if completionCount > 0 && !jsonOutput {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Published %d completion projection record(s) to public mirror\n", completionCount)
		}
		if report.GetStatus() == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS {
			published, pubErr := publication.PublishRunVerification(cmd.Context(), runID, report)
			if pubErr != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: campaign verify publication: %v\n", pubErr)
			} else if published > 0 && !jsonOutput {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Published %d verification projection record(s) to public mirror\n", published)
			}
		}
	}
	return report, nil
}

type campaignStartFlowOptions struct {
	ModelTag                   string
	ModelTags                  []string
	QueueRef                   string
	CampaignID                 string
	InventoryFile              string
	RunID                      string
	InferenceSessionID         string
	DataSessionID              string
	EnsembleURL                string
	OllamaEndpoint             string
	DryRun                     bool
	PrintPlan                  bool
	PrepareOnly                bool
	Publish                    bool
	Daemon                     bool
	Verify                     bool
	RequireProviderObservation bool
	RequireModelProvenance     bool
	NoAutoRefresh              bool
	JSONOutput                 bool
}

type campaignStartFlowResult struct {
	Plan     *evaluation.CampaignStartPlan
	Executed int
	Report   *evalv1.EvaluationVerificationReport
}

func runCampaignStartFlow(cmd *cobra.Command, deps nativeEvalDeps, opts campaignStartFlowOptions) (*campaignStartFlowResult, error) {
	cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
	if err != nil {
		return nil, err
	}
	plan, err := evaluation.ResolveCampaignStartPlan(evaluation.CampaignStartPlanRequest{
		Context:       cmd.Context(),
		FileService:   fileSvc,
		ModelTag:      opts.ModelTag,
		ModelTags:     opts.ModelTags,
		QueueRef:      opts.QueueRef,
		CampaignID:    opts.CampaignID,
		InventoryFile: opts.InventoryFile,
		RunID:         opts.RunID,
		Now:           deps.now().UTC(),
	})
	if err != nil {
		return nil, err
	}
	sessions, err := resolveCampaignOperatorSessions(cmd, deps, cfg, opts.InferenceSessionID, opts.DataSessionID)
	if err != nil {
		return nil, fmt.Errorf("evaluation: campaign start: %w", err)
	}
	if opts.PrintPlan || opts.DryRun {
		if err := writeCampaignStartPlan(cmd.OutOrStdout(), plan, sessions, opts.JSONOutput); err != nil {
			return nil, err
		}
	}
	if opts.DryRun {
		return &campaignStartFlowResult{Plan: plan}, nil
	}
	startedAt := deps.now().UTC()
	if err := initializeCampaignRun(cmd, deps, plan, sessions); err != nil {
		return nil, err
	}
	if !opts.JSONOutput {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Initialized campaign %s run %s\n", plan.CampaignID, plan.RunID)
	}
	assignmentCount, err := scheduleHomogeneousCampaignRun(cmd, deps, plan.RunID, opts.Publish)
	if err != nil {
		return nil, err
	}
	if !opts.JSONOutput {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Scheduled %d assignments for run %s\n", assignmentCount, plan.RunID)
	}
	if err := persistActiveCampaignRunWithFileService(cmd.Context(), fileSvc, plan, startedAt); err != nil {
		return nil, err
	}
	result := &campaignStartFlowResult{Plan: plan}
	if opts.PrepareOnly {
		return result, nil
	}
	executed, err := runCampaignExecute(cmd, deps, campaignExecuteOptions{
		RunID:                    plan.RunID,
		Publish:                  opts.Publish,
		Daemon:                   opts.Daemon,
		InferenceSessionID:       sessions.InferenceSessionID,
		DataSessionID:            sessions.DataSessionID,
		EnsembleURL:              opts.EnsembleURL,
		OllamaEndpoint:           opts.OllamaEndpoint,
		EnforceProviderResidency: opts.RequireProviderObservation || opts.RequireModelProvenance,
		NoAutoRefresh:            opts.NoAutoRefresh,
		JSONOutput:               opts.JSONOutput,
	})
	if err != nil {
		return result, err
	}
	result.Executed = executed
	if !opts.JSONOutput {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Executed %d assignment(s) for run %s\n", executed, plan.RunID)
	}
	if !opts.Verify {
		return result, nil
	}
	report, err := verifyCampaignRunWithPublication(cmd, deps, plan.RunID, opts.RequireProviderObservation, opts.RequireModelProvenance, opts.Publish, opts.JSONOutput)
	if err != nil {
		return result, err
	}
	result.Report = report
	if !opts.JSONOutput {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Run %s verification: %s (%d failure(s))\n", plan.RunID, report.GetStatus().String(), report.GetFailureCount())
		if report.GetFailureCount() > 0 {
			for _, reason := range report.GetFailureReasons() {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", reason)
			}
		}
	}
	if report.GetFailureCount() > 0 {
		return result, constants.ErrEvalRunVerificationFailed
	}
	if err := markQueueEntryVerifiedAfterPassWithFileService(cmd.Context(), fileSvc, plan, report, opts.RequireProviderObservation && opts.RequireModelProvenance); err != nil {
		return result, err
	}
	return result, nil
}

func markQueueEntryVerifiedAfterPassWithFileService(ctx context.Context, fileSvc fs.RuntimeFileService, plan *evaluation.CampaignStartPlan, report *evalv1.EvaluationVerificationReport, tierA bool) error {
	if plan == nil || plan.QueueEntry == nil || report == nil {
		return nil
	}
	if report.GetStatus() != evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS {
		return nil
	}
	notes := "75/75 verify PASS; run " + plan.RunID
	if tierA {
		notes = evaluation.TierAVerifyNotes(plan.RunID)
	}
	_, err := evaluation.MarkCampaignQueueEntry(evaluation.MarkCampaignQueueEntryRequest{
		Context:       ctx,
		FileService:   fileSvc,
		VariantID:     plan.QueueEntry.VariantID,
		Status:        "verified",
		VerifiedRunID: plan.RunID,
		Notes:         notes,
	})
	if err != nil {
		return fmt.Errorf("evaluation: update init campaign queue: %w", err)
	}
	return nil
}

type campaignStartPlanJSON struct {
	CampaignID           string   `json:"campaign_id"`
	RunID                string   `json:"run_id"`
	InventoryFile        string   `json:"inventory_file"`
	ModelTags            []string `json:"model_tags"`
	ModelRegistryDigest  string   `json:"model_registry_digest"`
	HomogeneousCellCount uint64   `json:"homogeneous_cell_count"`
	InferenceSession     string   `json:"inference_session"`
	DataSession          string   `json:"data_session"`
}

func writeCampaignStartPlan(out io.Writer, plan *evaluation.CampaignStartPlan, sessions campaignOperatorSessions, jsonOutput bool) error {
	if jsonOutput {
		payload, err := json.MarshalIndent(campaignStartPlanJSON{
			CampaignID:           plan.CampaignID,
			RunID:                plan.RunID,
			InventoryFile:        plan.InventoryPath,
			ModelTags:            plan.ModelTags,
			ModelRegistryDigest:  plan.RegistryDigest,
			HomogeneousCellCount: plan.HomogeneousCellCount,
			InferenceSession:     sessions.InferenceSessionID,
			DataSession:          sessions.DataSessionID,
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

func persistActiveCampaignRunWithFileService(ctx context.Context, fileSvc fs.RuntimeFileService, plan *evaluation.CampaignStartPlan, startedAt time.Time) error {
	return evaluation.SaveActiveCampaignRunToRuntime(ctx, fileSvc, evaluation.ActiveCampaignRun{
		RunID:         plan.RunID,
		CampaignID:    plan.CampaignID,
		InventoryFile: plan.InventoryPath,
		ModelTags:     plan.ModelTags,
		StartedAt:     startedAt,
	})
}

func resolveCampaignRunID(cmd *cobra.Command, deps nativeEvalDeps, command, flagValue string, args []string) (string, error) {
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
	_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
	if err == nil {
		if active, activeErr := evaluation.LoadActiveCampaignRunFromRuntime(cmd.Context(), fileSvc); activeErr == nil && active.RunID != "" {
			return active.RunID, nil
		}
	}
	return "", fmt.Errorf("evaluation: campaign %s: %w (pass --run-id or run `g8e eval campaign start` first)", command, constants.ErrMissingRequiredField)
}
