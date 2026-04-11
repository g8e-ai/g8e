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
// Response models
//
// Every object with Date fields sent via res.json() must be one of these models,
// serialized via .forWire() or .forClient() at the res.json() boundary.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// HealthResponse  (GET /health/)
// ---------------------------------------------------------------------------

export class HealthResponse extends VSOBaseModel {
    static fields = {
        status:    { type: F.string, required: true },
        timestamp: { type: F.date,   default: () => now() },
        service:   { type: F.string, required: true },
        checks:    { type: F.object, default: null },
        error:     { type: F.string, default: null },
    };
}

// ---------------------------------------------------------------------------
// WebSessionResponse
// ---------------------------------------------------------------------------

export class WebSessionResponse extends VSOBaseModel {
    static fields = {
        success:       { type: F.boolean, required: true },
        authenticated: { type: F.boolean, required: true },
        session:       { type: F.object,  default: null },
    };
}

// ---------------------------------------------------------------------------
// AccountStatusResponse
// ---------------------------------------------------------------------------

export class AccountStatusResponse extends VSOBaseModel {
    static fields = {
        success:          { type: F.boolean, required: true },
        locked:           { type: F.boolean, required: true },
        locked_at:        { type: F.date,    default: null },
        failed_attempts:  { type: F.number,  default: 0 },
        requires_captcha: { type: F.boolean, default: false },
    };
}

// ---------------------------------------------------------------------------
// UserMeResponse
// ---------------------------------------------------------------------------

export class UserMeResponse extends VSOBaseModel {
    static fields = {
        id:                 { type: F.string,  required: true },
        email:              { type: F.string,  required: true },
        name:               { type: F.string,  default: null },
        roles:              { type: F.array,   default: () => [] },
        organization_id:    { type: F.string,  default: null },
        dev_logs_enabled:   { type: F.boolean, default: false },
        created_at:         { type: F.date,    default: null },
    };
}

// ---------------------------------------------------------------------------
// OperatorApiKeyResponse
// ---------------------------------------------------------------------------

export class OperatorApiKeyResponse extends VSOBaseModel {
    static fields = {
        success:     { type: F.boolean, required: true },
        operator_id: { type: F.string,  required: true },
        api_key:     { type: F.string,  required: true },
    };
}


// ---------------------------------------------------------------------------
// OperatorRefreshKeyResponse
// ---------------------------------------------------------------------------

export class OperatorRefreshKeyResponse extends VSOBaseModel {
    static fields = {
        success:         { type: F.boolean, required: true },
        message:         { type: F.string,  default: null },
        old_operator_id: { type: F.string,  required: true },
        new_operator_id: { type: F.string,  required: true },
        slot_number:     { type: F.number,  required: true },
        new_api_key:     { type: F.string,  required: true },
    };
}

// ---------------------------------------------------------------------------
// ChatMessageResponse
// ---------------------------------------------------------------------------

export class ChatMessageResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        data:    { type: F.any,     default: null },
        error:   { type: F.string,  default: null },
    };
}

// ---------------------------------------------------------------------------
// InvestigationListResponse
// ---------------------------------------------------------------------------

export class InvestigationListResponse extends VSOBaseModel {
    static fields = {
        success:        { type: F.boolean, required: true },
        investigations: { type: F.array,   default: () => [] },
        count:          { type: F.number,  default: 0 },
    };
}

// ---------------------------------------------------------------------------
// LockedAccountsResponse
// ---------------------------------------------------------------------------

export class LockedAccountsResponse extends VSOBaseModel {
    static fields = {
        success:         { type: F.boolean, required: true },
        locked_accounts: { type: F.array,   default: () => [] },
        count:           { type: F.number,  default: 0 },
    };
}

// ---------------------------------------------------------------------------
// DeviceLinkResponse
// ---------------------------------------------------------------------------

export class DeviceLinkResponse extends VSOBaseModel {
    static fields = {
        success:          { type: F.boolean, required: true },
        token:            { type: F.string,  required: true },
        operator_command: { type: F.string,  required: true },
        expires_at:       { type: F.date,    required: true },
        name:             { type: F.string,  default: null },
        max_uses:         { type: F.number,  default: null },
    };
}

// ---------------------------------------------------------------------------
// DeviceLinkListResponse
// ---------------------------------------------------------------------------

export class DeviceLinkListResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        links:   { type: F.array,   default: () => [] },
    };
}

// ---------------------------------------------------------------------------
// DeviceRegistrationResponse
// ---------------------------------------------------------------------------

