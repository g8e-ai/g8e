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
    GrantedIntent,
    HeartbeatNotification,
    SystemInfo,
    HeartbeatSnapshot,
    HistoryEntry,
    CertInfo,
    OperatorStatusInfo,
    OperatorDocument,
    OperatorListUpdatedEvent,
    GeneratedCertificate,
    CRLDocument,
    OperatorSlotCreationResponse,
    OperatorRefreshKeyResponse,
    BindOperatorsResponse,
    UnbindOperatorsResponse,
    OperatorWithSessionContext,
    OperatorSlot,
} from '@g8ed/models/operator_model.js';
import { OperatorStatus, OperatorType, CloudOperatorSubtype, HistoryEventType } from '@g8ed/constants/operator.js';
import { SourceComponent } from '@g8ed/constants/ai.js';
import { INTENT_TTL_SECONDS } from '@g8ed/constants/auth.js';

describe('GrantedIntent [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields', () => {
        const grantedAt = new Date('2026-01-01T00:00:00.000Z');
        const expiresAt = new Date('2026-01-01T01:00:00.000Z');
        const intent = GrantedIntent.parse({
            name: 'file.write',
            granted_at: grantedAt,
            expires_at: expiresAt,
        });
        expect(intent.name).toBe('file.write');
        expect(intent.granted_at).toBe(grantedAt);
        expect(intent.expires_at).toBe(expiresAt);
    });

    it('isActive() returns true when expires_at is in the future', () => {
        const future = new Date(Date.now() + 3600000);
        const intent = new GrantedIntent({
            name: 'test',
            granted_at: new Date(),
            expires_at: future,
        });
        expect(intent.isActive()).toBe(true);
    });

    it('isActive() returns false when expires_at is in the past', () => {
        const past = new Date(Date.now() - 3600000);
        const intent = new GrantedIntent({
            name: 'test',
            granted_at: new Date(Date.now() - 7200000),
            expires_at: past,
        });
        expect(intent.isActive()).toBe(false);
    });

    it('from() returns the same instance if already GrantedIntent', () => {
        const original = new GrantedIntent({
            name: 'test',
            granted_at: new Date(),
            expires_at: new Date(Date.now() + 3600000),
        });
        const result = GrantedIntent.from(original);
        expect(result).toBe(original);
    });

    it('from() parses plain object via parse()', () => {
        const data = {
            name: 'file.read',
            granted_at: new Date(),
            expires_at: new Date(Date.now() + 3600000),
        };
        const result = GrantedIntent.from(data);
        expect(result).toBeInstanceOf(GrantedIntent);
        expect(result.name).toBe('file.read');
    });

    it('create() generates intent with proper TTL', () => {
        const intent = GrantedIntent.create('file.write');
        expect(intent.name).toBe('file.write');
        expect(intent.granted_at).toBeInstanceOf(Date);
        expect(intent.expires_at).toBeInstanceOf(Date);
        const diff = intent.expires_at.getTime() - intent.granted_at.getTime();
        expect(diff).toBe(INTENT_TTL_SECONDS * 1000);
    });
});

describe('HeartbeatNotification [UNIT - PURE LOGIC]', () => {
    it('accepts heartbeat_data directly', () => {
        const hbData = { status: 'active', uptime: 3600 };
        const notification = new HeartbeatNotification({
            heartbeat_data: hbData,
            investigation_id: 'inv-123',
            case_id: 'case-456',
        });
        expect(notification.heartbeat_data).toBe(hbData);
        expect(notification.investigation_id).toBe('inv-123');
        expect(notification.case_id).toBe('case-456');
    });

    it('extracts investigation_id from heartbeat_data when not provided', () => {
        const hbData = { investigation_id: 'inv-789', case_id: 'case-101' };
        const notification = new HeartbeatNotification(hbData);
        expect(notification.investigation_id).toBe('inv-789');
        expect(notification.case_id).toBe('case-101');
    });

    it('parse() handles nested heartbeat_data structure', () => {
        const raw = {
            heartbeat_data: { status: 'active', investigation_id: 'inv-123' },
            investigation_id: 'inv-456',
        };
        const notification = HeartbeatNotification.parse(raw);
        expect(notification.heartbeat_data.status).toBe('active');
        expect(notification.investigation_id).toBe('inv-456');
    });

    it('from() returns same instance if already HeartbeatNotification', () => {
        const original = new HeartbeatNotification({ status: 'active' });
        const result = HeartbeatNotification.from(original);
        expect(result).toBe(original);
    });
});

