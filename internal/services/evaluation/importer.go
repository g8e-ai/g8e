// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type EvidenceImporter struct {
	reader complianceevidence.ArtifactReader
	runID  string
	now    func() time.Time
}

func NewEvidenceImporter(reader complianceevidence.ArtifactReader, runID string, now func() time.Time) *EvidenceImporter {
	if now == nil {
		now = time.Now
	}
	return &EvidenceImporter{reader: reader, runID: runID, now: now}
}

func (i *EvidenceImporter) SourceID() string {
	return constants.EvaluationSourceKindNative
}

// RunID returns the run ID this importer is bound to.
func (i *EvidenceImporter) RunID() string {
	if i == nil {
		return ""
	}
	return i.runID
}

func (i *EvidenceImporter) Import(ctx context.Context) ([]complianceevidence.EvidenceNode, error) {
	if i == nil || i.reader == nil || !complianceevidence.ValidPathElement(i.runID) || i.now == nil {
		return nil, fmt.Errorf("%w: native evaluation importer is incomplete", constants.ErrInvalidEvidenceGraph)
	}
	verification, err := NewVerifier(i.reader, NewRegistry(), i.now).Verify(ctx, i.runID)
	if err != nil {
		return nil, err
	}
	if !verification.GetValid() {
		return nil, fmt.Errorf("%w: native evaluation run %s has %d verification failures", constants.ErrEvalRunVerificationFailed, i.runID, len(verification.GetFailures()))
	}
	reportBody, err := i.reader.ReadFile(ctx, filepath.Join(evaluationRunDir(i.runID), constants.EvaluationReportFilename))
	if err != nil {
		return nil, err
	}
	report := &evalv1.EvaluationReport{}
	if err := evalv1.UnmarshalCanonical(reportBody, report); err != nil {
		return nil, fmt.Errorf("%w: native evaluation report: %v", constants.ErrEvidenceArtifactMalformed, err)
	}
	scopeID := constants.EvalScopePrefix + report.GetRun().GetSuiteRef().GetId()
	nodes := make([]complianceevidence.EvidenceNode, 0, len(report.GetEvidenceRefs())+1)
	reportID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeEvalManifest, reportBody)
	_, reportDigest, _ := complianceevidence.ParseContentAddress(reportID)
	nodes = append(nodes, complianceevidence.EvidenceNode{ArtifactID: reportID, ArtifactType: complianceevidence.ArtifactTypeEvalManifest, SHA256: reportDigest, MediaType: constants.MediaTypeJSON, SchemaRef: "g8e.eval.v1.EvaluationReport", ProducerIdentity: GraderID, ProducedAt: report.GetRun().GetCompletedAt().AsTime(), ScopeID: scopeID, RunID: i.runID, VerificationStatus: complianceevidence.VerificationStatusVerified, VerifierID: constants.EvalRunVerifierID, VerifierVersion: constants.EvalRunVerifierVersion, VerifiedAt: verification.GetVerifiedAt().AsTime(), BundlePath: constants.EvaluationReportFilename, CanonicalBytes: reportBody})
	seen := make(map[string]bool)
	for _, reference := range report.GetEvidenceRefs() {
		if reference == nil || seen[reference.GetArtifactId()] {
			continue
		}
		seen[reference.GetArtifactId()] = true
		artifactType, digest, ok := complianceevidence.ParseContentAddress(reference.GetArtifactId())
		if !ok {
			return nil, fmt.Errorf("%w: %s", constants.ErrEvidenceArtifactMalformed, reference.GetArtifactId())
		}
		body, err := i.reader.ReadFile(ctx, filepath.Join(evaluationEvidenceDir(i.runID), digest+constants.FileExtJSON))
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, complianceevidence.EvidenceNode{ArtifactID: reference.GetArtifactId(), ArtifactType: artifactType, SHA256: digest, MediaType: reference.GetMediaType(), SchemaRef: reference.GetSchemaRef(), ProducerIdentity: GraderID, ProducedAt: report.GetRun().GetCompletedAt().AsTime(), ScopeID: scopeID, RunID: reference.GetRunId(), AttemptID: reference.GetAttemptId(), ScenarioID: reference.GetScenarioId(), TransactionID: reference.GetTransactionId(), VerificationStatus: complianceevidence.VerificationStatusVerified, VerifierID: constants.EvalRunVerifierID, VerifierVersion: constants.EvalRunVerifierVersion, VerifiedAt: verification.GetVerifiedAt().AsTime(), BundlePath: filepath.Join(constants.EvaluationEvidenceDirname, digest+constants.FileExtJSON), CanonicalBytes: body})
	}
	return nodes, nil
}
