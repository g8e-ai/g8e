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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

type GenerationSource struct {
	AdmissionID string
	Importer    evidence.EvidenceImporter
}

type GenerationRequest struct {
	Scope       *compliancev1.AssessmentScope
	Sources     []GenerationSource
	Assertions  *compliancev1.ControlAssertionCatalog
	Frameworks  *compliancev1.FrameworkCatalog
	Crosswalks  *compliancev1.ControlCrosswalkCatalog
	Diagnostics []*compliancev1.AssessmentDiagnostic
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
	GeneratedAt     time.Time
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
		ScopeRef:            request.Generation.Scope.GetScopeId(),
		ReportID:            request.ReportID,
		GeneratedAt:         request.GeneratedAt,
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
		FormatCSV:      constants.ComplianceBundleCSVPath,
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
	scope, err := compliancev1.MarshalCanonical(request.Scope)
	if err != nil {
		return nil, fmt.Errorf("compliance report: canonicalize assessment scope: %w", err)
	}
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
		{BundlePath: constants.ComplianceBundleScopeFilename, Body: scope, MediaType: constants.MediaTypeJSON},
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

func MarshalAssessmentDiagnostics(diagnostics []*compliancev1.AssessmentDiagnostic) ([]byte, error) {
	return marshalCanonicalMessages(diagnostics)
}

func UnmarshalAssessmentDiagnostics(body []byte) ([]*compliancev1.AssessmentDiagnostic, error) {
	var records []json.RawMessage
	if err := json.Unmarshal(body, &records); err != nil {
		return nil, fmt.Errorf("%w: decode assessment diagnostics: %v", constants.ErrEvidenceArtifactMalformed, err)
	}
	diagnostics := make([]*compliancev1.AssessmentDiagnostic, 0, len(records))
	for _, record := range records {
		diagnostic := &compliancev1.AssessmentDiagnostic{}
		if err := compliancev1.UnmarshalCanonical(record, diagnostic); err != nil {
			return nil, fmt.Errorf("%w: decode canonical assessment diagnostic: %v", constants.ErrEvidenceArtifactMalformed, err)
		}
		diagnostics = append(diagnostics, diagnostic)
	}
	canonical, err := MarshalAssessmentDiagnostics(diagnostics)
	if err != nil {
		return nil, fmt.Errorf("%w: canonicalize assessment diagnostics: %v", constants.ErrEvidenceArtifactMalformed, err)
	}
	if !bytes.Equal(canonical, body) {
		return nil, fmt.Errorf("%w: assessment diagnostics are not canonical", constants.ErrEvidenceArtifactMalformed)
	}
	return diagnostics, nil
}

type admittedEvidenceImporter struct {
	admission *compliancev1.AssessmentSourceAdmission
	importer  evidence.EvidenceImporter
}

func (i admittedEvidenceImporter) Import(ctx context.Context) ([]evidence.EvidenceNode, error) {
	nodes, err := i.importer.Import(ctx)
	if err != nil {
		return nil, err
	}
	selectedArtifacts := make(map[string]struct{}, len(i.admission.GetArtifactIds()))
	for _, artifactID := range i.admission.GetArtifactIds() {
		selectedArtifacts[artifactID] = struct{}{}
	}
	observedArtifacts := make(map[string]struct{}, len(nodes))
	for index := range nodes {
		node := &nodes[index]
		sharedDefinition := node.ArtifactType == evidence.ArtifactTypeDemoDefinition
		if node.SourceAdmissionID != "" && node.SourceAdmissionID != i.admission.GetAdmissionId() {
			return nil, fmt.Errorf("%w: source admission %s imported artifact %s already bound to %s", constants.ErrEvidenceScopeMismatch, i.admission.GetAdmissionId(), node.ArtifactID, node.SourceAdmissionID)
		}
		if !sharedDefinition && i.admission.GetRunId() != "" && node.RunID != i.admission.GetRunId() {
			return nil, fmt.Errorf("%w: source admission %s selected run %s but imported %s", constants.ErrEvidenceScopeMismatch, i.admission.GetAdmissionId(), i.admission.GetRunId(), node.RunID)
		}
		if len(selectedArtifacts) > 0 {
			if _, selected := selectedArtifacts[node.ArtifactID]; !selected {
				return nil, fmt.Errorf("%w: source admission %s imported unselected artifact %s", constants.ErrEvidenceScopeMismatch, i.admission.GetAdmissionId(), node.ArtifactID)
			}
		}
		if !sharedDefinition {
			node.SourceAdmissionID = i.admission.GetAdmissionId()
		}
		for _, diagnostic := range node.Diagnostics {
			if diagnostic == nil {
				continue
			}
			if diagnostic.GetSourceAdmissionId() != "" && diagnostic.GetSourceAdmissionId() != i.admission.GetAdmissionId() {
				return nil, fmt.Errorf("%w: source admission %s imported diagnostic for %s", constants.ErrEvidenceScopeMismatch, i.admission.GetAdmissionId(), diagnostic.GetSourceAdmissionId())
			}
			if diagnostic.GetSourceAdmissionId() == "" {
				diagnostic.SourceAdmissionId = i.admission.GetAdmissionId()
			}
		}
		observedArtifacts[node.ArtifactID] = struct{}{}
	}
	for artifactID := range selectedArtifacts {
		if _, observed := observedArtifacts[artifactID]; !observed {
			return nil, fmt.Errorf("%w: source admission %s did not import selected artifact %s", constants.ErrUnresolvedReference, i.admission.GetAdmissionId(), artifactID)
		}
	}
	return nodes, nil
}

func (i admittedEvidenceImporter) SourceID() string {
	return i.admission.GetAdmissionId()
}

func admittedImporters(scope *compliancev1.AssessmentScope, sources []GenerationSource) ([]evidence.EvidenceImporter, error) {
	admissions := make(map[string]*compliancev1.AssessmentSourceAdmission, len(scope.GetSourceAdmissions()))
	for _, admission := range scope.GetSourceAdmissions() {
		admissions[admission.GetAdmissionId()] = admission
	}
	if len(sources) != len(admissions) {
		return nil, fmt.Errorf("%w: generation sources do not match protected source admissions", constants.ErrInvalidEvidenceGraph)
	}
	result := make([]evidence.EvidenceImporter, 0, len(sources))
	seen := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		admission := admissions[source.AdmissionID]
		if admission == nil || source.Importer == nil {
			return nil, fmt.Errorf("%w: generation source %s is not admitted", constants.ErrInvalidEvidenceGraph, source.AdmissionID)
		}
		if _, exists := seen[source.AdmissionID]; exists {
			return nil, fmt.Errorf("%w: duplicate generation source %s", constants.ErrInvalidEvidenceGraph, source.AdmissionID)
		}
		seen[source.AdmissionID] = struct{}{}
		result = append(result, admittedEvidenceImporter{admission: admission, importer: source.Importer})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].SourceID() < result[j].SourceID() })
	return result, nil
}

