package evidence

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type commitmentFixture struct {
	reader      *memoryArtifactReader
	trust       *assessedSignerStub
	attestation *operatorv1.CommitmentAttestation
	body        []byte
	binding     CommitmentImportBinding
}

func newCommitmentFixture(t *testing.T) *commitmentFixture {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyID := hex.EncodeToString(publicKey)
	attestation := &operatorv1.CommitmentAttestation{
		TransactionId:               "transaction-1",
		TransactionHash:             strings.Repeat("1", 64),
		PriorCommitmentHash:         strings.Repeat("2", 64),
		StateRootAtCommit:           strings.Repeat("3", 64),
		L2SignatureDigest:           strings.Repeat("4", 64),
		WardenIntentSignatureDigest: strings.Repeat("5", 64),
		HumanSignatureDigest:        strings.Repeat("6", 64),
		ActionType:                  "FILE_EDIT",
		TargetResource:              constants.DemosTargetDataDir,
		CommittedAtUnixMs:           1_700_000_000_000,
		AuditorKeyId:                keyID,
	}
	payload, err := governance.CanonicalizeCommitmentAttestation(attestation)
	require.NoError(t, err)
	digest := sha256.Sum256(payload)
	attestation.Hash = hex.EncodeToString(digest[:])
	attestation.Signature = hex.EncodeToString(ed25519.Sign(privateKey, payload))
	body, err := compliancev1.MarshalCanonical(attestation)
	require.NoError(t, err)
	reference := ContentReferenceForBody(constants.CommitmentReferencePrefix, body)
	path := filepath.Join(constants.CommitmentsDirname, digestHex(body)+constants.FileExtJSON)
	return &commitmentFixture{
		reader:      &memoryArtifactReader{files: map[string][]byte{path: body}},
		trust:       &assessedSignerStub{keys: map[string]ed25519.PublicKey{keyID: publicKey}},
		attestation: attestation,
		body:        body,
		binding: CommitmentImportBinding{
			Reference:     reference,
			Path:          path,
			ScopeID:       "scope-1",
			RunID:         "run-1",
			AttemptID:     "attempt-1",
			ScenarioID:    "scenario-1",
			TransactionID: attestation.TransactionId,
		},
	}
}

func (f *commitmentFixture) replaceAttestation(t *testing.T) {
	t.Helper()
	body, err := compliancev1.MarshalCanonical(f.attestation)
	require.NoError(t, err)
	f.body = body
	f.reader.files[f.binding.Path] = body
	f.binding.Reference = ContentReferenceForBody(constants.CommitmentReferencePrefix, body)
}

func TestCommitmentImporter_SourceID(t *testing.T) {
	fixture := newCommitmentFixture(t)
	assert.Equal(t, "commitment", NewCommitmentImporter(fixture.reader, fixture.trust, fixture.binding).SourceID())
}

func TestCommitmentImporter_Import_VerifiesCanonicalAttestation(t *testing.T) {
	fixture := newCommitmentFixture(t)
	importer := NewCommitmentImporter(fixture.reader, fixture.trust, fixture.binding)
	importer.nowFunc = func() time.Time { return time.Unix(1_700_000_100, 0).UTC() }
	nodes, err := importer.Import(context.Background())
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	node := nodes[0]
	assert.Equal(t, fixture.binding.Reference, node.ArtifactID)
	assert.Equal(t, ArtifactTypeCommitment, node.ArtifactType)
	assert.Equal(t, fixture.attestation.AuditorKeyId, node.ProducerIdentity)
	assert.Equal(t, fixture.binding.TransactionID, node.TransactionID)
	assert.Equal(t, fixture.binding.ScopeID, node.ScopeID)
	assert.Equal(t, fixture.binding.RunID, node.RunID)
	assert.Equal(t, fixture.binding.AttemptID, node.AttemptID)
	assert.Equal(t, fixture.binding.ScenarioID, node.ScenarioID)
	assert.Equal(t, VerificationStatusVerified, node.VerificationStatus)
	assert.Equal(t, constants.CommitmentEvidenceVerifierID, node.VerifierID)
	assert.Equal(t, constants.CommitmentEvidenceVerifierVersion, node.VerifierVersion)
	assert.Equal(t, time.UnixMilli(fixture.attestation.CommittedAtUnixMs), node.ProducedAt)
	assert.Equal(t, time.Unix(1_700_000_100, 0).UTC(), node.VerifiedAt)
	assert.Equal(t, fixture.body, node.CanonicalBytes)
	assert.Empty(t, node.References)
}

func TestCommitmentImporter_Import_PreservesUnverifiedStatusWithoutAssessedKey(t *testing.T) {
	fixture := newCommitmentFixture(t)
	fixture.trust.keys = map[string]ed25519.PublicKey{}
	nodes, err := NewCommitmentImporter(fixture.reader, fixture.trust, fixture.binding).Import(context.Background())
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, VerificationStatusUnverified, nodes[0].VerificationStatus)
	assert.Empty(t, nodes[0].VerifierID)
}

func TestCommitmentImporter_Import_MarksCryptographicMutationsFailed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*commitmentFixture)
	}{
		{name: "signature mismatch", mutate: func(f *commitmentFixture) { f.attestation.Signature = strings.Repeat("0", ed25519.SignatureSize*2) }},
		{name: "payload hash mismatch", mutate: func(f *commitmentFixture) { f.attestation.Hash = strings.Repeat("0", sha256.Size*2) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCommitmentFixture(t)
			test.mutate(fixture)
			fixture.replaceAttestation(t)
			nodes, err := NewCommitmentImporter(fixture.reader, fixture.trust, fixture.binding).Import(context.Background())
			require.NoError(t, err)
			require.Len(t, nodes, 1)
			assert.Equal(t, VerificationStatusFailed, nodes[0].VerificationStatus)
			assert.Equal(t, constants.CommitmentEvidenceVerifierID, nodes[0].VerifierID)
		})
	}
}

