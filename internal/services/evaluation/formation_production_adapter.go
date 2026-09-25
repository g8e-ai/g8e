// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

const formationSmokeScenarioID = "formation-smoke"

// FormationProvenancePreflight attests one sovereign model at the storage
// boundary before local allocation.
type FormationProvenancePreflight interface {
	PreflightSovereignModel(ctx context.Context, model FormationModel) (*FormationAttestation, error)
}

// FormationObservationLoader loads provider-boundary evidence keyed by
// provider_attempt_id after governed inference completes.
type FormationObservationLoader interface {
	LoadObservationWindow(ctx context.Context, providerAttemptID string) (*evalv1.ProviderBoundaryObservationWindow, error)
}

// FormationInferenceDispatcher submits governed inference probes through the
// exact Inference Operator session.
type FormationInferenceDispatcher interface {
	DispatchInference(ctx context.Context, req *operatorv1.InferenceDispatchRequest) (*operatorv1.InferenceDispatchResponse, error)
}

// FormationProductionDependencies wires production coordinators into the
// FormationRunner seam. Callers supply exact Operator sessions and frozen
// registry digests; adapters never contact Ollama directly.
type FormationProductionDependencies struct {
	RunContext             FormationRunContext
	Variants               []*evalv1.ModelVariant
	ProvenancePreflight    FormationProvenancePreflight
	ObservationLoader      FormationObservationLoader
	InferenceDispatcher    FormationInferenceDispatcher
	ModelCommandDispatcher OllamaModelCommandDispatcher
	OllamaEnvironment      map[string]string
	NewID                  func(string) string
	Now                    func() time.Time
}

// NewFormationProductionRunner constructs a FormationRunner backed by governed
// production adapters instead of the in-memory harness fakes.
func NewFormationProductionRunner(deps FormationProductionDependencies) (*FormationRunner, error) {
	if deps.InferenceDispatcher == nil || deps.ProvenancePreflight == nil || deps.ObservationLoader == nil {
		return nil, fmt.Errorf("formation: production runner: %w", constants.ErrFormationRunnerDependency)
	}
	if deps.RunContext.InferenceSessionID == "" || deps.RunContext.CampaignID == "" || deps.RunContext.ModelRegistryDigest == "" {
		return nil, fmt.Errorf("formation: production runner: %w", constants.ErrMissingRequiredField)
	}
	if len(deps.Variants) == 0 {
		return nil, fmt.Errorf("formation: production runner: %w", constants.ErrFormationRegistryBinding)
	}
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	newID := deps.NewID
	if newID == nil {
		newID = func(prefix string) string { return fmt.Sprintf("%s-%d", prefix, now().UTC().UnixNano()) }
	}
	registry := InferenceVariantsFromEvalRegistry(deps.Variants)
	return NewFormationRunner(
		&formationProductionProvenance{preflight: deps.ProvenancePreflight},
		&formationProductionObserver{loader: deps.ObservationLoader},
		&formationProductionAllocator{
			runContext:      deps.RunContext,
			registryDigest:  deps.RunContext.ModelRegistryDigest,
			registry:        registry,
			dispatcher:      deps.InferenceDispatcher,
			modelDispatcher: deps.ModelCommandDispatcher,
			environment:     deps.OllamaEnvironment,
			newID:           newID,
		},
		&formationProductionExecutor{
			runContext:     deps.RunContext,
			registryDigest: deps.RunContext.ModelRegistryDigest,
			registry:       registry,
			dispatcher:     deps.InferenceDispatcher,
		},
		formationProductionPolicyGate{},
		now,
		newID,
	)
}

// NewCampaignFormationObservationLoader constructs a reader-backed observation
// loader with bounded retries for post-dispatch evidence persistence.
func NewCampaignFormationObservationLoader(fileSvc fs.RuntimeFileService) (FormationObservationLoader, error) {
	reader, err := NewCampaignProviderObservationReader(fileSvc)
	if err != nil {
		return nil, err
	}
	return NewRetryingFormationObservationLoader(reader.LoadObservationWindow, 12, 250*time.Millisecond), nil
}

type retryingFormationObservationLoader struct {
	load     func(context.Context, string) (*evalv1.ProviderBoundaryObservationWindow, error)
	attempts int
	delay    time.Duration
}

