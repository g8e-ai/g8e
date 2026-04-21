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
 * Operator Domain Models for g8ed
 *
 * Aligned with:
 * - components/g8eo/models/ (Go structs)
 * - components/g8ee/app/models/ (Python/Pydantic)
 *
 * Construction from untrusted data (DB read, wire payload):
 *   OperatorDocument.parse(raw)   — validates, coerces, strips unknown fields
 *
 * Internal construction (already-typed data from within the app):
 *   new OperatorDocument({ operator_id, user_id, ... })
 *   OperatorDocument.forCreate(data) / forSlot(data) / forRefresh(data) / forReset(data)
 */

import {
    OperatorStatus,
    OperatorType,
    CloudOperatorSubtype,
    HistoryEventType,
} from '../constants/operator.js';
import { SourceComponent } from '../constants/ai.js';
import { INTENT_TTL_SECONDS } from '../constants/auth.js';
import { G8eBaseModel, G8eIdentifiableModel, F, now, addSeconds } from './base.js';

// ---------------------------------------------------------------------------
// GrantedIntent
// ---------------------------------------------------------------------------

export class GrantedIntent extends G8eBaseModel {
    static fields = {
        name:       { type: F.string, required: true },
        granted_at: { type: F.date,   required: true },
        expires_at: { type: F.date,   required: true },
    };

    isActive() {
        return this.expires_at instanceof Date && this.expires_at > now();
    }

    static from(data) {
        if (data instanceof GrantedIntent) return data;
        return GrantedIntent.parse(data);
    }

    static create(name) {
        const ts = now();
        return new GrantedIntent({
            name,
            granted_at: ts,
            expires_at: addSeconds(ts, INTENT_TTL_SECONDS),
        });
    }
}

// ---------------------------------------------------------------------------
// HeartbeatNotification
// ---------------------------------------------------------------------------

export class HeartbeatNotification extends G8eBaseModel {
    static fields = {
        heartbeat_data:   { type: F.any, default: null },
        investigation_id: { type: F.string, default: null },
        case_id:          { type: F.string, default: null },
    };

    constructor(data = {}) {
        const hb = 'heartbeat_data' in data ? data.heartbeat_data : data;
        super({
            heartbeat_data:   hb,
            investigation_id: data.investigation_id ?? hb?.investigation_id ?? null,
            case_id:          data.case_id ?? hb?.case_id ?? null,
        });
    }

    static parse(raw = {}) {
        const hb = raw.heartbeat_data || raw;
        return new HeartbeatNotification({
            heartbeat_data:   hb,
            investigation_id: raw.investigation_id ?? hb.investigation_id ?? null,
            case_id:          raw.case_id ?? hb.case_id ?? null,
        });
    }

    static from(data) {
        if (data instanceof HeartbeatNotification) return data;
        return HeartbeatNotification.parse(data);
    }
}

// ---------------------------------------------------------------------------
// SystemInfo
// ---------------------------------------------------------------------------

export class SystemInfo extends G8eBaseModel {
    static fields = {
        hostname:               { type: F.string,  default: null },
        os:                     { type: F.string,  default: null },
        architecture:           { type: F.string,  default: null },
        cpu_count:              { type: F.any,     default: null },
        memory_mb:              { type: F.any,     default: null },
        public_ip:              { type: F.string,  default: null },
        internal_ip:            { type: F.string,  default: null },
        interfaces:             { type: F.array,   default: () => [] },
        current_user:           { type: F.string,  default: null },
        cloud_provider:         { type: F.string,  default: null },
        is_cloud_operator:      { type: F.boolean, default: false },
        system_fingerprint:     { type: F.string,  default: null },
        fingerprint_details:    { type: F.any,     default: null },
        os_details:             { type: F.any,     default: null },
        user_details:           { type: F.any,     default: null },
        disk_details:           { type: F.any,     default: null },
        memory_details:         { type: F.any,     default: null },
        environment:            { type: F.any,     default: null },
        local_storage_enabled:  { type: F.boolean, default: true },
    };

