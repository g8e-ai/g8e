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

import { G8eBaseModel, G8eIdentifiableModel, F, now } from './base.js';
import { ApiKeyStatus, AuthMethod, DeviceLinkStatus } from '../constants/auth.js';
import { SessionType } from '../constants/session.js';

// ---------------------------------------------------------------------------
// AuthAdminAuditEntry  (auth admin access audit record stored in g8es)
// ---------------------------------------------------------------------------

export class AuthAdminAuditEntry extends G8eIdentifiableModel {
    static fields = {
        event_type:   { type: F.string, required: true },
        action:       { type: F.string, required: true },
        timestamp:    { type: F.date,   default: () => now() },
        user_id:      { type: F.string, default: null },
        user_email:   { type: F.string, default: null },
        ip:           { type: F.string, default: null },
        user_agent:   { type: F.string, default: null },
        path:         { type: F.string, default: null },
        method:       { type: F.string, default: null },
        query_params: { type: F.any,    default: null },
        metadata:     { type: F.object, default: () => ({}) },
    };
}

// ---------------------------------------------------------------------------
// DownloadTokenData  (one-time download token payload stored in g8es KV)
// ---------------------------------------------------------------------------

export class DownloadTokenData extends G8eBaseModel {
    static fields = {
        user_id:     { type: F.string, required: true },
        operator_id: { type: F.string, default: null },
    };

    static fromKV(raw) {
        return DownloadTokenData.parse(raw);
    }

}

// ---------------------------------------------------------------------------
// DownloadAuditEntry  (operator binary download token audit record stored in g8es)
// ---------------------------------------------------------------------------

export class DownloadAuditEntry extends G8eBaseModel {
    static fields = {
        event_type:   { type: F.string, required: true },
        token_prefix: { type: F.string, required: true },
        timestamp:    { type: F.date,   default: () => now() },
        ip_address:   { type: F.string, default: null },
        user_agent:   { type: F.string, default: null },
        user_id:      { type: F.string, default: null },
        operator_id:  { type: F.string, default: null },
    };
}

// ---------------------------------------------------------------------------
// LoginAttemptEntry  (single entry in failed-attempt history)
// ---------------------------------------------------------------------------

export class LoginAttemptEntry extends G8eBaseModel {
    static fields = {
        timestamp:          { type: F.date,   default: () => now() },
        ip:                 { type: F.string, default: null },
        user_agent:         { type: F.string, default: null },
        device_fingerprint: { type: F.string, default: null },
    };
}

// ---------------------------------------------------------------------------
// AccountLockData  (persisted to KV + g8es document store on account lockout)
// ---------------------------------------------------------------------------

export class AccountLockData extends G8eIdentifiableModel {
    static fields = {
        identifier:       { type: F.string, required: true },
        locked_at:        { type: F.date,   default: () => now() },
        failed_attempts:  { type: F.number, default: 0 },
        last_attempt_ip:  { type: F.string, default: null },
        attempt_history:  { type: F.array,  items: LoginAttemptEntry, default: () => [] },
    };
}

// ---------------------------------------------------------------------------
// FailedAttemptsData  (persisted to KV with TTL on each failed login)
// ---------------------------------------------------------------------------

export class FailedAttemptsData extends G8eIdentifiableModel {
    static fields = {
        count:          { type: F.number, default: 0 },
        first_attempt:  { type: F.date,   default: () => now() },
        last_attempt:   { type: F.date,   default: () => now() },
        history:        { type: F.array,  items: LoginAttemptEntry, default: () => [] },
    };
}

// ---------------------------------------------------------------------------
// IpTrackEntry  (single entry in per-IP multi-account anomaly tracker)
// ---------------------------------------------------------------------------

export class IpTrackEntry extends G8eBaseModel {
    static fields = {
        id: { type: F.string, required: true },
        ts: { type: F.date,   default: () => now() },
    };
}

// ---------------------------------------------------------------------------
// LoginAuditEntry  (login audit log record stored in g8es)
// ---------------------------------------------------------------------------

export class LoginAuditEntry extends G8eIdentifiableModel {
    static fields = {
        event_type:           { type: F.string,  required: true },
        identifier:           { type: F.string,  required: true },
        identifier_redacted:  { type: F.string,  required: true },
        timestamp:            { type: F.date,    default: () => now() },
        ip:                   { type: F.string,  default: null },
        user_agent:           { type: F.string,  default: null },
        device_fingerprint:   { type: F.string,  default: null },
        metadata:             { type: F.object,  default: () => ({}) },
    };
}

// ---------------------------------------------------------------------------
// AuthAuditEntry  (operator auth audit log record stored in g8es)
// ---------------------------------------------------------------------------

export class AuthAuditEntry extends G8eIdentifiableModel {
    static fields = {
        event_type:      { type: F.string,  required: true },
        result:          { type: F.string,  required: true },
        user_id:         { type: F.string,  default: null },
        api_key_prefix:  { type: F.string,  default: null },
        auth_method:     { type: F.string,  default: () => AuthMethod.KV_PUBSUB },
        timestamp:       { type: F.date,    default: () => now() },
        ip:              { type: F.string,  default: null },
        user_agent:      { type: F.string,  default: null },
        metadata:        { type: F.object,  default: () => ({}) },
    };
}