export class DeviceRegistrationResponse extends VSOBaseModel {
    static fields = {
        operator_session_id: { type: F.string,  required: true },
        operator_id:         { type: F.string,  required: true },
    };
}

// ---------------------------------------------------------------------------
// PlatformSetupConfigResponse
// ---------------------------------------------------------------------------

export class PlatformSetupConfigResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        config:  { type: F.object,  required: true },
    };
}



// ---------------------------------------------------------------------------
// CacheStatsResponse  (GET /health/cache-stats)
// ---------------------------------------------------------------------------

export class CacheStatsResponse extends VSOBaseModel {
    static fields = {
        timestamp:         { type: F.date,   default: () => now() },
        service:           { type: F.string, required: true },
        cache_performance: { type: F.any,    default: null },
        cache_by_type:     { type: F.any,    default: null },
        cost_savings:      { type: F.any,    default: null },
        message:           { type: F.string, default: null },
    };
}

// ---------------------------------------------------------------------------
// MetricsHealthResponse  (GET /metrics/health)
// ---------------------------------------------------------------------------

export class MetricsHealthResponse extends VSOBaseModel {
    static fields = {
        success:   { type: F.boolean, required: true },
        status:    { type: F.string,  required: true },
        service:   { type: F.string,  required: true },
        vsodb:     { type: F.object,  default: () => ({}) },
        timestamp: { type: F.date,    default: () => now() },
        error:     { type: F.string,  default: null },
    };
}

// ---------------------------------------------------------------------------
// ChatHealthResponse  (GET /api/chat/health)
// ---------------------------------------------------------------------------

export class ChatHealthResponse extends VSOBaseModel {
    static fields = {
        service:           { type: F.string, required: true },
        status:            { type: F.string, required: true },
        internal_services: { type: F.any,    default: null },
        timestamp:         { type: F.date,   default: () => now() },
        error:             { type: F.string, default: null },
    };
}

// ---------------------------------------------------------------------------
// SSEHealthResponse  (GET /api/sse/health)
// ---------------------------------------------------------------------------

export class SSEHealthResponse extends VSOBaseModel {
    static fields = {
        status:           { type: F.string, required: true },
        service:          { type: F.string, required: true },
        timestamp:        { type: F.date,   default: () => now() },
        localConnections: { type: F.number, default: 0 },
        uniqueSessions:   { type: F.number, default: 0 },
        config:           { type: F.object, default: null },
    };
}

// ---------------------------------------------------------------------------
// OperatorAuthResponse
// ---------------------------------------------------------------------------

export class OperatorAuthResponse extends VSOBaseModel {
    static fields = {
        success:             { type: F.boolean, required: true },
        operator_session_id: { type: F.string,  required: true },
        operator_id:         { type: F.string,  required: true },
        user_id:             { type: F.string,  required: true },
        api_key:             { type: F.string,  required: true },
        config:              { type: F.object,  required: true },
        session:             { type: F.object,  required: true },
        operator_cert:       { type: F.string,  default: null },
        operator_cert_key:   { type: F.string,  default: null },
    };
}

// ---------------------------------------------------------------------------
// UserRegisterResponse
// ---------------------------------------------------------------------------

export class UserRegisterResponse extends VSOBaseModel {
    static fields = {
        message:           { type: F.string,  default: null },
        user_id:           { type: F.string,  required: true },
        challenge_options: { type: F.object,  default: null },
    };
}

// ---------------------------------------------------------------------------
// PasskeyRegisterChallengeResponse
// ---------------------------------------------------------------------------

export class PasskeyRegisterChallengeResponse extends VSOBaseModel {
    static fields = {
        message: { type: F.string,  default: null },
        options: { type: F.object,  required: true },
        token:   { type: F.string,  default: null }, // Only for pending registration
    };
}

// ---------------------------------------------------------------------------
// PasskeyAuthChallengeResponse
// ---------------------------------------------------------------------------

export class PasskeyAuthChallengeResponse extends VSOBaseModel {
    static fields = {
        success:     { type: F.boolean, required: true },
        message:     { type: F.string,  default: null },
        options:     { type: F.object,  default: null },
        needs_setup: { type: F.boolean, default: false },
        user_id:     { type: F.string,  default: null },
    };
}

// ---------------------------------------------------------------------------
// PasskeyVerifyResponse
// ---------------------------------------------------------------------------

export class PasskeyVerifyResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        message: { type: F.string,  default: null },
        session: { type: F.object,  default: null },
    };
}

