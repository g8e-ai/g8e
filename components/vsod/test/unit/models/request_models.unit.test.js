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
    VSOHttpContext,
    ChatMessageRequest,
    InvestigationQueryRequest,
    SessionCreateRequest,
    ApprovalRespondRequest,
    IntentRequest,
    UnlockAccountRequest,
    SSEPushRequest,
    IntentRequestPayload,
    DirectCommandRequest,
    CreateOperatorRequest,
    PasskeyRegisterChallengeRequest,
    AttestationResponseJSON,
    AssertionResponseJSON,
    PasskeyRegisterVerifyRequest,
    PasskeyAuthChallengeRequest,
    PasskeyAuthVerifyRequest,
    CreateDeviceLinkRequest,
    GenerateDeviceLinkRequest,
    RegisterDeviceRequest,
    BindOperatorsRequest,
    UnbindOperatorsRequest,
    OperatorSessionRegistrationRequest,
    StopOperatorRequest,
    OperatorRegisterSessionRequest,
    SettingsUpdateRequest,
    RefreshOperatorKeyRequest,
    InitializeOperatorSlotsRequest,
    CreateUserRequest,
    UpdateUserRolesRequest,
    StopAIRequest,
    SessionAuthResponse,
    BoundOperatorContext,
    RequestModelFactory,
} from '@vsod/models/request_models.js';

describe('VSOHttpContext [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const ctx = VSOHttpContext.parse({
            web_session_id: 'ws-123',
            user_id: 'user-456',
        });
        expect(ctx.web_session_id).toBe('ws-123');
        expect(ctx.user_id).toBe('user-456');
        expect(ctx.organization_id).toBeNull();
        expect(ctx.case_id).toBeNull();
        expect(ctx.investigation_id).toBeNull();
        expect(ctx.task_id).toBeNull();
        expect(ctx.bound_operators).toEqual([]);
        expect(ctx.execution_id).toBeNull();
        expect(ctx.source_component).toBe('vsod');
    });

    it('accepts all fields with values', () => {
        const boundOp = {
            operator_id: 'op-1',
            operator_session_id: 'ops-1',
        };
        const ctx = VSOHttpContext.parse({
            web_session_id: 'ws-123',
            user_id: 'user-456',
            organization_id: 'org-789',
            case_id: 'case-abc',
            investigation_id: 'inv-def',
            task_id: 'task-ghi',
            bound_operators: [boundOp],
            execution_id: 'exec-jkl',
            source_component: 'g8ee',
        });
        expect(ctx.organization_id).toBe('org-789');
        expect(ctx.case_id).toBe('case-abc');
        expect(ctx.investigation_id).toBe('inv-def');
        expect(ctx.task_id).toBe('task-ghi');
        expect(ctx.bound_operators).toEqual([boundOp]);
        expect(ctx.execution_id).toBe('exec-jkl');
        expect(ctx.source_component).toBe('g8ee');
    });

    it('converts empty string case_id to null for new case signal', () => {
        const ctx = VSOHttpContext.parse({
            web_session_id: 'ws-123',
            user_id: 'user-456',
            case_id: '',
        });
        expect(ctx.case_id).toBeNull();
    });

    it('throws when web_session_id is missing', () => {
        expect(() => VSOHttpContext.parse({ user_id: 'user-456' }))
            .toThrow('web_session_id is required');
    });

    it('throws when user_id is missing', () => {
        expect(() => VSOHttpContext.parse({ web_session_id: 'ws-123' }))
            .toThrow('user_id is required');
    });

    it('forWire() serializes correctly', () => {
        const ctx = VSOHttpContext.parse({
            web_session_id: 'ws-123',
            user_id: 'user-456',
        });
        const wire = ctx.forWire();
        expect(wire.web_session_id).toBe('ws-123');
        expect(wire.user_id).toBe('user-456');
    });
});

describe('ChatMessageRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const req = ChatMessageRequest.parse({
            web_session_id: 'ws-123',
            user_id: 'user-456',
            message: 'test message',
        });
        expect(req.web_session_id).toBe('ws-123');
        expect(req.user_id).toBe('user-456');
        expect(req.message).toBe('test message');
        expect(req.attachments).toEqual([]);
        expect(req.sentinel_mode).toBe(true);
        expect(req.llm_primary_model).toBeNull();
        expect(req.llm_assistant_model).toBeNull();
        expect(req.case_id).toBeNull();
        expect(req.investigation_id).toBeNull();
    });

    it('accepts all fields with values', () => {
        const attachments = [{ name: 'test.txt', content: 'data' }];
        const req = ChatMessageRequest.parse({
            web_session_id: 'ws-123',
            user_id: 'user-456',
            message: 'test message',
            attachments: attachments,
            sentinel_mode: false,
            llm_primary_model: 'gemini-2.5-pro',
            llm_assistant_model: 'gemini-2.5-flash',
            case_id: 'case-abc',
            investigation_id: 'inv-def',
        });
        expect(req.attachments).toEqual(attachments);
        expect(req.sentinel_mode).toBe(false);
        expect(req.llm_primary_model).toBe('gemini-2.5-pro');
        expect(req.llm_assistant_model).toBe('gemini-2.5-flash');
        expect(req.case_id).toBe('case-abc');
        expect(req.investigation_id).toBe('inv-def');
    });

    it('throws when web_session_id is missing', () => {
        expect(() => ChatMessageRequest.parse({
            user_id: 'user-456',
            message: 'test',
        })).toThrow('web_session_id is required');
    });

    it('throws when user_id is missing', () => {
        expect(() => ChatMessageRequest.parse({
            web_session_id: 'ws-123',
            message: 'test',
        })).toThrow('user_id is required');
    });

    it('throws when message is missing', () => {
        expect(() => ChatMessageRequest.parse({
            web_session_id: 'ws-123',
            user_id: 'user-456',
        })).toThrow('message is required');
    });

    it('throws when message is empty string after trim', () => {
        expect(() => ChatMessageRequest.parse({
            web_session_id: 'ws-123',
            user_id: 'user-456',
            message: '   ',
        })).toThrow('message must be at least 1 character(s)');
    });

    it('forWire() omits identity fields (travel via headers)', () => {
        const req = ChatMessageRequest.parse({
            web_session_id: 'ws-123',
            user_id: 'user-456',
            message: 'test',
            llm_primary_model: 'gemini-2.5-pro',
        });
        const wire = req.forWire();
        expect(wire.message).toBe('test');
        expect(wire.attachments).toEqual([]);
        expect(wire.sentinel_mode).toBe(true);
        expect(wire.llm_primary_model).toBe('gemini-2.5-pro');
        expect(wire.llm_assistant_model).toBeNull();
        expect(wire.web_session_id).toBeUndefined();
        expect(wire.user_id).toBeUndefined();
        expect(wire.case_id).toBeUndefined();
        expect(wire.investigation_id).toBeUndefined();
    });
});

