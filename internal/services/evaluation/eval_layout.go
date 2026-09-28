// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"
	"path/filepath"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

// evalLayout addresses campaign and run directories. The active layout is
// .g8e/data/eval. The archive mirrors it beneath .g8e/data/eval/archive, so an
// archived campaign or run keeps its internal structure and stays readable in
// place. Campaigns and runs archive independently: an archived run can belong
// to a campaign that is still active, so the two roots are separate.
type evalLayout struct {
	campaignsRoot string
	runsRoot      string
}

func evalRoot() string {
	return filepath.Join(constants.DataDirname, constants.EvaluationDirname)
}

func archiveRoot() string {
	return filepath.Join(evalRoot(), constants.EvaluationArchiveDirname)
}

func activeEvalLayout() evalLayout {
	return evalLayout{
		campaignsRoot: filepath.Join(evalRoot(), constants.EvaluationCampaignsDirname),
		runsRoot:      filepath.Join(evalRoot(), constants.EvaluationRunsDirname),
	}
}

func archivedEvalLayout() evalLayout {
	return evalLayout{
		campaignsRoot: filepath.Join(archiveRoot(), constants.EvaluationCampaignsDirname),
		runsRoot:      filepath.Join(archiveRoot(), constants.EvaluationRunsDirname),
	}
}

// archivedRunLayout addresses an archived run whose campaign is still active.
func archivedRunLayout() evalLayout {
	return evalLayout{
		campaignsRoot: activeEvalLayout().campaignsRoot,
		runsRoot:      archivedEvalLayout().runsRoot,
	}
}

func (l evalLayout) campaignsRootDir() string {
	return l.campaignsRoot
}

func (l evalLayout) campaignDir(campaignID string) string {
	return filepath.Join(l.campaignsRootDir(), campaignID)
}

func (l evalLayout) runsRootDir() string {
	return l.runsRoot
}

func (l evalLayout) runDir(runID string) string {
	return filepath.Join(l.runsRootDir(), runID)
}

func (l evalLayout) campaignSpecPath(campaignID string) string {
	return filepath.Join(l.campaignDir(campaignID), constants.EvaluationCampaignSpecFilename)
}

func (l evalLayout) heterogeneousStackSetPath(campaignID string) string {
	return filepath.Join(l.campaignDir(campaignID), constants.EvaluationHeterogeneousStackSetFilename)
}

func (l evalLayout) scenarioCatalogPath(campaignID string) string {
	return filepath.Join(l.campaignDir(campaignID), constants.EvaluationScenarioCatalogFilename)
}

func (l evalLayout) scenarioArtifactPath(campaignID string, reference *compliancev1.ComplianceEvidenceReference) (string, error) {
	if reference == nil {
		return "", fmt.Errorf("%w: frozen scenario artifact reference is missing", constants.ErrEvidenceArtifactMalformed)
	}
	_, digest, ok := complianceevidence.ParseContentAddress(reference.GetArtifactId())
	if !ok || digest != reference.GetSha256() {
		return "", fmt.Errorf("%w: frozen scenario artifact reference is invalid", constants.ErrEvidenceArtifactMalformed)
	}
	return filepath.Join(l.campaignDir(campaignID), constants.EvaluationScenarioArtifactsDirname, digest+constants.FileExtJSON), nil
}

func (l evalLayout) assignmentsDir(runID string) string {
	return filepath.Join(l.runDir(runID), constants.EvaluationAssignmentsDirname)
}

func (l evalLayout) assignmentPath(runID, assignmentID string) string {
	return filepath.Join(l.assignmentsDir(runID), assignmentID+constants.FileExtJSON)
}

func (l evalLayout) assignmentResultPath(runID, assignmentID string) string {
	return filepath.Join(l.assignmentsDir(runID), assignmentID+"-result"+constants.FileExtJSON)
}

func (l evalLayout) assignmentTracePath(runID, assignmentID string) string {
	return filepath.Join(l.assignmentsDir(runID), assignmentID+"-trace"+constants.FileExtJSON)
}

func (l evalLayout) assignmentFormationRunPath(runID, assignmentID string) string {
	return filepath.Join(l.assignmentsDir(runID), assignmentID+"-formation-run"+constants.FileExtJSON)
}

func (l evalLayout) runStatePath(runID string) string {
	return filepath.Join(l.runDir(runID), constants.EvaluationRunStateFilename)
}

func (l evalLayout) campaignVerificationPath(runID string) string {
	return filepath.Join(l.runDir(runID), constants.CampaignVerificationFilename)
}

func (l evalLayout) runLeasePath(runID string) string {
	return filepath.Join(l.runDir(runID), constants.EvaluationRunLeaseFilename)
}
