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
 * g8ed Service Initialization
 *
 * Composition root for all platform services. Call initializeServices() once
 * at process startup. All getXxx() accessors throw if called before init.
 *
 * Initialization phases:
 *   1. g8es clients       — document store, KV, pub/sub transport layer
 *   2. Core platform       — DB service wrapper, models, cache-aside, session
 *   3. Auth services       — API keys, users, passkey auth
 *   4. Operator subsystem  — operator service, binary cache, operator cache
 *   5. Platform services   — SSE, attachments, device links, certificates, settings
 *   6. Configuration       — load global settings from g8es
 */

import { G8esDocumentClient } from './clients/g8es_document_client.js';
import { G8esPubSubClient } from './clients/g8es_pubsub_client.js';
import { KVCacheClient } from './clients/g8es_kv_cache_client.js';
import { g8esBlobClient } from './platform/g8es_blob_client.js';
import { WebSessionService } from './auth/web_session_service.js';
import { CliSessionService } from './auth/cli_session_service.js';
import { BoundSessionsService } from './auth/bound_sessions_service.js';
import { CacheAsideService } from './cache/cache_aside_service.js';
import { ApiKeyService } from './auth/api_key_service.js';
import { ApiKeyDataService } from './auth/api_key_data_service.js';
import { UserService } from './platform/user_service.js';
import { OperatorDataService } from './operator/operator_data_service.js';
import { OperatorService } from './operator/operator_service.js';
import { OperatorDownloadService } from './operator/operator_download_service.js';
import { LoginSecurityService } from './auth/login_security_service.js';
import { DownloadAuthService } from './auth/download_auth_service.js';
import { DeviceRegistrationService } from './auth/device_registration_service.js';
import { InternalHttpClient } from './clients/internal_http_client.js';
import { InvestigationService } from './platform/investigation_service.js';
import { logger } from '../utils/logger.js';
import { PasskeyAuthService } from './auth/passkey_auth_service.js';
import { AttachmentService } from './platform/attachment_service.js';
import { SSEService } from './platform/sse_service.js';
import { CertificateService } from './platform/certificate_service.js';
import { DeviceLinkService } from './auth/device_link_service.js';
import { OrganizationModel } from '../models/organization_model.js';
import { BootstrapService } from './platform/bootstrap_service.js';
import { SettingsService } from './platform/settings_service.js';
import { G8ENodeOperatorService } from './platform/g8ep_operator_service.js';
import { BindOperatorsService } from './operator/operator_bind_service.js';
import { ConsoleMetricsService } from './platform/console_metrics_service.js';
import { PostLoginService } from './auth/post_login_service.js';
import { SetupService } from './platform/setup_service.js';
import { AuditService } from './platform/audit_service.js';
import { HealthCheckService } from './platform/health_check_service.js';
import { SourceComponent } from '../constants/ai.js';
import { G8ES_INTERNAL_HTTP_URL, G8ES_INTERNAL_PUBSUB_URL } from '../constants/http_client.js';

// --- Service instances (locally-owned only) ---
let organizationModel = null;
let pubSubClient = null;
let cacheAsideService = null;
let webSessionService = null;
let cliSessionService = null;
let boundSessionsService = null;
let apiKeyService = null;
let userService = null;
let operatorServiceInstance = null;
let passkeyAuthService = null;
let loginSecurityService = null;
let downloadAuthService = null;
let attachmentService = null;
let sseService = null;
let certificateService = null;
let deviceLinkService = null;
let deviceRegistrationService = null;
let settingsService = null;
let g8eNodeOperatorService = null;
let consoleMetricsService = null;
let operatorDownloadService = null;
let postLoginService = null;
let setupService = null;
let auditService = null;
let internalHttpClientInstance = null;
let bindOperatorsServiceInstance = null;
let apiKeyDataService = null;
let operatorDataService = null;
let investigationService = null;
let healthCheckService = null;
let _sigTermHandler = null;
let _sigIntHandler = null;

let blobStorage = null;
let initialized = false;
let initializingPromise = null;

