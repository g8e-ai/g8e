// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// validAgentLifecycleStatuses is the set of recognized agent lifecycle
// statuses. A first write accepts only a member of this set; "no existing
// projection" relaxes the source-state requirement, not target enum
// validation.
var validAgentLifecycleStatuses = map[models.AgentLifecycleStatus]struct{}{
	models.AgentLifecycleStatusIdle:      {},
	models.AgentLifecycleStatusQueued:    {},
	models.AgentLifecycleStatusRunning:   {},
	models.AgentLifecycleStatusWaiting:   {},
	models.AgentLifecycleStatusCompleted: {},
	models.AgentLifecycleStatusFailed:    {},
	models.AgentLifecycleStatusOffline:   {},
}

// validRunLifecycleStatuses is the set of recognized run lifecycle statuses.
var validRunLifecycleStatuses = map[models.RunLifecycleStatus]struct{}{
	models.RunLifecycleStatusQueued:    {},
	models.RunLifecycleStatusRunning:   {},
	models.RunLifecycleStatusWaiting:   {},
	models.RunLifecycleStatusCompleted: {},
	models.RunLifecycleStatusFailed:    {},
	models.RunLifecycleStatusCancelled: {},
}

// validRunKinds is the set of recognized run kinds.
var validRunKinds = map[models.RunKind]struct{}{
	models.RunKindInvestigation: {},
	models.RunKindEval:          {},
	models.RunKindDemo:          {},
	models.RunKindWorkflow:      {},
}

// supportedProducerSchemaVersion is the single schema version accepted at the
// producer boundary. Both agent and run producer requests carry the observe
// event payload schema version.
const supportedProducerSchemaVersion = constants.ObserveEventPayloadSchemaVersion

// validateAgentProducerRequest validates the agent producer payload fields at
// the Gateway boundary. It checks the supported schema version, non-empty
// display name and role, and a recognized agent status even on first write.
// Transition validation is separate and performed by the producer service
// against the persisted projection.
func validateAgentProducerRequest(req models.ObserveProducerAgentStateRequest) error {
	if req.SchemaVersion != supportedProducerSchemaVersion {
		return fmt.Errorf("observe producer: validate agent request: %w: got %q", constants.ErrObserveUnsupportedSchemaVersion, req.SchemaVersion)
	}
	if req.DisplayName == "" {
		return fmt.Errorf("observe producer: validate agent request: %w", constants.ErrObserveAgentDisplayNameRequired)
	}
	if req.Role == "" {
		return fmt.Errorf("observe producer: validate agent request: %w", constants.ErrObserveAgentRoleRequired)
	}
	if _, ok := validAgentLifecycleStatuses[req.Status]; !ok {
		return fmt.Errorf("observe producer: validate agent request: %w: %q", constants.ErrObserveInvalidTransition, req.Status)
	}
	return nil
}

// validateRunProducerRequest validates the run producer payload fields at the
// Gateway boundary. It checks the supported schema version, non-empty display
// name, recognized run kind, recognized run status even on first write,
// non-negative task counters, completed tasks not exceeding total tasks, and
// end time not preceding start time. Transition validation is separate and
// performed by the producer service against the persisted projection.
func validateRunProducerRequest(req models.ObserveProducerRunStateRequest) error {
	if req.SchemaVersion != supportedProducerSchemaVersion {
		return fmt.Errorf("observe producer: validate run request: %w: got %q", constants.ErrObserveUnsupportedSchemaVersion, req.SchemaVersion)
	}
	if req.DisplayName == "" {
		return fmt.Errorf("observe producer: validate run request: %w", constants.ErrObserveRunDisplayNameRequired)
	}
	if _, ok := validRunKinds[req.RunKind]; !ok {
		return fmt.Errorf("observe producer: validate run request: %w: %q", constants.ErrObserveRunKindRequired, req.RunKind)
	}
	if _, ok := validRunLifecycleStatuses[req.Status]; !ok {
		return fmt.Errorf("observe producer: validate run request: %w: %q", constants.ErrObserveInvalidTransition, req.Status)
	}
	if req.CompletedTasks < 0 || req.TotalTasks < 0 {
		return fmt.Errorf("observe producer: validate run request: %w: completed=%d total=%d", constants.ErrObserveNegativeTaskCount, req.CompletedTasks, req.TotalTasks)
	}
	if req.CompletedTasks > req.TotalTasks {
		return fmt.Errorf("observe producer: validate run request: %w: completed=%d total=%d", constants.ErrObserveCompletedExceedsTotal, req.CompletedTasks, req.TotalTasks)
	}
	if req.StartedAt != nil && req.EndedAt != nil && req.EndedAt.Before(*req.StartedAt) {
		return fmt.Errorf("observe producer: validate run request: %w: started=%s ended=%s", constants.ErrObserveEndBeforeStart, req.StartedAt.Format(time.RFC3339Nano), req.EndedAt.Format(time.RFC3339Nano))
	}
	return nil
}

// decodeProducerRequest decodes a producer request body with strict JSON
// semantics: unknown fields and trailing JSON after the top-level value are
// rejected. It returns a wrapped constants.ErrInvalidJSONBody so the
// controller maps the failure to a single 400 response without hand-rolled
// error text.
func decodeProducerRequest(body []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("observe producer: decode request: %w", constants.ErrInvalidJSONBody)
	}
	// Reject trailing JSON after the top-level value.
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("observe producer: decode request: %w: trailing content", constants.ErrInvalidJSONBody)
	}
	return nil
}
