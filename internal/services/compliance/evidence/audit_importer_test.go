package evidence

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/timesvc"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type auditRecordFixture struct {
	reader  *memoryArtifactReader
	body    []byte
	binding AuditRecordImportBinding
}

func newAuditRecordFixture(t *testing.T) *auditRecordFixture {
	t.Helper()
	event := &operatorv1.AuditEvent{
		Id:                  41,
		OperatorSessionId:   "session-1",
		Timestamp:           timesvc.FormatTimestamp(time.Unix(1_700_000_000, 0)),
		Type:                string(constants.EventAppTaskCompleted),
		CommandRaw:          "printf test",
		CommandExitCode:     0,
		ExecutionDurationMs: 12,
		StoredLocally:       true,
		FileMutations: []*operatorv1.AuditFileMutation{{
			Id:               7,
			Filepath:         constants.DemosTargetDataDir,
			Operation:        "WRITE",
			LedgerHashBefore: strings.Repeat("a", 64),
			LedgerHashAfter:  strings.Repeat("b", 64),
		}},
	}
	body, err := compliancev1.MarshalCanonical(event)
	require.NoError(t, err)
	reference := ContentReferenceForBody(constants.AuditRecordReferencePrefix, body)
	path := filepath.Join(constants.AuditRecordsDirname, digestHex(body)+constants.FileExtJSON)
	return &auditRecordFixture{
		reader: &memoryArtifactReader{files: map[string][]byte{path: body}},
		body:   body,
		binding: AuditRecordImportBinding{
			Reference:         reference,
			Path:              path,
			ScopeID:           "scope-1",
			RunID:             "run-1",
			AttemptID:         "attempt-1",
			ScenarioID:        "scenario-1",
			OperatorSessionID: event.OperatorSessionId,
		},
	}
}

func TestAuditRecordImporter_SourceID(t *testing.T) {
	fixture := newAuditRecordFixture(t)
	assert.Equal(t, "audit-record", NewAuditRecordImporter(fixture.reader, fixture.binding).SourceID())
}

func TestAuditRecordImporter_Import_LoadsCanonicalEvent(t *testing.T) {
	fixture := newAuditRecordFixture(t)
	nodes, err := NewAuditRecordImporter(fixture.reader, fixture.binding).Import(context.Background())
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	node := nodes[0]
	assert.Equal(t, fixture.binding.Reference, node.ArtifactID)
	assert.Equal(t, ArtifactTypeAuditRecord, node.ArtifactType)
	assert.Equal(t, fixture.binding.OperatorSessionID, node.ProducerIdentity)
	assert.Equal(t, fixture.binding.ScopeID, node.ScopeID)
	assert.Equal(t, fixture.binding.RunID, node.RunID)
	assert.Equal(t, fixture.binding.AttemptID, node.AttemptID)
	assert.Equal(t, fixture.binding.ScenarioID, node.ScenarioID)
	assert.Equal(t, VerificationStatusUnverified, node.VerificationStatus)
	assert.Empty(t, node.VerifierID)
	assert.Equal(t, time.Unix(1_700_000_000, 0).UTC(), node.ProducedAt)
	assert.Equal(t, fixture.body, node.CanonicalBytes)
	assert.Empty(t, node.References)
}

func TestAuditRecordImporter_Import_RejectsInvalidConfiguration(t *testing.T) {
	fixture := newAuditRecordFixture(t)
	tests := []struct {
		name     string
		importer *AuditRecordImporter
	}{
		{name: "nil importer", importer: nil},
		{name: "nil reader", importer: NewAuditRecordImporter(nil, fixture.binding)},
		{name: "invalid reference", importer: NewAuditRecordImporter(fixture.reader, AuditRecordImportBinding{Reference: "invalid", Path: fixture.binding.Path, ScopeID: fixture.binding.ScopeID, RunID: fixture.binding.RunID, OperatorSessionID: fixture.binding.OperatorSessionID})},
		{name: "unsafe path", importer: NewAuditRecordImporter(fixture.reader, AuditRecordImportBinding{Reference: fixture.binding.Reference, Path: filepath.Join(constants.PathParentDir, constants.AuditRecordsDirname), ScopeID: fixture.binding.ScopeID, RunID: fixture.binding.RunID, OperatorSessionID: fixture.binding.OperatorSessionID})},
		{name: "empty scope", importer: NewAuditRecordImporter(fixture.reader, AuditRecordImportBinding{Reference: fixture.binding.Reference, Path: fixture.binding.Path, RunID: fixture.binding.RunID, OperatorSessionID: fixture.binding.OperatorSessionID})},
		{name: "empty run", importer: NewAuditRecordImporter(fixture.reader, AuditRecordImportBinding{Reference: fixture.binding.Reference, Path: fixture.binding.Path, ScopeID: fixture.binding.ScopeID, OperatorSessionID: fixture.binding.OperatorSessionID})},
		{name: "empty session", importer: NewAuditRecordImporter(fixture.reader, AuditRecordImportBinding{Reference: fixture.binding.Reference, Path: fixture.binding.Path, ScopeID: fixture.binding.ScopeID, RunID: fixture.binding.RunID})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.importer.Import(context.Background())
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
		})
	}
}

