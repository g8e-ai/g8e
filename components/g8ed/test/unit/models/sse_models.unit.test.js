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
    ConnectionEstablishedEvent,
    KeepaliveEvent,
    LLMConfigData,
    LLMConfigEvent,
    InvestigationListData,
    InvestigationListEvent,
    HeartbeatSSEEvent,
    AuditDownloadResponse,
    OperatorStatusUpdatedData,
    OperatorStatusUpdatedEvent,
    OperatorPanelListUpdatedData,
    OperatorPanelListUpdatedEvent,
    CommandResultSSEEvent,
    ApprovalResponseEvent,
    DirectCommandResponseEvent,
    LogStreamEvent,
    LogStreamConnectedEvent,
    G8eePassthroughEvent,
} from '@g8ed/models/sse_models.js';

describe('ConnectionEstablishedEvent [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields', () => {
        const event = ConnectionEstablishedEvent.parse({
            type: 'connection.established',
            connectionId: 'conn-123',
        });
        expect(event.type).toBe('connection.established');
        expect(event.connectionId).toBe('conn-123');
        expect(event.timestamp).toBeInstanceOf(Date);
    });

    it('throws when type is missing', () => {
        expect(() => ConnectionEstablishedEvent.parse({ connectionId: 'conn-123' }))
            .toThrow('type is required');
    });

    it('throws when connectionId is missing', () => {
        expect(() => ConnectionEstablishedEvent.parse({ type: 'connection.established' }))
            .toThrow('connectionId is required');
    });

    it('accepts a custom timestamp', () => {
        const customTs = new Date('2026-01-01T00:00:00.000Z');
        const event = ConnectionEstablishedEvent.parse({
            type: 'connection.established',
            connectionId: 'conn-123',
            timestamp: customTs,
        });
        expect(event.timestamp).toBe(customTs);
    });

    it('forWire() serializes timestamp to ISO string', () => {
        const event = ConnectionEstablishedEvent.parse({
            type: 'connection.established',
            connectionId: 'conn-123',
        });
        const wire = event.forWire();
        expect(typeof wire.timestamp).toBe('string');
        expect(wire.type).toBe('connection.established');
        expect(wire.connectionId).toBe('conn-123');
    });
});

describe('KeepaliveEvent [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields', () => {
        const event = KeepaliveEvent.parse({ type: 'keepalive' });
        expect(event.type).toBe('keepalive');
        expect(event.timestamp).toBeInstanceOf(Date);
        expect(event.serverTime).toBeNull();
    });

    it('defaults serverTime to null when not provided', () => {
        const event = KeepaliveEvent.parse({ type: 'keepalive' });
        expect(event.serverTime).toBeNull();
    });

    it('accepts serverTime when provided', () => {
        const event = KeepaliveEvent.parse({
            type: 'keepalive',
            serverTime: 1234567890,
        });
        expect(event.serverTime).toBe(1234567890);
    });

    it('throws when type is missing', () => {
        expect(() => KeepaliveEvent.parse({}))
            .toThrow('type is required');
    });

    it('forWire() includes all fields', () => {
        const event = KeepaliveEvent.parse({
            type: 'keepalive',
            serverTime: 1234567890,
        });
        const wire = event.forWire();
        expect(wire.type).toBe('keepalive');
        expect(wire.serverTime).toBe(1234567890);
        expect(typeof wire.timestamp).toBe('string');
    });
});

describe('LLMConfigData [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const data = LLMConfigData.parse({
            provider: 'gemini',
        });
        expect(data.provider).toBe('gemini');
        expect(data.default_primary_model).toBe('');
        expect(data.default_assistant_model).toBe('');
        expect(data.primary_models).toEqual([]);
        expect(data.assistant_models).toEqual([]);
        expect(data.timestamp).toBeInstanceOf(Date);
    });

    it('accepts all fields with values', () => {
        const data = LLMConfigData.parse({
            provider: 'gemini',
            default_primary_model: 'gemini-2.5-pro',
            default_assistant_model: 'gemini-2.5-flash',
            primary_models: ['gemini-2.5-pro', 'gemini-2.5-flash'],
            assistant_models: ['gemini-2.5-flash', 'gemini-2.5-flash-lite'],
        });
        expect(data.provider).toBe('gemini');
        expect(data.default_primary_model).toBe('gemini-2.5-pro');
        expect(data.default_assistant_model).toBe('gemini-2.5-flash');
        expect(data.primary_models).toEqual(['gemini-2.5-pro', 'gemini-2.5-flash']);
        expect(data.assistant_models).toEqual(['gemini-2.5-flash', 'gemini-2.5-flash-lite']);
    });

    it('throws when provider is missing', () => {
        expect(() => LLMConfigData.parse({}))
            .toThrow('provider is required');
    });

    it('forWire() serializes timestamp to ISO string', () => {
        const data = LLMConfigData.parse({ provider: 'gemini' });
        const wire = data.forWire();
        expect(typeof wire.timestamp).toBe('string');
    });
});

