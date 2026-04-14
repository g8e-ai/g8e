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
 * Rate limiting Middleware
 * Protects against brute force and DoS attacks
 */

import { ErrorResponse } from '../models/response_models.js';
import rateLimit from 'express-rate-limit';
import { logger } from '../utils/logger.js';
import { redactWebSessionId } from '../utils/security.js';
import { BEARER_PREFIX } from '../constants/auth.js';
import {
    GlobalPublicRateLimit,
    UserAuthRateLimit,
    ChatRateLimit,
    SSERateLimit,
    ApiRateLimit,
    UploadRateLimit,
    OperatorRefreshRateLimit,
    AuthRateLimit,
    OperatorApiRateLimit,
    AuditRateLimit,
    ConsoleRateLimit,
    DeviceLinkRateLimit,
    DeviceLinkGenerateRateLimit,
    DeviceLinkListRateLimit,
    DeviceLinkRevokeRateLimit,
    SettingsRateLimit,
    PasskeyRateLimit,
    RateLimitError,
} from '../constants/rate_limits.js';
import { WEB_SESSION_ID_HEADER } from '../constants/auth.js';

/**
 * Extract API key prefix for rate limiting
 * @param {Object} req - Express request
 * @returns {string|null} API key prefix or null
 */
function extractApiKeyForRateLimit(req) {
    const authHeader = req.headers.authorization;
    if (!authHeader || !authHeader.startsWith(BEARER_PREFIX)) {
        return null;
    }
    const apiKey = authHeader.substring(BEARER_PREFIX.length);
    if (!apiKey) {
        return null;
    }
    // Return first 16 chars as prefix for rate limiting
    return apiKey.substring(0, 16);
}

/**
 * Rate limiting Middleware Factory
 * Protects against brute force and DoS attacks
 * 
 * @param {Object} options
 * @param {Object} [options.config] - Platform config
 * @returns {Object} Collection of rate limiter middleware
 */
