// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// heartbeatUpdate is the telemetry a heartbeat records for an Operator. It is
// stored as one observed-tier kv_store row per Operator, not in the Operator
// document, so heartbeat traffic never changes the bound state root.
type heartbeatUpdate struct {
	LatestHeartbeatSnapshot json.RawMessage `json:"latest_heartbeat_snapshot"`
	LastHeartbeatAt         time.Time       `json:"last_heartbeat_at"`
	CurrentHostname         string          `json:"current_hostname,omitempty"`
}

func operatorHeartbeatKey(operatorID string) string {
	return constants.OperatorHeartbeatKeyPrefix + operatorID
}

// RecordOperatorHeartbeat stores an Operator's latest heartbeat telemetry in the
// observed tier. Reads overlay it onto the Operator document.
func (s *DocumentStoreService) RecordOperatorHeartbeat(operatorID string, update heartbeatUpdate) error {
	b, err := json.Marshal(update)
	if err != nil {
		return fmt.Errorf("gateway: document store: encode operator heartbeat: %w", err)
	}
	return s.kv.KVSetObserved(operatorHeartbeatKey(operatorID), string(b), 0)
}

// overlayOperatorHeartbeats merges each Operator document's observed heartbeat
// telemetry into its data, so every reader sees the same document shape as
// before telemetry left the bound document. It is a no-op for every other
// collection.
func (s *DocumentStoreService) overlayOperatorHeartbeats(docs ...*models.Document) {
	if len(docs) == 0 || docs[0].Collection != marshaler.CollectionName(constants.CollectionOperators) {
		return
	}
	// A registry read overlays every Operator, so read all telemetry rows in
	// one query rather than one lookup per document.
	var all map[string]string
	if len(docs) > 1 {
		var err error
		if all, err = s.kv.KVEntries(constants.OperatorHeartbeatKeyPrefix + "*"); err != nil {
			s.logger.Warn("Operator heartbeat telemetry unreadable; ignored", "error", err)
			return
		}
	}
	for _, doc := range docs {
		raw, ok := all[operatorHeartbeatKey(doc.ID)]
		if len(docs) == 1 {
			raw, ok = s.kv.KVGet(operatorHeartbeatKey(doc.ID))
		}
		if !ok {
			continue
		}
		var telemetry map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &telemetry); err != nil {
			s.logger.Warn("Operator heartbeat telemetry undecodable; ignored", "operator_id", doc.ID, "error", err)
			continue
		}
		for k, v := range telemetry {
			doc.Data[k] = v
		}
	}
}