describe('LLMConfigEvent [UNIT - PURE LOGIC]', () => {
    it('accepts valid event with nested LLMConfigData', () => {
        const event = LLMConfigEvent.parse({
            type: 'llm.config',
            data: {
                provider: 'gemini',
                default_primary_model: 'gemini-2.5-pro',
            },
        });
        expect(event.type).toBe('llm.config');
        expect(event.data).toBeInstanceOf(LLMConfigData);
        expect(event.data.provider).toBe('gemini');
        expect(event.data.default_primary_model).toBe('gemini-2.5-pro');
    });

    it('defaults data to null when not provided', () => {
        const event = LLMConfigEvent.parse({ type: 'llm.config' });
        expect(event.data).toBeNull();
    });

    it('throws when type is missing', () => {
        expect(() => LLMConfigEvent.parse({}))
            .toThrow('type is required');
    });

    it('parses nested data via LLMConfigData.parse()', () => {
        const event = LLMConfigEvent.parse({
            type: 'llm.config',
            data: {
                provider: 'openai',
                primary_models: ['gpt-4o', 'gpt-4o-mini'],
            },
        });
        expect(event.data.primary_models).toEqual(['gpt-4o', 'gpt-4o-mini']);
    });

    it('forWire() serializes nested model to plain object', () => {
        const event = LLMConfigEvent.parse({
            type: 'llm.config',
            data: { provider: 'gemini' },
        });
        const wire = event.forWire();
        expect(wire.data instanceof LLMConfigData).toBe(false);
        expect(typeof wire.data).toBe('object');
        expect(wire.data.provider).toBe('gemini');
    });
});

describe('InvestigationListData [UNIT - PURE LOGIC]', () => {
    it('accepts valid data with defaults', () => {
        const data = InvestigationListData.parse({});
        expect(data.investigations).toEqual([]);
        expect(data.count).toBe(0);
        expect(data.timestamp).toBeInstanceOf(Date);
    });

    it('accepts all fields with values', () => {
        const investigations = [
            { id: 'inv-1', title: 'Test 1' },
            { id: 'inv-2', title: 'Test 2' },
        ];
        const data = InvestigationListData.parse({
            investigations,
            count: 2,
        });
        expect(data.investigations).toEqual(investigations);
        expect(data.count).toBe(2);
    });

    it('forWire() serializes timestamp to ISO string', () => {
        const data = InvestigationListData.parse({
            investigations: [],
            count: 0,
        });
        const wire = data.forWire();
        expect(typeof wire.timestamp).toBe('string');
    });
});

describe('InvestigationListEvent [UNIT - PURE LOGIC]', () => {
    it('accepts valid event with nested InvestigationListData', () => {
        const event = InvestigationListEvent.parse({
            type: 'investigation.list',
            data: {
                investigations: [{ id: 'inv-1' }],
                count: 1,
            },
        });
        expect(event.type).toBe('investigation.list');
        expect(event.data).toBeInstanceOf(InvestigationListData);
        expect(event.data.count).toBe(1);
    });

    it('defaults data to null when not provided', () => {
        const event = InvestigationListEvent.parse({ type: 'investigation.list' });
        expect(event.data).toBeNull();
    });

    it('throws when type is missing', () => {
        expect(() => InvestigationListEvent.parse({}))
            .toThrow('type is required');
    });

    it('forWire() serializes nested model to plain object', () => {
        const event = InvestigationListEvent.parse({
            type: 'investigation.list',
            data: { investigations: [], count: 0 },
        });
        const wire = event.forWire();
        expect(wire.data instanceof InvestigationListData).toBe(false);
        expect(typeof wire.data).toBe('object');
    });
});

