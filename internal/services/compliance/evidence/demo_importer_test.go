// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version.0.

package evidence

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// importerFixture holds the in-memory reader, provenance source, and run ID
// for a DemoRunImporter test, along with helper data for assertions.
type importerFixture struct {
	reader  *memoryArtifactReader
	source  memoryProvenanceSource
	runID   string
	scopeID string
	demoID  string
}

// newImporterFixture builds a minimal demo run with a manifest and one
// scenario result. The result has no receipt, persistence, state observation,
// or metric references; callers add those as needed via the helper methods.
func newImporterFixture(t *testing.T) *importerFixture {
	t.Helper()
	demoID := constants.DemosOrgFedRAMP
	scopeID := constants.DemoScopeFedRAMP
	runID := demoID + "-run-importer-test"
	generatedAt := time.Unix(1_700_000_000, 0).UTC()

	manifest := &compliancev1.DemoManifest{
		DemoId: demoID, DemoVersion: constants.DemoVersion, RunId: runID, ScopeId: scopeID,
		GeneratedAt:    timestamppb.New(generatedAt),
		SupportedLanes: []string{"automated"},
	}
	manifestBody, err := compliancev1.MarshalCanonical(manifest)
	require.NoError(t, err)

	result := &compliancev1.DemoScenarioResult{
		ResultId:    runID + ":scenario-1",
		ScenarioRef: &compliancev1.VersionedReference{Id: "scenario-1", Version: "1.0.0"},
		DemoId:      demoID, ScopeId: scopeID, RunId: runID,
		StartedAt:          timestamppb.New(generatedAt.Add(time.Second)),
		CompletedAt:        timestamppb.New(generatedAt.Add(2 * time.Second)),
		Status:             "passed",
		VerificationStatus: "verified",
	}
	resultBody, err := compliancev1.MarshalCanonical(result)
	require.NoError(t, err)

	reader := &memoryArtifactReader{files: map[string][]byte{
		runArtifactPath(runID, constants.DemoRunManifestFilename): manifestBody,
		runArtifactPath(runID, constants.DemoRunResultsFilename):  resultBody,
	}}
	provenance := []ProvenanceArtifact{{Name: constants.DemosComposeFile, Body: []byte("services: {}")}}
	return &importerFixture{
		reader:  reader,
		source:  memoryProvenanceSource{artifacts: provenance},
		runID:   runID,
		scopeID: scopeID,
		demoID:  demoID,
	}
}

// addSignedReceipt creates a signed ActionReceipt with a valid persistence
// attestation, writes both the receipt and persistence artifacts to the
// reader, links the result to both, and returns the receipt reference and
// the signed receipt.
func (f *importerFixture) addSignedReceipt(t *testing.T) (string, *operatorv1.ActionReceipt) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signerKeyID := hex.EncodeToString(pub)
	txID := "tx-receipt-1"
	receipt := &operatorv1.ActionReceipt{
		TransactionId:    txID,
		TransactionHash:  "hash-" + txID,
		SignerKeyId:      signerKeyID,
		ExecutedAtUnixMs: 1_700_000_001_000,
	}
	// Sign the receipt using the governance canonicalization.
	payload, err := governance.CanonicalizeActionReceipt(receipt)
	require.NoError(t, err)
	receipt.Signature = hex.EncodeToString(ed25519.Sign(priv, payload))

	// Build and sign the persistence attestation.
	attestation := &operatorv1.ReceiptPersistenceAttestation{
		TransactionId:          receipt.TransactionId,
		ReceiptSignatureDigest: governance.SignatureDigest([]string{receipt.Signature}),
		PersistedAtUnixMs:      1_700_000_002_000,
		AuditRecordId:          receipt.TransactionId,
		SignerKeyId:            signerKeyID,
	}
	attPayload, err := governance.CanonicalizeReceiptPersistenceAttestation(attestation)
	require.NoError(t, err)
	attestation.Signature = hex.EncodeToString(ed25519.Sign(priv, attPayload))
	receipt.FinalPersistenceAttestation = attestation

	receiptBody, err := compliancev1.MarshalCanonical(receipt)
	require.NoError(t, err)
	digest := sha256.Sum256(receiptBody)
	digestHex := hex.EncodeToString(digest[:])
	ref := "action-receipt:sha256:" + digestHex
	f.reader.files[runArtifactPath(f.runID, constants.DemoRunReceiptsDirname, digestHex+constants.FileExtJSON)] = receiptBody

	// Write the persistence attestation as a separate artifact.
	persistenceBody, err := compliancev1.MarshalCanonical(attestation)
	require.NoError(t, err)
	persistenceDigest := sha256.Sum256(persistenceBody)
	persistenceDigestHex := hex.EncodeToString(persistenceDigest[:])
	persistenceRef := "receipt-persistence:sha256:" + persistenceDigestHex
	f.reader.files[runArtifactPath(f.runID, constants.DemoRunPersistenceDirname, persistenceDigestHex+constants.FileExtJSON)] = persistenceBody

	// Link the result to both the receipt and persistence artifacts.
	result := f.decodeResult(t)
	result.ReceiptRefs = append(result.ReceiptRefs, ref, persistenceRef)
	f.encodeResult(t, result)

	return ref, receipt
}

