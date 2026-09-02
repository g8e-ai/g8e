// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type memoryArtifactReader struct {
	files map[string][]byte
}

func (r *memoryArtifactReader) ReadFile(_ context.Context, path string) ([]byte, error) {
	body, ok := r.files[path]
	if !ok {
		return nil, constants.ErrNotFound
	}
	return append([]byte(nil), body...), nil
}

func (r *memoryArtifactReader) ReadDir(_ context.Context, path string) ([]os.DirEntry, error) {
	prefix := path + string(os.PathSeparator)
	names := make(map[string]bool)
	for candidate := range r.files {
		if !strings.HasPrefix(candidate, prefix) {
			continue
		}
		remainder := strings.TrimPrefix(candidate, prefix)
		parts := strings.SplitN(remainder, string(os.PathSeparator), 2)
		names[parts[0]] = len(parts) == 2
	}
	if len(names) == 0 {
		return nil, constants.ErrNotFound
	}
	entries := make([]os.DirEntry, 0, len(names))
	for name, directory := range names {
		entries = append(entries, memoryDirEntry{name: name, directory: directory})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

type memoryDirEntry struct {
	name      string
	directory bool
}

func (e memoryDirEntry) Name() string               { return e.name }
func (e memoryDirEntry) IsDir() bool                { return e.directory }
func (e memoryDirEntry) Type() os.FileMode          { return 0 }
func (e memoryDirEntry) Info() (os.FileInfo, error) { return nil, nil }

type memoryProvenanceSource struct {
	artifacts []ProvenanceArtifact
}

func (s memoryProvenanceSource) Artifacts(_ context.Context, _ string) ([]ProvenanceArtifact, error) {
	return append([]ProvenanceArtifact(nil), s.artifacts...), nil
}

func TestVerifyDemoRun_AcceptsCanonicalCorrelatedRun(t *testing.T) {
	reader, source, runID := validDemoRunFixture(t)

	report, err := VerifyDemoRun(context.Background(), reader, runID, source, time.Unix(1_800_000_000, 0).UTC())

	require.NoError(t, err)
	assert.True(t, report.GetValid(), report.GetFailures())
	assert.Empty(t, report.GetFailures())
	assert.Equal(t, runID, report.GetReportId())
}

func TestVerifyDemoRun_ReportsIntegrityFailuresFailClosed(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(t *testing.T, reader *memoryArtifactReader, runID string)
		failureErr error
	}{
		{
			name: "non-canonical manifest",
			mutate: func(t *testing.T, reader *memoryArtifactReader, runID string) {
				path := runArtifactPath(runID, constants.DemoRunManifestFilename)
				reader.files[path] = append(reader.files[path], '\n')
			},
			failureErr: constants.ErrEvidenceArtifactMalformed,
		},
		{
			name: "cross-run scenario result",
			mutate: func(t *testing.T, reader *memoryArtifactReader, runID string) {
				result := decodeOnlyResult(t, reader, runID)
				result.RunId = "different-run"
				writeOnlyResult(t, reader, runID, result)
			},
			failureErr: constants.ErrEvidenceScopeMismatch,
		},
		{
			name: "dangling state observation",
			mutate: func(t *testing.T, reader *memoryArtifactReader, runID string) {
				result := decodeOnlyResult(t, reader, runID)
				body := []byte(`{"collector_id":"missing"}`)
				digest := sha256.Sum256(body)
				ref := "state-observation:sha256:" + hex.EncodeToString(digest[:])
				result.StateObservationRefs = []string{ref}
				result.StepResults[0].EvidenceRefs = []string{ref}
				result.StepResults[0].ProtocolResult = string(body)
				writeOnlyResult(t, reader, runID, result)
			},
			failureErr: constants.ErrUnresolvedReference,
		},
		{
			name: "unexpected root artifact",
			mutate: func(_ *testing.T, reader *memoryArtifactReader, runID string) {
				reader.files[runArtifactPath(runID, "unexpected.json")] = []byte("{}")
			},
			failureErr: constants.ErrUnexpectedEvidenceArtifact,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, source, runID := validDemoRunFixture(t)
			tt.mutate(t, reader, runID)

			report, err := VerifyDemoRun(context.Background(), reader, runID, source, time.Unix(1_800_000_000, 0).UTC())

			require.NoError(t, err)
			assert.False(t, report.GetValid())
			assertReportContainsFailure(t, report, tt.failureErr)
		})
	}
}