// ---------------------------------------------------------------------------
// SimpleSuccessResponse
// ---------------------------------------------------------------------------

export class SimpleSuccessResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        message: { type: F.string,  default: null },
    };
}

// ---------------------------------------------------------------------------
// ErrorResponse
// ---------------------------------------------------------------------------

export class ErrorResponse extends VSOBaseModel {
    static fields = {
        error:        { type: F.object,  required: true },
        trace_id:     { type: F.string,  default: null },
        execution_id: { type: F.string,  default: null },
    };

    forClient() {
        return this.forWire();
    }
}

// ---------------------------------------------------------------------------
// SettingsResponse
// ---------------------------------------------------------------------------

export class SettingsResponse extends VSOBaseModel {
    static fields = {
        success:  { type: F.boolean, required: true },
        message:  { type: F.string,  default: null },
        settings: { type: F.object,  required: true },
        sections: { type: F.array,   default: () => [] },
    };
}

// ---------------------------------------------------------------------------
// SettingsUpdateResponse
// ---------------------------------------------------------------------------

export class SettingsUpdateResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        message: { type: F.string,  default: null },
        saved:   { type: F.array,   default: () => [] },
        skipped: { type: F.array,   default: () => [] },
        invalid: { type: F.array,   default: () => [] },
    };
}

// ---------------------------------------------------------------------------
// OperatorListResponse
// ---------------------------------------------------------------------------

export class OperatorListResponse extends VSOBaseModel {
    static fields = {
        success:     { type: F.boolean, required: true },
        data:        { type: F.array,   default: () => [] },
        total_count: { type: F.number,  default: 0 },
        active_count:{ type: F.number,  default: 0 },
    };
}

// ---------------------------------------------------------------------------
// OperatorSlotsResponse
// ---------------------------------------------------------------------------

export class OperatorSlotsResponse extends VSOBaseModel {
    static fields = {
        success:      { type: F.boolean, required: true },
        operator_ids: { type: F.array,   default: () => [] },
        count:        { type: F.number,  default: 0 },
    };
}

// ---------------------------------------------------------------------------
// InternalUserListResponse
// ---------------------------------------------------------------------------

export class InternalUserListResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        users:   { type: F.array,   default: () => [] },
        count:   { type: F.number,  default: 0 },
    };
}

// ---------------------------------------------------------------------------
// InternalUserResponse
// ---------------------------------------------------------------------------

export class InternalUserResponse extends VSOBaseModel {
    static fields = {
        user:    { type: F.object,  default: null },
    };
}

// ---------------------------------------------------------------------------
// AuditEventResponse
// ---------------------------------------------------------------------------

export class AuditEventResponse extends VSOBaseModel {
    static fields = {
        events:  { type: F.array,   default: () => [] },
        count:   { type: F.number,  default: 0 },
        total_investigations: { type: F.number, default: 0 },
    };
}

// ---------------------------------------------------------------------------
// UserDevLogsResponse
// ---------------------------------------------------------------------------

export class UserDevLogsResponse extends VSOBaseModel {
    static fields = {
        message:          { type: F.string,  default: null },
        dev_logs_enabled: { type: F.boolean, required: true },
    };
}

// ---------------------------------------------------------------------------
// UserG8eKeyRefreshResponse
// ---------------------------------------------------------------------------

export class UserG8eKeyRefreshResponse extends VSOBaseModel {
    static fields = {
        success:  { type: F.boolean, required: true },
        message:  { type: F.string,  default: null },
        g8e_key: { type: F.string,  required: true },
    };
}

// ---------------------------------------------------------------------------
// PasskeyListResponse
// ---------------------------------------------------------------------------

export class PasskeyListResponse extends VSOBaseModel {
    static fields = {
        message:     { type: F.string,  default: null },
        user_id:     { type: F.string,  required: true },
        credentials: { type: F.array,   default: () => [] },
        count:       { type: F.number,  default: 0 },
    };
}

// ---------------------------------------------------------------------------
// PasskeyRevokeResponse
// ---------------------------------------------------------------------------

export class PasskeyRevokeResponse extends VSOBaseModel {
    static fields = {
        message:       { type: F.string,  default: null },
        user_id:       { type: F.string,  required: true },
        credential_id: { type: F.string,  required: true },
        remaining:     { type: F.number,  default: 0 },
    };
}

// ---------------------------------------------------------------------------
// PasskeyRevokeAllResponse
// ---------------------------------------------------------------------------

