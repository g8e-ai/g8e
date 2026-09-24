// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type campaignMemoryFileService struct {
	files map[string][]byte
}

func newCampaignMemoryFileService() *campaignMemoryFileService {
	return &campaignMemoryFileService{files: make(map[string][]byte)}
}

func (m *campaignMemoryFileService) MkdirAll(_ context.Context, relPath string, _ os.FileMode) error {
	m.files[relPath] = nil
	return nil
}

func (m *campaignMemoryFileService) CreateRuntimeTree(context.Context) error { return nil }

func (m *campaignMemoryFileService) ReadFile(_ context.Context, relPath string) ([]byte, error) {
	body, ok := m.files[relPath]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), body...), nil
}

func (m *campaignMemoryFileService) FileExists(_ context.Context, relPath string) (bool, error) {
	_, ok := m.files[relPath]
	return ok, nil
}

func (m *campaignMemoryFileService) Stat(context.Context, string) (os.FileInfo, error) {
	return nil, nil
}
func (m *campaignMemoryFileService) Lstat(context.Context, string) (os.FileInfo, error) {
	return nil, nil
}

func (m *campaignMemoryFileService) WriteFile(_ context.Context, relPath string, data []byte, _ os.FileMode) error {
	m.files[relPath] = append([]byte(nil), data...)
	return nil
}

func (m *campaignMemoryFileService) OpenForAppend(context.Context, string, os.FileMode) (*os.File, error) {
	return nil, os.ErrInvalid
}

func (m *campaignMemoryFileService) OpenForRead(context.Context, string) (*os.File, error) {
	return nil, os.ErrInvalid
}

func (m *campaignMemoryFileService) Resolve(relPath string) string { return relPath }
func (m *campaignMemoryFileService) Remove(_ context.Context, relPath string) error {
	delete(m.files, relPath)
	return nil
}

func (m *campaignMemoryFileService) RemoveAll(_ context.Context, relPath string) error {
	if relPath == "" {
		return nil
	}
	prefix := relPath + string(filepath.Separator)
	for path := range m.files {
		if path == relPath || strings.HasPrefix(path, prefix) {
			delete(m.files, path)
		}
	}
	return nil
}
func (m *campaignMemoryFileService) Rename(context.Context, string, string) error { return nil }
func (m *campaignMemoryFileService) ReadDir(_ context.Context, relPath string) ([]os.DirEntry, error) {
	prefix := relPath + string(filepath.Separator)
	seen := make(map[string]bool)
	for path := range m.files {
		if !strings.HasPrefix(path, prefix) {
			continue
		}
		remainder := strings.TrimPrefix(path, prefix)
		parts := strings.Split(remainder, string(filepath.Separator))
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		if len(parts) > 1 {
			seen[parts[0]] = true
		} else if _, ok := seen[parts[0]]; !ok {
			seen[parts[0]] = false
		}
	}
	if len(seen) == 0 {
		return nil, fs.ErrNotExist
	}
	entries := make([]os.DirEntry, 0, len(seen))
	for name, isDir := range seen {
		entries = append(entries, campaignMemoryDirEntry{name: name, isDir: isDir})
	}
	return entries, nil
}

type campaignMemoryDirEntry struct {
	name  string
	isDir bool
}

func (e campaignMemoryDirEntry) Name() string               { return e.name }
func (e campaignMemoryDirEntry) IsDir() bool                { return e.isDir }
func (e campaignMemoryDirEntry) Type() fs.FileMode          { return 0 }
func (e campaignMemoryDirEntry) Info() (fs.FileInfo, error) { return nil, fs.ErrInvalid }
func (m *campaignMemoryFileService) EnforceDirPermissions(context.Context, string, os.FileMode) error {
	return nil
}
func (m *campaignMemoryFileService) EnforceFilePermissions(context.Context, string, os.FileMode) error {
	return nil
}
func (m *campaignMemoryFileService) Rel(string) (string, error)        { return "", nil }
func (m *campaignMemoryFileService) RelFromAbs(string) (string, error) { return "", nil }

func TestStoreSaveAndLoadCampaignSpec(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	spec := testCampaignSpec()
	digest, err := ComputeCampaignSpecDigest(spec)
	require.NoError(t, err)
	spec.CampaignDigest = digest

	require.NoError(t, store.SaveCampaignSpec(context.Background(), spec))
	loaded, err := store.LoadCampaignSpec(context.Background(), spec.GetCampaignId())
	require.NoError(t, err)
	assert.Equal(t, spec.GetCampaignId(), loaded.GetCampaignId())
	assert.Equal(t, digest, loaded.GetCampaignDigest())
}

