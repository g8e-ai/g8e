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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/model_provenance"
	"github.com/g8e-ai/g8e/v2/internal/services/storage/storagetest"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func testModelProvenanceWindow(t *testing.T, providerAttemptID string) *evalv1.ModelProvenanceAttestationWindow {
	t.Helper()
	digest := strings.Repeat("a", 64)
	window := &evalv1.ModelProvenanceAttestationWindow{
		SchemaVersion:              model_provenance.SchemaVersion,
		ProviderAttemptId:          providerAttemptID,
		ProvenanceOperatorId:       "test-provenance-operator",
		ServedModelTag:             "probe-model:7b",
		ExpectedModelDigest:        digest,
		ObservedModelDigest:        digest,
		ManifestDigest:             strings.Repeat("b", 64),
		ManifestVerificationStatus: evalv1.ModelManifestVerificationStatus_MODEL_MANIFEST_VERIFICATION_STATUS_UNSIGNED,
		AttestedAtUnixMs:           1_700_000_000_000,
		DigestMatch:                true,
	}
	attestationDigest, err := model_provenance.ComputeAttestationDigest(window)
	require.NoError(t, err)
	window.AttestationDigest = attestationDigest
	return window
}

func TestVerifyModelProvenanceWindow_InterimPolicy_AllowsMissingWindow(t *testing.T) {
	t.Parallel()
	err := VerifyModelProvenanceWindow(nil, strings.Repeat("a", 64), ModelProvenancePolicyInterim)
	assert.NoError(t, err)
}

func TestVerifyModelProvenanceWindow_StrictPolicy_RequiresWindow(t *testing.T) {
	t.Parallel()
	err := VerifyModelProvenanceWindow(nil, strings.Repeat("a", 64), ModelProvenancePolicyStrict)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationObservationUnavailable)
}

func TestVerifyModelProvenanceWindow_StrictPolicy_RequiresDigestMatch(t *testing.T) {
	t.Parallel()
	window := testModelProvenanceWindow(t, "attempt-1")
	window.DigestMatch = false
	attestationDigest, err := model_provenance.ComputeAttestationDigest(window)
	require.NoError(t, err)
	window.AttestationDigest = attestationDigest

	err = VerifyModelProvenanceWindow(window, window.GetExpectedModelDigest(), ModelProvenancePolicyStrict)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrModelProvenanceDigestMismatch)
}

func TestVerifyModelProvenanceWindow_ExpectedDigestMismatch(t *testing.T) {
	t.Parallel()
	window := testModelProvenanceWindow(t, "attempt-1")
	err := VerifyModelProvenanceWindow(window, strings.Repeat("c", 64), ModelProvenancePolicyInterim)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected digest mismatch")
}

func TestLocalModelProvenanceReader_LoadRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	store, err := model_provenance.NewWindowStore(fileSvc)
	require.NoError(t, err)
	window := testModelProvenanceWindow(t, "attempt-1")
	require.NoError(t, store.Save(ctx, window))

	reader, err := NewLocalModelProvenanceReader(fileSvc)
	require.NoError(t, err)
	loaded, err := reader.Load(ctx, "attempt-1")
	require.NoError(t, err)
	assert.Equal(t, window.GetAttestationDigest(), loaded.GetAttestationDigest())
}

type stubModelProvenanceRemote struct {
	window *evalv1.ModelProvenanceAttestationWindow
}

func (s *stubModelProvenanceRemote) Load(_ context.Context, providerAttemptID string) (*evalv1.ModelProvenanceAttestationWindow, error) {
	if s == nil || s.window == nil || s.window.GetProviderAttemptId() != providerAttemptID {
		return nil, constants.ErrNotFound
	}
	return s.window, nil
}

func TestCampaignModelProvenanceReader_CaptureAssignmentEvidencePersistsGatewayEvidence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	window := testModelProvenanceWindow(t, "attempt-captured")
	reader, err := NewCampaignModelProvenanceReaderWithRemote(fileSvc, &stubModelProvenanceRemote{window: window})
	require.NoError(t, err)
	result := &evalv1.EvaluationAssignmentResult{ModelInferences: []*evalv1.ModelInferenceRecord{{ProviderAttemptId: window.GetProviderAttemptId(), ModelVariant: &evalv1.ModelVariant{ModelDigest: window.GetExpectedModelDigest()}}}}

	require.NoError(t, reader.CaptureAssignmentEvidence(ctx, result))
	localReader, err := NewCampaignModelProvenanceReader(fileSvc)
	require.NoError(t, err)
	failures, unavailable := localReader.VerifyAssignmentModelProvenance(ctx, result, ModelProvenancePolicyStrict)
	assert.Empty(t, failures)
	assert.Empty(t, unavailable)
}

func TestCampaignModelProvenanceReader_VerifyAssignmentModelProvenance_StrictMissingWindow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	reader, err := NewCampaignModelProvenanceReader(fileSvc)
	require.NoError(t, err)
	result := &evalv1.EvaluationAssignmentResult{
		ModelInferences: []*evalv1.ModelInferenceRecord{{
			InferenceRecordId: "inf-1",
			ProviderAttemptId: "attempt-missing",
			ModelVariant:      &evalv1.ModelVariant{ModelDigest: strings.Repeat("a", 64)},
		}},
	}
	failures, unavailable := reader.VerifyAssignmentModelProvenance(ctx, result, ModelProvenancePolicyStrict)
	require.NotEmpty(t, failures)
	require.NotEmpty(t, unavailable)
}

func TestCampaignModelProvenanceReader_VerifyAssignmentModelProvenance_StrictDigestMatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	store, err := model_provenance.NewWindowStore(fileSvc)
	require.NoError(t, err)
	window := testModelProvenanceWindow(t, "attempt-1")
	require.NoError(t, store.Save(ctx, window))
	reader, err := NewCampaignModelProvenanceReader(fileSvc)
	require.NoError(t, err)
	result := &evalv1.EvaluationAssignmentResult{
		ModelInferences: []*evalv1.ModelInferenceRecord{{
			InferenceRecordId: "inf-1",
			ProviderAttemptId: "attempt-1",
			ModelVariant:      &evalv1.ModelVariant{ModelDigest: window.GetExpectedModelDigest()},
		}},
	}
	failures, unavailable := reader.VerifyAssignmentModelProvenance(ctx, result, ModelProvenancePolicyStrict)
	assert.Empty(t, failures)
	assert.Empty(t, unavailable)
}

func TestLocalModelProvenanceReader_RejectsMissingProviderAttemptID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	reader, err := NewLocalModelProvenanceReader(fileSvc)
	require.NoError(t, err)
	_, err = reader.Load(ctx, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
}