// NewRetryingFormationObservationLoader polls for provider-boundary windows when
// gateway evidence persistence trails governed inference completion.
func NewRetryingFormationObservationLoader(
	load func(context.Context, string) (*evalv1.ProviderBoundaryObservationWindow, error),
	attempts int,
	delay time.Duration,
) FormationObservationLoader {
	if attempts <= 0 {
		attempts = 1
	}
	return &retryingFormationObservationLoader{load: load, attempts: attempts, delay: delay}
}

func (l *retryingFormationObservationLoader) LoadObservationWindow(ctx context.Context, providerAttemptID string) (*evalv1.ProviderBoundaryObservationWindow, error) {
	if l == nil || l.load == nil {
		return nil, fmt.Errorf("formation: observation loader: %w", constants.ErrFormationRunnerDependency)
	}
	var lastErr error
	for attempt := 0; attempt < l.attempts; attempt++ {
		window, err := l.load(ctx, providerAttemptID)
		if err == nil && window != nil {
			return window, nil
		}
		lastErr = err
		if attempt+1 == l.attempts {
			break
		}
		if l.delay > 0 {
			timer := time.NewTimer(l.delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
	if lastErr == nil {
		lastErr = constants.ErrFormationWitnessUnavailable
	}
	if errors.Is(lastErr, constants.ErrNotFound) {
		return nil, fmt.Errorf("formation: observation window %q: %w", providerAttemptID, constants.ErrFormationWitnessUnavailable)
	}
	return nil, fmt.Errorf("formation: observation window %q: %w", providerAttemptID, lastErr)
}

// RunFormationProduction binds one catalog formation to frozen digests and
// executes it through governed production adapters.
func RunFormationProduction(ctx context.Context, binding FormationBindingRequest, deps FormationProductionDependencies, initialState []byte) (*FormationRunResult, error) {
	formation, err := BindFormation(binding)
	if err != nil {
		return nil, err
	}
	return runBoundFormationProduction(ctx, formation, deps, initialState)
}

// RunHeterogeneousFormationProduction binds one campaign heterogeneous stack to
// frozen digests and executes it through governed production adapters.
func RunHeterogeneousFormationProduction(ctx context.Context, binding FormationBindingRequest, deps FormationProductionDependencies, initialState []byte) (*FormationRunResult, error) {
	formation, err := ResolveFormationBinding(binding)
	if err != nil {
		return nil, err
	}
	return runBoundFormationProduction(ctx, formation, deps, initialState)
}

func runBoundFormationProduction(ctx context.Context, formation Formation, deps FormationProductionDependencies, initialState []byte) (*FormationRunResult, error) {
	runner, err := NewFormationProductionRunner(deps)
	if err != nil {
		return nil, err
	}
	return runner.Run(ctx, formation, initialState)
}

type formationProductionProvenance struct {
	preflight FormationProvenancePreflight
}

func (p *formationProductionProvenance) Attest(ctx context.Context, model FormationModel) (*FormationAttestation, error) {
	if model.Trust == FormationTrustDelegated {
		return &FormationAttestation{Verified: true}, nil
	}
	if p == nil || p.preflight == nil {
		return nil, fmt.Errorf("formation: provenance preflight: %w", constants.ErrFormationRunnerDependency)
	}
	return p.preflight.PreflightSovereignModel(ctx, model)
}

type formationProductionObserver struct {
	loader FormationObservationLoader
}

func (o *formationProductionObserver) Begin(_ context.Context, _ string, _ FormationModel) error {
	return nil
}

func (o *formationProductionObserver) Finalize(ctx context.Context, attemptID string, _ FormationModel, _ bool) (*FormationObserverEvidence, error) {
	if o == nil || o.loader == nil {
		return nil, fmt.Errorf("formation: observer finalize: %w", constants.ErrFormationRunnerDependency)
	}
	window, err := o.loader.LoadObservationWindow(ctx, attemptID)
	if err != nil {
		return nil, fmt.Errorf("formation: observer finalize: %w", err)
	}
	if window == nil {
		return nil, fmt.Errorf("formation: observer evidence: %w", constants.ErrFormationWitnessUnavailable)
	}
	return &FormationObserverEvidence{Window: window}, nil
}

type formationProductionAllocator struct {
	runContext      FormationRunContext
	registryDigest  string
	registry        []*operatorv1.InferenceModelVariant
	dispatcher      FormationInferenceDispatcher
	modelDispatcher OllamaModelCommandDispatcher
	environment     map[string]string
	newID           func(string) string
}

func (a *formationProductionAllocator) Allocate(ctx context.Context, model FormationModel) error {
	if model.Trust != FormationTrustSovereign {
		return nil
	}
	_, err := a.dispatchWarmup(ctx, model, a.newID("formation-warmup"))
	return err
}

func (a *formationProductionAllocator) Release(ctx context.Context, model FormationModel) error {
	if model.Trust != FormationTrustSovereign || a == nil || a.modelDispatcher == nil {
		return nil
	}
	return ReleaseOllamaModels(ctx, a.modelDispatcher, a.runContext.InferenceSessionID, a.runContext.RunID, []string{model.ServedModelTag}, a.environment, a.newID)
}

func (a *formationProductionAllocator) dispatchWarmup(ctx context.Context, model FormationModel, attemptID string) (*operatorv1.InferenceDispatchResponse, error) {
	if a == nil || a.dispatcher == nil {
		return nil, fmt.Errorf("formation: warmup: %w", constants.ErrFormationRunnerDependency)
	}
	dispatchReq, err := a.buildDispatchRequest(attemptID, models.InferenceModelRolePrimary, model, formationWarmupPrompt(model.ServedModelTag))
	if err != nil {
		return nil, err
	}
	resp, err := a.dispatcher.DispatchInference(ctx, dispatchReq)
	if err != nil {
		return nil, fmt.Errorf("formation: warmup %s: %w", model.ServedModelTag, err)
	}
	probeReq := InferenceProbeRequest{
		ProviderAttemptID:       attemptID,
		Role:                    models.InferenceModelRolePrimary,
		Model:                   model.ServedModelTag,
		ModelDigest:             model.ModelDigest,
		TargetOperatorSessionID: a.runContext.InferenceSessionID,
		CampaignID:              a.runContext.CampaignID,
		ModelRegistryDigest:     a.registryDigest,
		ModelRegistry:           a.registry,
	}
	if err := ValidateInferenceProbeResponse(probeReq, resp); err != nil {
		return nil, fmt.Errorf("formation: warmup %s: %w", model.ServedModelTag, err)
	}
	return resp, nil
}

func (a *formationProductionAllocator) buildDispatchRequest(attemptID string, role models.InferenceModelRole, model FormationModel, prompt string) (*operatorv1.InferenceDispatchRequest, error) {
	dispatchReq, err := BuildInferenceProbeDispatchRequest(InferenceProbeRequest{
		ProviderAttemptID:       attemptID,
		Role:                    role,
		Model:                   model.ServedModelTag,
		ModelDigest:             model.ModelDigest,
		TargetOperatorSessionID: a.runContext.InferenceSessionID,
		Prompt:                  prompt,
		CampaignID:              a.runContext.CampaignID,
		ModelRegistryDigest:     a.registryDigest,
		ModelRegistry:           a.registry,
	})
	if err != nil {
		return nil, err
	}
	dispatchReq.RunId = a.runContext.RunID
	dispatchReq.AssignmentId = a.runContext.AssignmentID
	dispatchReq.EvaluationAttemptId = a.runContext.EvaluationAttemptID
	dispatchReq.ScenarioId = a.runContext.ScenarioID
	if dispatchReq.ScenarioId == "" {
		dispatchReq.ScenarioId = formationSmokeScenarioID
	}
	return dispatchReq, nil
}

type formationProductionExecutor struct {
	runContext     FormationRunContext
	registryDigest string
	registry       []*operatorv1.InferenceModelVariant
	dispatcher     FormationInferenceDispatcher
}

func (e *formationProductionExecutor) ExecuteRole(ctx context.Context, req FormationRoleRequest) (FormationRoleResult, error) {
	role, err := formationRoleToInferenceRole(req.Role)
	if err != nil {
		return FormationRoleResult{}, err
	}
	prompt := formationRolePrompt(req.Role, req.InputState)
	allocator := &formationProductionAllocator{
		runContext:     e.runContext,
		registryDigest: e.registryDigest,
		registry:       e.registry,
		dispatcher:     e.dispatcher,
	}
	dispatchReq, err := allocator.buildDispatchRequest(req.AttemptID, role, req.Model, prompt)
	if err != nil {
		return FormationRoleResult{}, err
	}
	resp, err := e.dispatcher.DispatchInference(ctx, dispatchReq)
	if err != nil {
		return FormationRoleResult{}, fmt.Errorf("formation: execute %s: %w", req.Role, err)
	}
	probeReq := InferenceProbeRequest{
		ProviderAttemptID:       req.AttemptID,
		Role:                    role,
		Model:                   req.Model.ServedModelTag,
		ModelDigest:             req.Model.ModelDigest,
		TargetOperatorSessionID: e.runContext.InferenceSessionID,
		CampaignID:              e.runContext.CampaignID,
		ModelRegistryDigest:     e.registryDigest,
		ModelRegistry:           e.registry,
	}
	if err := ValidateInferenceProbeResponse(probeReq, resp); err != nil {
		return FormationRoleResult{}, fmt.Errorf("formation: execute %s: %w", req.Role, err)
	}
	return formationRoleResultFromInference(req, resp.GetResult())
}

type formationProductionPolicyGate struct{}

func (formationProductionPolicyGate) ValidateMutation(_ context.Context, _ string, _ FormationRole, _ []byte) (FormationPolicyValidation, error) {
	return FormationPolicyValidation{
		L1Validated: true,
		L2Validated: true,
		L3Validated: true,
		L4Validated: true,
		L5Validated: true,
		Intercepted: true,
		ReceiptRef:  "formation-inference-dispatch",
	}, nil
}

func formationRoleToInferenceRole(role FormationRole) (models.InferenceModelRole, error) {
	switch role {
	case FormationRolePrimary:
		return models.InferenceModelRolePrimary, nil
	case FormationRoleAssistant:
		return models.InferenceModelRoleAssistant, nil
	case FormationRoleLite:
		return models.InferenceModelRoleLite, nil
	default:
		return models.InferenceModelRoleUnspecified, fmt.Errorf("formation role %q: %w", role, constants.ErrFormationInvalid)
	}
}

func formationWarmupPrompt(modelTag string) string {
	return fmt.Sprintf("Reply with exactly: formation-warmup-%s-ok", modelTag)
}

func formationRolePrompt(role FormationRole, inputState []byte) string {
	if len(inputState) == 0 {
		return fmt.Sprintf("Formation role %s: begin with a concise response.", role)
	}
	return fmt.Sprintf("Formation role %s state:\n%s\nContinue with a concise response.", role, string(inputState))
}

func formationRoleResultFromInference(req FormationRoleRequest, result *operatorv1.InferenceResult) (FormationRoleResult, error) {
	if result == nil {
		return FormationRoleResult{}, fmt.Errorf("formation: role result: %w", constants.ErrMissingRequiredField)
	}
	outputText := formationCollectInferenceText(result)
	outputState := formationAppendRoleState(req.InputState, req.Role, outputText)
	roleResult := FormationRoleResult{
		OutputState:       outputState,
		MutationCandidate: append([]byte(nil), outputState...),
		StateMutation:     req.Role == FormationRolePrimary,
		ProviderAttemptID: result.GetProviderAttemptId(),
		GenerationTokens:  uint32(result.GetCompletionTokens()),
	}
	if result.TimeToFirstTokenNs != nil {
		roleResult.TTFTNanos = uint64(*result.TimeToFirstTokenNs)
	}
	if result.GenerationDurationNs != nil {
		roleResult.GenerationDurationNanos = uint64(*result.GenerationDurationNs)
	}
	return roleResult, nil
}

func formationCollectInferenceText(result *operatorv1.InferenceResult) string {
	var builder strings.Builder
	for _, part := range result.GetParts() {
		if part.GetText() != "" {
			builder.WriteString(part.GetText())
		}
	}
	return builder.String()
}

func formationAppendRoleState(input []byte, role FormationRole, outputText string) []byte {
	out := append([]byte(nil), input...)
	out = append(out, '/')
	out = append(out, string(role)...)
	out = append(out, ':')
	out = append(out, outputText...)
	return out
}