export async function initializeSettingsService() {
    if (settingsService) {
        return settingsService;
    }
    const listenUrl = G8ES_INTERNAL_HTTP_URL;
    const isHttps = listenUrl.startsWith('https://');
    
    // 1. Initialize bootstrap service for g8es volume data
    const bootstrapService = new BootstrapService();
    
    // 2. Load bootstrap secrets using the dedicated bootstrap service
    const internalAuthToken = bootstrapService.loadInternalAuthToken();
    const caCertPath = bootstrapService.loadCaCertPath();

    if (!internalAuthToken) {
        logger.error('[G8ED-INIT] Internal auth token not found in g8es volume');
        throw new Error('Internal auth token not found in g8es volume. Platform bootstrap failed.');
    }

    // Tamper-evidence: confirm the token read from the volume matches the
    // SHA-256 digest g8eo SecretManager recorded in bootstrap_digest.json at
    // write time. This is the only cryptographic link g8ed has to the
    // DB-authoritative value; without it, a divergent volume file would
    // surface as an opaque 401 during the first downstream API call instead
    // of a clear startup abort.
    bootstrapService.verifyAgainstManifest('internal_auth_token', internalAuthToken);

    if (isHttps && !caCertPath) {
        logger.error('[G8ED-INIT] CA certificate not found for HTTPS connection to g8es');
        throw new Error('CA certificate not found for HTTPS connection to g8es');
    }

    const { dbClient, kvClient } = await _initializeBaseClients(listenUrl, internalAuthToken, caCertPath);
    
    // 3. Wait for g8es to be ready before proceeding
    // This prevents race conditions during platform startup
    try {
        await dbClient.waitForReady();
    } catch (err) {
        logger.error('[G8ED-INIT] g8es connection failed', { error: err.message });
        throw err;
    }

    cacheAsideService = new CacheAsideService(kvClient, dbClient, SourceComponent.G8ED);

    settingsService = new SettingsService({
        cacheAsideService,
        bootstrapService
    });
    await settingsService.initialize();
    
    return settingsService;
}

export async function initializeServices() {
    if (initialized) {
        return;
    }
    if (initializingPromise) {
        return initializingPromise;
    }

    initializingPromise = _doInitialize();
    return initializingPromise;
}

