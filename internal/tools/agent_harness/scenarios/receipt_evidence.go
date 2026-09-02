// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package scenarios

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	clientpkg "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

const (
	receiptEvidenceQueryTimeout  = 5 * time.Second
	receiptEvidenceQueryInterval = 100 * time.Millisecond
)

type ReceiptEvidence struct {
	RunID           string                    `json:"run_id"`
	ScenarioID      string                    `json:"scenario_id"`
	AttemptID       string                    `json:"attempt_id"`
	ExecutionID     string                    `json:"execution_id"`
	InvestigationID string                    `json:"investigation_id"`
	TransactionID   string                    `json:"transaction_id"`
	ReceiptRef      string                    `json:"receipt_ref"`
	PersistenceRef  string                    `json:"persistence_ref"`
	Receipt         *operatorv1.ActionReceipt `json:"receipt"`
}

func BuildReceiptEvidence(result *Result, projection clientpkg.Receipt, receipt *operatorv1.ActionReceipt) (*ReceiptEvidence, error) {
	return buildReceiptEvidence(result, projection, receipt)
}

func buildReceiptEvidence(result *Result, projection clientpkg.Receipt, receipt *operatorv1.ActionReceipt) (*ReceiptEvidence, error) {
	if result == nil || result.RunID == "" || result.ScenarioID == "" || len(result.AttemptIDs) != 1 ||
		!containsString(result.ExecutionIDs, projection.ExecutionID) || !containsString(result.TransactionIDs, projection.TransactionID) ||
		!containsString(result.InvestigationIDs, projection.InvestigationID) {
		return nil, fmt.Errorf("%w: receipt projection is not bound to one demo run, scenario, and attempt", constants.ErrInvalidEvidenceGraph)
	}
	if projection.ExecutionID == "" || projection.TransactionID == "" || projection.TransactionHash == "" || projection.InvestigationID == "" || projection.SignerKeyID == "" || projection.Signature == "" || receipt == nil {
		return nil, fmt.Errorf("%w: receipt projection or canonical receipt is incomplete", constants.ErrInvalidEvidenceGraph)
	}
	if receipt.GetTransactionId() != projection.TransactionID || receipt.GetTransactionHash() != projection.TransactionHash ||
		receipt.GetSignerKeyId() != projection.SignerKeyID || receipt.GetSignature() != projection.Signature {
		return nil, fmt.Errorf("%w: canonical receipt does not match the governed response projection", constants.ErrInvalidEvidenceGraph)
	}
	if !receiptHasInvestigationBinding(receipt, projection) {
		return nil, fmt.Errorf("%w: canonical receipt lacks the authoritative investigation binding", constants.ErrInvalidEvidenceGraph)
	}
	attestation := receipt.GetFinalPersistenceAttestation()
	if attestation == nil || attestation.GetTransactionId() != projection.TransactionID || attestation.GetAuditRecordId() != projection.TransactionID ||
		attestation.GetSignerKeyId() != projection.SignerKeyID || attestation.GetReceiptSignatureDigest() == "" || attestation.GetPersistedAtUnixMs() <= 0 || attestation.GetSignature() == "" {
		return nil, fmt.Errorf("%w: canonical receipt lacks a bound final-persistence attestation", constants.ErrInvalidEvidenceGraph)
	}
	receiptRef, err := contentAddress("action-receipt", receipt)
	if err != nil {
		return nil, err
	}
	persistenceRef, err := contentAddress("receipt-persistence", attestation)
	if err != nil {
		return nil, err
	}
	return &ReceiptEvidence{
		RunID: result.RunID, ScenarioID: result.ScenarioID, AttemptID: result.AttemptIDs[0], ExecutionID: projection.ExecutionID,
		InvestigationID: projection.InvestigationID, TransactionID: projection.TransactionID, ReceiptRef: receiptRef,
		PersistenceRef: persistenceRef, Receipt: receipt,
	}, nil
}

func ResolveReceiptEvidence(ctx context.Context, client *clientpkg.Client, result *Result) error {
	if result == nil || len(result.Receipts) == 0 {
		return nil
	}
	evidenceRecords := make([]ReceiptEvidence, 0, len(result.Receipts))
	for _, projection := range result.Receipts {
		receipt, err := waitForCanonicalReceipt(ctx, client, projection.TransactionID)
		if err != nil {
			return err
		}
		evidence, err := buildReceiptEvidence(result, projection, receipt)
		if err != nil {
			return err
		}
		evidenceRecords = append(evidenceRecords, *evidence)
	}
	result.ReceiptEvidence = append(result.ReceiptEvidence, evidenceRecords...)
	return nil
}

func waitForCanonicalReceipt(ctx context.Context, client *clientpkg.Client, transactionID string) (*operatorv1.ActionReceipt, error) {
	deadline := time.NewTimer(receiptEvidenceQueryTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(receiptEvidenceQueryInterval)
	defer ticker.Stop()
	for {
		receipt, _, err := client.GetActionReceipt(ctx, transactionID)
		if err != nil {
			return nil, fmt.Errorf("resolve canonical action receipt %s: %w", transactionID, err)
		}
		if receipt != nil && receipt.GetFinalPersistenceAttestation() != nil {
			return receipt, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, fmt.Errorf("%w: canonical receipt %s with final persistence was not available", constants.ErrInvalidEvidenceGraph, transactionID)
		case <-ticker.C:
		}
	}
}

func receiptHasInvestigationBinding(receipt *operatorv1.ActionReceipt, projection clientpkg.Receipt) bool {
	for _, stage := range receipt.GetDeterministicStageEvidence() {
		if stage.GetTransactionId() == projection.TransactionID && stage.GetTransactionHash() == projection.TransactionHash && stage.GetInvestigationId() == projection.InvestigationID {
			return true
		}
	}
	return false
}

func contentAddress(prefix string, message proto.Message) (string, error) {
	encoded, err := compliancev1.MarshalCanonical(message)
	if err != nil {
		return "", fmt.Errorf("%w: canonicalize %s evidence: %w", constants.ErrInvalidEvidenceGraph, prefix, err)
	}
	digest := sha256.Sum256(encoded)
	return prefix + ":sha256:" + hex.EncodeToString(digest[:]), nil
}

func containsString(values []string, target string) bool {
	if target == "" {
		return false
	}
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