describe('SystemInfo [UNIT - PURE LOGIC]', () => {
    it('accepts all fields with defaults', () => {
        const info = new SystemInfo({});
        expect(info.hostname).toBeNull();
        expect(info.os).toBeNull();
        expect(info.is_cloud_operator).toBe(false);
        expect(info.local_storage_enabled).toBe(true);
    });

    it('forCloudOperator() sets cloud fields with explicit subtype', () => {
        const info = SystemInfo.forCloudOperator(CloudOperatorSubtype.AWS);
        expect(info.cloud_provider).toBe(CloudOperatorSubtype.AWS);
        expect(info.is_cloud_operator).toBe(true);
    });

    it('forCloudOperator() sets G8E_POD subtype', () => {
        const info = SystemInfo.forCloudOperator(CloudOperatorSubtype.G8E_POD);
        expect(info.cloud_provider).toBe(CloudOperatorSubtype.G8E_POD);
        expect(info.is_cloud_operator).toBe(true);
    });

    it('forCloudOperator() throws when subtype is missing', () => {
        expect(() => SystemInfo.forCloudOperator()).toThrow('requires an explicit cloud subtype');
        expect(() => SystemInfo.forCloudOperator(null)).toThrow('requires an explicit cloud subtype');
    });
});

describe('HeartbeatSnapshot [UNIT - PURE LOGIC]', () => {
    it('empty() returns snapshot with all nulls', () => {
        const snapshot = HeartbeatSnapshot.empty();
        expect(snapshot.timestamp).toBeNull();
        expect(snapshot.cpu_percent).toBeNull();
        expect(snapshot.memory_percent).toBeNull();
    });

    it('fromHeartbeat() extracts performance metrics from canonical wire format', () => {
        const heartbeat = {
            performance_metrics: {
                cpu_percent: 75.5,
                memory_percent: 60.2,
                disk_percent: 45.0,
                network_latency: 25,
                memory_used_mb: 9830,
                memory_total_mb: 16384,
                disk_used_gb: 250.5,
                disk_total_gb: 500.0,
            },
            uptime_info: {
                uptime: '2 days, 3 hours',
                uptime_seconds: 183600,
            },
        };
        const timestamp = new Date('2026-01-01T00:00:00.000Z');
        const snapshot = HeartbeatSnapshot.fromHeartbeat(heartbeat, timestamp);
        expect(snapshot.timestamp).toBe(timestamp);
        expect(snapshot.cpu_percent).toBe(75.5);
        expect(snapshot.memory_percent).toBe(60.2);
        expect(snapshot.disk_percent).toBe(45.0);
        expect(snapshot.network_latency).toBe(25);
        expect(snapshot.uptime).toBe('2 days, 3 hours');
        expect(snapshot.uptime_seconds).toBe(183600);
    });

    it('fromHeartbeat() handles missing metrics', () => {
        const heartbeat = {};
        const timestamp = new Date();
        const snapshot = HeartbeatSnapshot.fromHeartbeat(heartbeat, timestamp);
        expect(snapshot.cpu_percent).toBeNull();
        expect(snapshot.memory_percent).toBeNull();
    });

    it('fromHeartbeat() uses uptime_string as fallback for uptime', () => {
        const heartbeat = {
            uptime_info: {
                uptime_string: '1h 30m',
                uptime_seconds: 5400,
            },
        };
        const timestamp = new Date();
        const snapshot = HeartbeatSnapshot.fromHeartbeat(heartbeat, timestamp);
        expect(snapshot.uptime).toBe('1h 30m');
        expect(snapshot.uptime_seconds).toBe(5400);
    });

    it('fromHeartbeat() handles null performance_metrics', () => {
        const heartbeat = {
            performance_metrics: null,
            uptime_info: null,
        };
        const timestamp = new Date();
        const snapshot = HeartbeatSnapshot.fromHeartbeat(heartbeat, timestamp);
        expect(snapshot.cpu_percent).toBeNull();
        expect(snapshot.memory_percent).toBeNull();
        expect(snapshot.disk_percent).toBeNull();
        expect(snapshot.network_latency).toBeNull();
        expect(snapshot.uptime).toBeNull();
        expect(snapshot.uptime_seconds).toBeNull();
    });
});

describe('HistoryEntry [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const entry = new HistoryEntry({
            event_type: HistoryEventType.CREATED,
            summary: 'Operator created',
        });
        expect(entry.event_type).toBe(HistoryEventType.CREATED);
        expect(entry.summary).toBe('Operator created');
        expect(entry.actor).toBe(SourceComponent.G8ED);
        expect(entry.details).toEqual({});
        expect(entry.timestamp).toBeInstanceOf(Date);
    });

    it('accepts custom actor and details', () => {
        const entry = HistoryEntry.parse({
            event_type: HistoryEventType.BOUND,
            summary: 'Operator bound',
            actor: SourceComponent.G8EO,
            details: { web_session_id: 'ws-123' },
        });
        expect(entry.actor).toBe(SourceComponent.G8EO);
        expect(entry.details).toEqual({ web_session_id: 'ws-123' });
    });

    it('throws when event_type is missing', () => {
        expect(() => HistoryEntry.parse({ summary: 'test' }))
            .toThrow('event_type is required');
    });

    it('throws when summary is missing', () => {
        expect(() => HistoryEntry.parse({ event_type: HistoryEventType.CREATED }))
            .toThrow('summary is required');
    });
});