describe('InvestigationQueryRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid fields with defaults', () => {
        const req = InvestigationQueryRequest.parse({});
        expect(req.case_id).toBeNull();
        expect(req.web_session_id).toBeNull();
        expect(req.status).toBeNull();
        expect(req.investigation_type).toBeNull();
        expect(req.priority).toBeNull();
        expect(req.limit).toBe(20);
    });

    it('accepts all fields with values', () => {
        const req = InvestigationQueryRequest.parse({
            case_id: 'case-abc',
            web_session_id: 'ws-123',
            status: 'active',
            investigation_type: 'chat',
            priority: 'high',
            limit: 50,
        });
        expect(req.case_id).toBe('case-abc');
        expect(req.web_session_id).toBe('ws-123');
        expect(req.status).toBe('active');
        expect(req.investigation_type).toBe('chat');
        expect(req.priority).toBe('high');
        expect(req.limit).toBe(50);
    });

    it('enforces limit min constraint', () => {
        expect(() => InvestigationQueryRequest.parse({ limit: 0 }))
            .toThrow('limit must be >= 1');
    });

    it('enforces limit max constraint', () => {
        expect(() => InvestigationQueryRequest.parse({ limit: 101 }))
            .toThrow('limit must be <= 100');
    });

    it('forWire() omits null fields', () => {
        const req = InvestigationQueryRequest.parse({
            case_id: 'case-abc',
            limit: 50,
        });
        const wire = req.forWire();
        expect(wire.case_id).toBe('case-abc');
        expect(wire.limit).toBe(50);
        expect(wire.web_session_id).toBeUndefined();
        expect(wire.status).toBeUndefined();
        expect(wire.investigation_type).toBeUndefined();
        expect(wire.priority).toBeUndefined();
    });
});

describe('SessionCreateRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const req = SessionCreateRequest.parse({
            user_id: 'user-456',
        });
        expect(req.user_id).toBe('user-456');
        expect(req.organization_id).toBeNull();
        expect(req.metadata).toEqual({});
    });

    it('accepts all fields with values', () => {
        const req = SessionCreateRequest.parse({
            user_id: 'user-456',
            organization_id: 'org-789',
            metadata: { ip: '192.168.1.1' },
        });
        expect(req.organization_id).toBe('org-789');
        expect(req.metadata).toEqual({ ip: '192.168.1.1' });
    });

    it('throws when user_id is missing', () => {
        expect(() => SessionCreateRequest.parse({}))
            .toThrow('user_id is required');
    });
});

describe('ApprovalRespondRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const req = ApprovalRespondRequest.parse({
            approval_id: 'appr-123',
            approved: true,
        });
        expect(req.approval_id).toBe('appr-123');
        expect(req.approved).toBe(true);
        expect(req.reason).toBe('');
    });

    it('accepts all fields with values', () => {
        const req = ApprovalRespondRequest.parse({
            approval_id: 'appr-123',
            approved: false,
            reason: 'Unsafe command',
        });
        expect(req.approved).toBe(false);
        expect(req.reason).toBe('Unsafe command');
    });

    it('throws when approval_id is missing', () => {
        expect(() => ApprovalRespondRequest.parse({ approved: true }))
            .toThrow('approval_id is required');
    });

    it('throws when approved is missing', () => {
        expect(() => ApprovalRespondRequest.parse({ approval_id: 'appr-123' }))
            .toThrow('approved is required');
    });
});

describe('IntentRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid required field', () => {
        const req = IntentRequest.parse({ intent: 'read_file' });
        expect(req.intent).toBe('read_file');
    });

    it('throws when intent is missing', () => {
        expect(() => IntentRequest.parse({}))
            .toThrow('intent is required');
    });
});

describe('IntentRequestPayload [UNIT - PURE LOGIC]', () => {
    it('accepts valid required field', () => {
        const req = IntentRequestPayload.parse({ intent: 'write_file' });
        expect(req.intent).toBe('write_file');
    });

    it('throws when intent is missing', () => {
        expect(() => IntentRequestPayload.parse({}))
            .toThrow('intent is required');
    });
});

describe('UnlockAccountRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid required field', () => {
        const req = UnlockAccountRequest.parse({ user_id: 'user-456' });
        expect(req.user_id).toBe('user-456');
    });

    it('throws when user_id is missing', () => {
        expect(() => UnlockAccountRequest.parse({}))
            .toThrow('user_id is required');
    });
});

describe('SSEPushRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields', () => {
        const req = SSEPushRequest.parse({
            web_session_id: 'ws-123',
            user_id: 'user-456',
            event: { type: 'test', data: {} },
        });
        expect(req.web_session_id).toBe('ws-123');
        expect(req.user_id).toBe('user-456');
        expect(req.event).toEqual({ type: 'test', data: {} });
    });

    it('throws when web_session_id is missing', () => {
        expect(() => SSEPushRequest.parse({ event: {} }))
            .toThrow('web_session_id is required');
    });

    it('throws when event is missing', () => {
        expect(() => SSEPushRequest.parse({ web_session_id: 'ws-123' }))
            .toThrow('event is required');
    });
});

describe('DirectCommandRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const req = DirectCommandRequest.parse({
            command: 'ls -la',
            execution_id: 'exec-123',
        });
        expect(req.command).toBe('ls -la');
        expect(req.execution_id).toBe('exec-123');
        expect(req.hostname).toBeNull();
        expect(req.source).toBe('anchored_terminal');
    });

    it('accepts all fields with values', () => {
        const req = DirectCommandRequest.parse({
            command: 'pwd',
            execution_id: 'exec-456',
            hostname: 'server-1',
            source: 'web_terminal',
        });
        expect(req.hostname).toBe('server-1');
        expect(req.source).toBe('web_terminal');
    });

    it('throws when command is missing', () => {
        expect(() => DirectCommandRequest.parse({ execution_id: 'exec-123' }))
            .toThrow('command is required');
    });

    it('throws when command is empty string', () => {
        expect(() => DirectCommandRequest.parse({
            command: '',
            execution_id: 'exec-123',
        })).toThrow('command must be at least 1 character(s)');
    });

    it('throws when execution_id is missing', () => {
        expect(() => DirectCommandRequest.parse({ command: 'ls' }))
            .toThrow('execution_id is required');
    });
});

