// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { VSOBaseModel, F, now } from './base.js';

// ---------------------------------------------------------------------------
// SSE event models
//
// Every payload written to an SSE stream or published via sseService.publishEvent()
// must be an instance of one of these models, serialized via .forWire().
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// ConnectionEstablishedEvent
// ---------------------------------------------------------------------------

export class ConnectionEstablishedEvent extends VSOBaseModel {
    static fields = {
        type:         { type: F.string, required: true },
        connectionId: { type: F.string, required: true },
        timestamp:    { type: F.date,   default: () => now() },
    };
}

// ---------------------------------------------------------------------------
// KeepaliveEvent
// ---------------------------------------------------------------------------

export class KeepaliveEvent extends VSOBaseModel {
    static fields = {
        type:       { type: F.string, required: true },
        timestamp:  { type: F.date,   default: () => now() },
        serverTime: { type: F.number, default: null },
    };
}

// ---------------------------------------------------------------------------
// LLMConfigEvent
// ---------------------------------------------------------------------------

export class LLMConfigData extends VSOBaseModel {
    static fields = {
        provider:                { type: F.string, required: true },
        default_primary_model:   { type: F.string, default: '' },
        default_assistant_model: { type: F.string, default: '' },
        primary_models:          { type: F.array,  default: () => [] },
        assistant_models:        { type: F.array,  default: () => [] },
        timestamp:               { type: F.date,   default: () => now() },
    };
}

export class LLMConfigEvent extends VSOBaseModel {
    static fields = {
        type: { type: F.string,  required: true },
        data: { type: F.object,  model: LLMConfigData, default: null },
    };
}

// ---------------------------------------------------------------------------
// InvestigationListEvent
// ---------------------------------------------------------------------------

export class InvestigationListData extends VSOBaseModel {
    static fields = {
        investigations: { type: F.array,  default: () => [] },
        count:          { type: F.number, default: 0 },
        timestamp:      { type: F.date,   default: () => now() },
    };
}

export class InvestigationListEvent extends VSOBaseModel {
    static fields = {
        type: { type: F.string,  required: true },
        data: { type: F.object,  model: InvestigationListData, default: null },
    };
}

// ---------------------------------------------------------------------------
// HeartbeatSSEEvent  (broadcast to web session when VSE sends heartbeat)
// ---------------------------------------------------------------------------

export class HeartbeatSSEEvent extends VSOBaseModel {
    static fields = {
        type:        { type: F.string, required: true },
        data:        { type: F.any,    default: null },
        operator_id: { type: F.string, required: true },
        timestamp:   { type: F.date,   default: () => now() },
    };
}


// ---------------------------------------------------------------------------
// AuditDownloadResponse  (JSON export response body)
// ---------------------------------------------------------------------------

export class AuditDownloadResponse extends VSOBaseModel {
    static fields = {
        exported_at:          { type: F.date,   default: () => now() },
        user_id:              { type: F.string, required: true },
        total_events:         { type: F.number, default: 0 },
        total_investigations: { type: F.number, default: 0 },
        filters:              { type: F.object, default: () => ({}) },
        events:               { type: F.array,  default: () => [] },
    };
}

// ---------------------------------------------------------------------------
// OperatorStatusUpdatedEvent  (SSE payload for OPERATOR_STATUS_UPDATED_* events)
// ---------------------------------------------------------------------------

export class OperatorStatusUpdatedData extends VSOBaseModel {
    static fields = {
        operator_id:         { type: F.string, required: true },
        status:              { type: F.string, required: true },
        hostname:            { type: F.string, default: null },
        system_fingerprint:  { type: F.string, default: null },
        system_info:         { type: F.object, default: null },
        operator_data:       { type: F.any,    default: null },
        reason:              { type: F.string, default: null },
        total_count:         { type: F.number, default: null },
        active_count:        { type: F.number, default: null },
        timestamp:           { type: F.date,   default: null },
    };
}

export class OperatorStatusUpdatedEvent extends VSOBaseModel {
    static fields = {
        type:         { type: F.string, required: true },
        data:         { type: F.object, model: OperatorStatusUpdatedData, default: null },
        timestamp:    { type: F.date,   default: () => now() },
    };
}

// ---------------------------------------------------------------------------
// OperatorPanelListUpdatedEvent  (SSE payload for OPERATOR_PANEL_LIST_UPDATED)
// Carries operator context fields — not a status transition.
// ---------------------------------------------------------------------------

export class OperatorPanelListUpdatedData extends VSOBaseModel {
    static fields = {
        operator_id:      { type: F.string, required: true },
        case_id:          { type: F.string, default: null },
        investigation_id: { type: F.string, default: null },
        task_id:          { type: F.string, default: null },
        timestamp:        { type: F.date,   default: null },
    };
}

