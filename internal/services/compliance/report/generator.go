// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package report

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

type GenerationRequest struct {
	ScopeID     string
	WindowStart time.Time
	WindowEnd   time.Time
	EvaluatedAt time.Time
	Importers   []evidence.EvidenceImporter
	Assertions  *compliancev1.ControlAssertionCatalog
	Frameworks  *compliancev1.FrameworkCatalog
	Crosswalks  *compliancev1.ControlCrosswalkCatalog
}

type GenerationResult struct {
	Analysis    *compliancev1.ComplianceAnalysis
	Profiles    []*compliancev1.FrameworkProfile
	GraphReport *evidence.EvidenceGraphReport
}

type SignedBundleGenerationRequest struct {
	Generation      GenerationRequest
	Profile         BundleProfile
	ReportID        string
	SigningIdentity *ComplianceReportSigningIdentity
	SourceArtifacts []SourceArtifact
}

func GenerateSignedComplianceBundle(ctx context.Context, request SignedBundleGenerationRequest) (*BundleAssemblyResult, error) {
	result, err := GenerateComplianceAnalysis(ctx, request.Generation)
	if err != nil {
		return nil, err
	}
	renderedFormats, err := renderAllFormats(result.Analysis)
	if err != nil {
		return nil, err
	}
	sourceArtifacts, err := canonicalReportSourceArtifacts(request.Generation, result)
	if err != nil {
		return nil, err
	}
	sourceArtifacts = append(sourceArtifacts, request.SourceArtifacts...)
	frameworkRefs := make([]*compliancev1.VersionedReference, 0, len(request.Generation.Frameworks.GetFrameworks()))
	for _, framework := range request.Generation.Frameworks.GetFrameworks() {
		frameworkRefs = append(frameworkRefs, &compliancev1.VersionedReference{Id: framework.GetFrameworkId(), Version: framework.GetFrameworkVersion()})
	}
	sort.Slice(frameworkRefs, func(i, j int) bool {
		if frameworkRefs[i].GetId() == frameworkRefs[j].GetId() {
			return frameworkRefs[i].GetVersion() < frameworkRefs[j].GetVersion()
		}
		return frameworkRefs[i].GetId() < frameworkRefs[j].GetId()
	})
	assembly, err := AssembleBundle(BundleAssemblyRequest{
		Profile:             request.Profile,
		Analysis:            result.Analysis,
		Profiles:            result.Profiles,
		RenderedFormats:     renderedFormats,
		ScopeRef:            request.Generation.ScopeID,
		ReportID:            request.ReportID,
		GeneratedAt:         request.Generation.EvaluatedAt,
		FrameworkRefs:       frameworkRefs,
		AssertionCatalogRef: path.Join(constants.ComplianceBundleAssertionsDirname, constants.ComplianceBundleAssertionCatalogFilename),
		CrosswalkRefs:       []string{path.Join(constants.ComplianceBundleCrosswalksDirname, constants.ComplianceBundleCrosswalkFilename)},
		AssessmentRefs: []string{
			path.Join(constants.ComplianceBundleAssessmentsDirname, constants.ComplianceBundleAssertionAssessmentsFilename),
			path.Join(constants.ComplianceBundleAssessmentsDirname, constants.ComplianceBundleControlAssessmentsFilename),
		},
		EvidenceIndexRef: path.Join(constants.ComplianceBundleEvidenceDirname, constants.ComplianceBundleEvidenceIndexFilename),
		SourceArtifacts:  sourceArtifacts,
	})
	if err != nil {
		return nil, err
	}
	if err := SignBundle(assembly, request.SigningIdentity); err != nil {
		return nil, err
	}
	return assembly, nil
}

func renderAllFormats(analysis *compliancev1.ComplianceAnalysis) ([]RenderedFormat, error) {
	bundlePaths := map[Format]string{
		FormatJSON:     constants.ComplianceBundleJSONPath,
		FormatOSCAL:    constants.ComplianceBundleOSCALPath,
		FormatMarkdown: constants.ComplianceBundleMarkdownPath,
		FormatHTML:     constants.ComplianceBundleHTMLPath,
		FormatCLI:      constants.ComplianceBundleCLIPath,
	}
	formats := make([]RenderedFormat, 0, len(SupportedFormats()))
	for _, format := range SupportedFormats() {
		rendered, err := RenderComplianceAnalysis(analysis, format)
		if err != nil {
			return nil, fmt.Errorf("compliance report: render protected %s output: %w", format, err)
		}
		formats = append(formats, RenderedFormat{Format: format, MediaType: rendered.MediaType, BundlePath: bundlePaths[format], Body: rendered.Body})
	}
	return formats, nil
}

