// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evidence

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/scenarios"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// ArtifactReader is the read-only runtime-file capability used by the verifier.
type ArtifactReader interface {
	ReadFile(context.Context, string) ([]byte, error)
	ReadDir(context.Context, string) ([]os.DirEntry, error)
}

// ProvenanceArtifact is one source artifact covered by a DemoManifest digest.
type ProvenanceArtifact struct {
	Name string
	Body []byte
}

// ProvenanceSource enumerates the complete source provenance set for a demo.
type ProvenanceSource interface {
	Artifacts(context.Context, string) ([]ProvenanceArtifact, error)
}

type demoRunVerifier struct {
	ctx         context.Context
	reader      ArtifactReader
	runID       string
	source      ProvenanceSource
	report      *compliancev1.ComplianceVerificationReport
	definitions map[string]*compliancev1.DemoScenarioDefinition
}

// VerifyDemoRun imports and independently verifies one persisted demo evidence run without mutating it.
func VerifyDemoRun(ctx context.Context, reader ArtifactReader, runID string, source ProvenanceSource, verifiedAt time.Time) (*compliancev1.ComplianceVerificationReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	report := &compliancev1.ComplianceVerificationReport{
		ReportId: runID, VerifiedAt: timestamppb.New(verifiedAt), VerifierId: constants.DemoRunVerifierID,
		VerifierVersion: constants.DemoRunVerifierVersion,
	}
	verifier := &demoRunVerifier{ctx: ctx, reader: reader, runID: runID, source: source, report: report}
	if reader == nil || source == nil || !validPathElement(runID) {
		verifier.fail(constants.ErrInvalidEvidenceGraph, runID, "reader, provenance source, and canonical run ID are required")
		return report, nil
	}
	manifest := verifier.loadManifest()
	if manifest == nil {
		return report, nil
	}
	verifier.verifyManifest(manifest)
	results := verifier.loadResults(manifest)
	verifier.verifyArtifacts(manifest, results)
	verifier.verifyRootEntries()
	report.Valid = len(report.Failures) == 0
	return report, nil
}

func (v *demoRunVerifier) loadManifest() *compliancev1.DemoManifest {
	path := v.runPath(constants.DemoRunManifestFilename)
	body, err := v.read(path)
	if err != nil {
		v.fail(classifyReadError(err), path, err.Error())
		return nil
	}
	manifest := &compliancev1.DemoManifest{}
	if err := compliancev1.UnmarshalCanonical(body, manifest); err != nil {
		v.fail(constants.ErrEvidenceArtifactMalformed, path, err.Error())
		return nil
	}
	return manifest
}

func (v *demoRunVerifier) verifyManifest(manifest *compliancev1.DemoManifest) {
	assertions, frameworks, _, err := catalog.LoadCanonicalCatalogs()
	if err != nil {
		v.fail(constants.ErrInvalidEvidenceGraph, v.runID, err.Error())
		return
	}
	scenarioCatalog, err := catalog.LoadDemoScenarioCatalog(assertions, frameworks)
	if err != nil {
		v.fail(constants.ErrInvalidEvidenceGraph, v.runID, err.Error())
		return
	}
	v.definitions = make(map[string]*compliancev1.DemoScenarioDefinition)
	expectedRefs := make([]string, 0)
	for _, definition := range scenarioCatalog.GetDefinitions() {
		if strings.HasPrefix(definition.GetScenarioId(), manifest.GetDemoId()+"-") {
			key := versionedKey(definition.GetScenarioId(), definition.GetScenarioVersion())
			v.definitions[key] = definition
			expectedRefs = append(expectedRefs, key)
		}
	}
	definitions := make([]*compliancev1.DemoScenarioDefinition, 0, len(v.definitions))
	for _, definition := range v.definitions {
		definitions = append(definitions, definition)
	}
	if err := catalog.ValidateDemoManifest(manifest, definitions, frameworks); err != nil {
		v.fail(constants.ErrInvalidEvidenceGraph, constants.DemoRunManifestFilename, err.Error())
	}
	if manifest.GetRunId() != v.runID || manifest.GetScopeId() != demoScope(manifest.GetDemoId()) {
		v.fail(constants.ErrEvidenceScopeMismatch, constants.DemoRunManifestFilename, "manifest run or scope does not match the selected run")
	}
	if manifest.GetDemoVersion() != constants.DemoVersion || len(v.definitions) == 0 {
		v.fail(constants.ErrInvalidEvidenceGraph, constants.DemoRunManifestFilename, "manifest demo identity or version is unsupported")
	}
	actualRefs := make([]string, 0, len(manifest.GetScenarioDefinitionRefs()))
	for _, reference := range manifest.GetScenarioDefinitionRefs() {
		actualRefs = append(actualRefs, versionedKey(reference.GetId(), reference.GetVersion()))
	}
	if !equalStringSets(actualRefs, expectedRefs) {
		v.fail(constants.ErrUnresolvedReference, constants.DemoRunManifestFilename, "manifest scenario definitions do not match the canonical demo catalog")
	}
	v.verifyProvenance(manifest)
}