func unavailableAssessmentContextDiagnostics(scope *compliancev1.AssessmentScope) []*compliancev1.AssessmentDiagnostic {
	labels := map[compliancev1.AssessmentContextKind]string{
		compliancev1.AssessmentContextKind_ASSESSMENT_CONTEXT_KIND_BUILD_IDENTITY:      "build identity",
		compliancev1.AssessmentContextKind_ASSESSMENT_CONTEXT_KIND_SOURCE_REVISION:     "source revision",
		compliancev1.AssessmentContextKind_ASSESSMENT_CONTEXT_KIND_COMPONENT_INVENTORY: "component inventory",
		compliancev1.AssessmentContextKind_ASSESSMENT_CONTEXT_KIND_NETWORK_TOPOLOGY:    "network topology",
		compliancev1.AssessmentContextKind_ASSESSMENT_CONTEXT_KIND_CONFIGURATION:       "configuration",
		compliancev1.AssessmentContextKind_ASSESSMENT_CONTEXT_KIND_DOCTRINE_BUNDLES:    "doctrine bundles",
		compliancev1.AssessmentContextKind_ASSESSMENT_CONTEXT_KIND_TRUST_ANCHORS:       "trust anchors",
	}
	diagnostics := make([]*compliancev1.AssessmentDiagnostic, 0, len(scope.GetUnavailableContext()))
	for _, record := range scope.GetUnavailableContext() {
		diagnostics = append(diagnostics, &compliancev1.AssessmentDiagnostic{
			Code:     "assessment_context_unavailable",
			Severity: "warning",
			Message:  labels[record.GetKind()] + ": " + record.GetReason(),
		})
	}
	return diagnostics
}

