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

import { describe, it, expect } from 'vitest';
import {
    HealthResponse,
    WebSessionResponse,
    AccountStatusResponse,
    UserMeResponse,
    OperatorApiKeyResponse,
    OperatorRefreshKeyResponse,
    ChatMessageResponse,
    InvestigationListResponse,
    LockedAccountsResponse,
    DeviceLinkResponse,
    DeviceLinkListResponse,
    DeviceRegistrationResponse,
    PlatformSetupConfigResponse,
    CacheStatsResponse,
    MetricsHealthResponse,
    ChatHealthResponse,
    SSEHealthResponse,
    OperatorAuthResponse,
    UserRegisterResponse,
    PasskeyRegisterChallengeResponse,
    PasskeyAuthChallengeResponse,
    PasskeyVerifyResponse,
    SimpleSuccessResponse,
    ErrorResponse,
    SettingsResponse,
    SettingsUpdateResponse,
    OperatorListResponse,
    OperatorSlotsResponse,
    InternalUserListResponse,
    InternalUserResponse,
    AuditEventResponse,
    UserDevLogsResponse,
    PasskeyListResponse,
    PasskeyRevokeResponse,
    PasskeyRevokeAllResponse,
    UserDeleteResponse,
    PlatformOverviewResponse,
    UserStatsResponse,
    OperatorStatsResponse,
    SessionStatsResponse,
    AIUsageStatsResponse,
    LoginAuditStatsResponse,
    RealTimeMetricsResponse,
    ComponentHealthResponse,
    ConsoleDataResponse,
    DBCollectionsResponse,
    DBQueryResponse,
    KVScanResponse,
    KVKeyResponse,
    DocsTreeResponse,
    DocsFileResponse,
    SystemNetworkInterfacesResponse,
    ChatActionResponse,
    SimpleStatusResponse,
    InternalHealthResponse,
    InternalSettingsResponse,
    InternalSessionValidationResponse,
    BindOperatorsResponse,
    UnbindOperatorsResponse,
    OperatorBinaryAvailabilityResponse,
    OperatorSessionRefreshResponse,
} from '@g8ed/models/response_models.js';

describe('OperatorListResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with default array', () => {
        const response = OperatorListResponse.parse({ success: true });
        expect(response.success).toBe(true);
        expect(response.data).toEqual([]);
        expect(response.total_count).toBe(0);
        expect(response.active_count).toBe(0);
    });

    it('accepts all fields with values', () => {
        const operators = [{ operator_id: 'op-1', status: 'ACTIVE' }];
        const response = OperatorListResponse.parse({
            success: true,
            data: operators,
            total_count: 1,
            active_count: 1,
        });
        expect(response.data).toEqual(operators);
        expect(response.total_count).toBe(1);
        expect(response.active_count).toBe(1);
    });

    it('throws when success is missing', () => {
        expect(() => OperatorListResponse.parse({}))
            .toThrow('success is required');
    });

    it('forWire() serializes response correctly', () => {
        const response = OperatorListResponse.parse({ success: true });
        const wire = response.forWire();
        expect(wire.success).toBe(true);
        expect(wire.data).toEqual([]);
        expect(wire.total_count).toBe(0);
    });
});

describe('OperatorSlotsResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with default array', () => {
        const response = OperatorSlotsResponse.parse({ success: true });
        expect(response.success).toBe(true);
        expect(response.operator_ids).toEqual([]);
        expect(response.count).toBe(0);
    });

    it('accepts all fields with values', () => {
        const operatorIds = ['op-1', 'op-2'];
        const response = OperatorSlotsResponse.parse({
            success: true,
            operator_ids: operatorIds,
            count: 2,
        });
        expect(response.operator_ids).toEqual(operatorIds);
        expect(response.count).toBe(2);
    });

    it('throws when success is missing', () => {
        expect(() => OperatorSlotsResponse.parse({}))
            .toThrow('success is required');
    });

    it('forWire() serializes response correctly', () => {
        const response = OperatorSlotsResponse.parse({ success: true });
        const wire = response.forWire();
        expect(wire.success).toBe(true);
        expect(wire.operator_ids).toEqual([]);
        expect(wire.count).toBe(0);
    });
});