async function _doInitialize() {
    try {
        logger.info('[G8ED-INIT] Starting service initialization');

        // --- Phase 1: g8es clients ---
        // settingsService.initialize() handles secret resolution and base clients via bootstrap logic.
        const settingsSvc = await initializeSettingsService();
        const bootstrapSvc = settingsSvc.getBootstrapService();
        const internalAuthToken = bootstrapSvc.loadInternalAuthToken();
        const caCertPath = bootstrapSvc.loadCaCertPath();
        
        const listenUrl = G8ES_INTERNAL_HTTP_URL;
        const pubsubUrl = G8ES_INTERNAL_PUBSUB_URL;

        // PubSub ALWAYS uses WSS.
        pubSubClient = new G8esPubSubClient({ 
            pubsubUrl, 
            caCertPath, 
            internalAuthToken 
        });
        blobStorage = new g8esBlobClient({ 
            baseUrl: listenUrl, 
            internalAuthToken
        });
        logger.info('[G8ED-INIT] Phase 1 complete: g8es clients');

        // --- Phase 2: Core platform ---
        // cacheAsideService is already initialized in initializeSettingsService()
        organizationModel = new OrganizationModel({ cacheAsideService });
        logger.info('[G8ED-INIT] Phase 2 complete: core platform (DB service, models, cache-aside)');

        // --- Phase 2b: Resolve all settings from g8es (DB > schema default) ---
        // settingsSvc is already the SettingsService instance

        // Reinitialize pubSubClient with the resolved CA cert path and internal auth token.
        await pubSubClient.terminate();
        pubSubClient = new G8esPubSubClient({
            pubsubUrl: settingsSvc.g8e_internal_pubsub_url || pubsubUrl,
            caCertPath: settingsSvc.g8e_pubsub_ca_cert || null,
            internalAuthToken: internalAuthToken
        });
        logger.info('[G8ED-INIT] Phase 2b complete: all settings resolved via SettingsService');

        // --- Phase 3: Session + auth services ---
        webSessionService = new WebSessionService({ 
            cacheAsideService, 
            bootstrapService: settingsSvc.getBootstrapService()
        });
        // auditService is required by CliSessionService; construct it here
        // (moved up from Phase 6) before dependent services are wired.
        auditService = new AuditService();
        cliSessionService = new CliSessionService({
            cacheAsideService,
            bootstrapService: settingsSvc.getBootstrapService(),
            auditService
        });
        apiKeyDataService = new ApiKeyDataService({ 
            cacheAsideService 
        });
        apiKeyService = new ApiKeyService({ 
            apiKeyDataService 
        });
        userService = new UserService({ 
            cacheAsideService, 
            organizationService: organizationModel, 
            apiKeyService: apiKeyService 
        });
        passkeyAuthService = new PasskeyAuthService({ 
            userService: userService, 
            cacheAsideService, 
            settingsService: settingsSvc 
        });
        loginSecurityService = new LoginSecurityService({ 
            cacheAsideService 
        });
        downloadAuthService = new DownloadAuthService({ 
            cacheAsideService, 
            userService: userService, 
            apiKeyService: apiKeyService 
        });
        logger.info('[G8ED-INIT] Phase 3 complete: auth services');

        // --- Phase 4: Platform services (moved up for dependency order) ---
        internalHttpClientInstance = new InternalHttpClient({ 
            bootstrapService: bootstrapSvc, 
            settingsService: settingsSvc 
        });
        sseService = new SSEService();

        await sseService.waitForReady();
        if (_sigTermHandler) process.removeListener('SIGTERM', _sigTermHandler);
        if (_sigIntHandler) process.removeListener('SIGINT', _sigIntHandler);
        _sigTermHandler = async () => {
            if (sseService) await sseService.close();
        };
        _sigIntHandler  = async () => {
            if (sseService) await sseService.close();
        };
        process.once('SIGTERM', _sigTermHandler);
        process.once('SIGINT',  _sigIntHandler);

        // --- Phase 5: Operator subsystem ---
        certificateService = new CertificateService({ bootstrapService: bootstrapSvc, internalHttpClient: internalHttpClientInstance });
        await certificateService.initialize();

        investigationService = new InvestigationService({
            cacheAsideService
        });

        operatorDataService = new OperatorDataService({ 
            cacheAsideService 
        });

        operatorServiceInstance = new OperatorService({
            operatorDataService,
            userService: userService,
            apiKeyService: apiKeyService,
            webSessionService: webSessionService,
            certificateService: certificateService,
            sseService: sseService,
            internalHttpClient: internalHttpClientInstance,
        });
        
        operatorDownloadService = new OperatorDownloadService(listenUrl, internalAuthToken);
        boundSessionsService = new BoundSessionsService({
            cacheAsideService,
            operatorService: operatorServiceInstance,
        });

        // Inject dependencies into SSEService after other services are ready
        sseService.setDependencies({
            settingsService: settingsSvc,
            internalHttpClient: internalHttpClientInstance,
            boundSessionsService: boundSessionsService,
            investigationService: investigationService
        });

        logger.info('[G8ED-INIT] Phase 5 complete: operator subsystem and SSE initialization');

        // --- Phase 6: Other Platform services ---
        attachmentService = new AttachmentService({ 
            cacheAsideService, 
            blobStorage: blobStorage 
        });
        consoleMetricsService = new ConsoleMetricsService({ 
            cacheAsideService, 
            internalHttpClient: internalHttpClientInstance 
        });
        healthCheckService = new HealthCheckService({ 
            cacheAsideService, 
            webSessionService: webSessionService 
        });
        
        g8eNodeOperatorService = new G8ENodeOperatorService({ 
            settingsService: settingsSvc, 
            operatorService: operatorServiceInstance,
            internalHttpClient: internalHttpClientInstance
        });

        postLoginService = new PostLoginService({
            webSessionService: webSessionService,
            apiKeyService: apiKeyService,
            userService: userService,
            operatorService: operatorServiceInstance,
            g8eNodeOperatorService: g8eNodeOperatorService,
            sseService: sseService,
            consoleMetricsService: consoleMetricsService,
        });
        
        // auditService already constructed in Phase 3 for CliSessionService

        deviceRegistrationService = new DeviceRegistrationService({
            operatorService: operatorServiceInstance,
            userService: userService,
            sseService: sseService,
            internalHttpClient: internalHttpClientInstance,
        });
        
        deviceLinkService = new DeviceLinkService({
            cacheAsideService,
            operatorService: operatorServiceInstance,
            webSessionService: webSessionService,
            deviceRegistrationService: deviceRegistrationService,
        });
        
        setupService = new SetupService({ 
            userService: userService, 
            settingsService: settingsSvc 
        });
        
        bindOperatorsServiceInstance = new BindOperatorsService({
            operatorService: operatorServiceInstance,
            bindingService: boundSessionsService,
            webSessionService: webSessionService,
            sseService: sseService,
        });

        logger.info('[G8ED-INIT] Phase 5 complete: platform services (SSE, attachments, device links, certificates, g8ep operator, console metrics, post-login, setup, audit, operator-bind)');

        // --- Phase 6: Configuration ---
        // All configuration is now available via settingsService
        logger.info('[G8ED-INIT] Phase 6 complete: configuration loaded');

        initialized = true;
        logger.info('[G8ED-INIT] All services initialized successfully');
        return;
    } catch (error) {
        initializingPromise = null;
        logger.error('[G8ED-INIT] Initialization failed', { error: error.message, stack: error.stack });
        throw error;
    }
}