    static forCloudOperator(subtype) {
        if (!subtype) {
            throw new Error('SystemInfo.forCloudOperator() requires an explicit cloud subtype');
        }
        return SystemInfo.parse({
            cloud_provider:    subtype,
            is_cloud_operator: true,
        });
    }
}

// ---------------------------------------------------------------------------
// HistoryEntry
// ---------------------------------------------------------------------------

export class HistoryEntry extends G8eBaseModel {
    static fields = {
        timestamp:  { type: F.date,   default: () => now() },
        event_type: { type: F.string, required: true },
        summary:    { type: F.string, required: true },
        actor:      { type: F.string, default: SourceComponent.G8ED },
        details:    { type: F.object, default: () => ({}) },
    };
}

// ---------------------------------------------------------------------------
// CertInfo
// ---------------------------------------------------------------------------

export class CertInfo extends G8eBaseModel {
    static fields = {
        cert:       { type: F.string, default: null },
        key:        { type: F.string, default: null },
        serial:     { type: F.string, default: null },
        not_before: { type: F.any,    default: null },
        not_after:  { type: F.any,    default: null },
    };

    static empty() {
        return CertInfo.parse({});
    }

    static fromCertData(certData) {
        return CertInfo.parse({
            cert:       certData.cert,
            key:        certData.key,
            serial:     certData.serial,
            not_before: certData.notBefore,
            not_after:  certData.notAfter,
        });
    }
}

// ---------------------------------------------------------------------------
// OperatorStatusInfo  (read-only projection returned by getOperatorStatus)
// ---------------------------------------------------------------------------

export class OperatorStatusInfo extends G8eBaseModel {
    static fields = {
        operator_id:               { type: F.string, required: true },
        user_id:                   { type: F.string, required: true },
        status:                    { type: F.string, required: true },
        bound_web_session_id:      { type: F.string, default: null },
        operator_session_id:       { type: F.string, default: null },
        last_heartbeat:            { type: F.date,   default: null },
        system_info:               { type: F.object, model: SystemInfo,        default: () => new SystemInfo({}) },
        investigation_id:          { type: F.string, default: null },
        case_id:                   { type: F.string, default: null },
        is_active:                 { type: F.boolean, default: false },
        operator_type:             { type: F.string, default: null },
        granted_intents:           { type: F.array,  default: () => [] },
        cloud_subtype:             { type: F.string, default: null },
        current_hostname:          { type: F.string, default: null },
        session_token:             { type: F.string, default: null },
        session_expires_at:        { type: F.date,   default: null },
    };

    static fromOperator(operator) {
        return new OperatorStatusInfo({
            operator_id:               operator.id,
            user_id:                   operator.user_id,
            status:                    operator.status,
            bound_web_session_id:      operator.bound_web_session_id ?? null,
            operator_session_id:       operator.operator_session_id ?? null,
            last_heartbeat:            operator.last_heartbeat ?? null,
            system_info:               operator.system_info instanceof SystemInfo
                                           ? operator.system_info
                                           : new SystemInfo(operator.system_info || {}),
            investigation_id:          operator.investigation_id ?? null,
            case_id:                   operator.case_id ?? null,
            is_active:                 operator.status === OperatorStatus.ACTIVE,
            operator_type:             operator.operator_type ?? null,
            granted_intents:           Array.isArray(operator.granted_intents) ? operator.granted_intents : [],
            cloud_subtype:             operator.cloud_subtype ?? null,
            current_hostname:          operator.system_info?.hostname ?? null,
            session_token:             operator.session_token ?? null,
            session_expires_at:        operator.session_expires_at ?? null,
        });
    }
}

// ---------------------------------------------------------------------------
// OperatorDocument  (full Operator record stored in g8es document store)
// ---------------------------------------------------------------------------

