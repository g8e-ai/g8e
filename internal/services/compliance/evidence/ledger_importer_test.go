package evidence

import (
	"bytes"
	"context"
	"crypto/sha1"
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

type ledgerCommitFixtureRecord struct {
	SchemaVersion    string `json:"schema_version"`
	ProducerIdentity string `json:"producer_identity"`
	CommitHash       string `json:"commit_hash"`
	ParentHash       string `json:"parent_hash"`
	TimestampUTC     string `json:"timestamp_utc"`
	Message          string `json:"message"`
	FilesChanged     int    `json:"files_changed"`
	DiffStat         string `json:"diff_stat"`
}

type ledgerStateFixtureRecord struct {
	SchemaVersion    string `json:"schema_version"`
	ProducerIdentity string `json:"producer_identity"`
	MerkleRoot       string `json:"merkle_root"`
	CapturedAtUTC    string `json:"captured_at_utc"`
}

type ledgerImporterFixture struct {
	reader      *memoryArtifactReader
	commits     []ledgerCommitFixtureRecord
	state       ledgerStateFixtureRecord
	commitsBody []byte
	stateBody   []byte
	binding     LedgerImportBinding
}

func newLedgerImporterFixture(t *testing.T) *ledgerImporterFixture {
	t.Helper()
	firstHash := strings.Repeat("1", sha1.Size*2)
	secondHash := strings.Repeat("2", sha1.Size*2)
	commits := []ledgerCommitFixtureRecord{
		{SchemaVersion: constants.LedgerEvidenceSchemaVersion, ProducerIdentity: "gateway-1", CommitHash: firstHash, TimestampUTC: "2026-09-06T10:00:00Z", Message: "bootstrap", FilesChanged: 1, DiffStat: "1 file changed"},
		{SchemaVersion: constants.LedgerEvidenceSchemaVersion, ProducerIdentity: "gateway-1", CommitHash: secondHash, ParentHash: firstHash, TimestampUTC: "2026-09-06T10:01:00Z", Message: "governed mutation", FilesChanged: 1, DiffStat: "1 file changed"},
	}
	state := ledgerStateFixtureRecord{SchemaVersion: constants.LedgerEvidenceSchemaVersion, ProducerIdentity: "gateway-1", MerkleRoot: secondHash, CapturedAtUTC: "2026-09-06T10:02:00Z"}
	fixture := &ledgerImporterFixture{reader: &memoryArtifactReader{files: map[string][]byte{}}, commits: commits, state: state}
	fixture.binding = LedgerImportBinding{
		CommitsPath: filepath.Join(constants.LedgerEvidenceDirname, constants.LedgerCommitsFilename),
		StatePath:   filepath.Join(constants.LedgerEvidenceDirname, constants.LedgerStateFilename),
		ScopeID:     "scope-1",
		RunID:       "run-1",
		AttemptID:   "attempt-1",
		ScenarioID:  "scenario-1",
	}
	fixture.replaceCommits(t)
	fixture.replaceState(t)
	return fixture
}

func (f *ledgerImporterFixture) replaceCommits(t *testing.T) {
	t.Helper()
	lines := make([][]byte, 0, len(f.commits))
	for _, commit := range f.commits {
		line, err := json.Marshal(commit)
		require.NoError(t, err)
		lines = append(lines, line)
	}
	f.commitsBody = append(bytes.Join(lines, []byte{'\n'}), '\n')
	f.reader.files[f.binding.CommitsPath] = f.commitsBody
	f.binding.CommitsReference = ContentReferenceForBody(constants.LedgerCommitCollectionReferencePrefix, f.commitsBody)
}

func (f *ledgerImporterFixture) replaceState(t *testing.T) {
	t.Helper()
	body, err := json.Marshal(f.state)
	require.NoError(t, err)
	f.stateBody = body
	f.reader.files[f.binding.StatePath] = body
	f.binding.StateReference = ContentReferenceForBody(constants.LedgerStateReferencePrefix, body)
}

func TestLedgerImporter_SourceID(t *testing.T) {
	fixture := newLedgerImporterFixture(t)
	assert.Equal(t, "ledger", NewLedgerImporter(fixture.reader, fixture.binding).SourceID())
}

func TestLedgerImporter_Import_LoadsCanonicalChainAndState(t *testing.T) {
	fixture := newLedgerImporterFixture(t)
	nodes, err := NewLedgerImporter(fixture.reader, fixture.binding).Import(context.Background())
	require.NoError(t, err)
	require.Len(t, nodes, 3)

	first := nodes[0]
	assert.Equal(t, ArtifactTypeLedgerCommit, first.ArtifactType)
	assert.Empty(t, first.TransactionID)
	assert.Equal(t, fixture.binding.ScopeID, first.ScopeID)
	assert.Equal(t, fixture.binding.RunID, first.RunID)
	assert.Equal(t, fixture.binding.AttemptID, first.AttemptID)
	assert.Equal(t, fixture.binding.ScenarioID, first.ScenarioID)
	assert.Equal(t, fixture.commits[0].ProducerIdentity, first.ProducerIdentity)
	assert.Equal(t, VerificationStatusUnverified, first.VerificationStatus)
	assert.Empty(t, first.References)

	second := nodes[1]
	assert.Equal(t, ArtifactTypeLedgerCommit, second.ArtifactType)
	assert.Empty(t, second.TransactionID)
	assert.Equal(t, []string{first.ArtifactID}, second.References)

	state := nodes[2]
	assert.Equal(t, fixture.binding.StateReference, state.ArtifactID)
	assert.Equal(t, ArtifactTypeLedgerState, state.ArtifactType)
	assert.Empty(t, state.TransactionID)
	assert.Equal(t, []string{second.ArtifactID}, state.References)
	assert.Equal(t, VerificationStatusUnverified, state.VerificationStatus)
	assert.Equal(t, fixture.stateBody, state.CanonicalBytes)
	assert.Equal(t, time.Date(2026, 9, 6, 10, 2, 0, 0, time.UTC), state.ProducedAt)
}

func TestLedgerImporter_Import_RejectsInvalidConfiguration(t *testing.T) {
	fixture := newLedgerImporterFixture(t)
	tests := []struct {
		name     string
		importer *LedgerImporter
	}{
		{name: "nil importer", importer: nil},
		{name: "nil reader", importer: NewLedgerImporter(nil, fixture.binding)},
		{name: "invalid commits reference", importer: NewLedgerImporter(fixture.reader, LedgerImportBinding{CommitsReference: "invalid", StateReference: fixture.binding.StateReference, CommitsPath: fixture.binding.CommitsPath, StatePath: fixture.binding.StatePath, ScopeID: fixture.binding.ScopeID, RunID: fixture.binding.RunID})},
		{name: "invalid state reference", importer: NewLedgerImporter(fixture.reader, LedgerImportBinding{CommitsReference: fixture.binding.CommitsReference, StateReference: "invalid", CommitsPath: fixture.binding.CommitsPath, StatePath: fixture.binding.StatePath, ScopeID: fixture.binding.ScopeID, RunID: fixture.binding.RunID})},
		{name: "unsafe commits path", importer: NewLedgerImporter(fixture.reader, LedgerImportBinding{CommitsReference: fixture.binding.CommitsReference, StateReference: fixture.binding.StateReference, CommitsPath: filepath.Join(constants.PathParentDir, constants.LedgerCommitsFilename), StatePath: fixture.binding.StatePath, ScopeID: fixture.binding.ScopeID, RunID: fixture.binding.RunID})},
		{name: "unsafe state path", importer: NewLedgerImporter(fixture.reader, LedgerImportBinding{CommitsReference: fixture.binding.CommitsReference, StateReference: fixture.binding.StateReference, CommitsPath: fixture.binding.CommitsPath, StatePath: filepath.Join(constants.PathParentDir, constants.LedgerStateFilename), ScopeID: fixture.binding.ScopeID, RunID: fixture.binding.RunID})},
		{name: "empty scope", importer: NewLedgerImporter(fixture.reader, LedgerImportBinding{CommitsReference: fixture.binding.CommitsReference, StateReference: fixture.binding.StateReference, CommitsPath: fixture.binding.CommitsPath, StatePath: fixture.binding.StatePath, RunID: fixture.binding.RunID})},
		{name: "empty run", importer: NewLedgerImporter(fixture.reader, LedgerImportBinding{CommitsReference: fixture.binding.CommitsReference, StateReference: fixture.binding.StateReference, CommitsPath: fixture.binding.CommitsPath, StatePath: fixture.binding.StatePath, ScopeID: fixture.binding.ScopeID})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.importer.Import(context.Background())
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
		})
	}
}