// addPersistenceArtifact writes a persistence attestation artifact and returns
// its reference. The attestation is derived from the given signed receipt.
func (f *importerFixture) addPersistenceArtifact(t *testing.T, receipt *operatorv1.ActionReceipt) string {
	t.Helper()
	attestation := receipt.GetFinalPersistenceAttestation()
	require.NotNil(t, attestation)
	body, err := compliancev1.MarshalCanonical(attestation)
	require.NoError(t, err)
	digest := sha256.Sum256(body)
	digestHex := hex.EncodeToString(digest[:])
	ref := "receipt-persistence:sha256:" + digestHex
	f.reader.files[runArtifactPath(f.runID, constants.DemoRunPersistenceDirname, digestHex+constants.FileExtJSON)] = body

	// Link the result to the persistence artifact.
	result := f.decodeResult(t)
	result.ReceiptRefs = append(result.ReceiptRefs, ref)
	f.encodeResult(t, result)

	return ref
}

// addStateObservation writes a state observation artifact and returns its
// reference. The body is canonical JSON.
func (f *importerFixture) addStateObservation(t *testing.T) string {
	t.Helper()
	body := []byte(`{"collector_id":"test","collected_at":"2026-09-02T16:30:01Z","observation":{"value":42}}`)
	digest := sha256.Sum256(body)
	digestHex := hex.EncodeToString(digest[:])
	ref := "state-observation:sha256:" + digestHex
	f.reader.files[runArtifactPath(f.runID, constants.DemoRunStateObservationsDirname, digestHex+constants.FileExtJSON)] = body

	result := f.decodeResult(t)
	result.StateObservationRefs = append(result.StateObservationRefs, ref)
	f.encodeResult(t, result)

	return ref
}

// addMetric writes a DemoMetricEvidence artifact and returns its reference.
func (f *importerFixture) addMetric(t *testing.T, sourceEvidenceRef string) string {
	t.Helper()
	metric := &compliancev1.DemoMetricEvidence{
		MetricId: "test-metric", MetricVersion: constants.DemoMetricEvidenceVersion,
		RunId: f.runID, ScopeId: f.scopeID,
		ScenarioRef:       &compliancev1.VersionedReference{Id: "scenario-1", Version: "1.0.0"},
		SourceEvidenceRef: sourceEvidenceRef,
		Unit:              "count", Comparison: constants.DemoMetricComparisonGreaterThanOrEqual,
		Passed: true, MeasuredValue: 42, ThresholdValue: 10,
		EvaluatedAt: timestamppb.New(time.Unix(1_700_000_002, 0).UTC()),
		GraderRef:   &compliancev1.VersionedReference{Id: constants.DemoMetricGraderID, Version: constants.DemoMetricGraderVersion},
	}
	body, err := compliancev1.MarshalCanonical(metric)
	require.NoError(t, err)
	digest := sha256.Sum256(body)
	digestHex := hex.EncodeToString(digest[:])
	ref := "metric:sha256:" + digestHex
	f.reader.files[runArtifactPath(f.runID, constants.DemoRunMetricsDirname, digestHex+constants.FileExtJSON)] = body

	result := f.decodeResult(t)
	result.MetricRefs = append(result.MetricRefs, ref)
	f.encodeResult(t, result)

	return ref
}

