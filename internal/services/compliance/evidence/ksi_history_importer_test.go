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
	"github.com/g8e-ai/g8e/v2/internal/services/compliance"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

type ksiHistoryImporterFixture struct {
	reader    *memoryArtifactReader
	snapshots []compliance.KSIResultSet
	body      []byte
	binding   KSIHistoryImportBinding
}

func newKSIHistoryImporterFixture(t *testing.T) *ksiHistoryImporterFixture {
	t.Helper()
	fixture := &ksiHistoryImporterFixture{
		reader: &memoryArtifactReader{files: map[string][]byte{}},
		snapshots: []compliance.KSIResultSet{{
			Class:         compliance.ClassC,
			EvaluatedAtMs: 1_700_000_000_000,
			Results: []compliance.KSIResult{{
				ID:                  "KSI-CMT-01",
				Status:              compliance.KSIStatusNotSatisfied,
				Evidence:            []*compliancev1.ComplianceEvidenceReference{{ArtifactType: string(compliance.EvidenceTypeLedgerCommit), ArtifactId: "commit-1"}},
				LastValidatedUnixMs: 1_699_999_999_000,
				MethodCount:         2,
			}},
		},
			{
				Class:         compliance.ClassC,
				EvaluatedAtMs: 1_700_000_100_000,
				Results: []compliance.KSIResult{{
					ID:                  "KSI-CMT-01",
					Status:              compliance.KSIStatusSatisfied,
					Evidence:            []*compliancev1.ComplianceEvidenceReference{{ArtifactType: string(compliance.EvidenceTypeReceiptID), ArtifactId: "transaction-1"}},
					LastValidatedUnixMs: 1_700_000_099_000,
					MethodCount:         2,
				}},
			}},
		binding: KSIHistoryImportBinding{
			Path:             filepath.Join(constants.KSIHistoryDirname, constants.ComplianceBundleKSIHistoryFilename),
			ScopeID:          "scope-1",
			RunID:            "run-1",
			Class:            compliance.ClassC,
			ProducerIdentity: "compliance-evaluator-1",
		},
	}
	fixture.replaceHistory(t)
	return fixture
}

func (f *ksiHistoryImporterFixture) replaceHistory(t *testing.T) {
	t.Helper()
	lines := make([][]byte, 0, len(f.snapshots))
	for _, snapshot := range f.snapshots {
		line, err := json.Marshal(snapshot)
		require.NoError(t, err)
		lines = append(lines, line)
	}
	f.body = append(bytes.Join(lines, []byte{'\n'}), '\n')
	f.reader.files[f.binding.Path] = f.body
	f.binding.Reference = ContentReferenceForBody(constants.KSIHistoryReferencePrefix, f.body)
}

func TestKSIHistoryImporter_SourceID(t *testing.T) {
	fixture := newKSIHistoryImporterFixture(t)
	assert.Equal(t, "ksi-history", NewKSIHistoryImporter(fixture.reader, fixture.binding).SourceID())
}

func TestKSIHistoryImporter_Import_LoadsChronologicalSnapshots(t *testing.T) {
	fixture := newKSIHistoryImporterFixture(t)
	nodes, err := NewKSIHistoryImporter(fixture.reader, fixture.binding).Import(context.Background())
	require.NoError(t, err)
	require.Len(t, nodes, 2)

	first := nodes[0]
	assert.Equal(t, ArtifactTypeKSIResult, first.ArtifactType)
	assert.Equal(t, ContentReferenceForBody(constants.KSIResultReferencePrefix, first.CanonicalBytes), first.ArtifactID)
	assert.Equal(t, constants.MediaTypeJSON, first.MediaType)
	assert.Equal(t, "g8e.compliance.KSIResultSet@"+constants.KSIHistorySchemaVersion, first.SchemaRef)
	assert.Equal(t, fixture.binding.ProducerIdentity, first.ProducerIdentity)
	assert.Equal(t, fixture.binding.ScopeID, first.ScopeID)
	assert.Equal(t, fixture.binding.RunID, first.RunID)
	assert.Equal(t, VerificationStatusUnverified, first.VerificationStatus)
	assert.Empty(t, first.VerifierID)
	assert.Empty(t, first.References)
	assert.Equal(t, time.UnixMilli(fixture.snapshots[0].EvaluatedAtMs).UTC(), first.ProducedAt)

	second := nodes[1]
	assert.True(t, second.ProducedAt.After(first.ProducedAt))
	assert.NotEqual(t, first.ArtifactID, second.ArtifactID)
	assert.Empty(t, second.References)
}

