// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package report

import (
	"context"
	"fmt"
	"time"

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