func TestAuditRecordImporter_Import_RejectsContentMutations(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*testing.T, *auditRecordFixture)
		targetErr error
	}{
		{name: "digest mismatch", mutate: func(_ *testing.T, fixture *auditRecordFixture) {
			fixture.binding.Reference = constants.AuditRecordReferencePrefix + ":sha256:" + strings.Repeat("0", 64)
		}, targetErr: constants.ErrChecksumMismatch},
		{name: "noncanonical protojson", mutate: func(_ *testing.T, fixture *auditRecordFixture) {
			body := append(append([]byte{}, fixture.body...), '\n')
			fixture.reader.files[fixture.binding.Path] = body
			fixture.binding.Reference = ContentReferenceForBody(constants.AuditRecordReferencePrefix, body)
		}, targetErr: constants.ErrEvidenceArtifactMalformed},
		{name: "malformed event binding", mutate: func(t *testing.T, fixture *auditRecordFixture) {
			event := &operatorv1.AuditEvent{Id: 41, OperatorSessionId: fixture.binding.OperatorSessionID, Timestamp: timesvc.FormatTimestamp(time.Unix(1_700_000_000, 0))}
			body, err := compliancev1.MarshalCanonical(event)
			require.NoError(t, err)
			fixture.reader.files[fixture.binding.Path] = body
			fixture.binding.Reference = ContentReferenceForBody(constants.AuditRecordReferencePrefix, body)
		}, targetErr: constants.ErrEvidenceArtifactMalformed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAuditRecordFixture(t)
			test.mutate(t, fixture)
			_, err := NewAuditRecordImporter(fixture.reader, fixture.binding).Import(context.Background())
			require.Error(t, err)
			assert.ErrorIs(t, err, test.targetErr)
		})
	}
}

func TestAuditRecordImporter_Import_RejectsSessionAndTimestampMismatch(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*testing.T, *auditRecordFixture)
		targetErr error
	}{
		{name: "operator session mismatch", mutate: func(_ *testing.T, fixture *auditRecordFixture) {
			fixture.binding.OperatorSessionID = "session-2"
		}, targetErr: constants.ErrEvidenceScopeMismatch},
		{name: "invalid timestamp", mutate: func(t *testing.T, fixture *auditRecordFixture) {
			event := &operatorv1.AuditEvent{Id: 41, OperatorSessionId: fixture.binding.OperatorSessionID, Timestamp: "invalid", Type: string(constants.EventAppTaskCompleted)}
			body, err := compliancev1.MarshalCanonical(event)
			require.NoError(t, err)
			fixture.reader.files[fixture.binding.Path] = body
			fixture.binding.Reference = ContentReferenceForBody(constants.AuditRecordReferencePrefix, body)
		}, targetErr: constants.ErrEvidenceArtifactMalformed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAuditRecordFixture(t)
			test.mutate(t, fixture)
			_, err := NewAuditRecordImporter(fixture.reader, fixture.binding).Import(context.Background())
			require.Error(t, err)
			assert.ErrorIs(t, err, test.targetErr)
		})
	}
}

func TestAuditRecordImporter_Import_WrapsReadFailure(t *testing.T) {
	fixture := newAuditRecordFixture(t)
	readErr := fmt.Errorf("audit source unavailable")
	_, err := NewAuditRecordImporter(&failingArtifactReader{err: readErr}, fixture.binding).Import(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceImporterFailed)
	assert.ErrorIs(t, err, readErr)
}

func TestAuditRecordImporter_Import_RespectsCancellation(t *testing.T) {
	fixture := newAuditRecordFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewAuditRecordImporter(fixture.reader, fixture.binding).Import(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestAuditRecordImporter_Import_ProducesValidGraph(t *testing.T) {
	fixture := newAuditRecordFixture(t)
	nodes, err := NewAuditRecordImporter(fixture.reader, fixture.binding).Import(context.Background())
	require.NoError(t, err)
	graph := NewEvidenceGraph(constants.DemoRunMaxArtifactBytes, []string{constants.MediaTypeJSON})
	for _, node := range nodes {
		require.NoError(t, graph.AddNode(node))
	}
	graph.ValidateAll(time.Unix(1_600_000_000, 0).UTC(), time.Unix(1_800_000_000, 0).UTC())
	assert.True(t, graph.Valid(), "graph failures: %v", graph.Failures())
	assert.Equal(t, 1, graph.NodeCount())
}

func TestAuditRecordImporter_Import_PreservesMissingArtifactError(t *testing.T) {
	fixture := newAuditRecordFixture(t)
	fixture.reader.files = map[string][]byte{}
	_, err := NewAuditRecordImporter(fixture.reader, fixture.binding).Import(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrEvidenceImporterFailed))
	assert.True(t, errors.Is(err, constants.ErrNotFound))
}