func TestKSIHistoryImporter_Import_RejectsInvalidConfiguration(t *testing.T) {
	fixture := newKSIHistoryImporterFixture(t)
	tests := []struct {
		name     string
		importer *KSIHistoryImporter
	}{
		{name: "nil importer", importer: nil},
		{name: "nil reader", importer: NewKSIHistoryImporter(nil, fixture.binding)},
		{name: "invalid reference", importer: NewKSIHistoryImporter(fixture.reader, KSIHistoryImportBinding{Reference: "invalid", Path: fixture.binding.Path, ScopeID: fixture.binding.ScopeID, RunID: fixture.binding.RunID, Class: fixture.binding.Class, ProducerIdentity: fixture.binding.ProducerIdentity})},
		{name: "unsafe path", importer: NewKSIHistoryImporter(fixture.reader, KSIHistoryImportBinding{Reference: fixture.binding.Reference, Path: filepath.Join(constants.PathParentDir, constants.ComplianceBundleKSIHistoryFilename), ScopeID: fixture.binding.ScopeID, RunID: fixture.binding.RunID, Class: fixture.binding.Class, ProducerIdentity: fixture.binding.ProducerIdentity})},
		{name: "empty scope", importer: NewKSIHistoryImporter(fixture.reader, KSIHistoryImportBinding{Reference: fixture.binding.Reference, Path: fixture.binding.Path, RunID: fixture.binding.RunID, Class: fixture.binding.Class, ProducerIdentity: fixture.binding.ProducerIdentity})},
		{name: "empty run", importer: NewKSIHistoryImporter(fixture.reader, KSIHistoryImportBinding{Reference: fixture.binding.Reference, Path: fixture.binding.Path, ScopeID: fixture.binding.ScopeID, Class: fixture.binding.Class, ProducerIdentity: fixture.binding.ProducerIdentity})},
		{name: "invalid class", importer: NewKSIHistoryImporter(fixture.reader, KSIHistoryImportBinding{Reference: fixture.binding.Reference, Path: fixture.binding.Path, ScopeID: fixture.binding.ScopeID, RunID: fixture.binding.RunID, Class: compliance.CertificationClass("E"), ProducerIdentity: fixture.binding.ProducerIdentity})},
		{name: "empty producer", importer: NewKSIHistoryImporter(fixture.reader, KSIHistoryImportBinding{Reference: fixture.binding.Reference, Path: fixture.binding.Path, ScopeID: fixture.binding.ScopeID, RunID: fixture.binding.RunID, Class: fixture.binding.Class})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.importer.Import(context.Background())
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
		})
	}
}

func TestKSIHistoryImporter_Import_RejectsSourceDigestMismatch(t *testing.T) {
	fixture := newKSIHistoryImporterFixture(t)
	fixture.binding.Reference = constants.KSIHistoryReferencePrefix + ":sha256:" + strings.Repeat("0", 64)
	_, err := NewKSIHistoryImporter(fixture.reader, fixture.binding).Import(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrChecksumMismatch)
}

func TestKSIHistoryImporter_Import_RejectsMalformedCanonicalJSONL(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ksiHistoryImporterFixture)
	}{
		{name: "noncanonical snapshot", mutate: func(f *ksiHistoryImporterFixture) { f.body = bytes.Replace(f.body, []byte("}\n"), []byte("} \n"), 1) }},
		{name: "unknown snapshot field", mutate: func(f *ksiHistoryImporterFixture) {
			f.body = bytes.Replace(f.body, []byte("}\n"), []byte(",\"unknown\":true}\n"), 1)
		}},
		{name: "unknown result field", mutate: func(f *ksiHistoryImporterFixture) {
			f.body = bytes.Replace(f.body, []byte("\"method_count\":2}"), []byte("\"method_count\":2,\"unknown\":true}"), 1)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newKSIHistoryImporterFixture(t)
			test.mutate(fixture)
			fixture.reader.files[fixture.binding.Path] = fixture.body
			fixture.binding.Reference = ContentReferenceForBody(constants.KSIHistoryReferencePrefix, fixture.body)
			_, err := NewKSIHistoryImporter(fixture.reader, fixture.binding).Import(context.Background())
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
		})
	}
}