describe('CreateOperatorRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const req = CreateOperatorRequest.parse({
            operator_id: 'op-123',
            user_id: 'user-456',
            operator_session_id: 'ops-789',
        });
        expect(req.operator_id).toBe('op-123');
        expect(req.user_id).toBe('user-456');
        expect(req.operator_session_id).toBe('ops-789');
        expect(req.web_session_id).toBeNull();
        expect(req.organization_id).toBeNull();
        expect(req.system_info).toEqual({});
        expect(req.runtime_config).toEqual({});
        expect(req.api_key).toBeNull();
        expect(req.operator_type).toBeNull();
        expect(req.cloud_subtype).toBeNull();
    });

    it('accepts all fields with values', () => {
        const systemInfo = { hostname: 'server-1' };
        const runtimeConfig = { timeout: 300 };
        const req = CreateOperatorRequest.parse({
            operator_id: 'op-123',
            user_id: 'user-456',
            operator_session_id: 'ops-789',
            web_session_id: 'ws-abc',
            organization_id: 'org-def',
            system_info: systemInfo,
            runtime_config: runtimeConfig,
            api_key: 'key-ghi',
            operator_type: 'local',
            cloud_subtype: 'aws',
        });
        expect(req.system_info).toEqual(systemInfo);
        expect(req.runtime_config).toEqual(runtimeConfig);
        expect(req.api_key).toBe('key-ghi');
        expect(req.operator_type).toBe('local');
        expect(req.cloud_subtype).toBe('aws');
    });

    it('throws when operator_id is missing', () => {
        expect(() => CreateOperatorRequest.parse({
            user_id: 'user-456',
            operator_session_id: 'ops-789',
        })).toThrow('operator_id is required');
    });

    it('throws when user_id is missing', () => {
        expect(() => CreateOperatorRequest.parse({
            operator_id: 'op-123',
            operator_session_id: 'ops-789',
        })).toThrow('user_id is required');
    });

    it('throws when operator_session_id is missing', () => {
        expect(() => CreateOperatorRequest.parse({
            operator_id: 'op-123',
            user_id: 'user-456',
        })).toThrow('operator_session_id is required');
    });
});

describe('PasskeyRegisterChallengeRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid required field', () => {
        const req = PasskeyRegisterChallengeRequest.parse({ user_id: 'user-456' });
        expect(req.user_id).toBe('user-456');
    });

    it('throws when user_id is missing', () => {
        expect(() => PasskeyRegisterChallengeRequest.parse({}))
            .toThrow('user_id is required');
    });
});

describe('AttestationResponseJSON [UNIT - PURE LOGIC]', () => {
    const validResponse = {
        id: 'cred-123',
        rawId: 'raw-cred-123',
        type: 'public-key',
        clientExtensionResults: {},
        response: {
            clientDataJSON: 'eyJjaGFsbGVuZ2UiOiJhYmMifQ==',
            attestationObject: 'o2NmbXRkbm9uZWdhdHRTdG10oGhhdXRoRGF0YViUSZYN5YgOjGh0NBcPZHZgW4_krrmihjLHmVzzuoMdl2NFXwl_RNzHGx2OAuP4lN',
        },
    };

    it('accepts valid attestation response', () => {
        const req = AttestationResponseJSON.parse(validResponse);
        expect(req.id).toBe('cred-123');
        expect(req.rawId).toBe('raw-cred-123');
        expect(req.type).toBe('public-key');
        expect(req.response).toBeDefined();
    });

    it('throws when raw is not an object', () => {
        expect(() => AttestationResponseJSON.parse(null))
            .toThrow('AttestationResponseJSON.parse() requires a plain object');
    });

    it('throws when raw is not an object (string)', () => {
        expect(() => AttestationResponseJSON.parse('invalid'))
            .toThrow('AttestationResponseJSON.parse() requires a plain object');
    });

    it('throws when response object is missing', () => {
        expect(() => AttestationResponseJSON.parse({
            id: 'cred-123',
            rawId: 'raw-cred-123',
            type: 'public-key',
        })).toThrow('AttestationResponseJSON requires response object');
    });

    it('throws when response is not an object', () => {
        expect(() => AttestationResponseJSON.parse({
            id: 'cred-123',
            rawId: 'raw-cred-123',
            type: 'public-key',
            response: 'invalid',
        })).toThrow('AttestationResponseJSON requires response object');
    });

    it('throws when response.clientDataJSON is missing', () => {
        expect(() => AttestationResponseJSON.parse({
            id: 'cred-123',
            rawId: 'raw-cred-123',
            type: 'public-key',
            response: {
                attestationObject: 'o2NmbXRkbm9uZWdhdHRTdG10',
            },
        })).toThrow('AttestationResponseJSON requires response.clientDataJSON string');
    });

    it('throws when response.clientDataJSON is not a string', () => {
        expect(() => AttestationResponseJSON.parse({
            id: 'cred-123',
            rawId: 'raw-cred-123',
            type: 'public-key',
            response: {
                clientDataJSON: 123,
                attestationObject: 'o2NmbXRkbm9uZWdhdHRTdG10',
            },
        })).toThrow('AttestationResponseJSON requires response.clientDataJSON string');
    });

    it('throws when response.attestationObject is missing', () => {
        expect(() => AttestationResponseJSON.parse({
            id: 'cred-123',
            rawId: 'raw-cred-123',
            type: 'public-key',
            response: {
                clientDataJSON: 'eyJjaGFsbGVuZ2UiOiJhYmMifQ==',
            },
        })).toThrow('AttestationResponseJSON requires response.attestationObject string');
    });

    it('throws when response.attestationObject is not a string', () => {
        expect(() => AttestationResponseJSON.parse({
            id: 'cred-123',
            rawId: 'raw-cred-123',
            type: 'public-key',
            response: {
                clientDataJSON: 'eyJjaGFsbGVuZ2UiOiJhYmMifQ==',
                attestationObject: 123,
            },
        })).toThrow('AttestationResponseJSON requires response.attestationObject string');
    });

    it('accepts response with clientExtensionResults', () => {
        const req = AttestationResponseJSON.parse({
            ...validResponse,
            clientExtensionResults: { credProps: true },
        });
        expect(req.clientExtensionResults).toEqual({ credProps: true });
    });
});

