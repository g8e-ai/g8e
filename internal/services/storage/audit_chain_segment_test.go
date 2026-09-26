// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/timesvc"
)

func sampleAuditChainSegmentEntries(t *testing.T) []AuditChainSegmentEntry {
	t.Helper()
	timestamp := timesvc.FormatTimestamp(time.Unix(1_700_000_000, 0).UTC())
	digest, err := ComputeAuditEventContentDigest(&Event{ContentText: `{"transactionId":"tx-1"}`})
	require.NoError(t, err)
	first := AuditChainSegmentEntry{
		Seq:           1,
		PrevHash:      auditChainGenesisPrevHash,
		EventType:     string(constants.EventOperatorReceiptRecorded),
		Timestamp:     timestamp,
		ContentDigest: digest,
		TransactionID: "tx-1",
		ContentText:   `{"transactionId":"tx-1"}`,
	}
	first.Hash = AuditEventChainHash(first.Seq, first.PrevHash, first.EventType, "", first.Timestamp, first.ContentDigest, first.TransactionID)

	secondDigest, err := ComputeAuditEventContentDigest(&Event{ContentText: `{"transactionId":"tx-2"}`})
	require.NoError(t, err)
	second := AuditChainSegmentEntry{
		Seq:           2,
		PrevHash:      first.Hash,
		EventType:     string(constants.EventOperatorReceiptRecorded),
		Timestamp:     timestamp,
		ContentDigest: secondDigest,
		TransactionID: "tx-2",
		ContentText:   `{"transactionId":"tx-2"}`,
	}
	second.Hash = AuditEventChainHash(second.Seq, second.PrevHash, second.EventType, "", second.Timestamp, second.ContentDigest, second.TransactionID)
	return []AuditChainSegmentEntry{first, second}
}

func TestVerifyAuditChainSegment_AcceptsLinkedSegment(t *testing.T) {
	entries := sampleAuditChainSegmentEntries(t)
	require.NoError(t, VerifyAuditChainSegment(entries, auditChainGenesisPrevHash, entries[1].Hash))
}

func TestVerifyAuditChainSegment_RejectsTamperedHash(t *testing.T) {
	entries := sampleAuditChainSegmentEntries(t)
	entries[0].Hash = "tampered"
	err := VerifyAuditChainSegment(entries, auditChainGenesisPrevHash, entries[1].Hash)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hash mismatch")
}

func TestMarshalAuditChainSegmentEntry_RoundTrip(t *testing.T) {
	entries := sampleAuditChainSegmentEntries(t)
	body, err := MarshalAuditChainSegmentEntry(entries[0])
	require.NoError(t, err)
	decoded, err := UnmarshalAuditChainSegmentEntry(body)
	require.NoError(t, err)
	assert.Equal(t, entries[0], decoded)
}