func (v *demoRunVerifier) verifyProvenance(manifest *compliancev1.DemoManifest) {
	artifacts, err := v.source.Artifacts(v.ctx, manifest.GetDemoId())
	if err != nil {
		v.fail(constants.ErrUnresolvedReference, constants.DemoRunManifestFilename, fmt.Sprintf("load provenance: %v", err))
		return
	}
	expected := make(map[string]string, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.Name == "" || filepath.IsAbs(artifact.Name) || !validRelativePath(artifact.Name) {
			v.fail(constants.ErrEvidenceArtifactMalformed, constants.DemoRunManifestFilename, "provenance source returned an invalid artifact name")
			continue
		}
		digest := sha256.Sum256(artifact.Body)
		expected[filepath.Clean(artifact.Name)] = hex.EncodeToString(digest[:])
	}
	actual := make(map[string]string, len(manifest.GetProvenanceHashes()))
	for _, digest := range manifest.GetProvenanceHashes() {
		actual[filepath.Clean(digest.GetName())] = digest.GetSha256()
	}
	if len(expected) != len(actual) {
		v.fail(constants.ErrChecksumMismatch, constants.DemoRunManifestFilename, "manifest provenance set is incomplete or contains unexpected entries")
	}
	for name, digest := range expected {
		if actual[name] != digest {
			v.fail(constants.ErrChecksumMismatch, name, "manifest provenance digest does not match the source artifact")
		}
	}
}

func (v *demoRunVerifier) loadResults(manifest *compliancev1.DemoManifest) []*compliancev1.DemoScenarioResult {
	path := v.runPath(constants.DemoRunResultsFilename)
	body, err := v.read(path)
	if err != nil {
		v.fail(classifyReadError(err), path, err.Error())
		return nil
	}
	lines := bytes.Split(body, []byte{'\n'})
	if len(lines) == 0 || len(lines) > constants.DemoRunMaxResults {
		v.fail(constants.ErrEvidenceArtifactMalformed, path, "scenario result count is empty or exceeds the limit")
		return nil
	}
	results := make([]*compliancev1.DemoScenarioResult, 0, len(lines))
	seenResults := make(map[string]struct{}, len(lines))
	seenScenarios := make(map[string]struct{}, len(lines))
	for index, line := range lines {
		subject := fmt.Sprintf("%s#%d", path, index+1)
		result := &compliancev1.DemoScenarioResult{}
		if len(line) == 0 {
			v.fail(constants.ErrEvidenceArtifactMalformed, subject, "empty JSONL record")
			continue
		}
		if err := compliancev1.UnmarshalCanonical(line, result); err != nil {
			v.fail(constants.ErrEvidenceArtifactMalformed, subject, err.Error())
			continue
		}
		key := versionedKey(result.GetScenarioRef().GetId(), result.GetScenarioRef().GetVersion())
		definition := v.definitions[key]
		if err := catalog.ValidateDemoScenarioResult(result, definition, manifest.GetScopeId()); err != nil {
			v.fail(constants.ErrInvalidEvidenceGraph, subject, err.Error())
		}
		if result.GetRunId() != v.runID || result.GetDemoId() != manifest.GetDemoId() || result.GetScopeId() != manifest.GetScopeId() || result.GetResultId() != v.runID+":"+result.GetScenarioRef().GetId() {
			v.fail(constants.ErrEvidenceScopeMismatch, subject, "scenario result identity does not match its manifest")
		}
		if result.GetStartedAt().AsTime().Before(manifest.GetGeneratedAt().AsTime()) {
			v.fail(constants.ErrEvidenceScopeMismatch, subject, "scenario result predates its manifest")
		}
		if _, duplicate := seenResults[result.GetResultId()]; duplicate {
			v.fail(constants.ErrInvalidEvidenceGraph, subject, "duplicate result ID")
		}
		if _, duplicate := seenScenarios[key]; duplicate {
			v.fail(constants.ErrInvalidEvidenceGraph, subject, "duplicate scenario result")
		}
		seenResults[result.GetResultId()] = struct{}{}
		seenScenarios[key] = struct{}{}
		results = append(results, result)
	}
	return results
}