describe('HeartbeatSSEEvent [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields', () => {
        const event = HeartbeatSSEEvent.parse({
            type: 'operator.heartbeat',
            operator_id: 'op-123',
        });
        expect(event.type).toBe('operator.heartbeat');
        expect(event.operator_id).toBe('op-123');
        expect(event.data).toBeNull();
        expect(event.timestamp).toBeInstanceOf(Date);
    });

    it('accepts data field with any value', () => {
        const heartbeatData = { status: 'active', uptime: 3600 };
        const event = HeartbeatSSEEvent.parse({
            type: 'operator.heartbeat',
            operator_id: 'op-123',
            data: heartbeatData,
        });
        expect(event.data).toEqual(heartbeatData);
    });

    it('throws when type is missing', () => {
        expect(() => HeartbeatSSEEvent.parse({ operator_id: 'op-123' }))
            .toThrow('type is required');
    });

    it('throws when operator_id is missing', () => {
        expect(() => HeartbeatSSEEvent.parse({ type: 'operator.heartbeat' }))
            .toThrow('operator_id is required');
    });

    it('forWire() serializes timestamp to ISO string', () => {
        const event = HeartbeatSSEEvent.parse({
            type: 'operator.heartbeat',
            operator_id: 'op-123',
        });
        const wire = event.forWire();
        expect(typeof wire.timestamp).toBe('string');
    });
});

describe('AuditDownloadResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const response = AuditDownloadResponse.parse({
            user_id: 'user-123',
        });
        expect(response.user_id).toBe('user-123');
        expect(response.total_events).toBe(0);
        expect(response.total_investigations).toBe(0);
        expect(response.filters).toEqual({});
        expect(response.events).toEqual([]);
        expect(response.exported_at).toBeInstanceOf(Date);
    });

    it('accepts all fields with values', () => {
        const events = [{ id: 'evt-1', type: 'test' }];
        const response = AuditDownloadResponse.parse({
            user_id: 'user-123',
            total_events: 100,
            total_investigations: 5,
            filters: { date_range: '2026-01-01' },
            events,
        });
        expect(response.total_events).toBe(100);
        expect(response.total_investigations).toBe(5);
        expect(response.filters).toEqual({ date_range: '2026-01-01' });
        expect(response.events).toEqual(events);
    });

    it('throws when user_id is missing', () => {
        expect(() => AuditDownloadResponse.parse({}))
            .toThrow('user_id is required');
    });

    it('forWire() serializes timestamp to ISO string', () => {
        const response = AuditDownloadResponse.parse({ user_id: 'user-123' });
        const wire = response.forWire();
        expect(typeof wire.exported_at).toBe('string');
    });
});

describe('OperatorStatusUpdatedData [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const data = OperatorStatusUpdatedData.parse({
            operator_id: 'op-123',
            status: 'ACTIVE',
        });
        expect(data.operator_id).toBe('op-123');
        expect(data.status).toBe('ACTIVE');
        expect(data.hostname).toBeNull();
        expect(data.system_fingerprint).toBeNull();
        expect(data.system_info).toBeNull();
        expect(data.operator_data).toBeNull();
        expect(data.reason).toBeNull();
        expect(data.total_count).toBeNull();
        expect(data.active_count).toBeNull();
        expect(data.timestamp).toBeNull();
    });

    it('accepts all fields with values', () => {
        const systemInfo = { os: 'linux', arch: 'amd64' };
        const data = OperatorStatusUpdatedData.parse({
            operator_id: 'op-123',
            status: 'ACTIVE',
            hostname: 'host-1',
            system_fingerprint: 'fp-abc123',
            system_info: systemInfo,
            operator_data: { uptime: 3600 },
            reason: 'User request',
            total_count: 10,
            active_count: 5,
            timestamp: new Date('2026-01-01T00:00:00.000Z'),
        });
        expect(data.hostname).toBe('host-1');
        expect(data.system_fingerprint).toBe('fp-abc123');
        expect(data.system_info).toEqual(systemInfo);
        expect(data.operator_data).toEqual({ uptime: 3600 });
        expect(data.reason).toBe('User request');
        expect(data.total_count).toBe(10);
        expect(data.active_count).toBe(5);
        expect(data.timestamp).toBeInstanceOf(Date);
    });

    it('throws when operator_id is missing', () => {
        expect(() => OperatorStatusUpdatedData.parse({ status: 'ACTIVE' }))
            .toThrow('operator_id is required');
    });

    it('throws when status is missing', () => {
        expect(() => OperatorStatusUpdatedData.parse({ operator_id: 'op-123' }))
            .toThrow('status is required');
    });
});

