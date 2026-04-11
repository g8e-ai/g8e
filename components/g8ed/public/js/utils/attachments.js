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
 * g8ed Attachments Utility - Local-First Attachment Management
 * 
 * Handles file attachments entirely on the client side until message send.
 * Files are read as base64 locally for instant preview and sent with the
 * chat message payload. g8ed backend stores them securely in g8es KV.
 * 
 * Flow:
 * 1. User selects file -> file read as base64 via FileReader (non-blocking)
 * 2. Image preview via data: URI from base64 (CSP-compliant, no blob: URLs)
 * 3. Attachment stored in local Map with preview + base64 data
 * 4. On send: getFormattedForBackend() returns base64 payloads with metadata
 * 5. g8ed stores in g8es KV, forwards metadata to g8ee
 * 6. g8ee retrieves from g8es KV for AI processing
 */

import { MAX_ATTACHMENT_SIZE, MAX_ATTACHMENT_FILES, ALLOWED_ATTACHMENT_CONTENT_TYPES } from '../constants/service-client-constants.js';
import { nowISOString } from './timestamp.js';
import { CssClass } from '../constants/ui-constants.js';
import { ATTACHMENT_ERROR_DISPLAY_MS } from '../constants/app-constants.js';
import { escapeHtml } from './html.js';

export class AttachmentsManager {
    constructor(options = {}) {
        this.config = {
            maxFileSize: options.maxFileSize || MAX_ATTACHMENT_SIZE,
            allowedTypes: options.allowedTypes || ALLOWED_ATTACHMENT_CONTENT_TYPES,
            maxFiles: options.maxFiles || MAX_ATTACHMENT_FILES
        };

        this.attachments = new Map();
        this.eventCallbacks = new Map();
    }

    on(event, callback) {
        if (!this.eventCallbacks.has(event)) {
            this.eventCallbacks.set(event, []);
        }
        this.eventCallbacks.get(event).push(callback);
    }

    emit(event, data) {
        const callbacks = this.eventCallbacks.get(event);
        if (callbacks) {
            callbacks.forEach(callback => callback(data));
        }
    }

    validateFile(file) {
        const errors = [];

        if (file.size > this.config.maxFileSize) {
            errors.push(`File "${file.name}" exceeds maximum size of ${this.formatFileSize(this.config.maxFileSize)}`);
        }

        if (this.config.allowedTypes.length > 0 && !this.config.allowedTypes.includes(file.type)) {
            errors.push(`File type "${file.type || 'unknown'}" is not allowed for "${file.name}"`);
        }

        if (this.attachments.size >= this.config.maxFiles) {
            errors.push(`Maximum of ${this.config.maxFiles} files allowed`);
        }

        return {
            valid: errors.length === 0,
            errors
        };
    }

    async addFiles(files) {
        const results = [];

        for (const file of files) {
            try {
                const result = await this.addFile(file);
                results.push(result);
            } catch (error) {
                console.error('[AttachmentsManager] Error in addFiles:', error);
                results.push({
                    success: false,
                    file: file.name,
                    error: error.message
                });
                this.emit('error', { file: file.name, error: error.message });
            }
        }

        return results;
    }

    /**
     * Add a single file - reads base64 locally, creates instant preview.
     * No network calls. File data stays in memory until message is sent.
     */
    async addFile(file) {
        const validation = this.validateFile(file);

        if (!validation.valid) {
            throw new Error(validation.errors.join(', '));
        }

        const fileId = this.generateFileId();

        // Read file as base64 (non-blocking)
        const base64Data = await this._readFileAsBase64(file);

        // Image preview via data: URI (CSP-compliant — blob: is not in img-src)
        const previewUrl = file.type.startsWith('image/') ? `data:${file.type};base64,${base64Data}` : null;

        const attachment = {
            id: fileId,
            name: file.name,
            size: file.size,
            type: file.type,
            lastModified: file.lastModified,
            base64Data: base64Data,
            previewUrl: previewUrl,
            addedAt: nowISOString(),
            preview: this.generatePreview(file)
        };

        this.attachments.set(fileId, attachment);
        this.emit('added', attachment);

        return {
            success: true,
            file: file.name,
            attachment
        };
    }

    /**
     * Read file as base64 string (without the data URI prefix)
     */
    _readFileAsBase64(file) {
        return new Promise((resolve, reject) => {
            const reader = new FileReader();
            reader.onload = () => {
                // Strip the "data:mime/type;base64," prefix
                const base64 = reader.result.split(',')[1];
                resolve(base64);
            };
            reader.onerror = () => reject(new Error(`Failed to read file: ${file.name}`));
            reader.readAsDataURL(file);
        });
    }

    removeAttachment(fileId) {
        const attachment = this.attachments.get(fileId);
        if (attachment) {
            this.attachments.delete(fileId);
            this.emit('removed', attachment);
            return true;
        }
        return false;
    }

    clearAll() {
        const count = this.attachments.size;
        this.attachments.clear();
        this.emit('cleared', { count });
    }

    getAll() {
        return Array.from(this.attachments.values());
    }