describe('AssertionResponseJSON [UNIT - PURE LOGIC]', () => {
    const validResponse = {
        id: 'cred-123',
        rawId: 'raw-cred-123',
        type: 'public-key',
        clientExtensionResults: {},
        response: {
            clientDataJSON: 'eyJjaGFsbGVuZ2UiOiJhYmMifQ==',
            authenticatorData: 'SZYN5YgOjGh0NBcPZHZgW4_krrmihjLHmVzzuoMdl2N',
            signature: 'MEUCIQDqZ-1hZy-9hZy-9hZy-9hZy-9hZy-9hZy-9hZy-9hZy-9hQIgK-1hZy-9hZy-9hZy-9hZy-9hZy-9hZy-9hZy-9hZy-9hZy-9hZy-9hQ',
        },
    };

    it('accepts valid assertion response', () => {
        const req = AssertionResponseJSON.parse(validResponse);
        expect(req.id).toBe('cred-123');
        expect(req.rawId).toBe('raw-cred-123');
        expect(req.type).toBe('public-key');
        expect(req.response).toBeDefined();
    });

    it('throws when raw is not an object', () => {
        expect(() => AssertionResponseJSON.parse(null))
            .toThrow('AssertionResponseJSON.parse() requires a plain object');
    });

    it('throws when raw is not an object (string)', () => {
        expect(() => AssertionResponseJSON.parse('invalid'))
            .toThrow('AssertionResponseJSON.parse() requires a plain object');
    });

    it('throws when response object is missing', () => {
        expect(() => AssertionResponseJSON.parse({
            id: 'cred-123',
            rawId: 'raw-cred-123',
            type: 'public-key',
        })).toThrow('AssertionResponseJSON requires response object');
    });

    it('throws when response is not an object', () => {
        expect(() => AssertionResponseJSON.parse({
            id: 'cred-123',
            rawId: 'raw-cred-123',
            type: 'public-key',
            response: 'invalid',
        })).toThrow('AssertionResponseJSON requires response object');
    });

    it('throws when response.clientDataJSON is missing', () => {
        expect(() => AssertionResponseJSON.parse({
            id: 'cred-123',
            rawId: 'raw-cred-123',
            type: 'public-key',
            response: {
                authenticatorData: 'SZYN5YgOjGh0NBcPZHZgW4',
                signature: 'MEUCIQDqZ',
            },
        })).toThrow('AssertionResponseJSON requires response.clientDataJSON string');
    });

    it('throws when response.clientDataJSON is not a string', () => {
        expect(() => AssertionResponseJSON.parse({
            id: 'cred-123',
            rawId: 'raw-cred-123',
            type: 'public-key',
            response: {
                clientDataJSON: 123,
                authenticatorData: 'SZYN5YgOjGh0NBcPZHZgW4',
                signature: 'MEUCIQDqZ',
            },
        })).toThrow('AssertionResponseJSON requires response.clientDataJSON string');
    });

    it('throws when response.authenticatorData is missing', () => {
        expect(() => AssertionResponseJSON.parse({
            id: 'cred-123',
            rawId: 'raw-cred-123',
            type: 'public-key',
            response: {
                clientDataJSON: 'eyJjaGFsbGVuZ2UiOiJhYmMifQ==',
                signature: 'MEUCIQDqZ',
            },
        })).toThrow('AssertionResponseJSON requires response.authenticatorData string');
    });

    it('throws when response.authenticatorData is not a string', () => {
        expect(() => AssertionResponseJSON.parse({
            id: 'cred-123',
            rawId: 'raw-cred-123',
            type: 'public-key',
            response: {
                clientDataJSON: 'eyJjaGFsbGVuZ2UiOiJhYmMifQ==',
                authenticatorData: 123,
                signature: 'MEUCIQDqZ',
            },
        })).toThrow('AssertionResponseJSON requires response.authenticatorData string');
    });

    it('throws when response.signature is missing', () => {
        expect(() => AssertionResponseJSON.parse({
            id: 'cred-123',
            rawId: 'raw-cred-123',
            type: 'public-key',
            response: {
                clientDataJSON: 'eyJjaGFsbGVuZ2UiOiJhYmMifQ==',
                authenticatorData: 'SZYN5YgOjGh0NBcPZHZgW4',
            },
        })).toThrow('AssertionResponseJSON requires response.signature string');
    });

    it('throws when response.signature is not a string', () => {
        expect(() => AssertionResponseJSON.parse({
            id: 'cred-123',
            rawId: 'raw-cred-123',
            type: 'public-key',
            response: {
                clientDataJSON: 'eyJjaGFsbGVuZ2UiOiJhYmMifQ==',
                authenticatorData: 'SZYN5YgOjGh0NBcPZHZgW4',
                signature: 123,
            },
        })).toThrow('AssertionResponseJSON requires response.signature string');
    });

    it('accepts response with clientExtensionResults', () => {
        const req = AssertionResponseJSON.parse({
            ...validResponse,
            clientExtensionResults: { appid: true },
        });
        expect(req.clientExtensionResults).toEqual({ appid: true });
    });
});

describe('PasskeyRegisterVerifyRequest [UNIT - PURE LOGIC]', () => {
    const validAttestation = {
        id: 'cred-123',
        rawId: 'raw-cred-123',
        type: 'public-key',
        clientExtensionResults: {},
        response: {
            clientDataJSON: 'eyJjaGFsbGVuZ2UiOiJhYmMifQ==',
            attestationObject: 'o2NmbXRkbm9uZWdhdHRTdG10',
        },
    };

    it('accepts valid required fields', () => {
        const req = PasskeyRegisterVerifyRequest.parse({
            user_id: 'user-456',
            attestation_response: validAttestation,
        });
        expect(req.user_id).toBe('user-456');
        expect(req.attestation_response).toBeDefined();
    });

    it('throws when user_id is missing', () => {
        expect(() => PasskeyRegisterVerifyRequest.parse({
            attestation_response: validAttestation,
        })).toThrow('user_id is required');
    });

    it('throws when attestation_response is missing', () => {
        expect(() => PasskeyRegisterVerifyRequest.parse({
            user_id: 'user-456',
        })).toThrow('attestation_response is required');
    });
});

describe('PasskeyAuthChallengeRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid required field', () => {
        const req = PasskeyAuthChallengeRequest.parse({ email: 'test@example.com' });
        expect(req.email).toBe('test@example.com');
    });

    it('throws when email is missing', () => {
        expect(() => PasskeyAuthChallengeRequest.parse({}))
            .toThrow('email is required');
    });
});

describe('PasskeyAuthVerifyRequest [UNIT - PURE LOGIC]', () => {
    const validAssertion = {
        id: 'cred-123',
        rawId: 'raw-cred-123',
        type: 'public-key',
        clientExtensionResults: {},
        response: {
            clientDataJSON: 'eyJjaGFsbGVuZ2UiOiJhYmMifQ==',
            authenticatorData: 'SZYN5YgOjGh0NBcPZHZgW4',
            signature: 'MEUCIQDqZ',
        },
    };

    it('accepts valid required fields', () => {
        const req = PasskeyAuthVerifyRequest.parse({
            email: 'test@example.com',
            assertion_response: validAssertion,
        });
        expect(req.email).toBe('test@example.com');
        expect(req.assertion_response).toBeDefined();
    });

    it('throws when email is missing', () => {
        expect(() => PasskeyAuthVerifyRequest.parse({
            assertion_response: validAssertion,
        })).toThrow('email is required');
    });

    it('throws when assertion_response is missing', () => {
        expect(() => PasskeyAuthVerifyRequest.parse({
            email: 'test@example.com',
        })).toThrow('assertion_response is required');
    });
});