describe('InternalUserListResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with default array', () => {
        const response = InternalUserListResponse.parse({ success: true });
        expect(response.success).toBe(true);
        expect(response.users).toEqual([]);
        expect(response.count).toBe(0);
    });

    it('accepts all fields with values', () => {
        const users = [{ user_id: 'user-1', email: 'test@example.com' }];
        const response = InternalUserListResponse.parse({
            success: true,
            users: users,
            count: 1,
        });
        expect(response.users).toEqual(users);
        expect(response.count).toBe(1);
    });

    it('throws when success is missing', () => {
        expect(() => InternalUserListResponse.parse({}))
            .toThrow('success is required');
    });

    it('forWire() serializes response correctly', () => {
        const response = InternalUserListResponse.parse({ success: true });
        const wire = response.forWire();
        expect(wire.success).toBe(true);
        expect(wire.users).toEqual([]);
        expect(wire.count).toBe(0);
    });
});

describe('AuditEventResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid fields with default array', () => {
        const response = AuditEventResponse.parse({});
        expect(response.events).toEqual([]);
        expect(response.count).toBe(0);
        expect(response.total_investigations).toBe(0);
    });

    it('accepts all fields with values', () => {
        const events = [{ event_id: 'evt-1', type: 'login' }];
        const response = AuditEventResponse.parse({
            events: events,
            count: 1,
            total_investigations: 5,
        });
        expect(response.events).toEqual(events);
        expect(response.count).toBe(1);
        expect(response.total_investigations).toBe(5);
    });

    it('forWire() serializes response correctly', () => {
        const response = AuditEventResponse.parse({});
        const wire = response.forWire();
        expect(wire.events).toEqual([]);
        expect(wire.count).toBe(0);
        expect(wire.total_investigations).toBe(0);
    });
});

describe('PasskeyListResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with default array', () => {
        const response = PasskeyListResponse.parse({ user_id: 'user-123' });
        expect(response.user_id).toBe('user-123');
        expect(response.credentials).toEqual([]);
        expect(response.count).toBe(0);
        expect(response.message).toBeNull();
    });

    it('accepts all fields with values', () => {
        const credentials = [{ credential_id: 'cred-1', name: 'My Key' }];
        const response = PasskeyListResponse.parse({
            user_id: 'user-123',
            credentials: credentials,
            count: 1,
            message: 'Retrieved credentials',
        });
        expect(response.credentials).toEqual(credentials);
        expect(response.count).toBe(1);
        expect(response.message).toBe('Retrieved credentials');
    });

    it('throws when user_id is missing', () => {
        expect(() => PasskeyListResponse.parse({}))
            .toThrow('user_id is required');
    });

    it('forWire() serializes response correctly', () => {
        const response = PasskeyListResponse.parse({ user_id: 'user-123' });
        const wire = response.forWire();
        expect(wire.user_id).toBe('user-123');
        expect(wire.credentials).toEqual([]);
        expect(wire.count).toBe(0);
    });
});

describe('DBCollectionsResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with default array', () => {
        const response = DBCollectionsResponse.parse({ success: true });
        expect(response.success).toBe(true);
        expect(response.collections).toEqual([]);
        expect(response.message).toBeNull();
    });

    it('accepts all fields with values', () => {
        const collections = ['users', 'operators', 'sessions'];
        const response = DBCollectionsResponse.parse({
            success: true,
            collections: collections,
            message: 'Collections retrieved',
        });
        expect(response.collections).toEqual(collections);
        expect(response.message).toBe('Collections retrieved');
    });

    it('throws when success is missing', () => {
        expect(() => DBCollectionsResponse.parse({}))
            .toThrow('success is required');
    });

    it('forWire() serializes response correctly', () => {
        const response = DBCollectionsResponse.parse({ success: true });
        const wire = response.forWire();
        expect(wire.success).toBe(true);
        expect(wire.collections).toEqual([]);
    });
});

