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
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type OperationalExportImporter struct {
	reader        ArtifactReader
	trust         AssessedSignerSource
	inventoryPath string
	sourceRoot    string
	scopeID       string
	admission     *compliancev1.AssessmentSourceAdmission
	verifiedAt    time.Time
	nowFunc       func() time.Time
}

func NewOperationalExportImporter(reader ArtifactReader, trust AssessedSignerSource, inventoryPath, sourceRoot, scopeID string, admission *compliancev1.AssessmentSourceAdmission, verifiedAt time.Time, nowFunc func() time.Time) *OperationalExportImporter {
	if nowFunc == nil {
		nowFunc = time.Now
	}
	return &OperationalExportImporter{reader: reader, trust: trust, inventoryPath: inventoryPath, sourceRoot: sourceRoot, scopeID: scopeID, admission: admission, verifiedAt: verifiedAt, nowFunc: nowFunc}
}

func (i *OperationalExportImporter) SourceID() string {
	return "operational-export"
}

func (i *OperationalExportImporter) Import(ctx context.Context) ([]EvidenceNode, error) {
	if i == nil || i.reader == nil || i.trust == nil || !ValidRelativePath(i.inventoryPath) || i.scopeID == "" || i.admission == nil || i.admission.GetAdmissionId() == "" || i.admission.GetRunId() == "" || i.verifiedAt.IsZero() {
		return nil, fmt.Errorf("%w: operational export reader, trust, inventory, scope, admission, run, and assessment time are required", constants.ErrInvalidEvidenceGraph)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	inventoryBody, err := i.reader.ReadFile(ctx, i.inventoryPath)
	if err != nil {
		return nil, fmt.Errorf("%w: read operational inventory: %w", constants.ErrEvidenceImporterFailed, err)
	}
	inventory := &OperationalSourceInventory{}
	if err := json.Unmarshal(inventoryBody, inventory); err != nil {
		return nil, fmt.Errorf("%w: decode operational inventory: %w", constants.ErrEvidenceArtifactMalformed, err)
	}
	if !i.inventoryMatchesAdmission(inventory) {
		return nil, fmt.Errorf("%w: operational inventory does not match its protected source admission", constants.ErrEvidenceScopeMismatch)
	}
	artifacts := append([]OperationalExportArtifact(nil), inventory.Artifacts...)
	sort.Slice(artifacts, func(left, right int) bool { return artifacts[left].RelativePath < artifacts[right].RelativePath })
	entriesByType := make(map[ArtifactType][]OperationalExportArtifact)
	seenPaths := make(map[string]struct{}, len(artifacts))
	seenArtifactIDs := make(map[string]struct{}, len(artifacts))
	for _, artifact := range artifacts {
		artifactType := ArtifactType(artifact.ArtifactType)
		if !ValidRelativePath(artifact.RelativePath) || artifact.SHA256 == "" || artifact.ArtifactID == "" || artifact.TransactionID == "" || artifact.ProducedAtUTC == "" || artifactType != ArtifactTypeActionReceipt && artifactType != ArtifactTypeReceiptPersistence && artifactType != ArtifactTypeCommitment {
			return nil, fmt.Errorf("%w: operational artifact inventory entry is incomplete", constants.ErrEvidenceArtifactMalformed)
		}
		if _, duplicate := seenPaths[artifact.RelativePath]; duplicate {
			return nil, fmt.Errorf("%w: duplicate operational artifact path %s", constants.ErrEvidenceDuplicateID, artifact.RelativePath)
		}
		if _, duplicate := seenArtifactIDs[artifact.ArtifactID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate operational artifact identity %s", constants.ErrEvidenceDuplicateID, artifact.ArtifactID)
		}
		seenPaths[artifact.RelativePath] = struct{}{}
		seenArtifactIDs[artifact.ArtifactID] = struct{}{}
		artifactPath := i.sourceArtifactPath(artifact.RelativePath)
		body, err := i.reader.ReadFile(ctx, artifactPath)
		if err != nil {
			return nil, fmt.Errorf("%w: read operational artifact %s: %w", constants.ErrEvidenceImporterFailed, artifactPath, err)
		}
		if !matchesOperationalArtifact(artifact, body) {
			return nil, fmt.Errorf("%w: operational artifact %s does not match its inventory digest", constants.ErrChecksumMismatch, artifact.RelativePath)
		}
		entriesByType[artifactType] = append(entriesByType[artifactType], artifact)
	}
	if inventory.ReceiptCount != len(entriesByType[ArtifactTypeActionReceipt]) || inventory.PersistenceCount != len(entriesByType[ArtifactTypeReceiptPersistence]) || inventory.CommitmentCount != len(entriesByType[ArtifactTypeCommitment]) {
		return nil, fmt.Errorf("%w: operational artifact counts do not match inventory", constants.ErrEvidenceArtifactMalformed)
	}

	receiptEntries := entriesByType[ArtifactTypeActionReceipt]
	receiptBindings := make(map[string]ReceiptImportBinding, len(receiptEntries))
	for _, entry := range receiptEntries {
		entryPath := i.sourceArtifactPath(entry.RelativePath)
		body, err := i.reader.ReadFile(ctx, entryPath)
		if err != nil {
			return nil, err
		}
		transactionID, err := receiptTransactionID(body)
		if err != nil {
			return nil, err
		}
		if transactionID != entry.TransactionID {
			return nil, fmt.Errorf("%w: operational receipt transaction does not match inventory", constants.ErrEvidenceScopeMismatch)
		}
		if _, exists := receiptBindings[transactionID]; exists {
			return nil, fmt.Errorf("%w: duplicate operational receipt transaction %s", constants.ErrEvidenceDuplicateID, transactionID)
		}
		receiptBindings[transactionID] = ReceiptImportBinding{Reference: entry.ArtifactID, Path: entryPath, ScopeID: i.scopeID, RunID: i.admission.GetRunId()}
	}

	nodes := make([]EvidenceNode, 0, len(artifacts))
	for _, entry := range receiptEntries {
		binding := receiptBindingsForEntry(receiptBindings, entry, i.scopeID, i.admission.GetRunId())
		imported, err := NewReceiptImporterAt(i.reader, i.trust, binding, i.nowFunc).Import(ctx)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, imported...)
		body, err := i.reader.ReadFile(ctx, binding.Path)
		if err != nil {
			return nil, err
		}
		receipt := &operatorv1.ActionReceipt{}
		if err := compliancev1.UnmarshalCanonical(body, receipt); err != nil {
			return nil, fmt.Errorf("%w: decode operational receipt for protocol chain: %w", constants.ErrEvidenceArtifactMalformed, err)
		}
		chainNode, err := operationalProtocolChainNode(receipt, imported[0])
		if err != nil {
			return nil, err
		}
		if chainNode != nil {
			nodes = append(nodes, *chainNode)
		}
	}
	for _, entry := range entriesByType[ArtifactTypeReceiptPersistence] {
		entryPath := i.sourceArtifactPath(entry.RelativePath)
		body, err := i.reader.ReadFile(ctx, entryPath)
		if err != nil {
			return nil, err
		}
		transactionID, err := persistenceTransactionID(body)
		if err != nil {
			return nil, err
		}
		if transactionID != entry.TransactionID {
			return nil, fmt.Errorf("%w: operational persistence transaction does not match inventory", constants.ErrEvidenceScopeMismatch)
		}
		receiptBinding, exists := receiptBindings[transactionID]
		if !exists {
			return nil, fmt.Errorf("%w: persistence attestation %s has no exported receipt", constants.ErrUnresolvedReference, transactionID)
		}
		imported, err := NewPersistenceImporterAt(i.reader, i.trust, PersistenceImportBinding{Reference: entry.ArtifactID, Path: entryPath, ReceiptReference: receiptBinding.Reference, ReceiptPath: receiptBinding.Path, ScopeID: i.scopeID, RunID: i.admission.GetRunId()}, i.nowFunc).Import(ctx)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, imported...)
	}
	for _, entry := range entriesByType[ArtifactTypeCommitment] {
		imported, err := NewCommitmentImporter(i.reader, i.trust, CommitmentImportBinding{Reference: entry.ArtifactID, Path: i.sourceArtifactPath(entry.RelativePath), ScopeID: i.scopeID, RunID: i.admission.GetRunId(), TransactionID: entry.TransactionID}, i.verifiedAt).Import(ctx)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, imported...)
	}
	return nodes, nil
}