func TestCommitmentImporter_Import_RejectsInvalidConfiguration(t *testing.T) {
	fixture := newCommitmentFixture(t)
	tests := []struct {
		name     string
		importer *CommitmentImporter
	}{
		{name: "nil importer", importer: nil},
		{name: "nil reader", importer: NewCommitmentImporter(nil, fixture.trust, fixture.binding)},
		{name: "nil trust", importer: NewCommitmentImporter(fixture.reader, nil, fixture.binding)},
		{name: "invalid reference", importer: NewCommitmentImporter(fixture.reader, fixture.trust, CommitmentImportBinding{Reference: "invalid", Path: fixture.binding.Path, ScopeID: fixture.binding.ScopeID, RunID: fixture.binding.RunID, TransactionID: fixture.binding.TransactionID})},
		{name: "unsafe path", importer: NewCommitmentImporter(fixture.reader, fixture.trust, CommitmentImportBinding{Reference: fixture.binding.Reference, Path: filepath.Join(constants.PathParentDir, constants.CommitmentsDirname), ScopeID: fixture.binding.ScopeID, RunID: fixture.binding.RunID, TransactionID: fixture.binding.TransactionID})},
		{name: "empty scope", importer: NewCommitmentImporter(fixture.reader, fixture.trust, CommitmentImportBinding{Reference: fixture.binding.Reference, Path: fixture.binding.Path, RunID: fixture.binding.RunID, TransactionID: fixture.binding.TransactionID})},
		{name: "empty run", importer: NewCommitmentImporter(fixture.reader, fixture.trust, CommitmentImportBinding{Reference: fixture.binding.Reference, Path: fixture.binding.Path, ScopeID: fixture.binding.ScopeID, TransactionID: fixture.binding.TransactionID})},
		{name: "empty transaction", importer: NewCommitmentImporter(fixture.reader, fixture.trust, CommitmentImportBinding{Reference: fixture.binding.Reference, Path: fixture.binding.Path, ScopeID: fixture.binding.ScopeID, RunID: fixture.binding.RunID})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.importer.Import(context.Background())
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
		})
	}
}

func TestCommitmentImporter_Import_RejectsContentAndBindingMutations(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*testing.T, *commitmentFixture)
		targetErr error
	}{
		{name: "artifact digest mismatch", mutate: func(_ *testing.T, f *commitmentFixture) {
			f.binding.Reference = constants.CommitmentReferencePrefix + ":sha256:" + strings.Repeat("0", sha256.Size*2)
		}, targetErr: constants.ErrChecksumMismatch},
		{name: "noncanonical protojson", mutate: func(_ *testing.T, f *commitmentFixture) {
			body := append(append([]byte{}, f.body...), '\n')
			f.reader.files[f.binding.Path] = body
			f.binding.Reference = ContentReferenceForBody(constants.CommitmentReferencePrefix, body)
		}, targetErr: constants.ErrEvidenceArtifactMalformed},
		{name: "incomplete attestation", mutate: func(t *testing.T, f *commitmentFixture) {
			f.attestation.TransactionHash = ""
			f.replaceAttestation(t)
		}, targetErr: constants.ErrEvidenceArtifactMalformed},
		{name: "transaction mismatch", mutate: func(_ *testing.T, f *commitmentFixture) { f.binding.TransactionID = "transaction-2" }, targetErr: constants.ErrEvidenceScopeMismatch},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCommitmentFixture(t)
			test.mutate(t, fixture)
			_, err := NewCommitmentImporter(fixture.reader, fixture.trust, fixture.binding).Import(context.Background())
			require.Error(t, err)
			assert.ErrorIs(t, err, test.targetErr)
		})
	}
}

func TestCommitmentImporter_Import_PropagatesTrustAssessmentFailure(t *testing.T) {
	fixture := newCommitmentFixture(t)
	trustErr := fmt.Errorf("commitment trust unavailable")
	fixture.trust.err = trustErr
	_, err := NewCommitmentImporter(fixture.reader, fixture.trust, fixture.binding).Import(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceTrustNotAssessed)
	assert.ErrorIs(t, err, trustErr)
}

func TestCommitmentImporter_Import_WrapsReadFailure(t *testing.T) {
	fixture := newCommitmentFixture(t)
	readErr := fmt.Errorf("commitment source unavailable")
	_, err := NewCommitmentImporter(&failingArtifactReader{err: readErr}, fixture.trust, fixture.binding).Import(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceImporterFailed)
	assert.ErrorIs(t, err, readErr)
}

func TestCommitmentImporter_Import_RespectsCancellation(t *testing.T) {
	fixture := newCommitmentFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewCommitmentImporter(fixture.reader, fixture.trust, fixture.binding).Import(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestCommitmentImporter_Import_ProducesValidGraph(t *testing.T) {
	fixture := newCommitmentFixture(t)
	nodes, err := NewCommitmentImporter(fixture.reader, fixture.trust, fixture.binding).Import(context.Background())
	require.NoError(t, err)
	graph := NewEvidenceGraph(constants.DemoRunMaxArtifactBytes, []string{constants.MediaTypeJSON})
	for _, node := range nodes {
		require.NoError(t, graph.AddNode(node))
	}
	graph.ValidateAll(time.Unix(1_600_000_000, 0).UTC(), time.Unix(1_800_000_000, 0).UTC())
	assert.True(t, graph.Valid(), "graph failures: %v", graph.Failures())
	assert.Equal(t, 1, graph.NodeCount())
}