func (f *importerFixture) decodeResult(t *testing.T) *compliancev1.DemoScenarioResult {
	t.Helper()
	result := &compliancev1.DemoScenarioResult{}
	require.NoError(t, compliancev1.UnmarshalCanonical(
		f.reader.files[runArtifactPath(f.runID, constants.DemoRunResultsFilename)], result))
	return result
}

func (f *importerFixture) encodeResult(t *testing.T, result *compliancev1.DemoScenarioResult) {
	t.Helper()
	body, err := compliancev1.MarshalCanonical(result)
	require.NoError(t, err)
	f.reader.files[runArtifactPath(f.runID, constants.DemoRunResultsFilename)] = body
}

func TestDemoRunImporter_SourceID(t *testing.T) {
	importer := NewDemoRunImporter(&memoryArtifactReader{files: map[string][]byte{}}, "run-1", memoryProvenanceSource{})
	assert.Equal(t, "demo-run", importer.SourceID())
}

func TestDemoRunImporter_Import_AcceptsMinimalRun(t *testing.T) {
	fix := newImporterFixture(t)
	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)

	nodes, err := importer.Import(context.Background())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(nodes), 2)

	// The manifest node must be present and verified.
	var manifestNode *EvidenceNode
	for i := range nodes {
		if nodes[i].ArtifactType == ArtifactTypeDemoManifest {
			manifestNode = &nodes[i]
		}
	}
	require.NotNil(t, manifestNode)
	assert.Equal(t, VerificationStatusVerified, manifestNode.VerificationStatus)
	assert.Equal(t, constants.DemoRunVerifierID, manifestNode.VerifierID)
	assert.Equal(t, fix.scopeID, manifestNode.ScopeID)
	assert.Equal(t, fix.runID, manifestNode.RunID)

	// The result node must be present.
	var resultNode *EvidenceNode
	for i := range nodes {
		if nodes[i].ArtifactType == ArtifactTypeDemoResult {
			resultNode = &nodes[i]
		}
	}
	require.NotNil(t, resultNode)
	assert.Equal(t, "scenario-1", resultNode.ScenarioID)
	assert.Equal(t, VerificationStatusUnverified, resultNode.VerificationStatus)
}

func TestDemoRunImporter_Import_RejectsNilReader(t *testing.T) {
	importer := NewDemoRunImporter(nil, "run-1", memoryProvenanceSource{})
	_, err := importer.Import(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrInvalidEvidenceGraph))
}

func TestDemoRunImporter_Import_RejectsNilSource(t *testing.T) {
	reader := &memoryArtifactReader{files: map[string][]byte{}}
	importer := NewDemoRunImporter(reader, "run-1", nil)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
}

func TestDemoRunImporter_Import_RejectsInvalidRunID(t *testing.T) {
	fix := newImporterFixture(t)
	importer := NewDemoRunImporter(fix.reader, "../escape", fix.source)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
}

func TestDemoRunImporter_Import_RejectsMissingManifest(t *testing.T) {
	fix := newImporterFixture(t)
	delete(fix.reader.files, runArtifactPath(fix.runID, constants.DemoRunManifestFilename))
	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
}

func TestDemoRunImporter_Import_RejectsMalformedManifest(t *testing.T) {
	fix := newImporterFixture(t)
	fix.reader.files[runArtifactPath(fix.runID, constants.DemoRunManifestFilename)] = []byte(`{invalid}`)
	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
}

func TestDemoRunImporter_Import_RejectsManifestRunIDMismatch(t *testing.T) {
	fix := newImporterFixture(t)
	importer := NewDemoRunImporter(fix.reader, "different-run-id", fix.source)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
}

func TestDemoRunImporter_Import_RejectsMissingResults(t *testing.T) {
	fix := newImporterFixture(t)
	delete(fix.reader.files, runArtifactPath(fix.runID, constants.DemoRunResultsFilename))
	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
}

func TestDemoRunImporter_Import_RejectsResultScopeMismatch(t *testing.T) {
	fix := newImporterFixture(t)
	result := fix.decodeResult(t)
	result.ScopeId = "wrong-scope"
	fix.encodeResult(t, result)
	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
}

func TestDemoRunImporter_Import_RejectsResultRunMismatch(t *testing.T) {
	fix := newImporterFixture(t)
	result := fix.decodeResult(t)
	result.RunId = "wrong-run"
	fix.encodeResult(t, result)
	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
}