// ---------------------------------------------------------------------------
// ApiKeyDocument  (API key record stored in g8es document store + g8es KV)
// ---------------------------------------------------------------------------

export class ApiKeyDocument extends G8eIdentifiableModel {
    static fields = {
        user_id:         { type: F.string,  required: true },
        organization_id: { type: F.string,  default: null },
        operator_id:     { type: F.string,  default: null },
        client_name:     { type: F.string,  required: true },
        permissions:     { type: F.array,   default: () => [] },
        status:          { type: F.string,  default: () => ApiKeyStatus.ACTIVE },
        last_used_at:    { type: F.date,    default: null },
        expires_at:      { type: F.date,    default: null },
    };
}

// ---------------------------------------------------------------------------
// SessionDocument  (base — shared fields for all session types)
// ---------------------------------------------------------------------------

export class SessionDocument extends G8eIdentifiableModel {
    static fields = {
        id:                   { type: F.string,  required: true },
        session_type:         { type: F.string,  required: true },
        user_id:              { type: F.string,  required: true },
        organization_id:      { type: F.string,  default: null },
        user_data:            { type: F.any,     default: null },
        api_key:              { type: F.any,     default: null },
        client_ip:            { type: F.string,  default: null },
        user_agent:           { type: F.string,  default: null },
        login_method:         { type: F.string,  default: null },
        absolute_expires_at:  { type: F.date,    required: true },
        idle_expires_at:      { type: F.date,    required: true },
        last_activity:        { type: F.date,    default: () => now() },
        last_ip:              { type: F.string,  default: null },
        ip_changes:           { type: F.number,  default: 0 },
        suspicious_activity:  { type: F.boolean, default: false },
        is_active:            { type: F.boolean, default: true },
        operator_status:      { type: F.string,  default: null },
        metadata:             { type: F.object,  default: null },
    };

    static parse(raw = {}) {
        const type = raw?.session_type;
        if (type === SessionType.OPERATOR) return OperatorSessionDocument._parse(raw);
        if (type === SessionType.WEB) return WebSessionDocument._parse(raw);
        return G8eIdentifiableModel.parse.call(SessionDocument, raw);
    }
}

// ---------------------------------------------------------------------------
// WebSessionDocument  (web browser session — tracks bound operators)
// ---------------------------------------------------------------------------

export class WebSessionDocument extends SessionDocument {
    static fields = {
        operator_ids: { type: F.array, default: () => [] },
        operator_id:  { type: F.string, default: null },
    };

    static _parse(raw = {}) {
        return G8eIdentifiableModel.parse.call(WebSessionDocument, raw);
    }

    static parse(raw = {}) {
        return WebSessionDocument._parse(raw);
    }
}

// ---------------------------------------------------------------------------
// OperatorSessionDocument  (operator process session — operator_id required)
// ---------------------------------------------------------------------------

export class OperatorSessionDocument extends SessionDocument {
    static fields = {
        operator_id: { type: F.any, required: true },
    };

    static _parse(raw = {}) {
        return G8eIdentifiableModel.parse.call(OperatorSessionDocument, raw);
    }

    static parse(raw = {}) {
        return OperatorSessionDocument._parse(raw);
    }
}

// ---------------------------------------------------------------------------
// DeviceLinkClaim  (single operator slot claim entry within a DeviceLinkData)
// ---------------------------------------------------------------------------

export class DeviceLinkClaim extends G8eBaseModel {
    static fields = {
        system_fingerprint: { type: F.string, required: true },
        hostname:           { type: F.string, default: null },
        operator_id:        { type: F.string, default: null },
        claimed_at:         { type: F.date,   default: () => now() },
    };
}

// ---------------------------------------------------------------------------
// DeviceLinkData  (device link record stored in g8es KV)
// ---------------------------------------------------------------------------

export class DeviceLinkData extends G8eBaseModel {
    static fields = {
        token:           { type: F.string, required: true },
        user_id:         { type: F.string, required: true },
        organization_id: { type: F.string, default: null },
        operator_id:     { type: F.string, default: null },
        web_session_id:  { type: F.string, default: null },
        name:            { type: F.string, default: null },
        max_uses:        { type: F.number, default: null },
        uses:            { type: F.number, default: 0 },
        status:          { type: F.string, required: true },
        created_at:      { type: F.date,   default: null },
        expires_at:      { type: F.date,   default: null },
        used_at:         { type: F.date,   default: null },
        revoked_at:      { type: F.date,   default: null },
        device_info:     { type: F.any,    default: null },
        claims:          { type: F.array,  items: DeviceLinkClaim, default: () => [] },
    };

    static fromKV(raw) {
        return DeviceLinkData.parse(raw);
    }
}
