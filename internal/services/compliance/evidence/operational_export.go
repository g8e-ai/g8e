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
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

const operationalExportSchemaVersion = "operational-evidence-export@1.0.0"

type OperationalExportRequest struct {
	ScopeID              string
	OwnerRuntimeBoundary string
	AcquisitionBoundary  string
	WindowStart          time.Time
	WindowEnd            time.Time
	MaxRows              int
	OutputDir            string
}

type OperationalSourceInventory struct {
	SchemaVersion        string                      `json:"schema_version"`
	SourceKind           string                      `json:"source_kind"`
	SourceVersion        string                      `json:"source_version"`
	ScopeID              string                      `json:"scope_id"`
	OwnerRuntimeBoundary string                      `json:"owner_runtime_boundary"`
	AcquisitionBoundary  string                      `json:"acquisition_boundary"`
	WindowStartUTC       string                      `json:"window_start_utc"`
	WindowEndUTC         string                      `json:"window_end_utc"`
	ReceiptCount         int                         `json:"receipt_count"`
	CommitmentCount      int                         `json:"commitment_count"`
	PersistenceCount     int                         `json:"persistence_count"`
	Limitations          []string                    `json:"limitations,omitempty"`
	Artifacts            []OperationalExportArtifact `json:"artifacts"`
}

type OperationalExportArtifact struct {
	ArtifactType  string `json:"artifact_type"`
	ArtifactID    string `json:"artifact_id"`
	SHA256        string `json:"sha256"`
	TransactionID string `json:"transaction_id,omitempty"`
	ProducedAtUTC string `json:"produced_at_utc"`
	RelativePath  string `json:"relative_path"`
}

