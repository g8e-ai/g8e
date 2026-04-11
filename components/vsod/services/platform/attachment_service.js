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

import crypto from 'crypto';
import { logger } from '../../utils/logger.js';
import { now } from '../../models/base.js';
import { AttachmentRecord, AttachmentMeta } from '../../models/attachment_model.js';
import { KVKey } from '../../constants/kv_keys.js';
import { CacheTTL } from '../../constants/service_config.js';
import { MAX_ATTACHMENT_SIZE, MAX_TOTAL_ATTACHMENT_SIZE, ALLOWED_ATTACHMENT_CONTENT_TYPES } from '../../constants/service_config.js';

export class AttachmentService {
    /**
     * @param {Object} options
     * @param {Object} options.cacheAsideService - CacheAsideService instance
     * @param {import('./vsodb_blob_client.js').VSODBBlobClient} options.blobStorage
     */
    constructor({ cacheAsideService, blobStorage } = {}) {
        this._cache_aside = cacheAsideService;
        this.blobStorage = blobStorage;
    }

    validateAttachment(attachment) {
        const errors = [];

        if (!attachment.filename || typeof attachment.filename !== 'string') {
            errors.push('filename is required and must be a string');
        }

        if (!attachment.content_type || typeof attachment.content_type !== 'string') {
            errors.push('content_type is required and must be a string');
        } else if (!ALLOWED_ATTACHMENT_CONTENT_TYPES.includes(attachment.content_type)) {
            errors.push(`content_type "${attachment.content_type}" is not allowed`);
        }

        if (!attachment.base64_data || typeof attachment.base64_data !== 'string') {
            errors.push('base64_data is required and must be a string');
        } else {
            const base64Regex = /^[A-Za-z0-9+/]*={0,2}$/;
            if (!base64Regex.test(attachment.base64_data)) {
                errors.push('base64_data contains invalid characters');
            }
        }

        const fileSize = attachment.file_size;
        if (typeof fileSize !== 'number' || fileSize <= 0) {
            errors.push('file_size is required and must be a positive number');
        } else if (fileSize > MAX_ATTACHMENT_SIZE) {
            errors.push(`File size ${fileSize} exceeds maximum of ${MAX_ATTACHMENT_SIZE} bytes`);
        }

        return { valid: errors.length === 0, errors };
    }

    sanitizeFilename(filename) {
        return filename
            .replace(/[^a-zA-Z0-9._\- ]/g, '_')
            .replace(/\.{2,}/g, '.')
            .substring(0, 255);
    }

    async storeAttachments(investigationId, userId, attachments) {
        if (!this._cache_aside || !this.blobStorage) {
            throw new Error('AttachmentService not initialized');
        }

        if (!investigationId || !userId) {
            throw new Error('investigationId and userId are required');
        }

        if (!Array.isArray(attachments) || attachments.length === 0) {
            return [];
        }

        const totalSize = attachments.reduce((sum, att) => sum + (att.file_size || 0), 0);
        if (totalSize > MAX_TOTAL_ATTACHMENT_SIZE) {
            throw new Error(`Total attachment size ${totalSize} exceeds maximum of ${MAX_TOTAL_ATTACHMENT_SIZE} bytes`);
        }

        const stored = [];
        const indexKey = KVKey.attachmentIndex(investigationId);

        for (const attachment of attachments) {
            const validation = this.validateAttachment(attachment);
            if (!validation.valid) {
                logger.warn('[ATTACHMENTS] Skipping invalid attachment', {
                    filename: attachment.filename,
                    errors:   validation.errors,
                });
                continue;
            }

            const attachmentId    = crypto.randomBytes(16).toString('hex');
            const kvKey           = KVKey.attachment(investigationId, attachmentId);
            const sanitizedFilename = this.sanitizeFilename(attachment.filename);

            try {
                const objectKey = await this.blobStorage.putAttachment(
                    investigationId,
                    attachmentId,
                    attachment.base64_data,
                    attachment.content_type,
                );

                const record = new AttachmentRecord({
                    attachment_id:     attachmentId,
                    investigation_id:  investigationId,
                    user_id:           userId,
                    filename:          sanitizedFilename,
                    original_filename: attachment.filename,
                    file_size:         attachment.file_size,
                    content_type:      attachment.content_type,
                    object_key:        objectKey,
                    stored_at:         now(),
                });

                await this._cache_aside.kvSetJson(kvKey, record.forKV(), CacheTTL.ATTACHMENT);
                await this._cache_aside.kvRpush(indexKey, attachmentId);
                await this._cache_aside.kvExpire(indexKey, CacheTTL.ATTACHMENT);

                const meta = new AttachmentMeta({
                    attachment_id:    attachmentId,
                    kv_key:           kvKey,
                    filename:         sanitizedFilename,
                    file_size:        attachment.file_size,
                    content_type:     attachment.content_type,
                    investigation_id: investigationId,
                });

                stored.push(meta.forWire());

                logger.info('[ATTACHMENTS] Stored attachment', {
                    attachment_id:    attachmentId,
                    filename:         sanitizedFilename,
                    file_size:        attachment.file_size,
                    content_type:     attachment.content_type,
                    investigation_id: investigationId,
                    object_key:       objectKey,
                });
            } catch (error) {
                logger.error('[ATTACHMENTS] Failed to store attachment', {
                    filename: attachment.filename,
                    error:    error.message,
                });
            }
        }

        logger.info('[ATTACHMENTS] Stored attachments for investigation', {
            investigation_id: investigationId,
            count:            stored.length,
            total_size:       totalSize,
        });

        return stored;
    }