describe('CreateDeviceLinkRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid fields with defaults', () => {
        const req = CreateDeviceLinkRequest.parse({});
        expect(req.name).toBeNull();
        expect(req.max_uses).toBe(1);
        expect(req.expires_in_hours).toBe(24);
    });

    it('accepts all fields with values', () => {
        const req = CreateDeviceLinkRequest.parse({
            name: 'My Device',
            max_uses: 5,
            expires_in_hours: 48,
        });
        expect(req.name).toBe('My Device');
        expect(req.max_uses).toBe(5);
        expect(req.expires_in_hours).toBe(48);
    });

    it('enforces max_uses min constraint', () => {
        expect(() => CreateDeviceLinkRequest.parse({ max_uses: 0 }))
            .toThrow('max_uses must be >= 1');
    });

    it('enforces expires_in_hours min constraint', () => {
        expect(() => CreateDeviceLinkRequest.parse({ expires_in_hours: 0 }))
            .toThrow('expires_in_hours must be >= 1');
    });
});

describe('GenerateDeviceLinkRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid required field', () => {
        const req = GenerateDeviceLinkRequest.parse({ operator_id: 'op-123' });
        expect(req.operator_id).toBe('op-123');
    });

    it('throws when operator_id is missing', () => {
        expect(() => GenerateDeviceLinkRequest.parse({}))
            .toThrow('operator_id is required');
    });
});

describe('RegisterDeviceRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const req = RegisterDeviceRequest.parse({
            hostname: 'server-1',
            os: 'linux',
            arch: 'amd64',
            system_fingerprint: 'fp-abc123',
        });
        expect(req.hostname).toBe('server-1');
        expect(req.os).toBe('linux');
        expect(req.arch).toBe('amd64');
        expect(req.system_fingerprint).toBe('fp-abc123');
        expect(req.version).toBe('unknown');
        expect(req.system_info).toEqual({});
    });

    it('accepts all fields with values', () => {
        const req = RegisterDeviceRequest.parse({
            hostname: 'server-1',
            os: 'linux',
            arch: 'amd64',
            system_fingerprint: 'fp-abc123',
            version: '1.0.0',
            system_info: { cpu: 'x86_64' },
        });
        expect(req.version).toBe('1.0.0');
        expect(req.system_info).toEqual({ cpu: 'x86_64' });
    });

    it('throws when hostname is missing', () => {
        expect(() => RegisterDeviceRequest.parse({
            os: 'linux',
            arch: 'amd64',
            system_fingerprint: 'fp-abc123',
        })).toThrow('hostname is required');
    });

    it('throws when os is missing', () => {
        expect(() => RegisterDeviceRequest.parse({
            hostname: 'server-1',
            arch: 'amd64',
            system_fingerprint: 'fp-abc123',
        })).toThrow('os is required');
    });

    it('throws when arch is missing', () => {
        expect(() => RegisterDeviceRequest.parse({
            hostname: 'server-1',
            os: 'linux',
            system_fingerprint: 'fp-abc123',
        })).toThrow('arch is required');
    });

    it('throws when system_fingerprint is missing', () => {
        expect(() => RegisterDeviceRequest.parse({
            hostname: 'server-1',
            os: 'linux',
            arch: 'amd64',
        })).toThrow('system_fingerprint is required');
    });
});

describe('BindOperatorsRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields', () => {
        const req = BindOperatorsRequest.parse({
            operator_ids: ['op-1', 'op-2'],
            web_session_id: 'ws-123',
            user_id: 'user-456',
        });
        expect(req.operator_ids).toEqual(['op-1', 'op-2']);
        expect(req.web_session_id).toBe('ws-123');
        expect(req.user_id).toBe('user-456');
    });

    it('throws when operator_ids is missing', () => {
        expect(() => BindOperatorsRequest.parse({
            web_session_id: 'ws-123',
            user_id: 'user-456',
        })).toThrow('operator_ids is required');
    });

    it('throws when operator_ids is empty array during parse()', () => {
        expect(() => BindOperatorsRequest.parse({
            operator_ids: [],
            web_session_id: 'ws-123',
            user_id: 'user-456',
        })).toThrow('operator_ids must be a non-empty array');
    });

    it('throws when operator_ids is not an array', () => {
        expect(() => BindOperatorsRequest.parse({
            operator_ids: 'op-1',
            web_session_id: 'ws-123',
            user_id: 'user-456',
        })).toThrow('operator_ids must be an array');
    });

    it('throws validation error with validationErrors property', () => {
        try {
            BindOperatorsRequest.parse({
                operator_ids: [],
                web_session_id: 'ws-123',
                user_id: 'user-456',
            });
            throw new Error('Should have thrown');
        } catch (err) {
            expect(err.message).toBe('BindOperatorsRequest validation failed: operator_ids must be a non-empty array');
            expect(err.validationErrors).toEqual(['operator_ids must be a non-empty array']);
        }
    });

    it('throws when web_session_id is missing', () => {
        expect(() => BindOperatorsRequest.parse({
            operator_ids: ['op-1'],
            user_id: 'user-456',
        })).toThrow('web_session_id is required');
    });

    it('throws when user_id is missing', () => {
        expect(() => BindOperatorsRequest.parse({
            operator_ids: ['op-1'],
            web_session_id: 'ws-123',
        })).toThrow('user_id is required');
    });
});

describe('UnbindOperatorsRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields', () => {
        const req = UnbindOperatorsRequest.parse({
            operator_ids: ['op-1', 'op-2'],
            web_session_id: 'ws-123',
            user_id: 'user-456',
        });
        expect(req.operator_ids).toEqual(['op-1', 'op-2']);
        expect(req.web_session_id).toBe('ws-123');
        expect(req.user_id).toBe('user-456');
    });

    it('throws when operator_ids is missing', () => {
        expect(() => UnbindOperatorsRequest.parse({
            web_session_id: 'ws-123',
            user_id: 'user-456',
        })).toThrow('operator_ids is required');
    });

    it('throws when operator_ids is empty array during parse()', () => {
        expect(() => UnbindOperatorsRequest.parse({
            operator_ids: [],
            web_session_id: 'ws-123',
            user_id: 'user-456',
        })).toThrow('operator_ids must be a non-empty array');
    });

    it('throws when operator_ids is not an array', () => {
        expect(() => UnbindOperatorsRequest.parse({
            operator_ids: 'op-1',
            web_session_id: 'ws-123',
            user_id: 'user-456',
        })).toThrow('operator_ids must be an array');
    });

    it('throws validation error with validationErrors property', () => {
        try {
            UnbindOperatorsRequest.parse({
                operator_ids: [],
                web_session_id: 'ws-123',
                user_id: 'user-456',
            });
            throw new Error('Should have thrown');
        } catch (err) {
            expect(err.message).toBe('UnbindOperatorsRequest validation failed: operator_ids must be a non-empty array');
            expect(err.validationErrors).toEqual(['operator_ids must be a non-empty array']);
        }
    });

    it('throws when web_session_id is missing', () => {
        expect(() => UnbindOperatorsRequest.parse({
            operator_ids: ['op-1'],
            user_id: 'user-456',
        })).toThrow('web_session_id is required');
    });

    it('throws when user_id is missing', () => {
        expect(() => UnbindOperatorsRequest.parse({
            operator_ids: ['op-1'],
            web_session_id: 'ws-123',
        })).toThrow('user_id is required');
    });
});

describe('OperatorSessionRegistrationRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields', () => {
        const req = OperatorSessionRegistrationRequest.parse({
            operator_id: 'op-123',
            operator_session_id: 'ops-456',
        });
        expect(req.operator_id).toBe('op-123');
        expect(req.operator_session_id).toBe('ops-456');
    });

    it('throws when operator_id is missing', () => {
        expect(() => OperatorSessionRegistrationRequest.parse({
            operator_session_id: 'ops-456',
        })).toThrow('operator_id is required');
    });

    it('throws when operator_session_id is missing', () => {
        expect(() => OperatorSessionRegistrationRequest.parse({
            operator_id: 'op-123',
        })).toThrow('operator_session_id is required');
    });
});

describe('StopOperatorRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields', () => {
        const req = StopOperatorRequest.parse({
            operator_id: 'op-123',
            operator_session_id: 'ops-456',
            user_id: 'user-789',
        });
        expect(req.operator_id).toBe('op-123');
        expect(req.operator_session_id).toBe('ops-456');
        expect(req.user_id).toBe('user-789');
    });

    it('throws when operator_id is missing', () => {
        expect(() => StopOperatorRequest.parse({
            operator_session_id: 'ops-456',
            user_id: 'user-789',
        })).toThrow('operator_id is required');
    });

    it('throws when operator_session_id is missing', () => {
        expect(() => StopOperatorRequest.parse({
            operator_id: 'op-123',
            user_id: 'user-789',
        })).toThrow('operator_session_id is required');
    });

    it('throws when user_id is missing', () => {
        expect(() => StopOperatorRequest.parse({
            operator_id: 'op-123',
            operator_session_id: 'ops-456',
        })).toThrow('user_id is required');
    });
});

describe('OperatorRegisterSessionRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields', () => {
        const req = OperatorRegisterSessionRequest.parse({
            operator_id: 'op-123',
            operator_session_id: 'ops-456',
        });
        expect(req.operator_id).toBe('op-123');
        expect(req.operator_session_id).toBe('ops-456');
    });

    it('throws when operator_id is missing', () => {
        expect(() => OperatorRegisterSessionRequest.parse({
            operator_session_id: 'ops-456',
        })).toThrow('operator_id is required');
    });

    it('throws when operator_session_id is missing', () => {
        expect(() => OperatorRegisterSessionRequest.parse({
            operator_id: 'op-123',
        })).toThrow('operator_session_id is required');
    });
});

describe('SettingsUpdateRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const req = SettingsUpdateRequest.parse({
            settings: { llm: { provider: 'gemini' } },
        });
        expect(req.settings).toEqual({ llm: { provider: 'gemini' } });
        expect(req.setup_secret).toBeNull();
    });

    it('accepts all fields with values', () => {
        const req = SettingsUpdateRequest.parse({
            settings: { llm: { provider: 'gemini' } },
            setup_secret: 'secret-abc',
        });
        expect(req.setup_secret).toBe('secret-abc');
    });

    it('throws when settings is missing', () => {
        expect(() => SettingsUpdateRequest.parse({}))
            .toThrow('settings is required');
    });

    it('throws when settings is an array during parse()', () => {
        expect(() => SettingsUpdateRequest.parse({
            settings: [{ key: 'value' }],
        })).toThrow('settings must be an object, not an array');
    });

    it('throws validation error with descriptive message', () => {
        try {
            SettingsUpdateRequest.parse({
                settings: ['item1', 'item2'],
            });
            throw new Error('Should have thrown');
        } catch (err) {
            expect(err.message).toBe('SettingsUpdateRequest validation failed: settings must be an object, not an array');
        }
    });
});

describe('RefreshOperatorKeyRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid required field', () => {
        const req = RefreshOperatorKeyRequest.parse({ user_id: 'user-456' });
        expect(req.user_id).toBe('user-456');
    });

    it('throws when user_id is missing', () => {
        expect(() => RefreshOperatorKeyRequest.parse({}))
            .toThrow('user_id is required');
    });
});

describe('InitializeOperatorSlotsRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid fields with defaults', () => {
        const req = InitializeOperatorSlotsRequest.parse({});
        expect(req.organization_id).toBeNull();
    });

    it('accepts all fields with values', () => {
        const req = InitializeOperatorSlotsRequest.parse({
            organization_id: 'org-789',
        });
        expect(req.organization_id).toBe('org-789');
    });
});

describe('CreateUserRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const req = CreateUserRequest.parse({
            email: 'test@example.com',
            name: 'Test User',
        });
        expect(req.email).toBe('test@example.com');
        expect(req.name).toBe('Test User');
        expect(req.roles).toBeNull();
    });

    it('accepts all fields with values', () => {
        const req = CreateUserRequest.parse({
            email: 'test@example.com',
            name: 'Test User',
            roles: ['admin'],
        });
        expect(req.roles).toEqual(['admin']);
    });

    it('throws when email is missing', () => {
        expect(() => CreateUserRequest.parse({ name: 'Test User' }))
            .toThrow('email is required');
    });

    it('throws when email is too short', () => {
        expect(() => CreateUserRequest.parse({
            email: 'ab',
            name: 'Test User',
        })).toThrow('email must be at least 3 character(s)');
    });

    it('throws when name is missing', () => {
        expect(() => CreateUserRequest.parse({ email: 'test@example.com' }))
            .toThrow('name is required');
    });

    it('throws when name is empty string', () => {
        expect(() => CreateUserRequest.parse({
            email: 'test@example.com',
            name: '',
        })).toThrow('name must be at least 1 character(s)');
    });
});

describe('UpdateUserRolesRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const req = UpdateUserRolesRequest.parse({ role: 'admin' });
        expect(req.role).toBe('admin');
        expect(req.action).toBe('set');
    });

    it('accepts all fields with values', () => {
        const req = UpdateUserRolesRequest.parse({
            role: 'admin',
            action: 'add',
        });
        expect(req.role).toBe('admin');
        expect(req.action).toBe('add');
    });

    it('throws when role is missing', () => {
        expect(() => UpdateUserRolesRequest.parse({}))
            .toThrow('role is required');
    });
});

