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
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
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
		if !ValidRelativePath(artifact.RelativePath) || artifact.SHA256 == "" || artifact.ArtifactID == "" || artifact.ProducedAtUTC == "" || artifactType != ArtifactTypeActionReceipt && artifactType != ArtifactTypeReceiptPersistence && artifactType != ArtifactTypeCommitment && artifactType != ArtifactTypeAuditChainEntry {
			return nil, fmt.Errorf("%w: operational artifact inventory entry is incomplete", constants.ErrEvidenceArtifactMalformed)
		}
		if artifactType != ArtifactTypeAuditChainEntry && artifact.TransactionID == "" {
			return nil, fmt.Errorf("%w: operational artifact inventory entry is incomplete", constants.ErrEvidenceArtifactMalformed)
		}
		if artifactType == ArtifactTypeAuditChainEntry && artifact.Sequence <= 0 {
			return nil, fmt.Errorf("%w: operational audit chain entry sequence is missing", constants.ErrEvidenceArtifactMalformed)
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
	if inventory.ReceiptCount != len(entriesByType[ArtifactTypeActionReceipt]) || inventory.PersistenceCount != len(entriesByType[ArtifactTypeReceiptPersistence]) || inventory.CommitmentCount != len(entriesByType[ArtifactTypeCommitment]) || inventory.AuditChainCount != len(entriesByType[ArtifactTypeAuditChainEntry]) {
		return nil, fmt.Errorf("%w: operational artifact counts do not match inventory", constants.ErrEvidenceArtifactMalformed)
	}

	receiptEntries := entriesByType[ArtifactTypeActionReceipt]
	receiptBindings := make(map[string]ReceiptImportBinding, len(receiptEntries))
	receiptsByTransaction := make(map[string]*operatorv1.ActionReceipt, len(receiptEntries))
	receiptNodesByTransaction := make(map[string]EvidenceNode, len(receiptEntries))
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
		receiptsByTransaction[entry.TransactionID] = receipt
		receiptNodesByTransaction[entry.TransactionID] = imported[0]
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
	commitmentEntries := append([]OperationalExportArtifact(nil), entriesByType[ArtifactTypeCommitment]...)
	sort.Slice(commitmentEntries, func(left, right int) bool {
		return commitmentEntries[left].Sequence < commitmentEntries[right].Sequence
	})
	commitmentRecords := make([]operationalCommitmentRecord, 0, len(commitmentEntries))
	for _, entry := range commitmentEntries {
		if entry.Sequence <= 0 {
			return nil, fmt.Errorf("%w: operational commitment sequence is missing", constants.ErrEvidenceArtifactMalformed)
		}
		entryPath := i.sourceArtifactPath(entry.RelativePath)
		imported, err := NewCommitmentImporter(i.reader, i.trust, CommitmentImportBinding{Reference: entry.ArtifactID, Path: entryPath, ScopeID: i.scopeID, RunID: i.admission.GetRunId(), TransactionID: entry.TransactionID}, i.verifiedAt).Import(ctx)
		if err != nil {
			return nil, err
		}
		if len(imported) != 1 {
			return nil, fmt.Errorf("%w: operational commitment importer returned %d nodes", constants.ErrInvalidEvidenceGraph, len(imported))
		}
		attestation := &operatorv1.CommitmentAttestation{}
		body, err := i.reader.ReadFile(ctx, entryPath)
		if err != nil {
			return nil, fmt.Errorf("%w: read operational commitment for segment: %w", constants.ErrEvidenceImporterFailed, err)
		}
		if err := compliancev1.UnmarshalCanonical(body, attestation); err != nil {
			return nil, fmt.Errorf("%w: decode operational commitment for segment: %w", constants.ErrEvidenceArtifactMalformed, err)
		}
		nodes = append(nodes, imported[0])
		commitmentRecords = append(commitmentRecords, operationalCommitmentRecord{sequence: entry.Sequence, nodeID: imported[0].ArtifactID, attestation: attestation})
	}
	if err := validateOperationalCommitmentSegment(inventory, commitmentRecords); err != nil {
		return nil, err
	}
	chainNodesByTransaction, chainSegment, err := importOperationalAuditChainEntries(ctx, i, entriesByType[ArtifactTypeAuditChainEntry], inventory)
	if err != nil {
		return nil, err
	}
	nodes = append(nodes, chainNodesByTransaction...)

	for _, record := range commitmentRecords {
		receipt := receiptsByTransaction[record.attestation.GetTransactionId()]
		if receipt == nil {
			continue
		}
		receiptNode := receiptNodesByTransaction[record.attestation.GetTransactionId()]
		receiptReference, err := validateOperationalCommitmentReceiptLink(record.attestation, receipt, receiptNode.ArtifactID)
		if err != nil {
			return nil, err
		}
		if receiptReference != "" {
			for index := range nodes {
				if nodes[index].ArtifactID == record.nodeID {
					nodes[index].References = append(nodes[index].References, receiptReference)
					break
				}
			}
		}
	}
	for transactionID, receiptNode := range receiptNodesByTransaction {
		receipt := receiptsByTransaction[transactionID]
		for _, chainNode := range chainNodesByTransaction {
			if chainNode.TransactionID != transactionID {
				continue
			}
			if !operationalReceiptMatchesChainEntry(receipt, chainNode.CanonicalBytes) {
				continue
			}
			for index := range nodes {
				if nodes[index].ArtifactID == receiptNode.ArtifactID {
					nodes[index].References = append(nodes[index].References, chainNode.ArtifactID)
				}
				if nodes[index].ArtifactID == chainNode.ArtifactID {
					nodes[index].References = append(nodes[index].References, receiptNode.ArtifactID)
				}
			}
		}
	}
	if err := validateOperationalAuditChainInventory(inventory, chainSegment); err != nil {
		return nil, err
	}
	return nodes, nil
}

