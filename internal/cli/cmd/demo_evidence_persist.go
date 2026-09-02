// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/scenarios"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

// persistDemoScenarioResult writes a typed DemoScenarioResult as canonical
// protojson to the per-run demo evidence tree under the runtime compliance
// directory. The path is data/compliance/demo-evidence/<run-id>/scenario-results.jsonl.
// Each result is appended as a single JSON line so multiple scenario results
// from the same run accumulate in one file without overwriting prior runs.
//
// Only evidence-grade results (those carrying a RunId and ScenarioRef) are
// persisted. Minimal display-only results from scenarios without typed
// evidence fields are skipped silently — they remain in the terminal summary
// table but do not produce canonical evidence until their full typed slice
// is implemented.
func persistDemoScenarioResult(ctx context.Context, fileSvc fs.RuntimeFileService, result *compliancev1.DemoScenarioResult) error {
	if result == nil || result.RunId == "" || result.ScenarioRef == nil {
		return nil
	}

	runDir := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.DemoEvidenceDirname, result.RunId)
	if err := fileSvc.MkdirAll(ctx, runDir, constants.PermDirStandard); err != nil {
		return fmt.Errorf("%w: create demo evidence run dir: %w", constants.ErrDemoEvidencePersistFailed, err)
	}

	encoded, err := compliancev1.MarshalCanonical(result)
	if err != nil {
		return fmt.Errorf("%w: marshal demo scenario result: %w", constants.ErrDemoEvidencePersistFailed, err)
	}

	relPath := filepath.Join(runDir, constants.DemoRunResultsFilename)
	existing, readErr := fileSvc.ReadFile(ctx, relPath)
	if readErr != nil && !errors.Is(readErr, constants.ErrNotFound) {
		return fmt.Errorf("%w: read existing demo results: %w", constants.ErrDemoEvidencePersistFailed, readErr)
	}

	var data []byte
	if len(existing) > 0 {
		data = append(existing, '\n')
	}
	data = append(data, encoded...)

	if err := fileSvc.WriteFile(ctx, relPath, data, constants.PermFileReadOnly); err != nil {
		return fmt.Errorf("%w: write demo scenario result: %w", constants.ErrDemoEvidencePersistFailed, err)
	}

	return nil
}

// persistReceiptEvidenceBodies writes the canonical ActionReceipt body and its
// final-persistence attestation body as resolvable runtime evidence artifacts
// under the per-run demo evidence tree. Each body is persisted as canonical
// protojson named by its SHA-256 content-address digest so the ReceiptRefs
// carried on DemoScenarioResult resolve to concrete artifacts rather than
// dangling references.
//
// Paths are data/compliance/demo-evidence/<run-id>/receipts/<hex>.json and
// data/compliance/demo-evidence/<run-id>/persistence/<hex>.json. The function
// fails closed: a nil receipt body or missing persistence attestation is
// rejected rather than persisting an incomplete artifact.
func persistReceiptEvidenceBodies(ctx context.Context, fileSvc fs.RuntimeFileService, runID string, evidence []scenarios.ReceiptEvidence) error {
	if runID == "" || len(evidence) == 0 {
		return nil
	}

	runDir := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.DemoEvidenceDirname, runID)
	receiptsDir := filepath.Join(runDir, constants.DemoRunReceiptsDirname)
	persistenceDir := filepath.Join(runDir, constants.DemoRunPersistenceDirname)
	if err := fileSvc.MkdirAll(ctx, receiptsDir, constants.PermDirStandard); err != nil {
		return fmt.Errorf("%w: create demo evidence receipts dir: %w", constants.ErrDemoEvidencePersistFailed, err)
	}
	if err := fileSvc.MkdirAll(ctx, persistenceDir, constants.PermDirStandard); err != nil {
		return fmt.Errorf("%w: create demo evidence persistence dir: %w", constants.ErrDemoEvidencePersistFailed, err)
	}

	for _, ev := range evidence {
		if ev.Receipt == nil {
			return fmt.Errorf("%w: receipt evidence for transaction %s has nil receipt body", constants.ErrDemoEvidencePersistFailed, ev.TransactionID)
		}
		attestation := ev.Receipt.GetFinalPersistenceAttestation()
		if attestation == nil {
			return fmt.Errorf("%w: receipt evidence for transaction %s has nil persistence attestation", constants.ErrDemoEvidencePersistFailed, ev.TransactionID)
		}

		receiptHex := contentAddressDigestHex(ev.ReceiptRef)
		persistenceHex := contentAddressDigestHex(ev.PersistenceRef)
		if receiptHex == "" || persistenceHex == "" {
			return fmt.Errorf("%w: receipt evidence for transaction %s has malformed content address", constants.ErrDemoEvidencePersistFailed, ev.TransactionID)
		}

		receiptBytes, err := compliancev1.MarshalCanonical(ev.Receipt)
		if err != nil {
			return fmt.Errorf("%w: marshal receipt body for transaction %s: %w", constants.ErrDemoEvidencePersistFailed, ev.TransactionID, err)
		}
		persistenceBytes, err := compliancev1.MarshalCanonical(attestation)
		if err != nil {
			return fmt.Errorf("%w: marshal persistence attestation for transaction %s: %w", constants.ErrDemoEvidencePersistFailed, ev.TransactionID, err)
		}

		receiptPath := filepath.Join(receiptsDir, receiptHex+".json")
		if err := fileSvc.WriteFile(ctx, receiptPath, receiptBytes, constants.PermFileReadOnly); err != nil {
			return fmt.Errorf("%w: write receipt body for transaction %s: %w", constants.ErrDemoEvidencePersistFailed, ev.TransactionID, err)
		}
		persistencePath := filepath.Join(persistenceDir, persistenceHex+".json")
		if err := fileSvc.WriteFile(ctx, persistencePath, persistenceBytes, constants.PermFileReadOnly); err != nil {
			return fmt.Errorf("%w: write persistence attestation for transaction %s: %w", constants.ErrDemoEvidencePersistFailed, ev.TransactionID, err)
		}
	}

	return nil
}

// contentAddressDigestHex extracts the hex digest from a content address of the
// form "prefix:sha256:<hex>". It returns an empty string if the address is
// malformed.
func contentAddressDigestHex(contentAddress string) string {
	parts := strings.SplitN(contentAddress, ":", 3)
	if len(parts) != 3 {
		return ""
	}
	return parts[2]
}