export class PasskeyRevokeAllResponse extends VSOBaseModel {
    static fields = {
        message: { type: F.string,  default: null },
        user_id: { type: F.string,  required: true },
        revoked: { type: F.number,  default: 0 },
    };
}

// ---------------------------------------------------------------------------
// UserDeleteResponse
// ---------------------------------------------------------------------------

export class UserDeleteResponse extends VSOBaseModel {
    static fields = {
        message: { type: F.string,  default: null },
        user_id: { type: F.string,  required: true },
    };
}

// ---------------------------------------------------------------------------
// PlatformOverviewResponse
// ---------------------------------------------------------------------------

export class PlatformOverviewResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        message: { type: F.string,  default: null },
        timestamp: { type: F.date,   required: true },
        users: { type: F.object,  required: true },
        operators: { type: F.object,  required: true },
        sessions: { type: F.object,  required: true },
        cache: { type: F.object,  required: true },
        system: { type: F.object,  required: true },
    };
}

// ---------------------------------------------------------------------------
// UserStatsResponse
// ---------------------------------------------------------------------------

export class UserStatsResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        message: { type: F.string,  default: null },
        total: { type: F.number,  required: true },
        activity: { type: F.object,  required: true },
        newUsersLastWeek: { type: F.number,  required: true },
    };
}

// ---------------------------------------------------------------------------
// OperatorStatsResponse
// ---------------------------------------------------------------------------

export class OperatorStatsResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        message: { type: F.string,  default: null },
        total: { type: F.number,  required: true },
        statusDistribution: { type: F.object,  required: true },
        typeDistribution: { type: F.object,  required: true },
        health: { type: F.object,  required: true },
    };
}

// ---------------------------------------------------------------------------
// SessionStatsResponse
// ---------------------------------------------------------------------------

export class SessionStatsResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        message: { type: F.string,  default: null },
        web: { type: F.number,  required: true },
        operator: { type: F.number,  required: true },
        total: { type: F.number,  required: true },
        boundOperators: { type: F.number,  required: true },
    };
}

// ---------------------------------------------------------------------------
// AIUsageStatsResponse
// ---------------------------------------------------------------------------

export class AIUsageStatsResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        message: { type: F.string,  default: null },
        totalInvestigations: { type: F.number,  required: true },
        activeInvestigations: { type: F.number,  required: true },
        completedInvestigations: { type: F.number,  required: true },
    };
}

// ---------------------------------------------------------------------------
// LoginAuditStatsResponse
// ---------------------------------------------------------------------------

export class LoginAuditStatsResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        message: { type: F.string,  default: null },
        total: { type: F.number,  required: true },
        successful: { type: F.number,  required: true },
        failed: { type: F.number,  required: true },
        locked: { type: F.number,  required: true },
        anomalies: { type: F.number,  required: true },
        byHour: { type: F.object,  required: true },
    };
}

// ---------------------------------------------------------------------------
// RealTimeMetricsResponse
// ---------------------------------------------------------------------------

export class RealTimeMetricsResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        message: { type: F.string,  default: null },
        timestamp: { type: F.date,   required: true },
        vsodb: { type: F.object,  required: true },
        cache: { type: F.object,  required: true },
    };
}

// ---------------------------------------------------------------------------
// ComponentHealthResponse
// ---------------------------------------------------------------------------

export class ComponentHealthResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        message: { type: F.string,  default: null },
        overall: { type: F.string,  required: true },
        timestamp: { type: F.date,   required: true },
        components: { type: F.object,  required: true },
    };
}

// ---------------------------------------------------------------------------
// ConsoleDataResponse (DEPRECATED - Use specific response models instead)
// ---------------------------------------------------------------------------

export class ConsoleDataResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        message: { type: F.string,  default: null },
        data:    { type: F.any,     default: null },
    };
}

// ---------------------------------------------------------------------------
// DBCollectionsResponse
// ---------------------------------------------------------------------------

export class DBCollectionsResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        message: { type: F.string,  default: null },
        collections: { type: F.array,   default: () => [] },
    };
}

// ---------------------------------------------------------------------------
// DBQueryResponse
// ---------------------------------------------------------------------------

export class DBQueryResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        message: { type: F.string,  default: null },
        collection: { type: F.string,  required: true },
        documents: { type: F.array,   default: () => [] },
        count: { type: F.number,  default: 0 },
        limit: { type: F.number,  default: null },
    };
}

// ---------------------------------------------------------------------------
// KVScanResponse
// ---------------------------------------------------------------------------