type operationalCommitmentRecord struct {
	sequence    int64
	nodeID      string
	attestation *operatorv1.CommitmentAttestation
}

func validateOperationalCommitmentSegment(inventory *OperationalSourceInventory, records []operationalCommitmentRecord) error {
	if len(records) == 0 {
		return nil
	}
	if inventory.CommitmentFirstSequence != records[0].sequence || inventory.CommitmentLastSequence != records[len(records)-1].sequence || inventory.CommitmentBoundaryPriorHash != records[0].attestation.GetPriorCommitmentHash() || inventory.CommitmentHeadHash != records[len(records)-1].attestation.GetHash() {
		return fmt.Errorf("%w: operational commitment segment bounds do not match its attestations", constants.ErrEvidenceScopeMismatch)
	}
	if !inventory.CommitmentSequenceContiguous || records[len(records)-1].sequence-records[0].sequence+1 != int64(len(records)) {
		return fmt.Errorf("%w: operational commitment segment sequence has a gap", constants.ErrInvalidEvidenceGraph)
	}
	for index := 1; index < len(records); index++ {
		if records[index].sequence != records[index-1].sequence+1 || records[index].attestation.GetPriorCommitmentHash() != records[index-1].attestation.GetHash() {
			return fmt.Errorf("%w: operational commitment segment predecessor link is invalid", constants.ErrInvalidEvidenceGraph)
		}
	}
	return nil
}

