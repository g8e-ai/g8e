// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evidence

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// DemoRunImporter imports a persisted demo evidence run as EvidenceNode
// records. It reads the manifest, scenario results, receipts, persistence
// attestations, state observations, and metrics from the runtime tree,
// validates canonical bytes and content digests, verifies receipt signatures
// and persistence attestations, and yields typed nodes without mutating the
// source. The importer does not perform catalog-level validation; that
// remains the responsibility of the demo run verifier for the verification
// report path. The importer produces evidence graph nodes for cross-source
// analysis.
type DemoRunImporter struct {
	reader  ArtifactReader
	runID   string
	source  ProvenanceSource
	nowFunc func() time.Time
}

// NewDemoRunImporter creates an importer for the given run ID. The reader
// provides access to the runtime tree; the source provides provenance
// artifacts for manifest verification.
func NewDemoRunImporter(reader ArtifactReader, runID string, source ProvenanceSource) *DemoRunImporter {
	return &DemoRunImporter{
		reader:  reader,
		runID:   runID,
		source:  source,
		nowFunc: time.Now,
	}
}

// SourceID returns "demo-run".
func (i *DemoRunImporter) SourceID() string {
	return "demo-run"
}

// Import reads the demo run evidence and yields EvidenceNode records.
func (i *DemoRunImporter) Import(ctx context.Context) ([]EvidenceNode, error) {
	if i.reader == nil || i.source == nil || !ValidPathElement(i.runID) {
		return nil, fmt.Errorf("%w: reader, provenance source, and canonical run ID are required", constants.ErrInvalidEvidenceGraph)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	manifest, manifestBody, err := i.loadManifest(ctx)
	if err != nil {
		return nil, err
	}
	scopeID := manifest.GetScopeId()
	runID := manifest.GetRunId()
	nodes := make([]EvidenceNode, 0, 32)

	definitionNodes, definitionIndex, err := i.loadDefinitions(ctx, manifest)
	if err != nil {
		return nil, err
	}
	nodes = append(nodes, definitionNodes...)

	manifestRefs := make([]string, 0, len(definitionNodes))
	for _, node := range definitionNodes {
		manifestRefs = append(manifestRefs, node.ArtifactID)
	}

	manifestNode := EvidenceNode{
		ArtifactID:         ContentAddress(ArtifactTypeDemoManifest, manifestBody),
		ArtifactType:       ArtifactTypeDemoManifest,
		SHA256:             digestHex(manifestBody),
		MediaType:          "application/json",
		SchemaRef:          "g8e.compliance.v1.DemoManifest",
		ProducerIdentity:   manifest.GetDemoId(),
		ProducedAt:         manifest.GetGeneratedAt().AsTime(),
		ScopeID:            scopeID,
		RunID:              runID,
		VerificationStatus: VerificationStatusVerified,
		VerifierID:         constants.DemoRunVerifierID,
		VerifierVersion:    constants.DemoRunVerifierVersion,
		VerifiedAt:         i.nowFunc(),
		BundlePath:         filepath.Join(constants.DemoRunManifestFilename),
		CanonicalBytes:     manifestBody,
		References:         manifestRefs,
	}
	nodes = append(nodes, manifestNode)

	results, resultNodes, err := i.loadResults(ctx, manifest, scopeID, runID, definitionIndex)
	if err != nil {
		return nil, err
	}
	nodes = append(nodes, resultNodes...)

	receiptNodes, err := i.loadReceipts(ctx, results, scopeID, runID)
	if err != nil {
		return nil, err
	}
	nodes = append(nodes, receiptNodes...)

	observationNodes, err := i.loadStateObservations(ctx, results, scopeID, runID)
	if err != nil {
		return nil, err
	}
	nodes = append(nodes, observationNodes...)

	metricNodes, err := i.loadMetrics(ctx, results, scopeID, runID)
	if err != nil {
		return nil, err
	}
	nodes = append(nodes, metricNodes...)

	for idx := range nodes {
		if nodes[idx].References == nil {
			nodes[idx].References = []string{}
		}
	}
	return nodes, nil
}

func (i *DemoRunImporter) loadManifest(ctx context.Context) (*compliancev1.DemoManifest, []byte, error) {
	path := i.runPath(constants.DemoRunManifestFilename)
	result, err := ReadAndDigest(i.reader, ctx, path, constants.DemoRunMaxArtifactBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s: %v", constants.ErrEvidenceImporterFailed, path, err)
	}
	manifest := &compliancev1.DemoManifest{}
	if err := compliancev1.UnmarshalCanonical(result.Bytes, manifest); err != nil {
		return nil, nil, fmt.Errorf("%w: %s: %v", constants.ErrEvidenceArtifactMalformed, path, err)
	}
	if manifest.GetRunId() != i.runID {
		return nil, nil, fmt.Errorf("%w: manifest run ID %s does not match importer run ID %s", constants.ErrEvidenceScopeMismatch, manifest.GetRunId(), i.runID)
	}
	return manifest, result.Bytes, nil
}

func (i *DemoRunImporter) loadDefinitions(ctx context.Context, manifest *compliancev1.DemoManifest) ([]EvidenceNode, map[string]string, error) {
	scopeID := manifest.GetScopeId()
	runID := manifest.GetRunId()
	artifacts, err := i.source.Definitions(ctx, manifest.GetDemoId())
	if err != nil {
		return nil, nil, fmt.Errorf("%w: load scenario definitions: %w", constants.ErrEvidenceImporterFailed, err)
	}
	manifestRefs := manifest.GetScenarioDefinitionRefs()
	definitionNodes := make([]EvidenceNode, 0, len(artifacts))
	definitionIndex := make(map[string]string, len(artifacts))
	seenIDs := make(map[string]bool, len(artifacts))
	for idx, artifact := range artifacts {
		body := artifact.Body
		if err := ValidateCanonicalJSON(body); err != nil {
			return nil, nil, fmt.Errorf("%w: %s#%d: %v", constants.ErrEvidenceArtifactMalformed, constants.ComplianceBundleDemoDefinitionsFilename, idx+1, err)
		}
		definition := &compliancev1.DemoScenarioDefinition{}
		if err := compliancev1.UnmarshalCanonical(body, definition); err != nil {
			return nil, nil, fmt.Errorf("%w: %s#%d: %v", constants.ErrEvidenceArtifactMalformed, constants.ComplianceBundleDemoDefinitionsFilename, idx+1, err)
		}
		artifactID := ContentAddress(ArtifactTypeDemoDefinition, body)
		if seenIDs[artifactID] {
			return nil, nil, fmt.Errorf("%w: %s#%d: duplicate scenario definition %s", constants.ErrEvidenceArtifactMalformed, constants.ComplianceBundleDemoDefinitionsFilename, idx+1, definition.GetScenarioId())
		}
		seenIDs[artifactID] = true
		key := versionedKey(definition.GetScenarioId(), definition.GetScenarioVersion())
		definitionIndex[key] = artifactID
		definitionNodes = append(definitionNodes, EvidenceNode{
			ArtifactID:         artifactID,
			ArtifactType:       ArtifactTypeDemoDefinition,
			SHA256:             digestHex(body),
			MediaType:          "application/json",
			SchemaRef:          "g8e.compliance.v1.DemoScenarioDefinition",
			ProducerIdentity:   manifest.GetDemoId(),
			ProducedAt:         manifest.GetGeneratedAt().AsTime(),
			ScopeID:            scopeID,
			RunID:              runID,
			ScenarioID:         definition.GetScenarioId(),
			VerificationStatus: VerificationStatusVerified,
			VerifierID:         constants.DemoRunVerifierID,
			VerifierVersion:    constants.DemoRunVerifierVersion,
			VerifiedAt:         i.nowFunc(),
			BundlePath:         fmt.Sprintf("%s#%d", constants.ComplianceBundleDemoDefinitionsFilename, idx+1),
			CanonicalBytes:     body,
			References:         []string{},
		})
	}
	for _, ref := range manifestRefs {
		key := versionedKey(ref.GetId(), ref.GetVersion())
		if _, ok := definitionIndex[key]; !ok {
			return nil, nil, fmt.Errorf("%w: manifest scenario definition %s is not in the source definition set", constants.ErrUnresolvedReference, key)
		}
	}
	return definitionNodes, definitionIndex, nil
}

func (i *DemoRunImporter) loadResults(ctx context.Context, manifest *compliancev1.DemoManifest, scopeID, runID string, definitionIndex map[string]string) ([]*compliancev1.DemoScenarioResult, []EvidenceNode, error) {
	path := i.runPath(constants.DemoRunResultsFilename)
	result, err := ReadAndDigest(i.reader, ctx, path, constants.DemoRunMaxArtifactBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s: %v", constants.ErrEvidenceImporterFailed, path, err)
	}
	lines := splitJSONL(result.Bytes)
	if len(lines) == 0 || len(lines) > constants.DemoRunMaxResults {
		return nil, nil, fmt.Errorf("%w: %s: result count is empty or exceeds the limit", constants.ErrEvidenceArtifactMalformed, path)
	}
	results := make([]*compliancev1.DemoScenarioResult, 0, len(lines))
	nodes := make([]EvidenceNode, 0, len(lines))
	for idx, line := range lines {
		scenarioResult := &compliancev1.DemoScenarioResult{}
		if err := compliancev1.UnmarshalCanonical(line, scenarioResult); err != nil {
			return nil, nil, fmt.Errorf("%w: %s#%d: %v", constants.ErrEvidenceArtifactMalformed, path, idx+1, err)
		}
		if scenarioResult.GetRunId() != runID || scenarioResult.GetScopeId() != scopeID {
			return nil, nil, fmt.Errorf("%w: %s#%d: result scope or run does not match manifest", constants.ErrEvidenceScopeMismatch, path, idx+1)
		}
		scenarioKey := versionedKey(scenarioResult.GetScenarioRef().GetId(), scenarioResult.GetScenarioRef().GetVersion())
		definitionID, ok := definitionIndex[scenarioKey]
		if len(definitionIndex) > 0 && !ok {
			return nil, nil, fmt.Errorf("%w: %s#%d: result scenario %s is not in the manifest definition set", constants.ErrUnresolvedReference, path, idx+1, scenarioKey)
		}
		results = append(results, scenarioResult)
		refs := append(append(append([]string{}, scenarioResult.GetReceiptRefs()...), scenarioResult.GetStateObservationRefs()...), scenarioResult.GetMetricRefs()...)
		if ok {
			refs = append(refs, definitionID)
		}
		node := EvidenceNode{
			ArtifactID:         ContentAddress(ArtifactTypeDemoResult, line),
			ArtifactType:       ArtifactTypeDemoResult,
			SHA256:             digestHex(line),
			MediaType:          "application/json",
			SchemaRef:          "g8e.compliance.v1.DemoScenarioResult",
			ProducerIdentity:   scenarioResult.GetDemoId(),
			ProducedAt:         scenarioResult.GetStartedAt().AsTime(),
			ScopeID:            scopeID,
			RunID:              runID,
			ScenarioID:         scenarioResult.GetScenarioRef().GetId(),
			VerificationStatus: VerificationStatusUnverified,
			BundlePath:         fmt.Sprintf("%s#%d", constants.DemoRunResultsFilename, idx+1),
			CanonicalBytes:     line,
			References:         refs,
		}
		nodes = append(nodes, node)
	}
	return results, nodes, nil
}

func (i *DemoRunImporter) loadReceipts(ctx context.Context, results []*compliancev1.DemoScenarioResult, scopeID, runID string) ([]EvidenceNode, error) {
	nodes := make([]EvidenceNode, 0, len(results)*2)
	for _, result := range results {
		for _, ref := range result.GetReceiptRefs() {
			prefix, digest, ok := ParseContentReference(ref)
			if !ok {
				continue
			}
			switch prefix {
			case "action-receipt":
				node, err := i.loadReceipt(ctx, ref, digest, result, scopeID, runID)
				if err != nil {
					return nil, err
				}
				nodes = append(nodes, node)
			case "receipt-persistence":
				node, err := i.loadPersistence(ctx, ref, digest, result, scopeID, runID)
				if err != nil {
					return nil, err
				}
				nodes = append(nodes, node)
			}
		}
	}
	return nodes, nil
}

func (i *DemoRunImporter) loadReceipt(ctx context.Context, ref, digest string, result *compliancev1.DemoScenarioResult, scopeID, runID string) (EvidenceNode, error) {
	path := i.runPath(constants.DemoRunReceiptsDirname, digest+constants.FileExtJSON)
	readResult, err := ReadAndDigest(i.reader, ctx, path, constants.DemoRunMaxArtifactBytes)
	if err != nil {
		return EvidenceNode{}, fmt.Errorf("%w: %s: %v", constants.ErrEvidenceImporterFailed, ref, err)
	}
	if !VerifyDigest(readResult.Bytes, digest) {
		return EvidenceNode{}, fmt.Errorf("%w: %s: receipt content digest does not match reference", constants.ErrChecksumMismatch, ref)
	}
	receipt := &operatorv1.ActionReceipt{}
	if err := compliancev1.UnmarshalCanonical(readResult.Bytes, receipt); err != nil {
		return EvidenceNode{}, fmt.Errorf("%w: %s: %v", constants.ErrEvidenceArtifactMalformed, ref, err)
	}
	verified := VerificationStatusUnverified
	verifierID := ""
	verifierVersion := ""
	verifiedAt := time.Time{}
	publicKey, keyErr := SignerPublicKey(receipt.GetSignerKeyId())
	if keyErr == nil {
		if sigErr := governance.VerifyActionReceiptSignature(receipt, publicKey); sigErr == nil {
			if persistErr := governance.VerifyReceiptPersistenceAttestation(receipt, publicKey); persistErr == nil {
				verified = VerificationStatusVerified
				verifierID = constants.DemoRunVerifierID
				verifierVersion = constants.DemoRunVerifierVersion
				verifiedAt = i.nowFunc()
			}
		}
	}
	if verified != VerificationStatusVerified {
		verified = VerificationStatusFailed
		verifierID = constants.DemoRunVerifierID
		verifierVersion = constants.DemoRunVerifierVersion
		verifiedAt = i.nowFunc()
	}
	persistenceRef := ""
	if receipt.GetFinalPersistenceAttestation() != nil {
		attestationBody, _ := compliancev1.MarshalCanonical(receipt.GetFinalPersistenceAttestation())
		persistenceRef = ContentReferenceForBody("receipt-persistence", attestationBody)
	}
	refs := []string{}
	if persistenceRef != "" {
		refs = append(refs, persistenceRef)
	}
	return EvidenceNode{
		ArtifactID:         ref,
		ArtifactType:       ArtifactTypeActionReceipt,
		SHA256:             digest,
		MediaType:          "application/json",
		SchemaRef:          "g8e.operator.v1.ActionReceipt",
		ProducerIdentity:   receipt.GetSignerKeyId(),
		ProducedAt:         time.UnixMilli(receipt.GetExecutedAtUnixMs()),
		ScopeID:            scopeID,
		RunID:              runID,
		ScenarioID:         result.GetScenarioRef().GetId(),
		TransactionID:      receipt.GetTransactionId(),
		VerificationStatus: verified,
		VerifierID:         verifierID,
		VerifierVersion:    verifierVersion,
		VerifiedAt:         verifiedAt,
		BundlePath:         filepath.Join(constants.DemoRunReceiptsDirname, digest+constants.FileExtJSON),
		CanonicalBytes:     readResult.Bytes,
		References:         refs,
	}, nil
}

func (i *DemoRunImporter) loadPersistence(ctx context.Context, ref, digest string, result *compliancev1.DemoScenarioResult, scopeID, runID string) (EvidenceNode, error) {
	path := i.runPath(constants.DemoRunPersistenceDirname, digest+constants.FileExtJSON)
	readResult, err := ReadAndDigest(i.reader, ctx, path, constants.DemoRunMaxArtifactBytes)
	if err != nil {
		return EvidenceNode{}, fmt.Errorf("%w: %s: %v", constants.ErrEvidenceImporterFailed, ref, err)
	}
	if !VerifyDigest(readResult.Bytes, digest) {
		return EvidenceNode{}, fmt.Errorf("%w: %s: persistence content digest does not match reference", constants.ErrChecksumMismatch, ref)
	}
	attestation := &operatorv1.ReceiptPersistenceAttestation{}
	if err := compliancev1.UnmarshalCanonical(readResult.Bytes, attestation); err != nil {
		return EvidenceNode{}, fmt.Errorf("%w: %s: %v", constants.ErrEvidenceArtifactMalformed, ref, err)
	}
	return EvidenceNode{
		ArtifactID:         ref,
		ArtifactType:       ArtifactTypeReceiptPersistence,
		SHA256:             digest,
		MediaType:          "application/json",
		SchemaRef:          "g8e.operator.v1.ReceiptPersistenceAttestation",
		ProducerIdentity:   attestation.GetSignerKeyId(),
		ProducedAt:         time.UnixMilli(attestation.GetPersistedAtUnixMs()),
		ScopeID:            scopeID,
		RunID:              runID,
		ScenarioID:         result.GetScenarioRef().GetId(),
		TransactionID:      attestation.GetTransactionId(),
		VerificationStatus: VerificationStatusVerified,
		VerifierID:         constants.DemoRunVerifierID,
		VerifierVersion:    constants.DemoRunVerifierVersion,
		VerifiedAt:         i.nowFunc(),
		BundlePath:         filepath.Join(constants.DemoRunPersistenceDirname, digest+constants.FileExtJSON),
		CanonicalBytes:     readResult.Bytes,
		References:         []string{},
	}, nil
}

func (i *DemoRunImporter) loadStateObservations(ctx context.Context, results []*compliancev1.DemoScenarioResult, scopeID, runID string) ([]EvidenceNode, error) {
	nodes := make([]EvidenceNode, 0, len(results))
	for _, result := range results {
		for _, ref := range result.GetStateObservationRefs() {
			_, digest, ok := ParseExpectedContentReference(ref, "state-observation")
			if !ok {
				continue
			}
			path := i.runPath(constants.DemoRunStateObservationsDirname, digest+constants.FileExtJSON)
			readResult, err := ReadAndDigest(i.reader, ctx, path, constants.DemoRunMaxArtifactBytes)
			if err != nil {
				return nil, fmt.Errorf("%w: %s: %v", constants.ErrEvidenceImporterFailed, ref, err)
			}
			if !VerifyDigest(readResult.Bytes, digest) {
				return nil, fmt.Errorf("%w: %s: state-observation digest does not match reference", constants.ErrChecksumMismatch, ref)
			}
			if err := ValidateCanonicalJSON(readResult.Bytes); err != nil {
				return nil, fmt.Errorf("%w: %s: %v", constants.ErrEvidenceArtifactMalformed, ref, err)
			}
			nodes = append(nodes, EvidenceNode{
				ArtifactID:         ref,
				ArtifactType:       ArtifactTypeStateObservation,
				SHA256:             digest,
				MediaType:          "application/json",
				SchemaRef:          "state-observation",
				ProducerIdentity:   result.GetDemoId(),
				ProducedAt:         result.GetCompletedAt().AsTime(),
				ScopeID:            scopeID,
				RunID:              runID,
				ScenarioID:         result.GetScenarioRef().GetId(),
				VerificationStatus: VerificationStatusUnverified,
				BundlePath:         filepath.Join(constants.DemoRunStateObservationsDirname, digest+constants.FileExtJSON),
				CanonicalBytes:     readResult.Bytes,
				References:         []string{},
			})
		}
	}
	return nodes, nil
}

func (i *DemoRunImporter) loadMetrics(ctx context.Context, results []*compliancev1.DemoScenarioResult, scopeID, runID string) ([]EvidenceNode, error) {
	nodes := make([]EvidenceNode, 0, len(results))
	for _, result := range results {
		for _, ref := range result.GetMetricRefs() {
			_, digest, ok := ParseExpectedContentReference(ref, "metric")
			if !ok {
				continue
			}
			path := i.runPath(constants.DemoRunMetricsDirname, digest+constants.FileExtJSON)
			readResult, err := ReadAndDigest(i.reader, ctx, path, constants.DemoRunMaxArtifactBytes)
			if err != nil {
				return nil, fmt.Errorf("%w: %s: %v", constants.ErrEvidenceImporterFailed, ref, err)
			}
			if !VerifyDigest(readResult.Bytes, digest) {
				return nil, fmt.Errorf("%w: %s: metric content digest does not match reference", constants.ErrChecksumMismatch, ref)
			}
			metric := &compliancev1.DemoMetricEvidence{}
			if err := compliancev1.UnmarshalCanonical(readResult.Bytes, metric); err != nil {
				return nil, fmt.Errorf("%w: %s: %v", constants.ErrEvidenceArtifactMalformed, ref, err)
			}
			refs := []string{}
			if metric.GetSourceEvidenceRef() != "" {
				refs = append(refs, metric.GetSourceEvidenceRef())
			}
			nodes = append(nodes, EvidenceNode{
				ArtifactID:         ref,
				ArtifactType:       ArtifactTypeDemoMetric,
				SHA256:             digest,
				MediaType:          "application/json",
				SchemaRef:          "g8e.compliance.v1.DemoMetricEvidence",
				ProducerIdentity:   constants.DemoMetricGraderID,
				ProducedAt:         metric.GetEvaluatedAt().AsTime(),
				ScopeID:            scopeID,
				RunID:              runID,
				ScenarioID:         result.GetScenarioRef().GetId(),
				VerificationStatus: VerificationStatusUnverified,
				BundlePath:         filepath.Join(constants.DemoRunMetricsDirname, digest+constants.FileExtJSON),
				CanonicalBytes:     readResult.Bytes,
				References:         refs,
			})
		}
	}
	return nodes, nil
}

func (i *DemoRunImporter) runPath(parts ...string) string {
	base := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.DemoEvidenceDirname, i.runID)
	return filepath.Join(append([]string{base}, parts...)...)
}

func digestHex(body []byte) string {
	d := sha256.Sum256(body)
	return hex.EncodeToString(d[:])
}

func splitJSONL(body []byte) [][]byte {
	lines := strings.Split(string(body), "\n")
	result := make([][]byte, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		result = append(result, []byte(trimmed))
	}
	return result
}

// The following declarations ensure the imports are used.
var _ proto.Message = (*compliancev1.DemoManifest)(nil)
var _ ed25519.PublicKey = ed25519.PublicKey{}
