// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type PublicScenarioContext struct {
	CampaignID          string
	RunID               string
	ScenarioID          string
	ScenarioVersion     string
	Category            evalv1.EvaluationScenarioCategory
	PublicDescription   string
	GradingMethod       evalv1.EvaluationGradingMethod
	AllowedTools        []string
	ExpectedTools       []string
	ForbiddenTools      []string
	Criteria            []*evalv1.PublicScenarioCriterion
	ToolScoreDimensions []*evalv1.PublicToolScoreDimensionRequirement
	CatalogRef          *compliancev1.VersionedReference
	CatalogDigest       string
	ScenarioReference   *compliancev1.ComplianceEvidenceReference
}

type PublicGradeSummarySet struct {
	Deterministic []PublicGradeSummary
	Semantic      []*evalv1.PublicSemanticGradeSummary
}

type PublicAssignmentEvidenceInput struct {
	Result             *evalv1.EvaluationAssignmentResult
	Activity           *PublicAssignmentActivity
	PublicProofs       []*evalv1.PublicEvidenceBinding
	EvidenceReferences []*compliancev1.ComplianceEvidenceReference
}

type PublicAssignmentActivity struct {
	Summary *evalv1.PublicAssignmentActivitySummary
}

type PublicResourceMetric struct {
	Value             *float64                       `json:"value,omitempty"`
	UnavailableReason evalv1.PublicUnavailableReason `json:"unavailable_reason,omitempty"`
}

type PublicResourceSummary struct {
	LatencyMS      PublicResourceMetric `json:"latency_ms,omitempty"`
	InputTokens    PublicResourceMetric `json:"input_tokens,omitempty"`
	OutputTokens   PublicResourceMetric `json:"output_tokens,omitempty"`
	ThinkingTokens PublicResourceMetric `json:"thinking_tokens,omitempty"`
	CacheTokens    PublicResourceMetric `json:"cache_tokens,omitempty"`
	Retries        PublicResourceMetric `json:"retries,omitempty"`
}

type PublicAssignmentRecordExtensions struct {
	BenchmarkObservations *PublicBenchmarkObservations `json:"benchmark_observations,omitempty"`
	ResourceSummary       *PublicResourceSummary       `json:"resource_summary,omitempty"`
}

type PublicAssignmentBuildInput struct {
	Assignment           *evalv1.EvaluationAssignment
	Result               *evalv1.EvaluationAssignmentResult
	ScenarioContext      *PublicScenarioContext
	GradeSummaries       *PublicGradeSummarySet
	Activity             *PublicAssignmentActivity
	EvidenceBindings     []*evalv1.PublicEvidenceBinding
	VerificationMetadata *evalv1.PublicVerificationMetadata
	VerificationStatus   string
	ObservationReader    *CampaignProviderObservationReader
	Extensions           PublicAssignmentRecordExtensions
}

type PublicAssignmentRecord struct {
	Projection *evalv1.PublicAssignmentResultProjection
	Extensions PublicAssignmentRecordExtensions
}

type VerifiedModelSummaryBucket struct {
	VariantID     string
	Role          evalv1.ModelCampaignRole
	AssignmentIDs []string
}

type RunVerificationApplicability struct {
	Applicable              bool
	RunID                   string
	CampaignID              string
	ExpectedAssignmentCount uint32
	VerifiedAssignmentCount uint32
	Population              *evalv1.EvaluationVerifiedPopulation
	EligibleModelBuckets    []VerifiedModelSummaryBucket
	UnavailableReason       evalv1.PublicUnavailableReason
}

type VerifiedModelSummaryProjectionInput struct {
	Run           *evalv1.EvaluationRun
	Spec          *evalv1.EvaluationCampaignSpec
	Catalog       *evalv1.EvaluationScenarioCatalog
	Assignments   []*evalv1.EvaluationAssignment
	Results       map[string]*evalv1.EvaluationAssignmentResult
	Report        *evalv1.EvaluationVerificationReport
	Applicability *RunVerificationApplicability
}
