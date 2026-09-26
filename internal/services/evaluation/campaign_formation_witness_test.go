// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/services/inference/model_provenance"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/provider_observer"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func testFormationObserverWindow(t *testing.T, providerAttemptID string) *evalv1.ProviderBoundaryObservationWindow {
	t.Helper()
	window := &evalv1.ProviderBoundaryObservationWindow{
		SchemaVersion:              provider_observer.SchemaVersion,
		ProviderAttemptId:          providerAttemptID,
		ObserverId:                 "formation-test-observer",
		ObserverClockSource:        provider_observer.DefaultObserverClockSource,
		WindowStartedAtUnixNanos:   uint64(time.Unix(1_700_000_000, 0).UnixNano()),
		WindowCompletedAtUnixNanos: uint64(time.Unix(1_700_000_010, 0).UnixNano()),
		AttemptStartedAtUnixMs:     time.Unix(1_700_000_000, 0).UnixMilli(),
		AttemptCompletedAtUnixMs:   time.Unix(1_700_000_010, 0).UnixMilli(),
		Samples: []*evalv1.ProviderBoundaryHardwareSample{{
			ObservedAtUnixNanos:   uint64(time.Unix(1_700_000_001, 0).UnixNano()),
			VramBytesAvailability: evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
			VramUsedBytes:         2048 * 1024 * 1024,
		}},
	}
	digest, err := provider_observer.ComputeObservationDigest(window)
	require.NoError(t, err)
	window.ObservationDigest = digest
	return window
}

func TestPersistFormationWitnessEvidence_StoresObserverWindows(t *testing.T) {
	files := newCampaignMemoryFileService()
	observationReader, err := NewCampaignProviderObservationReader(files)
	require.NoError(t, err)
	provenanceReader, err := NewCampaignModelProvenanceReader(files)
	require.NoError(t, err)
	witnessReader := NewCampaignFormationWitnessReader(observationReader, provenanceReader)

	formationResult := &FormationRunResult{
		Roles: []FormationRoleTelemetry{{
			Role:              FormationRoleLite,
			ProviderAttemptID: "attempt-lite",
			ObserverEvidence: &FormationObserverEvidence{
				Window: testFormationObserverWindow(t, "attempt-lite"),
			},
		}},
	}
	require.NoError(t, witnessReader.PersistFormationWitnessEvidence(context.Background(), formationResult))

	loaded, err := observationReader.LoadObservationWindow(context.Background(), "attempt-lite")
	require.NoError(t, err)
	assert.Equal(t, formationResult.Roles[0].ObserverEvidence.Window.GetObservationDigest(), loaded.GetObservationDigest())
}

func TestVerifyFormationWitnessEvidence_AcceptsMatchingObserverDigest(t *testing.T) {
	files := newCampaignMemoryFileService()
	observationReader, err := NewCampaignProviderObservationReader(files)
	require.NoError(t, err)
	window := testFormationObserverWindow(t, "attempt-lite")
	require.NoError(t, observationReader.ImportObservationWindow(context.Background(), window))

	evidence := &FormationRunEvidence{
		Result: persistedFormationRunResult{
			Roles: []persistedFormationRoleTelemetry{{
				Role:                      string(FormationRoleLite),
				ProviderAttemptID:         "attempt-lite",
				ObserverObservationDigest: window.GetObservationDigest(),
				AttestationStatus:         string(FormationAttestationVerified),
			}},
		},
	}
	failures := VerifyFormationWitnessEvidence(
		context.Background(),
		evidence,
		observationReader,
		nil,
		ProviderObservationPolicyStrict,
		ModelProvenancePolicyInterim,
	)
	assert.Empty(t, failures)
}

func testFormationProvenanceWindow(t *testing.T, providerAttemptID, modelDigest string) *evalv1.ModelProvenanceAttestationWindow {
	t.Helper()
	window := &evalv1.ModelProvenanceAttestationWindow{
		SchemaVersion:              "1.0.0",
		ProviderAttemptId:          providerAttemptID,
		ProvenanceOperatorId:       "formation-test-provenance",
		ServedModelTag:             "probe-model:7b",
		ExpectedModelDigest:        modelDigest,
		ObservedModelDigest:        modelDigest,
		ManifestDigest:             strings.Repeat("b", 64),
		ManifestVerificationStatus: evalv1.ModelManifestVerificationStatus_MODEL_MANIFEST_VERIFICATION_STATUS_UNSIGNED,
		AttestedAtUnixMs:           time.Unix(1_700_000_000, 0).UnixMilli(),
		DigestMatch:                true,
	}
	digest, err := model_provenance.ComputeAttestationDigest(window)
	require.NoError(t, err)
	window.AttestationDigest = digest
	return window
}

func TestPersistFormationWitnessEvidence_StoresProvenanceWindows(t *testing.T) {
	files := newCampaignMemoryFileService()
	observationReader, err := NewCampaignProviderObservationReader(files)
	require.NoError(t, err)
	provenanceReader, err := NewCampaignModelProvenanceReader(files)
	require.NoError(t, err)
	witnessReader := NewCampaignFormationWitnessReader(observationReader, provenanceReader)

	modelDigest := strings.Repeat("a", 64)
	window := testFormationProvenanceWindow(t, "attempt-lite", modelDigest)
	formationResult := &FormationRunResult{
		Roles: []FormationRoleTelemetry{{
			Role:              FormationRoleLite,
			ProviderAttemptID: "attempt-lite",
			ProvenanceEvidence: &FormationAttestation{
				Verified: true,
				Digest:   modelDigest,
				Window:   window,
			},
		}},
	}
	require.NoError(t, witnessReader.PersistFormationWitnessEvidence(context.Background(), formationResult))

	loaded, err := provenanceReader.LoadProvenanceWindow(context.Background(), "attempt-lite")
	require.NoError(t, err)
	assert.Equal(t, window.GetAttestationDigest(), loaded.GetAttestationDigest())
}

