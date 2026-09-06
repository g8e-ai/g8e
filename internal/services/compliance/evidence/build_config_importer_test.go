package evidence

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

type buildConfigImporterFixture struct {
	reader  *memoryArtifactReader
	records []buildConfigAttestationRecord
	body    []byte
	binding BuildConfigImportBinding
}

func newBuildConfigImporterFixture(t *testing.T) *buildConfigImporterFixture {
	t.Helper()
	fixture := &buildConfigImporterFixture{
		reader: &memoryArtifactReader{files: map[string][]byte{}},
		records: []buildConfigAttestationRecord{
			{
				SchemaVersion:    constants.BuildAttestationSchemaVersion,
				AttestationType:  buildAttestationTypeBuild,
				ProducerIdentity: "build-system-1",
				ProducedAtUTC:    "2026-09-06T10:00:00Z",
				ScopeID:          "scope-1",
				RunID:            "run-1",
				BuildIdentity:    "build-1",
				SourceRevision:   "revision-1",
				ImageDigests:     []buildConfigNamedDigest{{Name: "gateway", SHA256: strings.Repeat("1", 64)}},
				ComponentInventory: []buildConfigComponent{
					{ComponentID: "gateway", ComponentType: "service", Version: "2.1.3", Digest: strings.Repeat("2", 64)},
				},
			},
			{
				SchemaVersion:       constants.BuildAttestationSchemaVersion,
				AttestationType:     buildAttestationTypeConfiguration,
				ProducerIdentity:    "build-system-1",
				ProducedAtUTC:       "2026-09-06T10:01:00Z",
				ScopeID:             "scope-1",
				RunID:               "run-1",
				BuildIdentity:       "build-1",
				SourceRevision:      "revision-1",
				ConfigurationHashes: []buildConfigNamedDigest{{Name: "gateway", SHA256: strings.Repeat("3", 64)}},
			},
		},
		binding: BuildConfigImportBinding{
			Path:             constants.BuildConfigAttestationsFilename,
			ScopeID:          "scope-1",
			RunID:            "run-1",
			BuildIdentity:    "build-1",
			SourceRevision:   "revision-1",
			ProducerIdentity: "build-system-1",
		},
	}
	fixture.replaceRecords(t)
	return fixture
}

func (f *buildConfigImporterFixture) replaceRecords(t *testing.T) {
	t.Helper()
	lines := make([][]byte, 0, len(f.records))
	for _, record := range f.records {
		line, err := json.Marshal(record)
		require.NoError(t, err)
		lines = append(lines, line)
	}
	f.body = append(bytes.Join(lines, []byte{'\n'}), '\n')
	f.reader.files[f.binding.Path] = f.body
	f.binding.Reference = ContentReferenceForBody(constants.BuildAttestationReferencePrefix, f.body)
}

func TestBuildConfigImporter_SourceID(t *testing.T) {
	fixture := newBuildConfigImporterFixture(t)
	assert.Equal(t, "build-config", NewBuildConfigImporter(fixture.reader, fixture.binding).SourceID())
}

func TestBuildConfigImporter_Import_LoadsCanonicalAttestations(t *testing.T) {
	fixture := newBuildConfigImporterFixture(t)
	nodes, err := NewBuildConfigImporter(fixture.reader, fixture.binding).Import(context.Background())
	require.NoError(t, err)
	require.Len(t, nodes, 2)

	build := nodes[0]
	assert.Equal(t, ContentReferenceForBody(constants.BuildAttestationReferencePrefix, build.CanonicalBytes), build.ArtifactID)
	assert.Equal(t, ArtifactTypeBuildAttestation, build.ArtifactType)
	assert.Equal(t, "g8e.evidence.BuildAttestation@"+constants.BuildAttestationSchemaVersion, build.SchemaRef)
	assert.Equal(t, fixture.binding.ProducerIdentity, build.ProducerIdentity)
	assert.Equal(t, fixture.binding.ScopeID, build.ScopeID)
	assert.Equal(t, fixture.binding.RunID, build.RunID)
	assert.Equal(t, VerificationStatusUnverified, build.VerificationStatus)
	assert.Equal(t, fixture.binding.Path, build.BundlePath)
	assert.Equal(t, time.Date(2026, 9, 6, 10, 0, 0, 0, time.UTC), build.ProducedAt)
	assert.Empty(t, build.References)

	configuration := nodes[1]
	assert.Equal(t, ContentReferenceForBody(constants.ConfigAttestationReferencePrefix, configuration.CanonicalBytes), configuration.ArtifactID)
	assert.Equal(t, ArtifactTypeConfigAttestation, configuration.ArtifactType)
	assert.Equal(t, "g8e.evidence.ConfigAttestation@"+constants.BuildAttestationSchemaVersion, configuration.SchemaRef)
	assert.Equal(t, time.Date(2026, 9, 6, 10, 1, 0, 0, time.UTC), configuration.ProducedAt)
}

