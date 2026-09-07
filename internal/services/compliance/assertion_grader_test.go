// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliance_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

func assertionGraderCatalog(missingPolicy string) *compliancev1.ControlAssertionCatalog {
	return &compliancev1.ControlAssertionCatalog{
		CatalogId:      "assertion-grader-test",
		CatalogVersion: "1.0.0",
		Sha256:         "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Assertions: []*compliancev1.ControlAssertionDefinition{{
			AssertionId:             "G8E-GOV-BLOCK-001",
			AssertionVersion:        "1.0.0",
			Title:                   "Governed block outcome",
			Statement:               "A prohibited governed action is rejected.",
			Category:                "governance",
			ComponentScope:          []string{"gateway"},
			Responsibility:          "platform",
			ApplicableActionClasses: []string{"governed_mutation"},
			ApplicableArms:          []string{"governed"},
			RequiredEvidenceTypes:   []string{"action_receipt"},
			RequiredGraderRefs:      []*compliancev1.VersionedReference{{Id: "policy_outcome", Version: "1.0.0"}},
			RequiredVerifierRefs:    []*compliancev1.VersionedReference{{Id: "receipt_integrity", Version: "1.0.0"}},
			MinimumEvidenceLevel:    "L3",
			ValidationCycle:         "7d",
			MissingEvidencePolicy:   missingPolicy,
			PassingRule:             "all_required",
		}},
	}
}

func assertionGraderNode(artifactType evidence.ArtifactType, producer string, body []byte, producedAt time.Time) evidence.EvidenceNode {
	digest := sha256.Sum256(body)
	return evidence.EvidenceNode{
		ArtifactID:         evidence.ContentAddress(artifactType, body),
		ArtifactType:       artifactType,
		SHA256:             hex.EncodeToString(digest[:]),
		MediaType:          constants.MediaTypeJSON,
		SchemaRef:          "test",
		ProducerIdentity:   producer,
		ProducedAt:         producedAt,
		ScopeID:            "scope-1",
		RunID:              "run-1",
		VerificationStatus: evidence.VerificationStatusVerified,
		VerifierID:         "test-verifier",
		VerifierVersion:    "1.0.0",
		VerifiedAt:         producedAt,
		CanonicalBytes:     body,
		References:         []string{},
	}
}

func assertionGraderGraph(t *testing.T, now time.Time, metricValue int) *evidence.EvidenceGraph {
	t.Helper()
	graph := evidence.NewEvidenceGraph(0, nil)
	receipt := assertionGraderNode(evidence.ArtifactTypeActionReceipt, "gateway", []byte(`{"receipt":"verified"}`), now.Add(-time.Hour))
	metric := assertionGraderNode(evidence.ArtifactTypeEvalMetric, "policy_outcome@1.0.0", []byte(`{"metric_id":"policy_outcome","metric_version":"1.0.0","value":`+strconv.Itoa(metricValue)+`,"eligible":true,"verification_status":"verified"}`), now.Add(-time.Hour))
	require.NoError(t, graph.AddNode(receipt))
	require.NoError(t, graph.AddNode(metric))
	return graph
}

func gradeAssertionTestGraph(t *testing.T, graph *evidence.EvidenceGraph, assertions *compliancev1.ControlAssertionCatalog, now time.Time) *compliancev1.ControlAssertionAssessment {
	t.Helper()
	assessments, err := evidence.GradeControlAssertions(context.Background(), evidence.AssertionGradingRequest{
		ScopeID:     "scope-1",
		WindowStart: now.Add(-24 * time.Hour),
		WindowEnd:   now,
		EvaluatedAt: now,
		Assertions:  assertions,
		Graph:       graph,
	})
	require.NoError(t, err)
	require.Len(t, assessments, 1)
	return assessments[0]
}

func TestGradeControlAssertions_SatisfiesAllRequiredVerifiedEvidence(t *testing.T) {
	now := time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC)
	assertions := assertionGraderCatalog("unverifiable")
	assessment := gradeAssertionTestGraph(t, assertionGraderGraph(t, now, 1), assertions, now)

	assert.Equal(t, "satisfied", assessment.GetStatus())
	assert.Equal(t, "fresh", assessment.GetFreshnessStatus())
	assert.Equal(t, "L3", assessment.GetEvidenceLevel())
	assert.Empty(t, assessment.GetFailureReason())
	assert.Len(t, assessment.GetEvidenceRefs(), 2)
	assert.Len(t, assessment.GetMetricRefs(), 1)
	assert.Equal(t, constants.AssertionGraderID, assessment.GetVerifierRef().GetId())
	assert.Equal(t, constants.AssertionGraderVersion, assessment.GetVerifierRef().GetVersion())
	assert.NoError(t, catalog.ValidateAssertionAssessment(assessment, "scope-1", assertions))
}

