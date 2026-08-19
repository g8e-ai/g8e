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

    // g8edB Configuration
    DROPOPS_INTERNAL_HTTP_URL: process.env.DROPOPS_INTERNAL_HTTP_URL || 'https://g8edb:8443',
    DROPOPS_SESSION_ENCRYPTION_KEY: process.env.DROPOPS_SESSION_ENCRYPTION_KEY || null,
    g8edB_VOLUME_PATH: process.env.g8edB_VOLUME_PATH || '/g8edb',

    // Network Configuration
    g8ed_HOST: process.env.g8ed_HOST || '0.0.0.0',
    g8ed_PORT: parseInt(process.env.g8ed_PORT || '3000', 10),
    ENABLE_CORS: process.env.ENABLE_CORS !== 'false',

    // Security Configuration
    SSL_CERT_PATH: process.env.SSL_CERT_PATH || '/g8edb/.g8e/pki/issued/hub/g8eg.crt',
    SSL_KEY_PATH: process.env.SSL_KEY_PATH || '/g8edb/.g8e/pki/issued/hub/g8eg.key',
    CA_CERT_PATH: process.env.CA_CERT_PATH || '/g8edb/.g8e/pki/trust/g8eg-ca-bundle.pem',

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
