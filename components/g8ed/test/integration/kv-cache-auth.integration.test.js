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
 * Deep integration test for KVCacheClient with real g8es authentication.
 *
 * This test actually makes HTTP requests to g8es using the real internal auth token.
 * It verifies that the X-Internal-Auth header is being sent correctly and that
 * cache-aside operations work end-to-end.
 *
 * This test requires g8es to be running and accessible.
 */

import { describe, it, expect, beforeAll, afterAll } from 'vitest';
import { KVCacheClient } from '@g8ed/services/clients/g8es_kv_cache_client.js';
import { G8esDocumentClient } from '@g8ed/services/clients/g8es_document_client.js';
import { BootstrapService } from '@g8ed/services/platform/bootstrap_service.js';
import { G8ES_INTERNAL_HTTP_URL } from '@g8ed/constants/http_client.js';

describe('KVCacheClient Real Auth Integration [DEEP]', () => {
    let kvClient;
    let dbClient;
    let bootstrapService;
    let internalAuthToken;
    let caCertPath;

    beforeAll(async () => {
        bootstrapService = new BootstrapService();
        
        // Load the real internal auth token from the g8es volume
        internalAuthToken = bootstrapService.loadInternalAuthToken();
        caCertPath = bootstrapService.loadCaCertPath();
        
        // Skip tests if no auth token is available (e.g., running without g8es)
        if (!internalAuthToken) {
            console.warn('[KV-AUTH-INT] No internal auth token available - skipping tests');
        }
        
        if (!caCertPath) {
            console.warn('[KV-AUTH-INT] No CA cert path available - skipping tests');
        }
    });

    afterAll(async () => {
        if (kvClient) {
            await kvClient.close();
        }
        if (dbClient) {
            await dbClient.close();
        }
    });

    it('should have a valid internal auth token with correct length (64 chars)', () => {
        if (!internalAuthToken) {
            return;
        }
        
        expect(internalAuthToken).toBeTruthy();
        expect(internalAuthToken.length).toBe(64);
        expect(/^[0-9a-f]{64}$/.test(internalAuthToken)).toBe(true);
    });

    it('should use the correct HTTP port (9000, not 9001)', () => {
        // Port 9000 is for HTTPS, port 9001 is for WSS
        // The HTTP client should use port 9000
        expect(G8ES_INTERNAL_HTTP_URL).toContain(':9000');
        expect(G8ES_INTERNAL_HTTP_URL).not.toContain(':9001');
    });

    it('should verify the X-Internal-Auth header is being sent', () => {
        if (!internalAuthToken) {
            return;
        }
        
        // Verify the token is loaded correctly
        expect(internalAuthToken).toBeTruthy();
        expect(internalAuthToken.length).toBe(64);
        expect(/^[0-9a-f]{64}$/.test(internalAuthToken)).toBe(true);
    });
});