func TestLedgerImporter_Import_RejectsSourceDigestMismatch(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ledgerImporterFixture)
	}{
		{name: "commit collection", mutate: func(f *ledgerImporterFixture) {
			f.binding.CommitsReference = constants.LedgerCommitCollectionReferencePrefix + ":sha256:" + strings.Repeat("0", 64)
		}},
		{name: "state", mutate: func(f *ledgerImporterFixture) {
			f.binding.StateReference = constants.LedgerStateReferencePrefix + ":sha256:" + strings.Repeat("0", 64)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newLedgerImporterFixture(t)
			test.mutate(fixture)
			_, err := NewLedgerImporter(fixture.reader, fixture.binding).Import(context.Background())
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrChecksumMismatch)
		})
	}
}

func TestLedgerImporter_Import_RejectsMalformedArtifacts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *ledgerImporterFixture)
	}{
		{name: "noncanonical commit", mutate: func(_ *testing.T, f *ledgerImporterFixture) {
			f.commitsBody = bytes.Replace(f.commitsBody, []byte("}\n"), []byte("} \n"), 1)
			f.reader.files[f.binding.CommitsPath] = f.commitsBody
			f.binding.CommitsReference = ContentReferenceForBody(constants.LedgerCommitCollectionReferencePrefix, f.commitsBody)
		}},
		{name: "noncanonical state", mutate: func(_ *testing.T, f *ledgerImporterFixture) {
			f.stateBody = append(f.stateBody, '\n')
			f.reader.files[f.binding.StatePath] = f.stateBody
			f.binding.StateReference = ContentReferenceForBody(constants.LedgerStateReferencePrefix, f.stateBody)
		}},
		{name: "unknown commit field", mutate: func(_ *testing.T, f *ledgerImporterFixture) {
			f.commitsBody = bytes.Replace(f.commitsBody, []byte("}\n"), []byte(",\"unknown\":true}\n"), 1)
			f.reader.files[f.binding.CommitsPath] = f.commitsBody
			f.binding.CommitsReference = ContentReferenceForBody(constants.LedgerCommitCollectionReferencePrefix, f.commitsBody)
		}},
		{name: "unknown state field", mutate: func(_ *testing.T, f *ledgerImporterFixture) {
			f.stateBody = bytes.Replace(f.stateBody, []byte("}"), []byte(",\"unknown\":true}"), 1)
			f.reader.files[f.binding.StatePath] = f.stateBody
			f.binding.StateReference = ContentReferenceForBody(constants.LedgerStateReferencePrefix, f.stateBody)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newLedgerImporterFixture(t)
			test.mutate(t, fixture)
			_, err := NewLedgerImporter(fixture.reader, fixture.binding).Import(context.Background())
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
		})
	}
}

