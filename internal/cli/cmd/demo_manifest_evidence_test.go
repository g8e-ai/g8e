// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancecatalog "github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

const (
	manifestPhase2AfterFix = "PHASE2: AFTER FIX"
	manifestPhase2Issue    = "PHASE2: ISSUE: v2.1.3 demo manifests not created or persisted as typed evidence"
)

// writeDemoProvenanceFixture creates a minimal demo directory tree with the
// files buildDemoManifest hashes for provenance. It returns the temp demo dir.
func writeDemoProvenanceFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, constants.DemosDoctrineDir), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, constants.DemosTargetDataDir), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "config"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, constants.DemosComposeFile), []byte("compose-content"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, constants.DemosDoctrineDir, "doctrine.json"), []byte("doctrine-content"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, constants.DemosTargetDataDir, "data.json"), []byte("target-data-content"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config", "gateway.yml"), []byte("gateway-config"), 0o644))
	return dir
}

func expectedProvenanceHashes(t *testing.T, demoDir string) map[string]string {
	t.Helper()
	files := []string{
		constants.DemosComposeFile,
		filepath.Join(constants.DemosDoctrineDir, "doctrine.json"),
		filepath.Join(constants.DemosTargetDataDir, "data.json"),
		filepath.Join("config", "gateway.yml"),
	}
	hashes := make(map[string]string, len(files))
	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(demoDir, rel))
		require.NoError(t, err)
		hashes[rel] = sha256Hex(data)
	}
	return hashes
}

func TestBuildDemoManifest_ProducesTypedValidatedManifest(t *testing.T) {
	tests := []struct {
		name      string
		demoID    string
		scopeID   string
		prefix    string
		wantCount int
	}{
		{name: "fedramp binds all four fedramp definitions", demoID: constants.DemosOrgFedRAMP, scopeID: "fedramp-demo-scope", prefix: "fedramp-", wantCount: 4},
		{name: "dhs binds all five dhs definitions", demoID: constants.DemosOrgDHS, scopeID: "dhs-demo-scope", prefix: "dhs-", wantCount: 5},
		{name: "finance binds the finance definition", demoID: constants.DemosOrgFinance, scopeID: "finance-demo-scope", prefix: "finance-", wantCount: 1},
		{name: "healthcare binds all four healthcare definitions", demoID: constants.DemosOrgHealthcare, scopeID: "healthcare-demo-scope", prefix: "healthcare-", wantCount: 4},
	}

	generatedAt := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)
	runID := "test-run-20260901T120000Z"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			demoDir := writeDemoProvenanceFixture(t)

			manifest, err := buildDemoManifest(tt.demoID, tt.scopeID, runID, generatedAt, demoDir)
			require.NoError(t, err, manifestPhase2AfterFix)

			assert.IsType(t, &compliancev1.DemoManifest{}, manifest, manifestPhase2AfterFix)
			assert.Equal(t, tt.demoID, manifest.GetDemoId(), manifestPhase2AfterFix)
			assert.Equal(t, tt.scopeID, manifest.GetScopeId(), manifestPhase2AfterFix)
			assert.Equal(t, runID, manifest.GetRunId(), manifestPhase2AfterFix)
			assert.NotNil(t, manifest.GetGeneratedAt(), manifestPhase2AfterFix)
			assert.True(t, manifest.GetGeneratedAt().AsTime().Equal(generatedAt), manifestPhase2AfterFix)

			require.Len(t, manifest.GetScenarioDefinitionRefs(), tt.wantCount, manifestPhase2AfterFix)
			for _, ref := range manifest.GetScenarioDefinitionRefs() {
				assert.True(t, stringsHasPrefix(ref.GetId(), tt.prefix), "scenario %s does not match org prefix %s", ref.GetId(), tt.prefix)
				assert.Equal(t, "1.0.0", ref.GetVersion(), manifestPhase2AfterFix)
			}

			assert.NotEmpty(t, manifest.GetProvenanceHashes(), manifestPhase2AfterFix)
			expected := expectedProvenanceHashes(t, demoDir)
			for _, digest := range manifest.GetProvenanceHashes() {
				want, ok := expected[digest.GetName()]
				require.True(t, ok, "unexpected provenance file %s", digest.GetName())
				assert.Equal(t, want, digest.GetSha256(), "provenance hash mismatch for %s", digest.GetName())
			}

			assert.NotEmpty(t, manifest.GetRequiredEnvironment(), manifestPhase2AfterFix)
			assert.NotEmpty(t, manifest.GetSupportedLanes(), manifestPhase2AfterFix)
			assert.NotEmpty(t, manifest.GetFrameworkControlRefs(), manifestPhase2AfterFix)

			assertions, frameworks, _, err := compliancecatalog.LoadCanonicalCatalogs()
			require.NoError(t, err)
			scenarios, err := compliancecatalog.LoadDemoScenarioCatalog(assertions, frameworks)
			require.NoError(t, err)
			var definitions []*compliancev1.DemoScenarioDefinition
			for _, ref := range manifest.GetScenarioDefinitionRefs() {
				def := compliancecatalog.FindDemoScenarioDefinition(scenarios, ref.GetId(), ref.GetVersion())
				require.NotNil(t, def)
				definitions = append(definitions, def)
			}
			assert.NoError(t, compliancecatalog.ValidateDemoManifest(manifest, definitions, frameworks), manifestPhase2AfterFix)
		})
	}
}