describe('StopAIRequest [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const req = StopAIRequest.parse({
            investigation_id: 'inv-123',
            web_session_id: 'ws-456',
        });
        expect(req.investigation_id).toBe('inv-123');
        expect(req.reason).toBe('User requested stop');
        expect(req.web_session_id).toBe('ws-456');
    });

    it('accepts all fields with values', () => {
        const req = StopAIRequest.parse({
            investigation_id: 'inv-123',
            reason: 'Manual stop',
            web_session_id: 'ws-456',
        });
        expect(req.reason).toBe('Manual stop');
    });

    it('throws when investigation_id is missing', () => {
        expect(() => StopAIRequest.parse({
            web_session_id: 'ws-456',
        })).toThrow('investigation_id is required');
    });

    it('throws when web_session_id is missing', () => {
        expect(() => StopAIRequest.parse({
            investigation_id: 'inv-123',
        })).toThrow('web_session_id is required');
    });
});

describe('SessionAuthResponse [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const req = SessionAuthResponse.parse({
            success: true,
        });
        expect(req.success).toBe(true);
        expect(req.operator_session_id).toBeNull();
        expect(req.operator_id).toBeNull();
        expect(req.user_id).toBeNull();
        expect(req.organization_id).toBeNull();
        expect(req.api_key).toBeNull();
        expect(req.config).toEqual({});
        expect(req.operator_cert).toBeNull();
        expect(req.operator_cert_key).toBeNull();
        expect(req.error).toBeNull();
    });

    it('accepts all fields with values', () => {
        const req = SessionAuthResponse.parse({
            success: true,
            operator_session_id: 'ops-123',
            operator_id: 'op-456',
            user_id: 'user-789',
            organization_id: 'org-abc',
            api_key: 'key-def',
            config: { timeout: 300 },
            operator_cert: 'cert-data',
            operator_cert_key: 'key-data',
            error: null,
        });
        expect(req.operator_session_id).toBe('ops-123');
        expect(req.operator_id).toBe('op-456');
        expect(req.user_id).toBe('user-789');
        expect(req.organization_id).toBe('org-abc');
        expect(req.api_key).toBe('key-def');
        expect(req.config).toEqual({ timeout: 300 });
        expect(req.operator_cert).toBe('cert-data');
        expect(req.operator_cert_key).toBe('key-data');
    });

    it('accepts failure response with error', () => {
        const req = SessionAuthResponse.parse({
            success: false,
            error: 'Authentication failed',
        });
        expect(req.success).toBe(false);
        expect(req.error).toBe('Authentication failed');
    });

    it('throws when success is missing', () => {
        expect(() => SessionAuthResponse.parse({}))
            .toThrow('success is required');
    });
});

describe('BoundOperatorContext [UNIT - PURE LOGIC]', () => {
    it('accepts valid required fields with defaults', () => {
        const req = BoundOperatorContext.parse({
            operator_id: 'op-123',
            operator_session_id: 'ops-456',
        });
        expect(req.operator_id).toBe('op-123');
        expect(req.operator_session_id).toBe('ops-456');
        expect(req.status).toBeNull();
    });

    it('accepts all fields with values', () => {
        const req = BoundOperatorContext.parse({
            operator_id: 'op-123',
            operator_session_id: 'ops-456',
            status: 'ACTIVE',
        });
        expect(req.status).toBe('ACTIVE');
    });

    it('throws when operator_id is missing', () => {
        expect(() => BoundOperatorContext.parse({
            operator_session_id: 'ops-456',
        })).toThrow('operator_id is required');
    });

    it('allows operator_session_id to be missing (optional)', () => {
        const result = BoundOperatorContext.parse({
            operator_id: 'op-123',
        });
        expect(result.operator_session_id).toBeNull();
    });
});