export class OperatorDocument extends G8eIdentifiableModel {
    static fields = {
        user_id:                      { type: F.string,  required: true },
        organization_id:              { type: F.string,  default: null },
        component:                    { type: F.string,  default: SourceComponent.G8EO },
        name:                         { type: F.string,  default: null },
        operator_session_id:          { type: F.string,  default: null },
        bound_web_session_id:         { type: F.string,  default: null },
        api_key:                      { type: F.string,  default: null },
        operator_api_key:             { type: F.string,  default: null },
        operator_api_key_created_at:  { type: F.date,    default: null },
        operator_api_key_updated_at:  { type: F.date,    default: null },
        operator_cert:                { type: F.string,  default: null },
        operator_cert_serial:         { type: F.string,  default: null },
        operator_cert_not_before:     { type: F.date,    default: null },
        operator_cert_not_after:      { type: F.date,    default: null },
        operator_cert_created_at:     { type: F.date,    default: null },
        status:                       { type: F.string,  default: null },
        started_at:                   { type: F.date,    default: null },
        terminated_at:                { type: F.date,    default: null },
        first_deployed:               { type: F.date,    default: null },
        last_heartbeat:               { type: F.date,    default: null },
        runtime_config:               { type: F.object,  default: () => ({}) },
        system_fingerprint:           { type: F.string,  default: null },
        fingerprint_details:          { type: F.any,     default: null },
        system_info:                  { type: F.object,  model: SystemInfo,        default: () => new SystemInfo({}) },
        slot_number:                  { type: F.any,     default: null },
        is_slot:                      { type: F.boolean, default: false },
        claimed:                      { type: F.boolean, default: false },
        operator_type:                { type: F.string,  default: OperatorType.SYSTEM },
        cloud_subtype:                { type: F.string,  default: null },
        is_g8ep:                  { type: F.boolean, default: false },
        slot_cost:                    { type: F.number,  default: 1 },
        consumed_by_operator_id:      { type: F.string,  default: null },
        case_id:                      { type: F.string,  default: null },
        investigation_id:             { type: F.string,  default: null },
        task_id:                      { type: F.string,  default: null },
        error_message:                { type: F.string,  default: null },
        granted_intents:              { type: F.array,   items: GrantedIntent,     default: null },
        termination_reason:           { type: F.string,  default: null },
        stop_reason:                  { type: F.string,  default: null },
        shutdown_reason:              { type: F.string,  default: null },
        history_trail:                { type: F.array,   items: HistoryEntry,      default: () => [] },
    };

    static parse(raw = {}) {
        const parsed = super.parse(raw);
        if (parsed.system_fingerprint === null && parsed.system_info?.system_fingerprint) {
            parsed.system_fingerprint = parsed.system_info.system_fingerprint;
        }
        if (parsed.fingerprint_details === null && parsed.system_info?.fingerprint_details) {
            parsed.fingerprint_details = parsed.system_info.fingerprint_details;
        }
        return parsed;
    }

    forWire() {
        const obj = this.forDB();
        delete obj.operator_cert;
        delete obj.api_key;
        delete obj.operator_api_key;
        delete obj.operator_api_key_created_at;
        delete obj.operator_api_key_updated_at;
        return obj;
    }

    forClient() {
        const obj = this.forDB();
        const hasApiKey = !!this.api_key;
        delete obj.operator_cert;
        delete obj.operator_cert_key;
        delete obj.operator_cert_serial;
        delete obj.operator_cert_not_before;
        delete obj.operator_cert_not_after;
        delete obj.api_key;
        delete obj.operator_api_key;
        delete obj.operator_api_key_created_at;
        delete obj.operator_api_key_updated_at;
        obj.has_api_key = hasApiKey;
        return obj;
    }

    forInternal() {
        return this.forDB();
    }

    static fromDB(raw) {
        return raw ? OperatorDocument.parse(raw) : null;
    }