func ExportOperationalEvidence(ctx context.Context, snapshot *storage.OperationalEvidenceSnapshot, request OperationalExportRequest) (*OperationalSourceInventory, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if snapshot == nil || request.ScopeID == "" || request.OwnerRuntimeBoundary == "" || request.AcquisitionBoundary == "" || request.WindowStart.IsZero() || request.WindowEnd.IsZero() || request.WindowEnd.Before(request.WindowStart) || request.MaxRows <= 0 || request.OutputDir == "" {
		return nil, fmt.Errorf("%w: operational export request is incomplete", constants.ErrValidationFailed)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(snapshot.Receipts) > request.MaxRows || len(snapshot.Commitments) > request.MaxRows {
		return nil, fmt.Errorf("%w: operational export exceeds row bound %d", constants.ErrEvidenceArtifactTooLarge, request.MaxRows)
	}
	if err := os.MkdirAll(request.OutputDir, constants.PermDirPrivate); err != nil {
		return nil, fmt.Errorf("%w: create operational export directory: %w", constants.ErrDirCreateFailed, err)
	}
	inventory := &OperationalSourceInventory{
		SchemaVersion:        operationalExportSchemaVersion,
		SourceKind:           "operator-audit",
		SourceVersion:        "1.0.0",
		ScopeID:              request.ScopeID,
		OwnerRuntimeBoundary: request.OwnerRuntimeBoundary,
		AcquisitionBoundary:  request.AcquisitionBoundary,
		WindowStartUTC:       request.WindowStart.UTC().Format(time.RFC3339Nano),
		WindowEndUTC:         request.WindowEnd.UTC().Format(time.RFC3339Nano),
		ReceiptCount:         len(snapshot.Receipts),
		CommitmentCount:      len(snapshot.Commitments),
		Artifacts:            make([]OperationalExportArtifact, 0, len(snapshot.Receipts)+len(snapshot.Commitments)),
	}
	for _, receipt := range snapshot.Receipts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		_, artifact, err := writeOperationalArtifact(request.OutputDir, constants.ComplianceOperationalReceiptsDirname, ArtifactTypeActionReceipt, receipt.TransactionID, receipt.ExecutedAt, receipt.Body)
		if err != nil {
			return nil, err
		}
		inventory.Artifacts = append(inventory.Artifacts, artifact)
		if len(receipt.Body) == 0 {
			inventory.Limitations = append(inventory.Limitations, fmt.Sprintf("receipt %s has no canonical body", receipt.TransactionID))
			continue
		}
		receiptMessage := &operatorv1.ActionReceipt{}
		if err := compliancev1.UnmarshalCanonical(receipt.Body, receiptMessage); err != nil {
			inventory.Limitations = append(inventory.Limitations, fmt.Sprintf("receipt %s could not yield persistence evidence: %v", receipt.TransactionID, err))
			continue
		}
		attestation := receiptMessage.GetFinalPersistenceAttestation()
		if attestation == nil {
			inventory.Limitations = append(inventory.Limitations, fmt.Sprintf("receipt %s has no final persistence attestation", receipt.TransactionID))
			continue
		}
		body, err := compliancev1.MarshalCanonical(attestation)
		if err != nil {
			return nil, fmt.Errorf("operational export: canonicalize persistence attestation %s: %w", receipt.TransactionID, err)
		}
		_, persistenceArtifact, err := writeOperationalArtifact(request.OutputDir, constants.ComplianceOperationalPersistenceDirname, ArtifactTypeReceiptPersistence, receipt.TransactionID, time.UnixMilli(attestation.GetPersistedAtUnixMs()), body)
		if err != nil {
			return nil, err
		}
		inventory.PersistenceCount++
		inventory.Artifacts = append(inventory.Artifacts, persistenceArtifact)
	}
	for _, commitment := range snapshot.Commitments {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		_, artifact, err := writeOperationalArtifact(request.OutputDir, constants.ComplianceOperationalCommitmentsDirname, ArtifactTypeCommitment, commitment.TransactionID, commitment.CommittedAt, commitment.Body)
		if err != nil {
			return nil, err
		}
		inventory.Artifacts = append(inventory.Artifacts, artifact)
		if len(commitment.Body) == 0 {
			inventory.Limitations = append(inventory.Limitations, fmt.Sprintf("commitment %s has no canonical body", commitment.TransactionID))
		}
	}
	sort.Slice(inventory.Artifacts, func(i, j int) bool { return inventory.Artifacts[i].RelativePath < inventory.Artifacts[j].RelativePath })
	sort.Strings(inventory.Limitations)
	body, err := json.Marshal(inventory)
	if err != nil {
		return nil, fmt.Errorf("operational export: marshal inventory: %w", err)
	}
	inventoryPath := filepath.Join(request.OutputDir, constants.ComplianceOperationalInventoryFilename)
	if err := os.WriteFile(inventoryPath, body, constants.PermFilePrivate); err != nil {
		return nil, fmt.Errorf("%w: write operational inventory: %w", constants.ErrFileWriteFailed, err)
	}
	return inventory, nil
}

func writeOperationalArtifact(outputDir, directory string, artifactType ArtifactType, transactionID string, producedAt time.Time, body []byte) (string, OperationalExportArtifact, error) {
	digest := sha256.Sum256(body)
	digestHex := hex.EncodeToString(digest[:])
	relativePath := filepath.Join(directory, digestHex+constants.FileExtJSON)
	absolutePath := filepath.Join(outputDir, relativePath)
	if err := os.MkdirAll(filepath.Dir(absolutePath), constants.PermDirPrivate); err != nil {
		return "", OperationalExportArtifact{}, fmt.Errorf("%w: create operational artifact directory: %w", constants.ErrDirCreateFailed, err)
	}
	if err := os.WriteFile(absolutePath, body, constants.PermFilePrivate); err != nil {
		return "", OperationalExportArtifact{}, fmt.Errorf("%w: write operational artifact: %w", constants.ErrFileWriteFailed, err)
	}
	return relativePath, OperationalExportArtifact{
		ArtifactType:  string(artifactType),
		ArtifactID:    ContentAddress(artifactType, body),
		SHA256:        digestHex,
		TransactionID: transactionID,
		ProducedAtUTC: producedAt.UTC().Format(time.RFC3339Nano),
		RelativePath:  relativePath,
	}, nil
}