func TestGradeControlAssertions_FailsWhenRequiredGraderReportsFailure(t *testing.T) {
	now := time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC)
	assessment := gradeAssertionTestGraph(t, assertionGraderGraph(t, now, 0), assertionGraderCatalog("unverifiable"), now)

	assert.Equal(t, "not_satisfied", assessment.GetStatus())
	assert.Equal(t, "fresh", assessment.GetFreshnessStatus())
	assert.NotEmpty(t, assessment.GetFailureReason())
}

func TestGradeControlAssertions_IgnoresUnverifiedEvidence(t *testing.T) {
	now := time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC)
	graph := assertionGraderGraph(t, now, 1)
	for _, node := range graph.NodesByScope("scope-1") {
		node.VerificationStatus = evidence.VerificationStatusUnverified
	}
	assessment := gradeAssertionTestGraph(t, graph, assertionGraderCatalog("unverifiable"), now)

	assert.Equal(t, "unverifiable", assessment.GetStatus())
	assert.Equal(t, "incomplete", assessment.GetFreshnessStatus())
	assert.Empty(t, assessment.GetEvidenceRefs())
	assert.Empty(t, assessment.GetMetricRefs())
}

func TestGradeControlAssertions_IgnoresEvidenceFromAnotherScope(t *testing.T) {
	now := time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC)
	graph := evidence.NewEvidenceGraph(0, nil)
	receipt := assertionGraderNode(evidence.ArtifactTypeActionReceipt, "gateway", []byte(`{"receipt":"verified"}`), now.Add(-time.Hour))
	metric := assertionGraderNode(evidence.ArtifactTypeEvalMetric, "policy_outcome@1.0.0", []byte(`{"metric_id":"policy_outcome","metric_version":"1.0.0","value":1,"eligible":true,"verification_status":"verified"}`), now.Add(-time.Hour))
	receipt.ScopeID = "scope-2"
	metric.ScopeID = "scope-2"
	require.NoError(t, graph.AddNode(receipt))
	require.NoError(t, graph.AddNode(metric))

	assessment := gradeAssertionTestGraph(t, graph, assertionGraderCatalog("unverifiable"), now)
	assert.Equal(t, "unverifiable", assessment.GetStatus())
	assert.Empty(t, assessment.GetEvidenceRefs())
}

func TestGradeControlAssertions_ReportsStaleRequiredEvidence(t *testing.T) {
	now := time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC)
	graph := assertionGraderGraph(t, now, 1)
	for _, node := range graph.NodesByScope("scope-1") {
		node.ProducedAt = now.Add(-48 * time.Hour)
	}
	assessment := gradeAssertionTestGraph(t, graph, assertionGraderCatalog("unverifiable"), now)

	assert.Equal(t, "unverifiable", assessment.GetStatus())
	assert.Equal(t, "stale", assessment.GetFreshnessStatus())
	assert.NotEmpty(t, assessment.GetFailureReason())
	assert.Len(t, assessment.GetEvidenceRefs(), 2)
	assert.Len(t, assessment.GetMetricRefs(), 1)
}

func TestGradeControlAssertions_AppliesMissingEvidencePolicy(t *testing.T) {
	now := time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		policy    string
		status    string
		freshness string
	}{
		{policy: "unverifiable", status: "unverifiable", freshness: "incomplete"},
		{policy: "customer_attestation_required", status: "customer_attestation_required", freshness: "incomplete"},
		{policy: "not_applicable", status: "not_applicable", freshness: "not_applicable"},
	}
	for _, tt := range tests {
		t.Run(tt.policy, func(t *testing.T) {
			assessment := gradeAssertionTestGraph(t, evidence.NewEvidenceGraph(0, nil), assertionGraderCatalog(tt.policy), now)
			assert.Equal(t, tt.status, assessment.GetStatus())
			assert.Equal(t, tt.freshness, assessment.GetFreshnessStatus())
			assert.Equal(t, "L0", assessment.GetEvidenceLevel())
		})
	}
}