describe('OperatorStatusUpdatedEvent [UNIT - PURE LOGIC]', () => {
    it('accepts valid event with nested OperatorStatusUpdatedData', () => {
        const event = OperatorStatusUpdatedEvent.parse({
            type: 'g8e.v1.operator.status.updated',
            data: {
                operator_id: 'op-123',
                status: 'ACTIVE',
            },
        });
        expect(event.type).toBe('g8e.v1.operator.status.updated');
        expect(event.data).toBeInstanceOf(OperatorStatusUpdatedData);
        expect(event.data.operator_id).toBe('op-123');
        expect(event.data.status).toBe('ACTIVE');
    });

    it('defaults data to null when not provided', () => {
        const event = OperatorStatusUpdatedEvent.parse({ type: 'g8e.v1.operator.status.updated' });
        expect(event.data).toBeNull();
    });

    it('throws when type is missing', () => {
        expect(() => OperatorStatusUpdatedEvent.parse({}))
            .toThrow('type is required');
    });

    it('forWire() serializes nested model to plain object', () => {
        const event = OperatorStatusUpdatedEvent.parse({
            type: 'g8e.v1.operator.status.updated',
            data: { operator_id: 'op-123', status: 'ACTIVE' },
        });
        const wire = event.forWire();
        expect(wire.data instanceof OperatorStatusUpdatedData).toBe(false);
        expect(typeof wire.data).toBe('object');
        expect(wire.data.operator_id).toBe('op-123');
    });
});

describe('OperatorPanelListUpdatedData [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const data = OperatorPanelListUpdatedData.parse({
            operator_id: 'op-123',
        });
        expect(data.operator_id).toBe('op-123');
        expect(data.case_id).toBeNull();
        expect(data.investigation_id).toBeNull();
        expect(data.task_id).toBeNull();
        expect(data.timestamp).toBeNull();
    });

    it('accepts all fields with values', () => {
        const data = OperatorPanelListUpdatedData.parse({
            operator_id: 'op-123',
            case_id: 'case-1',
            investigation_id: 'inv-1',
            task_id: 'task-1',
            timestamp: new Date('2026-01-01T00:00:00.000Z'),
        });
        expect(data.case_id).toBe('case-1');
        expect(data.investigation_id).toBe('inv-1');
        expect(data.task_id).toBe('task-1');
        expect(data.timestamp).toBeInstanceOf(Date);
    });

    it('throws when operator_id is missing', () => {
        expect(() => OperatorPanelListUpdatedData.parse({}))
            .toThrow('operator_id is required');
    });

    it('timestamp defaults to null when not provided', () => {
        const data = new OperatorPanelListUpdatedData({ operator_id: 'op-123' });
        expect(data.timestamp).toBeNull();
    });
});

describe('OperatorPanelListUpdatedEvent [UNIT - PURE LOGIC]', () => {
    it('accepts valid event with nested OperatorPanelListUpdatedData', () => {
        const event = OperatorPanelListUpdatedEvent.parse({
            type: 'g8e.v1.operator.panel.list.updated',
            data: {
                operator_id: 'op-123',
                case_id: 'case-1',
            },
        });
        expect(event.type).toBe('g8e.v1.operator.panel.list.updated');
        expect(event.data).toBeInstanceOf(OperatorPanelListUpdatedData);
        expect(event.data.operator_id).toBe('op-123');
        expect(event.data.case_id).toBe('case-1');
    });

    it('defaults data to null when not provided', () => {
        const event = OperatorPanelListUpdatedEvent.parse({ type: 'g8e.v1.operator.panel.list.updated' });
        expect(event.data).toBeNull();
    });

    it('throws when type is missing', () => {
        expect(() => OperatorPanelListUpdatedEvent.parse({}))
            .toThrow('type is required');
    });

    it('timestamp defaults to now() when not provided', () => {
        const before = new Date();
        const event = new OperatorPanelListUpdatedEvent({
            type: 'g8e.v1.operator.panel.list.updated',
            data: { operator_id: 'op-123' },
        });
        const after = new Date();
        expect(event.timestamp).toBeInstanceOf(Date);
        expect(event.timestamp.getTime()).toBeGreaterThanOrEqual(before.getTime());
        expect(event.timestamp.getTime()).toBeLessThanOrEqual(after.getTime());
    });

    it('forWire() serializes nested model to plain object', () => {
        const event = OperatorPanelListUpdatedEvent.parse({
            type: 'g8e.v1.operator.panel.list.updated',
            data: { operator_id: 'op-123' },
        });
        const wire = event.forWire();
        expect(wire.data instanceof OperatorPanelListUpdatedData).toBe(false);
        expect(typeof wire.data).toBe('object');
    });
});