func TestBuildConfigImporter_Import_RejectsInvalidConfiguration(t *testing.T) {
	fixture := newBuildConfigImporterFixture(t)
	tests := []struct {
		name     string
		importer *BuildConfigImporter
	}{
		{name: "nil importer", importer: nil},
		{name: "nil reader", importer: NewBuildConfigImporter(nil, fixture.binding)},
		{name: "invalid reference", importer: NewBuildConfigImporter(fixture.reader, BuildConfigImportBinding{Reference: "invalid", Path: fixture.binding.Path, ScopeID: fixture.binding.ScopeID, RunID: fixture.binding.RunID, BuildIdentity: fixture.binding.BuildIdentity, SourceRevision: fixture.binding.SourceRevision, ProducerIdentity: fixture.binding.ProducerIdentity})},
		{name: "unsafe path", importer: NewBuildConfigImporter(fixture.reader, BuildConfigImportBinding{Reference: fixture.binding.Reference, Path: filepath.Join(constants.PathParentDir, constants.BuildConfigAttestationsFilename), ScopeID: fixture.binding.ScopeID, RunID: fixture.binding.RunID, BuildIdentity: fixture.binding.BuildIdentity, SourceRevision: fixture.binding.SourceRevision, ProducerIdentity: fixture.binding.ProducerIdentity})},
		{name: "empty scope", importer: NewBuildConfigImporter(fixture.reader, BuildConfigImportBinding{Reference: fixture.binding.Reference, Path: fixture.binding.Path, RunID: fixture.binding.RunID, BuildIdentity: fixture.binding.BuildIdentity, SourceRevision: fixture.binding.SourceRevision, ProducerIdentity: fixture.binding.ProducerIdentity})},
		{name: "empty run", importer: NewBuildConfigImporter(fixture.reader, BuildConfigImportBinding{Reference: fixture.binding.Reference, Path: fixture.binding.Path, ScopeID: fixture.binding.ScopeID, BuildIdentity: fixture.binding.BuildIdentity, SourceRevision: fixture.binding.SourceRevision, ProducerIdentity: fixture.binding.ProducerIdentity})},
		{name: "empty build identity", importer: NewBuildConfigImporter(fixture.reader, BuildConfigImportBinding{Reference: fixture.binding.Reference, Path: fixture.binding.Path, ScopeID: fixture.binding.ScopeID, RunID: fixture.binding.RunID, SourceRevision: fixture.binding.SourceRevision, ProducerIdentity: fixture.binding.ProducerIdentity})},
		{name: "empty source revision", importer: NewBuildConfigImporter(fixture.reader, BuildConfigImportBinding{Reference: fixture.binding.Reference, Path: fixture.binding.Path, ScopeID: fixture.binding.ScopeID, RunID: fixture.binding.RunID, BuildIdentity: fixture.binding.BuildIdentity, ProducerIdentity: fixture.binding.ProducerIdentity})},
		{name: "empty producer", importer: NewBuildConfigImporter(fixture.reader, BuildConfigImportBinding{Reference: fixture.binding.Reference, Path: fixture.binding.Path, ScopeID: fixture.binding.ScopeID, RunID: fixture.binding.RunID, BuildIdentity: fixture.binding.BuildIdentity, SourceRevision: fixture.binding.SourceRevision})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.importer.Import(context.Background())
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
		})
	}
}

func TestBuildConfigImporter_Import_RejectsSourceDigestMismatch(t *testing.T) {
	fixture := newBuildConfigImporterFixture(t)
	fixture.binding.Reference = constants.BuildAttestationReferencePrefix + ":sha256:" + strings.Repeat("0", 64)
	_, err := NewBuildConfigImporter(fixture.reader, fixture.binding).Import(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrChecksumMismatch)
}

func TestBuildConfigImporter_Import_RejectsNoncanonicalAndUnknownFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*buildConfigImporterFixture)
	}{
		{name: "noncanonical record", mutate: func(f *buildConfigImporterFixture) {
			f.body = bytes.Replace(f.body, []byte("}\n"), []byte("} \n"), 1)
		}},
		{name: "unknown field", mutate: func(f *buildConfigImporterFixture) {
			f.body = bytes.Replace(f.body, []byte("}\n"), []byte(",\"unknown\":true}\n"), 1)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newBuildConfigImporterFixture(t)
			test.mutate(fixture)
			fixture.reader.files[fixture.binding.Path] = fixture.body
			fixture.binding.Reference = ContentReferenceForBody(constants.BuildAttestationReferencePrefix, fixture.body)
			_, err := NewBuildConfigImporter(fixture.reader, fixture.binding).Import(context.Background())
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
		})
	}
}