func TestVerifyDemoRun_RejectsInvalidReceiptAndPersistenceSignatures(t *testing.T) {
	reader, source, runID := validDemoRunFixture(t)
	result := decodeOnlyResult(t, reader, runID)
	receipt := &operatorv1.ActionReceipt{
		TransactionId: "transaction-1", TransactionHash: "hash-1", SignerKeyId: strings.Repeat("0", 64), Signature: "00",
		FinalPersistenceAttestation: &operatorv1.ReceiptPersistenceAttestation{
			TransactionId: "transaction-1", ReceiptSignatureDigest: "digest", PersistedAtUnixMs: 1,
			AuditRecordId: "transaction-1", SignerKeyId: strings.Repeat("0", 64), Signature: "00",
		},
	}
	receiptBody, err := compliancev1.MarshalCanonical(receipt)
	require.NoError(t, err)
	persistenceBody, err := compliancev1.MarshalCanonical(receipt.GetFinalPersistenceAttestation())
	require.NoError(t, err)
	receiptRef := contentReference("action-receipt", receiptBody)
	persistenceRef := contentReference("receipt-persistence", persistenceBody)
	result.TransactionIds = []string{receipt.GetTransactionId()}
	result.ReceiptRefs = []string{receiptRef, persistenceRef}
	writeOnlyResult(t, reader, runID, result)
	reader.files[runArtifactPath(runID, constants.DemoRunReceiptsDirname, digestFromReference(receiptRef)+constants.FileExtJSON)] = receiptBody
	reader.files[runArtifactPath(runID, constants.DemoRunPersistenceDirname, digestFromReference(persistenceRef)+constants.FileExtJSON)] = persistenceBody

	report, verifyErr := VerifyDemoRun(context.Background(), reader, runID, source, time.Unix(1_800_000_000, 0).UTC())

	require.NoError(t, verifyErr)
	assert.False(t, report.GetValid())
	assertReportContainsFailure(t, report, constants.ErrActionReceiptSignatureInvalid)
	assertReportContainsFailure(t, report, constants.ErrReceiptPersistenceSignatureMismatch)
}

