package evidence

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
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

type attestationImporterFixture struct {
	reader     *memoryArtifactReader
	trust      *assessedSignerStub
	privateKey ed25519.PrivateKey
	records    []attestationRecord
	body       []byte
	binding    AttestationImportBinding
	now        time.Time
}

func newAttestationImporterFixture(t *testing.T) *attestationImporterFixture {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyID := hex.EncodeToString(publicKey)
	fixture := &attestationImporterFixture{
		reader:     &memoryArtifactReader{files: map[string][]byte{}},
		trust:      &assessedSignerStub{keys: map[string]ed25519.PublicKey{keyID: publicKey}},
		privateKey: privateKey,
		now:        time.Date(2026, 9, 6, 10, 30, 0, 0, time.UTC),
		records: []attestationRecord{
			{
				SchemaVersion: constants.AttestationSchemaVersion, AttestationID: "customer-attestation-1", AttesterType: attesterTypeCustomer,
				AttesterIdentity: "customer-1", SignerKeyID: keyID, IssuedAtUTC: "2026-09-06T10:00:00Z", ValidFromUTC: "2026-09-06T10:00:00Z",
				ValidUntilUTC: "2026-09-06T11:00:00Z", ScopeID: "scope-1", RunID: "run-1", AssertionIDs: []string{"assertion-1"}, Statement: "Customer-operated control is in effect.",
			},
			{
				SchemaVersion: constants.AttestationSchemaVersion, AttestationID: "assessor-attestation-1", AttesterType: attesterTypeAssessor,
				AttesterIdentity: "assessor-1", SignerKeyID: keyID, IssuedAtUTC: "2026-09-06T10:01:00Z", ValidFromUTC: "2026-09-06T10:01:00Z",
				ValidUntilUTC: "2026-09-06T11:00:00Z", ScopeID: "scope-1", RunID: "run-1", AssertionIDs: []string{"assertion-2"}, Statement: "Assessor reviewed the declared evidence.",
			},
		},
		binding: AttestationImportBinding{Path: constants.ComplianceBundleAttestationsFilename, ScopeID: "scope-1", RunID: "run-1"},
	}
	fixture.replaceRecords(t)
	return fixture
}

func (f *attestationImporterFixture) replaceRecords(t *testing.T) {
	t.Helper()
	for index := range f.records {
		f.records[index].Signature = ""
		payload, err := canonicalAttestationPayload(f.records[index])
		require.NoError(t, err)
		f.records[index].Signature = hex.EncodeToString(ed25519.Sign(f.privateKey, payload))
	}
	f.writeRecords(t)
}

func (f *attestationImporterFixture) writeRecords(t *testing.T) {
	t.Helper()
	lines := make([][]byte, 0, len(f.records))
	for _, record := range f.records {
		line, err := json.Marshal(record)
		require.NoError(t, err)
		lines = append(lines, line)
	}
	f.body = append(bytes.Join(lines, []byte{'\n'}), '\n')
	f.reader.files[f.binding.Path] = f.body
	f.binding.Reference = ContentReferenceForBody(constants.AttestationCollectionReferencePrefix, f.body)
}

func (f *attestationImporterFixture) importer() *AttestationImporter {
	importer := NewAttestationImporter(f.reader, f.trust, f.binding)
	importer.nowFunc = func() time.Time { return f.now }
	return importer
}

func TestAttestationImporter_SourceID(t *testing.T) {
	fixture := newAttestationImporterFixture(t)
	assert.Equal(t, "attestation", fixture.importer().SourceID())
}