    async getAttachment(attachmentKey) {
        if (!this._cache_aside) {
            throw new Error('AttachmentService not initialized');
        }

        try {
            const data = await this._cache_aside.kvGetJson(attachmentKey);
            if (!data) {
                logger.warn('[ATTACHMENTS] Attachment not found or expired', { kv_key: attachmentKey });
                return null;
            }
            return AttachmentRecord.parse(data);
        } catch (error) {
            logger.error('[ATTACHMENTS] Failed to retrieve attachment', {
                kv_key: attachmentKey,
                error:  error.message,
            });
            return null;
        }
    }

    /**
     * Retrieve attachment metadata and hydrate base64_data from the blob store.
     * Used by g8ee when it needs the raw binary for LLM processing.
     *
     * @param {string} attachmentKey - VSODB KV key
     * @returns {Promise<{record: AttachmentRecord, base64_data: string}|null>}
     */
    async getAttachmentWithData(attachmentKey) {
        if (!this._cache_aside || !this.blobStorage) {
            throw new Error('AttachmentService not initialized');
        }

        const record = await this.getAttachment(attachmentKey);
        if (!record) return null;

        try {
            const base64_data = await this.blobStorage.getAttachment(record.object_key);
            return { record, base64_data };
        } catch (error) {
            logger.error('[ATTACHMENTS] Failed to retrieve attachment data', {
                kv_key:     attachmentKey,
                object_key: record.object_key,
                error:      error.message,
            });
            return null;
        }
    }

    async getAttachmentsForInvestigation(investigationId) {
        if (!this._cache_aside) {
            throw new Error('AttachmentService not initialized');
        }

        const indexKey = KVKey.attachmentIndex(investigationId);
        const attachmentIds = await this._cache_aside.kvLrange(indexKey, 0, -1);

        if (!attachmentIds || attachmentIds.length === 0) {
            return [];
        }

        const attachments = [];
        for (const attachmentId of attachmentIds) {
            const record = await this.getAttachment(KVKey.attachment(investigationId, attachmentId));
            if (record) {
                attachments.push(record);
            }
        }

        return attachments;
    }

    async deleteAttachmentsForInvestigation(investigationId) {
        if (!this._cache_aside || !this.blobStorage) {
            throw new Error('AttachmentService not initialized');
        }

        const indexKey = KVKey.attachmentIndex(investigationId);
        const attachmentIds = await this._cache_aside.kvLrange(indexKey, 0, -1);

        if (!attachmentIds || attachmentIds.length === 0) {
            return 0;
        }

        const keysToDelete = attachmentIds.map(id => KVKey.attachment(investigationId, id));
        keysToDelete.push(indexKey);

        await Promise.all([
            this._cache_aside.kvDel(...keysToDelete),
            this.blobStorage.deleteAttachmentsForInvestigation(investigationId),
        ]);

        logger.info('[ATTACHMENTS] Deleted attachments for investigation', {
            investigation_id: investigationId,
            count:            attachmentIds.length,
        });

        return attachmentIds.length;
    }
}

