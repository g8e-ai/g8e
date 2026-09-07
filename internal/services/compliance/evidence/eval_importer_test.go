package evidence

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type evalImporterFixture struct {
	reader    *memoryArtifactReader
	runID     string
	runDir    string
	suiteID   string
	attemptID string
	taskID    string
}

type evalFixtureManifest struct {
	SchemaVersion       string    `json:"schema_version"`
	RunID               string    `json:"run_id"`
	SuiteID             string    `json:"suite_id"`
	SuiteVersion        string    `json:"suite_version"`
	CreatedAt           time.Time `json:"created_at"`
	OrchestratorVersion string    `json:"orchestrator_version"`
}

type evalFixtureTask struct {
	SchemaVersion string `json:"schema_version"`
	TaskID        string `json:"task_id"`
	SuiteID       string `json:"suite_id"`
	SuiteVersion  string `json:"suite_version"`
	PromptHash    string `json:"prompt_hash"`
}

type evalFixtureAttempt struct {
	SchemaVersion string    `json:"schema_version"`
	AttemptID     string    `json:"attempt_id"`
	RunID         string    `json:"run_id"`
	TaskID        string    `json:"task_id"`
	ArmID         string    `json:"arm_id"`
	StartedAt     time.Time `json:"started_at"`
	EndedAt       time.Time `json:"ended_at"`
	ReceiptRefs   []string  `json:"receipt_refs"`
	GradeRefs     []string  `json:"grade_refs"`
}

type evalFixtureMetric struct {
	SchemaVersion      string   `json:"schema_version"`
	MetricID           string   `json:"metric_id"`
	MetricVersion      string   `json:"metric_version"`
	AttemptID          string   `json:"attempt_id"`
	RunID              string   `json:"run_id"`
	ArmID              string   `json:"arm_id"`
	TaskID             string   `json:"task_id"`
	Value              float64  `json:"value"`
	Unit               string   `json:"unit"`
	Eligible           bool     `json:"eligible"`
	VerificationStatus string   `json:"verification_status"`
	GraderClass        string   `json:"grader_class"`
	EvidenceRefs       []string `json:"evidence_refs"`
}