func TestStoreSaveAndLoadScenarioCatalog(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	catalog := testScenarioCatalog()
	digest, err := ComputeScenarioCatalogDigest(catalog)
	require.NoError(t, err)
	catalog.CatalogDigest = digest

	require.NoError(t, store.SaveScenarioCatalog(context.Background(), "phase1a-smoke", catalog))
	loaded, err := store.LoadScenarioCatalog(context.Background(), "phase1a-smoke")
	require.NoError(t, err)
	assert.Equal(t, digest, loaded.GetCatalogDigest())
	assert.Len(t, loaded.GetScenarios(), 2)
}

func TestStoreSaveAndLoadAssignmentResult(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	result := &evalv1.EvaluationAssignmentResult{
		SchemaVersion:   CampaignSchemaVersion,
		AssignmentId:    "assign-1",
		RunId:           "run-1",
		CampaignId:      "phase1a-smoke",
		Lane:            evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
		CompletedAt:     timestamppb.Now(),
	}
	digest, err := ComputeAssignmentResultDigest(result)
	require.NoError(t, err)
	result.ResultDigest = digest

	require.NoError(t, store.SaveAssignmentResult(context.Background(), result))
	loaded, err := store.LoadAssignmentResult(context.Background(), "run-1", "assign-1")
	require.NoError(t, err)
	assert.Equal(t, digest, loaded.GetResultDigest())
}

func TestCampaignImporterPreservesBoundIncompletePopulation(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	controller := NewCampaignController(store, nil, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" })
	req := testCampaignInitRequest(t)
	_, err := controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	count, err := controller.ScheduleHomogeneousRun(context.Background(), req.RunID)
	require.NoError(t, err)

	importer := NewCampaignImporter(files, req.RunID, CampaignVerificationPolicy{VerifierReleaseVersion: constants.EvaluationSourceVersion, ProviderObservation: ProviderObservationPolicyInterim, ModelProvenance: ModelProvenancePolicyInterim, AssessmentTime: func() time.Time { return time.Unix(1_700_000_100, 0).UTC() }})
	nodes, err := importer.Import(context.Background())

	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, constants.EvaluationSourceKindCampaign, importer.SourceID())
	assert.Equal(t, req.RunID, importer.RunID())
	assert.Equal(t, complianceevidence.ArtifactTypeEvalManifest, nodes[0].ArtifactType)
	report := &evalv1.EvaluationVerificationReport{}
	require.NoError(t, evalv1.UnmarshalCanonical(nodes[0].CanonicalBytes, report))
	assert.Equal(t, uint32(count), report.GetExpectedAssignmentCount())
	assert.Zero(t, report.GetVerifiedAssignmentCount())
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, report.GetStatus())
	assert.Len(t, report.GetVerifiedPopulationDigest(), 64)
	require.NotEmpty(t, nodes[0].Diagnostics)
	assert.Contains(t, diagnosticCodes(nodes[0].Diagnostics), "campaign_population_incomplete")
	assert.Contains(t, diagnosticCodes(nodes[0].Diagnostics), "campaign_native_assertion_unmapped")
	for _, diagnostic := range nodes[0].Diagnostics {
		assert.Equal(t, req.RunID, diagnostic.GetSubject().GetRunId())
	}
}

func diagnosticCodes(diagnostics []*compliancev1.AssessmentDiagnostic) []string {
	codes := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		if diagnostic != nil {
			codes = append(codes, diagnostic.GetCode())
		}
	}
	return codes
}

func TestCampaignVerificationDiagnosticsAppendsFailureReasonsBeyondInitialCapacity(t *testing.T) {
	diagnostics := campaignVerificationDiagnostics(&evalv1.EvaluationVerificationReport{
		RunId:                   "run-1",
		ExpectedAssignmentCount: 2,
		VerifiedAssignmentCount: 1,
		FailureReasons:          []string{"first failure", "second failure"},
	})

	assert.Equal(t, []string{
		"campaign_native_assertion_unmapped",
		"campaign_population_incomplete",
		"campaign_verification_failure",
		"campaign_verification_failure",
	}, diagnosticCodes(diagnostics))
	assert.Equal(t, "first failure", diagnostics[2].GetMessage())
	assert.Equal(t, "second failure", diagnostics[3].GetMessage())
}