describe('CommandResultSSEEvent [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const event = CommandResultSSEEvent.parse({
            type: 'g8e.v1.operator.command.completed',
            execution_id: 'cmd-123',
            status: 'COMPLETED',
        });
        expect(event.type).toBe('g8e.v1.operator.command.completed');
        expect(event.execution_id).toBe('cmd-123');
        expect(event.status).toBe('COMPLETED');
        expect(event.command).toBeNull();
        expect(event.output).toBeNull();
        expect(event.error).toBeNull();
        expect(event.stderr).toBeNull();
        expect(event.exit_code).toBeNull();
        expect(event.return_code).toBeNull();
        expect(event.execution_time_seconds).toBe(0);
        expect(event.web_session_id).toBeNull();
        expect(event.operator_session_id).toBeNull();
        expect(event.operator_id).toBeNull();
        expect(event.hostname).toBeNull();
        expect(event.case_id).toBeNull();
        expect(event.investigation_id).toBeNull();
        expect(event.direct_execution).toBe(false);
        expect(event.approval_id).toBeNull();
        expect(event.timestamp).toBeInstanceOf(Date);
    });

    it('accepts all fields with values', () => {
        const event = CommandResultSSEEvent.parse({
            type: 'operator.command.completed',
            execution_id: 'cmd-123',
            status: 'COMPLETED',
            command: 'ls -la',
            output: 'file1\nfile2',
            error: null,
            stderr: '',
            exit_code: 0,
            return_code: 0,
            execution_time_seconds: 1.5,
            web_session_id: 'ws-123',
            operator_session_id: 'os-123',
            operator_id: 'op-123',
            hostname: 'host-1',
            case_id: 'case-1',
            investigation_id: 'inv-1',
            direct_execution: true,
            approval_id: 'apr-123',
        });
        expect(event.command).toBe('ls -la');
        expect(event.output).toBe('file1\nfile2');
        expect(event.execution_time_seconds).toBe(1.5);
        expect(event.direct_execution).toBe(true);
        expect(event.approval_id).toBe('apr-123');
    });

    it('throws when type is missing', () => {
        expect(() => CommandResultSSEEvent.parse({ execution_id: 'cmd-123', status: 'COMPLETED' }))
            .toThrow('type is required');
    });

    it('throws when execution_id is missing', () => {
        expect(() => CommandResultSSEEvent.parse({ type: 'operator.command.completed', status: 'COMPLETED' }))
            .toThrow('execution_id is required');
    });

    it('throws when status is missing', () => {
        expect(() => CommandResultSSEEvent.parse({ type: 'operator.command.completed', execution_id: 'cmd-123' }))
            .toThrow('status is required');
    });

    it('forWire() splits type from data fields', () => {
        const event = CommandResultSSEEvent.parse({
            type: 'operator.command.completed',
            execution_id: 'cmd-123',
            status: 'COMPLETED',
            command: 'ls -la',
        });
        const wire = event.forWire();
        expect(wire.type).toBe('operator.command.completed');
        expect(wire.data).toBeDefined();
        expect(wire.data.execution_id).toBe('cmd-123');
        expect(wire.data.status).toBe('COMPLETED');
        expect(wire.data.command).toBe('ls -la');
        expect(wire.data.timestamp).toBeDefined();
        expect(typeof wire.data.timestamp).toBe('string');
        expect(wire.execution_id).toBeUndefined();
        expect(wire.status).toBeUndefined();
    });

    it('forWire() includes all data fields in data object', () => {
        const event = CommandResultSSEEvent.parse({
            type: 'operator.command.completed',
            execution_id: 'cmd-123',
            status: 'COMPLETED',
            output: 'test output',
            error: 'error message',
            exit_code: 1,
            execution_time_seconds: 2.5,
            direct_execution: true,
        });
        const wire = event.forWire();
        expect(wire.data.output).toBe('test output');
        expect(wire.data.error).toBe('error message');
        expect(wire.data.exit_code).toBe(1);
        expect(wire.data.execution_time_seconds).toBe(2.5);
        expect(wire.data.direct_execution).toBe(true);
    });
});

