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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
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

func (m *campaignMemoryFileService) Resolve(relPath string) string                { return relPath }
func (m *campaignMemoryFileService) Remove(context.Context, string) error         { return nil }
func (m *campaignMemoryFileService) RemoveAll(context.Context, string) error      { return nil }
func (m *campaignMemoryFileService) Rename(context.Context, string, string) error { return nil }
func (m *campaignMemoryFileService) ReadDir(_ context.Context, relPath string) ([]os.DirEntry, error) {
	prefix := relPath + string(filepath.Separator)
	seen := make(map[string]struct{})
	var entries []os.DirEntry
	for path := range m.files {
		if !strings.HasPrefix(path, prefix) {
			continue
		}
		remainder := strings.TrimPrefix(path, prefix)
		parts := strings.Split(remainder, string(filepath.Separator))
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		if _, ok := seen[parts[0]]; ok {
			continue
		}
		seen[parts[0]] = struct{}{}
		entries = append(entries, campaignMemoryDirEntry{name: parts[0], isDir: len(parts) > 1})
	}
	if len(entries) == 0 {
		return nil, fs.ErrNotExist
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

func TestCampaignImporterLoadsAssignmentResultEvidence(t *testing.T) {
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

	importer := NewCampaignImporter(files, "run-1")
	node, err := importer.ImportAssignmentResult(context.Background(), "assign-1")
	require.NoError(t, err)
	assert.Equal(t, "run-1", node.RunID)
	assert.Equal(t, "g8e.eval.v1.EvaluationAssignmentResult", node.SchemaRef)
	_, contentDigest, ok := complianceevidence.ParseContentAddress(node.ArtifactID)
	require.True(t, ok)
	assert.Equal(t, contentDigest, node.SHA256)
	assert.NotEqual(t, digest, node.SHA256)
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