func TestAttestationImporter_Import_VerifiesCanonicalCustomerAndAssessorAttestations(t *testing.T) {
	fixture := newAttestationImporterFixture(t)
	nodes, err := fixture.importer().Import(context.Background())
	require.NoError(t, err)
	require.Len(t, nodes, 2)

	customer := nodes[0]
	assert.Equal(t, ArtifactTypeCustomerAttestation, customer.ArtifactType)
	assert.Equal(t, ContentReferenceForBody(constants.CustomerAttestationReferencePrefix, customer.CanonicalBytes), customer.ArtifactID)
	assert.Equal(t, "g8e.evidence.CustomerAttestation@"+constants.AttestationSchemaVersion, customer.SchemaRef)
	assert.Equal(t, fixture.records[0].AttesterIdentity, customer.ProducerIdentity)
	assert.Equal(t, fixture.binding.ScopeID, customer.ScopeID)
	assert.Equal(t, fixture.binding.RunID, customer.RunID)
	assert.Equal(t, VerificationStatusVerified, customer.VerificationStatus)
	assert.Equal(t, constants.AttestationEvidenceVerifierID, customer.VerifierID)
	assert.Equal(t, constants.AttestationEvidenceVerifierVersion, customer.VerifierVersion)
	assert.Equal(t, fixture.now, customer.VerifiedAt)
	assert.Equal(t, time.Date(2026, 9, 6, 10, 0, 0, 0, time.UTC), customer.ProducedAt)
	assert.Equal(t, fixture.binding.Path, customer.BundlePath)
	assert.Empty(t, customer.References)

	assessor := nodes[1]
	assert.Equal(t, ArtifactTypeAssessorAttestation, assessor.ArtifactType)
	assert.Equal(t, ContentReferenceForBody(constants.AssessorAttestationReferencePrefix, assessor.CanonicalBytes), assessor.ArtifactID)
	assert.Equal(t, "g8e.evidence.AssessorAttestation@"+constants.AttestationSchemaVersion, assessor.SchemaRef)
	assert.Equal(t, VerificationStatusVerified, assessor.VerificationStatus)
}

func TestAttestationImporter_Import_PreservesUnverifiedStatusWithoutAssessedKey(t *testing.T) {
	fixture := newAttestationImporterFixture(t)
	fixture.trust.keys = map[string]ed25519.PublicKey{}
	nodes, err := fixture.importer().Import(context.Background())
	require.NoError(t, err)
	require.Len(t, nodes, 2)
	for _, node := range nodes {
		assert.Equal(t, VerificationStatusUnverified, node.VerificationStatus)
		assert.Empty(t, node.VerifierID)
		assert.True(t, node.VerifiedAt.IsZero())
	}
}

func TestAttestationImporter_Import_MarksInvalidSignatureTimeAndRevocationFailed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*attestationImporterFixture)
	}{
		{name: "invalid signature", mutate: func(f *attestationImporterFixture) {
			f.records[0].Signature = strings.Repeat("0", ed25519.SignatureSize*2)
		}},
		{name: "not yet valid", mutate: func(f *attestationImporterFixture) { f.now = time.Date(2026, 9, 6, 9, 59, 59, 0, time.UTC) }},
		{name: "expired", mutate: func(f *attestationImporterFixture) { f.now = time.Date(2026, 9, 6, 11, 0, 1, 0, time.UTC) }},
		{name: "revoked", mutate: func(f *attestationImporterFixture) {
			f.records[0].Revoked = true
			f.records[0].RevokedAtUTC = "2026-09-06T10:15:00Z"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAttestationImporterFixture(t)
			test.mutate(fixture)
			if test.name == "invalid signature" {
				fixture.writeRecords(t)
			} else {
				fixture.replaceRecords(t)
			}
			nodes, err := fixture.importer().Import(context.Background())
			require.NoError(t, err)
			require.NotEmpty(t, nodes)
			assert.Equal(t, VerificationStatusFailed, nodes[0].VerificationStatus)
			assert.Equal(t, constants.AttestationEvidenceVerifierID, nodes[0].VerifierID)
		})
	}
}