describe('ApprovalResponseEvent [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields', () => {
        const event = ApprovalResponseEvent.parse({
            success: true,
            approval_id: 'apr-123',
            approved: true,
        });
        expect(event.success).toBe(true);
        expect(event.approval_id).toBe('apr-123');
        expect(event.approved).toBe(true);
        expect(event.timestamp).toBeInstanceOf(Date);
    });

    it('accepts rejection response', () => {
        const event = ApprovalResponseEvent.parse({
            success: true,
            approval_id: 'apr-123',
            approved: false,
        });
        expect(event.approved).toBe(false);
    });

    it('throws when success is missing', () => {
        expect(() => ApprovalResponseEvent.parse({ approval_id: 'apr-123', approved: true }))
            .toThrow('success is required');
    });

    it('throws when approval_id is missing', () => {
        expect(() => ApprovalResponseEvent.parse({ success: true, approved: true }))
            .toThrow('approval_id is required');
    });

    it('throws when approved is missing', () => {
        expect(() => ApprovalResponseEvent.parse({ success: true, approval_id: 'apr-123' }))
            .toThrow('approved is required');
    });

    it('timestamp defaults to now() when not provided', () => {
        const before = new Date();
        const event = new ApprovalResponseEvent({
            success: true,
            approval_id: 'apr-123',
            approved: true,
        });
        const after = new Date();
        expect(event.timestamp).toBeInstanceOf(Date);
        expect(event.timestamp.getTime()).toBeGreaterThanOrEqual(before.getTime());
        expect(event.timestamp.getTime()).toBeLessThanOrEqual(after.getTime());
    });

    it('forWire() serializes timestamp to ISO string', () => {
        const event = ApprovalResponseEvent.parse({
            success: true,
            approval_id: 'apr-123',
            approved: true,
        });
        const wire = event.forWire();
        expect(typeof wire.timestamp).toBe('string');
    });
});

describe('DirectCommandResponseEvent [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with default message', () => {
        const event = DirectCommandResponseEvent.parse({
            success: true,
            execution_id: 'cmd-123',
        });
        expect(event.success).toBe(true);
        expect(event.execution_id).toBe('cmd-123');
        expect(event.message).toBe('Command sent to operator');
        expect(event.timestamp).toBeInstanceOf(Date);
    });

    it('accepts custom message', () => {
        const event = DirectCommandResponseEvent.parse({
            success: true,
            execution_id: 'cmd-123',
            message: 'Custom message',
        });
        expect(event.message).toBe('Custom message');
    });

    it('throws when success is missing', () => {
        expect(() => DirectCommandResponseEvent.parse({ execution_id: 'cmd-123' }))
            .toThrow('success is required');
    });

    it('throws when execution_id is missing', () => {
        expect(() => DirectCommandResponseEvent.parse({ success: true }))
            .toThrow('execution_id is required');
    });

    it('message defaults to "Command sent to operator" when not provided', () => {
        const event = new DirectCommandResponseEvent({
            success: true,
            execution_id: 'cmd-123',
        });
        expect(event.message).toBe('Command sent to operator');
    });

    it('forWire() serializes timestamp to ISO string', () => {
        const event = DirectCommandResponseEvent.parse({
            success: true,
            execution_id: 'cmd-123',
        });
        const wire = event.forWire();
        expect(typeof wire.timestamp).toBe('string');
    });
});

describe('LogStreamEvent [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with default data', () => {
        const event = LogStreamEvent.parse({ type: 'console.log.entry' });
        expect(event.type).toBe('console.log.entry');
        expect(event.entry).toBeNull();
    });

    it('accepts entry with any value', () => {
        const entry = { level: 'info', message: 'test log', timestamp: '2026-01-01T00:00:00.000Z' };
        const event = LogStreamEvent.parse({
            type: 'console.log.entry',
            entry,
        });
        expect(event.entry).toEqual(entry);
    });

    it('throws when type is missing', () => {
        expect(() => LogStreamEvent.parse({}))
            .toThrow('type is required');
    });

    it('forWire() preserves entry structure', () => {
        const entry = { level: 'error', message: 'error occurred' };
        const event = LogStreamEvent.parse({
            type: 'console.log.entry',
            entry,
        });
        const wire = event.forWire();
        expect(wire.entry).toEqual(entry);
    });
});

