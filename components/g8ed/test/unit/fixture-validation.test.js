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
 * Fixture Data Validation Tests
 *
 * Validates that fixture data matches expected model schemas to catch
 * field name mismatches early. This ensures fixture data integrity
 * and prevents silent failures due to typos or schema drift.
 */

import { describe, it, expect } from 'vitest';
import { mockUsers } from '@test/fixtures/users.fixture.js';
import { mockOperators } from '@test/fixtures/operators.fixture.js';
import { mockAttachments } from '@test/fixtures/attachment.fixture.js';

describe('Fixture Data Validation', () => {
    describe('users.fixture.js', () => {
        it('should have UserDocument instances for all mock users', () => {
            for (const [key, user] of Object.entries(mockUsers)) {
                expect(user.constructor.name).toBe('UserDocument');
                expect(user.id).toBeDefined();
                expect(user.email).toBeDefined();
            }
        });

        it('should have required fields in all user fixtures', () => {
            for (const [key, user] of Object.entries(mockUsers)) {
                expect(user.id, `${key} missing id`).toBeDefined();
                expect(user.email, `${key} missing email`).toBeDefined();
                expect(user.organization_id, `${key} missing organization_id`).toBeDefined();
                expect(user.roles, `${key} missing roles`).toBeDefined();
                expect(Array.isArray(user.roles), `${key} roles must be array`).toBe(true);
            }
        });

        it('should have valid email format in all user fixtures', () => {
            const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
            for (const [key, user] of Object.entries(mockUsers)) {
                expect(user.email, `${key} invalid email format`).toMatch(emailRegex);
            }
        });
    });

    describe('operators.fixture.js', () => {
        it('should have OperatorDocument instances for all mock operators', () => {
            for (const [key, operator] of Object.entries(mockOperators)) {
                expect(operator.constructor.name).toBe('OperatorDocument');
                expect(operator.id).toBeDefined();
                expect(operator.user_id).toBeDefined();
            }
        });

        it('should have required fields in all operator fixtures', () => {
            for (const [key, operator] of Object.entries(mockOperators)) {
                expect(operator.id, `${key} missing id`).toBeDefined();
                expect(operator.user_id, `${key} missing user_id`).toBeDefined();
                expect(operator.status, `${key} missing status`).toBeDefined();
                expect(operator.is_slot, `${key} missing is_slot`).toBeDefined();
                expect(typeof operator.is_slot, `${key} is_slot must be boolean`).toBe('boolean');
            }
        });

        it('should have SystemInfo instances where present', () => {
            const operatorsWithSystemInfo = ['claimed', 'stale', 'differentUser', 'activeOperator'];
            for (const key of operatorsWithSystemInfo) {
                const operator = mockOperators[key];
                expect(operator.system_info, `${key} missing system_info`).toBeDefined();
                expect(operator.system_info.constructor.name).toBe('SystemInfo');
            }
        });
    });

    describe('attachment.fixture.js', () => {
        it('should have AttachmentRecord instances for record fixtures', () => {
            const recordFixtures = ['recordWithObjectKey', 'record2WithObjectKey'];
            for (const key of recordFixtures) {
                const attachment = mockAttachments[key];
                expect(attachment.constructor.name).toBe('AttachmentRecord');
                expect(attachment.attachment_id).toBeDefined();
                expect(attachment.investigation_id).toBeDefined();
            }
        });

        it('should have required fields in attachment record fixtures', () => {
            const recordFixtures = ['recordWithObjectKey', 'record2WithObjectKey'];
            for (const key of recordFixtures) {
                const attachment = mockAttachments[key];
                expect(attachment.attachment_id, `${key} missing attachment_id`).toBeDefined();
                expect(attachment.investigation_id, `${key} missing investigation_id`).toBeDefined();
                expect(attachment.user_id, `${key} missing user_id`).toBeDefined();
                expect(attachment.filename, `${key} missing filename`).toBeDefined();
                expect(attachment.file_size, `${key} missing file_size`).toBeDefined();
                expect(attachment.content_type, `${key} missing content_type`).toBeDefined();
                expect(attachment.object_key, `${key} missing object_key`).toBeDefined();
            }
        });

        it('should have valid file sizes (non-negative numbers)', () => {
            const recordFixtures = ['recordWithObjectKey', 'record2WithObjectKey'];
            for (const key of recordFixtures) {
                const attachment = mockAttachments[key];
                expect(attachment.file_size, `${key} file_size must be non-negative`).toBeGreaterThanOrEqual(0);
                expect(typeof attachment.file_size, `${key} file_size must be number`).toBe('number');
            }
        });
    });
});
