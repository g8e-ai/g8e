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
import { Conversation } from '@g8ed/models/conversation_model.js';
import { ConversationStatus } from '@g8ed/constants/chat.js';

describe('Conversation Model [UNIT]', () => {
    describe('Field Definitions', () => {
        it('should have correct default values', () => {
            const conv = new Conversation({ web_session_id: 'ws_123' });
            expect(conv.web_session_id).toBe('ws_123');
            expect(conv.case_id).toBeNull();
            expect(conv.investigation_id).toBeNull();
            expect(conv.user_id).toBeNull();
            expect(conv.status).toBe(ConversationStatus.ACTIVE);
            expect(conv.sentinel_mode).toBe(true);
        });

        it('should require web_session_id', () => {
            expect(() => new Conversation({})).toThrow();
        });
    });

    describe('Validation', () => {
        it('should allow valid statuses', () => {
            const statuses = Object.values(ConversationStatus);
            for (const status of statuses) {
                const conv = new Conversation({ web_session_id: 'ws_1', status });
                expect(conv.status).toBe(status);
            }
        });

        it('should throw on invalid status', () => {
            expect(() => new Conversation({ 
                web_session_id: 'ws_1', 
                status: 'INVALID_STATUS' 
            })).toThrow(/status must be one of/);
        });
    });

    describe('State Management', () => {
        it('should report isActive() correctly', () => {
            const conv = new Conversation({ web_session_id: 'ws_1' });
            expect(conv.isActive()).toBe(true);
            
            conv.status = ConversationStatus.INACTIVE;
            expect(conv.isActive()).toBe(false);
        });

        it('should deactivate() correctly', () => {
            const conv = new Conversation({ web_session_id: 'ws_1' });
            const before = conv.updated_at;
            
            // Wait a tiny bit to ensure timestamp changes if logic uses now()
            conv.deactivate();
            
            expect(conv.status).toBe(ConversationStatus.INACTIVE);
            expect(conv.updated_at).not.toBe(before);
        });

        it('should complete() correctly', () => {
            const conv = new Conversation({ web_session_id: 'ws_1' });
            conv.complete();
            
            expect(conv.status).toBe(ConversationStatus.COMPLETED);
        });
    });

    describe('Serialization', () => {
        it('should remove null optional fields in forDB()', () => {
            const conv = new Conversation({ 
                web_session_id: 'ws_123',
                user_id: 'u_123'
                // case_id and investigation_id are null by default
            });
            
            const dbObj = conv.forDB();
            expect(dbObj.web_session_id).toBe('ws_123');
            expect(dbObj.user_id).toBe('u_123');
            expect(dbObj).not.toHaveProperty('case_id');
            expect(dbObj).not.toHaveProperty('investigation_id');
        });
    });

    describe('Static Factories', () => {
        it('should create from session and request', () => {
            const session = { id: 'ws_123', user_id: 'u_123' };
            const chatRequest = { 
                case_id: 'c_1', 
                investigation_id: 'i_1',
                sentinel_mode: false 
            };
            
            const conv = Conversation.fromSessionAndRequest(session, chatRequest);
            
            expect(conv.web_session_id).toBe('ws_123');
            expect(conv.user_id).toBe('u_123');
            expect(conv.case_id).toBe('c_1');
            expect(conv.investigation_id).toBe('i_1');
            expect(conv.sentinel_mode).toBe(false);
        });

        it('should use defaults in fromSessionAndRequest', () => {
            const session = { id: 'ws_123', user_id: 'u_123' };
            const chatRequest = {};
            
            const conv = Conversation.fromSessionAndRequest(session, chatRequest);
            
            expect(conv.case_id).toBeNull();
            expect(conv.investigation_id).toBeNull();
            expect(conv.sentinel_mode).toBe(true);
        });
    });
});