func TestKSIHistoryImporter_Import_RejectsInvalidSnapshotBindings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ksiHistoryImporterFixture)
	}{
		{name: "empty history", mutate: func(f *ksiHistoryImporterFixture) { f.snapshots = nil }},
		{name: "class mismatch", mutate: func(f *ksiHistoryImporterFixture) { f.snapshots[0].Class = compliance.ClassB }},
		{name: "invalid evaluation timestamp", mutate: func(f *ksiHistoryImporterFixture) { f.snapshots[0].EvaluatedAtMs = 0 }},
		{name: "non-increasing chronology", mutate: func(f *ksiHistoryImporterFixture) { f.snapshots[1].EvaluatedAtMs = f.snapshots[0].EvaluatedAtMs }},
		{name: "empty results", mutate: func(f *ksiHistoryImporterFixture) { f.snapshots[0].Results = nil }},
		{name: "empty KSI ID", mutate: func(f *ksiHistoryImporterFixture) { f.snapshots[0].Results[0].ID = "" }},
		{name: "duplicate KSI ID", mutate: func(f *ksiHistoryImporterFixture) {
			f.snapshots[0].Results = append(f.snapshots[0].Results, f.snapshots[0].Results[0])
		}},
		{name: "invalid status", mutate: func(f *ksiHistoryImporterFixture) { f.snapshots[0].Results[0].Status = compliance.KSIStatus("unknown") }},
		{name: "negative method count", mutate: func(f *ksiHistoryImporterFixture) { f.snapshots[0].Results[0].MethodCount = -1 }},
		{name: "missing validation timestamp", mutate: func(f *ksiHistoryImporterFixture) { f.snapshots[0].Results[0].LastValidatedUnixMs = 0 }},
		{name: "validation after evaluation", mutate: func(f *ksiHistoryImporterFixture) {
			f.snapshots[0].Results[0].LastValidatedUnixMs = f.snapshots[0].EvaluatedAtMs + 1
		}},
		{name: "invalid evidence type", mutate: func(f *ksiHistoryImporterFixture) {
			f.snapshots[0].Results[0].Evidence[0].ArtifactType = "unknown"
		}},
		{name: "empty evidence reference", mutate: func(f *ksiHistoryImporterFixture) { f.snapshots[0].Results[0].Evidence[0].ArtifactId = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newKSIHistoryImporterFixture(t)
			test.mutate(fixture)
			fixture.replaceHistory(t)
			_, err := NewKSIHistoryImporter(fixture.reader, fixture.binding).Import(context.Background())
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
		})
	}
}

func TestKSIHistoryImporter_Import_EnforcesSnapshotLimit(t *testing.T) {
	fixture := newKSIHistoryImporterFixture(t)
	fixture.snapshots = make([]compliance.KSIResultSet, constants.KSIHistoryMaxSnapshots+1)
	for index := range fixture.snapshots {
		fixture.snapshots[index] = compliance.KSIResultSet{Class: compliance.ClassC, EvaluatedAtMs: int64(1_700_000_000_000 + index), Results: []compliance.KSIResult{{ID: "KSI-CMT-01", Status: compliance.KSIStatusSatisfied, LastValidatedUnixMs: int64(1_699_999_999_000 + index), MethodCount: 2}}}
	}
	fixture.replaceHistory(t)
	_, err := NewKSIHistoryImporter(fixture.reader, fixture.binding).Import(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceArtifactTooLarge)
}

func TestKSIHistoryImporter_Import_WrapsReadFailure(t *testing.T) {
	fixture := newKSIHistoryImporterFixture(t)
	readErr := fmt.Errorf("KSI history source unavailable")
	_, err := NewKSIHistoryImporter(&failingArtifactReader{err: readErr}, fixture.binding).Import(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceImporterFailed)
	assert.ErrorIs(t, err, readErr)
}

func TestKSIHistoryImporter_Import_RespectsCancellation(t *testing.T) {
	fixture := newKSIHistoryImporterFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewKSIHistoryImporter(fixture.reader, fixture.binding).Import(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestKSIHistoryImporter_Import_ProducesValidGraph(t *testing.T) {
	fixture := newKSIHistoryImporterFixture(t)
	nodes, err := NewKSIHistoryImporter(fixture.reader, fixture.binding).Import(context.Background())
	require.NoError(t, err)
	graph := NewEvidenceGraph(constants.DemoRunMaxArtifactBytes, []string{constants.MediaTypeJSON})
	for _, node := range nodes {
		require.NoError(t, graph.AddNode(node))
	}
	graph.ValidateAll(time.UnixMilli(1_699_999_000_000), time.UnixMilli(1_700_001_000_000))
	assert.True(t, graph.Valid(), "graph failures: %v", graph.Failures())
	assert.Equal(t, 2, graph.NodeCount())
}