func TestDemoRunImporter_Import_LoadsSignedReceipt(t *testing.T) {
	fix := newImporterFixture(t)
	receiptRef, _ := fix.addSignedReceipt(t)

	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	nodes, err := importer.Import(context.Background())
	require.NoError(t, err)

	var receiptNode *EvidenceNode
	for i := range nodes {
		if nodes[i].ArtifactID == receiptRef {
			receiptNode = &nodes[i]
		}
	}
	require.NotNil(t, receiptNode)
	assert.Equal(t, ArtifactTypeActionReceipt, receiptNode.ArtifactType)
	assert.Equal(t, VerificationStatusVerified, receiptNode.VerificationStatus)
	assert.Equal(t, constants.DemoRunVerifierID, receiptNode.VerifierID)
}

func TestDemoRunImporter_Import_ReceiptReferencesPersistence(t *testing.T) {
	fix := newImporterFixture(t)
	receiptRef, receipt := fix.addSignedReceipt(t)
	persistenceRef := fix.addPersistenceArtifact(t, receipt)

	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	nodes, err := importer.Import(context.Background())
	require.NoError(t, err)

	var receiptNode *EvidenceNode
	for i := range nodes {
		if nodes[i].ArtifactID == receiptRef {
			receiptNode = &nodes[i]
		}
	}
	require.NotNil(t, receiptNode)
	assert.Contains(t, receiptNode.References, persistenceRef)

	var persistenceNode *EvidenceNode
	for i := range nodes {
		if nodes[i].ArtifactID == persistenceRef {
			persistenceNode = &nodes[i]
		}
	}
	require.NotNil(t, persistenceNode)
	assert.Equal(t, ArtifactTypeReceiptPersistence, persistenceNode.ArtifactType)
	assert.Equal(t, VerificationStatusVerified, persistenceNode.VerificationStatus)
}

func TestDemoRunImporter_Import_RejectsReceiptDigestMismatch(t *testing.T) {
	fix := newImporterFixture(t)
	receiptRef, _ := fix.addSignedReceipt(t)
	// Corrupt the receipt body.
	digest := digestFromReference(receiptRef)
	path := runArtifactPath(fix.runID, constants.DemoRunReceiptsDirname, digest+constants.FileExtJSON)
	fix.reader.files[path] = []byte(`{"tampered":true}`)

	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
}

func TestDemoRunImporter_Import_LoadsStateObservation(t *testing.T) {
	fix := newImporterFixture(t)
	obsRef := fix.addStateObservation(t)

	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	nodes, err := importer.Import(context.Background())
	require.NoError(t, err)

	var obsNode *EvidenceNode
	for i := range nodes {
		if nodes[i].ArtifactID == obsRef {
			obsNode = &nodes[i]
		}
	}
	require.NotNil(t, obsNode)
	assert.Equal(t, ArtifactTypeStateObservation, obsNode.ArtifactType)
	assert.Equal(t, VerificationStatusUnverified, obsNode.VerificationStatus)
	assert.Equal(t, "scenario-1", obsNode.ScenarioID)
}

func TestDemoRunImporter_Import_LoadsMetricWithSourceEvidenceRef(t *testing.T) {
	fix := newImporterFixture(t)
	obsRef := fix.addStateObservation(t)
	metricRef := fix.addMetric(t, obsRef)

	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	nodes, err := importer.Import(context.Background())
	require.NoError(t, err)

	var metricNode *EvidenceNode
	for i := range nodes {
		if nodes[i].ArtifactID == metricRef {
			metricNode = &nodes[i]
		}
	}
	require.NotNil(t, metricNode)
	assert.Equal(t, ArtifactTypeDemoMetric, metricNode.ArtifactType)
	assert.Contains(t, metricNode.References, obsRef)
}

func TestDemoRunImporter_Import_RejectsStateObservationDigestMismatch(t *testing.T) {
	fix := newImporterFixture(t)
	obsRef := fix.addStateObservation(t)
	digest := digestFromReference(obsRef)
	path := runArtifactPath(fix.runID, constants.DemoRunStateObservationsDirname, digest+constants.FileExtJSON)
	fix.reader.files[path] = []byte(`{"tampered":true}`)

	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
}

