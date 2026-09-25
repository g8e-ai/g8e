// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License 2.0.

package evaluation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/model_provenance"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	FormationMaxVRAMMiB    uint64 = 12 * 1024
	FormationSchemaVersion        = "execution-topologies.v1"
)

// FormationRole identifies one responsibility in a formation.
type FormationRole string

const (
	FormationRolePrimary   FormationRole = "primary"
	FormationRoleAssistant FormationRole = "assistant"
	FormationRoleLite      FormationRole = "lite"
)

// FormationTrust classifies the model boundary used by a formation role.
type FormationTrust string

const (
	FormationTrustSovereign FormationTrust = "sovereign"
	FormationTrustDelegated FormationTrust = "delegated"
)

// FormationAttestationStatus identifies the provenance state recorded for a role.
type FormationAttestationStatus string

const (
	FormationAttestationVerified  FormationAttestationStatus = "verified"
	FormationAttestationNotNeeded FormationAttestationStatus = "not_needed"
)

// FormationModel is the typed model contract used by an execution topology.
// Estimated memory is intentionally explicit: the runner rejects a formation
// before any local model allocation when its model and KV estimates exceed the
// 16 GB host safety budget.
type FormationModel struct {
	VariantID             string
	DisplayName           string
	Provider              string
	Family                string
	ProviderClass         string
	ServedModelTag        string
	Trust                 FormationTrust
	Quantization          string
	ParameterCount        uint64
	EstimatedModelVRAMMiB uint64
	EstimatedKVCacheMiB   uint64
	ModelDigest           string
}

// Formation binds the three roles that make up one benchmark topology.
type Formation struct {
	ID          string
	DisplayName string
	Description string
	MaxVRAMMiB  uint64
	Primary     FormationModel
	Assistant   FormationModel
	Lite        FormationModel
	// RelaxedValidation skips catalog provider/family/VRAM gates for campaign
	// heterogeneous stacks that already passed scheduler digest validation.
	RelaxedValidation bool
}

// Roles returns the execution order. Lite runs first as the L1 gatekeeper,
// then Assistant handles fast work, and Primary receives the accumulated state.
func (f Formation) Roles() []FormationRole {
	return []FormationRole{FormationRoleLite, FormationRoleAssistant, FormationRolePrimary}
}

// Model returns the immutable model binding for one role.
func (f Formation) Model(role FormationRole) (FormationModel, error) {
	switch role {
	case FormationRolePrimary:
		return f.Primary, nil
	case FormationRoleAssistant:
		return f.Assistant, nil
	case FormationRoleLite:
		return f.Lite, nil
	default:
		return FormationModel{}, fmt.Errorf("formation: model for role %q: %w", role, constants.ErrFormationInvalid)
	}
}

// Models returns the three model bindings in declaration order.
func (f Formation) Models() []FormationModel {
	return []FormationModel{f.Primary, f.Assistant, f.Lite}
}

// EstimatedVRAMMiB returns local model weights plus local KV-cache estimates.
func (f Formation) EstimatedVRAMMiB() uint64 {
	var total uint64
	for _, model := range f.Models() {
		if model.Trust == FormationTrustSovereign {
			total += model.EstimatedModelVRAMMiB + model.EstimatedKVCacheMiB
		}
	}
	return total
}