func GenerateComplianceAnalysis(ctx context.Context, request GenerationRequest) (*GenerationResult, error) {
	if err := catalog.ValidateAssessmentScope(request.Scope); err != nil {
		return nil, fmt.Errorf("compliance report: validate assessment scope: %w", err)
	}
	importers, err := admittedImporters(request.Scope, request.Sources)
	if err != nil {
		return nil, err
	}
	windowStart := request.Scope.GetAssessmentWindowStart().AsTime()
	windowEnd := request.Scope.GetAssessmentWindowEnd().AsTime()
	evaluatedAt := request.Scope.GetAssessmentAsOf().AsTime()
	graph, graphReport := evidence.BuildAndValidateGraph(ctx, importers, windowStart, windowEnd, evaluatedAt)
	result := &GenerationResult{GraphReport: graphReport}
	if !graphReport.Valid {
		return result, fmt.Errorf("%w: evidence graph verification failed", constants.ErrReportVerificationFailed)
	}
	scopeID := request.Scope.GetScopeId()
	for graphScopeID := range graphReport.NodesByScope {
		if graphScopeID != scopeID {
			return result, fmt.Errorf("%w: evidence belongs to scope %s instead of %s", constants.ErrEvidenceScopeMismatch, graphScopeID, scopeID)
		}
	}
	applicability := &evidence.AssertionApplicability{
		Components:    append([]string(nil), request.Scope.GetApplicability().GetComponents()...),
		ActionClasses: append([]string(nil), request.Scope.GetApplicability().GetActionClasses()...),
		Arms:          append([]string(nil), request.Scope.GetApplicability().GetArms()...),
	}
	population := &evidence.AssertionPopulation{Subjects: make([]evidence.AssertionSubject, 0, len(request.Scope.GetSelectedPopulation().GetSubjects()))}
	for _, subject := range request.Scope.GetSelectedPopulation().GetSubjects() {
		population.Subjects = append(population.Subjects, evidence.AssertionSubject{
			SourceAdmissionID: subject.GetSourceAdmissionId(),
			RunID:             subject.GetRunId(),
			AttemptID:         subject.GetAttemptId(),
			ScenarioID:        subject.GetScenarioId(),
			TransactionID:     subject.GetTransactionId(),
		})
	}

	assertionAssessments, err := evidence.GradeControlAssertions(ctx, evidence.AssertionGradingRequest{
		ScopeID:       scopeID,
		WindowStart:   windowStart,
		WindowEnd:     windowEnd,
		EvaluatedAt:   evaluatedAt,
		Applicability: applicability,
		Population:    population,
		Assertions:    request.Assertions,
		Graph:         graph,
	})
	if err != nil {
		return result, fmt.Errorf("compliance report: grade assertions: %w", err)
	}
	frameworkAssessments, err := evidence.GradeFrameworkControls(ctx, evidence.FrameworkGradingRequest{
		ScopeID:              scopeID,
		EvaluatedAt:          evaluatedAt,
		Frameworks:           request.Frameworks,
		Crosswalks:           request.Crosswalks,
		Assertions:           request.Assertions,
		AssertionAssessments: assertionAssessments,
	})
	if err != nil {
		return result, fmt.Errorf("compliance report: grade frameworks: %w", err)
	}
	scopeBytes, err := compliancev1.MarshalCanonical(request.Scope)
	if err != nil {
		return result, fmt.Errorf("compliance report: canonicalize assessment scope: %w", err)
	}
	scopeDigest := sha256.Sum256(scopeBytes)
	diagnostics := append([]*compliancev1.AssessmentDiagnostic(nil), request.Diagnostics...)
	diagnostics = append(diagnostics, unavailableAssessmentContextDiagnostics(request.Scope)...)
	analysis, err := evidence.BuildComplianceAnalysis(ctx, evidence.AnalysisRequest{
		ScopeID:               scopeID,
		AssessmentScopeSHA256: hex.EncodeToString(scopeDigest[:]),
		WindowStart:           windowStart,
		WindowEnd:             windowEnd,
		EvaluatedAt:           evaluatedAt,
		Graph:                 graph,
		Assertions:            request.Assertions,
		Frameworks:            request.Frameworks,
		Crosswalks:            request.Crosswalks,
		AssertionAssessments:  assertionAssessments,
		FrameworkAssessments:  frameworkAssessments,
		Diagnostics:           diagnostics,
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
