// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

const (
	frameworkMappingTypeFull          = "full"
	frameworkMappingTypePartial       = "partial"
	frameworkMappingTypeSupporting    = "supporting"
	frameworkMappingTypeNotApplicable = "not_applicable"
)

type FrameworkGradingRequest struct {
	ScopeID              string
	EvaluatedAt          time.Time
	Frameworks           *compliancev1.FrameworkCatalog
	Crosswalks           *compliancev1.ControlCrosswalkCatalog
	Assertions           *compliancev1.ControlAssertionCatalog
	AssertionAssessments []*compliancev1.ControlAssertionAssessment
}

func GradeFrameworkControls(ctx context.Context, request FrameworkGradingRequest) ([]*compliancev1.FrameworkControlAssessment, error) {
	if err := validateFrameworkGradingRequest(request); err != nil {
		return nil, err
	}
	assertionIndex := indexAssertionAssessments(request.AssertionAssessments)
	controls := collectMappedControls(request.Frameworks, request.Crosswalks)
	result := make([]*compliancev1.FrameworkControlAssessment, 0, len(controls))
	for _, control := range controls {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		assessment, err := gradeFrameworkControl(request, control, assertionIndex)
		if err != nil {
			return nil, err
		}
		result = append(result, assessment)
	}
	sort.Slice(result, func(i, j int) bool {
		return frameworkControlKey(result[i]) < frameworkControlKey(result[j])
	})
	return result, nil
}

func validateFrameworkGradingRequest(request FrameworkGradingRequest) error {
	if request.ScopeID == "" || request.EvaluatedAt.IsZero() || request.Frameworks == nil || request.Crosswalks == nil || request.Assertions == nil {
		return fmt.Errorf("%w: framework grading request is incomplete", constants.ErrInvalidEvidenceGraph)
	}
	if err := catalog.ValidateFrameworkCatalog(request.Frameworks); err != nil {
		return fmt.Errorf("framework grading frameworks: %w", err)
	}
	if err := catalog.ValidateAssertionCatalog(request.Assertions); err != nil {
		return fmt.Errorf("framework grading assertions: %w", err)
	}
	assessmentIDs := make(map[string]struct{}, len(request.AssertionAssessments))
	assertionRefs := make(map[string]struct{}, len(request.AssertionAssessments))
	for _, assessment := range request.AssertionAssessments {
		if err := catalog.ValidateAssertionAssessment(assessment, request.ScopeID, request.Assertions); err != nil {
			return fmt.Errorf("framework grading assertion assessment: %w", err)
		}
		if _, exists := assessmentIDs[assessment.GetAssessmentId()]; exists {
			return fmt.Errorf("%w: duplicate assertion assessment %s", constants.ErrEvidenceDuplicateID, assessment.GetAssessmentId())
		}
		assessmentIDs[assessment.GetAssessmentId()] = struct{}{}
		assertionRef := versionedReferenceKey(assessment.GetAssertionRef().GetId(), assessment.GetAssertionRef().GetVersion())
		if _, exists := assertionRefs[assertionRef]; exists {
			return fmt.Errorf("%w: duplicate assertion assessment reference %s", constants.ErrEvidenceDuplicateID, assertionRef)
		}
		assertionRefs[assertionRef] = struct{}{}
	}
	return nil
}

type mappedControl struct {
	Framework *compliancev1.FrameworkDefinition
	Control   *compliancev1.FrameworkControlDefinition
	Mappings  []*compliancev1.ControlCrosswalk
}

func collectMappedControls(frameworks *compliancev1.FrameworkCatalog, crosswalks *compliancev1.ControlCrosswalkCatalog) []mappedControl {
	seen := make(map[string]mappedControl)
	for _, mapping := range crosswalks.Mappings {
		if mapping == nil || mapping.FrameworkRef == nil || mapping.MappingType == frameworkMappingTypeNotApplicable {
			continue
		}
		framework := catalog.FindFramework(frameworks, mapping.FrameworkRef.Id, mapping.FrameworkRef.Version)
		if framework == nil {
			continue
		}
		control := catalog.FindFrameworkControl(framework, mapping.ControlId)
		if control == nil || control.SupportStatus != "mapped" {
			continue
		}
		key := frameworkControlKeyFromParts(framework.FrameworkId, framework.FrameworkVersion, control.ControlId)
		entry, exists := seen[key]
		if !exists {
			entry = mappedControl{Framework: framework, Control: control}
		}
		entry.Mappings = append(entry.Mappings, mapping)
		seen[key] = entry
	}
	result := make([]mappedControl, 0, len(seen))
	for _, entry := range seen {
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool {
		return frameworkControlKeyFromParts(result[i].Framework.FrameworkId, result[i].Framework.FrameworkVersion, result[i].Control.ControlId) <
			frameworkControlKeyFromParts(result[j].Framework.FrameworkId, result[j].Framework.FrameworkVersion, result[j].Control.ControlId)
	})
	return result
}