describe('DBQueryResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with default array', () => {
        const response = DBQueryResponse.parse({
            success: true,
            collection: 'users',
        });
        expect(response.success).toBe(true);
        expect(response.collection).toBe('users');
        expect(response.documents).toEqual([]);
        expect(response.count).toBe(0);
        expect(response.limit).toBeNull();
    });

    it('accepts all fields with values', () => {
        const documents = [{ doc_id: 'doc-1', name: 'Test' }];
        const response = DBQueryResponse.parse({
            success: true,
            collection: 'users',
            documents: documents,
            count: 1,
            limit: 10,
        });
        expect(response.documents).toEqual(documents);
        expect(response.count).toBe(1);
        expect(response.limit).toBe(10);
    });

    it('throws when success is missing', () => {
        expect(() => DBQueryResponse.parse({ collection: 'users' }))
            .toThrow('success is required');
    });

    it('throws when collection is missing', () => {
        expect(() => DBQueryResponse.parse({ success: true }))
            .toThrow('collection is required');
    });

    it('forWire() serializes response correctly', () => {
        const response = DBQueryResponse.parse({
            success: true,
            collection: 'users',
        });
        const wire = response.forWire();
        expect(wire.success).toBe(true);
        expect(wire.collection).toBe('users');
        expect(wire.documents).toEqual([]);
    });
});

describe('KVScanResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with default array', () => {
        const response = KVScanResponse.parse({
            success: true,
            pattern: 'user:*',
        });
        expect(response.success).toBe(true);
        expect(response.pattern).toBe('user:*');
        expect(response.keys).toEqual([]);
        expect(response.count).toBe(0);
        expect(response.cursor).toBeNull();
        expect(response.has_more).toBe(false);
    });

    it('accepts all fields with values', () => {
        const keys = ['user:1', 'user:2'];
        const response = KVScanResponse.parse({
            success: true,
            pattern: 'user:*',
            cursor: 'abc123',
            keys: keys,
            count: 2,
            has_more: true,
        });
        expect(response.keys).toEqual(keys);
        expect(response.count).toBe(2);
        expect(response.cursor).toBe('abc123');
        expect(response.has_more).toBe(true);
    });

    it('throws when success is missing', () => {
        expect(() => KVScanResponse.parse({ pattern: 'user:*' }))
            .toThrow('success is required');
    });

    it('throws when pattern is missing', () => {
        expect(() => KVScanResponse.parse({ success: true }))
            .toThrow('pattern is required');
    });

    it('forWire() serializes response correctly', () => {
        const response = KVScanResponse.parse({
            success: true,
            pattern: 'user:*',
        });
        const wire = response.forWire();
        expect(wire.success).toBe(true);
        expect(wire.pattern).toBe('user:*');
        expect(wire.keys).toEqual([]);
    });
});

describe('DocsTreeResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with default array', () => {
        const response = DocsTreeResponse.parse({ success: true });
        expect(response.success).toBe(true);
        expect(response.tree).toEqual([]);
    });

    it('accepts all fields with values', () => {
        const tree = [
            { name: 'docs', type: 'dir' },
            { name: 'README.md', type: 'file' },
        ];
        const response = DocsTreeResponse.parse({
            success: true,
            tree: tree,
        });
        expect(response.tree).toEqual(tree);
    });

    it('throws when success is missing', () => {
        expect(() => DocsTreeResponse.parse({}))
            .toThrow('success is required');
    });

    it('forWire() serializes response correctly', () => {
        const response = DocsTreeResponse.parse({ success: true });
        const wire = response.forWire();
        expect(wire.success).toBe(true);
        expect(wire.tree).toEqual([]);
    });
});

describe('SystemNetworkInterfacesResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with default array', () => {
        const response = SystemNetworkInterfacesResponse.parse({ success: true });
        expect(response.success).toBe(true);
        expect(response.interfaces).toEqual([]);
    });

    it('accepts all fields with values', () => {
        const interfaces = [
            { name: 'eth0', ip: '192.168.1.100' },
            { name: 'lo', ip: '127.0.0.1' },
        ];
        const response = SystemNetworkInterfacesResponse.parse({
            success: true,
            interfaces: interfaces,
        });
        expect(response.interfaces).toEqual(interfaces);
    });

    it('throws when success is missing', () => {
        expect(() => SystemNetworkInterfacesResponse.parse({}))
            .toThrow('success is required');
    });

    it('forWire() serializes response correctly', () => {
        const response = SystemNetworkInterfacesResponse.parse({ success: true });
        const wire = response.forWire();
        expect(wire.success).toBe(true);
        expect(wire.interfaces).toEqual([]);
    });
});