export class KVScanResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        message: { type: F.string,  default: null },
        pattern: { type: F.string,  required: true },
        cursor: { type: F.string,  default: null },
        keys: { type: F.array,   default: () => [] },
        count: { type: F.number,  default: 0 },
        has_more: { type: F.boolean, default: false },
    };
}

// ---------------------------------------------------------------------------
// KVKeyResponse
// ---------------------------------------------------------------------------

export class KVKeyResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        message: { type: F.string,  default: null },
        key: { type: F.string,  required: true },
        exists: { type: F.boolean, required: true },
        value: { type: F.any,     default: null },
        content_type: { type: F.string, default: null },
        created_at: { type: F.date,   default: null },
        updated_at: { type: F.date,   default: null },
    };
}

// ---------------------------------------------------------------------------
// DocsTreeResponse
// ---------------------------------------------------------------------------

export class DocsTreeResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        tree:    { type: F.array,   default: () => [] },
    };
}

// ---------------------------------------------------------------------------
// DocsFileResponse
// ---------------------------------------------------------------------------

export class DocsFileResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        content: { type: F.string,  required: true },
        path:    { type: F.string,  required: true },
    };
}

// ---------------------------------------------------------------------------
// SystemNetworkInterfacesResponse
// ---------------------------------------------------------------------------

export class SystemNetworkInterfacesResponse extends VSOBaseModel {
    static fields = {
        success:    { type: F.boolean, required: true },
        interfaces: { type: F.array,   default: () => [] },
    };
}

// ---------------------------------------------------------------------------
// ChatActionResponse
// ---------------------------------------------------------------------------

export class ChatActionResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        message: { type: F.string,  default: null },
        data:    { type: F.any,     default: null },
    };
}

// ---------------------------------------------------------------------------
// SimpleStatusResponse
// ---------------------------------------------------------------------------

export class SimpleStatusResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        status:  { type: F.string,  required: true },
        message: { type: F.string,  default: null },
        details: { type: F.object,  default: null },
    };
}

// ---------------------------------------------------------------------------
// InternalHealthResponse
// ---------------------------------------------------------------------------

export class InternalHealthResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        message: { type: F.string,  required: true },
        vsodb_status: { type: F.string,  default: 'unknown' },
        g8ee_status: { type: F.string,  default: 'unknown' },
        g8eo_status: { type: F.string,  default: 'unknown' },
        uptime_seconds: { type: F.number,  default: 0 },
        memory_usage: { type: F.object,  default: () => ({}) },
    };
}

// ---------------------------------------------------------------------------
// InternalSettingsResponse
// ---------------------------------------------------------------------------

export class InternalSettingsResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        message: { type: F.string,  default: null },
        settings: { type: F.object,  default: () => ({ settings: {} }) },
    };
}

// ---------------------------------------------------------------------------
// InternalSessionValidationResponse
// ---------------------------------------------------------------------------

export class InternalSessionValidationResponse extends VSOBaseModel {
    static fields = {
        success: { type: F.boolean, required: true },
        message: { type: F.string,  default: null },
        session_id: { type: F.string,  default: null },
        user_id: { type: F.string,  default: null },
        valid: { type: F.boolean,  default: false },
        expires_at: { type: F.date,   default: null },
        validation_details: { type: F.object,  default: null },
    };
}


// ---------------------------------------------------------------------------
// BindOperatorsResponse
// ---------------------------------------------------------------------------

export class BindOperatorsResponse extends VSOBaseModel {
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

    forClient() {
        return this.forWire();
    }
}

// ---------------------------------------------------------------------------
// UnbindOperatorsResponse
// ---------------------------------------------------------------------------

export class UnbindOperatorsResponse extends VSOBaseModel {
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


export class OperatorBinaryAvailabilityResponse extends VSOBaseModel {
    static fields = {
        success:   { type: F.boolean, required: true },
        status:    { type: F.string,  required: true },
        component: { type: F.string,  required: true },
        version:   { type: F.string,  required: true },
        platforms: { type: F.array,   default: () => [] },
    };
}

// ---------------------------------------------------------------------------
// OperatorSessionRefreshResponse
// ---------------------------------------------------------------------------

export class OperatorSessionRefreshResponse extends VSOBaseModel {
    static fields = {
        message:     { type: F.string,  default: null },
        operator_id: { type: F.string,  required: true },
        session:     { type: F.object,  required: true },
    };
}

// ---------------------------------------------------------------------------
// HealthResponse  (GET /health/)
// ---------------------------------------------------------------------------