describe('CertInfo [UNIT - PURE LOGIC]', () => {
    it('empty() returns CertInfo with all nulls', () => {
        const cert = CertInfo.empty();
        expect(cert.cert).toBeNull();
        expect(cert.key).toBeNull();
        expect(cert.serial).toBeNull();
    });

    it('fromCertData() maps notBefore/notAfter to snake_case', () => {
        const certData = {
            cert: '-----BEGIN CERT-----',
            key: '-----BEGIN KEY-----',
            serial: 'ABC123',
            notBefore: new Date('2026-01-01T00:00:00.000Z'),
            notAfter: new Date('2027-01-01T00:00:00.000Z'),
        };
        const cert = CertInfo.fromCertData(certData);
        expect(cert.cert).toBe('-----BEGIN CERT-----');
        expect(cert.key).toBe('-----BEGIN KEY-----');
        expect(cert.serial).toBe('ABC123');
        expect(cert.not_before).toBeInstanceOf(Date);
        expect(cert.not_after).toBeInstanceOf(Date);
    });
});

describe('OperatorStatusInfo [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const info = OperatorStatusInfo.parse({
            operator_id: 'op-123',
            user_id: 'user-456',
            status: OperatorStatus.ACTIVE,
        });
        expect(info.operator_id).toBe('op-123');
        expect(info.user_id).toBe('user-456');
        expect(info.status).toBe(OperatorStatus.ACTIVE);
        expect(info.web_session_id).toBeNull();
        expect(info.is_active).toBe(false);
    });

    it('is_active is computed by fromOperator() based on status', () => {
        const activeOp = new OperatorDocument({
            operator_id: 'op-1',
            user_id: 'user-1',
            status: OperatorStatus.ACTIVE,
        });
        const active = OperatorStatusInfo.fromOperator(activeOp);
        expect(active.is_active).toBe(true);

        const boundOp = new OperatorDocument({
            operator_id: 'op-2',
            user_id: 'user-2',
            status: OperatorStatus.BOUND,
        });
        const bound = OperatorStatusInfo.fromOperator(boundOp);
        expect(bound.is_active).toBe(false);
    });

    it('fromOperator() maps OperatorDocument to status info', () => {
        const operator = new OperatorDocument({
            operator_id: 'op-123',
            user_id: 'user-456',
            status: OperatorStatus.ACTIVE,
            web_session_id: 'ws-789',
            operator_session_id: 'os-101',
            last_heartbeat: new Date('2026-01-01T00:00:00.000Z'),
            system_info: new SystemInfo({ hostname: 'test-host' }),
            investigation_id: 'inv-111',
            case_id: 'case-222',
            operator_type: OperatorType.SYSTEM,
            granted_intents: [GrantedIntent.create('test')],
            cloud_subtype: CloudOperatorSubtype.AWS,
        });
        const info = OperatorStatusInfo.fromOperator(operator);
        expect(info.operator_id).toBe('op-123');
        expect(info.user_id).toBe('user-456');
        expect(info.status).toBe(OperatorStatus.ACTIVE);
        expect(info.web_session_id).toBe('ws-789');
        expect(info.operator_session_id).toBe('os-101');
        expect(info.is_active).toBe(true);
        expect(info.current_hostname).toBe('test-host');
        expect(info.investigation_id).toBe('inv-111');
        expect(info.case_id).toBe('case-222');
        expect(info.operator_type).toBe(OperatorType.SYSTEM);
        expect(info.cloud_subtype).toBe(CloudOperatorSubtype.AWS);
        expect(info.granted_intents).toHaveLength(1);
    });

    it('fromOperator() handles plain object system_info', () => {
        const operator = new OperatorDocument({
            operator_id: 'op-123',
            user_id: 'user-456',
            status: OperatorStatus.ACTIVE,
            system_info: { hostname: 'plain-host' },
        });
        const info = OperatorStatusInfo.fromOperator(operator);
        expect(info.system_info).toBeInstanceOf(SystemInfo);
        expect(info.system_info.hostname).toBe('plain-host');
    });
});

