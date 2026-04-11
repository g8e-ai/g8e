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
 * Request Timestamp Validation Middleware
 * 
 * Provides replay attack protection by validating request timestamps and nonces.
 * 
 * Security Features:
 * 1. Timestamp validation: Request timestamp must be within ±5 minutes of server time
 * 2. Nonce tracking: Optional nonce prevents exact request replay within time window
 * 
 * Headers:
 * - X-Request-Timestamp: ISO 8601 timestamp of when request was created (REQUIRED)
 * - X-Request-Nonce: Unique request identifier for replay prevention (OPTIONAL but recommended)
 * 
 * Usage:
 * - Apply to sensitive endpoints (operator auth, heartbeat, command execution)
 * - Rejects requests with missing/invalid timestamps
 * - Rejects duplicate nonces within time window
 */

import { ErrorResponse } from '../models/response_models.js';
import { logger } from '../utils/logger.js';
import { now, addSeconds, secondsBetween } from '../models/base.js';
import { NONCE_TTL_SECONDS, NONCE_CACHE_CLEANUP_INTERVAL_MS, TIMESTAMP_WINDOW_SECONDS } from '../constants/auth.js';
import { KVKey } from '../constants/kv_keys.js';

// In-memory nonce cache (for single-instance deployments)
// For production multi-instance, use VSODB KV
const nonceCache = new Map();

// Cleanup interval for in-memory cache
let cleanupInterval = null;

/**
 * Start nonce cache cleanup interval
 */
function startNonceCacheCleanup() {
    if (cleanupInterval) return;
    
    cleanupInterval = setInterval(() => {
        const expiredBefore = addSeconds(now(), -NONCE_TTL_SECONDS);
        
        for (const [nonce, usedAt] of nonceCache.entries()) {
            if (usedAt < expiredBefore) {
                nonceCache.delete(nonce);
            }
        }
    }, NONCE_CACHE_CLEANUP_INTERVAL_MS);
    
    cleanupInterval.unref();
}

/**
 * Check if nonce has been used (in-memory fallback)
 */
function isNonceUsedInMemory(nonce) {
    return nonceCache.has(nonce);
}

/**
 * Mark nonce as used (in-memory fallback)
 */
function markNonceUsedInMemory(nonce) {
    nonceCache.set(nonce, now());
}

/**
 * Validate request timestamp is within acceptable window
 * @param {string} timestampStr - ISO 8601 timestamp string
 * @returns {{valid: boolean, error?: string, skewMs?: number}}
 */
function validateTimestamp(timestampStr) {
    if (!timestampStr) {
        return { valid: false, error: 'Missing X-Request-Timestamp header' };
    }
    
    const requestTime = new Date(timestampStr);
    
    if (isNaN(requestTime.getTime())) {
        return { valid: false, error: 'Invalid timestamp format (use ISO 8601)' };
    }
    
    const serverTime = now();
    const earliest = addSeconds(serverTime, -TIMESTAMP_WINDOW_SECONDS);
    const latest = addSeconds(serverTime, TIMESTAMP_WINDOW_SECONDS);
    
    if (requestTime < earliest || requestTime > latest) {
        const skewSeconds = Math.abs(secondsBetween(serverTime, requestTime));
        return {
            valid: false,
            error: `Request timestamp outside acceptable window (${skewSeconds}s skew, max ${TIMESTAMP_WINDOW_SECONDS}s)`,
        };
    }
    
    return { valid: true };
}

/**
 * Request Timestamp Validation Middleware Factory
 * 
 * Provides replay attack protection by validating request timestamps and nonces.
 * 
 * @param {Object} options
 * @param {Object} [options.cacheAsideService] - CacheAsideService instance for distributed nonce tracking
 * @returns {Object} Collection of request timestamp middleware
 */
