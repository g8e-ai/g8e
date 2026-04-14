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

import { vi } from 'vitest';
import { now } from '@test/fixtures/base.fixture.js';

export const mockAttachments = {
    recordWithObjectKey: {
        attachment_id:     'abc123def456abc123def456abc123de',
        investigation_id:  'inv-test-001',
        user_id:           'user-test-001',
        filename:          'test.txt',
        original_filename: 'test.txt',
        file_size:         100,
        content_type:      'text/plain',
        object_key:        'attachments/inv-test-001/abc123def456abc123def456abc123de',
        stored_at:         now().toISOString(),
    },

    record2WithObjectKey: {
        attachment_id:     'def456abc123def456abc123def456ab',
        investigation_id:  'inv-test-001',
        user_id:           'user-test-001',
        filename:          'report.pdf',
        original_filename: 'report.pdf',
        file_size:         2048,
        content_type:      'application/pdf',
        object_key:        'attachments/inv-test-001/def456abc123def456abc123def456ab',
        stored_at:         now().toISOString(),
    },

    validInput: {
        filename:    'test.txt',
        content_type: 'text/plain',
        file_size:   100,
        base64_data: 'SGVsbG8gV29ybGQ=',
    },

    validInputPdf: {
        filename:    'report.pdf',
        content_type: 'application/pdf',
        file_size:   2048,
        base64_data: 'JVBERi0xLjQ=',
    },

    invalidContentType: {
        filename:    'video.mp4',
        content_type: 'video/mp4',
        file_size:   100,
        base64_data: 'SGVsbG8=',
    },

    oversized: {
        filename:    'huge.pdf',
        content_type: 'application/pdf',
        file_size:   15 * 1024 * 1024,
        base64_data: 'SGVsbG8=',
    },
};

export function makeAttachmentServiceMock(overrides = {}) {
    const mockInstance = {
        initialize: vi.fn(),
        storeAttachments: vi.fn().mockResolvedValue([]),
        getAttachment: vi.fn().mockResolvedValue(null),
        getAttachmentsForInvestigation: vi.fn().mockResolvedValue([]),
        deleteAttachmentsForInvestigation: vi.fn().mockResolvedValue(0),
        validateAttachment: vi.fn().mockReturnValue({ valid: true, errors: [] }),
        ...overrides,
    };

    return {
        getAttachmentService: vi.fn().mockReturnValue(mockInstance),
    };
}
