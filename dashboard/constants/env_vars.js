// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * g8ed Constants - Environment Variables & Configuration
 * 
 * Centralized configuration management for g8ed.
 * Provides single source of truth for all environment-based configuration.
 */

import { DEFAULT_LOG_LEVEL } from './service_config.js';

// ---------------------------------------------------------------------------
// Environment Variable Configuration
// ---------------------------------------------------------------------------

export const g8edEnvConfig = {
    // Logging Configuration
    LOG_LEVEL: process.env.LOG_LEVEL || DEFAULT_LOG_LEVEL,
    ENABLE_CONSOLE_LOGGING: process.env.ENABLE_CONSOLE_LOGGING !== 'false',
    ENABLE_FILE_LOGGING: process.env.ENABLE_FILE_LOGGING !== 'false',

    // g8eg Configuration
    G8E_INTERNAL_HTTP_URL: process.env.G8E_INTERNAL_HTTP_URL || 'https://g8eg:8443',
    G8E_SESSION_ENCRYPTION_KEY: process.env.G8E_SESSION_ENCRYPTION_KEY || null,
    g8eg_VOLUME_PATH: process.env.g8eg_VOLUME_PATH || '/g8eg',

    // App Enrollment (server-to-server mTLS identity)
    // The gateway's plain-HTTP bootstrap surface for AppEnrollmentService
    // (CA bundle fetch + /api/v1/pki/apps/enroll). Required — fail closed
    // if unset. Compose sets http://g8eg:8080.
    G8E_GATEWAY_HTTP_URL: process.env.G8E_GATEWAY_HTTP_URL || null,
    // The dashboard's own runtime directory. The enrollment service writes
    // pki/issued/apps/g8ed.crt and friends under here. Compose sets
    // /data (the g8e-dashboard-data volume mount root). The dashboard
    // container runs as the non-root g8e user (UID 1001), so /data is used
    // instead of /root/.g8e (which is not writable by the g8e user).
    G8E_RUNTIME_DIR: process.env.G8E_RUNTIME_DIR || null,

    // Network Configuration
    g8ed_HOST: process.env.g8ed_HOST || '0.0.0.0',
    g8ed_PORT: parseInt(process.env.g8ed_PORT || '3000', 10),
    ENABLE_CORS: process.env.ENABLE_CORS !== 'false',

    // Security Configuration
    SSL_CERT_PATH: process.env.SSL_CERT_PATH || '/g8eg/.g8e/pki/issued/hub/g8eg.crt',
    SSL_KEY_PATH: process.env.SSL_KEY_PATH || '/g8eg/.g8e/pki/issued/hub/g8eg.key',
    CA_CERT_PATH: process.env.CA_CERT_PATH || '/g8eg/.g8e/pki/trust/g8eg-ca-bundle.pem',

    // Development Configuration
    ENABLE_DEV_LOGS: process.env.ENABLE_DEV_LOGS === 'true',
    MOCK_EXTERNAL_SERVICES: process.env.MOCK_EXTERNAL_SERVICES === 'true'
};

// ---------------------------------------------------------------------------
// Legacy Compatibility (Deprecated)
// ---------------------------------------------------------------------------

export const MAPPINGS = Object.freeze({});
export const EnvVar = Object.freeze({});
export default EnvVar;