export function createRequestTimestampMiddleware({ cacheAsideService = null } = {}) {
    // Start cleanup if using in-memory cache
    if (!cacheAsideService) {
        startNonceCacheCleanup();
    }

    /**
     * Require request timestamp validation middleware
     * Rejects requests with missing/invalid timestamps or replayed nonces.
     * 
     * @param {Object} options - Configuration options
     * @param {boolean} options.requireNonce - Whether to require nonce (default: false)
     */
    const requireRequestTimestamp = (options = {}) => {
        const { requireNonce = false } = options;
        
        return async (req, res, next) => {
            const timestamp = req.headers['x-request-timestamp'];
            const nonce = req.headers['x-request-nonce'];
            const apiKeyPrefix = req.headers.authorization 
                ? req.headers.authorization.substring(7, 17) + '...'
                : 'none';
            
            // Step 1: Validate timestamp
            const timestampResult = validateTimestamp(timestamp);
            
            if (!timestampResult.valid) {
                logger.warn('[REQUEST-TIMESTAMP] Request rejected - invalid timestamp', {
                    path: req.path,
                    method: req.method,
                    ip: req.ip,
                    error: timestampResult.error,
                    provided_timestamp: timestamp,
                    api_key_prefix: apiKeyPrefix,
                    security_event: 'replay_protection_timestamp_rejected'
                });
                
                return res.status(400).json(new ErrorResponse({
                    error: 'Invalid request timestamp'
                }).forWire());
            }
            
            // Step 2: Validate nonce (if provided or required)
            if (nonce) {
                let isUsed = false;
                
                if (cacheAsideService) {
                    // Use VSODB KV for distributed nonce tracking via CacheAsideService
                    try {
                        const nonceKey = KVKey.nonce(nonce);
                        const exists = await cacheAsideService.kvGet(nonceKey);
                        
                        if (exists) {
                            isUsed = true;
                        } else {
                            await cacheAsideService.kvSetex(nonceKey, NONCE_TTL_SECONDS, '1');
                        }
                    } catch (kvError) {
                        logger.warn('[REQUEST-TIMESTAMP] VSODB KV nonce check failed, using in-memory', {
                            error: kvError.message
                        });
                        // Fallback to in-memory
                        isUsed = isNonceUsedInMemory(nonce);
                        if (!isUsed) {
                            markNonceUsedInMemory(nonce);
                        }
                    }
                } else {
                    // Use in-memory nonce tracking
                    isUsed = isNonceUsedInMemory(nonce);
                    if (!isUsed) {
                        markNonceUsedInMemory(nonce);
                    }
                }
                
                if (isUsed) {
                    logger.error('[REQUEST-TIMESTAMP] Request rejected - nonce replay detected', {
                        path: req.path,
                        method: req.method,
                        ip: req.ip,
                        nonce_prefix: nonce.substring(0, 16) + '...',
                        api_key_prefix: apiKeyPrefix,
                        security_event: 'replay_attack_detected'
                    });
                    
                    return res.status(400).json(new ErrorResponse({
                        error: 'Request replay detected'
                    }).forWire());
                }
            } else if (requireNonce) {
                logger.warn('[REQUEST-TIMESTAMP] Request rejected - missing required nonce', {
                    path: req.path,
                    method: req.method,
                    ip: req.ip,
                    api_key_prefix: apiKeyPrefix,
                    security_event: 'missing_nonce'
                });
                
                return res.status(400).json(new ErrorResponse({
                    error: 'Missing request nonce'
                }).forWire());
            }
            
            // Attach validation info to request for downstream use
            req.requestTimestamp = {
                timestamp: timestamp ? new Date(timestamp) : null,
                nonce: nonce || null
            };
            
            logger.info('[REQUEST-TIMESTAMP] Request timestamp validated', {
                path: req.path,
                has_nonce: !!nonce
            });
            
            next();
        };
    };

    /**
     * Optional request timestamp validation
     * Validates timestamp if provided, but doesn't reject if missing.
     */
    const optionalRequestTimestamp = () => {
        return async (req, res, next) => {
            const timestamp = req.headers['x-request-timestamp'];
            
            if (!timestamp) {
                // No timestamp provided - continue without validation
                // Log for monitoring gradual adoption
                logger.info('[REQUEST-TIMESTAMP] No timestamp provided (optional)', {
                    path: req.path,
                    method: req.method
                });
                return next();
            }
            
            // Timestamp provided - validate it
            const timestampResult = validateTimestamp(timestamp);
            
            if (!timestampResult.valid) {
                logger.warn('[REQUEST-TIMESTAMP] Optional timestamp invalid', {
                    path: req.path,
                    error: timestampResult.error,
                    provided_timestamp: timestamp
                });
                // For optional validation, log but don't reject
                // This allows gradual rollout
            }
            
            // Attach validation info
            req.requestTimestamp = {
                timestamp: timestamp ? new Date(timestamp) : null,
                skewMs: timestampResult.skewMs,
                valid: timestampResult.valid
            };
            
            next();
        };
    };

    return {
        requireRequestTimestamp,
        optionalRequestTimestamp
    };
}