describe('OperatorDocument [UNIT - PURE LOGIC]', () => {
    it('parse() migrates system_fingerprint from system_info', () => {
        const raw = {
            operator_id: 'op-123',
            user_id: 'user-456',
            status: OperatorStatus.AVAILABLE,
            system_fingerprint: null,
            system_info: {
                system_fingerprint: 'fp-abc123',
                fingerprint_details: { cpu: 'x86_64' },
            },
        };
        const doc = OperatorDocument.parse(raw);
        expect(doc.system_fingerprint).toBe('fp-abc123');
        expect(doc.fingerprint_details).toEqual({ cpu: 'x86_64' });
    });

    it('forWire() removes operator_cert, api_key, and operator_api_key fields', () => {
        const doc = new OperatorDocument({
            operator_id: 'op-123',
            user_id: 'user-456',
            status: OperatorStatus.AVAILABLE,
            operator_cert: 'cert-data',
            api_key: 'secret-key',
            operator_api_key: 'operator-secret-key',
            operator_api_key_created_at: new Date('2026-01-01'),
            operator_api_key_updated_at: new Date('2026-01-15'),
        });
        const wire = doc.forWire();
        expect(wire.operator_cert).toBeUndefined();
        expect(wire.api_key).toBeUndefined();
        expect(wire.operator_api_key).toBeUndefined();
        expect(wire.operator_api_key_created_at).toBeUndefined();
        expect(wire.operator_api_key_updated_at).toBeUndefined();
        expect(wire.operator_id).toBe('op-123');
    });

    it('forClient() removes sensitive fields and adds has_api_key flag', () => {
        const doc = new OperatorDocument({
            operator_id: 'op-123',
            user_id: 'user-456',
            status: OperatorStatus.AVAILABLE,
            operator_cert: 'cert-data',
            operator_cert_key: 'key-data',
            operator_cert_serial: 'serial-123',
            operator_cert_not_before: new Date(),
            operator_cert_not_after: new Date(),
            api_key: 'secret-key',
            operator_api_key: 'operator-secret-key',
        });
        const client = doc.forClient();
        expect(client.operator_cert).toBeUndefined();
        expect(client.operator_cert_key).toBeUndefined();
        expect(client.operator_cert_serial).toBeUndefined();
        expect(client.operator_cert_not_before).toBeUndefined();
        expect(client.operator_cert_not_after).toBeUndefined();
        expect(client.api_key).toBeUndefined();
        expect(client.operator_api_key).toBeUndefined();
        expect(client.has_api_key).toBe(true);
    });

    it('forInternal() preserves all sensitive fields including api_key and operator_api_key', () => {
        const doc = new OperatorDocument({
            operator_id: 'op-123',
            user_id: 'user-456',
            status: OperatorStatus.AVAILABLE,
            operator_cert: 'cert-data',
            api_key: 'secret-key',
            operator_api_key: 'operator-secret-key',
            operator_api_key_created_at: new Date('2026-01-01'),
            operator_api_key_updated_at: new Date('2026-01-15'),
        });
        const internal = doc.forInternal();
        expect(internal.operator_cert).toBe('cert-data');
        expect(internal.api_key).toBe('secret-key');
        expect(internal.operator_api_key).toBe('operator-secret-key');
        expect(internal.operator_api_key_created_at).toBe('2026-01-01T00:00:00.000Z');
        expect(internal.operator_api_key_updated_at).toBe('2026-01-15T00:00:00.000Z');
        expect(internal.operator_id).toBe('op-123');
    });

    it('forClient() sets has_api_key to false when api_key is null', () => {
        const doc = new OperatorDocument({
            operator_id: 'op-123',
            user_id: 'user-456',
            status: OperatorStatus.AVAILABLE,
            api_key: null,
        });
        const client = doc.forClient();
        expect(client.has_api_key).toBe(false);
    });

    it('fromDB() returns null for null input', () => {
        const doc = OperatorDocument.fromDB(null);
        expect(doc).toBeNull();
    });

    it('fromDB() parses valid raw document', () => {
        const raw = {
            operator_id: 'op-123',
            user_id: 'user-456',
            status: OperatorStatus.AVAILABLE,
        };
        const doc = OperatorDocument.fromDB(raw);
        expect(doc).toBeInstanceOf(OperatorDocument);
        expect(doc.operator_id).toBe('op-123');
    });

    it('forCreate() creates operator with AVAILABLE status', () => {
        const systemInfo = new SystemInfo({ hostname: 'test-host' });
        const doc = OperatorDocument.forCreate({
            operator_id: 'op-123',
            user_id: 'user-456',
            system_info: systemInfo,
            name: 'Test Operator',
            operator_session_id: 'os-789',
            web_session_id: 'ws-101',
            api_key: 'key-abc',
            runtime_config: { timeout: 30 },
            slot_number: 1,
            operator_type: OperatorType.CLOUD,
            cloud_subtype: CloudOperatorSubtype.AWS,
            is_g8ep: true,
        });
        expect(doc.operator_id).toBe('op-123');
        expect(doc.user_id).toBe('user-456');
        expect(doc.status).toBe(OperatorStatus.AVAILABLE);
        expect(doc.component).toBe(SourceComponent.G8EO);
        expect(doc.name).toBe('Test Operator');
        expect(doc.system_info).toBe(systemInfo);
        expect(doc.system_fingerprint).toBe(systemInfo.system_fingerprint);
        expect(doc.operator_type).toBe(OperatorType.CLOUD);
        expect(doc.cloud_subtype).toBe(CloudOperatorSubtype.AWS);
        expect(doc.is_g8ep).toBe(true);
        expect(doc.slot_cost).toBe(1);
        expect(doc.history_trail).toHaveLength(1);
        expect(doc.history_trail[0].event_type).toBe(HistoryEventType.CREATED);
    });

    it('forCreate() handles plain object system_info', () => {
        const doc = OperatorDocument.forCreate({
            operator_id: 'op-123',
            user_id: 'user-456',
            system_info: { hostname: 'test-host', system_fingerprint: 'fp-123' },
        });
        expect(doc.system_info).toBeInstanceOf(SystemInfo);
        expect(doc.system_info.hostname).toBe('test-host');
        expect(doc.system_fingerprint).toBe('fp-123');
    });

    it('forCreate() creates history entry with truncated session IDs', () => {
        const doc = OperatorDocument.forCreate({
            operator_id: 'op-123',
            user_id: 'user-456',
            operator_session_id: 'very-long-operator-session-id-12345',
            web_session_id: 'very-long-web-session-id-67890',
        });
        expect(doc.history_trail[0].details.operator_session_id).toBe('very-long-op...');
        expect(doc.history_trail[0].details.web_session_id).toBe('very-long-we...');
    });

    it('forCreate() handles null session IDs in history', () => {
        const doc = OperatorDocument.forCreate({
            operator_id: 'op-123',
            user_id: 'user-456',
        });
        expect(doc.history_trail[0].details.operator_session_id).toBeNull();
        expect(doc.history_trail[0].details.web_session_id).toBeNull();
    });

    it('forSlot() creates slot with is_slot=true', () => {
        const doc = OperatorDocument.forSlot({
            operator_id: 'op-123',
            userId: 'user-456',
            namePrefix: 'operator',
            slotNumber: 1,
            operatorType: OperatorType.SYSTEM,
            operatorApiKey: 'key-abc',
        });
        expect(doc.operator_id).toBe('op-123');
        expect(doc.user_id).toBe('user-456');
        expect(doc.name).toBe('operator-1');
        expect(doc.is_slot).toBe(true);
        expect(doc.claimed).toBe(false);
        expect(doc.slot_number).toBe(1);
        expect(doc.status).toBe(OperatorStatus.AVAILABLE);
        expect(doc.history_trail[0].event_type).toBe(HistoryEventType.SLOT_CREATED);
    });

    it('forSlot() creates cloud operator with SystemInfo.forCloudOperator()', () => {
        const doc = OperatorDocument.forSlot({
            operator_id: 'op-123',
            userId: 'user-456',
            namePrefix: 'cloud-operator',
            slotNumber: 1,
            operatorType: OperatorType.CLOUD,
            cloudSubtype: CloudOperatorSubtype.AWS,
        });
        expect(doc.system_info.cloud_provider).toBe(CloudOperatorSubtype.AWS);
        expect(doc.system_info.is_cloud_operator).toBe(true);
    });

    it('forSlot() uses regular SystemInfo for non-cloud operators', () => {
        const doc = OperatorDocument.forSlot({
            operator_id: 'op-123',
            userId: 'user-456',
            namePrefix: 'operator',
            slotNumber: 1,
            operatorType: OperatorType.SYSTEM,
        });
        expect(doc.system_info.cloud_provider).toBeNull();
        expect(doc.system_info.is_cloud_operator).toBe(false);
    });

    it('forSlot() forces name to "g8ep" for g8ep operators', () => {
        const doc = OperatorDocument.forSlot({
            operator_id: 'op-123',
            userId: 'user-456',
            namePrefix: 'operator',
            slotNumber: 1,
            operatorType: OperatorType.CLOUD,
            cloudSubtype: CloudOperatorSubtype.G8E_POD,
            isG8eNode: true,
        });
        expect(doc.name).toBe('g8ep');
        expect(doc.is_g8ep).toBe(true);
    });

    it('forRefresh() creates operator from refresh data', () => {
        const certInfo = {
            cert: 'cert-data',
            key: 'key-data',
            serial: 'serial-123',
            not_before: new Date('2026-01-01T00:00:00.000Z'),
            not_after: new Date('2027-01-01T00:00:00.000Z'),
        };
        const doc = OperatorDocument.forRefresh({
            newOperatorId: 'op-new',
            userId: 'user-456',
            name: 'Refreshed Operator',
            slotNumber: 1,
            operatorType: OperatorType.SYSTEM,
            newApiKey: 'new-key',
            certInfo: certInfo,
            oldOperatorId: 'op-old',
            oldCertSerial: 'old-serial-456',
        });
        expect(doc.operator_id).toBe('op-new');
        expect(doc.api_key).toBe('new-key');
        expect(doc.operator_cert).toBe('cert-data');
        expect(doc.operator_cert_serial).toBe('serial-123');
        expect(doc.history_trail[0].event_type).toBe(HistoryEventType.CREATED_FROM_REFRESH);
        expect(doc.history_trail[0].details.predecessor_operator_id).toBe('op-old');
    });

    it('forRefresh() sets operator_cert_created_at when serial present', () => {
        const certInfo = {
            cert: 'cert-data',
            key: 'key-data',
            serial: 'serial-123',
            not_before: new Date(),
            not_after: new Date(),
        };
        const doc = OperatorDocument.forRefresh({
            newOperatorId: 'op-new',
            userId: 'user-456',
            name: 'Test',
            slotNumber: 1,
            certInfo: certInfo,
        });
        expect(doc.operator_cert_created_at).toBeInstanceOf(Date);
    });

    it('forReset() creates operator with reset history', () => {
        const doc = OperatorDocument.forReset({
            operator_id: 'op-123',
            user_id: 'user-456',
            name: 'Reset Operator',
            slot_number: 1,
            api_key: 'key-abc',
        });
        expect(doc.operator_id).toBe('op-123');
        expect(doc.status).toBe(OperatorStatus.AVAILABLE);
        expect(doc.history_trail[0].event_type).toBe(HistoryEventType.RESET);
        expect(doc.history_trail[0].details.reset_type).toBe('demo_reset');
    });
});