func canonicalReportSourceArtifacts(request GenerationRequest, result *GenerationResult) ([]SourceArtifact, error) {
	assertions, err := compliancev1.MarshalCanonical(request.Assertions)
	if err != nil {
		return nil, fmt.Errorf("compliance report: canonicalize assertion catalog: %w", err)
	}
	frameworks, err := compliancev1.MarshalCanonical(request.Frameworks)
	if err != nil {
		return nil, fmt.Errorf("compliance report: canonicalize framework catalog: %w", err)
	}
	crosswalks, err := compliancev1.MarshalCanonical(request.Crosswalks)
	if err != nil {
		return nil, fmt.Errorf("compliance report: canonicalize crosswalk catalog: %w", err)
	}
	assertionAssessments, err := marshalCanonicalMessages(result.Analysis.GetAssertionAssessments())
	if err != nil {
		return nil, fmt.Errorf("compliance report: canonicalize assertion assessments: %w", err)
	}
	frameworkAssessments, err := marshalCanonicalMessages(result.Analysis.GetFrameworkAssessments())
	if err != nil {
		return nil, fmt.Errorf("compliance report: canonicalize framework assessments: %w", err)
	}
	evidenceIndex, err := marshalCanonicalMessages(result.Analysis.GetEvidenceResources())
	if err != nil {
		return nil, fmt.Errorf("compliance report: canonicalize evidence index: %w", err)
	}
	return []SourceArtifact{
		{BundlePath: path.Join(constants.ComplianceBundleAssertionsDirname, constants.ComplianceBundleAssertionCatalogFilename), Body: assertions, MediaType: constants.MediaTypeJSON},
		{BundlePath: path.Join(constants.ComplianceBundleFrameworkCatalogsDirname, constants.ComplianceBundleFrameworkCatalogFilename), Body: frameworks, MediaType: constants.MediaTypeJSON},
		{BundlePath: path.Join(constants.ComplianceBundleCrosswalksDirname, constants.ComplianceBundleCrosswalkFilename), Body: crosswalks, MediaType: constants.MediaTypeJSON},
		{BundlePath: path.Join(constants.ComplianceBundleAssessmentsDirname, constants.ComplianceBundleAssertionAssessmentsFilename), Body: assertionAssessments, MediaType: constants.MediaTypeJSON},
		{BundlePath: path.Join(constants.ComplianceBundleAssessmentsDirname, constants.ComplianceBundleControlAssessmentsFilename), Body: frameworkAssessments, MediaType: constants.MediaTypeJSON},
		{BundlePath: path.Join(constants.ComplianceBundleEvidenceDirname, constants.ComplianceBundleEvidenceIndexFilename), Body: evidenceIndex, MediaType: constants.MediaTypeJSON},
	}, nil
}

func marshalCanonicalMessages[T proto.Message](messages []T) ([]byte, error) {
	var body bytes.Buffer
	body.WriteByte('[')
	for index, message := range messages {
		if index > 0 {
			body.WriteByte(',')
		}
		encoded, err := compliancev1.MarshalCanonical(message)
		if err != nil {
			return nil, err
		}
		body.Write(encoded)
	}
	body.WriteByte(']')
	return body.Bytes(), nil
}

func GenerateComplianceAnalysis(ctx context.Context, request GenerationRequest) (*GenerationResult, error) {
	if len(request.Importers) == 0 {
		return nil, fmt.Errorf("%w: report generation requires evidence importers", constants.ErrInvalidEvidenceGraph)
	}
	graph, graphReport := evidence.BuildAndValidateGraph(ctx, request.Importers, request.WindowStart, request.WindowEnd, request.EvaluatedAt)
	result := &GenerationResult{GraphReport: graphReport}
	if !graphReport.Valid {
		return result, fmt.Errorf("%w: evidence graph verification failed", constants.ErrReportVerificationFailed)
	}
	if len(graph.NodesByScope(request.ScopeID)) == 0 {
		return result, fmt.Errorf("%w: no evidence belongs to scope %s", constants.ErrEvidenceScopeMismatch, request.ScopeID)
	}
	for scopeID := range graphReport.NodesByScope {
		if scopeID != request.ScopeID {
			return result, fmt.Errorf("%w: evidence belongs to scope %s instead of %s", constants.ErrEvidenceScopeMismatch, scopeID, request.ScopeID)
		}
	}

	assertionAssessments, err := evidence.GradeControlAssertions(ctx, evidence.AssertionGradingRequest{
		ScopeID:     request.ScopeID,
		WindowStart: request.WindowStart,
		WindowEnd:   request.WindowEnd,
		EvaluatedAt: request.EvaluatedAt,
		Assertions:  request.Assertions,
		Graph:       graph,
	})
	if err != nil {
		return result, fmt.Errorf("compliance report: grade assertions: %w", err)
	}
	frameworkAssessments, err := evidence.GradeFrameworkControls(ctx, evidence.FrameworkGradingRequest{
		ScopeID:              request.ScopeID,
		EvaluatedAt:          request.EvaluatedAt,
		Frameworks:           request.Frameworks,
		Crosswalks:           request.Crosswalks,
		Assertions:           request.Assertions,
		AssertionAssessments: assertionAssessments,
	})
	if err != nil {
		return result, fmt.Errorf("compliance report: grade frameworks: %w", err)
	}
	analysis, err := evidence.BuildComplianceAnalysis(ctx, evidence.AnalysisRequest{
		ScopeID:              request.ScopeID,
		WindowStart:          request.WindowStart,
		WindowEnd:            request.WindowEnd,
		EvaluatedAt:          request.EvaluatedAt,
		Graph:                graph,
		Assertions:           request.Assertions,
		Frameworks:           request.Frameworks,
		Crosswalks:           request.Crosswalks,
		AssertionAssessments: assertionAssessments,
		FrameworkAssessments: frameworkAssessments,
	})
	if err != nil {
		return result, fmt.Errorf("compliance report: build analysis: %w", err)
	}
	if _, err := compliance.NewOSCALExporter(nil).GenerateAssessmentResults(analysis); err != nil {
		return result, fmt.Errorf("compliance report: validate OSCAL projection: %w", err)
	}
	profiles, err := evidence.BuildFrameworkProfiles(ctx, evidence.FrameworkProfileRequest{
		Analysis:   analysis,
		Frameworks: request.Frameworks,
	})
	if err != nil {
		return result, fmt.Errorf("compliance report: build framework profiles: %w", err)
	}
	result.Analysis = analysis
	result.Profiles = profiles
	return result, nil
}
