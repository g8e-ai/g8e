// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/gwremote"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const assignmentTimeout = 8 * time.Minute

type runExecuteOptions struct {
	RunID                    string
	Publish                  bool
	Daemon                   bool
	Limit                    uint32
	EnsembleURL              string
	EnforceProviderResidency bool
	NoAutoRefresh            bool
	JSONOutput               bool
	ResultOutput             func(*evalv1.EvaluationAssignmentResult)
}

// executeRun executes queued assignments of one run while holding the run's
// lease. The lease is the durable handle that lets status, cancel, logs, and
// archive tell a live run from an interrupted one. Output is teed into the
// run's execution log.
func executeRun(cmd *cobra.Command, deps nativeEvalDeps, opts runExecuteOptions) (executed int, runErr error) {
	_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
	if err != nil {
		return 0, err
	}
	if err := evaluation.RejectArchivedRun(cmd.Context(), fileSvc, opts.RunID); err != nil {
		return 0, err
	}
	store := evaluation.NewStore(fileSvc)
	// The lease is written under the run's directory, so a run that does not
	// exist must fail before one is acquired.
	if _, err := store.LoadRun(cmd.Context(), opts.RunID); err != nil {
		return 0, fmt.Errorf("evaluation: run execute: %w", err)
	}
	control, err := deps.runControl.process(fileSvc)
	if err != nil {
		return 0, err
	}
	host, err := control.Hostname()
	if err != nil {
		return 0, fmt.Errorf("evaluation: run lease: read hostname: %w", err)
	}
	startedAt := deps.now().UTC()
	log, err := openRunLog(cmd, fileSvc, opts.RunID, startedAt)
	if err != nil {
		return 0, err
	}
	defer func() {
		if closeErr := log.close(); closeErr != nil {
			runErr = errors.Join(runErr, fmt.Errorf("evaluation: close run log: %w", closeErr))
		}
	}()

	stale, err := store.AcquireRunLease(cmd.Context(), evaluation.RunLease{
		RunID:     opts.RunID,
		PID:       control.PID(),
		Host:      host,
		StartedAt: startedAt,
		LogPath:   log.path,
	}, leaseLiveness(control))
	if err != nil {
		return 0, err
	}
	defer func() {
		if releaseErr := store.ReleaseRunLease(context.WithoutCancel(cmd.Context()), opts.RunID, control.PID()); releaseErr != nil {
			runErr = errors.Join(runErr, fmt.Errorf("evaluation: release run lease: %w", releaseErr))
		}
	}()
	if stale != nil && !opts.JSONOutput {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Cleared stale lease held by process %d on %s\n", stale.PID, stale.Host)
	}

	signals, stopSignals := deps.runControl.interrupts()
	defer stopSignals()
	parent := cmd.Context()
	hardCtx, hardCancel := context.WithCancel(parent)
	defer hardCancel()
	cmd.SetContext(hardCtx)
	defer cmd.SetContext(parent)
	watcher := startRunStopWatcher(hardCtx, store, opts.RunID, deps.runControl.interval(), signals, hardCancel)
	defer watcher.Stop()

	return executeAssignments(cmd, deps, opts, watcher)
}

// defaultSessionPin prefers an explicit pin, then the session recorded on the
// run when it is still active, and otherwise leaves the choice to the resolver.
func defaultSessionPin(explicit, recorded string, candidates []operatorCandidate) string {
	if explicit != "" {
		return explicit
	}
	for _, candidate := range candidates {
		if candidate.SessionID == recorded {
			return recorded
		}
	}
	return ""
}