describe('OperatorListUpdatedEvent [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const event = OperatorListUpdatedEvent.parse({
            type: 'operator.list.updated',
        });
        expect(event.type).toBe('operator.list.updated');
        expect(event.operators).toEqual([]);
        expect(event.total_count).toBe(0);
        expect(event.active_count).toBe(0);
        expect(event.used_slots).toBe(0);
        expect(event.max_slots).toBe(0);
        expect(event.timestamp).toBeInstanceOf(Date);
    });

    it('accepts all fields with values', () => {
        const operators = [new OperatorSlot({ operator_id: 'op-1', status: 'ACTIVE' })];
        const event = OperatorListUpdatedEvent.parse({
            type: 'operator.list.updated',
            operators: operators,
            total_count: 10,
            active_count: 5,
            used_slots: 3,
            max_slots: 10,
        });
        expect(event.operators[0].operator_id).toBe('op-1');
        expect(event.operators[0].status).toBe('ACTIVE');
        expect(event.total_count).toBe(10);
        expect(event.active_count).toBe(5);
        expect(event.used_slots).toBe(3);
        expect(event.max_slots).toBe(10);
    });

    it('forWire() splits type from data fields', () => {
        const event = new OperatorListUpdatedEvent({
            type: 'operator.list.updated',
            operators: [{ id: 'op-1' }],
            total_count: 5,
        });
        const wire = event.forWire();
        expect(wire.type).toBe('operator.list.updated');
        expect(wire.data).toBeDefined();
        expect(wire.data.operators).toEqual([{ id: 'op-1' }]);
        expect(wire.data.total_count).toBe(5);
        expect(wire.operators).toBeUndefined();
        expect(wire.total_count).toBeUndefined();
    });
});