func (v *demoRunVerifier) verifyArtifacts(_ *compliancev1.DemoManifest, results []*compliancev1.DemoScenarioResult) {
	expected := map[string]map[string]struct{}{
		constants.DemoRunReceiptsDirname: {}, constants.DemoRunPersistenceDirname: {}, constants.DemoRunStateObservationsDirname: {},
	}
	for _, result := range results {
		definition := v.definitions[versionedKey(result.GetScenarioRef().GetId(), result.GetScenarioRef().GetVersion())]
		resultRefs := make(map[string]struct{})
		for _, ref := range append(append([]string{}, result.GetReceiptRefs()...), result.GetStateObservationRefs()...) {
			resultRefs[ref] = struct{}{}
		}
		for _, ref := range result.GetProtocolChainRefs() {
			resultRefs[ref] = struct{}{}
		}
		for _, ref := range result.GetMetricRefs() {
			v.fail(constants.ErrUnresolvedReference, ref, "metric evidence bodies are not present in the demo run artifact layout")
		}
		for _, ref := range result.GetKsiRefs() {
			v.fail(constants.ErrUnresolvedReference, ref, "KSI evidence bodies are not present in the demo run artifact layout")
		}
		v.verifyStepReferences(result, resultRefs)
		for _, ref := range result.GetReceiptRefs() {
			prefix, digest, ok := parseContentReference(ref)
			if !ok {
				v.fail(constants.ErrEvidenceArtifactMalformed, ref, "receipt reference is not a canonical SHA-256 content address")
				continue
			}
			switch prefix {
			case "action-receipt":
				expected[constants.DemoRunReceiptsDirname][digest] = struct{}{}
				v.verifyReceipt(result, definition, ref, digest, expected)
			case "receipt-persistence":
				expected[constants.DemoRunPersistenceDirname][digest] = struct{}{}
			default:
				v.fail(constants.ErrEvidenceArtifactMalformed, ref, "unsupported receipt evidence type")
			}
		}
		for _, ref := range result.GetStateObservationRefs() {
			_, digest, ok := parseExpectedContentReference(ref, "state-observation")
			if !ok {
				v.fail(constants.ErrEvidenceArtifactMalformed, ref, "state observation reference is malformed")
				continue
			}
			expected[constants.DemoRunStateObservationsDirname][digest] = struct{}{}
			v.verifyObservation(result, ref, digest)
		}
	}
	for directory, digests := range expected {
		v.verifyArtifactDirectory(directory, digests)
	}
}