func executeAssignments(cmd *cobra.Command, deps nativeEvalDeps, opts runExecuteOptions, watcher *runStopWatcher) (executed int, runErr error) {
	if opts.Daemon {
		opts.Limit = ^uint32(0)
	} else if opts.Limit == 0 {
		opts.Limit = 1
	}
	chatDeps := deps.chatDeps()
	cfg, fileSvc, authContext, err := chatEvalEnvironment(cmd, chatDeps)
	if err != nil {
		return 0, err
	}
	store := evaluation.NewStore(fileSvc)
	controllerFactory := func(executor evaluation.CampaignAssignmentExecutor) *evaluation.CampaignController {
		return evaluation.NewCampaignController(store, executor, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
	}
	summary, err := controllerFactory(nil).RunSummary(cmd.Context(), opts.RunID)
	if err != nil {
		return 0, fmt.Errorf("evaluation: run execute: %w", err)
	}
	binding := summary.Run.GetCampaignBinding()
	if binding == nil {
		return 0, fmt.Errorf("evaluation: run execute: missing campaign binding")
	}
	operators, err := chatEvalListOperators(cmd, chatDeps, cfg, authContext)
	if err != nil {
		return 0, err
	}
	inferencePin, dataPin, err := sessionPinsFromFlags(cmd)
	if err != nil {
		return 0, err
	}
	sessions, err := resolveOperatorSessionsFrom(operators,
		defaultSessionPin(inferencePin, binding.GetInferenceOperatorSessionId(), inferenceCandidates(operators)),
		defaultSessionPin(dataPin, binding.GetDataOperatorSessionId(), dataCandidates(operators)),
		operatorRoleInference, operatorRoleData)
	if err != nil {
		return 0, fmt.Errorf("evaluation: run execute: %w", err)
	}
	if !opts.NoAutoRefresh {
		authContext, err = chatEvalEnsureOperatorBinding(cmd, chatDeps, cfg, fileSvc, authContext, operators, sessions.DataSessionID)
		if err != nil {
			return 0, fmt.Errorf("evaluation: run execute: %w", err)
		}
	}
	selected, err := evaluation.SelectInferenceOperator(operators, sessions.InferenceSessionID)
	if err != nil {
		return 0, err
	}
	dataOperator, err := chatEvalResolveDataOperator(operators, authContext, sessions.DataSessionID)
	if err != nil {
		return 0, fmt.Errorf("evaluation: run execute: %w", err)
	}
	spec, err := store.LoadCampaignSpec(cmd.Context(), binding.GetCampaignId())
	if err != nil {
		return 0, fmt.Errorf("evaluation: run execute: %w", err)
	}
	_, artifacts, err := evaluation.LoadScenarioCatalog()
	if err != nil {
		return 0, fmt.Errorf("evaluation: run execute: %w", err)
	}
	ensembleClient, err := chatEvalEnsembleClient(cfg, authContext, resolveChatEvalEnsembleURL(opts.EnsembleURL), chatDeps)
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
	gatewayClient, err := chatDeps.clientFactory(nativeEvalClientConfig(cfg, authContext))
	if err != nil {
		return 0, fmt.Errorf("evaluation: run execute: %w", err)
	}
	fileWriter := evaluation.NewCommandLane(gatewayClient, store, persona, 0, 0)
	chatExecutor := evaluation.NewCampaignChatExecutor(
		&campaignChatHarnessClient{client: ensembleClient},
		persona,
		dataOperator.OperatorID,
		dataOperator.OperatorSessionID,
		store,
		func(ctx context.Context, fetch func(context.Context) (evaluation.EvaluationTrace, error)) (evaluation.EvaluationTrace, error) {
			return chatEvalWaitForTrace(ctx, fetch, newChatAcceptReporter(cmd.OutOrStdout(), opts.JSONOutput))
		},
		fileWriter,
		deps.now,
		func(prefix string) string { return prefix + "-" + deps.newID() },
	)
	formationExecutor := evaluation.NewLazyCampaignFormationExecutor(func() (evaluation.CampaignAssignmentExecutor, error) {
		formationRunner, err := buildCampaignFormationProductionRunner(
			cmd,
			deps,
			cfg,
			fileSvc,
			authContext,
			dataOperator,
			operators,
			spec.GetModelRegistry(),
			spec.GetModelRegistryDigest(),
			campaignFormationProductionOptions{
				InferenceSessionID: selected.OperatorSessionID,
				DataSessionID:      dataOperator.OperatorSessionID,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("evaluation: campaign formation runner: %w", err)
		}
		observationReader, err := gwremote.NewCampaignProviderObservationReader(fileSvc, cfg)
		if err != nil {
			return nil, err
		}
		provenanceReader, err := gwremote.NewCampaignModelProvenanceReader(fileSvc, cfg)
		if err != nil {
			return nil, err
		}
		return evaluation.NewCampaignFormationExecutorWithWitness(
			spec.GetModelRegistry(),
			formationRunner,
			store,
			evaluation.NewCampaignFormationWitnessReader(observationReader, provenanceReader),
			deps.now,
			func(prefix string) string { return prefix + "-" + deps.newID() },
		), nil
	})
	executor := evaluation.NewCampaignAssignmentRouter(chatExecutor, formationExecutor)
	controller := controllerFactory(executor)
	var publication *evaluation.CampaignPublicationCoordinator
	if opts.Publish {
		publication, err = NewCampaignPublicationCoordinator(cmd, fileSvc)
		if err != nil {
			return 0, fmt.Errorf("evaluation: run execute: %w", err)
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
	if err := gwremote.PreflightProviderObservationDelivery(fileSvc, cfg); err != nil {
		return executed, fmt.Errorf("evaluation: run execute: %w", err)
	}
	if opts.EnforceProviderResidency {
		defer func() {
			if runErr == nil {
				return
			}
			cleanupErr := releaseRunModels(cmd, deps, opts, cfg, authContext, dataOperator, operators, selected.OperatorSessionID, spec)
			if cleanupErr == nil {
				return
			}
			runErr = errors.Join(runErr, fmt.Errorf("evaluation: run execute: release provider models: %w", cleanupErr))
		}()
		endpoint, err := resolveCampaignOllamaEndpoint(operators, selected.OperatorSessionID)
		if err != nil {
			return executed, fmt.Errorf("evaluation: run execute: %w", err)
		}
		if err := releaseResidentProviderModels(cmd, deps, opts, cfg, authContext, dataOperator, selected.OperatorSessionID, endpoint); err != nil {
			return executed, fmt.Errorf("evaluation: run execute: %w", err)
		}
	}
	modelBindings, err := evaluation.CampaignModelBindingsFromSpec(spec)
	if err != nil {
		return executed, fmt.Errorf("evaluation: run execute: %w", err)
	}
	if err := gwremote.PreflightCampaignModelProvenance(fileSvc, cfg, modelBindings); err != nil {
		return executed, fmt.Errorf("evaluation: run execute: %w", err)
	}
	iterations := int64(opts.Limit)
	stopped := false
	for i := int64(0); i < iterations; i++ {
		if watcher.Requested() {
			stopped = true
			break
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), assignmentTimeout)
		result, ok, err := controller.ExecuteNextAssignment(ctx, opts.RunID, executionBinding, artifacts)
		cancel()
		if err != nil {
			return executed, fmt.Errorf("evaluation: run execute: %w", err)
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
	if stopped && !opts.JSONOutput {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Stopped after %d assignment(s): cancel requested\n", executed)
	}

	summary, err = controller.RunSummary(cmd.Context(), opts.RunID)
	if err != nil {
		return executed, fmt.Errorf("evaluation: run execute: %w", err)
	}
	if summary.QueuedCount == 0 && summary.RunningCount == 0 {
		if err := releaseRunModels(cmd, deps, opts, cfg, authContext, dataOperator, operators, selected.OperatorSessionID, spec); err != nil {
			return executed, fmt.Errorf("evaluation: run execute: %w", err)
		}
		if publication != nil {
			completionCount, publishErr := publication.PublishRunCompletion(cmd.Context(), opts.RunID, deps.now().UTC())
			if publishErr != nil {
				return executed, fmt.Errorf("evaluation: run execute: %w", publishErr)
			}
			if completionCount > 0 && !opts.JSONOutput {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Published %d completion projection record(s) for run %s\n", completionCount, opts.RunID)
			}
		}
	}
	return executed, nil
}

func releaseResidentProviderModels(
	cmd *cobra.Command,
	deps nativeEvalDeps,
	opts runExecuteOptions,
	cfg *config.Config,
	authContext *auth.ClientAuthContext,
	dataOperator *evaluation.DataOperatorStatus,
	inferenceSessionID string,
	endpoint string,
) error {
	dispatcher, err := newHarnessOllamaModelCommandDispatcher(cfg, authContext, dataOperator, deps.chatDeps())
	if err != nil {
		return err
	}
	maintenance := evaluation.OllamaModelMaintenanceContext{
		TargetOperatorSessionID: inferenceSessionID,
		Environment:             modelCommandEnvironment(endpoint),
		CaseID:                  opts.RunID,
		NewID:                   func(prefix string) string { return prefix + "-" + deps.newID() },
	}
	residency, err := evaluation.ReadOllamaProviderResidency(cmd.Context(), dispatcher, maintenance)
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
	if err := evaluation.ReleaseOllamaModels(cmd.Context(), dispatcher, inferenceSessionID, opts.RunID, residentTags, modelCommandEnvironment(endpoint), maintenance.NewID); err != nil {
		return err
	}
	waitCtx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
	defer cancel()
	if err := evaluation.WaitForOllamaModelsAbsent(waitCtx, dispatcher, maintenance, residentTags); err != nil {
		return err
	}
	if !opts.JSONOutput {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Released %d resident model(s) through inference session %s\n", len(residentTags), inferenceSessionID)
	}
	return nil
}

func releaseRunModels(
	cmd *cobra.Command,
	deps nativeEvalDeps,
	opts runExecuteOptions,
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
	endpoint, err := resolveCampaignOllamaEndpoint(operators, inferenceSessionID)
	if err != nil {
		return err
	}
	modelTags := make([]string, 0, len(spec.GetModelRegistry()))
	for _, variant := range spec.GetModelRegistry() {
		if variant != nil && variant.GetServedModelTag() != "" {
			modelTags = append(modelTags, variant.GetServedModelTag())
		}
	}
	dispatcher, err := newHarnessOllamaModelCommandDispatcher(cfg, authContext, dataOperator, deps.chatDeps())
	if err != nil {
		return err
	}
	maintenance := evaluation.OllamaModelMaintenanceContext{
		TargetOperatorSessionID: inferenceSessionID,
		Environment:             modelCommandEnvironment(endpoint),
		CaseID:                  opts.RunID,
		NewID:                   func(prefix string) string { return prefix + "-" + deps.newID() },
	}
	residency, err := evaluation.ReadOllamaProviderResidency(cmd.Context(), dispatcher, maintenance)
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
	if err := evaluation.ReleaseOllamaModels(cmd.Context(), dispatcher, inferenceSessionID, opts.RunID, residentTags, modelCommandEnvironment(endpoint), maintenance.NewID); err != nil {
		return err
	}
	waitCtx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
	defer cancel()
	if err := evaluation.WaitForOllamaModelsAbsent(waitCtx, dispatcher, maintenance, residentTags); err != nil {
		return err
	}
	if !opts.JSONOutput {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Released %d campaign model(s) through inference session %s\n", len(residentTags), inferenceSessionID)
	}
	return nil
}

func resolveCampaignOllamaEndpoint(operators []models.OperatorDocumentGo, inferenceSessionID string) (string, error) {
	endpoint, err := evaluation.GovernedInferenceOllamaEndpoint(operators, inferenceSessionID)
	if err != nil {
		return "", fmt.Errorf("evaluation: run execute: %w", err)
	}
	return endpoint, nil
}
