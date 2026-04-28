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
 * HTTP Test Client - Production-Quality HTTP Testing
 * 
 * Provides a robust, reusable HTTP client for integration testing using supertest.
 * Uses the full Express app created by createG8edApp to test actual URL path registration.
 * 
 * Features:
 * - Creates a test Express app with real services and middleware
 * - Uses supertest for HTTP request testing
 * - Supports authenticated requests via session cookies
 * - Proper cleanup and resource management
 * - Test-specific configuration (smaller body limits, debug logging)
 * 
 * Usage:
 *   const client = await createTestClient();
 *   const res = await client.get('/health');
 *   expect(res.status).toBe(200);
 *   await client.close();
 */

import request from 'supertest';
import { createG8edApp } from '../../app_factory.js';
import { getTestServices } from './test-services.js';
import { 
    createRateLimiters
} from '../../middleware/rate-limit.js';
import { 
    createRequestTimestampMiddleware
} from '../../middleware/request_timestamp.js';
import {
    createErrorHandlerMiddleware
} from '../../middleware/error_handler.js';
import { createAuthMiddleware } from '../../middleware/authentication.js';
import { createApiKeyMiddleware } from '../../middleware/api_key_auth.js';
import { createAuthorizationMiddleware } from '../../middleware/authorization.js';
import { getVersionInfo } from '../../utils/version.js';

/**
 * Create a test Express app with real services for HTTP integration testing
 * 
 * @returns {Promise<Object>} Object containing app and close function
 */
export async function createTestApp() {
    const services = await getTestServices();
    
    const rateLimiters = createRateLimiters({});
    const requestTimestampMiddleware = createRequestTimestampMiddleware({ 
        cacheAsideService: services.cacheAsideService 
    });
    const errorHandlerMiddleware = createErrorHandlerMiddleware({});
    
    const authMiddleware = createAuthMiddleware({ 
        userService: services.userService, 
        webSessionService: services.webSessionService, 
        setupService: services.setupService,
        settingsService: services.settingsService,
        bindingService: services.bindingService
    });
    
    const apiKeyMiddleware = createApiKeyMiddleware({ 
        apiKeyService: services.apiKeyService, 
        userService: services.userService 
    });
    
    const authorizationMiddleware = createAuthorizationMiddleware({ 
        operatorService: services.operatorService, 
        settingsService: services.settingsService 
    });

    const app = createG8edApp({
        services,
        rateLimiters,
        authMiddleware,
        authorizationMiddleware,
        apiKeyMiddleware,
        requestTimestampMiddleware,
        errorHandlerMiddleware,
        versionInfo: getVersionInfo(),
        isTest: true,
        viewsPath: null,
        publicPath: null
    });

    return { app, services };
}

/**
 * HTTP Test Client Class
 * 
 * Wraps supertest with the test app for convenient HTTP testing.
 */
export class HttpTestClient {
    constructor(app, services) {
        this.app = app;
        this.services = services;
        this.agent = request.agent(app);
        this.sessionCookie = null;
    }

    /**
     * Make a GET request
     */
    get(url) {
        return this.agent.get(url);
    }

    /**
     * Make a POST request
     */
    post(url) {
        return this.agent.post(url);
    }

    /**
     * Make a PUT request
     */
    put(url) {
        return this.agent.put(url);
    }

    /**
     * Make a DELETE request
     */
    delete(url) {
        return this.agent.delete(url);
    }

    /**
     * Make a PATCH request
     */
    patch(url) {
        return this.agent.patch(url);
    }

    /**
     * Set a session cookie for authenticated requests
     */
    setSessionCookie(sessionId) {
        this.sessionCookie = sessionId;
        this.agent = request.agent(this.app);
        this.agent.set('Cookie', `web_session_id=${sessionId}`);
        return this;
    }

    /**
     * Set an auth header for API key authentication
     */
    setApiKey(apiKey) {
        this.agent.set('X-API-Key', apiKey);
        return this;
    }

    /**
     * Set custom headers
     */
    set(headers) {
        this.agent.set(headers);
        return this;
    }

    /**
     * Reset the agent (clear cookies and headers)
     */
    reset() {
        this.agent = request.agent(this.app);
        this.sessionCookie = null;
        return this;
    }

    /**
     * Close the client (no-op for HTTP, but included for interface consistency)
     */
    async close() {
        this.agent = null;
    }
}

/**
 * Factory function to create an HTTP test client
 * 
 * @returns {Promise<HttpTestClient>} Configured HTTP test client
 */
export async function createTestClient() {
    const { app, services } = await createTestApp();
    return new HttpTestClient(app, services);
}

export default HttpTestClient;