func (v *demoRunVerifier) verifyReceipt(result *compliancev1.DemoScenarioResult, definition *compliancev1.DemoScenarioDefinition, ref, digest string, expected map[string]map[string]struct{}) {
	path := v.runPath(constants.DemoRunReceiptsDirname, digest+constants.FileExtJSON)
	body, err := v.read(path)
	if err != nil {
		v.fail(classifyReadError(err), ref, err.Error())
		return
	}
	receipt := &operatorv1.ActionReceipt{}
	if err := compliancev1.UnmarshalCanonical(body, receipt); err != nil {
		v.fail(constants.ErrEvidenceArtifactMalformed, ref, err.Error())
		return
	}
	if !digestMatches(body, digest) {
		v.fail(constants.ErrChecksumMismatch, ref, "receipt content digest does not match its reference")
	}
	if !contains(result.GetTransactionIds(), receipt.GetTransactionId()) {
		v.fail(constants.ErrEvidenceScopeMismatch, ref, "receipt transaction is not bound to the scenario result")
	}
	if !receiptInvestigationBound(receipt, result.GetInvestigationIds()) && len(result.GetInvestigationIds()) > 0 {
		v.fail(constants.ErrEvidenceScopeMismatch, ref, "receipt investigation is not bound to the scenario result")
	}
	publicKey, keyErr := signerPublicKey(receipt.GetSignerKeyId())
	if keyErr != nil {
		v.fail(constants.ErrTrustedSignerKeyNotFound, ref, keyErr.Error())
	} else {
		if err := governance.VerifyActionReceiptSignature(receipt, publicKey); err != nil {
			v.fail(constants.ErrActionReceiptSignatureInvalid, ref, err.Error())
		}
		if err := governance.VerifyReceiptPersistenceAttestation(receipt, publicKey); err != nil {
			v.fail(receiptAttestationError(err), ref, err.Error())
		}
	}
	attestation := receipt.GetFinalPersistenceAttestation()
	if attestation == nil {
		v.fail(constants.ErrReceiptPersistenceAttestationMissing, ref, "receipt has no final-persistence attestation")
		return
	}
	attestationBody, err := compliancev1.MarshalCanonical(attestation)
	if err != nil {
		v.fail(constants.ErrEvidenceArtifactMalformed, ref, err.Error())
		return
	}
	persistenceRef := contentReferenceForBody("receipt-persistence", attestationBody)
	_, persistenceDigest, _ := parseContentReference(persistenceRef)
	expected[constants.DemoRunPersistenceDirname][persistenceDigest] = struct{}{}
	if !contains(result.GetReceiptRefs(), persistenceRef) {
		v.fail(constants.ErrUnresolvedReference, persistenceRef, "receipt persistence reference is absent from the scenario result")
	}
	v.verifyPersistence(attestation, persistenceRef, persistenceDigest)
	if definition != nil {
		grade, err := scenarios.GradeProtocolChain(receipt, definition.GetRequiredDeterministicStages())
		if err != nil {
			v.fail(constants.ErrInvalidEvidenceGraph, ref, fmt.Sprintf("protocol chain: %v", err))
		} else if !contains(result.GetProtocolChainRefs(), grade.StageEvidenceRef) {
			v.fail(constants.ErrUnresolvedReference, grade.StageEvidenceRef, "recomputed protocol-chain reference is absent from the scenario result")
		}
	}
}

func (v *demoRunVerifier) verifyPersistence(expected *operatorv1.ReceiptPersistenceAttestation, ref, digest string) {
	path := v.runPath(constants.DemoRunPersistenceDirname, digest+constants.FileExtJSON)
	body, err := v.read(path)
	if err != nil {
		v.fail(classifyReadError(err), ref, err.Error())
		return
	}
	attestation := &operatorv1.ReceiptPersistenceAttestation{}
	if err := compliancev1.UnmarshalCanonical(body, attestation); err != nil {
		v.fail(constants.ErrEvidenceArtifactMalformed, ref, err.Error())
		return
	}
	if !digestMatches(body, digest) {
		v.fail(constants.ErrChecksumMismatch, ref, "persistence content digest does not match its reference")
	}
	if !proto.Equal(expected, attestation) {
		v.fail(constants.ErrInvalidEvidenceGraph, ref, "standalone persistence attestation differs from the signed receipt")
	}
}