// Validate enforces the formation's role, trust, heterogeneity, and hardware
// constraints before the runner can allocate a model.
func (f Formation) Validate() error {
	if f.ID == "" || f.DisplayName == "" || f.Description == "" || f.MaxVRAMMiB == 0 {
		return fmt.Errorf("formation %q: %w", f.ID, constants.ErrFormationInvalid)
	}
	if !f.RelaxedValidation {
		seenProviders := make(map[string]FormationRole, 3)
		seenFamilies := make(map[string]FormationRole, 3)
		for _, role := range []FormationRole{FormationRolePrimary, FormationRoleAssistant, FormationRoleLite} {
			model, err := f.Model(role)
			if err != nil {
				return err
			}
			provider := formationIdentity(model.Provider)
			if previous, exists := seenProviders[provider]; exists {
				return fmt.Errorf("formation %q: roles %s and %s share provider %q: %w", f.ID, previous, role, model.Provider, constants.ErrFormationProviderOverlap)
			}
			seenProviders[provider] = role
			family := formationIdentity(model.Family)
			if previous, exists := seenFamilies[family]; exists {
				return fmt.Errorf("formation %q: roles %s and %s share family %q: %w", f.ID, previous, role, model.Family, constants.ErrFormationFamilyOverlap)
			}
			seenFamilies[family] = role
		}
		if f.EstimatedVRAMMiB() >= f.MaxVRAMMiB {
			return fmt.Errorf("formation %q: estimated %d MiB must remain below %d MiB: %w", f.ID, f.EstimatedVRAMMiB(), f.MaxVRAMMiB, constants.ErrFormationVRAMBudgetExceeded)
		}
	}
	for _, role := range []FormationRole{FormationRolePrimary, FormationRoleAssistant, FormationRoleLite} {
		model, err := f.Model(role)
		if err != nil {
			return err
		}
		if err := validateFormationModel(role, model); err != nil {
			return err
		}
	}
	return nil
}

func validateFormationModel(role FormationRole, model FormationModel) error {
	if model.VariantID == "" || model.DisplayName == "" || model.Provider == "" || model.Family == "" || model.ProviderClass == "" || model.ServedModelTag == "" {
		return fmt.Errorf("formation role %s: %w", role, constants.ErrFormationInvalid)
	}
	switch model.Trust {
	case FormationTrustSovereign:
		if model.ProviderClass != "ollama" || model.Quantization == "" {
			return fmt.Errorf("formation role %s: sovereign model must use Ollama and declare quantization: %w", role, constants.ErrFormationInvalid)
		}
		if !isSupportedLocalQuantization(model.Quantization) {
			return fmt.Errorf("formation role %s: unsupported local quantization %q: %w", role, model.Quantization, constants.ErrFormationInvalid)
		}
	case FormationTrustDelegated:
		if model.ProviderClass == "ollama" || model.Quantization != "" || model.EstimatedModelVRAMMiB != 0 {
			return fmt.Errorf("formation role %s: delegated model cannot declare local allocation: %w", role, constants.ErrFormationInvalid)
		}
	default:
		return fmt.Errorf("formation role %s: unknown trust class %q: %w", role, model.Trust, constants.ErrFormationInvalid)
	}
	return nil
}

func isSupportedLocalQuantization(value string) bool {
	quantization := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(quantization, "q4") || strings.HasPrefix(quantization, "q8")
}