func TestAttestationImporter_Import_RejectsInvalidConfiguration(t *testing.T) {
	fixture := newAttestationImporterFixture(t)
	tests := []struct {
		name     string
		importer *AttestationImporter
	}{
		{name: "nil importer", importer: nil},
		{name: "nil reader", importer: NewAttestationImporter(nil, fixture.trust, fixture.binding)},
		{name: "nil trust", importer: NewAttestationImporter(fixture.reader, nil, fixture.binding)},
		{name: "invalid reference", importer: NewAttestationImporter(fixture.reader, fixture.trust, AttestationImportBinding{Reference: "invalid", Path: fixture.binding.Path, ScopeID: fixture.binding.ScopeID, RunID: fixture.binding.RunID})},
		{name: "unsafe path", importer: NewAttestationImporter(fixture.reader, fixture.trust, AttestationImportBinding{Reference: fixture.binding.Reference, Path: filepath.Join(constants.PathParentDir, constants.ComplianceBundleAttestationsFilename), ScopeID: fixture.binding.ScopeID, RunID: fixture.binding.RunID})},
		{name: "empty scope", importer: NewAttestationImporter(fixture.reader, fixture.trust, AttestationImportBinding{Reference: fixture.binding.Reference, Path: fixture.binding.Path, RunID: fixture.binding.RunID})},
		{name: "empty run", importer: NewAttestationImporter(fixture.reader, fixture.trust, AttestationImportBinding{Reference: fixture.binding.Reference, Path: fixture.binding.Path, ScopeID: fixture.binding.ScopeID})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.importer.Import(context.Background())
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
		})
	}
}

func TestAttestationImporter_Import_RejectsMalformedRecords(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*attestationImporterFixture)
	}{
		{name: "empty collection", mutate: func(f *attestationImporterFixture) { f.records = nil }},
		{name: "unsupported schema", mutate: func(f *attestationImporterFixture) { f.records[0].SchemaVersion = "2.0.0" }},
		{name: "empty attestation identity", mutate: func(f *attestationImporterFixture) { f.records[0].AttestationID = "" }},
		{name: "unsupported attester type", mutate: func(f *attestationImporterFixture) { f.records[0].AttesterType = "operator" }},
		{name: "empty attester identity", mutate: func(f *attestationImporterFixture) { f.records[0].AttesterIdentity = "" }},
		{name: "empty signer key", mutate: func(f *attestationImporterFixture) { f.records[0].SignerKeyID = "" }},
		{name: "invalid issued timestamp", mutate: func(f *attestationImporterFixture) { f.records[0].IssuedAtUTC = "invalid" }},
		{name: "invalid validity interval", mutate: func(f *attestationImporterFixture) { f.records[0].ValidUntilUTC = f.records[0].ValidFromUTC }},
		{name: "issued after validity begins", mutate: func(f *attestationImporterFixture) { f.records[0].IssuedAtUTC = "2026-09-06T10:01:00Z" }},
		{name: "scope mismatch", mutate: func(f *attestationImporterFixture) { f.records[0].ScopeID = "scope-2" }},
		{name: "run mismatch", mutate: func(f *attestationImporterFixture) { f.records[0].RunID = "run-2" }},
		{name: "empty assertions", mutate: func(f *attestationImporterFixture) { f.records[0].AssertionIDs = nil }},
		{name: "duplicate assertion", mutate: func(f *attestationImporterFixture) {
			f.records[0].AssertionIDs = []string{"assertion-1", "assertion-1"}
		}},
		{name: "empty statement", mutate: func(f *attestationImporterFixture) { f.records[0].Statement = "" }},
		{name: "revoked without timestamp", mutate: func(f *attestationImporterFixture) { f.records[0].Revoked = true }},
		{name: "not revoked with timestamp", mutate: func(f *attestationImporterFixture) { f.records[0].RevokedAtUTC = "2026-09-06T10:15:00Z" }},
		{name: "revoked before issuance", mutate: func(f *attestationImporterFixture) {
			f.records[0].Revoked = true
			f.records[0].RevokedAtUTC = "2026-09-06T09:59:59Z"
		}},
		{name: "missing signature", mutate: func(f *attestationImporterFixture) { f.records[0].Signature = "" }},
		{name: "duplicate attestation identity", mutate: func(f *attestationImporterFixture) { f.records[1].AttestationID = f.records[0].AttestationID }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAttestationImporterFixture(t)
			test.mutate(fixture)
			if test.name == "missing signature" {
				fixture.writeRecords(t)
			} else {
				fixture.replaceRecords(t)
			}
			_, err := fixture.importer().Import(context.Background())
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
		})
	}
}