func (v *demoRunVerifier) verifyObservation(result *compliancev1.DemoScenarioResult, ref, digest string) {
	path := v.runPath(constants.DemoRunStateObservationsDirname, digest+constants.FileExtJSON)
	body, err := v.read(path)
	if err != nil {
		v.fail(classifyReadError(err), ref, err.Error())
		return
	}
	if !digestMatches(body, digest) {
		v.fail(constants.ErrChecksumMismatch, ref, "state-observation digest does not match its reference")
	}
	if err := validateCanonicalJSON(body); err != nil {
		v.fail(constants.ErrEvidenceArtifactMalformed, ref, err.Error())
	}
	for _, step := range result.GetStepResults() {
		if contains(step.GetEvidenceRefs(), ref) && step.GetProtocolResult() == string(body) {
			return
		}
	}
	v.fail(constants.ErrUnresolvedReference, ref, "state-observation body is not bound to a scenario step")
}

func (v *demoRunVerifier) verifyStepReferences(result *compliancev1.DemoScenarioResult, artifactRefs map[string]struct{}) {
	identities := make(map[string]struct{})
	for _, values := range [][]string{result.GetAttemptIds(), result.GetExecutionIds(), result.GetInvestigationIds(), result.GetTransactionIds()} {
		for _, value := range values {
			identities[value] = struct{}{}
		}
	}
	for _, step := range result.GetStepResults() {
		for _, ref := range step.GetEvidenceRefs() {
			_, artifact := artifactRefs[ref]
			_, identity := identities[ref]
			if !artifact && !identity {
				v.fail(constants.ErrUnresolvedReference, ref, "step evidence reference is not declared by the scenario result")
			}
		}
	}
}

func (v *demoRunVerifier) verifyArtifactDirectory(directory string, expected map[string]struct{}) {
	path := v.runPath(directory)
	entries, err := v.reader.ReadDir(v.ctx, path)
	if err != nil {
		if errors.Is(err, constants.ErrNotFound) && len(expected) == 0 {
			return
		}
		v.fail(classifyReadError(err), path, err.Error())
		return
	}
	if len(entries) > constants.DemoRunMaxArtifactsPerDirectory {
		v.fail(constants.ErrEvidenceArtifactTooLarge, path, "artifact count exceeds the verifier limit")
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), constants.FileExtJSON) {
			v.fail(constants.ErrUnexpectedEvidenceArtifact, filepath.Join(path, entry.Name()), "artifact directory contains an unsupported entry")
			continue
		}
		digest := strings.TrimSuffix(entry.Name(), constants.FileExtJSON)
		if _, ok := expected[digest]; !ok {
			v.fail(constants.ErrUnexpectedEvidenceArtifact, filepath.Join(path, entry.Name()), "artifact is not referenced by any scenario result")
		}
	}
}

func (v *demoRunVerifier) verifyRootEntries() {
	path := v.runPath()
	entries, err := v.reader.ReadDir(v.ctx, path)
	if err != nil {
		v.fail(classifyReadError(err), path, err.Error())
		return
	}
	allowed := map[string]bool{
		constants.DemoRunManifestFilename: false, constants.DemoRunResultsFilename: false,
		constants.DemoRunReceiptsDirname: true, constants.DemoRunPersistenceDirname: true, constants.DemoRunStateObservationsDirname: true,
	}
	for _, entry := range entries {
		directory, ok := allowed[entry.Name()]
		if !ok || directory != entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			v.fail(constants.ErrUnexpectedEvidenceArtifact, filepath.Join(path, entry.Name()), "run directory contains an unsupported entry")
		}
	}
}

func (v *demoRunVerifier) read(path string) ([]byte, error) {
	body, err := v.reader.ReadFile(v.ctx, path)
	if err != nil {
		return nil, err
	}
	if len(body) > constants.DemoRunMaxArtifactBytes {
		return nil, constants.ErrEvidenceArtifactTooLarge
	}
	return body, nil
}

