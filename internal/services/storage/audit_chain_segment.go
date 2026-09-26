// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package storage

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/timesvc"
)

// AuditChainSegmentEntry is one chained audit event exported for offline
// compliance verification without opening the source database.
type AuditChainSegmentEntry struct {
	Seq               int64
	PrevHash          string
	Hash              string
	EventType         string
	OperatorSessionID string
	Timestamp         string
	ContentDigest     string
	TransactionID     string
	ContentText       string
}

type auditChainSegmentExport struct {
	Seq               int64  `json:"seq"`
	PrevHash          string `json:"prev_hash"`
	Hash              string `json:"hash"`
	EventType         string `json:"event_type"`
	OperatorSessionID string `json:"operator_session_id,omitempty"`
	Timestamp         string `json:"timestamp"`
	ContentDigest     string `json:"content_digest"`
	TransactionID     string `json:"transaction_id,omitempty"`
	ContentText       string `json:"content_text,omitempty"`
}

// MarshalAuditChainSegmentEntry returns the canonical JSON body for one exported
// chain entry artifact.
func MarshalAuditChainSegmentEntry(entry AuditChainSegmentEntry) ([]byte, error) {
	body, err := json.Marshal(auditChainSegmentExport{
		Seq:               entry.Seq,
		PrevHash:          entry.PrevHash,
		Hash:              entry.Hash,
		EventType:         entry.EventType,
		OperatorSessionID: entry.OperatorSessionID,
		Timestamp:         entry.Timestamp,
		ContentDigest:     entry.ContentDigest,
		TransactionID:     entry.TransactionID,
		ContentText:       entry.ContentText,
	})
	if err != nil {
		return nil, fmt.Errorf("audit chain segment: marshal entry: %w", err)
	}
	return body, nil
}

// UnmarshalAuditChainSegmentEntry decodes one exported chain entry artifact.
func UnmarshalAuditChainSegmentEntry(body []byte) (AuditChainSegmentEntry, error) {
	wire := auditChainSegmentExport{}
	if err := json.Unmarshal(body, &wire); err != nil {
		return AuditChainSegmentEntry{}, fmt.Errorf("audit chain segment: decode entry: %w", err)
	}
	if wire.Seq <= 0 || wire.PrevHash == "" || wire.Hash == "" || wire.EventType == "" || wire.Timestamp == "" || wire.ContentDigest == "" {
		return AuditChainSegmentEntry{}, fmt.Errorf("%w: audit chain segment entry is incomplete", constants.ErrEvidenceArtifactMalformed)
	}
	return AuditChainSegmentEntry{
		Seq:               wire.Seq,
		PrevHash:          wire.PrevHash,
		Hash:              wire.Hash,
		EventType:         wire.EventType,
		OperatorSessionID: wire.OperatorSessionID,
		Timestamp:         wire.Timestamp,
		ContentDigest:     wire.ContentDigest,
		TransactionID:     wire.TransactionID,
		ContentText:       wire.ContentText,
	}, nil
}

// ComputeAuditEventContentDigest returns the plaintext content digest used by
// the audit chain hash for one event payload.
func ComputeAuditEventContentDigest(event *Event) (string, error) {
	return computeEventContentDigest(event)
}

// AuditEventChainHash returns the chained hash for one audit event row.
func AuditEventChainHash(seq int64, prevHash, eventType, sessionID, timestamp, contentDigest, transactionID string) string {
	return computeAuditEventHash(seq, prevHash, eventType, sessionID, timestamp, contentDigest, transactionID)
}

// VerifyAuditChainSegment recomputes linkage and hashes for one exported
// segment. boundaryPriorHash and headHash must match the first prev_hash and
// final hash when the inventory records segment bounds.
func VerifyAuditChainSegment(entries []AuditChainSegmentEntry, boundaryPriorHash, headHash string) error {
	if len(entries) == 0 {
		return nil
	}
	sorted := append([]AuditChainSegmentEntry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Seq < sorted[j].Seq })
	if boundaryPriorHash != "" && sorted[0].PrevHash != boundaryPriorHash {
		return fmt.Errorf("audit chain segment: boundary prior hash mismatch at seq %d", sorted[0].Seq)
	}
	if headHash != "" && sorted[len(sorted)-1].Hash != headHash {
		return fmt.Errorf("audit chain segment: head hash mismatch at seq %d", sorted[len(sorted)-1].Seq)
	}

	expectedPrevHash := sorted[0].PrevHash
	for index, entry := range sorted {
		if index > 0 && entry.Seq != sorted[index-1].Seq+1 {
			return fmt.Errorf("audit chain segment: sequence gap before seq %d", entry.Seq)
		}
		if entry.PrevHash != expectedPrevHash {
			return fmt.Errorf("audit chain segment: prev_hash mismatch at seq %d", entry.Seq)
		}
		recomputedDigest, err := ComputeAuditEventContentDigest(&Event{
			ContentText: entry.ContentText,
		})
		if err != nil {
			return fmt.Errorf("audit chain segment: content digest at seq %d: %w", entry.Seq, err)
		}
		if recomputedDigest != entry.ContentDigest {
			return fmt.Errorf("audit chain segment: content_digest mismatch at seq %d", entry.Seq)
		}
		if _, err := timesvc.ParseTimestamp(entry.Timestamp); err != nil {
			return fmt.Errorf("audit chain segment: timestamp at seq %d: %w", entry.Seq, err)
		}
		recomputedHash := computeAuditEventHash(
			entry.Seq,
			entry.PrevHash,
			entry.EventType,
			entry.OperatorSessionID,
			entry.Timestamp,
			entry.ContentDigest,
			entry.TransactionID,
		)
		if recomputedHash != entry.Hash {
			return fmt.Errorf("audit chain segment: hash mismatch at seq %d", entry.Seq)
		}
		expectedPrevHash = entry.Hash
	}
	return nil
}

// AuditChainSegmentEntryFromEvent maps a stored audit event into an export row.
func AuditChainSegmentEntryFromEvent(event *Event) (AuditChainSegmentEntry, error) {
	if event == nil || event.Seq <= 0 || event.PrevHash == "" || event.Hash == "" || event.ContentDigest == "" {
		return AuditChainSegmentEntry{}, fmt.Errorf("%w: chained audit event is incomplete", constants.ErrEvidenceArtifactMalformed)
	}
	return AuditChainSegmentEntry{
		Seq:               event.Seq,
		PrevHash:          event.PrevHash,
		Hash:              event.Hash,
		EventType:         string(event.Type),
		OperatorSessionID: event.OperatorSessionID,
		Timestamp:         timesvc.FormatTimestamp(event.Timestamp),
		ContentDigest:     event.ContentDigest,
		TransactionID:     event.TransactionID,
		ContentText:       event.ContentText,
	}, nil
}