/**
 * Shared logic for initializing core g8es clients.
 */
async function _initializeBaseClients(listenUrl, internalAuthToken, caCertPath = null) {
    const dbClient = new G8esDocumentClient({ listenUrl, internalAuthToken, caCertPath });
    logger.info(`[G8ES-CLIENT] Document client initialized for ${listenUrl}`);

    const kvClient = new KVCacheClient({ listenUrl, internalAuthToken, caCertPath });
    logger.info(`[G8ES-CLIENT] KV cache client initialized for ${listenUrl}`);

    return { dbClient, kvClient };
}

// --- Service accessors ---

export function getApiKeyDataService() {
    if (!apiKeyDataService) throw new Error('ApiKeyDataService not initialized. Call initializeServices() first.');
    return apiKeyDataService;
}

export function getOperatorDataService() {
    if (!operatorDataService) throw new Error('OperatorDataService not initialized. Call initializeServices() first.');
    return operatorDataService;
}

export function getOrganizationModel() {
    if (!organizationModel) throw new Error('OrganizationModel not initialized. Call initializeServices() first.');
    return organizationModel;
}

export function getPubSubClient() {
    if (!pubSubClient) throw new Error('PubSubClient not initialized. Call initializeServices() first.');
    return pubSubClient;
}

export function getCacheAsideService() {
    if (!cacheAsideService) throw new Error('CacheAsideService not initialized. Call initializeServices() first.');
    return cacheAsideService;
}

export function getWebSessionService() {
    if (!webSessionService) throw new Error('WebSessionService not initialized. Call initializeServices() first.');
    return webSessionService;
}

export function getCliSessionService() {
    if (!cliSessionService) throw new Error('CliSessionService not initialized. Call initializeServices() first.');
    return cliSessionService;
}

export function getBindingService() {
    if (!boundSessionsService) throw new Error('BoundSessionsService not initialized. Call initializeServices() first.');
    return boundSessionsService;
}

export function getApiKeyService() {
    if (!apiKeyService) throw new Error('ApiKeyService not initialized. Call initializeServices() first.');
    return apiKeyService;
}

export function getUserService() {
    if (!userService) throw new Error('UserService not initialized. Call initializeServices() first.');
    return userService;
}

export function getOperatorService() {
    if (!operatorServiceInstance) throw new Error('OperatorDataService not initialized. Call initializeServices() first.');
    return operatorServiceInstance;
}

export function getOperatorDownloadService() {
    if (!operatorDownloadService) throw new Error('OperatorDownloadService not initialized. Call initializeServices() first.');
    return operatorDownloadService;
}


export function getDownloadAuthService() {
    if (!downloadAuthService) throw new Error('DownloadAuthService not initialized. Call initializeServices() first.');
    return downloadAuthService;
}

