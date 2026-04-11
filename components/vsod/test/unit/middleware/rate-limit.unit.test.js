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

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { createRateLimiters } from '@vsod/middleware/rate-limit.js';
import { RateLimitError } from '@vsod/constants/rate_limits.js';
import { WEB_SESSION_ID_HEADER, BEARER_PREFIX } from '@vsod/constants/auth.js';
import { logger } from '@vsod/utils/logger.js';
import { redactWebSessionId } from '@vsod/utils/security.js';

// Mock express-rate-limit
vi.mock('express-rate-limit', () => {
    return {
        default: vi.fn((options) => {
            const middleware = (req, res, next) => {
                next();
            };
            middleware.options = options;
            return middleware;
        })
    };
});

// Mock logger
vi.mock('@vsod/utils/logger.js', () => ({
    logger: {
        warn: vi.fn(),
        error: vi.fn()
    }
}));

// Mock security
vi.mock('@vsod/utils/security.js', () => ({
    redactWebSessionId: vi.fn((id) => {
        if (!id || typeof id !== 'string') {
            return '[invalid]';
        }
        if (id.length <= 15) {
            return id;
        }
        return id.substring(0, 15) + '...';
    })
}));

describe('RateLimit Middleware', () => {
    let limiters;
    let req;
    let res;

    beforeEach(() => {
        limiters = createRateLimiters();
        req = {
            ip: '127.0.0.1',
            path: '/api/test',
            method: 'POST',
            headers: {},
            body: {},
            query: {}
        };
        res = {
            status: vi.fn().mockReturnThis(),
            json: vi.fn().mockReturnThis(),
            send: vi.fn().mockReturnThis()
        };
        vi.clearAllMocks();
    });

    describe('globalPublicRateLimiter', () => {
        it('should configure handler with correct error response', () => {
            const limiter = limiters.globalPublicRateLimiter;
            expect(limiter.options.handler).toBeDefined();

            limiter.options.handler(req, res);
            expect(res.status).toHaveBeenCalledWith(429);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: RateLimitError.GENERIC
            }));
        });

        it('should enable standardHeaders', () => {
            const limiter = limiters.globalPublicRateLimiter;
            expect(limiter.options.standardHeaders).toBe(true);
        });

        it('should log warning with IP, path, method, and userAgent', () => {
            const limiter = limiters.globalPublicRateLimiter;
            req.headers['user-agent'] = 'test-agent';

            limiter.options.handler(req, res);
            expect(logger.warn).toHaveBeenCalledWith(
                '[RATE-LIMIT] Global rate limit exceeded',
                expect.objectContaining({
                    ip: '127.0.0.1',
                    path: '/api/test',
                    method: 'POST',
                    userAgent: 'test-agent'
                })
            );
        });
    });

    describe('authRateLimiter', () => {
        it('should configure handler with correct error response', () => {
            const limiter = limiters.authRateLimiter;
            limiter.options.handler(req, res);
            expect(res.status).toHaveBeenCalledWith(429);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: RateLimitError.AUTH
            }));
        });

        it('should enable standardHeaders', () => {
            const limiter = limiters.authRateLimiter;
            expect(limiter.options.standardHeaders).toBe(true);
        });

        it('should log warning with IP, path, and userAgent', () => {
            const limiter = limiters.authRateLimiter;
            req.headers['user-agent'] = 'test-agent';

            limiter.options.handler(req, res);
            expect(logger.warn).toHaveBeenCalledWith(
                '[RATE-LIMIT] Auth rate limit exceeded',
                expect.objectContaining({
                    ip: '127.0.0.1',
                    path: '/api/test',
                    userAgent: 'test-agent'
                })
            );
        });
    });

    describe('chatRateLimiter', () => {
        it('should configure handler with correct error response', () => {
            const limiter = limiters.chatRateLimiter;
            req.headers[WEB_SESSION_ID_HEADER] = 'session-123';

            limiter.options.handler(req, res);
            expect(res.status).toHaveBeenCalledWith(429);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: RateLimitError.CHAT_WAIT
            }));
        });

        it('should enable standardHeaders', () => {
            const limiter = limiters.chatRateLimiter;
            expect(limiter.options.standardHeaders).toBe(true);
        });

        it('should extract webSessionId from header', () => {
            const limiter = limiters.chatRateLimiter;
            req.headers[WEB_SESSION_ID_HEADER] = 'session-123';

            limiter.options.handler(req, res);
            expect(redactWebSessionId).toHaveBeenCalledWith('session-123');
        });

        it('should extract webSessionId from body when header missing', () => {
            const limiter = limiters.chatRateLimiter;
            req.body.web_session_id = 'session-456';

            limiter.options.handler(req, res);
            expect(redactWebSessionId).toHaveBeenCalledWith('session-456');
        });

        it('should log warning with redacted webSessionId, IP, and path', () => {
            const limiter = limiters.chatRateLimiter;
            req.headers[WEB_SESSION_ID_HEADER] = 'session-123';

            limiter.options.handler(req, res);
            expect(logger.warn).toHaveBeenCalledWith(
                '[RATE-LIMIT] Chat rate limit exceeded',
                expect.objectContaining({
                    webSessionId: 'session-123',
                    ip: '127.0.0.1',
                    path: '/api/test'
                })
            );
        });
    });

    describe('sseRateLimiter', () => {
        it('should configure handler with correct error response', () => {
            const limiter = limiters.sseRateLimiter;
            req.query.webSessionId = 'session-123';

            limiter.options.handler(req, res);
            expect(res.status).toHaveBeenCalledWith(429);
            expect(res.send).toHaveBeenCalledWith(RateLimitError.SSE_ATTEMPTS_WAIT);
        });

        it('should enable standardHeaders', () => {
            const limiter = limiters.sseRateLimiter;
            expect(limiter.options.standardHeaders).toBe(true);
        });

        it('should skip successful requests', () => {
            const limiter = limiters.sseRateLimiter;
            expect(limiter.options.skipSuccessfulRequests).toBe(true);
        });

        it('should extract webSessionId from query params', () => {
            const limiter = limiters.sseRateLimiter;
            req.query.webSessionId = 'session-123';

            limiter.options.handler(req, res);
            expect(redactWebSessionId).toHaveBeenCalledWith('session-123');
        });

        it('should log warning with redacted webSessionId and IP', () => {
            const limiter = limiters.sseRateLimiter;
            req.query.webSessionId = 'session-123';

            limiter.options.handler(req, res);
            expect(logger.warn).toHaveBeenCalledWith(
                '[RATE-LIMIT] SSE connection rate limit exceeded',
                expect.objectContaining({
                    webSessionId: 'session-123',
                    ip: '127.0.0.1'
                })
            );
        });
    });

    describe('apiRateLimiter', () => {
        it('should configure handler with correct error response', () => {
            const limiter = limiters.apiRateLimiter;
            limiter.options.handler(req, res);
            expect(res.status).toHaveBeenCalledWith(429);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: RateLimitError.RATE_EXCEEDED
            }));
        });

        it('should enable standardHeaders', () => {
            const limiter = limiters.apiRateLimiter;
            expect(limiter.options.standardHeaders).toBe(true);
        });

        it('should log warning with IP, path, and method', () => {
            const limiter = limiters.apiRateLimiter;
            limiter.options.handler(req, res);
            expect(logger.warn).toHaveBeenCalledWith(
                '[RATE-LIMIT] API rate limit exceeded',
                expect.objectContaining({
                    ip: '127.0.0.1',
                    path: '/api/test',
                    method: 'POST'
                })
            );
        });
    });

    describe('uploadRateLimiter', () => {
        it('should configure handler with correct error response', () => {
            const limiter = limiters.uploadRateLimiter;
            req.headers[WEB_SESSION_ID_HEADER] = 'session-123';

            limiter.options.handler(req, res);
            expect(res.status).toHaveBeenCalledWith(429);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: RateLimitError.UPLOAD_WAIT
            }));
        });

        it('should enable standardHeaders', () => {
            const limiter = limiters.uploadRateLimiter;
            expect(limiter.options.standardHeaders).toBe(true);
        });

        it('should log warning with redacted webSessionId and IP', () => {
            const limiter = limiters.uploadRateLimiter;
            req.headers[WEB_SESSION_ID_HEADER] = 'session-123';

            limiter.options.handler(req, res);
            expect(logger.warn).toHaveBeenCalledWith(
                '[RATE-LIMIT] Upload rate limit exceeded',
                expect.objectContaining({
                    webSessionId: 'session-123',
                    ip: '127.0.0.1'
                })
            );
        });
    });

    describe('operatorRefreshRateLimiter', () => {
        it('should configure handler with correct error response', () => {
            const limiter = limiters.operatorRefreshRateLimiter;
            req.body.operator_session_id = 'op-session-1234567890';

            limiter.options.handler(req, res);
            expect(res.status).toHaveBeenCalledWith(429);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: RateLimitError.REFRESH_WAIT
            }));
        });

        it('should enable standardHeaders', () => {
            const limiter = limiters.operatorRefreshRateLimiter;
            expect(limiter.options.standardHeaders).toBe(true);
        });

        it('should log warning with IP and truncated operatorSessionId', () => {
            const limiter = limiters.operatorRefreshRateLimiter;
            req.body.operator_session_id = 'op-session-1234567890';

            limiter.options.handler(req, res);
            expect(logger.warn).toHaveBeenCalledWith(
                '[RATE-LIMIT] Operator refresh rate limit exceeded',
                expect.objectContaining({
                    ip: '127.0.0.1',
                    operatorSessionId: 'op-session-1...'
                })
            );
        });
    });

    describe('operatorAuthRateLimiter', () => {
        it('should use custom key generator with API key prefix', () => {
            const limiter = limiters.operatorAuthRateLimiter;
            expect(limiter.options.keyGenerator).toBeDefined();

            req.headers.authorization = `${BEARER_PREFIX}test-key-1234567890`;
            const key = limiter.options.keyGenerator(req);
            expect(key).toBe('apikey:test-key-1234567');
        });

        it('should fallback to IP for key when no API key', () => {
            const limiter = limiters.operatorAuthRateLimiter;
            const key = limiter.options.keyGenerator(req);
            expect(key).toBe(`ip:${req.ip}`);
        });

        it('should configure handler with correct error response', () => {
            const limiter = limiters.operatorAuthRateLimiter;
            limiter.options.handler(req, res);
            expect(res.status).toHaveBeenCalledWith(429);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: RateLimitError.AUTH_OPERATOR
            }));
        });

        it('should enable standardHeaders', () => {
            const limiter = limiters.operatorAuthRateLimiter;
            expect(limiter.options.standardHeaders).toBe(true);
        });

        it('should disable keyGeneratorIpFallback validation', () => {
            const limiter = limiters.operatorAuthRateLimiter;
            expect(limiter.options.validate.keyGeneratorIpFallback).toBe(false);
        });

        it('should log warning with IP, keyType, and truncated apiKeyPrefix', () => {
            const limiter = limiters.operatorAuthRateLimiter;
            req.headers.authorization = `${BEARER_PREFIX}test-key-1234567890`;
            req.headers['user-agent'] = 'test-agent';

            limiter.options.handler(req, res);
            expect(logger.warn).toHaveBeenCalledWith(
                '[RATE-LIMIT] Operator auth rate limit exceeded',
                expect.objectContaining({
                    ip: '127.0.0.1',
                    keyType: 'api_key',
                    apiKeyPrefix: 'test-key-123...',
                    path: '/api/test',
                    userAgent: 'test-agent'
                })
            );
        });

        it('should log warning with keyType ip when no API key', () => {
            const limiter = limiters.operatorAuthRateLimiter;
            limiter.options.handler(req, res);
            expect(logger.warn).toHaveBeenCalledWith(
                '[RATE-LIMIT] Operator auth rate limit exceeded',
                expect.objectContaining({
                    ip: '127.0.0.1',
                    keyType: 'ip',
                    apiKeyPrefix: null
                })
            );
        });
    });

    describe('operatorAuthIpBackstopLimiter', () => {
        it('should configure handler with correct error response', () => {
            const limiter = limiters.operatorAuthIpBackstopLimiter;
            limiter.options.handler(req, res);
            expect(res.status).toHaveBeenCalledWith(429);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: RateLimitError.AUTH_IP
            }));
        });

        it('should enable standardHeaders', () => {
            const limiter = limiters.operatorAuthIpBackstopLimiter;
            expect(limiter.options.standardHeaders).toBe(true);
        });

        it('should log error with IP, path, and userAgent', () => {
            const limiter = limiters.operatorAuthIpBackstopLimiter;
            req.headers['user-agent'] = 'test-agent';

            limiter.options.handler(req, res);
            expect(logger.error).toHaveBeenCalledWith(
                '[RATE-LIMIT] Operator auth IP backstop limit exceeded - possible attack',
                expect.objectContaining({
                    ip: '127.0.0.1',
                    path: '/api/test',
                    userAgent: 'test-agent'
                })
            );
        });
    });

    describe('auditRateLimiter', () => {
        it('should configure handler with correct error response', () => {
            const limiter = limiters.auditRateLimiter;
            req.userId = 'user-123';

            limiter.options.handler(req, res);
            expect(res.status).toHaveBeenCalledWith(429);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: RateLimitError.AUDIT_WAIT
            }));
        });

        it('should enable standardHeaders', () => {
            const limiter = limiters.auditRateLimiter;
            expect(limiter.options.standardHeaders).toBe(true);
        });

        it('should use userId as key when available', () => {
            const limiter = limiters.auditRateLimiter;
            req.userId = 'user-123';
            const key = limiter.options.keyGenerator(req);
            expect(key).toBe('user-123');
        });

        it('should fallback to IP as key when userId not available', () => {
            const limiter = limiters.auditRateLimiter;
            const key = limiter.options.keyGenerator(req);
            expect(key).toBe('127.0.0.1');
        });

        it('should disable keyGeneratorIpFallback validation', () => {
            const limiter = limiters.auditRateLimiter;
            expect(limiter.options.validate.keyGeneratorIpFallback).toBe(false);
        });

        it('should log warning with IP, userId, path, and method', () => {
            const limiter = limiters.auditRateLimiter;
            req.userId = 'user-123';

            limiter.options.handler(req, res);
            expect(logger.warn).toHaveBeenCalledWith(
                '[RATE-LIMIT] Audit rate limit exceeded',
                expect.objectContaining({
                    ip: '127.0.0.1',
                    userId: 'user-123',
                    path: '/api/test',
                    method: 'POST'
                })
            );
        });
    });

    describe('consoleRateLimiter', () => {
        it('should configure handler with correct error response', () => {
            const limiter = limiters.consoleRateLimiter;
            req.userId = 'user-123';

            limiter.options.handler(req, res);
            expect(res.status).toHaveBeenCalledWith(429);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: RateLimitError.CONSOLE_WAIT
            }));
        });

        it('should enable standardHeaders', () => {
            const limiter = limiters.consoleRateLimiter;
            expect(limiter.options.standardHeaders).toBe(true);
        });

        it('should use userId as key when available', () => {
            const limiter = limiters.consoleRateLimiter;
            req.userId = 'user-123';
            const key = limiter.options.keyGenerator(req);
            expect(key).toBe('user-123');
        });

        it('should fallback to IP as key when userId not available', () => {
            const limiter = limiters.consoleRateLimiter;
            const key = limiter.options.keyGenerator(req);
            expect(key).toBe('127.0.0.1');
        });

        it('should disable keyGeneratorIpFallback validation', () => {
            const limiter = limiters.consoleRateLimiter;
            expect(limiter.options.validate.keyGeneratorIpFallback).toBe(false);
        });

        it('should log warning with IP, userId, path, and method', () => {
            const limiter = limiters.consoleRateLimiter;
            req.userId = 'user-123';

            limiter.options.handler(req, res);
            expect(logger.warn).toHaveBeenCalledWith(
                '[RATE-LIMIT] Console rate limit exceeded',
                expect.objectContaining({
                    ip: '127.0.0.1',
                    userId: 'user-123',
                    path: '/api/test',
                    method: 'POST'
                })
            );
        });
    });

    describe('operatorApiRateLimiter', () => {
        it('should configure handler with correct error response', () => {
            const limiter = limiters.operatorApiRateLimiter;
            limiter.options.handler(req, res);
            expect(res.status).toHaveBeenCalledWith(429);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: RateLimitError.RATE_EXCEEDED_WAIT
            }));
        });

        it('should enable standardHeaders', () => {
            const limiter = limiters.operatorApiRateLimiter;
            expect(limiter.options.standardHeaders).toBe(true);
        });

        it('should use apiKey as key when available', () => {
            const limiter = limiters.operatorApiRateLimiter;
            req.apiKey = 'api-key-123';
            const key = limiter.options.keyGenerator(req);
            expect(key).toBe('api-key-123');
        });

        it('should fallback to IP as key when apiKey not available', () => {
            const limiter = limiters.operatorApiRateLimiter;
            const key = limiter.options.keyGenerator(req);
            expect(key).toBe('127.0.0.1');
        });

        it('should disable keyGeneratorIpFallback validation', () => {
            const limiter = limiters.operatorApiRateLimiter;
            expect(limiter.options.validate.keyGeneratorIpFallback).toBe(false);
        });

        it('should log warning with IP, path, and userId', () => {
            const limiter = limiters.operatorApiRateLimiter;
            req.userId = 'user-123';

            limiter.options.handler(req, res);
            expect(logger.warn).toHaveBeenCalledWith(
                '[RATE-LIMIT] Operator API rate limit exceeded',
                expect.objectContaining({
                    ip: '127.0.0.1',
                    path: '/api/test',
                    userId: 'user-123'
                })
            );
        });
    });

    describe('deviceLinkRateLimiter', () => {
        it('should configure handler with correct error response', () => {
            const limiter = limiters.deviceLinkRateLimiter;
            limiter.options.handler(req, res);
            expect(res.status).toHaveBeenCalledWith(429);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: RateLimitError.DEVICE_LINK_REGISTER
            }));
        });

        it('should enable standardHeaders', () => {
            const limiter = limiters.deviceLinkRateLimiter;
            expect(limiter.options.standardHeaders).toBe(true);
        });

        it('should log warning with IP and path', () => {
            const limiter = limiters.deviceLinkRateLimiter;
            limiter.options.handler(req, res);
            expect(logger.warn).toHaveBeenCalledWith(
                '[RATE-LIMIT] Device link register rate limit exceeded',
                expect.objectContaining({
                    ip: '127.0.0.1',
                    path: '/api/test'
                })
            );
        });
    });

    describe('deviceLinkGenerateLimiter', () => {
        it('should configure handler with correct error response', () => {
            const limiter = limiters.deviceLinkGenerateLimiter;
            req.userId = 'user-123';

            limiter.options.handler(req, res);
            expect(res.status).toHaveBeenCalledWith(429);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: RateLimitError.DEVICE_LINK_CREATE_WAIT
            }));
        });

        it('should enable standardHeaders', () => {
            const limiter = limiters.deviceLinkGenerateLimiter;
            expect(limiter.options.standardHeaders).toBe(true);
        });

        it('should use userId as key when available', () => {
            const limiter = limiters.deviceLinkGenerateLimiter;
            req.userId = 'user-123';
            const key = limiter.options.keyGenerator(req);
            expect(key).toBe('user-123');
        });

        it('should fallback to IP as key when userId not available', () => {
            const limiter = limiters.deviceLinkGenerateLimiter;
            const key = limiter.options.keyGenerator(req);
            expect(key).toBe('127.0.0.1');
        });

        it('should disable keyGeneratorIpFallback validation', () => {
            const limiter = limiters.deviceLinkGenerateLimiter;
            expect(limiter.options.validate.keyGeneratorIpFallback).toBe(false);
        });

        it('should log warning with IP, userId, and path', () => {
            const limiter = limiters.deviceLinkGenerateLimiter;
            req.userId = 'user-123';

            limiter.options.handler(req, res);
            expect(logger.warn).toHaveBeenCalledWith(
                '[RATE-LIMIT] Device link generate rate limit exceeded',
                expect.objectContaining({
                    ip: '127.0.0.1',
                    userId: 'user-123',
                    path: '/api/test'
                })
            );
        });
    });

    describe('deviceLinkCreateRateLimiter', () => {
        it('should configure handler with correct error response', () => {
            const limiter = limiters.deviceLinkCreateRateLimiter;
            req.userId = 'user-123';

            limiter.options.handler(req, res);
            expect(res.status).toHaveBeenCalledWith(429);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: RateLimitError.DEVICE_LINK_CREATE_WAIT
            }));
        });

        it('should enable standardHeaders', () => {
            const limiter = limiters.deviceLinkCreateRateLimiter;
            expect(limiter.options.standardHeaders).toBe(true);
        });

        it('should use userId as key when available', () => {
            const limiter = limiters.deviceLinkCreateRateLimiter;
            req.userId = 'user-123';
            const key = limiter.options.keyGenerator(req);
            expect(key).toBe('user-123');
        });

        it('should fallback to IP as key when userId not available', () => {
            const limiter = limiters.deviceLinkCreateRateLimiter;
            const key = limiter.options.keyGenerator(req);
            expect(key).toBe('127.0.0.1');
        });

        it('should disable keyGeneratorIpFallback validation', () => {
            const limiter = limiters.deviceLinkCreateRateLimiter;
            expect(limiter.options.validate.keyGeneratorIpFallback).toBe(false);
        });

        it('should log warning with IP, userId, and path', () => {
            const limiter = limiters.deviceLinkCreateRateLimiter;
            req.userId = 'user-123';

            limiter.options.handler(req, res);
            expect(logger.warn).toHaveBeenCalledWith(
                '[RATE-LIMIT] Device link creation rate limit exceeded',
                expect.objectContaining({
                    ip: '127.0.0.1',
                    userId: 'user-123',
                    path: '/api/test'
                })
            );
        });
    });

    describe('deviceLinkListRateLimiter', () => {
        it('should configure handler with correct error response', () => {
            const limiter = limiters.deviceLinkListRateLimiter;
            req.userId = 'user-123';

            limiter.options.handler(req, res);
            expect(res.status).toHaveBeenCalledWith(429);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: RateLimitError.GENERIC_WAIT
            }));
        });

        it('should enable standardHeaders', () => {
            const limiter = limiters.deviceLinkListRateLimiter;
            expect(limiter.options.standardHeaders).toBe(true);
        });

        it('should use userId as key when available', () => {
            const limiter = limiters.deviceLinkListRateLimiter;
            req.userId = 'user-123';
            const key = limiter.options.keyGenerator(req);
            expect(key).toBe('user-123');
        });

        it('should fallback to IP as key when userId not available', () => {
            const limiter = limiters.deviceLinkListRateLimiter;
            const key = limiter.options.keyGenerator(req);
            expect(key).toBe('127.0.0.1');
        });

        it('should disable keyGeneratorIpFallback validation', () => {
            const limiter = limiters.deviceLinkListRateLimiter;
            expect(limiter.options.validate.keyGeneratorIpFallback).toBe(false);
        });

        it('should log warning with IP, userId, and path', () => {
            const limiter = limiters.deviceLinkListRateLimiter;
            req.userId = 'user-123';

            limiter.options.handler(req, res);
            expect(logger.warn).toHaveBeenCalledWith(
                '[RATE-LIMIT] Device link list rate limit exceeded',
                expect.objectContaining({
                    ip: '127.0.0.1',
                    userId: 'user-123',
                    path: '/api/test'
                })
            );
        });
    });

    describe('deviceLinkRevokeRateLimiter', () => {
        it('should configure handler with correct error response', () => {
            const limiter = limiters.deviceLinkRevokeRateLimiter;
            req.userId = 'user-123';

            limiter.options.handler(req, res);
            expect(res.status).toHaveBeenCalledWith(429);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: RateLimitError.GENERIC_WAIT
            }));
        });

        it('should enable standardHeaders', () => {
            const limiter = limiters.deviceLinkRevokeRateLimiter;
            expect(limiter.options.standardHeaders).toBe(true);
        });

        it('should use userId as key when available', () => {
            const limiter = limiters.deviceLinkRevokeRateLimiter;
            req.userId = 'user-123';
            const key = limiter.options.keyGenerator(req);
            expect(key).toBe('user-123');
        });

        it('should fallback to IP as key when userId not available', () => {
            const limiter = limiters.deviceLinkRevokeRateLimiter;
            const key = limiter.options.keyGenerator(req);
            expect(key).toBe('127.0.0.1');
        });

        it('should disable keyGeneratorIpFallback validation', () => {
            const limiter = limiters.deviceLinkRevokeRateLimiter;
            expect(limiter.options.validate.keyGeneratorIpFallback).toBe(false);
        });

        it('should log warning with IP, userId, and path', () => {
            const limiter = limiters.deviceLinkRevokeRateLimiter;
            req.userId = 'user-123';

            limiter.options.handler(req, res);
            expect(logger.warn).toHaveBeenCalledWith(
                '[RATE-LIMIT] Device link revoke rate limit exceeded',
                expect.objectContaining({
                    ip: '127.0.0.1',
                    userId: 'user-123',
                    path: '/api/test'
                })
            );
        });
    });

    describe('settingsRateLimiter', () => {
        it('should configure handler with correct error response', () => {
            const limiter = limiters.settingsRateLimiter;
            req.userId = 'user-123';

            limiter.options.handler(req, res);
            expect(res.status).toHaveBeenCalledWith(429);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: RateLimitError.SETTINGS_WAIT
            }));
        });

        it('should enable standardHeaders', () => {
            const limiter = limiters.settingsRateLimiter;
            expect(limiter.options.standardHeaders).toBe(true);
        });

        it('should use userId as key when available', () => {
            const limiter = limiters.settingsRateLimiter;
            req.userId = 'user-123';
            const key = limiter.options.keyGenerator(req);
            expect(key).toBe('user-123');
        });

        it('should fallback to IP as key when userId not available', () => {
            const limiter = limiters.settingsRateLimiter;
            const key = limiter.options.keyGenerator(req);
            expect(key).toBe('127.0.0.1');
        });

        it('should disable keyGeneratorIpFallback validation', () => {
            const limiter = limiters.settingsRateLimiter;
            expect(limiter.options.validate.keyGeneratorIpFallback).toBe(false);
        });

        it('should log warning with IP, userId, path, and method', () => {
            const limiter = limiters.settingsRateLimiter;
            req.userId = 'user-123';

            limiter.options.handler(req, res);
            expect(logger.warn).toHaveBeenCalledWith(
                '[RATE-LIMIT] Settings rate limit exceeded',
                expect.objectContaining({
                    ip: '127.0.0.1',
                    userId: 'user-123',
                    path: '/api/test',
                    method: 'POST'
                })
            );
        });
    });

    describe('passkeyRateLimiter', () => {
        it('should configure handler with correct error response', () => {
            const limiter = limiters.passkeyRateLimiter;
            limiter.options.handler(req, res);
            expect(res.status).toHaveBeenCalledWith(429);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: RateLimitError.AUTH
            }));
        });

        it('should enable standardHeaders', () => {
            const limiter = limiters.passkeyRateLimiter;
            expect(limiter.options.standardHeaders).toBe(true);
        });

        it('should log warning with IP, path, and userAgent', () => {
            const limiter = limiters.passkeyRateLimiter;
            req.headers['user-agent'] = 'test-agent';

            limiter.options.handler(req, res);
            expect(logger.warn).toHaveBeenCalledWith(
                '[RATE-LIMIT] Passkey rate limit exceeded',
                expect.objectContaining({
                    ip: '127.0.0.1',
                    path: '/api/test',
                    userAgent: 'test-agent'
                })
            );
        });
    });

    describe('createRateLimiters', () => {
        it('should return all 18 rate limiters', () => {
            const result = createRateLimiters();
            const expectedLimiters = [
                'globalPublicRateLimiter',
                'authRateLimiter',
                'chatRateLimiter',
                'sseRateLimiter',
                'apiRateLimiter',
                'uploadRateLimiter',
                'operatorRefreshRateLimiter',
                'operatorAuthRateLimiter',
                'operatorAuthIpBackstopLimiter',
                'auditRateLimiter',
                'consoleRateLimiter',
                'operatorApiRateLimiter',
                'deviceLinkRateLimiter',
                'deviceLinkGenerateLimiter',
                'deviceLinkCreateRateLimiter',
                'deviceLinkListRateLimiter',
                'deviceLinkRevokeRateLimiter',
                'settingsRateLimiter',
                'passkeyRateLimiter'
            ];
            expectedLimiters.forEach(limiterName => {
                expect(result[limiterName]).toBeDefined();
            });
        });

        it('should accept config parameter', () => {
            const config = { test: 'value' };
            const result = createRateLimiters({ config });
            expect(result.globalPublicRateLimiter).toBeDefined();
        });

        it('should handle empty config parameter', () => {
            const result = createRateLimiters({});
            expect(result.globalPublicRateLimiter).toBeDefined();
        });

        it('should handle no parameters', () => {
            const result = createRateLimiters();
            expect(result.globalPublicRateLimiter).toBeDefined();
        });
    });
});