func TestBuildConfigImporter_Import_RejectsInvalidRecordBindings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*buildConfigImporterFixture)
	}{
		{name: "empty collection", mutate: func(f *buildConfigImporterFixture) { f.records = nil }},
		{name: "unsupported schema", mutate: func(f *buildConfigImporterFixture) { f.records[0].SchemaVersion = "2.0.0" }},
		{name: "unknown attestation type", mutate: func(f *buildConfigImporterFixture) { f.records[0].AttestationType = "unknown" }},
		{name: "producer mismatch", mutate: func(f *buildConfigImporterFixture) { f.records[0].ProducerIdentity = "build-system-2" }},
		{name: "scope mismatch", mutate: func(f *buildConfigImporterFixture) { f.records[0].ScopeID = "scope-2" }},
		{name: "run mismatch", mutate: func(f *buildConfigImporterFixture) { f.records[0].RunID = "run-2" }},
		{name: "build identity mismatch", mutate: func(f *buildConfigImporterFixture) { f.records[0].BuildIdentity = "build-2" }},
		{name: "source revision mismatch", mutate: func(f *buildConfigImporterFixture) { f.records[0].SourceRevision = "revision-2" }},
		{name: "invalid production timestamp", mutate: func(f *buildConfigImporterFixture) { f.records[0].ProducedAtUTC = "invalid" }},
		{name: "build without image digests", mutate: func(f *buildConfigImporterFixture) { f.records[0].ImageDigests = nil }},
		{name: "build without component inventory", mutate: func(f *buildConfigImporterFixture) { f.records[0].ComponentInventory = nil }},
		{name: "invalid image digest", mutate: func(f *buildConfigImporterFixture) { f.records[0].ImageDigests[0].SHA256 = "invalid" }},
		{name: "duplicate image name", mutate: func(f *buildConfigImporterFixture) {
			f.records[0].ImageDigests = append(f.records[0].ImageDigests, f.records[0].ImageDigests[0])
		}},
		{name: "incomplete component", mutate: func(f *buildConfigImporterFixture) { f.records[0].ComponentInventory[0].Version = "" }},
		{name: "duplicate component", mutate: func(f *buildConfigImporterFixture) {
			f.records[0].ComponentInventory = append(f.records[0].ComponentInventory, f.records[0].ComponentInventory[0])
		}},
		{name: "configuration without hashes", mutate: func(f *buildConfigImporterFixture) { f.records[1].ConfigurationHashes = nil }},
		{name: "invalid configuration hash", mutate: func(f *buildConfigImporterFixture) { f.records[1].ConfigurationHashes[0].SHA256 = "invalid" }},
		{name: "duplicate configuration name", mutate: func(f *buildConfigImporterFixture) {
			f.records[1].ConfigurationHashes = append(f.records[1].ConfigurationHashes, f.records[1].ConfigurationHashes[0])
		}},
		{name: "duplicate attestation content", mutate: func(f *buildConfigImporterFixture) { f.records = append(f.records, f.records[0]) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newBuildConfigImporterFixture(t)
			test.mutate(fixture)
			fixture.replaceRecords(t)
			_, err := NewBuildConfigImporter(fixture.reader, fixture.binding).Import(context.Background())
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
		})
	}
}

func TestBuildConfigImporter_Import_EnforcesRecordLimit(t *testing.T) {
	fixture := newBuildConfigImporterFixture(t)
	fixture.records = make([]buildConfigAttestationRecord, constants.BuildAttestationMaxRecords+1)
	for index := range fixture.records {
		fixture.records[index] = buildConfigAttestationRecord{
			SchemaVersion: constants.BuildAttestationSchemaVersion, AttestationType: buildAttestationTypeConfiguration, ProducerIdentity: fixture.binding.ProducerIdentity,
			ProducedAtUTC: "2026-09-06T10:01:00Z", ScopeID: fixture.binding.ScopeID, RunID: fixture.binding.RunID, BuildIdentity: fixture.binding.BuildIdentity,
			SourceRevision: fixture.binding.SourceRevision, ConfigurationHashes: []buildConfigNamedDigest{{Name: fmt.Sprintf("config-%d", index), SHA256: strings.Repeat("3", 64)}},
		}
	}
	fixture.replaceRecords(t)
	_, err := NewBuildConfigImporter(fixture.reader, fixture.binding).Import(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceArtifactTooLarge)
}

func TestBuildConfigImporter_Import_WrapsReadFailure(t *testing.T) {
	fixture := newBuildConfigImporterFixture(t)
	readErr := fmt.Errorf("build attestation source unavailable")
	_, err := NewBuildConfigImporter(&failingArtifactReader{err: readErr}, fixture.binding).Import(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceImporterFailed)
	assert.ErrorIs(t, err, readErr)
}

func TestBuildConfigImporter_Import_RespectsCancellation(t *testing.T) {
	fixture := newBuildConfigImporterFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewBuildConfigImporter(fixture.reader, fixture.binding).Import(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestBuildConfigImporter_Import_ProducesValidGraph(t *testing.T) {
	fixture := newBuildConfigImporterFixture(t)
	nodes, err := NewBuildConfigImporter(fixture.reader, fixture.binding).Import(context.Background())
	require.NoError(t, err)
	graph := NewEvidenceGraph(constants.DemoRunMaxArtifactBytes, []string{constants.MediaTypeJSON})
	for _, node := range nodes {
		require.NoError(t, graph.AddNode(node))
	}
	graph.ValidateAll(time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC), time.Date(2026, 9, 6, 11, 0, 0, 0, time.UTC))
	assert.True(t, graph.Valid(), "graph failures: %v", graph.Failures())
	assert.Equal(t, 2, graph.NodeCount())
}
