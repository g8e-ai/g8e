// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version  2.0.

package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

const (
	completenessStatusComplete     = "complete"
	completenessStatusPartial      = "partial"
	completenessStatusEmpty        = "empty"
	gapTypeNotSatisfied            = "not_satisfied"
	gapTypeUnverifiable            = "unverifiable"
	gapTypeCustomerAttestation     = "customer_attestation_required"
	findingSeverityHigh            = "high"
	findingSeverityMedium          = "medium"
	findingSeverityLow             = "low"
	remediationPriorityHigh        = "high"
	remediationPriorityMedium      = "medium"
	remediationPriorityLow         = "low"
	remediationOwnerPlatform       = "platform"
	remediationOwnerCustomer       = "customer"
	sectionResponsibilityPlatform  = "platform"
	sectionResponsibilityCustomer  = "customer"
	sectionResponsibilityShared    = "shared"
	sectionResponsibilityInherited = "inherited"
	evidenceLinkTypeReferences     = "references"
)

// AnalysisRequest carries the verified evidence and catalog inputs from
// which BuildComplianceAnalysis constructs a canonical ComplianceAnalysis.
// The assertion and framework assessments must already be produced by
// GradeControlAssertions and GradeFrameworkControls respectively; the
// builder aggregates them rather than re-grading.
type AnalysisRequest struct {
	ScopeID              string
	WindowStart          time.Time
	WindowEnd            time.Time
	EvaluatedAt          time.Time
	Graph                *EvidenceGraph
	Assertions           *compliancev1.ControlAssertionCatalog
	Frameworks           *compliancev1.FrameworkCatalog
	Crosswalks           *compliancev1.ControlCrosswalkCatalog
	AssertionAssessments []*compliancev1.ControlAssertionAssessment
	FrameworkAssessments []*compliancev1.FrameworkControlAssessment
}

// BuildComplianceAnalysis aggregates verified evidence, assertion
// assessments, and framework assessments into a canonical
// ComplianceAnalysis. The builder is read-only: it consumes the evidence
// graph and pre-graded assessments without mutating them. It computes
// evidence-window completeness, evidence links, gaps, limitations,
// findings, remediation, and sections from the supplied inputs and
// produces a deterministic content-addressed analysis identity.
func BuildComplianceAnalysis(ctx context.Context, request AnalysisRequest) (*compliancev1.ComplianceAnalysis, error) {
	if err := validateAnalysisRequest(request); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	assertionIndex := indexAnalysisAssertions(request.Assertions)
	completeness := buildEvidenceWindowCompleteness(request)
	evidenceLinks := buildEvidenceLinks(request)
	gaps := buildGaps(request, assertionIndex)
	limitations := buildLimitations(request)
	findings := buildFindings(request, assertionIndex)
	remediation := buildRemediation(findings)
	sections := buildSections(request)
	graphFailures := buildGraphFailureMessages(request.Graph)

	analysis := &compliancev1.ComplianceAnalysis{
		AnalysisId:                 analysisID(request),
		AnalysisSchemaVersion:      constants.AnalysisSchemaVersion,
		ScopeRef:                   request.ScopeID,
		GeneratedAt:                timestamppb.New(request.EvaluatedAt),
		GeneratorIdentity:          constants.AnalysisBuilderID,
		GeneratorVersion:           constants.AnalysisBuilderVersion,
		EvidenceWindowCompleteness: completeness,
		AssertionAssessments:       sortedAssertionAssessments(request.AssertionAssessments),
		FrameworkAssessments:       sortedFrameworkAssessments(request.FrameworkAssessments),
		Gaps:                       gaps,
		EvidenceLinks:              evidenceLinks,
		Limitations:                limitations,
		Findings:                   findings,
		Remediation:                remediation,
		Sections:                   sections,
		EvidenceGraphFailures:      graphFailures,
		EvidenceGraphValid:         request.Graph.Valid(),
	}
	return analysis, nil
}