func (v *demoRunVerifier) fail(code error, subject, reason string) {
	v.report.Failures = append(v.report.Failures, &compliancev1.VerificationFailure{Code: code.Error(), SubjectRef: subject, Reason: reason})
}

func (v *demoRunVerifier) runPath(parts ...string) string {
	base := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.DemoEvidenceDirname, v.runID)
	return filepath.Join(append([]string{base}, parts...)...)
}

func parseContentReference(reference string) (string, string, bool) {
	parts := strings.Split(reference, ":")
	if len(parts) != 3 || parts[0] == "" || parts[1] != "sha256" || len(parts[2]) != sha256.Size*2 || strings.ToLower(parts[2]) != parts[2] {
		return "", "", false
	}
	decoded, err := hex.DecodeString(parts[2])
	return parts[0], parts[2], err == nil && len(decoded) == sha256.Size
}

func parseExpectedContentReference(reference, expectedPrefix string) (string, string, bool) {
	prefix, digest, ok := parseContentReference(reference)
	return prefix, digest, ok && prefix == expectedPrefix
}

func signerPublicKey(keyID string) (ed25519.PublicKey, error) {
	decoded, err := hex.DecodeString(keyID)
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return nil, constants.ErrTrustedSignerKeyNotFound
	}
	return ed25519.PublicKey(decoded), nil
}

func receiptInvestigationBound(receipt *operatorv1.ActionReceipt, investigationIDs []string) bool {
	for _, stage := range receipt.GetDeterministicStageEvidence() {
		if contains(investigationIDs, stage.GetInvestigationId()) {
			return true
		}
	}
	return false
}

func receiptAttestationError(err error) error {
	for _, target := range []error{constants.ErrReceiptPersistenceAttestationMissing, constants.ErrReceiptPersistenceSignatureMismatch, constants.ErrReceiptPersistenceAttestationInvalid} {
		if errors.Is(err, target) {
			return target
		}
	}
	return constants.ErrReceiptPersistenceAttestationInvalid
}

func validateCanonicalJSON(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("JSON contains trailing data")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return err
	}
	if !bytes.Equal(body, compact.Bytes()) {
		return fmt.Errorf("JSON is not canonical compact encoding")
	}
	return nil
}

func contentReferenceForBody(prefix string, body []byte) string {
	digest := sha256.Sum256(body)
	return prefix + ":sha256:" + hex.EncodeToString(digest[:])
}

func digestMatches(body []byte, expected string) bool {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]) == expected
}

func classifyReadError(err error) error {
	if errors.Is(err, constants.ErrNotFound) {
		return constants.ErrUnresolvedReference
	}
	if errors.Is(err, constants.ErrEvidenceArtifactTooLarge) {
		return constants.ErrEvidenceArtifactTooLarge
	}
	return constants.ErrInvalidEvidenceGraph
}

func demoScope(demoID string) string {
	switch demoID {
	case constants.DemosOrgFedRAMP:
		return constants.DemoScopeFedRAMP
	case constants.DemosOrgDHS:
		return constants.DemoScopeDHS
	case constants.DemosOrgFinance:
		return constants.DemoScopeFinance
	case constants.DemosOrgHealthcare:
		return constants.DemoScopeHealthcare
	default:
		return ""
	}
}

func versionedKey(id, version string) string { return id + "@" + version }

func equalStringSets(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	sort.Strings(left)
	sort.Strings(right)
	return strings.Join(left, "\x00") == strings.Join(right, "\x00")
}

func validPathElement(value string) bool {
	return value != "" && value != "." && value != ".." && filepath.Base(value) == value && !filepath.IsAbs(value)
}

func validRelativePath(value string) bool {
	clean := filepath.Clean(value)
	return clean != "." && clean != ".." && !filepath.IsAbs(clean) && !strings.HasPrefix(clean, ".."+string(os.PathSeparator))
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
