// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License 2.0.

package evaluation

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestNewExecutionTopologies_ReturnsFiveValidTargetFormations(t *testing.T) {
	topologies, err := NewExecutionTopologies()
	require.NoError(t, err)
	formations := topologies.Formations()
	require.Len(t, formations, 5)
	for _, formation := range formations {
		require.NoError(t, formation.Validate())
		assert.Less(t, formation.EstimatedVRAMMiB(), formation.MaxVRAMMiB)
		assert.Less(t, formation.EstimatedVRAMMiB(), FormationMaxVRAMMiB)
	}

	hybrid, err := topologies.Formation("hybrid-delegator")
	require.NoError(t, err)
	assert.Equal(t, FormationTrustDelegated, hybrid.Primary.Trust)
	assert.Equal(t, FormationAttestationNotNeeded, formationAttestationStatus(hybrid.Primary.Trust))
	assert.Equal(t, "gemini-1.5-pro", hybrid.Primary.ServedModelTag)
}

func TestFormationValidateRejectsProviderAndFamilyOverlap(t *testing.T) {
	topologies, err := NewExecutionTopologies()
	require.NoError(t, err)
	formation, err := topologies.Formation("enterprise-polyglot")
	require.NoError(t, err)

	providerOverlap := formation
	providerOverlap.Assistant.Provider = providerOverlap.Primary.Provider
	err = providerOverlap.Validate()
	assert.ErrorIs(t, err, constants.ErrFormationProviderOverlap)

	familyOverlap := formation
	familyOverlap.Assistant.Family = familyOverlap.Primary.Family
	err = familyOverlap.Validate()
	assert.ErrorIs(t, err, constants.ErrFormationFamilyOverlap)
}

func TestFormationToStackDefinitionUsesCanonicalRoleBindings(t *testing.T) {
	topologies, err := NewExecutionTopologies()
	require.NoError(t, err)
	formation, err := topologies.Formation("code-logic-edge")
	require.NoError(t, err)

	stack, err := formation.ToStackDefinition()
	require.NoError(t, err)
	assert.Equal(t, formation.ID, stack.GetStackId())
	assert.Equal(t, formation.Primary.VariantID, stack.GetPrimarySlot().GetVariantId())
	assert.Equal(t, formation.Assistant.VariantID, stack.GetAssistantSlot().GetVariantId())
	assert.Equal(t, formation.Lite.VariantID, stack.GetLiteSlot().GetVariantId())
	require.NoError(t, ValidateHeterogeneousStackDigest(stack))
}

type formationTestProvenance struct {
	events []string
}

func (p *formationTestProvenance) Attest(_ context.Context, model FormationModel) (*FormationAttestation, error) {
	p.events = append(p.events, "attest:"+model.VariantID)
	return &FormationAttestation{Verified: true, Digest: model.ModelDigest}, nil
}

type formationTestObserver struct {
	events []string
}

func (o *formationTestObserver) Begin(_ context.Context, attemptID string, model FormationModel) error {
	o.events = append(o.events, "begin:"+attemptID+":"+model.VariantID)
	return nil
}

func (o *formationTestObserver) Finalize(_ context.Context, attemptID string, model FormationModel, failed bool) (*FormationObserverEvidence, error) {
	o.events = append(o.events, "finalize:"+attemptID+":"+model.VariantID)
	return &FormationObserverEvidence{Window: &evalv1.ProviderBoundaryObservationWindow{
		ProviderAttemptId: attemptID,
		Samples: []*evalv1.ProviderBoundaryHardwareSample{{
			VramBytesAvailability: evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
			VramUsedBytes:         2 * 1024 * 1024 * 1024,
		}},
	}}, nil
}

type formationTestAllocator struct {
	events []string
}

func (a *formationTestAllocator) Allocate(_ context.Context, model FormationModel) error {
	a.events = append(a.events, "allocate:"+model.VariantID)
	return nil
}

func (a *formationTestAllocator) Release(_ context.Context, model FormationModel) error {
	a.events = append(a.events, "release:"+model.VariantID)
	return nil
}

type formationTestExecutor struct {
	states []string
}