describe('InternalHealthResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with default object', () => {
        const response = InternalHealthResponse.parse({
            success: true,
            message: 'Health check passed',
        });
        expect(response.success).toBe(true);
        expect(response.message).toBe('Health check passed');
        expect(response.g8es_status).toBe('unknown');
        expect(response.g8ee_status).toBe('unknown');
        expect(response.g8eo_status).toBe('unknown');
        expect(response.uptime_seconds).toBe(0);
        expect(response.memory_usage).toEqual({});
    });

    it('accepts all fields with values', () => {
        const memoryUsage = { used: 1024, total: 4096 };
        const response = InternalHealthResponse.parse({
            success: true,
            message: 'All systems operational',
            g8es_status: 'healthy',
            g8ee_status: 'healthy',
            g8eo_status: 'healthy',
            uptime_seconds: 3600,
            memory_usage: memoryUsage,
        });
        expect(response.g8es_status).toBe('healthy');
        expect(response.g8ee_status).toBe('healthy');
        expect(response.g8eo_status).toBe('healthy');
        expect(response.uptime_seconds).toBe(3600);
        expect(response.memory_usage).toEqual(memoryUsage);
    });

    it('throws when success is missing', () => {
        expect(() => InternalHealthResponse.parse({ message: 'test' }))
            .toThrow('success is required');
    });

    it('throws when message is missing', () => {
        expect(() => InternalHealthResponse.parse({ success: true }))
            .toThrow('message is required');
    });

    it('forWire() serializes response correctly', () => {
        const response = InternalHealthResponse.parse({
            success: true,
            message: 'Health check passed',
        });
        const wire = response.forWire();
        expect(wire.success).toBe(true);
        expect(wire.message).toBe('Health check passed');
        expect(wire.memory_usage).toEqual({});
    });
});

describe('InternalSettingsResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with default object', () => {
        const response = InternalSettingsResponse.parse({ success: true });
        expect(response.success).toBe(true);
        expect(response.message).toBeNull();
        expect(response.settings).toEqual({ settings: {} });
    });

    it('accepts all fields with values', () => {
        const settings = { settings: { llm: { provider: 'gemini' } } };
        const response = InternalSettingsResponse.parse({
            success: true,
            message: 'Settings retrieved',
            settings: settings,
        });
        expect(response.message).toBe('Settings retrieved');
        expect(response.settings).toEqual(settings);
    });

    it('throws when success is missing', () => {
        expect(() => InternalSettingsResponse.parse({}))
            .toThrow('success is required');
    });

    it('forWire() serializes response correctly', () => {
        const response = InternalSettingsResponse.parse({ success: true });
        const wire = response.forWire();
        expect(wire.success).toBe(true);
        expect(wire.settings).toEqual({ settings: {} });
    });
});