func TestDemoRunImporter_Import_RejectsMetricDigestMismatch(t *testing.T) {
	fix := newImporterFixture(t)
	metricRef := fix.addMetric(t, "")
	digest := digestFromReference(metricRef)
	path := runArtifactPath(fix.runID, constants.DemoRunMetricsDirname, digest+constants.FileExtJSON)
	fix.reader.files[path] = []byte(`{"tampered":true}`)

	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
}

func TestDemoRunImporter_Import_RejectsMalformedResult(t *testing.T) {
	fix := newImporterFixture(t)
	fix.reader.files[runArtifactPath(fix.runID, constants.DemoRunResultsFilename)] = []byte(`{invalid}`)
	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
}

func TestDemoRunImporter_Import_RejectsEmptyResults(t *testing.T) {
	fix := newImporterFixture(t)
	fix.reader.files[runArtifactPath(fix.runID, constants.DemoRunResultsFilename)] = []byte(``)
	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
}

func TestDemoRunImporter_Import_GraphIntegrationValidates(t *testing.T) {
	fix := newImporterFixture(t)
	fix.addSignedReceipt(t)

	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	nodes, err := importer.Import(context.Background())
	require.NoError(t, err)

	g := NewEvidenceGraph(0, nil)
	for _, node := range nodes {
		require.NoError(t, g.AddNode(node))
	}
	g.ValidateAll(time.Unix(1_000_000_000, 0).UTC(), time.Unix(2_000_000_000, 0).UTC())
	assert.True(t, g.Valid(), "graph should be valid: %v", g.Failures())
}

func TestDemoRunImporter_Import_ProducesContentAddressedManifestNode(t *testing.T) {
	fix := newImporterFixture(t)
	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	nodes, err := importer.Import(context.Background())
	require.NoError(t, err)

	manifestBody := fix.reader.files[runArtifactPath(fix.runID, constants.DemoRunManifestFilename)]
	expectedAddr := ContentAddress(ArtifactTypeDemoManifest, manifestBody)
	found := false
	for _, node := range nodes {
		if node.ArtifactID == expectedAddr {
			found = true
			assert.Equal(t, ArtifactTypeDemoManifest, node.ArtifactType)
		}
	}
	assert.True(t, found, "manifest node with expected content address not found")
}

func TestDemoRunImporter_Import_AllNodesHaveNonNilReferences(t *testing.T) {
	fix := newImporterFixture(t)
	fix.addSignedReceipt(t)
	fix.addStateObservation(t)
	fix.addMetric(t, "")

	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	nodes, err := importer.Import(context.Background())
	require.NoError(t, err)
	for i, node := range nodes {
		assert.NotNilf(t, node.References, "node %d (%s) has nil references", i, node.ArtifactID)
	}
}

func TestDemoRunImporter_Import_CancelledContext(t *testing.T) {
	fix := newImporterFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	_, err := importer.Import(ctx)
	require.Error(t, err)
}

func TestDemoRunImporter_Import_MultipleResults(t *testing.T) {
	fix := newImporterFixture(t)
	generatedAt := time.Unix(1_700_000_000, 0).UTC()

	// Add a second result.
	result2 := &compliancev1.DemoScenarioResult{
		ResultId:    fix.runID + ":scenario-2",
		ScenarioRef: &compliancev1.VersionedReference{Id: "scenario-2", Version: "1.0.0"},
		DemoId:      fix.demoID, ScopeId: fix.scopeID, RunId: fix.runID,
		StartedAt:          timestamppb.New(generatedAt.Add(3 * time.Second)),
		CompletedAt:        timestamppb.New(generatedAt.Add(4 * time.Second)),
		Status:             "passed",
		VerificationStatus: "verified",
	}
	result2Body, err := compliancev1.MarshalCanonical(result2)
	require.NoError(t, err)

	// Rewrite results file with both results.
	result1 := fix.decodeResult(t)
	result1Body, err := compliancev1.MarshalCanonical(result1)
	require.NoError(t, err)
	fix.reader.files[runArtifactPath(fix.runID, constants.DemoRunResultsFilename)] = append(append(result1Body, '\n'), result2Body...)

	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	nodes, err := importer.Import(context.Background())
	require.NoError(t, err)

	resultCount := 0
	for _, node := range nodes {
		if node.ArtifactType == ArtifactTypeDemoResult {
			resultCount++
		}
	}
	assert.Equal(t, 2, resultCount)
}