export class OperatorPanelListUpdatedEvent extends VSOBaseModel {
    static fields = {
        type:      { type: F.string, required: true },
        data:      { type: F.object, model: OperatorPanelListUpdatedData, default: null },
        timestamp: { type: F.date,   default: () => now() },
    };
}

// ---------------------------------------------------------------------------
// CommandResultSSEEvent  (operator.command.completed / operator.command.failed)
// Mirrors: components/vse/app/models/operators.py CommandResultBroadcastEvent
// Schema:  shared/models/wire/result_payloads.json execution_result
// ---------------------------------------------------------------------------

export class CommandResultSSEEvent extends VSOBaseModel {
    static fields = {
        type:                     { type: F.string,  required: true },
        execution_id:             { type: F.string,  required: true },
        command:                  { type: F.string,  default: null },
        status:                   { type: F.string,  required: true },
        output:                   { type: F.string,  default: null },
        error:                    { type: F.string,  default: null },
        stderr:                   { type: F.string,  default: null },
        exit_code:                { type: F.number,  default: null },
        return_code:              { type: F.number,  default: null },
        execution_time_seconds:   { type: F.number,  default: 0 },
        web_session_id:           { type: F.string,  default: null },
        operator_session_id:      { type: F.string,  default: null },
        operator_id:              { type: F.string,  default: null },
        hostname:                 { type: F.string,  default: null },
        case_id:                  { type: F.string,  default: null },
        investigation_id:         { type: F.string,  default: null },
        direct_execution:         { type: F.boolean, default: false },
        approval_id:              { type: F.string,  default: null },
        timestamp:                { type: F.date,    default: () => now() },
    };

    forWire() {
        const base = super.forWire();
        const { type, ...data } = base;
        return { type, data };
    }
}

// ---------------------------------------------------------------------------
// ApprovalResponseEvent  (response body for POST /api/operator/approval/respond)
// ---------------------------------------------------------------------------

export class ApprovalResponseEvent extends VSOBaseModel {
    static fields = {
        success:     { type: F.boolean, required: true },
        approval_id: { type: F.string,  required: true },
        approved:    { type: F.boolean, required: true },
        timestamp:   { type: F.date,    default: () => now() },
    };
}

// ---------------------------------------------------------------------------
// DirectCommandResponseEvent  (response body for POST /api/operator/direct-command)
// ---------------------------------------------------------------------------

export class DirectCommandResponseEvent extends VSOBaseModel {
    static fields = {
        success:      { type: F.boolean, required: true },
        execution_id: { type: F.string,  required: true },
        message:      { type: F.string,  default: 'Command sent to operator' },
        timestamp:    { type: F.date,    default: () => now() },
    };
}

// ---------------------------------------------------------------------------
// LogStreamEvent  (superadmin console log stream — console.log.entry)
// ---------------------------------------------------------------------------

export class LogStreamEvent extends VSOBaseModel {
    static fields = {
        type:  { type: F.string, required: true },
        entry: { type: F.any,    default: null },
    };
}

// ---------------------------------------------------------------------------
// LogStreamConnectedEvent  (superadmin console log stream — console.log.connected)
// ---------------------------------------------------------------------------

export class LogStreamConnectedEvent extends VSOBaseModel {
    static fields = {
        type:      { type: F.string, required: true },
        timestamp: { type: F.date,   default: () => now() },
        buffered:  { type: F.number, default: 0 },
    };
}


// ---------------------------------------------------------------------------
// VSEPassthroughEvent  (internal SSE push — raw VSE payloads forwarded as-is)
//
// VSE sends typed events over HTTP to /api/internal/sse/push.
// The payload is already wire-formatted by VSE.  This model wraps it so the
// boundary contract of SSEService.publishEvent (requires VSOBaseModel) is
// satisfied.  forWire() returns the inner payload directly, preserving the
// original wire shape.
//
// Schema enforcement: the wrapped payload MUST have a non-empty string `type`
// field.  A missing or non-string `type` means VSE sent a malformed event —
// forwarding it would produce an untyped SSE message the frontend cannot route.
// ---------------------------------------------------------------------------

export class VSEPassthroughEvent extends VSOBaseModel {
    static fields = {
        _payload: { type: F.any, required: true },
    };

    _validate() {
        if (!this._payload || typeof this._payload !== 'object') {
            throw new Error('VSEPassthroughEvent: _payload must be a plain object');
        }
        if (typeof this._payload.type !== 'string' || this._payload.type.trim() === '') {
            throw new Error(
                `VSEPassthroughEvent: _payload.type must be a non-empty string, ` +
                `got ${JSON.stringify(this._payload.type)}`
            );
        }
    }

    forWire() {
        return this._payload;
    }
}
