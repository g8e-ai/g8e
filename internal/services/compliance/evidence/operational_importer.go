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
	"sort"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type OperationalExportImporter struct {
	reader        ArtifactReader
	trust         AssessedSignerSource
	inventoryPath string
	scopeID       string
	runID         string
	verifiedAt    time.Time
	nowFunc       func() time.Time
}

func NewOperationalExportImporter(reader ArtifactReader, trust AssessedSignerSource, inventoryPath, scopeID, runID string, verifiedAt time.Time, nowFunc func() time.Time) *OperationalExportImporter {
	if nowFunc == nil {
		nowFunc = time.Now
	}
	return &OperationalExportImporter{reader: reader, trust: trust, inventoryPath: inventoryPath, scopeID: scopeID, runID: runID, verifiedAt: verifiedAt, nowFunc: nowFunc}
}

func (i *OperationalExportImporter) SourceID() string {
	return "operational-export"
}

func (i *OperationalExportImporter) Import(ctx context.Context) ([]EvidenceNode, error) {
	if i == nil || i.reader == nil || i.trust == nil || !ValidRelativePath(i.inventoryPath) || i.scopeID == "" || i.runID == "" || i.verifiedAt.IsZero() {
		return nil, fmt.Errorf("%w: operational export reader, trust, inventory, scope, run, and assessment time are required", constants.ErrInvalidEvidenceGraph)
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
	if inventory.SchemaVersion != operationalExportSchemaVersion || inventory.ScopeID != i.scopeID {
		return nil, fmt.Errorf("%w: operational inventory schema or scope does not match importer binding", constants.ErrEvidenceScopeMismatch)
	}
	artifacts := append([]OperationalExportArtifact(nil), inventory.Artifacts...)
	sort.Slice(artifacts, func(left, right int) bool { return artifacts[left].RelativePath < artifacts[right].RelativePath })
	entriesByType := make(map[ArtifactType][]OperationalExportArtifact)
	for _, artifact := range artifacts {
		if !ValidRelativePath(artifact.RelativePath) || artifact.SHA256 == "" || artifact.ArtifactID == "" {
			return nil, fmt.Errorf("%w: operational artifact inventory entry is incomplete", constants.ErrEvidenceArtifactMalformed)
		}
		body, err := i.reader.ReadFile(ctx, artifact.RelativePath)
		if err != nil {
			return nil, fmt.Errorf("%w: read operational artifact %s: %w", constants.ErrEvidenceImporterFailed, artifact.RelativePath, err)
		}
		if !matchesOperationalArtifact(artifact, body) {
			return nil, fmt.Errorf("%w: operational artifact %s does not match its inventory digest", constants.ErrChecksumMismatch, artifact.RelativePath)
		}
		entriesByType[ArtifactType(artifact.ArtifactType)] = append(entriesByType[ArtifactType(artifact.ArtifactType)], artifact)
	}

	receiptEntries := entriesByType[ArtifactTypeActionReceipt]
	receiptBindings := make(map[string]ReceiptImportBinding, len(receiptEntries))
	for _, entry := range receiptEntries {
		body, err := i.reader.ReadFile(ctx, entry.RelativePath)
		if err != nil {
			return nil, err
		}
		_, transactionID, err := receiptTransactionID(body)
		if err != nil {
			return nil, err
		}
		if _, exists := receiptBindings[transactionID]; exists {
			return nil, fmt.Errorf("%w: duplicate operational receipt transaction %s", constants.ErrEvidenceDuplicateID, transactionID)
		}
		receiptBindings[transactionID] = ReceiptImportBinding{Reference: entry.ArtifactID, Path: entry.RelativePath, ScopeID: i.scopeID, RunID: i.runID}
	}

	nodes := make([]EvidenceNode, 0, len(artifacts))
	for _, entry := range receiptEntries {
		binding := receiptBindingsForEntry(receiptBindings, entry, i.scopeID, i.runID)
		imported, err := NewReceiptImporterAt(i.reader, i.trust, binding, i.nowFunc).Import(ctx)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, imported...)
	}
	for _, entry := range entriesByType[ArtifactTypeReceiptPersistence] {
		body, err := i.reader.ReadFile(ctx, entry.RelativePath)
		if err != nil {
			return nil, err
		}
		transactionID, err := persistenceTransactionID(body)
		if err != nil {
			return nil, err
		}
		receiptBinding, exists := receiptBindings[transactionID]
		if !exists {
			return nil, fmt.Errorf("%w: persistence attestation %s has no exported receipt", constants.ErrUnresolvedReference, transactionID)
		}
		imported, err := NewPersistenceImporterAt(i.reader, i.trust, PersistenceImportBinding{Reference: entry.ArtifactID, Path: entry.RelativePath, ReceiptReference: receiptBinding.Reference, ReceiptPath: receiptBinding.Path, ScopeID: i.scopeID, RunID: i.runID}, i.nowFunc).Import(ctx)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, imported...)
	}
	for _, entry := range entriesByType[ArtifactTypeCommitment] {
		imported, err := NewCommitmentImporter(i.reader, i.trust, CommitmentImportBinding{Reference: entry.ArtifactID, Path: entry.RelativePath, ScopeID: i.scopeID, RunID: i.runID, TransactionID: entry.TransactionID}, i.verifiedAt).Import(ctx)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, imported...)
	}
	return nodes, nil
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

func receiptTransactionID(body []byte) (string, string, error) {
	receipt := &operatorv1.ActionReceipt{}
	if err := compliancev1.UnmarshalCanonical(body, receipt); err != nil || receipt.GetTransactionId() == "" {
		return "", "", fmt.Errorf("%w: operational receipt transaction binding is incomplete", constants.ErrEvidenceArtifactMalformed)
	}
	return receipt.GetTransactionId(), receipt.GetTransactionId(), nil
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