describe('LogStreamConnectedEvent [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const event = LogStreamConnectedEvent.parse({ type: 'console.log.connected' });
        expect(event.type).toBe('console.log.connected');
        expect(event.buffered).toBe(0);
        expect(event.timestamp).toBeInstanceOf(Date);
    });

    it('accepts buffered count', () => {
        const event = LogStreamConnectedEvent.parse({
            type: 'console.log.connected',
            buffered: 100,
        });
        expect(event.buffered).toBe(100);
    });

    it('throws when type is missing', () => {
        expect(() => LogStreamConnectedEvent.parse({}))
            .toThrow('type is required');
    });

    it('timestamp defaults to now() when not provided', () => {
        const before = new Date();
        const event = new LogStreamConnectedEvent({ type: 'console.log.connected' });
        const after = new Date();
        expect(event.timestamp).toBeInstanceOf(Date);
        expect(event.timestamp.getTime()).toBeGreaterThanOrEqual(before.getTime());
        expect(event.timestamp.getTime()).toBeLessThanOrEqual(after.getTime());
    });

    it('forWire() serializes timestamp to ISO string', () => {
        const event = LogStreamConnectedEvent.parse({ type: 'console.log.connected' });
        const wire = event.forWire();
        expect(typeof wire.timestamp).toBe('string');
    });
});

describe('G8eePassthroughEvent [UNIT - PURE LOGIC]', () => {
    it('accepts valid payload with type field', () => {
        const payload = { type: 'llm.chat.iteration', data: { content: 'test' } };
        const event = G8eePassthroughEvent.parse({ _payload: payload });
        expect(event._payload).toEqual(payload);
    });

    it('throws when _payload is missing', () => {
        expect(() => G8eePassthroughEvent.parse({}))
            .toThrow('_payload is required');
    });

    it('throws when _payload is not an object', () => {
        expect(() => G8eePassthroughEvent.parse({ _payload: 'not-an-object' }))
            .toThrow('_payload must be a plain object');
    });

    it('throws when _payload is null', () => {
        expect(() => G8eePassthroughEvent.parse({ _payload: null }))
            .toThrow('_payload is required');
    });

    it('throws when _payload.type is missing', () => {
        expect(() => G8eePassthroughEvent.parse({ _payload: { data: 'test' } }))
            .toThrow('_payload.type must be a non-empty string');
    });

    it('throws when _payload.type is not a string', () => {
        expect(() => G8eePassthroughEvent.parse({ _payload: { type: 123 } }))
            .toThrow('_payload.type must be a non-empty string');
    });

    it('throws when _payload.type is an empty string', () => {
        expect(() => G8eePassthroughEvent.parse({ _payload: { type: '' } }))
            .toThrow('_payload.type must be a non-empty string');
    });

    it('throws when _payload.type is only whitespace', () => {
        expect(() => G8eePassthroughEvent.parse({ _payload: { type: '   ' } }))
            .toThrow('_payload.type must be a non-empty string');
    });

    it('accepts _payload.type with value', () => {
        const event = G8eePassthroughEvent.parse({
            _payload: { type: 'llm.chat.iteration' },
        });
        expect(event._payload.type).toBe('llm.chat.iteration');
    });

    it('forWire() returns the inner payload directly', () => {
        const payload = { type: 'llm.chat.iteration', data: { chunk: 'hello' } };
        const event = G8eePassthroughEvent.parse({ _payload: payload });
        const wire = event.forWire();
        expect(wire).toBe(payload);
        expect(wire).not.toBe(event);
    });

    it('forWire() preserves the original payload structure', () => {
        const payload = {
            type: 'llm.chat.iteration',
            data: { chunk: 'test', done: false },
            metadata: { model: 'gemini-2.5-pro' },
        };
        const event = G8eePassthroughEvent.parse({ _payload: payload });
        const wire = event.forWire();
        expect(wire.type).toBe('llm.chat.iteration');
        expect(wire.data.chunk).toBe('test');
        expect(wire.data.done).toBe(false);
        expect(wire.metadata.model).toBe('gemini-2.5-pro');
    });

    it('constructor validates _payload', () => {
        expect(() => new G8eePassthroughEvent({ _payload: { type: 'valid' } }))
            .not.toThrow();
        expect(() => new G8eePassthroughEvent({ _payload: {} }))
            .toThrow('_payload.type must be a non-empty string');
    });
});
