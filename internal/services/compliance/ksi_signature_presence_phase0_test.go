// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliance

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/models"
)

// TestPhase0KSI_ReceiptSignatureMethodAcceptsNonemptyInvalidSignature
// documents that the current receiptsHaveSignatures method (bound to
// KSI-MLA-08) checks only that the Signature and SignerKeyID fields are
// nonempty strings. It does not verify the signature against a trusted key,
// does not check the signature algorithm, does not bind the signature to the
// receipt's transaction hash or state roots, and does not reject a garbage
// string that is structurally invalid as a signature.
//
// This is the baseline gap that Phase 3 closes by replacing the
// signature-presence method with protocol-owned cryptographic receipt and
// final-persistence verification. When the fix lands, this test is flipped to
// assert that an invalid signature produces a not-satisfied result.
func TestPhase0KSI_ReceiptSignatureMethodAcceptsNonemptyInvalidSignature(t *testing.T) {
	deps := EvaluatorDeps{
		Audit: &mockAuditReader{
			receipts: []*models.ActionReceiptRecord{
				{
					TransactionID:   "tx-bad-sig",
					Signature:       "not-a-real-signature-just-some-nonempty-garbage",
					SignerKeyID:     "key-1",
					TransactionHash: "hash-tx-bad",
				},
			},
		},
	}

	methods := DefaultMethods(deps)
	mla08Methods, ok := methods["KSI-MLA-08"]
	require.True(t, ok, "KSI-MLA-08 must have registered methods")

	// Find the receiptsHaveSignatures method. It is the first method bound
	// to KSI-MLA-08 in DefaultMethods.
	require.GreaterOrEqual(t, len(mla08Methods), 1,
		"KSI-MLA-08 must have at least one method to test the signature check")

	// Run all methods; the signature-presence method must return true for the
	// garbage signature because it only checks field non-emptiness.
	ctx := context.Background()
	atLeastOnePassedOnBadSig := false
	for _, m := range mla08Methods {
		satisfied, _, err := m(ctx)
		require.NoError(t, err)
		if satisfied {
			atLeastOnePassedOnBadSig = true
		}
	}

	assert.True(t, atLeastOnePassedOnBadSig,
		phase0RegressionBeforeFix+
			": the current receipt-signature KSI method accepts a nonempty but cryptographically "+
			"invalid signature string. It checks field presence, not signature validity. "+
			"After Phase 3, this assertion flips to False: an invalid signature must not satisfy the method.")
}

// TestPhase0KSI_ReceiptSignatureMethodDoesNotVerifyAgainstTrustedKey documents
// that the current method has no access to a trusted public key, no signature
// algorithm binding, and no receipt-hash binding. The method signature is
// func(ctx) (bool, []Evidence, error) with no key material parameter, so it
// cannot perform cryptographic verification even if it wanted to.
func TestPhase0KSI_ReceiptSignatureMethodDoesNotVerifyAgainstTrustedKey(t *testing.T) {
	// The EvaluatorDeps struct carries Audit, Ledger, and Commitments readers.
	// None of these expose a trusted public key or a signature-verification
	// primitive. The method closure captured by DefaultMethods has no key
	// material in scope.
	deps := EvaluatorDeps{
		Audit: &mockAuditReader{
			receipts: []*models.ActionReceiptRecord{
				{
					TransactionID:   "tx-no-key",
					Signature:       "deadbeef",
					SignerKeyID:     "orphan-key-id",
					TransactionHash: "hash-no-key",
				},
			},
		},
	}

	methods := DefaultMethods(deps)
	mla08Methods := methods["KSI-MLA-08"]
	require.NotEmpty(t, mla08Methods)

	ctx := context.Background()
	satisfied, evidence, err := mla08Methods[0](ctx)
	require.NoError(t, err)

	// The method returns satisfied=true with a description claiming "valid
	// signatures" even though no key was consulted.
	assert.True(t, satisfied,
		phase0RegressionBeforeFix+
			": method returns satisfied without any trusted-key verification")
	if len(evidence) > 0 {
		assert.Contains(t, evidence[0].Description, "valid signatures",
			phase0RegressionBeforeFix+
				": evidence description claims valid signatures without cryptographic verification")
	}
}