func formationIdentity(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

// ToModelVariants converts the formation into the campaign registry shape.
func (f Formation) ToModelVariants() ([]*evalv1.ModelVariant, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	variants := make([]*evalv1.ModelVariant, 0, 3)
	for _, model := range f.Models() {
		variants = append(variants, &evalv1.ModelVariant{
			VariantId:      model.VariantID,
			ProviderClass:  model.ProviderClass,
			ServedModelTag: model.ServedModelTag,
			ModelDigest:    model.ModelDigest,
			ModelFamily:    model.Family,
			ParameterCount: model.ParameterCount,
			Quantization:   model.Quantization,
		})
	}
	return variants, nil
}

// ToStackDefinition converts the formation into the canonical campaign stack.
func (f Formation) ToStackDefinition() (*evalv1.HeterogeneousStackDefinition, error) {
	variants, err := f.ToModelVariants()
	if err != nil {
		return nil, err
	}
	stack, err := materializeStack(f.ID, f.ID, variants[0], variants[1], variants[2])
	if err != nil {
		return nil, err
	}
	return stack, nil
}

// ExecutionTopologies is the checked-in catalog of benchmark formations.
type ExecutionTopologies struct {
	formations []Formation
}

// NewExecutionTopologies returns the five preregistered benchmark formations.
func NewExecutionTopologies() (*ExecutionTopologies, error) {
	formations := defaultExecutionTopologies()
	for _, formation := range formations {
		if err := formation.Validate(); err != nil {
			return nil, err
		}
	}
	return &ExecutionTopologies{formations: formations}, nil
}

// Formations returns a copy of the checked-in formation catalog.
func (t *ExecutionTopologies) Formations() []Formation {
	if t == nil {
		return nil
	}
	return append([]Formation(nil), t.formations...)
}

// Formation returns one catalog entry by stable ID.
func (t *ExecutionTopologies) Formation(id string) (Formation, error) {
	if t == nil || id == "" {
		return Formation{}, fmt.Errorf("formation %q: %w", id, constants.ErrFormationInvalid)
	}
	for _, formation := range t.formations {
		if formation.ID == id {
			return formation, nil
		}
	}
	return Formation{}, fmt.Errorf("formation %q was not found: %w", id, constants.ErrFormationInvalid)
}

func defaultExecutionTopologies() []Formation {
	return []Formation{
		{
			ID: "heavy-reasoner", DisplayName: "Heavy Reasoner", Description: "Maximum local VRAM formation for a deep primary reasoner.", MaxVRAMMiB: FormationMaxVRAMMiB,
			Primary:   formationModel("qwen25-14b", "Qwen 2.5 14B", "Alibaba", "Qwen 2.5", "qwen2.5:14b-instruct-q4_K_M", 14_000_000_000, "Q4_K_M", 8704, 512),
			Assistant: formationModel("gemma2-2b", "Gemma 2 2B", "Google", "Gemma 2", "gemma2:2b-instruct-q4_K_M", 2_000_000_000, "Q4_K_M", 1536, 256),
			Lite:      formationModel("llama32-1b", "Llama 3.2 1B", "Meta", "Llama 3.2", "llama3.2:1b-instruct-q4_K_M", 1_000_000_000, "Q4_K_M", 768, 256),
		},
		{
			ID: "enterprise-polyglot", DisplayName: "Enterprise Polyglot", Description: "Balanced local formation across three independent model lineages.", MaxVRAMMiB: FormationMaxVRAMMiB,
			Primary:   formationModel("llama31-8b", "Llama 3.1 8B", "Meta", "Llama 3.1", "llama3.1:8b-instruct-q4_K_M", 8_000_000_000, "Q4_K_M", 5120, 512),
			Assistant: formationModel("phi35-mini-38b", "Phi-3.5 Mini 3.8B", "Microsoft", "Phi-3.5", "phi3.5:3.8b-mini-instruct-q4_K_M", 3_800_000_000, "Q4_K_M", 2560, 512),
			Lite:      formationModel("qwen25-05b", "Qwen 2.5 0.5B", "Alibaba", "Qwen 2.5", "qwen2.5:0.5b-instruct-q4_K_M", 500_000_000, "Q4_K_M", 512, 256),
		},
		{
			ID: "code-logic-edge", DisplayName: "Code & Logic Edge", Description: "Coding-focused primary and assistant models within the edge budget.", MaxVRAMMiB: FormationMaxVRAMMiB,
			Primary:   formationModel("gemma2-9b", "Gemma 2 9B", "Google", "Gemma 2", "gemma2:9b-instruct-q4_K_M", 9_000_000_000, "Q4_K_M", 5632, 512),
			Assistant: formationModel("qwen25-coder-7b", "Qwen 2.5 Coder 7B", "Alibaba", "Qwen 2.5 Coder", "qwen2.5-coder:7b-instruct-q4_K_M", 7_000_000_000, "Q4_K_M", 4352, 512),
			Lite:      formationModel("llama32-1b-edge", "Llama 3.2 1B", "Meta", "Llama 3.2", "llama3.2:1b-instruct-q4_K_M", 1_000_000_000, "Q4_K_M", 768, 256),
		},
		{
			ID: "ultra-light-speedster", DisplayName: "Ultra-Light Speedster", Description: "High-throughput local formation for latency-sensitive tasks.", MaxVRAMMiB: FormationMaxVRAMMiB,
			Primary:   formationModel("phi35-mini-38b-speed", "Phi-3.5 Mini 3.8B", "Microsoft", "Phi-3.5", "phi3.5:3.8b-mini-instruct-q4_K_M", 3_800_000_000, "Q4_K_M", 2560, 256),
			Assistant: formationModel("gemma2-2b-speed", "Gemma 2 2B", "Google", "Gemma 2", "gemma2:2b-instruct-q4_K_M", 2_000_000_000, "Q4_K_M", 1536, 256),
			Lite:      formationModel("qwen25-05b-speed", "Qwen 2.5 0.5B", "Alibaba", "Qwen 2.5", "qwen2.5:0.5b-instruct-q4_K_M", 500_000_000, "Q4_K_M", 512, 256),
		},
		{
			ID: "hybrid-delegator", DisplayName: "Hybrid Delegator", Description: "Delegated cloud reasoning with sovereign edge execution and gatekeeping.", MaxVRAMMiB: FormationMaxVRAMMiB,
			Primary:   FormationModel{VariantID: "gemini15-pro", DisplayName: "Gemini 1.5 Pro", Provider: "Google Cloud", Family: "Gemini 1.5", ProviderClass: "gemini", ServedModelTag: "gemini-1.5-pro", Trust: FormationTrustDelegated, ParameterCount: 0},
			Assistant: formationModel("llama31-8b-hybrid", "Llama 3.1 8B", "Meta", "Llama 3.1", "llama3.1:8b-instruct-q4_K_M", 8_000_000_000, "Q4_K_M", 5120, 512),
			Lite:      formationModel("qwen25-15b-hybrid", "Qwen 2.5 1.5B", "Alibaba", "Qwen 2.5 1.5", "qwen2.5:1.5b-instruct-q4_K_M", 1_500_000_000, "Q4_K_M", 1024, 256),
		},
	}
}

func formationModel(variantID, displayName, provider, family, tag string, parameters uint64, quantization string, modelVRAMMiB, cacheMiB uint64) FormationModel {
	return FormationModel{
		VariantID: variantID, DisplayName: displayName, Provider: provider, Family: family,
		ProviderClass: "ollama", ServedModelTag: tag, Trust: FormationTrustSovereign,
		Quantization: quantization, ParameterCount: parameters, EstimatedModelVRAMMiB: modelVRAMMiB,
		EstimatedKVCacheMiB: cacheMiB,
	}
}

// FormationAttestation is the provenance result required before local allocation.
type FormationAttestation struct {
	Verified bool
	Digest   string
	Window   *evalv1.ModelProvenanceAttestationWindow
}

// FormationObserverEvidence carries the typed provider-boundary witness.
type FormationObserverEvidence struct {
	Window *evalv1.ProviderBoundaryObservationWindow
}

// FormationProvenanceOperator attests sovereign model weights at their storage boundary.
type FormationProvenanceOperator interface {
	Attest(ctx context.Context, model FormationModel) (*FormationAttestation, error)
}

// FormationProviderObserver brackets one role execution at the provider boundary.
type FormationProviderObserver interface {
	Begin(ctx context.Context, attemptID string, model FormationModel) error
	Finalize(ctx context.Context, attemptID string, model FormationModel, failed bool) (*FormationObserverEvidence, error)
}

// FormationAllocator owns local model residency and VRAM allocation.
type FormationAllocator interface {
	Allocate(ctx context.Context, model FormationModel) error
	Release(ctx context.Context, model FormationModel) error
}

// FormationRoleRequest is the state handoff passed to one role.
type FormationRoleRequest struct {
	FormationID       string
	AttemptID         string
	Role              FormationRole
	Model             FormationModel
	InputState        []byte
	MutationCandidate []byte
}

// FormationRoleResult is the governed execution result for one role.
type FormationRoleResult struct {
	OutputState             []byte
	MutationCandidate       []byte
	StateMutation           bool
	ProviderAttemptID       string
	TTFTNanos               uint64
	GenerationTokens        uint32
	GenerationDurationNanos uint64
	PeakVRAMMiB             uint64
}

// FormationRoleExecutor invokes the role through the governed model path.
type FormationRoleExecutor interface {
	ExecuteRole(ctx context.Context, req FormationRoleRequest) (FormationRoleResult, error)
}

// FormationPolicyValidation records the five-layer mutation decision.
type FormationPolicyValidation struct {
	L1Validated bool
	L2Validated bool
	L3Validated bool
	L4Validated bool
	L5Validated bool
	Intercepted bool
	ReceiptRef  string
}

func (p FormationPolicyValidation) valid() bool {
	return p.L1Validated && p.L2Validated && p.L3Validated && p.L4Validated && p.L5Validated && p.Intercepted
}

// FormationPolicyGate is the Gateway-facing mutation verification boundary.
type FormationPolicyGate interface {
	ValidateMutation(ctx context.Context, formationID string, role FormationRole, payload []byte) (FormationPolicyValidation, error)
}

// FormationRoleTelemetry is the audit-safe telemetry emitted for one role.
type FormationRoleTelemetry struct {
	Role                    FormationRole
	Model                   FormationModel
	AttemptID               string
	AttestationStatus       FormationAttestationStatus
	AttestationVerified     bool
	AttestationDigest       string
	ProviderAttemptID       string
	PeakVRAMMiB             uint64
	TTFTNanos               uint64
	GenerationTokens        uint32
	GenerationDurationNanos uint64
	GenerationTokensPerSec  float64
	StateMutation           bool
	PolicyValidation        FormationPolicyValidation
	ObserverEvidence        *FormationObserverEvidence
	ProvenanceEvidence      *FormationAttestation
}

// FormationRunResult is the benchmark output for one formation.
type FormationRunResult struct {
	SchemaVersion        string
	FormationID          string
	Passed               bool
	PeakVRAMMiB          uint64
	Roles                []FormationRoleTelemetry
	MutationIntercepted  bool
	AllPolicyLayersValid bool
}

// FormationRunner runs Lite → Assistant → Primary with explicit witness and
// governance dependencies. It never allocates a sovereign model before its
// storage-side attestation passes.
type FormationRunner struct {
	provenance FormationProvenanceOperator
	observer   FormationProviderObserver
	allocator  FormationAllocator
	executor   FormationRoleExecutor
	policy     FormationPolicyGate
	now        func() time.Time
	newID      func(string) string
}

// NewFormationRunner constructs a runner for one governed execution topology.
func NewFormationRunner(provenance FormationProvenanceOperator, observer FormationProviderObserver, allocator FormationAllocator, executor FormationRoleExecutor, policy FormationPolicyGate, now func() time.Time, newID func(string) string) (*FormationRunner, error) {
	if executor == nil || policy == nil || observer == nil {
		return nil, fmt.Errorf("formation: construct runner: %w", constants.ErrFormationRunnerDependency)
	}
	if now == nil {
		now = time.Now
	}
	if newID == nil {
		newID = func(prefix string) string { return fmt.Sprintf("%s-%d", prefix, now().UTC().UnixNano()) }
	}
	return &FormationRunner{provenance: provenance, observer: observer, allocator: allocator, executor: executor, policy: policy, now: now, newID: newID}, nil
}

// Run executes one formation and returns canonical role telemetry. State is
// passed as opaque bytes so the governed model path remains the owner of its
// typed payload and content-addressing rules.
func (r *FormationRunner) Run(ctx context.Context, formation Formation, initialState []byte) (result *FormationRunResult, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := formation.Validate(); err != nil {
		return &FormationRunResult{SchemaVersion: FormationSchemaVersion, FormationID: formation.ID}, err
	}
	if r == nil || r.executor == nil || r.policy == nil || r.observer == nil {
		return nil, fmt.Errorf("formation: run: %w", constants.ErrFormationRunnerDependency)
	}
	result = &FormationRunResult{SchemaVersion: FormationSchemaVersion, FormationID: formation.ID, Roles: make([]FormationRoleTelemetry, 0, 3)}
	allocated := make([]FormationModel, 0, 3)
	defer func() {
		for index := len(allocated) - 1; index >= 0; index-- {
			if releaseErr := r.allocator.Release(ctx, allocated[index]); releaseErr != nil && err == nil {
				err = fmt.Errorf("formation: release %s: %w", allocated[index].ServedModelTag, releaseErr)
				result.Passed = false
			}
		}
	}()

	attestations := make([]FormationAttestation, 3)
	for index, role := range []FormationRole{FormationRolePrimary, FormationRoleAssistant, FormationRoleLite} {
		model, modelErr := formation.Model(role)
		if modelErr != nil {
			return result, modelErr
		}
		if model.Trust == FormationTrustDelegated {
			attestations[index] = FormationAttestation{Verified: true}
			continue
		}
		if r.provenance == nil || model.ModelDigest == "" {
			return result, fmt.Errorf("formation: attest %s: %w", model.ServedModelTag, constants.ErrFormationAttestationRequired)
		}
		attestation, attestErr := r.provenance.Attest(ctx, model)
		if attestErr != nil {
			return result, fmt.Errorf("formation: attest %s: %w: %v", model.ServedModelTag, constants.ErrFormationAttestationFailed, attestErr)
		}
		if attestation == nil || !attestation.Verified {
			return result, fmt.Errorf("formation: attest %s: %w", model.ServedModelTag, constants.ErrFormationAttestationFailed)
		}
		attestations[index] = *attestation
	}
	if r.allocator == nil && formation.EstimatedVRAMMiB() > 0 {
		return result, fmt.Errorf("formation: allocate local models: %w", constants.ErrFormationRunnerDependency)
	}
	for _, role := range []FormationRole{FormationRolePrimary, FormationRoleAssistant, FormationRoleLite} {
		model, _ := formation.Model(role)
		if model.Trust != FormationTrustSovereign {
			continue
		}
		if allocErr := r.allocator.Allocate(ctx, model); allocErr != nil {
			if isFormationOOM(allocErr) {
				return result, fmt.Errorf("formation: allocate %s: %w", model.ServedModelTag, constants.ErrFormationOutOfMemory)
			}
			return result, fmt.Errorf("formation: allocate %s: %w", model.ServedModelTag, allocErr)
		}
		allocated = append(allocated, model)
	}

	state := append([]byte(nil), initialState...)
	for _, role := range formation.Roles() {
		model, _ := formation.Model(role)
		attemptID := r.newID(fmt.Sprintf("%s-%s", formation.ID, role))
		if err := r.observer.Begin(ctx, attemptID, model); err != nil {
			return result, fmt.Errorf("formation: observer begin %s: %w", role, err)
		}
		roleResult, executeErr := r.executor.ExecuteRole(ctx, FormationRoleRequest{
			FormationID: formation.ID, AttemptID: attemptID, Role: role, Model: model, InputState: append([]byte(nil), state...), MutationCandidate: append([]byte(nil), state...),
		})
		failed := executeErr != nil
		observation, observeErr := r.observer.Finalize(ctx, attemptID, model, failed)
		if observeErr != nil {
			if executeErr != nil {
				return result, fmt.Errorf("formation: role %s: %w", role, errors.Join(executeErr, observeErr))
			}
			return result, fmt.Errorf("formation: observer finalize %s: %w", role, observeErr)
		}
		if executeErr != nil {
			if isFormationOOM(executeErr) {
				return result, fmt.Errorf("formation: execute %s: %w", role, constants.ErrFormationOutOfMemory)
			}
			return result, fmt.Errorf("formation: execute %s: %w", role, executeErr)
		}
		if observation == nil || observation.Window == nil {
			return result, fmt.Errorf("formation: observer evidence %s: %w", role, constants.ErrFormationWitnessUnavailable)
		}
		telemetry := FormationRoleTelemetry{
			Role: role, Model: model, AttemptID: attemptID,
			AttestationStatus:   formationAttestationStatus(model.Trust),
			AttestationVerified: model.Trust == FormationTrustDelegated || attestations[formationAttestationIndex(role)].Verified,
			AttestationDigest:   attestations[formationAttestationIndex(role)].Digest,
			ProviderAttemptID:   roleResult.ProviderAttemptID, PeakVRAMMiB: roleResult.PeakVRAMMiB,
			TTFTNanos: roleResult.TTFTNanos, GenerationTokens: roleResult.GenerationTokens,
			GenerationDurationNanos: roleResult.GenerationDurationNanos, StateMutation: roleResult.StateMutation,
			ObserverEvidence: observation,
			ProvenanceEvidence: bindFormationProvenanceEvidence(attestationForRole(attestations, role), roleResult.ProviderAttemptID),
		}
		if telemetry.GenerationDurationNanos > 0 {
			telemetry.GenerationTokensPerSec = float64(telemetry.GenerationTokens) / (float64(telemetry.GenerationDurationNanos) / float64(time.Second))
		}
		if telemetry.PeakVRAMMiB == 0 {
			telemetry.PeakVRAMMiB = observedPeakVRAMMiB(observation.Window)
		}
		if telemetry.PeakVRAMMiB > result.PeakVRAMMiB {
			result.PeakVRAMMiB = telemetry.PeakVRAMMiB
		}
		if roleResult.StateMutation {
			validation, policyErr := r.policy.ValidateMutation(ctx, formation.ID, role, roleResult.MutationCandidate)
			if policyErr != nil {
				return result, fmt.Errorf("formation: policy validation %s: %w", role, policyErr)
			}
			telemetry.PolicyValidation = validation
			if !validation.valid() {
				return result, fmt.Errorf("formation: policy validation %s: %w", role, constants.ErrFormationPolicyValidation)
			}
			result.MutationIntercepted = true
			result.AllPolicyLayersValid = true
		}
		result.Roles = append(result.Roles, telemetry)
		state = append([]byte(nil), roleResult.OutputState...)
	}
	result.Passed = true
	return result, nil
}

func formationAttestationStatus(trust FormationTrust) FormationAttestationStatus {
	if trust == FormationTrustDelegated {
		return FormationAttestationNotNeeded
	}
	return FormationAttestationVerified
}

func formationAttestationIndex(role FormationRole) int {
	switch role {
	case FormationRolePrimary:
		return 0
	case FormationRoleAssistant:
		return 1
	default:
		return 2
	}
}

func attestationForRole(attestations []FormationAttestation, role FormationRole) *FormationAttestation {
	attestation := attestations[formationAttestationIndex(role)]
	return &attestation
}

func bindFormationProvenanceEvidence(attestation *FormationAttestation, providerAttemptID string) *FormationAttestation {
	if attestation == nil || attestation.Window == nil || providerAttemptID == "" {
		return attestation
	}
	if attestation.Window.GetProviderAttemptId() == providerAttemptID {
		return attestation
	}
	cloned, ok := proto.Clone(attestation.Window).(*evalv1.ModelProvenanceAttestationWindow)
	if !ok {
		return attestation
	}
	cloned.ProviderAttemptId = providerAttemptID
	digest, err := model_provenance.ComputeAttestationDigest(cloned)
	if err != nil {
		return attestation
	}
	cloned.AttestationDigest = digest
	return &FormationAttestation{
		Verified: attestation.Verified,
		Digest:   attestation.Digest,
		Window:   cloned,
	}
}

func observedPeakVRAMMiB(window *evalv1.ProviderBoundaryObservationWindow) uint64 {
	var peak uint64
	if window == nil {
		return 0
	}
	for _, sample := range window.GetSamples() {
		if sample.GetVramBytesAvailability() != evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED {
			continue
		}
		value := sample.GetVramUsedBytes() / (1024 * 1024)
		if value > peak {
			peak = value
		}
	}
	return peak
}

func isFormationOOM(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, constants.ErrFormationOutOfMemory) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "out of memory") || strings.Contains(message, "oom") || strings.Contains(message, "cuda error 2")
}