func validateAnalysisRequest(request AnalysisRequest) error {
	if request.ScopeID == "" || request.Graph == nil || request.Assertions == nil || request.Frameworks == nil || request.Crosswalks == nil {
		return fmt.Errorf("%w: analysis request is incomplete", constants.ErrInvalidEvidenceGraph)
	}
	if request.WindowStart.IsZero() || request.WindowEnd.IsZero() || request.EvaluatedAt.IsZero() {
		return fmt.Errorf("%w: analysis request timestamps are missing", constants.ErrInvalidEvidenceGraph)
	}
	if request.WindowEnd.Before(request.WindowStart) || request.EvaluatedAt.Before(request.WindowStart) || request.EvaluatedAt.After(request.WindowEnd) {
		return fmt.Errorf("%w: analysis request evidence window is inverted", constants.ErrInvalidEvidenceGraph)
	}
	if err := catalog.ValidateAssertionCatalog(request.Assertions); err != nil {
		return fmt.Errorf("analysis assertion catalog: %w", err)
	}
	if err := catalog.ValidateFrameworkCatalog(request.Frameworks); err != nil {
		return fmt.Errorf("analysis framework catalog: %w", err)
	}
	for _, assessment := range request.AssertionAssessments {
		if assessment == nil || assessment.AssertionRef == nil {
			return fmt.Errorf("%w: assertion assessment is incomplete", constants.ErrInvalidEvidenceGraph)
		}
		if assessment.ScopeId != request.ScopeID {
			return fmt.Errorf("%w: assertion assessment %s belongs to scope %s", constants.ErrEvidenceScopeMismatch, assessment.AssessmentId, assessment.ScopeId)
		}
	}
	for _, assessment := range request.FrameworkAssessments {
		if assessment == nil || assessment.FrameworkRef == nil {
			return fmt.Errorf("%w: framework assessment is incomplete", constants.ErrInvalidEvidenceGraph)
		}
		if assessment.ScopeId != request.ScopeID {
			return fmt.Errorf("%w: framework assessment %s belongs to scope %s", constants.ErrEvidenceScopeMismatch, assessment.AssessmentId, assessment.ScopeId)
		}
	}
	return nil
}

func indexAnalysisAssertions(assertions *compliancev1.ControlAssertionCatalog) map[string]*compliancev1.ControlAssertionDefinition {
	index := make(map[string]*compliancev1.ControlAssertionDefinition, len(assertions.GetAssertions()))
	for _, assertion := range assertions.GetAssertions() {
		if assertion == nil {
			continue
		}
		index[versionedReferenceKey(assertion.GetAssertionId(), assertion.GetAssertionVersion())] = assertion
	}
	return index
}