export function getLoginSecurityService() {
    if (!loginSecurityService) throw new Error('LoginSecurityService not initialized. Call initializeServices() first.');
    return loginSecurityService;
}

export function getPasskeyAuthService() {
    if (!passkeyAuthService) throw new Error('PasskeyAuthService not initialized. Call initializeServices() first.');
    return passkeyAuthService;
}

export function getInvestigationService() {
    if (!investigationService) throw new Error('InvestigationService not initialized. Call initializeServices() first.');
    return investigationService;
}

export function getAttachmentService() {
    if (!attachmentService) throw new Error('AttachmentService not initialized. Call initializeServices() first.');
    return attachmentService;
}

export function getSSEService() {
    if (!sseService) throw new Error('SSEService not initialized. Call initializeServices() first.');
    return sseService;
}

export function getInternalHttpClient() {
    if (!internalHttpClientInstance) throw new Error('InternalHttpClient not initialized. Call initializeServices() first.');
    return internalHttpClientInstance;
}

export function getDeviceLinkService() {
    if (!deviceLinkService) throw new Error('DeviceLinkService not initialized. Call initializeServices() first.');
    return deviceLinkService;
}

export function getCertificateService() {
    if (!certificateService) throw new Error('CertificateService not initialized. Call initializeServices() first.');
    return certificateService;
}

export function getSettingsService() {
    if (!settingsService) throw new Error('SettingsService not initialized. Call initializeServices() first.');
    return settingsService;
}

export function getConsoleMetricsService() {
    if (!consoleMetricsService) throw new Error('ConsoleMetricsService not initialized. Call initializeServices() first.');
    return consoleMetricsService;
}

export function getBindOperatorsService() {
    if (!bindOperatorsServiceInstance) throw new Error('BindOperatorsService not initialized. Call initializeServices() first.');
    return bindOperatorsServiceInstance;
}

export function getG8ENodeOperatorService() {
    if (!g8eNodeOperatorService) throw new Error('G8ENodeOperatorService not initialized. Call initializeServices() first.');
    return g8eNodeOperatorService;
}

export function getPostLoginService() {
    if (!postLoginService) throw new Error('PostLoginService not initialized. Call initializeServices() first.');
    return postLoginService;
}

export function getDeviceRegistrationService() {
    if (!deviceRegistrationService) throw new Error('DeviceRegistrationService not initialized. Call initializeServices() first.');
    return deviceRegistrationService;
}

export function getAuditService() {
    if (!auditService) throw new Error('AuditService not initialized. Call initializeServices() first.');
    return auditService;
}

export function getSetupService() {
    if (!setupService) throw new Error('SetupService not initialized. Call initializeServices() first.');
    return setupService;
}

export function getG8esBlobClient() {
    if (!blobStorage) throw new Error('G8esBlobClient not initialized. Call initializeServices() first.');
    return blobStorage;
}

export function getHealthCheckService() {
    if (!healthCheckService) throw new Error('HealthCheckService not initialized. Call initializeServices() first.');
    return healthCheckService;
}

export function resetInitialization() {
    if (_sigTermHandler) { process.removeListener('SIGTERM', _sigTermHandler); _sigTermHandler = null; }
    if (_sigIntHandler)  { process.removeListener('SIGINT', _sigIntHandler);  _sigIntHandler = null; }
    initialized = false;
    initializingPromise = null;
    operatorDownloadService = null;
    passkeyAuthService = null;
    loginSecurityService = null;
    downloadAuthService = null;
    attachmentService = null;
    sseService = null;
    certificateService = null;
    deviceLinkService = null;
    deviceRegistrationService = null;
    settingsService = null;
    g8eNodeOperatorService = null;
    consoleMetricsService = null;
    operatorDownloadService = null;
    organizationModel = null;
    pubSubClient = null;
    webSessionService = null;
    boundSessionsService = null;
    apiKeyService = null;
    userService = null;
    operatorServiceInstance = null;
    postLoginService = null;
    setupService = null;
    auditService = null;
    internalHttpClientInstance = null;
    bindOperatorsServiceInstance = null;
    apiKeyDataService = null;
    operatorDataService = null;
    blobStorage = null;
    cacheAsideService = null;
    investigationService = null;
    healthCheckService = null;
}