func TestReviewedCampaignAssertionMappingsExcludeApplicationEvidence(t *testing.T) {
	mappings := reviewedCampaignAssertionMappings()
	require.NotEmpty(t, mappings)

	byEvidence := make(map[string]campaignAssertionMapping, len(mappings))
	for _, mapping := range mappings {
		require.NotEmpty(t, mapping.EvidenceKind)
		assert.NotEmpty(t, mapping.ClaimLimit)
		assert.NotEmpty(t, mapping.RequiredEvidenceTypes)
		byEvidence[mapping.EvidenceKind] = mapping
	}

	for _, evidenceKind := range []string{"campaign_verification", "deterministic_grade", "semantic_grade", "application_policy_outcome"} {
		mapping, ok := byEvidence[evidenceKind]
		require.True(t, ok, evidenceKind)
		assert.Empty(t, mapping.NativeAssertionRefs, evidenceKind)
	}

	governedActionMapping, ok := byEvidence["governed_action_binding"]
	require.True(t, ok)
	assertionRefs := make([]string, 0, len(governedActionMapping.NativeAssertionRefs))
	for _, reference := range governedActionMapping.NativeAssertionRefs {
		assertionRefs = append(assertionRefs, reference.GetId()+"@"+reference.GetVersion())
	}
	assert.ElementsMatch(t, []string{
		"G8E-AU-PERSIST-001@2.0.0",
		"G8E-AU-RECEIPT-001@2.0.0",
		"G8E-CM-STATE-001@2.0.0",
		"G8E-GOV-ALLOW-001@2.0.0",
		"G8E-GOV-BLOCK-001@2.0.0",
	}, assertionRefs)
}