func TestAttestationImporter_Import_RejectsDigestNoncanonicalAndUnknownFields(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*attestationImporterFixture)
		targetErr error
	}{
		{name: "source digest mismatch", mutate: func(f *attestationImporterFixture) {
			f.binding.Reference = constants.AttestationCollectionReferencePrefix + ":sha256:" + strings.Repeat("0", 64)
		}, targetErr: constants.ErrChecksumMismatch},
		{name: "noncanonical record", mutate: func(f *attestationImporterFixture) {
			f.body = bytes.Replace(f.body, []byte("}\n"), []byte("} \n"), 1)
			f.reader.files[f.binding.Path] = f.body
			f.binding.Reference = ContentReferenceForBody(constants.AttestationCollectionReferencePrefix, f.body)
		}, targetErr: constants.ErrEvidenceArtifactMalformed},
		{name: "unknown field", mutate: func(f *attestationImporterFixture) {
			f.body = bytes.Replace(f.body, []byte("}\n"), []byte(",\"unknown\":true}\n"), 1)
			f.reader.files[f.binding.Path] = f.body
			f.binding.Reference = ContentReferenceForBody(constants.AttestationCollectionReferencePrefix, f.body)
		}, targetErr: constants.ErrEvidenceArtifactMalformed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAttestationImporterFixture(t)
			test.mutate(fixture)
			_, err := fixture.importer().Import(context.Background())
			require.Error(t, err)
			assert.ErrorIs(t, err, test.targetErr)
		})
	}
}

func TestAttestationImporter_Import_EnforcesRecordLimit(t *testing.T) {
	fixture := newAttestationImporterFixture(t)
	templates := append([]attestationRecord(nil), fixture.records...)
	fixture.records = make([]attestationRecord, constants.AttestationMaxRecords+1)
	for index := range fixture.records {
		fixture.records[index] = templates[index%len(templates)]
		fixture.records[index].AttestationID = fmt.Sprintf("attestation-%d", index)
	}
	fixture.replaceRecords(t)
	_, err := fixture.importer().Import(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceArtifactTooLarge)
}

func TestAttestationImporter_Import_PropagatesTrustAndReadFailures(t *testing.T) {
	fixture := newAttestationImporterFixture(t)
	trustErr := fmt.Errorf("attestation trust unavailable")
	fixture.trust.err = trustErr
	_, err := fixture.importer().Import(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceTrustNotAssessed)
	assert.ErrorIs(t, err, trustErr)

	readErr := fmt.Errorf("attestation source unavailable")
	_, err = NewAttestationImporter(&failingArtifactReader{err: readErr}, fixture.trust, fixture.binding).Import(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceImporterFailed)
	assert.ErrorIs(t, err, readErr)
}

func TestAttestationImporter_Import_RespectsCancellation(t *testing.T) {
	fixture := newAttestationImporterFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := fixture.importer().Import(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestAttestationImporter_Import_ProducesValidGraph(t *testing.T) {
	fixture := newAttestationImporterFixture(t)
	nodes, err := fixture.importer().Import(context.Background())
	require.NoError(t, err)
	graph := NewEvidenceGraph(constants.DemoRunMaxArtifactBytes, []string{constants.MediaTypeJSON})
	for _, node := range nodes {
		require.NoError(t, graph.AddNode(node))
	}
	graph.ValidateAll(time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC), time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC))
	assert.True(t, graph.Valid(), "graph failures: %v", graph.Failures())
	assert.Equal(t, 2, graph.NodeCount())
}