func gradeFrameworkControl(request FrameworkGradingRequest, control mappedControl, assertionIndex map[string]*compliancev1.ControlAssertionAssessment) (*compliancev1.FrameworkControlAssessment, error) {
	mappingRefs := make([]string, 0, len(control.Mappings))
	assertionAssessmentRefs := make(map[string]struct{})
	evidenceLevels := make([]string, 0)
	statuses := make([]string, 0)
	limitations := make([]string, 0)
	customerAttestationRequired := false
	for _, mapping := range control.Mappings {
		mappingRefs = append(mappingRefs, mapping.CrosswalkId)
		for _, assertionRef := range mapping.AssertionRefs {
			key := versionedReferenceKey(assertionRef.Id, assertionRef.Version)
			assessment := assertionIndex[key]
			if assessment == nil {
				statuses = append(statuses, "unverifiable")
				limitations = append(limitations, fmt.Sprintf("assertion %s has no assessment", key))
				continue
			}
			assertionAssessmentRefs[assessment.AssessmentId] = struct{}{}
			statuses = append(statuses, assessment.Status)
			evidenceLevels = append(evidenceLevels, assessment.EvidenceLevel)
			if assessment.Status == "customer_attestation_required" {
				customerAttestationRequired = true
			}
			if mapping.MappingType == frameworkMappingTypePartial || mapping.MappingType == frameworkMappingTypeSupporting {
				limitations = append(limitations, fmt.Sprintf("mapping %s is %s: assertion supports but does not fully satisfy the control", mapping.CrosswalkId, mapping.MappingType))
			}
		}
	}
	status := frameworkControlStatus(statuses, control.Control.Responsibility, customerAttestationRequired)
	evidenceLevel := frameworkControlEvidenceLevel(status, evidenceLevels)
	if status == "satisfied" && control.Control.Responsibility == "customer" {
		limitations = append(limitations, "customer-responsibility control requires customer attestation evidence for satisfaction")
	}
	assessment := &compliancev1.FrameworkControlAssessment{
		AssessmentId:            frameworkControlAssessmentID(request.ScopeID, control.Framework.FrameworkId, control.Framework.FrameworkVersion, control.Control.ControlId),
		ScopeId:                 request.ScopeID,
		FrameworkRef:            &compliancev1.VersionedReference{Id: control.Framework.FrameworkId, Version: control.Framework.FrameworkVersion},
		ControlId:               control.Control.ControlId,
		Status:                  status,
		Responsibility:          control.Control.Responsibility,
		MappingRefs:             sortedStringSlice(mappingRefs),
		AssertionAssessmentRefs: sortedKeys(assertionAssessmentRefs),
		CustomerAttestationRefs: []string{},
		EvidenceLevel:           evidenceLevel,
		Findings:                []string{},
		Limitations:             limitations,
		Remediation:             []string{},
	}
	if err := catalog.ValidateControlAssessment(assessment, request.ScopeID, request.Frameworks, request.Crosswalks); err != nil {
		return nil, fmt.Errorf("framework grading %s/%s: %w", control.Framework.FrameworkId, control.Control.ControlId, err)
	}
	return assessment, nil
}

func frameworkControlStatus(statuses []string, responsibility string, customerAttestationRequired bool) string {
	if len(statuses) == 0 {
		return "unverifiable"
	}
	hasNotSatisfied := false
	hasCustomerAttestation := false
	hasUnverifiable := false
	hasNotApplicable := false
	allNotApplicable := true
	for _, status := range statuses {
		switch status {
		case "not_satisfied":
			hasNotSatisfied = true
			allNotApplicable = false
		case "customer_attestation_required":
			hasCustomerAttestation = true
			allNotApplicable = false
		case "unverifiable":
			hasUnverifiable = true
			allNotApplicable = false
		case "not_applicable":
			hasNotApplicable = true
		case "satisfied":
			allNotApplicable = false
		}
	}
	if allNotApplicable && hasNotApplicable {
		return "not_applicable"
	}
	if hasNotSatisfied {
		return "not_satisfied"
	}
	if hasCustomerAttestation || (customerAttestationRequired && responsibility == "customer") {
		return "customer_attestation_required"
	}
	if hasUnverifiable {
		return "unverifiable"
	}
	return "satisfied"
}

func frameworkControlEvidenceLevel(status string, levels []string) string {
	if status != "satisfied" && status != "not_satisfied" {
		return "L0"
	}
	if len(levels) == 0 {
		return "L0"
	}
	minIndex := len(evidenceLevelsOrdered)
	for _, level := range levels {
		idx := evidenceLevelIndexOrdered(level)
		if idx >= 0 && idx < minIndex {
			minIndex = idx
		}
	}
	if minIndex >= len(evidenceLevelsOrdered) {
		return "L0"
	}
	return evidenceLevelsOrdered[minIndex]
}

func indexAssertionAssessments(assessments []*compliancev1.ControlAssertionAssessment) map[string]*compliancev1.ControlAssertionAssessment {
	index := make(map[string]*compliancev1.ControlAssertionAssessment, len(assessments))
	for _, assessment := range assessments {
		if assessment == nil || assessment.AssertionRef == nil {
			continue
		}
		index[versionedReferenceKey(assessment.AssertionRef.Id, assessment.AssertionRef.Version)] = assessment
	}
	return index
}

func frameworkControlAssessmentID(scopeID, frameworkID, frameworkVersion, controlID string) string {
	identity := strings.Join([]string{scopeID, frameworkID, frameworkVersion, controlID, constants.FrameworkGraderID, constants.FrameworkGraderVersion}, "\x00")
	digest := sha256.Sum256([]byte(identity))
	return "framework-assessment:sha256:" + hex.EncodeToString(digest[:])
}

func frameworkControlKey(assessment *compliancev1.FrameworkControlAssessment) string {
	if assessment == nil || assessment.FrameworkRef == nil {
		return ""
	}
	return frameworkControlKeyFromParts(assessment.FrameworkRef.Id, assessment.FrameworkRef.Version, assessment.ControlId)
}

func frameworkControlKeyFromParts(frameworkID, frameworkVersion, controlID string) string {
	return versionedReferenceKey(frameworkID, frameworkVersion) + "/" + controlID
}

func sortedStringSlice(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

var evidenceLevelsOrdered = []string{"L0", "L1", "L2", "L3", "L4", "L5"}

func evidenceLevelIndexOrdered(level string) int {
	for i, candidate := range evidenceLevelsOrdered {
		if candidate == level {
			return i
		}
	}
	return -1
}