func (e *formationTestExecutor) ExecuteRole(_ context.Context, req FormationRoleRequest) (FormationRoleResult, error) {
	e.states = append(e.states, string(req.Role)+":"+string(req.InputState))
	output := append(append([]byte(nil), req.InputState...), []byte("/"+req.Role)...)
	return FormationRoleResult{
		OutputState:             output,
		MutationCandidate:       []byte("mutation:" + string(req.Role)),
		StateMutation:           req.Role == FormationRolePrimary,
		ProviderAttemptID:       req.AttemptID,
		TTFTNanos:               2 * uint64(time.Millisecond),
		GenerationTokens:        20,
		GenerationDurationNanos: 10 * uint64(time.Millisecond),
	}, nil
}

type formationTestPolicy struct {
	calls int
}

func (p *formationTestPolicy) ValidateMutation(_ context.Context, _ string, _ FormationRole, _ []byte) (FormationPolicyValidation, error) {
	p.calls++
	return FormationPolicyValidation{L1Validated: true, L2Validated: true, L3Validated: true, L4Validated: true, L5Validated: true, Intercepted: true, ReceiptRef: "receipt-1"}, nil
}

func formationWithDigests(t *testing.T, id string) Formation {
	t.Helper()
	topologies, err := NewExecutionTopologies()
	require.NoError(t, err)
	formation, err := topologies.Formation(id)
	require.NoError(t, err)
	formation.Primary.ModelDigest = "primary-digest"
	formation.Assistant.ModelDigest = "assistant-digest"
	formation.Lite.ModelDigest = "lite-digest"
	return formation
}

func TestFormationRunnerAttestsBeforeAllocationAndPassesStateThroughGovernance(t *testing.T) {
	formation := formationWithDigests(t, "heavy-reasoner")
	provenance := &formationTestProvenance{}
	observer := &formationTestObserver{}
	allocator := &formationTestAllocator{}
	executor := &formationTestExecutor{}
	policy := &formationTestPolicy{}
	runner, err := NewFormationRunner(provenance, observer, allocator, executor, policy, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-attempt" })
	require.NoError(t, err)

	result, err := runner.Run(context.Background(), formation, []byte("initial"))
	require.NoError(t, err)
	require.True(t, result.Passed)
	assert.Equal(t, uint64(2048), result.PeakVRAMMiB)
	assert.True(t, result.MutationIntercepted)
	assert.True(t, result.AllPolicyLayersValid)
	assert.Equal(t, 1, policy.calls)
	assert.Equal(t, []string{
		"lite:initial",
		"assistant:initial/lite",
		"primary:initial/lite/assistant",
	}, executor.states)
	assert.Len(t, provenance.events, 3)
	assert.Equal(t, []string{"allocate:qwen25-14b", "allocate:gemma2-2b", "allocate:llama32-1b"}, allocator.events[:3])
	assert.Len(t, result.Roles, 3)
	assert.Equal(t, FormationAttestationVerified, result.Roles[0].AttestationStatus)
	assert.InDelta(t, 2000.0, result.Roles[0].GenerationTokensPerSec, 0.1)
	assert.Len(t, allocator.events, 6)
}

func TestFormationRunnerSkipsProvenanceForDelegatedPrimary(t *testing.T) {
	formation := formationWithDigests(t, "hybrid-delegator")
	provenance := &formationTestProvenance{}
	runner, err := NewFormationRunner(provenance, &formationTestObserver{}, &formationTestAllocator{}, &formationTestExecutor{}, &formationTestPolicy{}, time.Now, nil)
	require.NoError(t, err)

	result, err := runner.Run(context.Background(), formation, nil)
	require.NoError(t, err)
	require.Len(t, result.Roles, 3)
	assert.Equal(t, FormationAttestationNotNeeded, result.Roles[2].AttestationStatus)
	assert.Len(t, provenance.events, 2)
}

type formationOOMExecutor struct{}

func (formationOOMExecutor) ExecuteRole(_ context.Context, _ FormationRoleRequest) (FormationRoleResult, error) {
	return FormationRoleResult{}, fmt.Errorf("CUDA out of memory")
}

func TestFormationRunnerFailsBenchmarkOnOOM(t *testing.T) {
	formation := formationWithDigests(t, "ultra-light-speedster")
	runner, err := NewFormationRunner(&formationTestProvenance{}, &formationTestObserver{}, &formationTestAllocator{}, formationOOMExecutor{}, &formationTestPolicy{}, time.Now, nil)
	require.NoError(t, err)

	result, err := runner.Run(context.Background(), formation, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFormationOutOfMemory)
	assert.False(t, result.Passed)
}
