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
)

func TestKSIReceiptVerification_RejectsSignedFieldTampering(t *testing.T) {
	deps := fullDeps(t)
	record := deps.Audit.(*mockAuditReader).receipts[0]
	record.ActionReceipt.ResultSummary = "tampered-after-signing"

	methods := DefaultMethods(deps)
	mla08Methods, ok := methods["KSI-MLA-08"]
	require.True(t, ok)
	require.NotEmpty(t, mla08Methods)

	satisfied, _, err := mla08Methods[0](context.Background())
	require.NoError(t, err)
	assert.False(t, satisfied, phase0RegressionAfterFix+": signed-field tampering fails cryptographic KSI verification")
}

func TestKSIReceiptVerification_RejectsMissingFinalPersistenceAttestation(t *testing.T) {
	deps := fullDeps(t)
	record := deps.Audit.(*mockAuditReader).receipts[0]
	record.ActionReceipt.FinalPersistenceAttestation = nil

	methods := DefaultMethods(deps)
	mla08Methods := methods["KSI-MLA-08"]
	require.NotEmpty(t, mla08Methods)

	satisfied, evidence, err := mla08Methods[0](context.Background())
	require.NoError(t, err)
	assert.False(t, satisfied, phase0RegressionAfterFix+": missing final-persistence evidence fails cryptographic KSI verification")
	require.NotEmpty(t, evidence)
	assert.Equal(t, string(EvidenceTypeReceiptID), evidence[0].GetArtifactType())
}