describe('GeneratedCertificate [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields', () => {
        const cert = GeneratedCertificate.parse({
            cert: '-----BEGIN CERT-----',
            key: '-----BEGIN KEY-----',
            serial: 'ABC123',
            not_before: new Date('2026-01-01T00:00:00.000Z'),
            not_after: new Date('2027-01-01T00:00:00.000Z'),
        });
        expect(cert.cert).toBe('-----BEGIN CERT-----');
        expect(cert.key).toBe('-----BEGIN KEY-----');
        expect(cert.serial).toBe('ABC123');
    });

    it('subject defaults to empty object', () => {
        const cert = new GeneratedCertificate({
            cert: 'cert',
            key: 'key',
            serial: 'serial',
            not_before: new Date(),
            not_after: new Date(),
        });
        expect(cert.subject).toEqual({});
    });

    it('forWire() converts snake_case to camelCase for dates', () => {
        const cert = new GeneratedCertificate({
            cert: 'cert',
            key: 'key',
            serial: 'serial',
            not_before: new Date('2026-01-01T00:00:00.000Z'),
            not_after: new Date('2027-01-01T00:00:00.000Z'),
        });
        const wire = cert.forWire();
        expect(wire.not_before).toBeUndefined();
        expect(wire.not_after).toBeUndefined();
        expect(typeof wire.notBefore).toBe('string');
        expect(typeof wire.notAfter).toBe('string');
        expect(wire.notBefore).toBe('2026-01-01T00:00:00.000Z');
        expect(wire.notAfter).toBe('2027-01-01T00:00:00.000Z');
    });
});