func (i *OperationalExportImporter) inventoryMatchesAdmission(inventory *OperationalSourceInventory) bool {
	admission := i.admission
	return inventory != nil &&
		inventory.SchemaVersion == operationalExportSchemaVersion &&
		inventory.ScopeID == i.scopeID &&
		inventory.AdmissionID == admission.GetAdmissionId() &&
		inventory.SourceKind == admission.GetSourceKind() &&
		inventory.SourceVersion == admission.GetSourceVersion() &&
		inventory.SourceScopeID == admission.GetSourceScopeId() &&
		inventory.OwnerRuntimeBoundary == admission.GetOwnerRuntimeBoundary() &&
		inventory.AcquisitionBoundary == admission.GetAcquisitionBoundary() &&
		inventory.RunID == admission.GetRunId() &&
		inventory.SnapshotID == admission.GetSnapshotId() &&
		inventory.VerifierID == admission.GetVerifierRef().GetId() &&
		inventory.VerifierVersion == admission.GetVerifierRef().GetVersion()
}

func (i *OperationalExportImporter) sourceArtifactPath(relativePath string) string {
	if i.sourceRoot == "" {
		return relativePath
	}
	return path.Join(i.sourceRoot, relativePath)
}

func operationalProtocolChainNode(receipt *operatorv1.ActionReceipt, receiptNode EvidenceNode) (*EvidenceNode, error) {
	if len(receipt.GetDeterministicStageEvidence()) == 0 {
		return nil, nil
	}
	chain, err := governance.ValidateDeterministicProtocolChain(receipt)
	if err != nil {
		return nil, fmt.Errorf("%w: validate operational protocol chain: %w", constants.ErrInvalidEvidenceGraph, err)
	}
	canonical, err := governance.CanonicalDeterministicStages(chain.Stages)
	if err != nil {
		return nil, err
	}
	_, digest, ok := ParseExpectedContentReference(chain.ContentReference, constants.DeterministicStagesReferencePrefix)
	if !ok || !VerifyDigest(canonical, digest) {
		return nil, fmt.Errorf("%w: operational deterministic stage digest does not match reference", constants.ErrChecksumMismatch)
	}
	return &EvidenceNode{
		ArtifactID:         chain.ContentReference,
		ArtifactType:       ArtifactTypeProtocolChain,
		SHA256:             digest,
		MediaType:          constants.MediaTypeOctetStream,
		SchemaRef:          "g8e.operator.v1.DeterministicStageEvidence[]",
		ProducerIdentity:   receiptNode.ProducerIdentity,
		ProducedAt:         receiptNode.ProducedAt,
		ScopeID:            receiptNode.ScopeID,
		RunID:              receiptNode.RunID,
		TransactionID:      receiptNode.TransactionID,
		VerificationStatus: receiptNode.VerificationStatus,
		VerifierID:         receiptNode.VerifierID,
		VerifierVersion:    receiptNode.VerifierVersion,
		VerifiedAt:         receiptNode.VerifiedAt,
		CanonicalBytes:     canonical,
		References:         []string{receiptNode.ArtifactID},
	}, nil
}