func TestDemoRunImporter_Import_RejectsTooManyResults(t *testing.T) {
	fix := newImporterFixture(t)
	// Build a results file with more than DemoRunMaxResults lines.
	var lines []byte
	result := fix.decodeResult(t)
	for i := 0; i <= constants.DemoRunMaxResults; i++ {
		body, err := compliancev1.MarshalCanonical(result)
		require.NoError(t, err)
		lines = append(lines, body...)
		lines = append(lines, '\n')
	}
	fix.reader.files[runArtifactPath(fix.runID, constants.DemoRunResultsFilename)] = lines

	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
}

func TestDemoRunImporter_Import_RejectsMissingReceiptFile(t *testing.T) {
	fix := newImporterFixture(t)
	// Add a receipt reference to the result but don't write the receipt file.
	missingDigest := sha256.Sum256([]byte("missing"))
	result := fix.decodeResult(t)
	result.ReceiptRefs = []string{"action-receipt:sha256:" + hex.EncodeToString(missingDigest[:])}
	fix.encodeResult(t, result)

	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
}

func TestDemoRunImporter_Import_RejectsMissingStateObservationFile(t *testing.T) {
	fix := newImporterFixture(t)
	// Add a state observation reference but don't write the file.
	missingDigest := sha256.Sum256([]byte("missing"))
	result := fix.decodeResult(t)
	result.StateObservationRefs = []string{"state-observation:sha256:" + hex.EncodeToString(missingDigest[:])}
	fix.encodeResult(t, result)

	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
}

func TestDemoRunImporter_Import_RejectsMissingMetricFile(t *testing.T) {
	fix := newImporterFixture(t)
	// Add a metric reference but don't write the file.
	missingDigest := sha256.Sum256([]byte("missing"))
	result := fix.decodeResult(t)
	result.MetricRefs = []string{"metric:sha256:" + hex.EncodeToString(missingDigest[:])}
	fix.encodeResult(t, result)

	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
}

func TestDemoRunImporter_Import_RejectsNonCanonicalStateObservation(t *testing.T) {
	fix := newImporterFixture(t)
	// Write non-canonical JSON (with whitespace) and compute its digest so the
	// digest check passes but ValidateCanonicalJSON rejects the body.
	nonCanonical := []byte(`{ "collector_id" : "test" }`)
	digest := sha256.Sum256(nonCanonical)
	digestHex := hex.EncodeToString(digest[:])
	ref := "state-observation:sha256:" + digestHex
	fix.reader.files[runArtifactPath(fix.runID, constants.DemoRunStateObservationsDirname, digestHex+constants.FileExtJSON)] = nonCanonical

	result := fix.decodeResult(t)
	result.StateObservationRefs = []string{ref}
	fix.encodeResult(t, result)

	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
}

func TestDemoRunImporter_Import_RejectsMalformedMetric(t *testing.T) {
	fix := newImporterFixture(t)
	// Write a malformed metric body with a matching digest.
	malformed := []byte(`{invalid}`)
	digest := sha256.Sum256(malformed)
	digestHex := hex.EncodeToString(digest[:])
	ref := "metric:sha256:" + digestHex
	fix.reader.files[runArtifactPath(fix.runID, constants.DemoRunMetricsDirname, digestHex+constants.FileExtJSON)] = malformed

	result := fix.decodeResult(t)
	result.MetricRefs = []string{ref}
	fix.encodeResult(t, result)

	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
}

func TestDemoRunImporter_Import_RejectsMalformedPersistence(t *testing.T) {
	fix := newImporterFixture(t)
	malformed := []byte(`{invalid}`)
	digest := sha256.Sum256(malformed)
	digestHex := hex.EncodeToString(digest[:])
	ref := "receipt-persistence:sha256:" + digestHex
	fix.reader.files[runArtifactPath(fix.runID, constants.DemoRunPersistenceDirname, digestHex+constants.FileExtJSON)] = malformed

	result := fix.decodeResult(t)
	result.ReceiptRefs = []string{ref}
	fix.encodeResult(t, result)

	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
}