describe('BindOperatorsResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with default arrays', () => {
        const response = BindOperatorsResponse.parse({ success: true });
        expect(response.success).toBe(true);
        expect(response.bound_count).toBe(0);
        expect(response.failed_count).toBe(0);
        expect(response.bound_operator_ids).toEqual([]);
        expect(response.failed_operator_ids).toEqual([]);
        expect(response.errors).toEqual([]);
        expect(response.statusCode).toBe(200);
        expect(response.error).toBeNull();
    });

    it('accepts all fields with values', () => {
        const response = BindOperatorsResponse.parse({
            success: true,
            bound_count: 2,
            failed_count: 0,
            bound_operator_ids: ['op-1', 'op-2'],
            failed_operator_ids: [],
            errors: [],
            statusCode: 200,
        });
        expect(response.bound_count).toBe(2);
        expect(response.bound_operator_ids).toEqual(['op-1', 'op-2']);
    });

    it('static forSuccess() creates success response with bound IDs', () => {
        const boundIds = ['op-1', 'op-2', 'op-3'];
        const response = BindOperatorsResponse.forSuccess(boundIds);
        expect(response.success).toBe(true);
        expect(response.bound_count).toBe(3);
        expect(response.bound_operator_ids).toEqual(boundIds);
        expect(response.failed_count).toBe(0);
        expect(response.failed_operator_ids).toEqual([]);
        expect(response.errors).toEqual([]);
        expect(response.statusCode).toBe(200);
        expect(response.error).toBeNull();
    });

    it('static forSuccess() with empty array creates zero-count response', () => {
        const response = BindOperatorsResponse.forSuccess([]);
        expect(response.success).toBe(true);
        expect(response.bound_count).toBe(0);
        expect(response.bound_operator_ids).toEqual([]);
    });

    it('static forSuccess() with no arguments defaults to empty array', () => {
        const response = BindOperatorsResponse.forSuccess();
        expect(response.success).toBe(true);
        expect(response.bound_count).toBe(0);
        expect(response.bound_operator_ids).toEqual([]);
    });

    it('static forFailure() creates failure response with error and status code', () => {
        const response = BindOperatorsResponse.forFailure('Invalid operator IDs', 400);
        expect(response.success).toBe(false);
        expect(response.error).toBe('Invalid operator IDs');
        expect(response.statusCode).toBe(400);
        expect(response.bound_count).toBe(0);
        expect(response.failed_count).toBe(0);
        expect(response.bound_operator_ids).toEqual([]);
        expect(response.failed_operator_ids).toEqual([]);
        expect(response.errors).toEqual([]);
    });

    it('static forFailure() defaults to status code 400', () => {
        const response = BindOperatorsResponse.forFailure('Validation failed');
        expect(response.success).toBe(false);
        expect(response.error).toBe('Validation failed');
        expect(response.statusCode).toBe(400);
    });

    it('static forFailure() accepts custom status code', () => {
        const response = BindOperatorsResponse.forFailure('Unauthorized', 401);
        expect(response.statusCode).toBe(401);
    });

    it('forClient() returns forWire() result', () => {
        const response = BindOperatorsResponse.parse({ success: true });
        const client = response.forClient();
        const wire = response.forWire();
        expect(client).toEqual(wire);
    });

    it('forWire() serializes response correctly', () => {
        const response = BindOperatorsResponse.parse({
            success: true,
            bound_count: 1,
            bound_operator_ids: ['op-1'],
        });
        const wire = response.forWire();
        expect(wire.success).toBe(true);
        expect(wire.bound_count).toBe(1);
        expect(wire.bound_operator_ids).toEqual(['op-1']);
    });

    it('throws when success is missing', () => {
        expect(() => BindOperatorsResponse.parse({}))
            .toThrow('success is required');
    });
});

describe('UnbindOperatorsResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with default arrays', () => {
        const response = UnbindOperatorsResponse.parse({ success: true });
        expect(response.success).toBe(true);
        expect(response.unbound_count).toBe(0);
        expect(response.failed_count).toBe(0);
        expect(response.unbound_operator_ids).toEqual([]);
        expect(response.failed_operator_ids).toEqual([]);
        expect(response.errors).toEqual([]);
        expect(response.statusCode).toBe(200);
        expect(response.error).toBeNull();
    });

    it('accepts all fields with values', () => {
        const response = UnbindOperatorsResponse.parse({
            success: true,
            unbound_count: 2,
            failed_count: 0,
            unbound_operator_ids: ['op-1', 'op-2'],
            failed_operator_ids: [],
            errors: [],
            statusCode: 200,
        });
        expect(response.unbound_count).toBe(2);
        expect(response.unbound_operator_ids).toEqual(['op-1', 'op-2']);
    });

    it('static forSuccess() creates success response with unbound IDs', () => {
        const unboundIds = ['op-1', 'op-2', 'op-3'];
        const response = UnbindOperatorsResponse.forSuccess(unboundIds);
        expect(response.success).toBe(true);
        expect(response.unbound_count).toBe(3);
        expect(response.unbound_operator_ids).toEqual(unboundIds);
        expect(response.failed_count).toBe(0);
        expect(response.failed_operator_ids).toEqual([]);
        expect(response.errors).toEqual([]);
        expect(response.statusCode).toBe(200);
        expect(response.error).toBeNull();
    });

    it('static forSuccess() with empty array creates zero-count response', () => {
        const response = UnbindOperatorsResponse.forSuccess([]);
        expect(response.success).toBe(true);
        expect(response.unbound_count).toBe(0);
        expect(response.unbound_operator_ids).toEqual([]);
    });

    it('static forSuccess() with no arguments defaults to empty array', () => {
        const response = UnbindOperatorsResponse.forSuccess();
        expect(response.success).toBe(true);
        expect(response.unbound_count).toBe(0);
        expect(response.unbound_operator_ids).toEqual([]);
    });

    it('static forFailure() creates failure response with error and status code', () => {
        const response = UnbindOperatorsResponse.forFailure('Invalid operator IDs', 400);
        expect(response.success).toBe(false);
        expect(response.error).toBe('Invalid operator IDs');
        expect(response.statusCode).toBe(400);
        expect(response.unbound_count).toBe(0);
        expect(response.failed_count).toBe(0);
        expect(response.unbound_operator_ids).toEqual([]);
        expect(response.failed_operator_ids).toEqual([]);
        expect(response.errors).toEqual([]);
    });

    it('static forFailure() defaults to status code 400', () => {
        const response = UnbindOperatorsResponse.forFailure('Validation failed');
        expect(response.success).toBe(false);
        expect(response.error).toBe('Validation failed');
        expect(response.statusCode).toBe(400);
    });

    it('static forFailure() accepts custom status code', () => {
        const response = UnbindOperatorsResponse.forFailure('Unauthorized', 401);
        expect(response.statusCode).toBe(401);
    });

    it('forClient() returns forWire() result', () => {
        const response = UnbindOperatorsResponse.parse({ success: true });
        const client = response.forClient();
        const wire = response.forWire();
        expect(client).toEqual(wire);
    });

    it('forWire() serializes response correctly', () => {
        const response = UnbindOperatorsResponse.parse({
            success: true,
            unbound_count: 1,
            unbound_operator_ids: ['op-1'],
        });
        const wire = response.forWire();
        expect(wire.success).toBe(true);
        expect(wire.unbound_count).toBe(1);
        expect(wire.unbound_operator_ids).toEqual(['op-1']);
    });

    it('throws when success is missing', () => {
        expect(() => UnbindOperatorsResponse.parse({}))
            .toThrow('success is required');
    });
});

describe('OperatorBinaryAvailabilityResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with default array', () => {
        const response = OperatorBinaryAvailabilityResponse.parse({
            success: true,
            status: 'available',
            component: 'operator',
            version: '1.0.0',
        });
        expect(response.success).toBe(true);
        expect(response.status).toBe('available');
        expect(response.component).toBe('operator');
        expect(response.version).toBe('1.0.0');
        expect(response.platforms).toEqual([]);
    });

    it('accepts all fields with values', () => {
        const platforms = ['linux-amd64', 'linux-arm64'];
        const response = OperatorBinaryAvailabilityResponse.parse({
            success: true,
            status: 'available',
            component: 'operator',
            version: '1.0.0',
            platforms: platforms,
        });
        expect(response.platforms).toEqual(platforms);
    });

    it('throws when success is missing', () => {
        expect(() => OperatorBinaryAvailabilityResponse.parse({
            status: 'available',
            component: 'operator',
            version: '1.0.0',
        })).toThrow('success is required');
    });

    it('throws when status is missing', () => {
        expect(() => OperatorBinaryAvailabilityResponse.parse({
            success: true,
            component: 'operator',
            version: '1.0.0',
        })).toThrow('status is required');
    });

    it('throws when component is missing', () => {
        expect(() => OperatorBinaryAvailabilityResponse.parse({
            success: true,
            status: 'available',
            version: '1.0.0',
        })).toThrow('component is required');
    });

    it('throws when version is missing', () => {
        expect(() => OperatorBinaryAvailabilityResponse.parse({
            success: true,
            status: 'available',
            component: 'operator',
        })).toThrow('version is required');
    });

    it('forWire() serializes response correctly', () => {
        const response = OperatorBinaryAvailabilityResponse.parse({
            success: true,
            status: 'available',
            component: 'operator',
            version: '1.0.0',
        });
        const wire = response.forWire();
        expect(wire.success).toBe(true);
        expect(wire.status).toBe('available');
        expect(wire.component).toBe('operator');
        expect(wire.version).toBe('1.0.0');
        expect(wire.platforms).toEqual([]);
    });
});