    static forCreate(data) {
        const _now = now();
        const systemInfo = data.system_info instanceof SystemInfo
            ? data.system_info
            : new SystemInfo(data.system_info || {});

        return new OperatorDocument({
            id:                       data.id,
            user_id:                   data.user_id,
            organization_id:           data.organization_id ?? null,
            component:                 SourceComponent.G8EO,
            name:                      data.name ?? null,
            operator_session_id:       data.operator_session_id ?? null,
            bound_web_session_id:      data.bound_web_session_id ?? null,
            api_key:                   data.api_key ?? null,
            status:                    OperatorStatus.AVAILABLE,
            created_at:                _now,
            updated_at:                _now,
            runtime_config:            data.runtime_config || {},
            system_fingerprint:        systemInfo.system_fingerprint,
            fingerprint_details:       systemInfo.fingerprint_details,
            system_info:               systemInfo,
            slot_number:               data.slot_number ?? null,
            operator_type:             data.operator_type || OperatorType.SYSTEM,
            cloud_subtype:             data.cloud_subtype ?? null,
            is_g8ep:               data.is_g8ep ?? false,
            slot_cost:                 1,
            history_trail:             [new HistoryEntry({
                timestamp:  _now,
                event_type: HistoryEventType.CREATED,
                summary:    'Operator created by g8ed',
                actor:      SourceComponent.G8ED,
                details:    {
                    operator_session_id: data.operator_session_id ? data.operator_session_id.substring(0, 12) + '...' : null,
                    bound_web_session_id: data.bound_web_session_id ? data.bound_web_session_id.substring(0, 12) + '...' : null,
                },
            })],
        });
    }

    static forSlot(data) {
        const _now = now();
        const isCloud = data.operatorType === OperatorType.CLOUD;
        const systemInfo = isCloud && data.cloudSubtype
            ? SystemInfo.forCloudOperator(data.cloudSubtype)
            : isCloud
                ? new SystemInfo({ is_cloud_operator: true })
                : new SystemInfo({});

        return new OperatorDocument({
            id:                       data.id,
            user_id:                   data.userId,
            organization_id:           data.organizationId ?? null,
            component:                 SourceComponent.G8EO,
            name:                      data.isG8eNode ? 'g8ep' : `${data.namePrefix}-${data.slotNumber}`,
            api_key:                   data.operatorApiKey ?? null,
            status:                    OperatorStatus.AVAILABLE,
            created_at:                _now,
            updated_at:                _now,
            slot_number:               data.slotNumber,
            is_slot:                   true,
            claimed:                   false,
            operator_type:             data.operatorType || OperatorType.SYSTEM,
            cloud_subtype:             data.cloudSubtype ?? null,
            is_g8ep:               data.isG8eNode ?? false,
            slot_cost:                 1,
            system_info:               systemInfo,
            runtime_config:            {},
            history_trail:             [new HistoryEntry({
                timestamp:  _now,
                event_type: HistoryEventType.SLOT_CREATED,
                summary:    `${isCloud ? 'Cloud operator' : 'Operator'} slot ${data.slotNumber} created`,
                actor:      SourceComponent.G8ED,
                details:    {
                    slot_number:   data.slotNumber,
                    operator_type: data.operatorType || OperatorType.SYSTEM,
                },
            })],
        });
    }

    static forRefresh(data) {
        const _now = now();

        return new OperatorDocument({
            id:                       data.id,
            user_id:                   data.userId,
            organization_id:           data.organizationId ?? null,
            component:                 SourceComponent.G8EO,
            name:                      data.name,
            slot_number:               data.slotNumber,
            operator_type:             data.operatorType || OperatorType.SYSTEM,
            cloud_subtype:             data.cloudSubtype ?? null,
            is_g8ep:               data.isG8eNode ?? false,
            slot_cost:                 data.slotCost ?? 1,
            status:                    OperatorStatus.AVAILABLE,
            created_at:                _now,
            updated_at:                _now,
            api_key:                   data.newApiKey ?? null,
            operator_cert:             data.certInfo?.cert ?? null,
            operator_cert_serial:      data.certInfo?.serial ?? null,
            operator_cert_not_before:  data.certInfo?.not_before ?? null,
            operator_cert_not_after:   data.certInfo?.not_after ?? null,
            operator_cert_created_at:  data.certInfo?.serial ? _now : null,
            system_info:               new SystemInfo({}),
            runtime_config:            {},
            history_trail:             [new HistoryEntry({
                timestamp:  _now,
                event_type: HistoryEventType.CREATED_FROM_REFRESH,
                summary:    'New Operator created from API key refresh',
                actor:      SourceComponent.USER,
                details:    {
                    predecessor_operator_id: data.oldId,
                    slot_number:             data.slotNumber,
                    operator_type:           data.operatorType || OperatorType.SYSTEM,
                    slot_cost:               data.slotCost ?? 1,
                    old_cert_serial:         data.oldCertSerial ? data.oldCertSerial.substring(0, 16) + '...' : null,
                    new_cert_serial:         data.certInfo?.serial ? data.certInfo.serial.substring(0, 16) + '...' : null,
                },
            })],
        });
    }