type evalFixtureStage struct {
	SchemaVersion string `json:"schema_version"`
	StageID       string `json:"stage_id"`
	AttemptID     string `json:"attempt_id"`
	RunID         string `json:"run_id"`
	Kind          string `json:"kind"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
}

type evalFixtureReceipt struct {
	SchemaVersion string          `json:"schema_version"`
	ReceiptID     string          `json:"receipt_id"`
	AttemptID     string          `json:"attempt_id"`
	RunID         string          `json:"run_id"`
	TransactionID string          `json:"transaction_id"`
	ActionType    string          `json:"action_type"`
	Primary       bool            `json:"primary"`
	Verified      bool            `json:"verified"`
	ActionReceipt json.RawMessage `json:"action_receipt"`
}

type evalFixtureEvidenceIndex struct {
	SchemaVersion         string                    `json:"schema_version"`
	ArtifactID            string                    `json:"artifact_id"`
	RunID                 string                    `json:"run_id"`
	AttemptID             string                    `json:"attempt_id,omitempty"`
	MediaType             string                    `json:"media_type"`
	SchemaRef             string                    `json:"schema_ref"`
	ByteLength            int                       `json:"byte_length"`
	SHA256                string                    `json:"sha256"`
	ProducerIdentity      string                    `json:"producer_identity"`
	PrivacyClassification string                    `json:"privacy_classification"`
	StorageLocation       string                    `json:"storage_location"`
	Encryption            *evalFixtureEncryption    `json:"encryption,omitempty"`
	AccessControl         *evalFixtureAccessControl `json:"access_control,omitempty"`
	ParentEvidenceRefs    []string                  `json:"parent_evidence_refs"`
}

type evalFixtureEncryption struct {
	Algorithm            string `json:"algorithm"`
	KeyID                string `json:"key_id"`
	AADSHA256            string `json:"aad_sha256"`
	CiphertextSHA256     string `json:"ciphertext_sha256"`
	CiphertextByteLength int    `json:"ciphertext_byte_length"`
}

type evalFixtureAccessControl struct {
	Policy             string `json:"policy"`
	AuthorizationScope string `json:"authorization_scope"`
}

type evalFixtureEncryptedEnvelope struct {
	Version       int    `json:"version"`
	Algorithm     string `json:"algorithm"`
	KeyID         string `json:"key_id"`
	NonceBase64   string `json:"nonce_b64"`
	CiphertextB64 string `json:"ciphertext_b64"`
}

type evalFixtureAAD struct {
	AccessControl         *evalFixtureAccessControl `json:"access_control"`
	ArtifactID            string                    `json:"artifact_id"`
	AttemptID             string                    `json:"attempt_id"`
	ByteLength            int                       `json:"byte_length"`
	MediaType             string                    `json:"media_type"`
	PrivacyClassification string                    `json:"privacy_classification"`
	RunID                 string                    `json:"run_id"`
	SchemaRef             string                    `json:"schema_ref"`
	SHA256                string                    `json:"sha256"`
	StorageLocation       string                    `json:"storage_location"`
}

type evalFixtureRecord interface {
	evalFixtureRecord()
}

func (evalFixtureTask) evalFixtureRecord()          {}
func (evalFixtureAttempt) evalFixtureRecord()       {}
func (evalFixtureMetric) evalFixtureRecord()        {}
func (evalFixtureStage) evalFixtureRecord()         {}
func (evalFixtureReceipt) evalFixtureRecord()       {}
func (evalFixtureEvidenceIndex) evalFixtureRecord() {}

func newEvalImporterFixture(t *testing.T) *evalImporterFixture {
	t.Helper()
	fix := &evalImporterFixture{
		reader:    &memoryArtifactReader{files: map[string][]byte{}},
		runID:     "eval-run-1",
		suiteID:   "synthetic-suite",
		attemptID: "attempt-1",
		taskID:    "task-1",
	}
	fix.runDir = filepath.Join(constants.EvalRunsDirname, fix.runID)
	createdAt := time.Unix(1_700_000_000, 0).UTC()
	fix.writeManifest(t, evalFixtureManifest{SchemaVersion: "1.40.0", RunID: fix.runID, SuiteID: fix.suiteID, SuiteVersion: "1.0.0", CreatedAt: createdAt, OrchestratorVersion: "g8e-evals-test"})
	fix.writeJSONL(t, constants.EvalRunTasksFilename, evalFixtureTask{SchemaVersion: "1.40.0", TaskID: fix.taskID, SuiteID: fix.suiteID, SuiteVersion: "1.0.0", PromptHash: "prompt-sha256"})
	fix.writeJSONL(t, constants.EvalRunAttemptsFilename, evalFixtureAttempt{SchemaVersion: "1.40.0", AttemptID: fix.attemptID, RunID: fix.runID, TaskID: fix.taskID, ArmID: "direct", StartedAt: createdAt.Add(time.Second), EndedAt: createdAt.Add(2 * time.Second), ReceiptRefs: []string{}, GradeRefs: []string{"metric-pass"}})
	fix.writeJSONL(t, constants.EvalRunMetricsFilename, evalFixtureMetric{SchemaVersion: "1.40.0", MetricID: "metric-pass", MetricVersion: "1.0.0", AttemptID: fix.attemptID, RunID: fix.runID, ArmID: "direct", TaskID: fix.taskID, Value: 1, Unit: "boolean", Eligible: true, VerificationStatus: "verified", GraderClass: "deterministic", EvidenceRefs: []string{}})
	fix.writeJSONL(t, constants.EvalRunStagesFilename)
	fix.writeJSONL(t, constants.EvalRunReceiptsFilename)
	fix.writeJSONL(t, constants.EvalRunEvidenceIndexFilename)
	return fix
}

func (f *evalImporterFixture) path(parts ...string) string {
	return filepath.Join(append([]string{f.runDir}, parts...)...)
}

func (f *evalImporterFixture) writeManifest(t *testing.T, manifest evalFixtureManifest) {
	t.Helper()
	body, err := json.MarshalIndent(manifest, "", "  ")
	require.NoError(t, err)
	f.reader.files[f.path(constants.EvalRunManifestFilename)] = body
}

func (f *evalImporterFixture) readManifest(t *testing.T) evalFixtureManifest {
	t.Helper()
	var manifest evalFixtureManifest
	require.NoError(t, json.Unmarshal(f.reader.files[f.path(constants.EvalRunManifestFilename)], &manifest))
	return manifest
}

func (f *evalImporterFixture) writeJSONL(t *testing.T, filename string, records ...evalFixtureRecord) {
	t.Helper()
	body := make([]byte, 0)
	for _, record := range records {
		line, err := json.Marshal(record)
		require.NoError(t, err)
		body = append(body, line...)
		body = append(body, '\n')
	}
	f.reader.files[f.path(filename)] = body
}

func (f *evalImporterFixture) importer() *EvalBundleImporter {
	return NewEvalBundleImporter(f.reader, f.runID, f.runDir)
}

func (f *evalImporterFixture) addEvidence(t *testing.T, artifactID string, body []byte) evalFixtureEvidenceIndex {
	t.Helper()
	digest := digestHex(body)
	location := filepath.Join(constants.EvalRunEvidenceDirname, digest+constants.FileExtJSON)
	f.reader.files[f.path(location)] = body
	return evalFixtureEvidenceIndex{SchemaVersion: "1.40.0", ArtifactID: artifactID, RunID: f.runID, AttemptID: f.attemptID, MediaType: constants.MediaTypeJSON, SchemaRef: "test.evidence.v1", ByteLength: len(body), SHA256: digest, ProducerIdentity: "test-producer", PrivacyClassification: "internal", StorageLocation: location, ParentEvidenceRefs: []string{}}
}

func (f *evalImporterFixture) addEncryptedEvidence(t *testing.T, artifactID string, plaintext, ciphertext []byte) evalFixtureEvidenceIndex {
	t.Helper()
	plaintextDigest := digestHex(plaintext)
	location := filepath.Join(constants.EvalRunEvidenceDirname, plaintextDigest+constants.FileExtJSON+constants.EvalRunEncryptedSuffix)
	accessControl := &evalFixtureAccessControl{Policy: "named_key_holders", AuthorizationScope: constants.EvalRestrictedEvidenceScope}
	index := evalFixtureEvidenceIndex{SchemaVersion: "1.40.0", ArtifactID: artifactID, RunID: f.runID, AttemptID: f.attemptID, MediaType: constants.MediaTypeJSON, SchemaRef: "test.evidence.v1", ByteLength: len(plaintext), SHA256: plaintextDigest, ProducerIdentity: "test-producer", PrivacyClassification: "restricted", StorageLocation: location, AccessControl: accessControl, ParentEvidenceRefs: []string{}}
	aadBody, err := json.Marshal(evalFixtureAAD{AccessControl: accessControl, ArtifactID: artifactID, AttemptID: f.attemptID, ByteLength: len(plaintext), MediaType: constants.MediaTypeJSON, PrivacyClassification: "restricted", RunID: f.runID, SchemaRef: "test.evidence.v1", SHA256: plaintextDigest, StorageLocation: location})
	require.NoError(t, err)
	index.Encryption = &evalFixtureEncryption{Algorithm: constants.EvalEvidenceEncryptionAES256GCM, KeyID: "key-1", AADSHA256: digestHex(aadBody), CiphertextSHA256: digestHex(ciphertext), CiphertextByteLength: len(ciphertext)}
	envelope, err := json.Marshal(evalFixtureEncryptedEnvelope{Version: constants.EvalEncryptedEvidenceVersion, Algorithm: index.Encryption.Algorithm, KeyID: index.Encryption.KeyID, NonceBase64: base64.StdEncoding.EncodeToString(make([]byte, constants.EvalEncryptedEvidenceNonceBytes)), CiphertextB64: base64.StdEncoding.EncodeToString(ciphertext)})
	require.NoError(t, err)
	f.reader.files[f.path(location)] = envelope
	return index
}

func (f *evalImporterFixture) addSignedReceipt(t *testing.T) string {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signerKeyID := hex.EncodeToString(pub)
	receipt := &operatorv1.ActionReceipt{TransactionId: "tx-1", TransactionHash: "tx-hash", SignerKeyId: signerKeyID, ExecutedAtUnixMs: 1_700_000_001_000, DeterministicStageEvidence: []*operatorv1.DeterministicStageEvidence{{ActionType: "GOVERNANCE_ACTION"}}}
	payload, err := governance.CanonicalizeActionReceipt(receipt)
	require.NoError(t, err)
	receipt.Signature = hex.EncodeToString(ed25519.Sign(priv, payload))
	attestation := &operatorv1.ReceiptPersistenceAttestation{TransactionId: receipt.TransactionId, ReceiptSignatureDigest: governance.SignatureDigest([]string{receipt.Signature}), PersistedAtUnixMs: 1_700_000_002_000, AuditRecordId: receipt.TransactionId, SignerKeyId: signerKeyID}
	attestationPayload, err := governance.CanonicalizeReceiptPersistenceAttestation(attestation)
	require.NoError(t, err)
	attestation.Signature = hex.EncodeToString(ed25519.Sign(priv, attestationPayload))
	receipt.FinalPersistenceAttestation = attestation
	receiptBody, err := compliancev1.MarshalCanonical(receipt)
	require.NoError(t, err)
	receiptID := "receipt-1"
	wrapper := evalFixtureReceipt{SchemaVersion: "1.40.0", ReceiptID: receiptID, AttemptID: f.attemptID, RunID: f.runID, TransactionID: receipt.TransactionId, ActionType: "GOVERNANCE_ACTION", Primary: true, Verified: true, ActionReceipt: receiptBody}
	f.writeJSONL(t, constants.EvalRunReceiptsFilename, wrapper)
	attempt := evalFixtureAttempt{SchemaVersion: "1.40.0", AttemptID: f.attemptID, RunID: f.runID, TaskID: f.taskID, ArmID: "direct", StartedAt: time.Unix(1_700_000_001, 0).UTC(), EndedAt: time.Unix(1_700_000_002, 0).UTC(), ReceiptRefs: []string{receiptID}, GradeRefs: []string{"metric-pass"}}
	f.writeJSONL(t, constants.EvalRunAttemptsFilename, attempt)
	return receiptID
}

func TestEvalBundleImporter_SourceID(t *testing.T) {
	fix := newEvalImporterFixture(t)
	assert.Equal(t, "eval-bundle", fix.importer().SourceID())
}

func TestEvalBundleImporter_Import_AcceptsMinimalRun(t *testing.T) {
	fix := newEvalImporterFixture(t)
	nodes, err := fix.importer().Import(context.Background())
	require.NoError(t, err)
	assert.Len(t, nodes, 4)
	assert.Len(t, nodesByType(nodes, ArtifactTypeEvalManifest), 1)
	assert.Len(t, nodesByType(nodes, ArtifactTypeEvalTask), 1)
	assert.Len(t, nodesByType(nodes, ArtifactTypeEvalAttempt), 1)
	assert.Len(t, nodesByType(nodes, ArtifactTypeEvalMetric), 1)
}

func TestEvalBundleImporter_Import_RejectsInvalidConfiguration(t *testing.T) {
	fix := newEvalImporterFixture(t)
	tests := []struct {
		name     string
		importer *EvalBundleImporter
	}{
		{name: "nil reader", importer: NewEvalBundleImporter(nil, fix.runID, fix.runDir)},
		{name: "invalid run ID", importer: NewEvalBundleImporter(fix.reader, "../escape", fix.runDir)},
		{name: "invalid run directory", importer: NewEvalBundleImporter(fix.reader, fix.runID, "../escape")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.importer.Import(context.Background())
			assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
		})
	}
}

func TestEvalBundleImporter_Import_ReturnsCancelledContext(t *testing.T) {
	fix := newEvalImporterFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := fix.importer().Import(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestEvalBundleImporter_Import_RejectsMissingRequiredFiles(t *testing.T) {
	filenames := []string{constants.EvalRunManifestFilename, constants.EvalRunTasksFilename, constants.EvalRunAttemptsFilename, constants.EvalRunMetricsFilename, constants.EvalRunStagesFilename, constants.EvalRunReceiptsFilename, constants.EvalRunEvidenceIndexFilename}
	for _, filename := range filenames {
		t.Run(filename, func(t *testing.T) {
			fix := newEvalImporterFixture(t)
			delete(fix.reader.files, fix.path(filename))
			_, err := fix.importer().Import(context.Background())
			assert.ErrorIs(t, err, constants.ErrEvidenceImporterFailed)
		})
	}
}

func TestEvalBundleImporter_Import_RejectsMalformedRecords(t *testing.T) {
	filenames := []string{constants.EvalRunManifestFilename, constants.EvalRunTasksFilename, constants.EvalRunAttemptsFilename, constants.EvalRunMetricsFilename, constants.EvalRunStagesFilename, constants.EvalRunReceiptsFilename, constants.EvalRunEvidenceIndexFilename}
	for _, filename := range filenames {
		t.Run(filename, func(t *testing.T) {
			fix := newEvalImporterFixture(t)
			fix.reader.files[fix.path(filename)] = []byte(`{invalid}`)
			_, err := fix.importer().Import(context.Background())
			assert.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
		})
	}
}

func TestEvalBundleImporter_Import_RejectsNonCanonicalJSONL(t *testing.T) {
	fix := newEvalImporterFixture(t)
	fix.reader.files[fix.path(constants.EvalRunTasksFilename)] = []byte("{ \"schema_version\": \"1.40.0\" }\n")
	_, err := fix.importer().Import(context.Background())
	assert.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
}

func TestEvalBundleImporter_Import_RejectsEmptyRequiredCollections(t *testing.T) {
	filenames := []string{constants.EvalRunTasksFilename, constants.EvalRunAttemptsFilename, constants.EvalRunMetricsFilename}
	for _, filename := range filenames {
		t.Run(filename, func(t *testing.T) {
			fix := newEvalImporterFixture(t)
			fix.writeJSONL(t, filename)
			_, err := fix.importer().Import(context.Background())
			assert.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
		})
	}
}

func TestEvalBundleImporter_Import_RejectsManifestRunIDMismatch(t *testing.T) {
	fix := newEvalImporterFixture(t)
	manifest := fix.readManifest(t)
	manifest.RunID = "other-run"
	fix.writeManifest(t, manifest)
	_, err := fix.importer().Import(context.Background())
	assert.ErrorIs(t, err, constants.ErrEvidenceScopeMismatch)
}

func TestEvalBundleImporter_Import_RejectsSchemaVersionMismatch(t *testing.T) {
	fix := newEvalImporterFixture(t)
	fix.writeJSONL(t, constants.EvalRunTasksFilename, evalFixtureTask{SchemaVersion: "1.39.0", TaskID: fix.taskID, SuiteID: fix.suiteID, SuiteVersion: "1.0.0", PromptHash: "prompt-sha256"})
	_, err := fix.importer().Import(context.Background())
	assert.ErrorIs(t, err, constants.ErrEvidenceSchemaMismatch)
}

func TestEvalBundleImporter_Import_RejectsTaskSuiteMismatch(t *testing.T) {
	fix := newEvalImporterFixture(t)
	fix.writeJSONL(t, constants.EvalRunTasksFilename, evalFixtureTask{SchemaVersion: "1.40.0", TaskID: fix.taskID, SuiteID: "other-suite", SuiteVersion: "1.0.0", PromptHash: "prompt-sha256"})
	_, err := fix.importer().Import(context.Background())
	assert.ErrorIs(t, err, constants.ErrEvidenceScopeMismatch)
}

func TestEvalBundleImporter_Import_RejectsUnknownAttemptTask(t *testing.T) {
	fix := newEvalImporterFixture(t)
	fix.writeJSONL(t, constants.EvalRunAttemptsFilename, evalFixtureAttempt{SchemaVersion: "1.40.0", AttemptID: fix.attemptID, RunID: fix.runID, TaskID: "missing-task", ArmID: "direct", StartedAt: time.Now(), EndedAt: time.Now(), GradeRefs: []string{"metric-pass"}})
	_, err := fix.importer().Import(context.Background())
	assert.ErrorIs(t, err, constants.ErrUnresolvedReference)
}

func TestEvalBundleImporter_Import_RejectsMetricBindingMismatch(t *testing.T) {
	fix := newEvalImporterFixture(t)
	fix.writeJSONL(t, constants.EvalRunMetricsFilename, evalFixtureMetric{SchemaVersion: "1.40.0", MetricID: "metric-pass", MetricVersion: "1.0.0", AttemptID: "other-attempt", RunID: fix.runID, ArmID: "direct", TaskID: fix.taskID, Value: 1, Unit: "boolean", Eligible: true, VerificationStatus: "verified", GraderClass: "deterministic"})
	_, err := fix.importer().Import(context.Background())
	assert.ErrorIs(t, err, constants.ErrUnresolvedReference)
}

func TestEvalBundleImporter_Import_LoadsStage(t *testing.T) {
	fix := newEvalImporterFixture(t)
	fix.writeJSONL(t, constants.EvalRunStagesFilename, evalFixtureStage{SchemaVersion: "1.40.0", StageID: "stage-1", AttemptID: fix.attemptID, RunID: fix.runID, Kind: "model_inference", Provider: "provider", Model: "model"})
	nodes, err := fix.importer().Import(context.Background())
	require.NoError(t, err)
	stages := nodesByType(nodes, ArtifactTypeEvalStage)
	require.Len(t, stages, 1)
	assert.Equal(t, fix.attemptID, stages[0].AttemptID)
	assert.Equal(t, "provider/model", stages[0].ProducerIdentity)
}

func TestEvalBundleImporter_Import_LoadsSignedReceipt(t *testing.T) {
	fix := newEvalImporterFixture(t)
	receiptID := fix.addSignedReceipt(t)
	nodes, err := fix.importer().Import(context.Background())
	require.NoError(t, err)
	receipts := nodesByType(nodes, ArtifactTypeEvalReceipt)
	require.Len(t, receipts, 1)
	assert.Equal(t, VerificationStatusVerified, receipts[0].VerificationStatus)
	assert.Equal(t, "tx-1", receipts[0].TransactionID)
	attempts := nodesByType(nodes, ArtifactTypeEvalAttempt)
	require.Len(t, attempts, 1)
	assert.NotContains(t, attempts[0].References, receiptID)
	assert.Contains(t, attempts[0].References, receipts[0].ArtifactID)
}

func TestEvalBundleImporter_Import_MarksUnsignedReceiptFailed(t *testing.T) {
	fix := newEvalImporterFixture(t)
	receipt := &operatorv1.ActionReceipt{TransactionId: "tx-1", SignerKeyId: "invalid", DeterministicStageEvidence: []*operatorv1.DeterministicStageEvidence{{ActionType: "GOVERNANCE_ACTION"}}}
	body, err := compliancev1.MarshalCanonical(receipt)
	require.NoError(t, err)
	fix.writeJSONL(t, constants.EvalRunReceiptsFilename, evalFixtureReceipt{SchemaVersion: "1.40.0", ReceiptID: "receipt-1", AttemptID: fix.attemptID, RunID: fix.runID, TransactionID: "tx-1", ActionType: "GOVERNANCE_ACTION", ActionReceipt: body})
	nodes, err := fix.importer().Import(context.Background())
	require.NoError(t, err)
	receipts := nodesByType(nodes, ArtifactTypeEvalReceipt)
	require.Len(t, receipts, 1)
	assert.Equal(t, VerificationStatusFailed, receipts[0].VerificationStatus)
}

func TestEvalBundleImporter_Import_LoadsEvidenceIndexArtifact(t *testing.T) {
	fix := newEvalImporterFixture(t)
	index := fix.addEvidence(t, "evidence-1", []byte(`{"value":1}`))
	fix.writeJSONL(t, constants.EvalRunEvidenceIndexFilename, index)
	nodes, err := fix.importer().Import(context.Background())
	require.NoError(t, err)
	observations := nodesByType(nodes, ArtifactTypeEvalObservation)
	require.Len(t, observations, 1)
	assert.Equal(t, index.ArtifactID, observations[0].ArtifactID)
	assert.Equal(t, index.SHA256, observations[0].SHA256)
}

func TestEvalBundleImporter_Import_RejectsEvidenceDigestMismatch(t *testing.T) {
	fix := newEvalImporterFixture(t)
	index := fix.addEvidence(t, "evidence-1", []byte(`{"value":1}`))
	index.SHA256 = digestHex([]byte(`{"value":2}`))
	fix.writeJSONL(t, constants.EvalRunEvidenceIndexFilename, index)
	_, err := fix.importer().Import(context.Background())
	assert.ErrorIs(t, err, constants.ErrChecksumMismatch)
}

func TestEvalBundleImporter_Import_RejectsEvidenceByteLengthMismatch(t *testing.T) {
	fix := newEvalImporterFixture(t)
	index := fix.addEvidence(t, "evidence-1", []byte(`{"value":1}`))
	index.ByteLength++
	fix.writeJSONL(t, constants.EvalRunEvidenceIndexFilename, index)
	_, err := fix.importer().Import(context.Background())
	assert.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
}

func TestEvalBundleImporter_Import_RejectsEvidencePathTraversal(t *testing.T) {
	fix := newEvalImporterFixture(t)
	index := fix.addEvidence(t, "evidence-1", []byte(`{"value":1}`))
	index.StorageLocation = "../escape.json"
	fix.writeJSONL(t, constants.EvalRunEvidenceIndexFilename, index)
	_, err := fix.importer().Import(context.Background())
	assert.ErrorIs(t, err, constants.ErrPathValidation)
}

func TestEvalBundleImporter_Import_AuthenticatesEncryptedEvidenceMetadata(t *testing.T) {
	fix := newEvalImporterFixture(t)
	index := fix.addEncryptedEvidence(t, "evidence-1", []byte(`{"secret":"value"}`), []byte("authenticated-ciphertext"))
	fix.writeJSONL(t, constants.EvalRunEvidenceIndexFilename, index)
	nodes, err := fix.importer().Import(context.Background())
	require.NoError(t, err)
	observations := nodesByType(nodes, ArtifactTypeEvalObservation)
	require.Len(t, observations, 1)
	require.NotNil(t, observations[0].Encryption)
	assert.Equal(t, constants.EvalRestrictedEvidenceScope, observations[0].Encryption.AuthorizationScope)
	assert.Equal(t, index.SHA256, observations[0].Encryption.PlaintextSHA256)
	assert.Equal(t, index.Encryption.AADSHA256, observations[0].Encryption.AuthenticatedMetadataSHA256)
	assert.Empty(t, observations[0].CanonicalBytes)
}

func TestEvalBundleImporter_Import_RejectsEncryptedEvidenceCiphertextMismatch(t *testing.T) {
	fix := newEvalImporterFixture(t)
	index := fix.addEncryptedEvidence(t, "evidence-1", []byte(`{"secret":"value"}`), []byte("authenticated-ciphertext"))
	index.Encryption.CiphertextSHA256 = digestHex([]byte("other-ciphertext"))
	fix.writeJSONL(t, constants.EvalRunEvidenceIndexFilename, index)
	_, err := fix.importer().Import(context.Background())
	assert.ErrorIs(t, err, constants.ErrEvidenceEncryptionInvalid)
}

func TestEvalBundleImporter_Import_ResolvesGraphReferences(t *testing.T) {
	fix := newEvalImporterFixture(t)
	evidenceIndex := fix.addEvidence(t, "evidence-1", []byte(`{"value":1}`))
	fix.writeJSONL(t, constants.EvalRunEvidenceIndexFilename, evidenceIndex)
	metric := evalFixtureMetric{SchemaVersion: "1.40.0", MetricID: "metric-pass", MetricVersion: "1.0.0", AttemptID: fix.attemptID, RunID: fix.runID, ArmID: "direct", TaskID: fix.taskID, Value: 1, Unit: "boolean", Eligible: true, VerificationStatus: "verified", GraderClass: "deterministic", EvidenceRefs: []string{evidenceIndex.ArtifactID}}
	fix.writeJSONL(t, constants.EvalRunMetricsFilename, metric)
	nodes, err := fix.importer().Import(context.Background())
	require.NoError(t, err)
	graph := NewEvidenceGraph(constants.DemoRunMaxArtifactBytes, []string{constants.MediaTypeJSON})
	for _, node := range nodes {
		require.NoError(t, graph.AddNode(node))
	}
	graph.ValidateAll(time.Unix(1_699_999_999, 0).UTC(), time.Unix(1_700_000_100, 0).UTC())
	assert.True(t, graph.Valid(), graph.Failures())
}

func TestEvalBundleImporter_Import_SkipsBlankJSONLLines(t *testing.T) {
	fix := newEvalImporterFixture(t)
	path := fix.path(constants.EvalRunStagesFilename)
	fix.reader.files[path] = []byte("\n\n")
	_, err := fix.importer().Import(context.Background())
	assert.NoError(t, err)
}

func TestEvalBundleImporter_Import_PreservesUnderlyingReadError(t *testing.T) {
	fix := newEvalImporterFixture(t)
	delete(fix.reader.files, fix.path(constants.EvalRunManifestFilename))
	_, err := fix.importer().Import(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrEvidenceImporterFailed))
}

func nodesByType(nodes []EvidenceNode, artifactType ArtifactType) []EvidenceNode {
	matched := make([]EvidenceNode, 0)
	for _, node := range nodes {
		if node.ArtifactType == artifactType {
			matched = append(matched, node)
		}
	}
	return matched
}