describe('RequestModelFactory [UNIT - PURE LOGIC]', () => {
    it('createChatRequest returns ChatMessageRequest instance', () => {
        const data = {
            web_session_id: 'ws-123',
            user_id: 'user-456',
            message: 'test',
        };
        const req = RequestModelFactory.createChatRequest(data);
        expect(req).toBeInstanceOf(ChatMessageRequest);
        expect(req.message).toBe('test');
    });

    it('createInvestigationQueryRequest returns InvestigationQueryRequest instance', () => {
        const data = { case_id: 'case-abc' };
        const req = RequestModelFactory.createInvestigationQueryRequest(data);
        expect(req).toBeInstanceOf(InvestigationQueryRequest);
        expect(req.case_id).toBe('case-abc');
    });

    it('createUnlockAccountRequest returns UnlockAccountRequest instance', () => {
        const data = { user_id: 'user-456' };
        const req = RequestModelFactory.createUnlockAccountRequest(data);
        expect(req).toBeInstanceOf(UnlockAccountRequest);
        expect(req.user_id).toBe('user-456');
    });

    it('createSettingsUpdateRequest returns SettingsUpdateRequest instance', () => {
        const data = { settings: { llm: { provider: 'gemini' } } };
        const req = RequestModelFactory.createSettingsUpdateRequest(data);
        expect(req).toBeInstanceOf(SettingsUpdateRequest);
        expect(req.settings).toEqual({ llm: { provider: 'gemini' } });
    });

    it('createRefreshOperatorKeyRequest returns RefreshOperatorKeyRequest instance', () => {
        const data = { user_id: 'user-456' };
        const req = RequestModelFactory.createRefreshOperatorKeyRequest(data);
        expect(req).toBeInstanceOf(RefreshOperatorKeyRequest);
        expect(req.user_id).toBe('user-456');
    });

    it('createInitializeOperatorSlotsRequest returns InitializeOperatorSlotsRequest instance', () => {
        const data = { organization_id: 'org-789' };
        const req = RequestModelFactory.createInitializeOperatorSlotsRequest(data);
        expect(req).toBeInstanceOf(InitializeOperatorSlotsRequest);
        expect(req.organization_id).toBe('org-789');
    });

    it('createUpdateUserRolesRequest returns UpdateUserRolesRequest instance', () => {
        const data = { role: 'admin', action: 'add' };
        const req = RequestModelFactory.createUpdateUserRolesRequest(data);
        expect(req).toBeInstanceOf(UpdateUserRolesRequest);
        expect(req.role).toBe('admin');
    });

    it('createCreateUserRequest returns CreateUserRequest instance', () => {
        const data = { email: 'test@example.com', name: 'Test User' };
        const req = RequestModelFactory.createCreateUserRequest(data);
        expect(req).toBeInstanceOf(CreateUserRequest);
        expect(req.email).toBe('test@example.com');
    });

    it('createSSEPushRequest returns SSEPushRequest instance', () => {
        const data = {
            web_session_id: 'ws-123',
            user_id: 'user-456',
            event: { type: 'test' },
        };
        const req = RequestModelFactory.createSSEPushRequest(data);
        expect(req).toBeInstanceOf(SSEPushRequest);
        expect(req.web_session_id).toBe('ws-123');
        expect(req.user_id).toBe('user-456');
    });

    it('createIntentRequest returns IntentRequest instance', () => {
        const data = { intent: 'read_file' };
        const req = RequestModelFactory.createIntentRequest(data);
        expect(req).toBeInstanceOf(IntentRequest);
        expect(req.intent).toBe('read_file');
    });

    it('createSessionRequest returns SessionCreateRequest instance', () => {
        const data = { user_id: 'user-456' };
        const req = RequestModelFactory.createSessionRequest(data);
        expect(req).toBeInstanceOf(SessionCreateRequest);
        expect(req.user_id).toBe('user-456');
    });

    it('createApprovalRespondRequest returns ApprovalRespondRequest instance', () => {
        const data = { approval_id: 'appr-123', approved: true };
        const req = RequestModelFactory.createApprovalRespondRequest(data);
        expect(req).toBeInstanceOf(ApprovalRespondRequest);
        expect(req.approval_id).toBe('appr-123');
    });

    it('createDirectCommandRequest returns DirectCommandRequest instance', () => {
        const data = { command: 'ls', execution_id: 'exec-123' };
        const req = RequestModelFactory.createDirectCommandRequest(data);
        expect(req).toBeInstanceOf(DirectCommandRequest);
        expect(req.command).toBe('ls');
    });

    it('createOperatorRequest returns CreateOperatorRequest instance', () => {
        const data = {
            operator_id: 'op-123',
            user_id: 'user-456',
            operator_session_id: 'ops-789',
        };
        const req = RequestModelFactory.createOperatorRequest(data);
        expect(req).toBeInstanceOf(CreateOperatorRequest);
        expect(req.operator_id).toBe('op-123');
    });

    it('createBindOperatorsRequest returns BindOperatorsRequest instance', () => {
        const data = {
            operator_ids: ['op-1'],
            web_session_id: 'ws-123',
            user_id: 'user-456',
        };
        const req = RequestModelFactory.createBindOperatorsRequest(data);
        expect(req).toBeInstanceOf(BindOperatorsRequest);
        expect(req.operator_ids).toEqual(['op-1']);
    });

    it('createUnbindOperatorsRequest returns UnbindOperatorsRequest instance', () => {
        const data = {
            operator_ids: ['op-1'],
            web_session_id: 'ws-123',
            user_id: 'user-456',
        };
        const req = RequestModelFactory.createUnbindOperatorsRequest(data);
        expect(req).toBeInstanceOf(UnbindOperatorsRequest);
        expect(req.operator_ids).toEqual(['op-1']);
    });

    it('createCreateDeviceLinkRequest returns CreateDeviceLinkRequest instance', () => {
        const data = { name: 'My Device' };
        const req = RequestModelFactory.createCreateDeviceLinkRequest(data);
        expect(req).toBeInstanceOf(CreateDeviceLinkRequest);
        expect(req.name).toBe('My Device');
    });

    it('createGenerateDeviceLinkRequest returns GenerateDeviceLinkRequest instance', () => {
        const data = { operator_id: 'op-123' };
        const req = RequestModelFactory.createGenerateDeviceLinkRequest(data);
        expect(req).toBeInstanceOf(GenerateDeviceLinkRequest);
        expect(req.operator_id).toBe('op-123');
    });

    it('createRegisterDeviceRequest returns RegisterDeviceRequest instance', () => {
        const data = {
            hostname: 'server-1',
            os: 'linux',
            arch: 'amd64',
            system_fingerprint: 'fp-abc',
        };
        const req = RequestModelFactory.createRegisterDeviceRequest(data);
        expect(req).toBeInstanceOf(RegisterDeviceRequest);
        expect(req.hostname).toBe('server-1');
    });

    it('createPasskeyRegisterChallengeRequest returns PasskeyRegisterChallengeRequest instance', () => {
        const data = { user_id: 'user-456' };
        const req = RequestModelFactory.createPasskeyRegisterChallengeRequest(data);
        expect(req).toBeInstanceOf(PasskeyRegisterChallengeRequest);
        expect(req.user_id).toBe('user-456');
    });

    it('createPasskeyRegisterVerifyRequest returns PasskeyRegisterVerifyRequest instance', () => {
        const attestation = {
            id: 'cred-123',
            rawId: 'raw-cred-123',
            type: 'public-key',
            clientExtensionResults: {},
            response: {
                clientDataJSON: 'eyJjaGFsbGVuZ2UiOiJhYmMifQ==',
                attestationObject: 'o2NmbXRkbm9uZWdhdHRTdG10',
            },
        };
        const data = {
            user_id: 'user-456',
            attestation_response: attestation,
        };
        const req = RequestModelFactory.createPasskeyRegisterVerifyRequest(data);
        expect(req).toBeInstanceOf(PasskeyRegisterVerifyRequest);
        expect(req.user_id).toBe('user-456');
    });

    it('createPasskeyAuthChallengeRequest returns PasskeyAuthChallengeRequest instance', () => {
        const data = { email: 'test@example.com' };
        const req = RequestModelFactory.createPasskeyAuthChallengeRequest(data);
        expect(req).toBeInstanceOf(PasskeyAuthChallengeRequest);
        expect(req.email).toBe('test@example.com');
    });

    it('createPasskeyAuthVerifyRequest returns PasskeyAuthVerifyRequest instance', () => {
        const assertion = {
            id: 'cred-123',
            rawId: 'raw-cred-123',
            type: 'public-key',
            clientExtensionResults: {},
            response: {
                clientDataJSON: 'eyJjaGFsbGVuZ2UiOiJhYmMifQ==',
                authenticatorData: 'SZYN5YgOjGh0NBcPZHZgW4',
                signature: 'MEUCIQDqZ',
            },
        };
        const data = {
            email: 'test@example.com',
            assertion_response: assertion,
        };
        const req = RequestModelFactory.createPasskeyAuthVerifyRequest(data);
        expect(req).toBeInstanceOf(PasskeyAuthVerifyRequest);
        expect(req.email).toBe('test@example.com');
    });

    it('all factory methods delegate to Model.parse() and throw on invalid data', () => {
        expect(() => RequestModelFactory.createChatRequest({}))
            .toThrow('web_session_id is required');
        expect(() => RequestModelFactory.createIntentRequest({}))
            .toThrow('intent is required');
        expect(() => RequestModelFactory.createUnlockAccountRequest({}))
            .toThrow('user_id is required');
    });
});