func TestLedgerImporter_Import_RejectsInvalidChainAndStateBindings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ledgerImporterFixture)
	}{
		{name: "empty commits", mutate: func(f *ledgerImporterFixture) { f.commits = nil }},
		{name: "unsupported commit schema", mutate: func(f *ledgerImporterFixture) { f.commits[0].SchemaVersion = "2.0.0" }},
		{name: "unsupported state schema", mutate: func(f *ledgerImporterFixture) { f.state.SchemaVersion = "2.0.0" }},
		{name: "invalid commit hash", mutate: func(f *ledgerImporterFixture) { f.commits[0].CommitHash = "invalid" }},
		{name: "genesis parent", mutate: func(f *ledgerImporterFixture) { f.commits[0].ParentHash = strings.Repeat("3", sha1.Size*2) }},
		{name: "broken parent chain", mutate: func(f *ledgerImporterFixture) { f.commits[1].ParentHash = strings.Repeat("3", sha1.Size*2) }},
		{name: "duplicate commit", mutate: func(f *ledgerImporterFixture) {
			f.commits[1].CommitHash = f.commits[0].CommitHash
			f.state.MerkleRoot = f.commits[0].CommitHash
		}},
		{name: "decreasing timestamp", mutate: func(f *ledgerImporterFixture) { f.commits[1].TimestampUTC = "2026-09-06T09:59:00Z" }},
		{name: "producer mismatch", mutate: func(f *ledgerImporterFixture) { f.commits[1].ProducerIdentity = "gateway-2" }},
		{name: "state producer mismatch", mutate: func(f *ledgerImporterFixture) { f.state.ProducerIdentity = "gateway-2" }},
		{name: "head root mismatch", mutate: func(f *ledgerImporterFixture) { f.state.MerkleRoot = strings.Repeat("3", sha1.Size*2) }},
		{name: "capture before head", mutate: func(f *ledgerImporterFixture) { f.state.CapturedAtUTC = "2026-09-06T09:59:00Z" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newLedgerImporterFixture(t)
			test.mutate(fixture)
			fixture.replaceCommits(t)
			fixture.replaceState(t)
			_, err := NewLedgerImporter(fixture.reader, fixture.binding).Import(context.Background())
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
		})
	}
}

func TestLedgerImporter_Import_WrapsReadFailures(t *testing.T) {
	fixture := newLedgerImporterFixture(t)
	readErr := fmt.Errorf("ledger source unavailable")
	_, err := NewLedgerImporter(&failingArtifactReader{err: readErr}, fixture.binding).Import(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceImporterFailed)
	assert.ErrorIs(t, err, readErr)
}

func TestLedgerImporter_Import_RespectsCancellation(t *testing.T) {
	fixture := newLedgerImporterFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewLedgerImporter(fixture.reader, fixture.binding).Import(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestLedgerImporter_Import_ProducesValidGraph(t *testing.T) {
	fixture := newLedgerImporterFixture(t)
	nodes, err := NewLedgerImporter(fixture.reader, fixture.binding).Import(context.Background())
	require.NoError(t, err)
	graph := NewEvidenceGraph(constants.DemoRunMaxArtifactBytes, []string{constants.MediaTypeJSON})
	for _, node := range nodes {
		require.NoError(t, graph.AddNode(node))
	}
	graph.ValidateAll(time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC), time.Date(2026, 9, 6, 11, 0, 0, 0, time.UTC))
	assert.True(t, graph.Valid(), "graph failures: %v", graph.Failures())
	assert.Equal(t, 3, graph.NodeCount())
}
