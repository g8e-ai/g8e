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

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { AttachmentService } from '@g8ed/services/platform/attachment_service.js';
import { AttachmentRecord, AttachmentMeta } from '@g8ed/models/attachment_model.js';
import { MAX_ATTACHMENT_SIZE, MAX_TOTAL_ATTACHMENT_SIZE } from '@g8ed/constants/service_config.js';

describe('AttachmentService [UNIT]', () => {
    let cacheAside;
    let blobStorage;
    let service;

    beforeEach(() => {
        cacheAside = {
            kvSetJson: vi.fn(),
            kvGetJson: vi.fn(),
            kvRpush: vi.fn(),
            kvLrange: vi.fn(),
            kvExpire: vi.fn(),
            kvDel: vi.fn(),
        };
        blobStorage = {
            putAttachment: vi.fn(),
            getAttachment: vi.fn(),
            deleteAttachmentsForInvestigation: vi.fn(),
        };
        service = new AttachmentService({ cacheAsideService: cacheAside, blobStorage });
    });

    describe('validateAttachment', () => {
        it('validates a correct attachment', () => {
            const attachment = {
                filename: 'test.txt',
                content_type: 'text/plain',
                base64_data: 'SGVsbG8gd29ybGQ=',
                file_size: 11
            };
            const result = service.validateAttachment(attachment);
            expect(result.valid).toBe(true);
            expect(result.errors).toHaveLength(0);
        });

        it('fails on missing filename', () => {
            const attachment = {
                content_type: 'text/plain',
                base64_data: 'SGVsbG8gd29ybGQ=',
                file_size: 11
            };
            const result = service.validateAttachment(attachment);
            expect(result.valid).toBe(false);
            expect(result.errors).toContain('filename is required and must be a string');
        });

        it('fails on invalid content type', () => {
            const attachment = {
                filename: 'test.exe',
                content_type: 'application/x-msdownload',
                base64_data: 'SGVsbG8gd29ybGQ=',
                file_size: 11
            };
            const result = service.validateAttachment(attachment);
            expect(result.valid).toBe(false);
            expect(result.errors[0]).toContain('is not allowed');
        });

        it('fails on oversized file', () => {
            const attachment = {
                filename: 'large.txt',
                content_type: 'text/plain',
                base64_data: 'SGVsbG8gd29ybGQ=',
                file_size: MAX_ATTACHMENT_SIZE + 1
            };
            const result = service.validateAttachment(attachment);
            expect(result.valid).toBe(false);
            expect(result.errors[0]).toContain('exceeds maximum');
        });

        it('fails on invalid base64 data', () => {
            const attachment = {
                filename: 'test.txt',
                content_type: 'text/plain',
                base64_data: 'invalid!!!',
                file_size: 11
            };
            const result = service.validateAttachment(attachment);
            expect(result.valid).toBe(false);
            expect(result.errors).toContain('base64_data contains invalid characters');
        });
    });

    describe('sanitizeFilename', () => {
        it('removes invalid characters', () => {
            expect(service.sanitizeFilename('test/file.txt')).toBe('test_file.txt');
            expect(service.sanitizeFilename('test*file.txt')).toBe('test_file.txt');
            expect(service.sanitizeFilename('test\0file.txt')).toBe('test_file.txt');
        });

        it('collapses multiple dots', () => {
            expect(service.sanitizeFilename('test..txt')).toBe('test.txt');
            expect(service.sanitizeFilename('test....txt')).toBe('test.txt');
        });

        it('truncates long filenames', () => {
            const longName = 'a'.repeat(300) + '.txt';
            expect(service.sanitizeFilename(longName)).toHaveLength(255);
        });
    });

    describe('storeAttachments', () => {
        const investigationId = 'inv_123';
        const userId = 'user_123';

        it('throws if not initialized', async () => {
            const uninitialized = new AttachmentService();
            await expect(uninitialized.storeAttachments(investigationId, userId, []))
                .rejects.toThrow('AttachmentService not initialized');
        });

        it('stores valid attachments and returns metadata', async () => {
            const attachments = [{
                filename: 'test.txt',
                content_type: 'text/plain',
                base64_data: 'SGVsbG8gd29ybGQ=',
                file_size: 11
            }];

            blobStorage.putAttachment.mockResolvedValue('attachments/inv_123/obj_456');

            const result = await service.storeAttachments(investigationId, userId, attachments);

            expect(result).toHaveLength(1);
            expect(result[0].filename).toBe('test.txt');
            expect(blobStorage.putAttachment).toHaveBeenCalledWith(
                investigationId,
                expect.any(String),
                attachments[0].base64_data,
                attachments[0].content_type
            );
            expect(cacheAside.kvSetJson).toHaveBeenCalled();
            expect(cacheAside.kvRpush).toHaveBeenCalled();
        });

        it('skips invalid attachments', async () => {
            const attachments = [
                { filename: 'valid.txt', content_type: 'text/plain', base64_data: 'SGVsbG8=', file_size: 5 },
                { filename: 'invalid.exe', content_type: 'application/exe', base64_data: 'SGVsbG8=', file_size: 5 }
            ];

            blobStorage.putAttachment.mockResolvedValue('obj_123');

            const result = await service.storeAttachments(investigationId, userId, attachments);

            expect(result).toHaveLength(1);
            expect(result[0].filename).toBe('valid.txt');
            expect(blobStorage.putAttachment).toHaveBeenCalledTimes(1);
        });

        it('throws if total size exceeds limit', async () => {
            const attachments = [
                { filename: '1.txt', content_type: 'text/plain', base64_data: 'abc', file_size: MAX_TOTAL_ATTACHMENT_SIZE },
                { filename: '2.txt', content_type: 'text/plain', base64_data: 'def', file_size: 1 }
            ];

            await expect(service.storeAttachments(investigationId, userId, attachments))
                .rejects.toThrow('Total attachment size');
        });
    });

    describe('getAttachment', () => {
        it('returns parsed record if found', async () => {
            const mockData = {
                attachment_id: 'att_123',
                investigation_id: 'inv_123',
                user_id: 'user_123',
                filename: 'test.txt',
                original_filename: 'test.txt',
                file_size: 11,
                content_type: 'text/plain',
                object_key: 'obj_123',
                stored_at: new Date().toISOString()
            };
            cacheAside.kvGetJson.mockResolvedValue(mockData);

            const result = await service.getAttachment('kv_key');
            expect(result).toBeInstanceOf(AttachmentRecord);
            expect(result.attachment_id).toBe('att_123');
        });

        it('returns null if not found', async () => {
            cacheAside.kvGetJson.mockResolvedValue(null);
            const result = await service.getAttachment('kv_key');
            expect(result).toBeNull();
        });
    });

    describe('getAttachmentWithData', () => {
        it('returns record and base64 data', async () => {
            const mockData = {
                attachment_id: 'att_123',
                investigation_id: 'inv_123',
                user_id: 'user_123',
                filename: 'test.txt',
                original_filename: 'test.txt',
                file_size: 11,
                content_type: 'text/plain',
                object_key: 'obj_123',
                stored_at: new Date().toISOString()
            };
            cacheAside.kvGetJson.mockResolvedValue(mockData);
            blobStorage.getAttachment.mockResolvedValue('SGVsbG8=');

            const result = await service.getAttachmentWithData('kv_key');
            expect(result.record).toBeInstanceOf(AttachmentRecord);
            expect(result.base64_data).toBe('SGVsbG8=');
        });
    });

    describe('deleteAttachmentsForInvestigation', () => {
        it('deletes all keys and blobs', async () => {
            const investigationId = 'inv_123';
            cacheAside.kvLrange.mockResolvedValue(['att1', 'att2']);

            const result = await service.deleteAttachmentsForInvestigation(investigationId);

            expect(result).toBe(2);
            expect(cacheAside.kvDel).toHaveBeenCalled();
            expect(blobStorage.deleteAttachmentsForInvestigation).toHaveBeenCalledWith(investigationId);
        });

        it('returns 0 if no attachments found', async () => {
            cacheAside.kvLrange.mockResolvedValue([]);
            const result = await service.deleteAttachmentsForInvestigation('inv_123');
            expect(result).toBe(0);
        });
    });
});