func TestBuildDemoManifest_FailsClosedOnMissingProvenanceSubdir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, constants.DemosDoctrineDir), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, constants.DemosTargetDataDir), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, constants.DemosComposeFile), []byte("compose"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, constants.DemosDoctrineDir, "doctrine.json"), []byte("doctrine"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, constants.DemosTargetDataDir, "data.json"), []byte("data"), 0o644))

	_, err := buildDemoManifest(constants.DemosOrgFedRAMP, "fedramp-demo-scope", "run-1", time.Now().UTC(), dir)
	assert.Error(t, err, manifestPhase2AfterFix)
}

func TestBuildDemoManifest_FailsClosedOnUnknownOrg(t *testing.T) {
	demoDir := writeDemoProvenanceFixture(t)
	_, err := buildDemoManifest("unknown-org", "unknown-scope", "run-1", time.Now().UTC(), demoDir)
	assert.Error(t, err, manifestPhase2AfterFix)
}

func TestBuildDemoManifest_FrameworkControlRefsAreUnionOfDefinitions(t *testing.T) {
	demoDir := writeDemoProvenanceFixture(t)
	manifest, err := buildDemoManifest(constants.DemosOrgFedRAMP, "fedramp-demo-scope", "run-1", time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC), demoDir)
	require.NoError(t, err)

	assertions, frameworks, _, err := compliancecatalog.LoadCanonicalCatalogs()
	require.NoError(t, err)
	scenarios, err := compliancecatalog.LoadDemoScenarioCatalog(assertions, frameworks)
	require.NoError(t, err)

	expected := make(map[string]struct{})
	for _, ref := range manifest.GetScenarioDefinitionRefs() {
		def := compliancecatalog.FindDemoScenarioDefinition(scenarios, ref.GetId(), ref.GetVersion())
		require.NotNil(t, def)
		for _, fcRef := range def.GetFrameworkControlRefs() {
			key := fcRef.GetFrameworkRef().GetId() + ":" + fcRef.GetFrameworkRef().GetVersion() + ":" + fcRef.GetControlId()
			expected[key] = struct{}{}
		}
	}

	actual := make(map[string]struct{}, len(manifest.GetFrameworkControlRefs()))
	for _, fcRef := range manifest.GetFrameworkControlRefs() {
		key := fcRef.GetFrameworkRef().GetId() + ":" + fcRef.GetFrameworkRef().GetVersion() + ":" + fcRef.GetControlId()
		actual[key] = struct{}{}
	}
	assert.Equal(t, expected, actual, manifestPhase2AfterFix)
}

func TestPersistDemoManifest_WritesCanonicalProtojsonToRunDir(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	ctx := context.Background()

	manifest := &compliancev1.DemoManifest{
		DemoId:      constants.DemosOrgFedRAMP,
		DemoVersion: "1.0.0",
		RunId:       "fedramp-run-20260901T120000Z",
		ScopeId:     "fedramp-demo-scope",
		GeneratedAt: timestamppb.New(time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)),
		ScenarioDefinitionRefs: []*compliancev1.VersionedReference{
			{Id: "fedramp-provision", Version: "1.0.0"},
			{Id: "fedramp-deny", Version: "1.0.0"},
			{Id: "fedramp-revert", Version: "1.0.0"},
			{Id: "fedramp-evidence-block", Version: "1.0.0"},
		},
		ProvenanceHashes: []*compliancev1.NamedDigest{
			{Name: "compose.yml", Sha256: "abc123"},
		},
		RequiredEnvironment:  []string{"docker", "g8e-binary"},
		FrameworkControlRefs: []*compliancev1.FrameworkControlReference{},
		SupportedLanes:       []string{"automated"},
	}

	require.NoError(t, persistDemoManifest(ctx, fileSvc, manifest), manifestPhase2AfterFix)

	relPath := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.DemoEvidenceDirname, manifest.GetRunId(), constants.DemoRunManifestFilename)
	exists, err := fileSvc.FileExists(ctx, relPath)
	require.NoError(t, err)
	assert.True(t, exists, manifestPhase2AfterFix)

	persisted, err := fileSvc.ReadFile(ctx, relPath)
	require.NoError(t, err)

	encoded, err := compliancev1.MarshalCanonical(manifest)
	require.NoError(t, err)
	assert.Equal(t, string(encoded), string(persisted), manifestPhase2AfterFix)

	var decoded compliancev1.DemoManifest
	require.NoError(t, compliancev1.UnmarshalCanonical(persisted, &decoded))
	assert.True(t, proto.Equal(manifest, &decoded), manifestPhase2AfterFix)
}