export function createRateLimiters({ config = {} } = {}) {
    /**
     * Global rate limiter for ALL public API endpoints
     * Applied at the app level before route handlers
     * 100 requests per minute per IP across all endpoints
     */
    const globalPublicRateLimiter = rateLimit({
        windowMs: GlobalPublicRateLimit.WINDOW_MS,
        max: GlobalPublicRateLimit.MAX,
        message: {
            success: false,
            error: RateLimitError.GENERIC
        },
        standardHeaders: true,
        handler: (req, res) => {
            logger.warn('[RATE-LIMIT] Global rate limit exceeded', {
                ip: req.ip,
                path: req.path,
                method: req.method,
                userAgent: req.headers['user-agent']
            });
            res.status(429).json(new ErrorResponse({
                error: RateLimitError.GENERIC
            }).forWire());
        }
    });

    /**
     * Rate limiter for authentication endpoints
     * Balanced limits to prevent brute force while allowing legitimate retries
     */
    const authRateLimiter = rateLimit({
        windowMs: UserAuthRateLimit.WINDOW_MS,
        max: UserAuthRateLimit.MAX,
        message: {
            success: false,
            error: RateLimitError.AUTH
        },
        standardHeaders: true,
        handler: (req, res) => {
            logger.warn('[RATE-LIMIT] Auth rate limit exceeded', {
                ip: req.ip,
                path: req.path,
                userAgent: req.headers['user-agent']
            });
            res.status(429).json(new ErrorResponse({
                error: RateLimitError.AUTH
            }).forWire());
        }
    });

    /**
     * Rate limiter for chat/message endpoints
     * Moderate limits to prevent spam
     */
    const chatRateLimiter = rateLimit({
        windowMs: ChatRateLimit.WINDOW_MS,
        max: ChatRateLimit.MAX,
        message: {
            success: false,
            error: RateLimitError.CHAT_SLOW
        },
        standardHeaders: true,
        handler: (req, res) => {
            const webSessionId = req.headers[WEB_SESSION_ID_HEADER] || req.body?.web_session_id;
            logger.warn('[RATE-LIMIT] Chat rate limit exceeded', {
                webSessionId: redactWebSessionId(webSessionId),
                ip: req.ip,
                path: req.path
            });
            res.status(429).json(new ErrorResponse({
                error: RateLimitError.CHAT_WAIT
            }).forWire());
        }
    });

    /**
     * Rate limiter for SSE connection attempts
     * Prevents connection spam
     */
    const sseRateLimiter = rateLimit({
        windowMs: SSERateLimit.WINDOW_MS,
        max: SSERateLimit.MAX,
        message: {
            success: false,
            error: RateLimitError.SSE_ATTEMPTS
        },
        standardHeaders: true,
        skipSuccessfulRequests: true, // Only count failed attempts
        handler: (req, res) => {
            const webSessionId = req.query.webSessionId;
            logger.warn('[RATE-LIMIT] SSE connection rate limit exceeded', {
                webSessionId: redactWebSessionId(webSessionId),
                ip: req.ip
            });
            res.status(429).send(RateLimitError.SSE_ATTEMPTS_WAIT);
        }
    });

    /**
     * Rate limiter for general API endpoints
     * Moderate limits for normal operations
     */
    const apiRateLimiter = rateLimit({
        windowMs: ApiRateLimit.WINDOW_MS,
        max: ApiRateLimit.MAX,
        message: {
            success: false,
            error: RateLimitError.GENERIC
        },
        standardHeaders: true,
        handler: (req, res) => {
            logger.warn('[RATE-LIMIT] API rate limit exceeded', {
                ip: req.ip,
                path: req.path,
                method: req.method
            });
            res.status(429).json(new ErrorResponse({
                error: RateLimitError.RATE_EXCEEDED
            }).forWire());
        }
    });

    /**
     * Rate limiter for file upload endpoints
     * Strict limits to prevent storage abuse
     */
    const uploadRateLimiter = rateLimit({
        windowMs: UploadRateLimit.WINDOW_MS,
        max: UploadRateLimit.MAX,
        message: {
            success: false,
            error: RateLimitError.UPLOAD
        },
        standardHeaders: true,
        handler: (req, res) => {
            const webSessionId = req.headers[WEB_SESSION_ID_HEADER];
            logger.warn('[RATE-LIMIT] Upload rate limit exceeded', {
                webSessionId: redactWebSessionId(webSessionId),
                ip: req.ip
            });
            res.status(429).json(new ErrorResponse({
                error: RateLimitError.UPLOAD_WAIT
            }).forWire());
        }
    });

    /**
     * Rate limiter for Operator session refresh
     * Moderate limit - g8eo refreshes periodically but shouldn't be too frequent
     * 10 refreshes per minute per IP (allows for multiple operators)
     */
    const operatorRefreshRateLimiter = rateLimit({
        windowMs: OperatorRefreshRateLimit.WINDOW_MS,
        max: OperatorRefreshRateLimit.MAX,
        message: {
            success: false,
            error: RateLimitError.REFRESH
        },
        standardHeaders: true,
        handler: (req, res) => {
            logger.warn('[RATE-LIMIT] Operator refresh rate limit exceeded', {
                ip: req.ip,
                operatorSessionId: req.body?.operator_session_id?.substring(0, 12) + '...'
            });
            res.status(429).json(new ErrorResponse({
                error: RateLimitError.REFRESH_WAIT
            }).forWire());
        }
    });

    /**
     * Rate limiter for Operator authentication (POST /api/auth/operator)
     */
    const operatorAuthRateLimiter = rateLimit({
        windowMs: AuthRateLimit.WINDOW_MS,
        max: AuthRateLimit.MAX_PER_KEY,
        keyGenerator: (req) => {
            const apiKeyPrefix = extractApiKeyForRateLimit(req);
            // Use API key prefix if available, otherwise fall back to IP
            return apiKeyPrefix ? `apikey:${apiKeyPrefix}` : `ip:${req.ip}`;
        },
        message: {
            success: false,
            error: RateLimitError.AUTH_OPERATOR
        },
        standardHeaders: true,
        // IP is intentional fallback, not primary key - disable IPv6 validation
        validate: { keyGeneratorIpFallback: false },
        handler: (req, res) => {
            const apiKeyPrefix = extractApiKeyForRateLimit(req);
            logger.warn('[RATE-LIMIT] Operator auth rate limit exceeded', {
                ip: req.ip,
                keyType: apiKeyPrefix ? 'api_key' : 'ip',
                apiKeyPrefix: apiKeyPrefix ? apiKeyPrefix.substring(0, 12) + '...' : null,
                path: req.path,
                userAgent: req.headers['user-agent']
            });
            res.status(429).json(new ErrorResponse({
                error: RateLimitError.AUTH_OPERATOR
            }).forWire());
        }
    });

    /**
     * Additional IP-based rate limiter for Operator auth as a global backstop
     */
    const operatorAuthIpBackstopLimiter = rateLimit({
        windowMs: AuthRateLimit.WINDOW_MS,
        max: AuthRateLimit.MAX_PER_IP,
        message: {
            success: false,
            error: RateLimitError.AUTH_IP
        },
        standardHeaders: true,
        handler: (req, res) => {
            logger.error('[RATE-LIMIT] Operator auth IP backstop limit exceeded - possible attack', {
                ip: req.ip,
                path: req.path,
                userAgent: req.headers['user-agent']
            });
            res.status(429).json(new ErrorResponse({
                error: RateLimitError.AUTH_IP
            }).forWire());
        }
    });

    /**
     * Rate limiter for Audit Log page and API endpoints
     */
    const auditRateLimiter = rateLimit({
        windowMs: AuditRateLimit.WINDOW_MS,
        max: AuditRateLimit.MAX,
        keyGenerator: (req) => req.userId || req.ip, // Per user, fallback to IP
        message: {
            success: false,
            error: RateLimitError.AUDIT_SLOW
        },
        standardHeaders: true,
        validate: { keyGeneratorIpFallback: false },
        handler: (req, res) => {
            logger.warn('[RATE-LIMIT] Audit rate limit exceeded', {
                ip: req.ip,
                userId: req.userId,
                path: req.path,
                method: req.method
            });
            res.status(429).json(new ErrorResponse({
                error: RateLimitError.AUDIT_WAIT
            }).forWire());
        }
    });

    /**
     * Rate limiter for Console endpoints
     */
    const consoleRateLimiter = rateLimit({
        windowMs: ConsoleRateLimit.WINDOW_MS,
        max: ConsoleRateLimit.MAX,
        keyGenerator: (req) => req.userId || req.ip, // Per user, fallback to IP
        message: {
            success: false,
            error: RateLimitError.CONSOLE_SLOW
        },
        standardHeaders: true,
        // IP is intentional fallback, not primary key - disable IPv6 validation
        validate: { keyGeneratorIpFallback: false },
        handler: (req, res) => {
            logger.warn('[RATE-LIMIT] Console rate limit exceeded', {
                ip: req.ip,
                userId: req.userId,
                path: req.path,
                method: req.method
            });
            res.status(429).json(new ErrorResponse({
                error: RateLimitError.CONSOLE_WAIT
            }).forWire());
        }
    });

    /**
     * Operator API rate limiter
     */
    const operatorApiRateLimiter = rateLimit({
        windowMs: OperatorApiRateLimit.WINDOW_MS,
        max: OperatorApiRateLimit.MAX,
        keyGenerator: (req) => req.apiKey || req.ip,
        standardHeaders: true,
        validate: { keyGeneratorIpFallback: false },
        message: {
            success: false,
            error: RateLimitError.RATE_EXCEEDED_WAIT
        },
        handler: (req, res) => {
            logger.warn('[RATE-LIMIT] Operator API rate limit exceeded', {
                ip: req.ip,
                path: req.path,
                userId: req.userId
            });
            res.status(429).json(new ErrorResponse({
                error: RateLimitError.RATE_EXCEEDED_WAIT
            }).forWire());
        }
    });

    /**
     * Rate limiter for device link register endpoint (public)
     */
    const deviceLinkRateLimiter = rateLimit({
        windowMs: DeviceLinkRateLimit.WINDOW_MS,
        max: DeviceLinkRateLimit.MAX,
        message: {
            success: false,
            error: RateLimitError.DEVICE_LINK
        },
        standardHeaders: true,
        handler: (req, res) => {
            logger.warn('[RATE-LIMIT] Device link register rate limit exceeded', {
                ip: req.ip,
                path: req.path
            });
            res.status(429).json(new ErrorResponse({
                error: RateLimitError.DEVICE_LINK_REGISTER
            }).forWire());
        }
    });

    /**
     * Rate limiter for device link generation (authenticated, operator-specific)
     */
    const deviceLinkGenerateLimiter = rateLimit({
        windowMs: DeviceLinkGenerateRateLimit.WINDOW_MS,
        max: DeviceLinkGenerateRateLimit.MAX,
        keyGenerator: (req) => req.userId || req.ip,
        message: {
            success: false,
            error: RateLimitError.DEVICE_LINK_CREATE
        },
        standardHeaders: true,
        validate: { keyGeneratorIpFallback: false },
        handler: (req, res) => {
            logger.warn('[RATE-LIMIT] Device link generate rate limit exceeded', {
                ip: req.ip,
                userId: req.userId,
                path: req.path
            });
            res.status(429).json(new ErrorResponse({
                error: RateLimitError.DEVICE_LINK_CREATE_WAIT
            }).forWire());
        }
    });

    /**
     * Rate limiter for device link creation (authenticated)
     */
    const deviceLinkCreateRateLimiter = rateLimit({
        windowMs: DeviceLinkRateLimit.WINDOW_MS,
        max: 5,
        keyGenerator: (req) => req.userId || req.ip,
        message: {
            success: false,
            error: RateLimitError.DEVICE_LINK_CREATE
        },
        standardHeaders: true,
        validate: { keyGeneratorIpFallback: false },
        handler: (req, res) => {
            logger.warn('[RATE-LIMIT] Device link creation rate limit exceeded', {
                ip: req.ip,
                userId: req.userId,
                path: req.path
            });
            res.status(429).json(new ErrorResponse({
                error: RateLimitError.DEVICE_LINK_CREATE_WAIT
            }).forWire());
        }
    });

    /**
     * Rate limiter for device link listing (authenticated)
     */
    const deviceLinkListRateLimiter = rateLimit({
        windowMs: DeviceLinkListRateLimit.WINDOW_MS,
        max: DeviceLinkListRateLimit.MAX,
        keyGenerator: (req) => req.userId || req.ip,
        message: {
            success: false,
            error: RateLimitError.GENERIC
        },
        standardHeaders: true,
        validate: { keyGeneratorIpFallback: false },
        handler: (req, res) => {
            logger.warn('[RATE-LIMIT] Device link list rate limit exceeded', {
                ip: req.ip,
                userId: req.userId,
                path: req.path
            });
            res.status(429).json(new ErrorResponse({
                error: RateLimitError.GENERIC_WAIT
            }).forWire());
        }
    });

    /**
     * Rate limiter for device link revocation (authenticated)
     */
    const deviceLinkRevokeRateLimiter = rateLimit({
        windowMs: DeviceLinkRevokeRateLimit.WINDOW_MS,
        max: DeviceLinkRevokeRateLimit.MAX,
        keyGenerator: (req) => req.userId || req.ip,
        message: {
            success: false,
            error: RateLimitError.GENERIC
        },
        standardHeaders: true,
        validate: { keyGeneratorIpFallback: false },
        handler: (req, res) => {
            logger.warn('[RATE-LIMIT] Device link revoke rate limit exceeded', {
                ip: req.ip,
                userId: req.userId,
                path: req.path
            });
            res.status(429).json(new ErrorResponse({
                error: RateLimitError.GENERIC_WAIT
            }).forWire());
        }
    });

    /**
     * Rate limiter for Settings API endpoints
     */
    const settingsRateLimiter = rateLimit({
        windowMs: SettingsRateLimit.WINDOW_MS,
        max: SettingsRateLimit.MAX,
        keyGenerator: (req) => req.userId || req.ip,
        message: {
            success: false,
            error: RateLimitError.SETTINGS_SLOW
        },
        standardHeaders: true,
        validate: { keyGeneratorIpFallback: false },
        handler: (req, res) => {
            logger.warn('[RATE-LIMIT] Settings rate limit exceeded', {
                ip: req.ip,
                userId: req.userId,
                path: req.path,
                method: req.method
            });
            res.status(429).json(new ErrorResponse({
                error: RateLimitError.SETTINGS_WAIT
            }).forWire());
        }
    });

    /**
     * Rate limiter for passkey auth endpoints
     */
    const passkeyRateLimiter = rateLimit({
        windowMs: PasskeyRateLimit.WINDOW_MS,
        max: PasskeyRateLimit.MAX,
        message: {
            success: false,
            error: RateLimitError.AUTH
        },
        standardHeaders: true,
        handler: (req, res) => {
            logger.warn('[RATE-LIMIT] Passkey rate limit exceeded', {
                ip: req.ip,
                path: req.path,
                userAgent: req.headers['user-agent']
            });
            res.status(429).json(new ErrorResponse({
                error: RateLimitError.AUTH
            }).forWire());
        }
    });

    return {
        globalPublicRateLimiter,
        authRateLimiter,
        chatRateLimiter,
        sseRateLimiter,
        apiRateLimiter,
        uploadRateLimiter,
        operatorRefreshRateLimiter,
        operatorAuthRateLimiter,
        operatorAuthIpBackstopLimiter,
        auditRateLimiter,
        consoleRateLimiter,
        operatorApiRateLimiter,
        deviceLinkRateLimiter,
        deviceLinkGenerateLimiter,
        deviceLinkCreateRateLimiter,
        deviceLinkListRateLimiter,
        deviceLinkRevokeRateLimiter,
        settingsRateLimiter,
        passkeyRateLimiter
    };
}