    getById(fileId) {
        return this.attachments.get(fileId);
    }

    /**
     * Get attachments formatted for the backend.
     * Includes base64 data so g8ed can store in g8es KV.
     */
    getFormattedForBackend() {
        return this.getAll().map(attachment => ({
            filename: attachment.name,
            file_size: attachment.size,
            content_type: attachment.type,
            base64_data: attachment.base64Data
        }));
    }

    getCount() {
        return this.attachments.size;
    }

    generateFileId() {
        return 'file_' + Date.now() + '_' + Math.random().toString(36).slice(2, 11);
    }

    generatePreview(file) {
        const isImage = file.type.startsWith('image/');
        return {
            isImage,
            icon: this.getFileIcon(file.type),
            canPreview: isImage || file.type === 'text/plain'
        };
    }

    getFileIcon(mimeType) {
        if (mimeType.startsWith('image/')) return 'image';
        if (mimeType === 'application/pdf') return 'picture_as_pdf';
        if (mimeType.includes('word')) return 'description';
        if (mimeType.includes('excel') || mimeType.includes('spreadsheet')) return 'table_chart';
        if (mimeType.includes('zip')) return 'folder_zip';
        if (mimeType.startsWith('text/')) return 'article';
        return 'insert_drive_file';
    }

    formatFileSize(bytes) {
        if (bytes === 0) return '0 Bytes';
        const k = 1024;
        const sizes = ['Bytes', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    createFileInput(options = {}) {
        const input = document.createElement('input');
        input.type = 'file';
        input.multiple = options.multiple !== false;
        input.accept = options.accept || this.config.allowedTypes.join(',');
        input.classList.add(CssClass.INITIALLY_HIDDEN);

        input.addEventListener('change', async (event) => {
            const files = Array.from(event.target.files);
            if (files.length > 0) {
                await this.addFiles(files);
            }
            input.value = '';
        });

        return input;
    }
}

/**
 * CompactAttachmentsUI - Inline attachment display for the anchored terminal
 */
export class CompactAttachmentsUI {
    constructor(manager, containerElement) {
        this.manager = manager;
        this.container = containerElement;
        this.fileInput = null;

        this.setupEventListeners();
        this.render();
    }

    setupEventListeners() {
        this.manager.on('added', () => this.render());
        this.manager.on('removed', () => this.render());
        this.manager.on('cleared', () => this.render());
        this.manager.on('error', (data) => this.showError(data.error));
    }

    render() {
        const attachments = this.manager.getAll();

        if (attachments.length === 0) {
            this.container.classList.add(CssClass.INITIALLY_HIDDEN);
            return;
        }

        this.container.classList.remove(CssClass.INITIALLY_HIDDEN);
        this.container.innerHTML = `
            <div class="compact-attachments-list">
                ${attachments.map(file => this.renderCompactAttachment(file)).join('')}
            </div>
            <button class="compact-clear-btn" title="Clear all attachments">×</button>
        `;

        this.container.querySelector('.compact-clear-btn')?.addEventListener('click', () => {
            this.manager.clearAll();
        });

        this.container.querySelectorAll('.compact-remove-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const fileId = e.target.getAttribute('data-file-id');
                this.manager.removeAttachment(fileId);
            });
        });
    }

    truncateFilename(filename) {
        if (filename.length <= 11) {
            return filename;
        }
        return filename.slice(0, 4) + '...' + filename.slice(-4);
    }

    renderCompactAttachment(file) {
        const isImage = file.preview.isImage;
        // Use local preview URL for instant image thumbnails
        const thumbnailHtml = isImage && file.previewUrl
            ? `<img src="${file.previewUrl}" class="compact-attachment-thumbnail" alt="${escapeHtml(file.name)}" />` 
            : `<span class="compact-attachment-icon material-symbols-outlined">${file.preview.icon}</span>`;
        
        const displayName = this.truncateFilename(file.name);
        
        return `
            <div class="compact-attachment-item ${isImage ? 'has-thumbnail' : ''}" data-file-id="${file.id}" title="${escapeHtml(file.name)} (${this.manager.formatFileSize(file.size)})">
                <div class="compact-attachment-preview">
                    ${thumbnailHtml}
                </div>
                <span class="compact-attachment-name">${escapeHtml(displayName)}</span>
                <button class="compact-remove-btn" data-file-id="${file.id}" title="Remove ${escapeHtml(file.name)}">×</button>
            </div>
        `;
    }

    createAttachButton(buttonElement) {
        if (!this.fileInput) {
            this.fileInput = this.manager.createFileInput();
            document.body.appendChild(this.fileInput);
        }

        buttonElement.addEventListener('click', () => {
            if (buttonElement.disabled) return;
            this.fileInput.click();
        });

        return buttonElement;
    }

    showError(message) {
        const errorDiv = document.createElement('div');
        errorDiv.className = 'compact-attachment-error';
        errorDiv.textContent = message;

        this.container.appendChild(errorDiv);

        setTimeout(() => {
            errorDiv.remove();
        }, ATTACHMENT_ERROR_DISPLAY_MS);
    }
}