func validateOperationalCommitmentReceiptLink(attestation *operatorv1.CommitmentAttestation, receipt *operatorv1.ActionReceipt, receiptReference string) (string, error) {
	if attestation.GetTransactionHash() != receipt.GetTransactionHash() {
		return "", fmt.Errorf("%w: commitment %s transaction hash does not match retained receipt", constants.ErrEvidenceScopeMismatch, attestation.GetTransactionId())
	}
	for _, stage := range receipt.GetDeterministicStageEvidence() {
		if stage.GetKind() != operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND {
			continue
		}
		if stage.GetTransactionId() != attestation.GetTransactionId() || stage.GetTransactionHash() != attestation.GetTransactionHash() || stage.GetCommitmentHash() != attestation.GetHash() || stage.GetPriorCommitmentHash() != attestation.GetPriorCommitmentHash() || stage.GetSignerKeyId() != attestation.GetAuditorKeyId() || stage.GetL2SignatureDigest() != attestation.GetL2SignatureDigest() || stage.GetL3SignatureDigest() != attestation.GetHumanSignatureDigest() {
			return "", fmt.Errorf("%w: commitment %s does not match its retained commitment stage", constants.ErrEvidenceScopeMismatch, attestation.GetTransactionId())
		}
		return receiptReference, nil
	}
	return "", nil
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
		ArtifactID:         ContentAddress(ArtifactTypeProtocolChain, canonical),
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
		BundlePath:         receiptNode.BundlePath,
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

func importOperationalAuditChainEntries(ctx context.Context, importer *OperationalExportImporter, entries []OperationalExportArtifact, inventory *OperationalSourceInventory) ([]EvidenceNode, []storage.AuditChainSegmentEntry, error) {
	if len(entries) == 0 {
		return nil, nil, nil
	}
	segment := make([]storage.AuditChainSegmentEntry, 0, len(entries))
	nodes := make([]EvidenceNode, 0, len(entries))
	for _, entry := range entries {
		entryPath := importer.sourceArtifactPath(entry.RelativePath)
		body, err := importer.reader.ReadFile(ctx, entryPath)
		if err != nil {
			return nil, nil, err
		}
		if !matchesOperationalArtifact(entry, body) {
			return nil, nil, fmt.Errorf("%w: operational audit chain entry %s does not match its inventory digest", constants.ErrChecksumMismatch, entry.RelativePath)
		}
		chainEntry, err := storage.UnmarshalAuditChainSegmentEntry(body)
		if err != nil {
			return nil, nil, err
		}
		if chainEntry.Seq != entry.Sequence {
			return nil, nil, fmt.Errorf("%w: operational audit chain sequence does not match inventory", constants.ErrEvidenceScopeMismatch)
		}
		if entry.TransactionID != "" && chainEntry.TransactionID != entry.TransactionID {
			return nil, nil, fmt.Errorf("%w: operational audit chain transaction does not match inventory", constants.ErrEvidenceScopeMismatch)
		}
		segment = append(segment, chainEntry)
		producedAt, err := time.Parse(time.RFC3339Nano, chainEntry.Timestamp)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: parse audit chain timestamp: %w", constants.ErrEvidenceArtifactMalformed, err)
		}
		nodes = append(nodes, EvidenceNode{
			ArtifactID:         entry.ArtifactID,
			ArtifactType:       ArtifactTypeAuditChainEntry,
			SHA256:             entry.SHA256,
			MediaType:          constants.MediaTypeJSON,
			SchemaRef:          "g8e.storage.AuditChainSegmentEntry",
			ProducerIdentity:   chainEntry.OperatorSessionID,
			ProducedAt:         producedAt.UTC(),
			ScopeID:            importer.scopeID,
			RunID:              importer.admission.GetRunId(),
			TransactionID:      chainEntry.TransactionID,
			VerificationStatus: VerificationStatusVerified,
			VerifierID:         constants.AuditChainEvidenceVerifierID,
			VerifierVersion:    constants.AuditChainEvidenceVerifierVersion,
			VerifiedAt:         importer.verifiedAt,
			BundlePath:         entryPath,
			CanonicalBytes:     body,
		})
	}
	if err := storage.VerifyAuditChainSegment(segment, inventory.AuditChainBoundaryPriorHash, inventory.AuditChainHeadHash); err != nil {
		return nil, nil, fmt.Errorf("%w: %w", constants.ErrInvalidEvidenceGraph, err)
	}
	return nodes, segment, nil
}

func validateOperationalAuditChainInventory(inventory *OperationalSourceInventory, segment []storage.AuditChainSegmentEntry) error {
	if len(segment) == 0 {
		if inventory.AuditChainCount != 0 {
			return fmt.Errorf("%w: operational audit chain inventory is non-empty without artifacts", constants.ErrEvidenceArtifactMalformed)
		}
		return nil
	}
	sorted := append([]storage.AuditChainSegmentEntry(nil), segment...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Seq < sorted[j].Seq })
	if inventory.AuditChainFirstSeq != sorted[0].Seq || inventory.AuditChainLastSeq != sorted[len(sorted)-1].Seq {
		return fmt.Errorf("%w: operational audit chain segment bounds do not match inventory", constants.ErrEvidenceScopeMismatch)
	}
	if !inventory.AuditChainSequenceContiguous || sorted[len(sorted)-1].Seq-sorted[0].Seq+1 != int64(len(sorted)) {
		return fmt.Errorf("%w: operational audit chain segment sequence has a gap", constants.ErrInvalidEvidenceGraph)
	}
	for index := 1; index < len(sorted); index++ {
		if sorted[index].Seq != sorted[index-1].Seq+1 || sorted[index].PrevHash != sorted[index-1].Hash {
			return fmt.Errorf("%w: operational audit chain segment predecessor link is invalid", constants.ErrInvalidEvidenceGraph)
		}
	}
	return nil
}

func operationalReceiptMatchesChainEntry(receipt *operatorv1.ActionReceipt, chainBody []byte) bool {
	if receipt == nil {
		return false
	}
	chainEntry, err := storage.UnmarshalAuditChainSegmentEntry(chainBody)
	if err != nil {
		return false
	}
	if chainEntry.TransactionID != "" && chainEntry.TransactionID != receipt.GetTransactionId() {
		return false
	}
	equal, err := CanonicalProtoBodyEqual([]byte(chainEntry.ContentText), receipt)
	if err != nil {
		return false
	}
	return equal
}