describe('CRLDocument [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const nextUpdate = new Date('2027-01-01T00:00:00.000Z');
        const crl = CRLDocument.parse({
            issuer: 'CN=Test CA',
            next_update: nextUpdate,
        });
        expect(crl.version).toBe(1);
        expect(crl.issuer).toBe('CN=Test CA');
        expect(crl.last_updated).toBeInstanceOf(Date);
        expect(crl.next_update).toBe(nextUpdate);
        expect(crl.revoked_certificates).toEqual([]);
        expect(crl.signature).toBeNull();
    });

    it('accepts all fields with values', () => {
        const crl = CRLDocument.parse({
            version: 2,
            issuer: 'CN=Test CA',
            next_update: new Date('2027-01-01T00:00:00.000Z'),
            revoked_certificates: ['serial-1', 'serial-2'],
            signature: 'sig-data',
        });
        expect(crl.version).toBe(2);
        expect(crl.revoked_certificates).toEqual(['serial-1', 'serial-2']);
        expect(crl.signature).toBe('sig-data');
    });

    it('last_updated defaults to now()', () => {
        const before = new Date();
        const crl = new CRLDocument({
            issuer: 'CN=Test CA',
            next_update: new Date('2027-01-01T00:00:00.000Z'),
        });
        const after = new Date();
        expect(crl.last_updated.getTime()).toBeGreaterThanOrEqual(before.getTime());
        expect(crl.last_updated.getTime()).toBeLessThanOrEqual(after.getTime());
    });
});

describe('OperatorSlotCreationResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const response = OperatorSlotCreationResponse.parse({
            success: true,
        });
        expect(response.success).toBe(true);
        expect(response.operator_id).toBeNull();
        expect(response.message).toBeNull();
    });

    it('forSuccess() creates success response with operator_id', () => {
        const response = OperatorSlotCreationResponse.forSuccess('op-123');
        expect(response.success).toBe(true);
        expect(response.operator_id).toBe('op-123');
        expect(response.message).toBeNull();
    });

    it('forFailure() creates failure response with message', () => {
        const response = OperatorSlotCreationResponse.forFailure('Slot limit reached');
        expect(response.success).toBe(false);
        expect(response.message).toBe('Slot limit reached');
        expect(response.operator_id).toBeNull();
    });
});

describe('OperatorRefreshKeyResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const response = OperatorRefreshKeyResponse.parse({
            success: true,
        });
        expect(response.success).toBe(true);
        expect(response.new_api_key).toBeNull();
        expect(response.new_operator_id).toBeNull();
        expect(response.message).toBeNull();
    });

    it('forSuccess() creates success response with new key and operator ID', () => {
        const response = OperatorRefreshKeyResponse.forSuccess('new-key-abc', 'op-new-456');
        expect(response.success).toBe(true);
        expect(response.new_api_key).toBe('new-key-abc');
        expect(response.new_operator_id).toBe('op-new-456');
    });

    it('forFailure() creates failure response with message', () => {
        const response = OperatorRefreshKeyResponse.forFailure('Refresh failed');
        expect(response.success).toBe(false);
        expect(response.message).toBe('Refresh failed');
    });
});

describe('BindOperatorsResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const response = BindOperatorsResponse.parse({
            success: true,
        });
        expect(response.success).toBe(true);
        expect(response.bound_count).toBe(0);
        expect(response.failed_count).toBe(0);
        expect(response.bound_operator_ids).toEqual([]);
        expect(response.failed_operator_ids).toEqual([]);
        expect(response.errors).toEqual([]);
        expect(response.statusCode).toBe(200);
        expect(response.error).toBeNull();
    });

    it('forSuccess() creates success response with bound IDs', () => {
        const boundIds = ['op-1', 'op-2', 'op-3'];
        const response = BindOperatorsResponse.forSuccess(boundIds);
        expect(response.success).toBe(true);
        expect(response.bound_count).toBe(3);
        expect(response.bound_operator_ids).toEqual(boundIds);
    });

    it('forSuccess() uses empty array when no IDs provided', () => {
        const response = BindOperatorsResponse.forSuccess();
        expect(response.bound_count).toBe(0);
        expect(response.bound_operator_ids).toEqual([]);
    });

    it('forFailure() creates failure response with error and status code', () => {
        const response = BindOperatorsResponse.forFailure('Invalid request', 400);
        expect(response.success).toBe(false);
        expect(response.error).toBe('Invalid request');
        expect(response.statusCode).toBe(400);
    });

    it('forFailure() defaults status code to 400', () => {
        const response = BindOperatorsResponse.forFailure('Server error');
        expect(response.statusCode).toBe(400);
    });
});