    static forReset(data) {
        const _now = now();

        return new OperatorDocument({
            id:                       data.id,
            user_id:                   data.user_id,
            organization_id:           data.organization_id ?? null,
            component:                 SourceComponent.G8EO,
            name:                      data.name,
            slot_number:               data.slot_number ?? null,
            api_key:                   data.api_key ?? null,
            status:                    OperatorStatus.AVAILABLE,
            created_at:                _now,
            updated_at:                _now,
            system_info:               new SystemInfo({}),
            runtime_config:            {},
            history_trail:             [new HistoryEntry({
                timestamp:  _now,
                event_type: HistoryEventType.RESET,
                summary:    'Operator reset to fresh state',
                actor:      SourceComponent.G8ED,
                details:    { reset_type: 'demo_reset' },
            })],
        });
    }
}

// ---------------------------------------------------------------------------
// OperatorSlotSystemInfo  (minimal system_info for operator panel list)
// ---------------------------------------------------------------------------

export class OperatorSlotSystemInfo extends G8eBaseModel {
    static fields = {
        hostname:       { type: F.string,  default: null },
        os:             { type: F.string,  default: null },
        architecture:   { type: F.string,  default: null },
        cpu_count:      { type: F.number,  default: null },
        memory_mb:      { type: F.number,  default: null },
        current_user:   { type: F.string,  default: null },
        internal_ip:    { type: F.string,  default: null },
        public_ip:      { type: F.string,  default: null },
        os_details:     { type: F.object,  default: null },
        user_details:   { type: F.object,  default: null },
        disk_details:   { type: F.object,  default: null },
        memory_details: { type: F.object,  default: null },
        environment:    { type: F.object,  default: null },
    };

    static fromSystemInfo(systemInfo) {
        if (!systemInfo) return new OperatorSlotSystemInfo({});
        return new OperatorSlotSystemInfo({
            hostname:       systemInfo.hostname ?? null,
            os:             systemInfo.os ?? null,
            architecture:   systemInfo.architecture ?? null,
            cpu_count:      systemInfo.cpu_count ?? null,
            memory_mb:      systemInfo.memory_mb ?? null,
            current_user:   systemInfo.current_user ?? null,
            internal_ip:    systemInfo.internal_ip ?? null,
            public_ip:      systemInfo.public_ip ?? null,
            os_details:     systemInfo.os_details ?? null,
            user_details:   systemInfo.user_details ?? null,
            disk_details:   systemInfo.disk_details ?? null,
            memory_details: systemInfo.memory_details ?? null,
            environment:    systemInfo.environment ?? null,
        });
    }
}

// ---------------------------------------------------------------------------
// OperatorSlot  (lightweight projection of OperatorDocument for panel list)
// ---------------------------------------------------------------------------