describe('ErrorResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields', () => {
        const error = { message: 'Test error', code: 'TEST_ERROR' };
        const response = ErrorResponse.parse({ error });
        expect(response.error).toEqual(error);
        expect(response.trace_id).toBeNull();
        expect(response.execution_id).toBeNull();
    });

    it('accepts optional trace_id and execution_id', () => {
        const error = { message: 'Test error' };
        const response = ErrorResponse.parse({
            error,
            trace_id: 'trace-123',
            execution_id: 'exec-456',
        });
        expect(response.trace_id).toBe('trace-123');
        expect(response.execution_id).toBe('exec-456');
    });

    it('throws when error is missing', () => {
        expect(() => ErrorResponse.parse({}))
            .toThrow('error is required');
    });

    it('forClient() returns forWire() result', () => {
        const error = { message: 'Test error' };
        const response = ErrorResponse.parse({ error });
        const client = response.forClient();
        const wire = response.forWire();
        expect(client).toEqual(wire);
    });

    it('forWire() serializes response correctly', () => {
        const error = { message: 'Test error', code: 'TEST' };
        const response = ErrorResponse.parse({ error });
        const wire = response.forWire();
        expect(wire.error).toEqual(error);
    });
});

describe('HealthResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with default timestamp', () => {
        const response = HealthResponse.parse({
            status: 'healthy',
            service: 'g8ed',
        });
        expect(response.status).toBe('healthy');
        expect(response.service).toBe('g8ed');
        expect(response.timestamp).toBeInstanceOf(Date);
        expect(response.checks).toBeNull();
        expect(response.error).toBeNull();
    });

    it('accepts all fields with values', () => {
        const checks = { database: 'ok', cache: 'ok' };
        const response = HealthResponse.parse({
            status: 'healthy',
            service: 'g8ed',
            checks: checks,
            error: null,
        });
        expect(response.checks).toEqual(checks);
    });

    it('throws when status is missing', () => {
        expect(() => HealthResponse.parse({ service: 'g8ed' }))
            .toThrow('status is required');
    });

    it('throws when service is missing', () => {
        expect(() => HealthResponse.parse({ status: 'healthy' }))
            .toThrow('service is required');
    });

    it('forWire() serializes timestamp to ISO string', () => {
        const response = HealthResponse.parse({
            status: 'healthy',
            service: 'g8ed',
        });
        const wire = response.forWire();
        expect(typeof wire.timestamp).toBe('string');
    });
});

describe('InternalSessionValidationResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const response = InternalSessionValidationResponse.parse({ success: true });
        expect(response.success).toBe(true);
        expect(response.message).toBeNull();
        expect(response.session_id).toBeNull();
        expect(response.user_id).toBeNull();
        expect(response.valid).toBe(false);
        expect(response.expires_at).toBeNull();
        expect(response.validation_details).toBeNull();
    });

    it('accepts all fields with values', () => {
        const expiresAt = new Date('2026-12-31T23:59:59.000Z');
        const validationDetails = { reason: 'valid', checks_passed: 3 };
        const response = InternalSessionValidationResponse.parse({
            success: true,
            message: 'Session is valid',
            session_id: 'sess-123',
            user_id: 'user-456',
            valid: true,
            expires_at: expiresAt,
            validation_details: validationDetails,
        });
        expect(response.session_id).toBe('sess-123');
        expect(response.user_id).toBe('user-456');
        expect(response.valid).toBe(true);
        expect(response.expires_at).toBe(expiresAt);
        expect(response.validation_details).toEqual(validationDetails);
    });

    it('forWire() serializes date to ISO string', () => {
        const expiresAt = new Date('2026-12-31T23:59:59.000Z');
        const response = InternalSessionValidationResponse.parse({
            success: true,
            expires_at: expiresAt,
        });
        const wire = response.forWire();
        expect(typeof wire.expires_at).toBe('string');
    });
});