func TestCampaignImporterRejectsTamperedPersistedScenarioBody(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	controller := NewCampaignController(store, nil, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" })
	req := testCampaignInitRequest(t)
	_, err := controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	_, err = controller.ScheduleHomogeneousRun(context.Background(), req.RunID)
	require.NoError(t, err)

	reference := req.Catalog.GetScenarios()[0].GetInputFixtureRef()
	artifactPath, err := scenarioArtifactPath(req.CampaignID, reference)
	require.NoError(t, err)
	require.NoError(t, files.WriteFile(context.Background(), artifactPath, []byte("{}"), constants.PermFileReadOnly))

	importer := NewCampaignImporter(files, req.RunID, CampaignVerificationPolicy{VerifierReleaseVersion: constants.EvaluationSourceVersion, ProviderObservation: ProviderObservationPolicyInterim, ModelProvenance: ModelProvenancePolicyInterim, AssessmentTime: func() time.Time { return time.Unix(1_700_000_100, 0).UTC() }})
	_, err = importer.Import(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
}

func TestStoreListCampaigns_ReturnsPersistedCampaignsWithRuns(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	specA := testCampaignSpec()
	digestA, err := ComputeCampaignSpecDigest(specA)
	require.NoError(t, err)
	specA.CampaignDigest = digestA
	require.NoError(t, store.SaveCampaignSpec(context.Background(), specA))

	specB := testCampaignSpec()
	specB.CampaignId = "phase2-full"
	digestB, err := ComputeCampaignSpecDigest(specB)
	require.NoError(t, err)
	specB.CampaignDigest = digestB
	require.NoError(t, store.SaveCampaignSpec(context.Background(), specB))

	require.NoError(t, store.SaveRun(context.Background(), &evalv1.EvaluationRun{
		SchemaVersion: CampaignSchemaVersion,
		RunId:         "run-a",
		CampaignBinding: &evalv1.ModelCampaignBinding{
			CampaignId: specA.GetCampaignId(),
		},
		StartedAt: timestamppb.Now(),
	}))
	require.NoError(t, store.SaveRun(context.Background(), &evalv1.EvaluationRun{
		SchemaVersion: CampaignSchemaVersion,
		RunId:         "run-b",
		CampaignBinding: &evalv1.ModelCampaignBinding{
			CampaignId: specB.GetCampaignId(),
		},
		StartedAt: timestamppb.Now(),
	}))

	campaigns, err := store.ListCampaigns(context.Background())
	require.NoError(t, err)
	require.Len(t, campaigns, 2)
	assert.Equal(t, "phase1a-smoke", campaigns[0].CampaignID)
	assert.Equal(t, 1, campaigns[0].ModelCount)
	assert.Equal(t, uint32(25), campaigns[0].ScenarioCount)
	assert.Equal(t, []string{"run-a"}, campaigns[0].RunIDs)
	assert.Equal(t, "phase2-full", campaigns[1].CampaignID)
	assert.Equal(t, []string{"run-b"}, campaigns[1].RunIDs)
}

func TestStoreListCampaigns_ReturnsEmptyWhenNoCampaignsExist(t *testing.T) {
	store := NewStore(newCampaignMemoryFileService())
	campaigns, err := store.ListCampaigns(context.Background())
	require.NoError(t, err)
	assert.Empty(t, campaigns)
}

func TestStoreListRunInventory_ClassifiesEveryPersistedCandidate(t *testing.T) {
	ctx := context.Background()
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	require.NoError(t, store.SaveReport(ctx, &evalv1.EvaluationReport{
		SchemaVersion: RegistryVersion,
		Run: &evalv1.EvaluationRun{
			SchemaVersion: RegistryVersion,
			RunId:         "native-run",
			SuiteRef:      &compliancev1.VersionedReference{Id: CoreExecutionBoundarySuiteID, Version: CoreExecutionBoundarySuiteVersion},
		},
	}))
	require.NoError(t, store.SaveRun(ctx, &evalv1.EvaluationRun{
		SchemaVersion: CampaignSchemaVersion,
		RunId:         "campaign-run",
		CampaignBinding: &evalv1.ModelCampaignBinding{
			CampaignId: "campaign-1",
		},
	}))
	require.NoError(t, store.SaveReport(ctx, &evalv1.EvaluationReport{
		SchemaVersion: RegistryVersion,
		Run: &evalv1.EvaluationRun{
			SchemaVersion: RegistryVersion,
			RunId:         "unsupported-run",
			SuiteRef:      &compliancev1.VersionedReference{Id: "unsupported-suite", Version: "1.0.0"},
		},
	}))
	require.NoError(t, files.WriteFile(ctx, filepath.Join(evaluationRunDir("incomplete-run"), constants.EvaluationVerificationFilename), []byte("{}"), constants.PermFileReadOnly))
	require.NoError(t, files.WriteFile(ctx, runStatePath("malformed-run"), []byte("{}"), constants.PermFileReadOnly))
	require.NoError(t, store.SaveReport(ctx, &evalv1.EvaluationReport{
		SchemaVersion: RegistryVersion,
		Run: &evalv1.EvaluationRun{
			SchemaVersion: RegistryVersion,
			RunId:         "ambiguous-run",
			SuiteRef:      &compliancev1.VersionedReference{Id: CoreExecutionBoundarySuiteID, Version: CoreExecutionBoundarySuiteVersion},
		},
	}))
	require.NoError(t, store.SaveRun(ctx, &evalv1.EvaluationRun{
		SchemaVersion: CampaignSchemaVersion,
		RunId:         "ambiguous-run",
		CampaignBinding: &evalv1.ModelCampaignBinding{
			CampaignId: "campaign-1",
		},
	}))

	inventory, err := store.ListRunInventory(ctx)
	require.NoError(t, err)
	require.Len(t, inventory, 6)
	assert.Equal(t, []RunInventoryEntry{
		{RunID: "ambiguous-run", Kind: RunKindMalformed, Reason: "native and campaign markers are both present"},
		{RunID: "campaign-run", Kind: RunKindCampaign},
		{RunID: "incomplete-run", Kind: RunKindIncomplete, Reason: "native and campaign markers are both missing"},
		{RunID: "malformed-run", Kind: RunKindMalformed, Reason: "campaign run record is malformed"},
		{RunID: "native-run", Kind: RunKindNative},
		{RunID: "unsupported-run", Kind: RunKindUnsupported, Reason: "native evaluation suite or schema is unsupported"},
	}, inventory)
}

func TestStoreInspectRun_ReturnsExplicitIncompleteDispositionForMissingSelection(t *testing.T) {
	store := NewStore(newCampaignMemoryFileService())
	entry, err := store.InspectRun(context.Background(), "missing-run")
	require.NoError(t, err)
	assert.Equal(t, RunInventoryEntry{RunID: "missing-run", Kind: RunKindIncomplete, Reason: "native and campaign markers are both missing"}, entry)
}
