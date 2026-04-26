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
 * Global Test Services
 * 
 * Uses globalThis to survive ES module reloads in Vitest.
 * Services are initialized once for the entire test process.
 * 
 * Rules (from testing.md):
 * 1. Everything is local - use real services and real inter-component communications.
 * 2. Mocks are prohibited - never mock internal services or database clients.
 * 3. Use real infrastructure (g8es) for all tests.
 */

import { logger } from '../../utils/logger.js';

const GLOBAL_KEY = '__G8ED_TEST_SERVICES__';

function getGlobalState() {
    if (!globalThis[GLOBAL_KEY]) {
        globalThis[GLOBAL_KEY] = {
            initialized: false,
            initPromise: null,
            services: null,
            pubSubClient: null,
        };
    }
    return globalThis[GLOBAL_KEY];
}

/**
 * Initialize all platform services for testing.
 * Uses the real initialization.js flow to ensure architectural consistency.
 */
export async function initializeTestServices() {
    const state = getGlobalState();
    
    if (state.initPromise) {
        return state.initPromise;
    }
    
    if (state.initialized && state.services) {
        return state.services;
    }
    
    state.initPromise = (async () => {
        try {
            logger.info('[TEST-SERVICES] Initializing platform services for testing...');
            const startTime = Date.now();
            
            // 1. Load initialization module
            const initModule = await import('../../services/initialization.js');
            
            // 2. Mock BootstrapService globally for tests to provide fallback secrets
            const { BootstrapService } = await import('../../services/platform/bootstrap_service.js');
            const originalLoadKey = BootstrapService.prototype.loadSessionEncryptionKey;
            const originalLoadToken = BootstrapService.prototype.loadInternalAuthToken;
            
            BootstrapService.prototype.loadSessionEncryptionKey = function() {
                const key = originalLoadKey.call(this);
                if (key) return key;
                return '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef';
            };
            
            BootstrapService.prototype.loadInternalAuthToken = function() {
                const token = originalLoadToken.call(this);
                if (token) return token;
                return 'test-auth-token';
            };
            
            logger.info('[TEST-SERVICES] Global BootstrapService mocks applied for test environment');
            
            // 3. Perform full multi-phase initialization (Phase 1-6)
            // This sets up real g8es clients, cache-aside, settings, and all services.
            await initModule.initializeServices();
            
            // 4. Extract services
            const services = {
                organizationModel:      initModule.getOrganizationModel(),
                pubSubClient:           initModule.getPubSubClient(),
                cacheAsideService:       initModule.getCacheAsideService(),
                webSessionService:      initModule.getWebSessionService(),
                operatorSessionService: initModule.getOperatorSessionService(),
                bindingService:         initModule.getBindingService(),
                apiKeyService:          initModule.getApiKeyService(),
                userService:            initModule.getUserService(),
                operatorService:        initModule.getOperatorService(),
                settingsService:        initModule.getSettingsService(),
                passkeyAuthService:     initModule.getPasskeyAuthService(),
                attachmentService:      initModule.getAttachmentService(),
                sseService:             initModule.getSSEService(),
                internalHttpClient:     initModule.getInternalHttpClient(),
                deviceLinkService:      initModule.getDeviceLinkService(),
                certificateService:     initModule.getCertificateService(),
                consoleMetricsService:  initModule.getConsoleMetricsService(),
                bindOperatorsService:   initModule.getBindOperatorsService(),
                g8eNodeOperatorService: initModule.getG8ENodeOperatorService(),
                postLoginService:       initModule.getPostLoginService(),
                auditService:           initModule.getAuditService(),
                setupService:           initModule.getSetupService(),
                deviceRegistrationService: initModule.getDeviceRegistrationService(),
                healthCheckService:     initModule.getHealthCheckService(),
                blobClient:             initModule.getG8esBlobClient(),
                initModule,
            };

            // 5. Apply test overrides (e.g. collection naming)
            if (services.operatorService && services.operatorService.collectionName) {
                services.operatorService.collectionName = `${services.operatorService.collectionName}_test`;
            }
            if (services.userService && services.userService.collectionName) {
                services.userService.collectionName = `${services.userService.collectionName}_test`;
            }
            if (services.settingsService && services.settingsService.collectionName) {
                services.settingsService.collectionName = `${services.settingsService.collectionName}_test`;
            }

            state.services = services;
            state.pubSubClient = services.pubSubClient;
            state.initialized = true;
            
            logger.info(`[TEST-SERVICES] Platform services initialized in ${Date.now() - startTime}ms`);
            return state.services;
        } catch (error) {
            state.initPromise = null;
            logger.error('[TEST-SERVICES] Failed to initialize platform services', { error: error.message, stack: error.stack });
            throw error;
        }
    })();
    
    return state.initPromise;
}

/**
 * Get initialized test services singleton.
 */
export async function getTestServices() {
    const state = getGlobalState();
    if (!state.initialized || !state.services) {
        await initializeTestServices();
    }
    return state.services;
}

/**
 * Get the shared G8esPubSubClient.
 */
export async function getTestG8esPubSubClient() {
    const state = getGlobalState();
    if (!state.pubSubClient) {
        await getTestServices();
    }
    return state.pubSubClient;
}

/**
 * Reset and cleanup all services.
 */
export async function cleanupTestServices() {
    const state = getGlobalState();
    if (!state.initialized) return;

    try {
        const { resetInitialization } = await import('../../services/initialization.js');
        resetInitialization();
    } catch (err) {
        logger.warn('[TEST-SERVICES] Failed to reset initialization state', { error: err.message });
    }

    state.initialized = false;
    state.initPromise = null;
    state.services = null;
    state.pubSubClient = null;
    
    logger.info('[TEST-SERVICES] Services cleaned up');
}