describe('OperatorRefreshKeyResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid operator API key format (g8e_ + 8 hex + _ + 64 hex)', () => {
        const validKey = 'g8e_1a2b3c4d_' + '0'.repeat(64);
        const response = OperatorRefreshKeyResponse.parse({
            success: true,
            old_operator_id: 'op-old-123',
            new_operator_id: 'op-new-456',
            slot_number: 1,
            new_api_key: validKey,
        });
        expect(response.new_api_key).toBe(validKey);
    });

    it('accepts valid regular API key format (g8e_ + 64 hex)', () => {
        const validKey = 'g8e_' + '0'.repeat(64);
        const response = OperatorRefreshKeyResponse.parse({
            success: true,
            old_operator_id: 'op-old-123',
            new_operator_id: 'op-new-456',
            slot_number: 1,
            new_api_key: validKey,
        });
        expect(response.new_api_key).toBe(validKey);
    });

    it('rejects invalid API key format - missing prefix', () => {
        try {
            OperatorRefreshKeyResponse.parse({
                success: true,
                old_operator_id: 'op-old-123',
                new_operator_id: 'op-new-456',
                slot_number: 1,
                new_api_key: 'invalid_key_format',
            });
            throw new Error('Should have thrown');
        } catch (err) {
            expect(err.message).toBe('OperatorRefreshKeyResponse validation failed: new_api_key must match g8e API key format (g8e_ prefix followed by hex characters)');
            expect(err.validationErrors).toEqual(['new_api_key must match g8e API key format (g8e_ prefix followed by hex characters)']);
        }
    });

    it('rejects status string as API key (regression test)', () => {
        try {
            OperatorRefreshKeyResponse.parse({
                success: true,
                old_operator_id: 'op-old-123',
                new_operator_id: 'op-new-456',
                slot_number: 1,
                new_api_key: 'AVAILABLE',
            });
            throw new Error('Should have thrown');
        } catch (err) {
            expect(err.message).toBe('OperatorRefreshKeyResponse validation failed: new_api_key must match g8e API key format (g8e_ prefix followed by hex characters)');
            expect(err.validationErrors).toEqual(['new_api_key must match g8e API key format (g8e_ prefix followed by hex characters)']);
        }
    });

    it('rejects API key with incorrect hex length', () => {
        try {
            OperatorRefreshKeyResponse.parse({
                success: true,
                old_operator_id: 'op-old-123',
                new_operator_id: 'op-new-456',
                slot_number: 1,
                new_api_key: 'g8e_' + '0'.repeat(32),
            });
            throw new Error('Should have thrown');
        } catch (err) {
            expect(err.message).toBe('OperatorRefreshKeyResponse validation failed: new_api_key must match g8e API key format (g8e_ prefix followed by hex characters)');
            expect(err.validationErrors).toEqual(['new_api_key must match g8e API key format (g8e_ prefix followed by hex characters)']);
        }
    });

    it('rejects API key with non-hex characters', () => {
        try {
            OperatorRefreshKeyResponse.parse({
                success: true,
                old_operator_id: 'op-old-123',
                new_operator_id: 'op-new-456',
                slot_number: 1,
                new_api_key: 'g8e_' + 'g'.repeat(64),
            });
            throw new Error('Should have thrown');
        } catch (err) {
            expect(err.message).toBe('OperatorRefreshKeyResponse validation failed: new_api_key must match g8e API key format (g8e_ prefix followed by hex characters)');
            expect(err.validationErrors).toEqual(['new_api_key must match g8e API key format (g8e_ prefix followed by hex characters)']);
        }
    });

    it('throws when required fields are missing', () => {
        expect(() => OperatorRefreshKeyResponse.parse({}))
            .toThrow('success is required');
    });

    it('forSuccess() creates success response with valid key, operator IDs, and slot number', () => {
        const validKey = 'g8e_' + '0'.repeat(64);
        const response = OperatorRefreshKeyResponse.forSuccess(
            validKey,
            'op-new-456',
            'op-old-123',
            1,
            'Key refreshed'
        );
        expect(response.success).toBe(true);
        expect(response.new_api_key).toBe(validKey);
        expect(response.new_operator_id).toBe('op-new-456');
        expect(response.old_operator_id).toBe('op-old-123');
        expect(response.slot_number).toBe(1);
        expect(response.message).toBe('Key refreshed');
    });

    it('forSuccess() throws with invalid API key format', () => {
        expect(() => OperatorRefreshKeyResponse.forSuccess('invalid-key', 'op-new-456', 'op-old-123', 1))
            .toThrow('new_api_key must match g8e API key format');
    });

    it('forFailure() creates failure response with message', () => {
        const response = OperatorRefreshKeyResponse.forFailure('Refresh failed');
        expect(response.success).toBe(false);
        expect(response.message).toBe('Refresh failed');
        expect(response.new_api_key).toBeNull();
    });
});
