// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZeroSHA256(t *testing.T) {
	assert.Equal(t, hex.EncodeToString(make([]byte, sha256.Size)), zeroSHA256())
}

func TestCampaignWitnessPolicyHelpers(t *testing.T) {
	assert.Equal(t, evaluation.ProviderObservationPolicyStrict, campaignProviderPolicy(evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_STRICT))
	assert.Equal(t, evaluation.ProviderObservationPolicyInterim, campaignProviderPolicy(evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_INTERIM))
	assert.Equal(t, evaluation.ModelProvenancePolicyStrict, campaignProvenancePolicy(evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_STRICT))
	assert.Equal(t, evaluation.ModelProvenancePolicyInterim, campaignProvenancePolicy(evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_INTERIM))

	assert.Equal(t, evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_STRICT, campaignWitnessPolicyFromAdmission(compliancev1.AssessmentWitnessPolicy_ASSESSMENT_WITNESS_POLICY_STRICT))
	assert.Equal(t, evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_INTERIM, campaignWitnessPolicyFromAdmission(compliancev1.AssessmentWitnessPolicy_ASSESSMENT_WITNESS_POLICY_INTERIM))
	assert.Equal(t, evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_UNSPECIFIED, campaignWitnessPolicyFromAdmission(compliancev1.AssessmentWitnessPolicy_ASSESSMENT_WITNESS_POLICY_UNSPECIFIED))
}

func TestLastKSIHistoryResultBody(t *testing.T) {
	body, err := lastKSIHistoryResultBody([]byte("first\nsecond\n"))
	require.NoError(t, err)
	assert.Equal(t, []byte("second"), body)

	_, err = lastKSIHistoryResultBody(nil)
	assert.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
}

func TestFirstKSIHistoryResultSet(t *testing.T) {
	resultSet := &compliance.KSIResultSet{Class: compliance.ClassA, EvaluatedAtMs: 1}
	line, err := json.Marshal(resultSet)
	require.NoError(t, err)

	decoded, err := firstKSIHistoryResultSet(append(line, '\n', '\n'))
	require.NoError(t, err)
	assert.Equal(t, resultSet.Class, decoded.Class)

	_, err = firstKSIHistoryResultSet([]byte("not-json"))
	assert.Error(t, err)

	_, err = firstKSIHistoryResultSet(nil)
	assert.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
}

func TestStableVerificationFailureCode(t *testing.T) {
	assert.ErrorIs(t, stableVerificationFailureCode(constants.ErrChecksumMismatch), constants.ErrChecksumMismatch)
	assert.ErrorIs(t, stableVerificationFailureCode(fmt.Errorf("wrapped: %w", constants.ErrRendererMismatch)), constants.ErrRendererMismatch)
	assert.ErrorIs(t, stableVerificationFailureCode(errors.New("unknown failure")), constants.ErrReportVerificationFailed)
}

func TestReplayedNodeMatchesNilInputs(t *testing.T) {
	assert.False(t, replayedNodeMatches(nil, &evidence.EvidenceNode{}))
	assert.False(t, replayedNodeMatches(&compliancev1.ComplianceEvidenceReference{}, nil))
}

func TestProtectedDecisionImporterSourceID(t *testing.T) {
	importer := protectedDecisionImporter{admissionID: "protected-source"}
	assert.Equal(t, "protected-source", importer.SourceID())
}

func TestFrameworkCatalogForManifest(t *testing.T) {
	catalog := frameworkCatalogForManifest(&compliancev1.ComplianceReportManifest{
		FrameworkRefs: []*compliancev1.VersionedReference{
			{Id: "fedramp", Version: "rev5"},
			nil,
		},
	})
	require.Len(t, catalog.GetFrameworks(), 1)
	assert.Equal(t, "fedramp", catalog.GetFrameworks()[0].GetFrameworkId())
	assert.NotNil(t, frameworkCatalogForManifest(nil))
}
