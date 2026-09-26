// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrAuditIngestInvalidRequest = errors.New("AUDIT_INGEST_INVALID: audit record request is invalid")
	ErrAuditIngestNoDelivery     = errors.New("AUDIT_INGEST_NO_DELIVERY: operator is not listening on audit channel")
	ErrAuditIngestTimeout        = errors.New("AUDIT_INGEST_TIMEOUT: operator did not acknowledge audit record")
)

// ValidateAuditRecordRequest ensures the event is a registered LFAA audit ingest request
// and returns the recorded fact event that must be persisted.
func ValidateAuditRecordRequest(event EventType) (EventType, error) {
	entry, ok := Registry.Lookup(event)
	if !ok {
		return "", ErrTxUnknownEventType
	}
	if entry.Kind != EventKindRequest {
		return "", fmt.Errorf("%w: %q is kind %q", ErrAuditIngestInvalidRequest, event, entry.Kind)
	}
	if !strings.Contains(string(event), ".operator.audit.") || !strings.HasSuffix(string(event), ".record.requested") {
		return "", fmt.Errorf("%w: %q", ErrAuditIngestInvalidRequest, event)
	}

	recorded, err := auditRecordedEventForRequest(event)
	if err != nil {
		return "", err
	}
	outcomeEntry, ok := Registry.Lookup(recorded)
	if !ok {
		return "", fmt.Errorf("%w: missing outcome %q", ErrAuditIngestInvalidRequest, recorded)
	}
	allowed := false
	for _, key := range entry.Outcomes {
		if outcomeEntry.Key == key {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", fmt.Errorf("%w: outcome %q not listed for request %q", ErrTxOutcomeNotAllowed, recorded, event)
	}
	return recorded, nil
}

func auditRecordedEventForRequest(event EventType) (EventType, error) {
	switch event {
	case EventOperatorAuditUserRecordRequested:
		return EventOperatorAuditUserRecorded, nil
	case EventOperatorAuditAiRecordRequested:
		return EventOperatorAuditAiRecorded, nil
	case EventOperatorAuditCommandRecordRequested:
		return EventOperatorAuditCommandRecorded, nil
	case EventOperatorAuditDirectCommandRecordRequested:
		return EventOperatorAuditDirectCommandRecorded, nil
	case EventOperatorAuditDirectCommandResultRecordRequested:
		return EventOperatorAuditDirectCommandResultRecorded, nil
	case EventOperatorAuditMcpCallRecordRequested:
		return EventOperatorAuditMcpCallRecorded, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrAuditIngestInvalidRequest, event)
	}
}
