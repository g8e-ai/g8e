// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

// stubProvenanceSourceFactory returns a ProvenanceSource that always yields
// the given artifacts, ignoring the project root argument. Used to inject
// hermetic provenance into the verify command without touching the filesystem.
func stubProvenanceSourceFactory(artifacts []evidence.ProvenanceArtifact) func(string) evidence.ProvenanceSource {
	return func(string) evidence.ProvenanceSource {
		return &stubProvenanceSource{artifacts: artifacts}
	}
}

type stubProvenanceSource struct {
	artifacts []evidence.ProvenanceArtifact
}

func (s *stubProvenanceSource) Artifacts(_ context.Context, _ string) ([]evidence.ProvenanceArtifact, error) {
	return append([]evidence.ProvenanceArtifact(nil), s.artifacts...), nil
}

// writeDemoProvenanceTree creates a minimal demos/<org>/ directory tree under
// a temp project root, with a compose file and provenance subdirectories so
// the directory-backed provenance source can enumerate real artifacts. Returns
// the project root.
func writeDemoProvenanceTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	demoDir := filepath.Join(root, constants.DemosDirname, constants.DemosOrgFedRAMP)
	require.NoError(t, os.MkdirAll(filepath.Join(demoDir, constants.DemosDoctrineDir), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(demoDir, constants.DemosTargetDataDir), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(demoDir, constants.DemoConfigDirname), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(demoDir, constants.DemosComposeFile), []byte("services: {}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(demoDir, constants.DemosDoctrineDir, "doctrine.json"), []byte("{}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(demoDir, constants.DemosTargetDataDir, "target.json"), []byte("{}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(demoDir, constants.DemoConfigDirname, "config.json"), []byte("{}"), 0o644))
	return root
}

// persistMinimalDemoRunFixture writes a canonical, correlated FedRAMP demo run
// (manifest + one failed scenario result with no receipt or state-observation
// refs) into the runtime file service. This is the simplest valid run: a
// failed scenario makes no evidence claims, so the verifier has nothing to
// reject. Returns the run ID.
func persistMinimalDemoRunFixture(t *testing.T, fileSvc fs.RuntimeFileService, projectRoot string) string {
	t.Helper()
	assertions, frameworks, _, err := catalog.LoadCanonicalCatalogs()
	require.NoError(t, err)
	scenarios, err := catalog.LoadDemoScenarioCatalog(assertions, frameworks)
	require.NoError(t, err)

	var definition *compliancev1.DemoScenarioDefinition
	var definitionRefs []*compliancev1.VersionedReference
	frameworkRefs := make(map[string]*compliancev1.FrameworkControlReference)
	for _, def := range scenarios.GetDefinitions() {
		if !strings.HasPrefix(def.GetScenarioId(), constants.DemosOrgFedRAMP+"-") {
			continue
		}
		definitionRefs = append(definitionRefs, &compliancev1.VersionedReference{
			Id: def.GetScenarioId(), Version: def.GetScenarioVersion(),
		})
		if definition == nil {
			definition = def
		}
		for _, fcRef := range def.GetFrameworkControlRefs() {
			key := fcRef.GetFrameworkRef().GetId() + ":" + fcRef.GetFrameworkRef().GetVersion() + ":" + fcRef.GetControlId()
			frameworkRefs[key] = fcRef
		}
	}
	require.NotNil(t, definition)
	sort.Slice(definitionRefs, func(i, j int) bool { return definitionRefs[i].GetId() < definitionRefs[j].GetId() })

	// Compute provenance hashes from the fixture directory.
	source := evidence.NewDemoDirectoryProvenanceSource(projectRoot)
	artifacts, err := source.Artifacts(context.Background(), constants.DemosOrgFedRAMP)
	require.NoError(t, err)
	provenanceHashes := make([]*compliancev1.NamedDigest, 0, len(artifacts))
	for _, artifact := range artifacts {
		digest := sha256.Sum256(artifact.Body)
		provenanceHashes = append(provenanceHashes, &compliancev1.NamedDigest{
			Name:   artifact.Name,
			Sha256: hex.EncodeToString(digest[:]),
		})
	}
	sort.Slice(provenanceHashes, func(i, j int) bool { return provenanceHashes[i].GetName() < provenanceHashes[j].GetName() })

	frameworkKeys := make([]string, 0, len(frameworkRefs))
	for key := range frameworkRefs {
		frameworkKeys = append(frameworkKeys, key)
	}
	sort.Strings(frameworkKeys)
	manifestFrameworkRefs := make([]*compliancev1.FrameworkControlReference, 0, len(frameworkKeys))
	for _, key := range frameworkKeys {
		manifestFrameworkRefs = append(manifestFrameworkRefs, frameworkRefs[key])
	}

	runID := "fedramp-run-fixture"
	generatedAt := time.Unix(1_700_000_000, 0).UTC()

	manifest := &compliancev1.DemoManifest{
		DemoId:                 constants.DemosOrgFedRAMP,
		DemoVersion:            constants.DemoVersion,
		RunId:                  runID,
		ScopeId:                constants.DemoScopeFedRAMP,
		GeneratedAt:            timestamppb.New(generatedAt),
		ScenarioDefinitionRefs: definitionRefs,
		ProvenanceHashes:       provenanceHashes,
		RequiredEnvironment:    []string{"docker", "g8e-binary"},
		FrameworkControlRefs:   manifestFrameworkRefs,
		SupportedLanes:         []string{"automated", "manual-notary"},
	}

	result := &compliancev1.DemoScenarioResult{
		ResultId:             runID + ":" + definition.GetScenarioId(),
		ScenarioRef:          &compliancev1.VersionedReference{Id: definition.GetScenarioId(), Version: definition.GetScenarioVersion()},
		DemoId:               constants.DemosOrgFedRAMP,
		ScopeId:              constants.DemoScopeFedRAMP,
		RunId:                runID,
		StartedAt:            timestamppb.New(generatedAt.Add(time.Second)),
		CompletedAt:          timestamppb.New(generatedAt.Add(2 * time.Second)),
		Status:               "failed",
		Failure:              "expected fixture failure",
		VerificationStatus:   "unverifiable",
		DisplayNumber:        definition.GetDisplayNumber(),
		Title:                definition.GetTitle(),
		AssertionRefs:        definition.GetAssertionRefs(),
		FrameworkControlRefs: definition.GetFrameworkControlRefs(),
		StepResults: []*compliancev1.DemoStepResult{{
			StepId:      "step-1",
			Operation:   "fixture",
			StartedAt:   timestamppb.New(generatedAt.Add(time.Second)),
			CompletedAt: timestamppb.New(generatedAt.Add(2 * time.Second)),
			Status:      "failed",
			Failure:     "expected fixture failure",
			Required:    true,
		}},
	}

	manifestBody, err := compliancev1.MarshalCanonical(manifest)
	require.NoError(t, err)
	resultBody, err := compliancev1.MarshalCanonical(result)
	require.NoError(t, err)

	runDir := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.DemoEvidenceDirname, runID)
	ctx := context.Background()
	require.NoError(t, fileSvc.MkdirAll(ctx, runDir, constants.PermDirStandard))
	require.NoError(t, fileSvc.WriteFile(ctx, filepath.Join(runDir, constants.DemoRunManifestFilename), manifestBody, constants.PermFileReadOnly))
	require.NoError(t, fileSvc.WriteFile(ctx, filepath.Join(runDir, constants.DemoRunResultsFilename), resultBody, constants.PermFileReadOnly))

	return runID
}

func TestComplianceDemoRunVerifyCmdWithConfig_AcceptsValidRun(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	projectRoot := writeDemoProvenanceTree(t)
	runID := persistMinimalDemoRunFixture(t, fileSvc, projectRoot)

	source := evidence.NewDemoDirectoryProvenanceSource(projectRoot)
	cmd := complianceDemoRunVerifyCmdWithConfig(
		fileSvcFactoryFor(fileSvc),
		func(string) evidence.ProvenanceSource { return source },
	)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{runID})
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, runID)
	assert.Contains(t, output, constants.DemoRunVerifierID)
	assert.Contains(t, output, `"valid":true`)
}

func TestComplianceDemoRunVerifyCmdWithConfig_RejectsMissingRunID(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	cmd := complianceDemoRunVerifyCmdWithConfig(
		fileSvcFactoryFor(fileSvc),
		stubProvenanceSourceFactory(nil),
	)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

func TestComplianceDemoRunVerifyCmdWithConfig_RejectsTooManyArgs(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	cmd := complianceDemoRunVerifyCmdWithConfig(
		fileSvcFactoryFor(fileSvc),
		stubProvenanceSourceFactory(nil),
	)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"run-1", "run-2"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

func TestComplianceDemoRunVerifyCmdWithConfig_NonexistentRunReportsFailures(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	cmd := complianceDemoRunVerifyCmdWithConfig(
		fileSvcFactoryFor(fileSvc),
		stubProvenanceSourceFactory(nil),
	)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"nonexistent-run"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrReportVerificationFailed)
	assert.Contains(t, buf.String(), "nonexistent-run")
}
