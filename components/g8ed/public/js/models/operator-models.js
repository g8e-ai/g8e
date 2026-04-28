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

/**
 * Operator Domain Models for Frontend
 *
 * Browser-side models mirroring the server-side operator_model.js.
 * These models extend FrontendBaseModel and are used for data received
 * from the wire (SSE events, API responses) in the browser.
 */

import { FrontendBaseModel, F } from './base.js';

// ---------------------------------------------------------------------------
// HeartbeatSnapshot
// ---------------------------------------------------------------------------

// Canonical shape: shared/models/wire/heartbeat.json#operator_heartbeat.
// This is the same instance the backend persists as latest_heartbeat_snapshot
// AND the envelope payload in shared/models/wire/heartbeat_sse.json#envelope.metrics.
// Frontend parses, persists, and consumes exactly one shape — no flat projection.

export class HeartbeatSystemIdentity extends FrontendBaseModel {
    static fields = {
        hostname:     { type: F.string, default: null },
        os:           { type: F.string, default: null },
        architecture: { type: F.string, default: null },
        pwd:          { type: F.string, default: null },
        current_user: { type: F.string, default: null },
        cpu_count:    { type: F.number, default: null },
        memory_mb:    { type: F.number, default: null },
    };
}

export class HeartbeatPerformanceMetrics extends FrontendBaseModel {
    static fields = {
        cpu_percent:     { type: F.number, default: null },
        memory_percent:  { type: F.number, default: null },
        disk_percent:    { type: F.number, default: null },
        network_latency: { type: F.number, default: null },
        memory_used_mb:  { type: F.number, default: null },
        memory_total_mb: { type: F.number, default: null },
        disk_used_gb:    { type: F.number, default: null },
        disk_total_gb:   { type: F.number, default: null },
    };
}

export class HeartbeatNetworkInterface extends FrontendBaseModel {
    static fields = {
        name: { type: F.string, default: null },
        ip:   { type: F.string, default: null },
        mtu:  { type: F.number, default: null },
    };
}

export class HeartbeatNetworkInfo extends FrontendBaseModel {
    static fields = {
        public_ip:           { type: F.string, default: null },
        internal_ip:         { type: F.string, default: null },
        interfaces:          { type: F.any,    default: null },
        connectivity_status: { type: F.any,    default: null },
    };
}

export class HeartbeatUptimeInfo extends FrontendBaseModel {
    static fields = {
        uptime_display: { type: F.string, default: null },
        uptime_seconds: { type: F.number, default: null },
    };
}

export class HeartbeatVersionInfo extends FrontendBaseModel {
    static fields = {
        operator_version: { type: F.string, default: null },
        status:           { type: F.string, default: null },
    };
}

export class HeartbeatSnapshot extends FrontendBaseModel {
    static fields = {
        timestamp:             { type: F.date,   default: null },
        heartbeat_type:        { type: F.string, default: null },

        system_identity:       { type: F.object, model: HeartbeatSystemIdentity,     default: () => new HeartbeatSystemIdentity({}) },
        performance:           { type: F.object, model: HeartbeatPerformanceMetrics, default: () => new HeartbeatPerformanceMetrics({}) },
        network:               { type: F.object, model: HeartbeatNetworkInfo,        default: () => new HeartbeatNetworkInfo({}) },
        uptime:                { type: F.object, model: HeartbeatUptimeInfo,         default: () => new HeartbeatUptimeInfo({}) },
        version_info:          { type: F.object, model: HeartbeatVersionInfo,        default: () => new HeartbeatVersionInfo({}) },

        os_details:            { type: F.any, default: null },
        user_details:          { type: F.any, default: null },
        disk_details:          { type: F.any, default: null },
        memory_details:        { type: F.any, default: null },
        environment:           { type: F.any, default: null },

        system_fingerprint:    { type: F.string, default: null },
        fingerprint_details:   { type: F.any,    default: null },

        is_cloud_operator:     { type: F.boolean, default: false },
        cloud_provider:        { type: F.string,  default: null },

        local_storage_enabled: { type: F.boolean, default: false },
        git_available:         { type: F.boolean, default: false },
        ledger_enabled:        { type: F.boolean, default: false },
    };

    static empty() {
        return HeartbeatSnapshot.parse({});
    }
}

export class OperatorSlot extends FrontendBaseModel {
    static fields = {
        operator_id:    { type: F.string,  required: true },
        name:           { type: F.string,  default: null },
        status:         { type: F.string,  default: null },
        status_display: { type: F.string,  default: null },
        status_class:   { type: F.string,  default: 'inactive' },
        bound_web_session_id: { type: F.string, default: null },
        is_g8ep:        { type: F.boolean, default: false },
        first_deployed: { type: F.date,    default: null },
        claimed_at:     { type: F.date,    default: null },
        last_heartbeat: { type: F.date,    default: null },
        latest_heartbeat_snapshot: { type: F.object, default: null },
    };
}

// ---------------------------------------------------------------------------
// SSE payload models — mirror the INNER `data` body the browser receives.
//
// The SSE transport envelope `{ type, data }` is unwrapped by
// sse-connection-manager before these models ever see it (the manager emits
// `eventBus.emit(type, data.data)`). Therefore these models MUST NOT declare
// a top-level `type` or re-nested `data` field — doing so would re-introduce
// the old wrap/unwrap workaround in every handler.
// ---------------------------------------------------------------------------

export class OperatorListUpdatedEvent extends FrontendBaseModel {
    static fields = {
        operators:    { type: F.array,  items: OperatorSlot, default: () => [] },
        total_count:  { type: F.number, default: 0 },
        active_count: { type: F.number, default: 0 },
        used_slots:   { type: F.number, default: 0 },
        max_slots:    { type: F.number, default: 0 },
        timestamp:    { type: F.date,   default: null },
    };
}

export class OperatorStatusUpdatedEvent extends FrontendBaseModel {
    static fields = {
        operator_id:         { type: F.string, required: true },
        status:              { type: F.string, required: true },
        web_session_id:      { type: F.string, default: null },
        hostname:            { type: F.string, default: null },
        system_fingerprint:  { type: F.string, default: null },
        reason:              { type: F.string, default: null },
        total_count:         { type: F.number, default: null },
        active_count:        { type: F.number, default: null },
        timestamp:           { type: F.date,   default: null },
    };
}

// Canonical wire shape: shared/models/wire/heartbeat_sse.json#envelope.
// Producer: g8ee HeartbeatSSEEnvelope (components/g8ee/app/models/operators.py).
export class HeartbeatSSEEnvelope extends FrontendBaseModel {
    static fields = {
        operator_id: { type: F.string, required: true },
        status:      { type: F.string, required: true },
        metrics:     { type: F.object, model: HeartbeatSnapshot, default: () => HeartbeatSnapshot.empty() },
    };
}