describe('UnbindOperatorsResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const response = UnbindOperatorsResponse.parse({
            success: true,
        });
        expect(response.success).toBe(true);
        expect(response.unbound_count).toBe(0);
        expect(response.failed_count).toBe(0);
        expect(response.unbound_operator_ids).toEqual([]);
        expect(response.failed_operator_ids).toEqual([]);
        expect(response.errors).toEqual([]);
        expect(response.statusCode).toBe(200);
        expect(response.error).toBeNull();
    });

    it('forSuccess() creates success response with unbound IDs', () => {
        const unboundIds = ['op-1', 'op-2'];
        const response = UnbindOperatorsResponse.forSuccess(unboundIds);
        expect(response.success).toBe(true);
        expect(response.unbound_count).toBe(2);
        expect(response.unbound_operator_ids).toEqual(unboundIds);
    });

    it('forSuccess() uses empty array when no IDs provided', () => {
        const response = UnbindOperatorsResponse.forSuccess();
        expect(response.unbound_count).toBe(0);
        expect(response.unbound_operator_ids).toEqual([]);
    });

    it('forFailure() creates failure response with error and status code', () => {
        const response = UnbindOperatorsResponse.forFailure('Not found', 404);
        expect(response.success).toBe(false);
        expect(response.error).toBe('Not found');
        expect(response.statusCode).toBe(404);
    });

    it('forFailure() defaults status code to 400', () => {
        const response = UnbindOperatorsResponse.forFailure('Bad request');
        expect(response.statusCode).toBe(400);
    });

    it('forClient() delegates to forWire()', () => {
        const response = new UnbindOperatorsResponse({
            success: true,
            unbound_operator_ids: ['op-1'],
        });
        const client = response.forClient();
        const wire = response.forWire();
        expect(client).toEqual(wire);
    });
});

describe('OperatorWithSessionContext [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const context = OperatorWithSessionContext.parse({
            operator_id: 'op-123',
            user_id: 'user-456',
            status: OperatorStatus.ACTIVE,
        });
        expect(context.operator_id).toBe('op-123');
        expect(context.user_id).toBe('user-456');
        expect(context.status).toBe(OperatorStatus.ACTIVE);
        expect(context.operator_session_id).toBeNull();
        expect(context.web_session_id).toBeNull();
        expect(context.system_info).toBeInstanceOf(SystemInfo);
        expect(context.organization_id).toBeNull();
        expect(context.case_id).toBeNull();
        expect(context.investigation_id).toBeNull();
        expect(context.task_id).toBeNull();
        expect(context.operator_type).toBeNull();
    });

    it('system_info defaults to empty SystemInfo', () => {
        const context = new OperatorWithSessionContext({
            operator_id: 'op-123',
            user_id: 'user-456',
            status: OperatorStatus.ACTIVE,
        });
        expect(context.system_info).toBeInstanceOf(SystemInfo);
        expect(context.system_info.hostname).toBeNull();
    });

    it('create() builds context from operator and sessions', () => {
        const operator = new OperatorDocument({
            operator_id: 'op-123',
            user_id: 'user-456',
            status: OperatorStatus.ACTIVE,
            operator_session_id: 'os-789',
            web_session_id: 'ws-101',
            system_info: new SystemInfo({ hostname: 'test-host' }),
            organization_id: 'org-111',
            case_id: 'case-222',
            investigation_id: 'inv-333',
            task_id: 'task-444',
            operator_type: OperatorType.CLOUD,
        });
        const operatorSession = { id: 'os-new-789' };
        const webSession = { id: 'ws-new-101' };

        const context = OperatorWithSessionContext.create(operator, operatorSession, webSession);
        expect(context.operator_id).toBe('op-123');
        expect(context.operator_session_id).toBe('os-new-789');
        expect(context.web_session_id).toBe('ws-new-101');
        expect(context.status).toBe(OperatorStatus.ACTIVE);
        expect(context.system_info.hostname).toBe('test-host');
        expect(context.user_id).toBe('user-456');
        expect(context.organization_id).toBe('org-111');
        expect(context.case_id).toBe('case-222');
        expect(context.investigation_id).toBe('inv-333');
        expect(context.task_id).toBe('task-444');
        expect(context.operator_type).toBe(OperatorType.CLOUD);
    });

    it('create() falls back to operator session IDs when sessions not provided', () => {
        const operator = new OperatorDocument({
            operator_id: 'op-123',
            user_id: 'user-456',
            status: OperatorStatus.ACTIVE,
            operator_session_id: 'os-789',
            web_session_id: 'ws-101',
        });
        const context = OperatorWithSessionContext.create(operator, null, null);
        expect(context.operator_session_id).toBe('os-789');
        expect(context.web_session_id).toBe('ws-101');
    });

    it('create() handles missing operator session ID', () => {
        const operator = new OperatorDocument({
            operator_id: 'op-123',
            user_id: 'user-456',
            status: OperatorStatus.ACTIVE,
        });
        const operatorSession = { id: 'os-789' };
        const context = OperatorWithSessionContext.create(operator, operatorSession, null);
        expect(context.operator_session_id).toBe('os-789');
    });

    it('forWire() delegates to forDB()', () => {
        const context = new OperatorWithSessionContext({
            operator_id: 'op-123',
            user_id: 'user-456',
            status: OperatorStatus.ACTIVE,
        });
        const wire = context.forWire();
        const db = context.forDB();
        expect(wire).toEqual(db);
    });
});