func TestPersistDemoManifest_SkipsNilOrEmptyRunID(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	ctx := context.Background()

	assert.NoError(t, persistDemoManifest(ctx, fileSvc, nil))

	empty := &compliancev1.DemoManifest{DemoId: "fedramp"}
	assert.NoError(t, persistDemoManifest(ctx, fileSvc, empty))
}

func TestBuildDemoManifest_ScenarioDefinitionRefsAreSortedAndDeduplicated(t *testing.T) {
	demoDir := writeDemoProvenanceFixture(t)
	manifest, err := buildDemoManifest(constants.DemosOrgFedRAMP, "fedramp-demo-scope", "run-1", time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC), demoDir)
	require.NoError(t, err)

	ids := make([]string, len(manifest.GetScenarioDefinitionRefs()))
	for i, ref := range manifest.GetScenarioDefinitionRefs() {
		ids[i] = ref.GetId()
	}
	sorted := make([]string, len(ids))
	copy(sorted, ids)
	sort.Strings(sorted)
	assert.Equal(t, sorted, ids, manifestPhase2AfterFix)

	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		_, dup := seen[id]
		assert.False(t, dup, "duplicate scenario definition ref %s", id)
		seen[id] = struct{}{}
	}
}

func TestBuildDemoManifest_ProvenanceHashesAreSortedByName(t *testing.T) {
	demoDir := writeDemoProvenanceFixture(t)
	manifest, err := buildDemoManifest(constants.DemosOrgFedRAMP, "fedramp-demo-scope", "run-1", time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC), demoDir)
	require.NoError(t, err)

	names := make([]string, len(manifest.GetProvenanceHashes()))
	for i, d := range manifest.GetProvenanceHashes() {
		names[i] = d.GetName()
	}
	sorted := make([]string, len(names))
	copy(sorted, names)
	sort.Strings(sorted)
	assert.Equal(t, sorted, names, manifestPhase2AfterFix)
}

// stubFSForManifest is a minimal helper to verify persistDemoManifest wraps
// fileSvc errors with ErrDemoEvidencePersistFailed.
func TestPersistDemoManifest_WrapsFileSvcErrors(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	ctx := context.Background()

	manifest := &compliancev1.DemoManifest{
		DemoId:      "fedramp",
		DemoVersion: "1.0.0",
		RunId:       "fedramp-run-test",
		ScopeId:     "fedramp-demo-scope",
		GeneratedAt: timestamppb.New(time.Now().UTC()),
		ScenarioDefinitionRefs: []*compliancev1.VersionedReference{
			{Id: "fedramp-deny", Version: "1.0.0"},
		},
		ProvenanceHashes: []*compliancev1.NamedDigest{
			{Name: "compose.yml", Sha256: "abc123def456"},
		},
		RequiredEnvironment:  []string{"docker"},
		FrameworkControlRefs: []*compliancev1.FrameworkControlReference{},
		SupportedLanes:       []string{"automated"},
	}

	// Place a file where the run directory should be so MkdirAll fails
	// because a non-directory file blocks directory creation.
	runDir := filepath.Join(fileSvc.Resolve(""), constants.DataDirname, constants.ComplianceDirname, constants.DemoEvidenceDirname, manifest.GetRunId())
	require.NoError(t, os.MkdirAll(filepath.Dir(runDir), 0o755))
	require.NoError(t, os.WriteFile(runDir, []byte("blocks-mkdir"), 0o644))

	err := persistDemoManifest(ctx, fileSvc, manifest)
	assert.Error(t, err, manifestPhase2AfterFix)
}

// stringsHasPrefix is a small helper to avoid importing strings just for one call.
func stringsHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// sha256Hex computes the hex-encoded SHA-256 of data. Defined here to avoid
// importing crypto/sha in the test file header; the implementation file uses
// the same logic.
func sha256Hex(data []byte) string {
	return computeSHA256Hex(data)
}

// compile-time assertion that fs is imported (used by newCmdTestEnv).
var _ fs.RuntimeFileService