func receiptBindingsForEntry(bindings map[string]ReceiptImportBinding, entry OperationalExportArtifact, scopeID, runID string) ReceiptImportBinding {
	for _, binding := range bindings {
		if binding.Reference == entry.ArtifactID {
			binding.ScopeID = scopeID
			binding.RunID = runID
			return binding
		}
	}
	return ReceiptImportBinding{}
}

func receiptTransactionID(body []byte) (string, error) {
	receipt := &operatorv1.ActionReceipt{}
	if err := compliancev1.UnmarshalCanonical(body, receipt); err != nil || receipt.GetTransactionId() == "" {
		return "", fmt.Errorf("%w: operational receipt transaction binding is incomplete", constants.ErrEvidenceArtifactMalformed)
	}
	return receipt.GetTransactionId(), nil
}

func persistenceTransactionID(body []byte) (string, error) {
	attestation := &operatorv1.ReceiptPersistenceAttestation{}
	if err := compliancev1.UnmarshalCanonical(body, attestation); err != nil || attestation.GetTransactionId() == "" {
		return "", fmt.Errorf("%w: operational persistence transaction binding is incomplete", constants.ErrEvidenceArtifactMalformed)
	}
	return attestation.GetTransactionId(), nil
}

func matchesOperationalArtifact(artifact OperationalExportArtifact, body []byte) bool {
	digest := sha256.Sum256(body)
	digestHex := hex.EncodeToString(digest[:])
	return digestHex == artifact.SHA256 && ContentAddress(ArtifactType(artifact.ArtifactType), body) == artifact.ArtifactID
}