func validDemoRunFixture(t *testing.T) (*memoryArtifactReader, memoryProvenanceSource, string) {
	t.Helper()
	assertions, frameworks, _, err := catalog.LoadCanonicalCatalogs()
	require.NoError(t, err)
	scenarios, err := catalog.LoadDemoScenarioCatalog(assertions, frameworks)
	require.NoError(t, err)
	definitions := make([]*compliancev1.DemoScenarioDefinition, 0)
	refs := make([]*compliancev1.VersionedReference, 0)
	frameworkRefs := make(map[string]*compliancev1.FrameworkControlReference)
	for _, definition := range scenarios.GetDefinitions() {
		if !strings.HasPrefix(definition.GetScenarioId(), constants.DemosOrgFedRAMP+"-") {
			continue
		}
		definitions = append(definitions, definition)
		refs = append(refs, &compliancev1.VersionedReference{Id: definition.GetScenarioId(), Version: definition.GetScenarioVersion()})
		for _, reference := range definition.GetFrameworkControlRefs() {
			key := reference.GetFrameworkRef().GetId() + ":" + reference.GetFrameworkRef().GetVersion() + ":" + reference.GetControlId()
			frameworkRefs[key] = reference
		}
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].GetId() < refs[j].GetId() })
	frameworkKeys := make([]string, 0, len(frameworkRefs))
	for key := range frameworkRefs {
		frameworkKeys = append(frameworkKeys, key)
	}
	sort.Strings(frameworkKeys)
	manifestFrameworkRefs := make([]*compliancev1.FrameworkControlReference, 0, len(frameworkKeys))
	for _, key := range frameworkKeys {
		manifestFrameworkRefs = append(manifestFrameworkRefs, frameworkRefs[key])
	}
	provenance := []ProvenanceArtifact{{Name: constants.DemosComposeFile, Body: []byte("services: {}")}}
	provenanceDigest := sha256.Sum256(provenance[0].Body)
	runID := "fedramp-run-test"
	generatedAt := time.Unix(1_700_000_000, 0).UTC()
	manifest := &compliancev1.DemoManifest{
		DemoId: constants.DemosOrgFedRAMP, DemoVersion: constants.DemoVersion, RunId: runID, ScopeId: constants.DemoScopeFedRAMP,
		GeneratedAt: timestamppb.New(generatedAt), ScenarioDefinitionRefs: refs,
		ProvenanceHashes:    []*compliancev1.NamedDigest{{Name: provenance[0].Name, Sha256: hex.EncodeToString(provenanceDigest[:])}},
		RequiredEnvironment: []string{"docker", "g8e-binary"}, FrameworkControlRefs: manifestFrameworkRefs,
		SupportedLanes: []string{"automated", "manual-notary"},
	}
	definition := definitions[0]
	result := &compliancev1.DemoScenarioResult{
		ResultId:    runID + ":" + definition.GetScenarioId(),
		ScenarioRef: &compliancev1.VersionedReference{Id: definition.GetScenarioId(), Version: definition.GetScenarioVersion()},
		DemoId:      constants.DemosOrgFedRAMP, ScopeId: constants.DemoScopeFedRAMP, RunId: runID,
		StartedAt: timestamppb.New(generatedAt.Add(time.Second)), CompletedAt: timestamppb.New(generatedAt.Add(2 * time.Second)),
		Status: "failed", Failure: "expected fixture failure", VerificationStatus: "unverifiable",
		DisplayNumber: definition.GetDisplayNumber(), Title: definition.GetTitle(), AssertionRefs: definition.GetAssertionRefs(),
		FrameworkControlRefs: definition.GetFrameworkControlRefs(),
		StepResults: []*compliancev1.DemoStepResult{{
			StepId: "step-1", Operation: "fixture", StartedAt: timestamppb.New(generatedAt.Add(time.Second)),
			CompletedAt: timestamppb.New(generatedAt.Add(2 * time.Second)), Status: "failed", Failure: "expected fixture failure", Required: true,
		}},
	}
	manifestBody, err := compliancev1.MarshalCanonical(manifest)
	require.NoError(t, err)
	resultBody, err := compliancev1.MarshalCanonical(result)
	require.NoError(t, err)
	reader := &memoryArtifactReader{files: map[string][]byte{
		runArtifactPath(runID, constants.DemoRunManifestFilename): manifestBody,
		runArtifactPath(runID, constants.DemoRunResultsFilename):  resultBody,
	}}
	return reader, memoryProvenanceSource{artifacts: provenance}, runID
}

func decodeOnlyResult(t *testing.T, reader *memoryArtifactReader, runID string) *compliancev1.DemoScenarioResult {
	t.Helper()
	result := &compliancev1.DemoScenarioResult{}
	require.NoError(t, compliancev1.UnmarshalCanonical(reader.files[runArtifactPath(runID, constants.DemoRunResultsFilename)], result))
	return result
}

func writeOnlyResult(t *testing.T, reader *memoryArtifactReader, runID string, result *compliancev1.DemoScenarioResult) {
	t.Helper()
	body, err := compliancev1.MarshalCanonical(result)
	require.NoError(t, err)
	reader.files[runArtifactPath(runID, constants.DemoRunResultsFilename)] = body
}

func assertReportContainsFailure(t *testing.T, report *compliancev1.ComplianceVerificationReport, target error) {
	t.Helper()
	for _, failure := range report.GetFailures() {
		if failure.GetCode() == target.Error() {
			return
		}
	}
	assert.Fail(t, "verification report does not contain failure", "target=%s failures=%v", target, report.GetFailures())
}

func runArtifactPath(runID string, parts ...string) string {
	base := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.DemoEvidenceDirname, runID)
	return filepath.Join(append([]string{base}, parts...)...)
}

func contentReference(prefix string, body []byte) string {
	digest := sha256.Sum256(body)
	return prefix + ":sha256:" + hex.EncodeToString(digest[:])
}

func digestFromReference(reference string) string {
	parts := strings.SplitN(reference, ":", 3)
	if len(parts) != 3 {
		return ""
	}
	return parts[2]
}