func buildEvidenceWindowCompleteness(request AnalysisRequest) *compliancev1.EvidenceWindowCompleteness {
	expected := int32(len(request.AssertionAssessments))
	satisfied := int32(0)
	missing := make([]string, 0)
	stale := make([]string, 0)
	for _, assessment := range request.AssertionAssessments {
		switch assessment.GetStatus() {
		case statusSatisfied:
			satisfied++
		}
		switch assessment.GetFreshnessStatus() {
		case freshnessIncomplete:
			missing = append(missing, assessment.GetAssessmentId())
		case freshnessStale:
			stale = append(stale, assessment.GetAssessmentId())
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	status := completenessStatusEmpty
	if satisfied > 0 && satisfied == expected {
		status = completenessStatusComplete
	} else if satisfied > 0 {
		status = completenessStatusPartial
	}
	return &compliancev1.EvidenceWindowCompleteness{
		ScopeId:               request.ScopeID,
		ExpectedEvidenceCount: expected,
		ActualEvidenceCount:   satisfied,
		MissingEvidenceRefs:   missing,
		StaleEvidenceRefs:     stale,
		CompletenessStatus:    status,
	}
}

func buildEvidenceLinks(request AnalysisRequest) []*compliancev1.EvidenceLink {
	links := make([]*compliancev1.EvidenceLink, 0)
	for _, node := range request.Graph.NodesByScope(request.ScopeID) {
		for _, ref := range node.References {
			if ref == "" {
				continue
			}
			links = append(links, &compliancev1.EvidenceLink{
				SourceRef: node.ArtifactID,
				TargetRef: ref,
				LinkType:  evidenceLinkTypeReferences,
			})
		}
	}
	sort.Slice(links, func(i, j int) bool {
		if links[i].GetSourceRef() != links[j].GetSourceRef() {
			return links[i].GetSourceRef() < links[j].GetSourceRef()
		}
		return links[i].GetTargetRef() < links[j].GetTargetRef()
	})
	return links
}

func buildGaps(request AnalysisRequest, assertionIndex map[string]*compliancev1.ControlAssertionDefinition) []*compliancev1.ComplianceGap {
	gaps := make([]*compliancev1.ComplianceGap, 0)
	for _, assessment := range request.AssertionAssessments {
		if assessment.GetStatus() == statusSatisfied || assessment.GetStatus() == "not_applicable" {
			continue
		}
		assertionKey := versionedReferenceKey(assessment.GetAssertionRef().GetId(), assessment.GetAssertionRef().GetVersion())
		assertion := assertionIndex[assertionKey]
		responsibility := ""
		if assertion != nil {
			responsibility = assertion.GetResponsibility()
		}
		gaps = append(gaps, &compliancev1.ComplianceGap{
			GapId:          gapID(request.ScopeID, assessment.GetAssessmentId(), "", ""),
			AssertionRef:   assessment.GetAssessmentId(),
			GapType:        assessment.GetStatus(),
			Description:    gapDescription(assessment.GetStatus(), assessment.GetFailureReason(), assessment.GetAssertionRef().GetId()),
			Responsibility: responsibility,
		})
	}
	for _, assessment := range request.FrameworkAssessments {
		if assessment.GetStatus() == statusSatisfied || assessment.GetStatus() == "not_applicable" {
			continue
		}
		frameworkRef := versionedReferenceKey(assessment.GetFrameworkRef().GetId(), assessment.GetFrameworkRef().GetVersion())
		gaps = append(gaps, &compliancev1.ComplianceGap{
			GapId:          gapID(request.ScopeID, "", frameworkRef, assessment.GetControlId()),
			FrameworkRef:   frameworkRef,
			ControlId:      assessment.GetControlId(),
			GapType:        assessment.GetStatus(),
			Description:    gapDescription(assessment.GetStatus(), "", assessment.GetControlId()),
			Responsibility: assessment.GetResponsibility(),
		})
	}
	sort.Slice(gaps, func(i, j int) bool {
		return gaps[i].GetGapId() < gaps[j].GetGapId()
	})
	return gaps
}

func buildLimitations(request AnalysisRequest) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, assessment := range request.AssertionAssessments {
		for _, limitation := range assessment.GetLimitations() {
			if limitation == "" {
				continue
			}
			if _, exists := seen[limitation]; exists {
				continue
			}
			seen[limitation] = struct{}{}
			result = append(result, limitation)
		}
	}
	for _, assessment := range request.FrameworkAssessments {
		for _, limitation := range assessment.GetLimitations() {
			if limitation == "" {
				continue
			}
			if _, exists := seen[limitation]; exists {
				continue
			}
			seen[limitation] = struct{}{}
			result = append(result, limitation)
		}
	}
	sort.Strings(result)
	return result
}

func buildFindings(request AnalysisRequest, assertionIndex map[string]*compliancev1.ControlAssertionDefinition) []*compliancev1.ComplianceFinding {
	findings := make([]*compliancev1.ComplianceFinding, 0)
	for _, assessment := range request.AssertionAssessments {
		if assessment.GetStatus() != statusNotSatisfied {
			continue
		}
		assertionKey := versionedReferenceKey(assessment.GetAssertionRef().GetId(), assessment.GetAssertionRef().GetVersion())
		assertion := assertionIndex[assertionKey]
		severity := findingSeverityMedium
		if assertion != nil && assertion.GetCategory() == "governance" {
			severity = findingSeverityHigh
		}
		findings = append(findings, &compliancev1.ComplianceFinding{
			FindingId:           findingID(request.ScopeID, assessment.GetAssessmentId(), ""),
			SubjectRef:          assessment.GetAssessmentId(),
			Severity:            severity,
			Description:         fmt.Sprintf("assertion %s is not satisfied: %s", assessment.GetAssertionRef().GetId(), assessment.GetFailureReason()),
			RelatedAssertionRef: assessment.GetAssessmentId(),
		})
	}
	for _, assessment := range request.FrameworkAssessments {
		if assessment.GetStatus() != statusNotSatisfied {
			continue
		}
		findings = append(findings, &compliancev1.ComplianceFinding{
			FindingId:        findingID(request.ScopeID, "", assessment.GetAssessmentId()),
			SubjectRef:       assessment.GetAssessmentId(),
			Severity:         findingSeverityMedium,
			Description:      fmt.Sprintf("control %s is not satisfied", assessment.GetControlId()),
			RelatedControlId: assessment.GetControlId(),
		})
	}
	sort.Slice(findings, func(i, j int) bool {
		return findings[i].GetFindingId() < findings[j].GetFindingId()
	})
	return findings
}

func buildRemediation(findings []*compliancev1.ComplianceFinding) []*compliancev1.ComplianceRemediation {
	remediation := make([]*compliancev1.ComplianceRemediation, 0, len(findings))
	for _, finding := range findings {
		priority := remediationPriorityMedium
		owner := remediationOwnerPlatform
		if finding.GetSeverity() == findingSeverityHigh {
			priority = remediationPriorityHigh
		} else if finding.GetSeverity() == findingSeverityLow {
			priority = remediationPriorityLow
		}
		if finding.GetRelatedControlId() != "" {
			owner = remediationOwnerCustomer
		}
		remediation = append(remediation, &compliancev1.ComplianceRemediation{
			RemediationId: remediationID(finding.GetFindingId()),
			FindingRef:    finding.GetFindingId(),
			Action:        remediationAction(finding),
			Priority:      priority,
			Owner:         owner,
		})
	}
	return remediation
}

func buildSections(request AnalysisRequest) []*compliancev1.ControlSection {
	groups := make(map[string][]string)
	for _, assessment := range request.FrameworkAssessments {
		responsibility := assessment.GetResponsibility()
		if responsibility == "" {
			responsibility = sectionResponsibilityPlatform
		}
		groups[responsibility] = append(groups[responsibility], assessment.GetAssessmentId())
	}
	sections := make([]*compliancev1.ControlSection, 0, len(groups))
	for responsibility, refs := range groups {
		sort.Strings(refs)
		sections = append(sections, &compliancev1.ControlSection{
			SectionId:             sectionID(request.ScopeID, responsibility),
			Title:                 sectionTitle(responsibility),
			Responsibility:        responsibility,
			ControlAssessmentRefs: refs,
			Description:           sectionDescription(responsibility, len(refs)),
		})
	}
	sort.Slice(sections, func(i, j int) bool {
		return sections[i].GetSectionId() < sections[j].GetSectionId()
	})
	return sections
}

func buildGraphFailureMessages(graph *EvidenceGraph) []string {
	failures := graph.Failures()
	messages := make([]string, 0, len(failures))
	for _, failure := range failures {
		messages = append(messages, fmt.Sprintf("%s: %s: %s", failure.Code.Error(), failure.Subject, failure.Reason))
	}
	sort.Strings(messages)
	return messages
}

func sortedAssertionAssessments(assessments []*compliancev1.ControlAssertionAssessment) []*compliancev1.ControlAssertionAssessment {
	result := append([]*compliancev1.ControlAssertionAssessment(nil), assessments...)
	sort.Slice(result, func(i, j int) bool {
		return result[i].GetAssessmentId() < result[j].GetAssessmentId()
	})
	return result
}

func sortedFrameworkAssessments(assessments []*compliancev1.FrameworkControlAssessment) []*compliancev1.FrameworkControlAssessment {
	result := append([]*compliancev1.FrameworkControlAssessment(nil), assessments...)
	sort.Slice(result, func(i, j int) bool {
		return result[i].GetAssessmentId() < result[j].GetAssessmentId()
	})
	return result
}

func analysisID(request AnalysisRequest) string {
	assertionIDs := make([]string, 0, len(request.AssertionAssessments))
	for _, a := range request.AssertionAssessments {
		assertionIDs = append(assertionIDs, a.GetAssessmentId())
	}
	sort.Strings(assertionIDs)
	frameworkIDs := make([]string, 0, len(request.FrameworkAssessments))
	for _, a := range request.FrameworkAssessments {
		frameworkIDs = append(frameworkIDs, a.GetAssessmentId())
	}
	sort.Strings(frameworkIDs)
	identity := strings.Join([]string{
		request.ScopeID,
		constants.AnalysisSchemaVersion,
		constants.AnalysisBuilderID,
		constants.AnalysisBuilderVersion,
		request.EvaluatedAt.UTC().Format(time.RFC3339Nano),
		strings.Join(assertionIDs, ","),
		strings.Join(frameworkIDs, ","),
	}, "\x00")
	digest := sha256.Sum256([]byte(identity))
	return "compliance-analysis:sha256:" + hex.EncodeToString(digest[:])
}

func gapID(scopeID, assertionRef, frameworkRef, controlID string) string {
	identity := strings.Join([]string{scopeID, assertionRef, frameworkRef, controlID}, "\x00")
	digest := sha256.Sum256([]byte(identity))
	return "compliance-gap:sha256:" + hex.EncodeToString(digest[:])
}

func findingID(scopeID, assertionRef, controlRef string) string {
	identity := strings.Join([]string{scopeID, assertionRef, controlRef}, "\x00")
	digest := sha256.Sum256([]byte(identity))
	return "compliance-finding:sha256:" + hex.EncodeToString(digest[:])
}

func remediationID(findingRef string) string {
	digest := sha256.Sum256([]byte(findingRef))
	return "compliance-remediation:sha256:" + hex.EncodeToString(digest[:])
}

func sectionID(scopeID, responsibility string) string {
	identity := strings.Join([]string{scopeID, responsibility}, "\x00")
	digest := sha256.Sum256([]byte(identity))
	return "control-section:sha256:" + hex.EncodeToString(digest[:])
}

func gapDescription(status, failureReason, subjectID string) string {
	if failureReason != "" {
		return fmt.Sprintf("%s %s: %s", subjectID, status, failureReason)
	}
	return fmt.Sprintf("%s is %s", subjectID, status)
}

func remediationAction(finding *compliancev1.ComplianceFinding) string {
	if finding.GetRelatedControlId() != "" {
		return fmt.Sprintf("remediate control %s to satisfy the failed assertion evidence requirements", finding.GetRelatedControlId())
	}
	return fmt.Sprintf("provide verified evidence to satisfy assertion %s", finding.GetRelatedAssertionRef())
}

func sectionTitle(responsibility string) string {
	switch responsibility {
	case sectionResponsibilityPlatform:
		return "Platform Controls"
	case sectionResponsibilityCustomer:
		return "Customer Controls"
	case sectionResponsibilityShared:
		return "Shared Controls"
	case sectionResponsibilityInherited:
		return "Inherited Controls"
	default:
		return responsibility + " Controls"
	}
}

func sectionDescription(responsibility string, count int) string {
	return fmt.Sprintf("%d control assessment(s) with %s responsibility", count, responsibility)
}