export class OperatorSlot extends G8eBaseModel {
    static fields = {
        operator_id:    { type: F.string,  required: true },
        name:           { type: F.string,  default: null },
        status:         { type: F.string,  default: null },
        status_display: { type: F.string,  default: null },
        status_class:   { type: F.string,  default: 'inactive' },
        bound_web_session_id: { type: F.string,  default: null },
        is_g8ep:        { type: F.boolean, default: false },
        first_deployed: { type: F.date,    default: null },
        last_heartbeat: { type: F.date,    default: null },
        system_info:    { type: F.object,  model: OperatorSlotSystemInfo, default: () => new OperatorSlotSystemInfo({}) },
        latest_heartbeat_snapshot: { type: F.object, default: null },
    };

    static fromOperator(operator) {
        const s = operator.status ?? OperatorStatus.OFFLINE;
        return new OperatorSlot({
            operator_id:    operator.id,
            name:           operator.name ?? null,
            status:         s,
            status_display: String(s).toUpperCase(),
            status_class:   String(s).toLowerCase(),
            bound_web_session_id: operator.bound_web_session_id ?? null,
            is_g8ep:        operator.is_g8ep ?? false,
            first_deployed: operator.first_deployed ?? null,
            last_heartbeat: operator.last_heartbeat ?? null,
            system_info:    OperatorSlotSystemInfo.fromSystemInfo(operator.system_info),
            latest_heartbeat_snapshot: operator.latest_heartbeat_snapshot ?? null,
        });
    }
}

// ---------------------------------------------------------------------------
// OperatorListUpdatedEvent  (SSE payload for OPERATOR_LIST_UPDATED events)
// ---------------------------------------------------------------------------

export class OperatorListUpdatedEvent extends G8eBaseModel {
    static fields = {
        type:         { type: F.string, required: true },
        operators:    { type: F.array,  items: OperatorSlot, default: () => [] },
        total_count:  { type: F.number, default: 0 },
        active_count: { type: F.number, default: 0 },
        used_slots:   { type: F.number, default: 0 },
        max_slots:    { type: F.number, default: 0 },
        timestamp:    { type: F.date,   default: () => now() },
    };

    forWire() {
        const { type, ...rest } = this.forDB();
        return { type, data: rest };
    }
}

// ---------------------------------------------------------------------------
// GeneratedCertificate  (return value from certificate generation)
// ---------------------------------------------------------------------------

export class GeneratedCertificate extends G8eBaseModel {
    static fields = {
        cert:       { type: F.string, required: true },
        key:        { type: F.string, required: true },
        serial:     { type: F.string, required: true },
        not_before: { type: F.date,   required: true },
        not_after:  { type: F.date,   required: true },
        subject:    { type: F.object, default: () => ({}) },
    };

    forWire() {
        const obj = this.forDB();
        obj.notBefore = obj.not_before;
        obj.notAfter  = obj.not_after;
        delete obj.not_before;
        delete obj.not_after;
        return obj;
    }
}

// ---------------------------------------------------------------------------
// CRLDocument  (Certificate Revocation List persisted to disk)
// ---------------------------------------------------------------------------

export class CRLDocument extends G8eBaseModel {
    static fields = {
        version:               { type: F.number, default: 1 },
        issuer:                { type: F.string, required: true },
        last_updated:          { type: F.date,   default: () => now() },
        next_update:           { type: F.date,   required: true },
        revoked_certificates:  { type: F.array,  default: () => [] },
        signature:             { type: F.any,    default: null },
    };
}

// ---------------------------------------------------------------------------
// OperatorSlotCreationResponse
// ---------------------------------------------------------------------------

export class OperatorSlotCreationResponse extends G8eBaseModel {
    static fields = {
        success:     { type: F.boolean, required: true },
        operator_id: { type: F.string,  default: null },
        message:     { type: F.string,  default: null },
    };

    static forSuccess(operatorId) {
        return new OperatorSlotCreationResponse({
            success:     true,
            operator_id: operatorId,
        });
    }

    static forFailure(message) {
        return new OperatorSlotCreationResponse({
            success: false,
            message: message,
        });
    }
}

// ---------------------------------------------------------------------------
// OperatorRefreshKeyResponse
// ---------------------------------------------------------------------------

