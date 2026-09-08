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

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

type FrameworkProfileRequest struct {
	Analysis   *compliancev1.ComplianceAnalysis
	Frameworks *compliancev1.FrameworkCatalog
}

func BuildFrameworkProfiles(ctx context.Context, request FrameworkProfileRequest) ([]*compliancev1.FrameworkProfile, error) {
	if err := validateFrameworkProfileRequest(request); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	frameworks := append([]*compliancev1.FrameworkDefinition(nil), request.Frameworks.GetFrameworks()...)
	sort.Slice(frameworks, func(i, j int) bool {
		return versionedReferenceKey(frameworks[i].GetFrameworkId(), frameworks[i].GetFrameworkVersion()) < versionedReferenceKey(frameworks[j].GetFrameworkId(), frameworks[j].GetFrameworkVersion())
	})
	profiles := make([]*compliancev1.FrameworkProfile, 0, len(frameworks))
	for _, framework := range frameworks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		profile, err := buildFrameworkProfile(request.Analysis, framework)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

func validateFrameworkProfileRequest(request FrameworkProfileRequest) error {
	analysis := request.Analysis
	if analysis == nil || request.Frameworks == nil {
		return fmt.Errorf("%w: request is incomplete", constants.ErrFrameworkProfileInvalid)
	}
	if analysis.GetAnalysisId() == "" || analysis.GetAnalysisSchemaVersion() != constants.AnalysisSchemaVersion || analysis.GetScopeRef() == "" || analysis.GetGeneratedAt() == nil || analysis.GetGeneratorIdentity() != constants.AnalysisBuilderID || analysis.GetGeneratorVersion() != constants.AnalysisBuilderVersion {
		return fmt.Errorf("%w: analysis identity is incomplete or unsupported", constants.ErrFrameworkProfileInvalid)
	}
	if analysis.GetGeneratedAt().CheckValid() != nil {
		return fmt.Errorf("%w: analysis generation time is invalid", constants.ErrFrameworkProfileInvalid)
	}
	if !analysis.GetEvidenceGraphValid() {
		return fmt.Errorf("%w: analysis evidence graph is invalid", constants.ErrFrameworkProfileInvalid)
	}
	if err := catalog.ValidateFrameworkCatalog(request.Frameworks); err != nil {
		return fmt.Errorf("%w: framework catalog: %w", constants.ErrFrameworkProfileInvalid, err)
	}
	assessmentIDs := make(map[string]struct{}, len(analysis.GetFrameworkAssessments()))
	controlKeys := make(map[string]struct{}, len(analysis.GetFrameworkAssessments()))
	for _, assessment := range analysis.GetFrameworkAssessments() {
		if assessment == nil || assessment.GetAssessmentId() == "" || assessment.GetFrameworkRef() == nil || assessment.GetControlId() == "" || assessment.GetResponsibility() == "" || assessment.GetEvidenceLevel() == "" || len(assessment.GetMappingRefs()) == 0 {
			return fmt.Errorf("%w: framework assessment is incomplete", constants.ErrFrameworkProfileInvalid)
		}
		if !validFrameworkProfileStatus(assessment.GetStatus()) || evidenceLevelIndexOrdered(assessment.GetEvidenceLevel()) < 0 {
			return fmt.Errorf("%w: framework assessment %s has invalid semantics", constants.ErrFrameworkProfileInvalid, assessment.GetAssessmentId())
		}
		if assessment.GetScopeId() != analysis.GetScopeRef() {
			return fmt.Errorf("%w: framework assessment %s belongs to scope %s", constants.ErrFrameworkProfileInvalid, assessment.GetAssessmentId(), assessment.GetScopeId())
		}
		framework := catalog.FindFramework(request.Frameworks, assessment.GetFrameworkRef().GetId(), assessment.GetFrameworkRef().GetVersion())
		if framework == nil {
			return fmt.Errorf("%w: framework assessment %s references an unknown framework", constants.ErrFrameworkProfileInvalid, assessment.GetAssessmentId())
		}
		control := catalog.FindFrameworkControl(framework, assessment.GetControlId())
		if control == nil {
			return fmt.Errorf("%w: framework assessment %s references an unknown control", constants.ErrFrameworkProfileInvalid, assessment.GetAssessmentId())
		}
		if assessment.GetResponsibility() != control.GetResponsibility() {
			return fmt.Errorf("%w: framework assessment %s responsibility does not match its control", constants.ErrFrameworkProfileInvalid, assessment.GetAssessmentId())
		}
		if _, exists := assessmentIDs[assessment.GetAssessmentId()]; exists {
			return fmt.Errorf("%w: duplicate assessment %s", constants.ErrFrameworkProfileInvalid, assessment.GetAssessmentId())
		}
		assessmentIDs[assessment.GetAssessmentId()] = struct{}{}
		controlKey := frameworkControlKey(assessment)
		if _, exists := controlKeys[controlKey]; exists {
			return fmt.Errorf("%w: duplicate assessed control %s", constants.ErrFrameworkProfileInvalid, controlKey)
		}
		controlKeys[controlKey] = struct{}{}
	}
	return nil
}

func validFrameworkProfileStatus(status string) bool {
	switch status {
	case "satisfied", "not_satisfied", "not_applicable", "unverifiable", "customer_attestation_required":
		return true
	default:
		return false
	}
}

func buildFrameworkProfile(analysis *compliancev1.ComplianceAnalysis, framework *compliancev1.FrameworkDefinition) (*compliancev1.FrameworkProfile, error) {
	assessments := make([]*compliancev1.FrameworkControlAssessment, 0)
	limitations := make(map[string]struct{})
	for _, assessment := range analysis.GetFrameworkAssessments() {
		if assessment.GetFrameworkRef().GetId() != framework.GetFrameworkId() || assessment.GetFrameworkRef().GetVersion() != framework.GetFrameworkVersion() {
			continue
		}
		assessments = append(assessments, proto.Clone(assessment).(*compliancev1.FrameworkControlAssessment))
		for _, limitation := range assessment.GetLimitations() {
			if limitation != "" {
				limitations[limitation] = struct{}{}
			}
		}
	}
	sort.Slice(assessments, func(i, j int) bool {
		return assessments[i].GetAssessmentId() < assessments[j].GetAssessmentId()
	})
	profile := &compliancev1.FrameworkProfile{
		FrameworkRef:       &compliancev1.VersionedReference{Id: framework.GetFrameworkId(), Version: framework.GetFrameworkVersion()},
		ProfileVersion:     constants.FrameworkProfileVersion,
		GeneratedAt:        timestamppb.New(analysis.GetGeneratedAt().AsTime()),
		AnalysisRef:        analysis.GetAnalysisId(),
		ControlAssessments: assessments,
		Limitations:        sortedKeys(limitations),
	}
	encoded, err := compliancev1.MarshalCanonical(profile)
	if err != nil {
		return nil, fmt.Errorf("%w: canonicalize %s: %w", constants.ErrFrameworkProfileInvalid, versionedReferenceKey(framework.GetFrameworkId(), framework.GetFrameworkVersion()), err)
	}
	digest := sha256.Sum256(encoded)
	profile.ProfileId = "framework-profile:sha256:" + hex.EncodeToString(digest[:])
	return profile, nil
}