func TestGradeControlAssertions_AppliesAssertionValidationCycle(t *testing.T) {
	now := time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC)
	graph := assertionGraderGraph(t, now, 1)
	for _, node := range graph.NodesByScope("scope-1") {
		node.ProducedAt = now.Add(-8 * 24 * time.Hour)
	}

	assessments, err := evidence.GradeControlAssertions(context.Background(), evidence.AssertionGradingRequest{ScopeID: "scope-1", WindowStart: now.Add(-90 * 24 * time.Hour), WindowEnd: now, EvaluatedAt: now, Assertions: assertionGraderCatalog("unverifiable"), Graph: graph})
	require.NoError(t, err)
	require.Len(t, assessments, 1)
	assert.Equal(t, "unverifiable", assessments[0].GetStatus())
	assert.Equal(t, "stale", assessments[0].GetFreshnessStatus())
}

func TestGradeControlAssertions_RejectsIncompleteRequests(t *testing.T) {
	now := time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC)
	valid := evidence.AssertionGradingRequest{ScopeID: "scope-1", WindowStart: now.Add(-time.Hour), WindowEnd: now, EvaluatedAt: now, Assertions: assertionGraderCatalog("unverifiable"), Graph: evidence.NewEvidenceGraph(0, nil)}
	tests := []struct {
		name   string
		mutate func(*evidence.AssertionGradingRequest)
	}{
		{name: "missing scope", mutate: func(request *evidence.AssertionGradingRequest) { request.ScopeID = "" }},
		{name: "missing graph", mutate: func(request *evidence.AssertionGradingRequest) { request.Graph = nil }},
		{name: "missing catalog", mutate: func(request *evidence.AssertionGradingRequest) { request.Assertions = nil }},
		{name: "inverted window", mutate: func(request *evidence.AssertionGradingRequest) { request.WindowStart = now.Add(time.Hour) }},
		{name: "evaluation outside window", mutate: func(request *evidence.AssertionGradingRequest) { request.EvaluatedAt = now.Add(time.Hour) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := valid
			tt.mutate(&request)
			_, err := evidence.GradeControlAssertions(context.Background(), request)
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
		})
	}
}

func TestGradeControlAssertions_StopsOnCancellation(t *testing.T) {
	now := time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := evidence.GradeControlAssertions(ctx, evidence.AssertionGradingRequest{ScopeID: "scope-1", WindowStart: now.Add(-time.Hour), WindowEnd: now, EvaluatedAt: now, Assertions: assertionGraderCatalog("unverifiable"), Graph: assertionGraderGraph(t, now, 1)})
	assert.ErrorIs(t, err, context.Canceled)
}

func TestGradeControlAssertions_RejectsInvalidGraph(t *testing.T) {
	now := time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC)
	graph := assertionGraderGraph(t, now, 1)
	graph.NodesByScope("scope-1")[0].References = []string{"missing"}
	graph.ResolveReferences()

	_, err := evidence.GradeControlAssertions(context.Background(), evidence.AssertionGradingRequest{ScopeID: "scope-1", WindowStart: now.Add(-time.Hour), WindowEnd: now, EvaluatedAt: now, Assertions: assertionGraderCatalog("unverifiable"), Graph: graph})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

func TestGradeControlAssertions_EmitsAssessmentForEveryCanonicalAssertion(t *testing.T) {
	assertions, _, _, err := catalog.LoadCanonicalCatalogs()
	require.NoError(t, err)
	now := time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC)

	assessments, err := evidence.GradeControlAssertions(context.Background(), evidence.AssertionGradingRequest{ScopeID: "scope-1", WindowStart: now.Add(-24 * time.Hour), WindowEnd: now, EvaluatedAt: now, Assertions: assertions, Graph: evidence.NewEvidenceGraph(0, nil)})
	require.NoError(t, err)
	assert.Len(t, assessments, len(assertions.GetAssertions()))
	for _, assessment := range assessments {
		assert.NotEqual(t, "satisfied", assessment.GetStatus())
		assert.NoError(t, catalog.ValidateAssertionAssessment(assessment, "scope-1", assertions))
	}
}