func TestDemoRunImporter_Import_RejectsPersistenceDigestMismatch(t *testing.T) {
	fix := newImporterFixture(t)
	body := []byte(`{"valid":"json"}`)
	digest := sha256.Sum256(body)
	digestHex := hex.EncodeToString(digest[:])
	ref := "receipt-persistence:sha256:" + digestHex
	// Write a different body.
	fix.reader.files[runArtifactPath(fix.runID, constants.DemoRunPersistenceDirname, digestHex+constants.FileExtJSON)] = []byte(`{"different":"body"}`)

	result := fix.decodeResult(t)
	result.ReceiptRefs = []string{ref}
	fix.encodeResult(t, result)

	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
}

func TestDemoRunImporter_Import_RejectsMalformedReceipt(t *testing.T) {
	fix := newImporterFixture(t)
	malformed := []byte(`{invalid}`)
	digest := sha256.Sum256(malformed)
	digestHex := hex.EncodeToString(digest[:])
	ref := "action-receipt:sha256:" + digestHex
	fix.reader.files[runArtifactPath(fix.runID, constants.DemoRunReceiptsDirname, digestHex+constants.FileExtJSON)] = malformed

	result := fix.decodeResult(t)
	result.ReceiptRefs = []string{ref}
	fix.encodeResult(t, result)

	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
}

func TestDemoRunImporter_Import_UnsignedReceiptIsFailed(t *testing.T) {
	fix := newImporterFixture(t)
	// Create a receipt without a valid signature.
	receipt := &operatorv1.ActionReceipt{
		TransactionId:    "tx-unsigned",
		SignerKeyId:      "invalid-key",
		ExecutedAtUnixMs: 1_700_000_001_000,
	}
	body, err := compliancev1.MarshalCanonical(receipt)
	require.NoError(t, err)
	digest := sha256.Sum256(body)
	digestHex := hex.EncodeToString(digest[:])
	ref := "action-receipt:sha256:" + digestHex
	fix.reader.files[runArtifactPath(fix.runID, constants.DemoRunReceiptsDirname, digestHex+constants.FileExtJSON)] = body

	result := fix.decodeResult(t)
	result.ReceiptRefs = []string{ref}
	fix.encodeResult(t, result)

	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	nodes, err := importer.Import(context.Background())
	require.NoError(t, err)

	var receiptNode *EvidenceNode
	for i := range nodes {
		if nodes[i].ArtifactID == ref {
			receiptNode = &nodes[i]
		}
	}
	require.NotNil(t, receiptNode)
	assert.Equal(t, VerificationStatusFailed, receiptNode.VerificationStatus)
}

func TestDemoRunImporter_Import_BlankLinesInResultsAreSkipped(t *testing.T) {
	fix := newImporterFixture(t)
	result := fix.decodeResult(t)
	body, err := compliancev1.MarshalCanonical(result)
	require.NoError(t, err)
	// Insert blank lines between results.
	fix.reader.files[runArtifactPath(fix.runID, constants.DemoRunResultsFilename)] = append(append(append(body, '\n', '\n'), body...), '\n')

	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	nodes, err := importer.Import(context.Background())
	require.NoError(t, err)

	resultCount := 0
	for _, node := range nodes {
		if node.ArtifactType == ArtifactTypeDemoResult {
			resultCount++
		}
	}
	assert.Equal(t, 2, resultCount)
}

func TestDemoRunImporter_Import_ProducesNodesForGraphIntegration(t *testing.T) {
	fix := newImporterFixture(t)
	receiptRef, receipt := fix.addSignedReceipt(t)
	fix.addPersistenceArtifact(t, receipt)
	obsRef := fix.addStateObservation(t)
	fix.addMetric(t, obsRef)

	importer := NewDemoRunImporter(fix.reader, fix.runID, fix.source)
	nodes, err := importer.Import(context.Background())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(nodes), 5)

	// Add all nodes to a graph and validate.
	g := NewEvidenceGraph(0, nil)
	for _, node := range nodes {
		require.NoError(t, g.AddNode(node))
	}
	g.ResolveReferences()
	assert.True(t, g.Valid(), "unresolved references: %v", g.Failures())

	// Verify the receipt node references the persistence node.
	receiptNode := g.Node(receiptRef)
	require.NotNil(t, receiptNode)
	assert.NotEmpty(t, receiptNode.References)
}