export class OperatorRefreshKeyResponse extends G8eBaseModel {
    static fields = {
        success:         { type: F.boolean, required: true },
        new_api_key:     { type: F.string,  default: null },
        new_operator_id: { type: F.string,  default: null },
        message:         { type: F.string,  default: null },
    };

    static forSuccess(newApiKey, newOperatorId) {
        return new OperatorRefreshKeyResponse({
            success:         true,
            new_api_key:     newApiKey,
            new_operator_id: newOperatorId,
        });
    }

    static forFailure(message) {
        return new OperatorRefreshKeyResponse({
            success: false,
            message: message,
        });
    }
}

// ---------------------------------------------------------------------------
// BindOperatorsResponse
// ---------------------------------------------------------------------------

export class BindOperatorsResponse extends G8eBaseModel {
    static fields = {
        success:             { type: F.boolean, required: true },
        bound_count:         { type: F.number,  default: 0 },
        failed_count:        { type: F.number,  default: 0 },
        bound_operator_ids:  { type: F.array,   default: () => [] },
        failed_operator_ids: { type: F.array,   default: () => [] },
        errors:              { type: F.array,   default: () => [] },
        statusCode:          { type: F.number,  default: 200 },
        error:               { type: F.string,  default: null },
    };

    static forSuccess(boundIds = []) {
        return new BindOperatorsResponse({
            success: true,
            bound_count: boundIds.length,
            bound_operator_ids: boundIds,
        });
    }

    static forFailure(error, statusCode = 400) {
        return new BindOperatorsResponse({
            success: false,
            statusCode,
            error,
        });
    }
}

// ---------------------------------------------------------------------------
// UnbindOperatorsResponse
// ---------------------------------------------------------------------------

export class UnbindOperatorsResponse extends G8eBaseModel {
    static fields = {
        success:              { type: F.boolean, required: true },
        unbound_count:        { type: F.number,  default: 0 },
        failed_count:         { type: F.number,  default: 0 },
        unbound_operator_ids: { type: F.array,   default: () => [] },
        failed_operator_ids:  { type: F.array,   default: () => [] },
        errors:               { type: F.array,   default: () => [] },
        statusCode:           { type: F.number,  default: 200 },
        error:                { type: F.string,  default: null },
    };

    static forSuccess(unboundIds = []) {
        return new UnbindOperatorsResponse({
            success: true,
            unbound_count: unboundIds.length,
            unbound_operator_ids: unboundIds,
        });
    }

    static forFailure(error, statusCode = 400) {
        return new UnbindOperatorsResponse({
            success: false,
            statusCode,
            error,
        });
    }

    forClient() {
        return this.forWire();
    }
}

// ---------------------------------------------------------------------------
// OperatorWithSessionContext
// ---------------------------------------------------------------------------

export class OperatorWithSessionContext extends G8eBaseModel {
    static fields = {
        operator_id:         { type: F.string,  required: true },
        operator_session_id: { type: F.string,  default: null },
        bound_web_session_id: { type: F.string,  default: null },
        status:              { type: F.string,  required: true },
        system_info:         { type: F.object,  model: SystemInfo, default: () => new SystemInfo({}) },
        user_id:             { type: F.string,  required: true },
        organization_id:     { type: F.string,  default: null },
        case_id:             { type: F.string,  default: null },
        investigation_id:    { type: F.string,  default: null },
        task_id:             { type: F.string,  default: null },
        operator_type:       { type: F.string,  default: null },
    };

    static create(operator, operatorSession, webSession) {
        return new OperatorWithSessionContext({
            operator_id:         operator.id,
            operator_session_id: operatorSession?.id || operator.operator_session_id,
            bound_web_session_id: webSession?.id || operator.bound_web_session_id,
            status:              operator.status,
            system_info:         operator.system_info,
            user_id:             operator.user_id,
            organization_id:     operator.organization_id,
            case_id:             operator.case_id,
            investigation_id:    operator.investigation_id,
            task_id:             operator.task_id,
            operator_type:       operator.operator_type,
        });
    }

    forWire() {
        return this.forDB();
    }
}