func TestVerifyFormationWitnessEvidence_AcceptsMatchingProvenanceDigest(t *testing.T) {
	files := newCampaignMemoryFileService()
	provenanceReader, err := NewCampaignModelProvenanceReader(files)
	require.NoError(t, err)
	modelDigest := strings.Repeat("a", 64)
	window := testFormationProvenanceWindow(t, "attempt-lite", modelDigest)
	require.NoError(t, provenanceReader.ImportProvenanceWindow(context.Background(), window))

	evidence := &FormationRunEvidence{
		Result: persistedFormationRunResult{
			Roles: []persistedFormationRoleTelemetry{{
				Role:                        string(FormationRoleLite),
				ProviderAttemptID:           "attempt-lite",
				ProvenanceAttestationDigest: window.GetAttestationDigest(),
				AttestationStatus:           string(FormationAttestationVerified),
				ModelDigest:                 modelDigest,
			}},
		},
	}
	failures := VerifyFormationWitnessEvidence(
		context.Background(),
		evidence,
		nil,
		provenanceReader,
		ProviderObservationPolicyInterim,
		ModelProvenancePolicyStrict,
	)
	assert.Empty(t, failures)
}

func TestBindFormationProvenanceEvidence_RekeysProviderAttemptID(t *testing.T) {
	modelDigest := strings.Repeat("a", 64)
	window := testFormationProvenanceWindow(t, "preflight-provenance", modelDigest)
	attestation := &FormationAttestation{Verified: true, Digest: modelDigest, Window: window}

	bound := bindFormationProvenanceEvidence(attestation, "attempt-lite")
	require.NotNil(t, bound)
	require.NotNil(t, bound.Window)
	assert.Equal(t, "attempt-lite", bound.Window.GetProviderAttemptId())
	assert.NotEqual(t, window.GetAttestationDigest(), bound.Window.GetAttestationDigest())
}

func TestVerifyFormationWitnessEvidence_FailsOnObserverDigestMismatch(t *testing.T) {
	files := newCampaignMemoryFileService()
	observationReader, err := NewCampaignProviderObservationReader(files)
	require.NoError(t, err)
	window := testFormationObserverWindow(t, "attempt-lite")
	require.NoError(t, observationReader.ImportObservationWindow(context.Background(), window))

	evidence := &FormationRunEvidence{
		Result: persistedFormationRunResult{
			Roles: []persistedFormationRoleTelemetry{{
				Role:                      string(FormationRoleLite),
				ProviderAttemptID:         "attempt-lite",
				ObserverObservationDigest: "digest-other",
			}},
		},
	}
	failures := VerifyFormationWitnessEvidence(
		context.Background(),
		evidence,
		observationReader,
		nil,
		ProviderObservationPolicyStrict,
		ModelProvenancePolicyInterim,
	)
	require.NotEmpty(t, failures)
	assert.Contains(t, failures[0], "observer digest mismatch")
}

func TestBuildFormationRunEvidence_PersistsWitnessDigests(t *testing.T) {
	req := heterogeneousAssignmentExecutionRequest(t, mustHeterogeneousStack(t), testHeterogeneousVariants())
	runContext := FormationRunContext{
		CampaignID:          req.Assignment.GetCampaignId(),
		RunID:               req.Assignment.GetRunId(),
		AssignmentID:        req.Assignment.GetAssignmentId(),
		EvaluationAttemptID: req.AttemptID,
		ScenarioID:          req.Assignment.GetScenarioId(),
		ModelRegistryDigest: req.Binding.ModelRegistryDigest,
		InferenceSessionID:  req.Binding.InferenceOperatorSessionID,
		DataSessionID:       req.Binding.DataOperatorSessionID,
	}
	harness, err := NewFormationHarness(
		func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		func(prefix string) string { return prefix + "-attempt" },
	)
	require.NoError(t, err)
	formation, err := BindHeterogeneousStack(FormationBindingRequest{Stack: req.Assignment.GetHeterogeneous().GetStack(), Variants: testHeterogeneousVariants()})
	require.NoError(t, err)
	initialState, err := BuildFormationInitialState(req.ScenarioInput)
	require.NoError(t, err)
	formationResult, err := harness.RunBoundFormation(context.Background(), formation, initialState)
	require.NoError(t, err)

	_, evidence, err := BuildFormationRunEvidence(req, runContext, formationResult)
	require.NoError(t, err)
	require.Len(t, evidence.Result.Roles, 3)
	for _, role := range evidence.Result.Roles {
		assert.NotEmpty(t, role.ObserverObservationDigest)
	}
}

func TestCollectRunAggregateState_AcceptsHeterogeneousAssignments(t *testing.T) {
	stack := mustHeterogeneousStack(t)
	assignment := &evalv1.EvaluationAssignment{
		AssignmentId: "assignment-hetero",
		RunId:        "run-hetero",
		ScenarioId:   "scenario-1",
		Target: &evalv1.EvaluationAssignment_Heterogeneous{
			Heterogeneous: &evalv1.HeterogeneousAssignmentTarget{Stack: stack},
		},
	}
	state, err := CollectRunAggregateState([]*evalv1.EvaluationAssignment{assignment}, map[string]*evalv1.EvaluationAssignmentResult{})
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, uint32(1), state.Scheduled)
}
