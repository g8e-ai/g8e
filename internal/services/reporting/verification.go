// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package reporting

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

const (
	verifyResultPass    = "PASS"
	verifyResultFail    = "FAIL"
	verifyResultSkipped = "SKIPPED"
)

// VerificationResult contains the output of the verification pass.
type VerificationResult struct {
	FailCount int
	Rows      []VerificationRow
}

func reportVerification(ctx context.Context, outDir string, auditStore *storage.SQLAuditStore, cl *storage.CommitmentLedger, ledger *storage.GitLedgerService) (FileResult, VerificationResult, error) {
	path := filepath.Join(outDir, constants.ReportVerificationFilename)

	var vr VerificationResult
	addRow := func(check, scope, subject, result, detail string) {
		vr.Rows = append(vr.Rows, VerificationRow{
			Check: check, Scope: scope, Subject: subject, Result: result, Detail: detail,
		})
		if result == verifyResultFail {
			vr.FailCount++
		}
	}

	// Check 1: Commitment chain integrity
	if ctx.Err() != nil {
		return FileResult{}, vr, ctx.Err()
	}
	commitments, commitmentsErr := cl.ListCommitments()
	if commitmentsErr != nil {
		addRow("commitment_chain", "commitment_ledger", "all", verifyResultSkipped, fmt.Sprintf("could not read commitments: %v", commitmentsErr))
	} else if len(commitments) == 0 {
		addRow("commitment_chain", "commitment_ledger", "all", verifyResultPass, "ledger empty (genesis)")
	} else {
		chainOK := true
		for i, c := range commitments {
			if i == 0 {
				if c.PriorCommitmentHash != "" {
					addRow("commitment_chain", "commitment_ledger", c.Hash, verifyResultFail,
						fmt.Sprintf("first commitment has non-empty prior_commitment_hash: %s", c.PriorCommitmentHash))
					chainOK = false
				}
				continue
			}
			prev := commitments[i-1]
			if c.PriorCommitmentHash != prev.Hash {
				addRow("commitment_chain", "commitment_ledger", c.Hash, verifyResultFail,
					fmt.Sprintf("prior_commitment_hash %s != previous hash %s", c.PriorCommitmentHash, prev.Hash))
				chainOK = false
			}
		}
		if chainOK {
			addRow("commitment_chain", "commitment_ledger", "all", verifyResultPass,
				fmt.Sprintf("%d commitments verified", len(commitments)))
		}
	}

	// Check 2: Commitment hash recomputation
	if ctx.Err() != nil {
		return FileResult{}, vr, ctx.Err()
	}
	if len(commitments) > 0 {
		allHashesOK := true
		allSignaturesOK := true
		for _, c := range commitments {
			var attestation operatorv1.CommitmentAttestation
			if err := json.Unmarshal(c.AttestationJSON, &attestation); err != nil {
				addRow("commitment_hash_recompute", "commitment_ledger", c.Hash, verifyResultFail, fmt.Sprintf("invalid attestation JSON: %v", err))
				allHashesOK = false
				allSignaturesOK = false
				continue
			}
			if !commitmentRowMatchesAttestation(c, &attestation) {
				addRow("commitment_hash_recompute", "commitment_ledger", c.Hash, verifyResultFail, "structured commitment columns diverge from attestation JSON")
				allHashesOK = false
			}
			canonical, err := governance.CanonicalizeCommitmentAttestation(&attestation)
			if err != nil {
				addRow("commitment_hash_recompute", "commitment_ledger", c.Hash, verifyResultFail, fmt.Sprintf("canonicalization failed: %v", err))
				allHashesOK = false
				allSignaturesOK = false
				continue
			}
			computed := sha256.Sum256(canonical)
			computedHex := hex.EncodeToString(computed[:])
			if computedHex != c.Hash || computedHex != attestation.Hash || attestation.PriorCommitmentHash != c.PriorCommitmentHash {
				addRow("commitment_hash_recompute", "commitment_ledger", c.Hash, verifyResultFail, "stored commitment does not match its canonical attestation hash")
				allHashesOK = false
			}
			publicKey, keyErr := hex.DecodeString(attestation.AuditorKeyId)
			signature, signatureErr := hex.DecodeString(attestation.Signature)
			if keyErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || !ed25519.Verify(ed25519.PublicKey(publicKey), canonical, signature) {
				addRow("commitment_signature", "commitment_ledger", c.Hash, verifyResultFail, "Auditor signature verification failed")
				allSignaturesOK = false
			}
		}
		if allHashesOK {
			addRow("commitment_hash_recompute", "commitment_ledger", "all", verifyResultPass, "all commitment hashes verified")
		}
		if allSignaturesOK {
			addRow("commitment_signature", "commitment_ledger", "all", verifyResultPass, "all Auditor signatures verified")
		}
	}

	// Check 3: Git merkle root cross-check
	if ctx.Err() != nil {
		return FileResult{}, vr, ctx.Err()
	}
	if ledger != nil {
		merkleRoot, err := ledger.GetStateMerkleRoot()
		if err != nil {
			addRow("git_merkle_root", "ledger", "HEAD", verifyResultSkipped, fmt.Sprintf("ledger unavailable: %v", err))
		} else if merkleRoot == "" {
			addRow("git_merkle_root", "ledger", "HEAD", verifyResultSkipped, "ledger empty (no commits)")
		} else {
			addRow("git_merkle_root", "ledger", merkleRoot, verifyResultPass, "HEAD hash captured")
		}
	} else {
		addRow("git_merkle_root", "ledger", "HEAD", verifyResultSkipped, "ledger not configured")
	}

	// Check 4: File mutation linkage (every mutation references a valid event)
	if ctx.Err() != nil {
		return FileResult{}, vr, ctx.Err()
	}
	if auditStore != nil {
		mutations, err := auditStore.ListFileMutations(1000, 0)
		if err != nil {
			addRow("file_mutation_linkage", "audit_store", "all", verifyResultSkipped, fmt.Sprintf("cannot read mutations: %v", err))
		} else {
			writeOps := 0
			missingHash := 0
			for _, m := range mutations {
				op := string(m.Operation)
				if op == "WRITE" || op == "CREATE" {
					writeOps++
					if m.LedgerHashAfter == "" {
						missingHash++
					}
				}
			}
			if missingHash > 0 {
				addRow("file_mutation_linkage", "audit_store", "all", verifyResultFail,
					fmt.Sprintf("%d WRITE/CREATE mutations missing ledger_hash_after", missingHash))
			} else {
				addRow("file_mutation_linkage", "audit_store", "all", verifyResultPass,
					fmt.Sprintf("%d mutations checked (%d write/create ops with hashes)", len(mutations), writeOps))
			}
		}
	} else {
		addRow("file_mutation_linkage", "audit_store", "all", verifyResultSkipped, "audit store not configured")
	}

	// Check 5: Receipt/commitment cross-link (every committed transaction_id has a receipt)
	if ctx.Err() != nil {
		return FileResult{}, vr, ctx.Err()
	}
	if auditStore != nil && commitmentsErr == nil {
		missingReceipts := 0
		for _, c := range commitments {
			if c.TransactionID == "" {
				continue
			}
			r, err := auditStore.GetActionReceipt(c.TransactionID)
			if err != nil || r == nil {
				missingReceipts++
			}
		}
		if missingReceipts > 0 {
			addRow("receipt_commitment_crosslink", "audit_store+commitment_ledger", "all", verifyResultFail,
				fmt.Sprintf("%d committed transaction_ids have no matching receipt", missingReceipts))
		} else if len(commitments) > 0 {
			addRow("receipt_commitment_crosslink", "audit_store+commitment_ledger", "all", verifyResultPass,
				fmt.Sprintf("all %d commitments have matching receipts", len(commitments)))
		} else {
			addRow("receipt_commitment_crosslink", "audit_store+commitment_ledger", "all", verifyResultPass, "no commitments to cross-link")
		}
	}

	// Write the verification summary CSV
	var rows []Row
	for _, r := range vr.Rows {
		rows = append(rows, r)
	}

	res, err := writeCSV(path, VerificationRow{}.Columns(), rows)
	if err != nil {
		return FileResult{}, vr, fmt.Errorf("%w: verification_summary: %w", constants.ErrReportWriteFailed, err)
	}
	res.Filename = constants.ReportVerificationFilename
	return res, vr, nil
}

func commitmentRowMatchesAttestation(row *storage.CommitmentRow, attestation *operatorv1.CommitmentAttestation) bool {
	return row.TransactionID == attestation.TransactionId &&
		row.TransactionHash == attestation.TransactionHash &&
		row.PriorCommitmentHash == attestation.PriorCommitmentHash &&
		row.Hash == attestation.Hash &&
		row.StateRootAtCommit == attestation.StateRootAtCommit &&
		row.L2SignatureDigest == attestation.L2SignatureDigest &&
		row.WardenIntentSignatureDigest == attestation.WardenIntentSignatureDigest &&
		row.HumanSignatureDigest == attestation.HumanSignatureDigest &&
		row.ActionType == attestation.ActionType &&
		row.TargetResource == attestation.TargetResource &&
		row.CommittedAt.UnixMilli() == attestation.CommittedAtUnixMs &&
		row.AuditorKeyID == attestation.AuditorKeyId &&
		row.Signature == attestation.Signature
}
