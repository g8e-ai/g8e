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

import crypto from 'crypto';
import { logger } from '../utils/logger.js';
import { AuthenticationError } from '../services/error_service.js';
import { COOKIE_SAME_SITE } from '../constants/session.js';

const CSRF_COOKIE_NAME = 'g8e_csrf_token';
const CSRF_HEADER_NAME = 'x-csrf-token';

/**
 * CSRF Protection Middleware
 * 
 * Implements the Double Submit Cookie pattern:
 * 1. Generates a random CSRF token if not present
 * 2. Sets the token in a non-httpOnly cookie (so client JS can read it)
 * 3. Validates that state-changing requests (POST, PUT, DELETE) include
 *    the same token in the X-CSRF-Token header.
 */
export const createCsrfProtection = ({ isTest = false } = {}) => {
    return (req, res, next) => {
        // 1. Skip for specific paths if needed (e.g. internal health checks)
        if (req.path === '/health' || req.path === '/health/live') {
            return next();
        }

        // 2. Skip in test environment if requested via header
        if (isTest && req.headers['x-skip-csrf'] === 'true') {
            return next();
        }

        // 3. Get or generate CSRF token
        let csrfToken = req.cookies[CSRF_COOKIE_NAME];
        
        // We only strictly NEED a CSRF token if we are using cookie-based auth.
        // However, to satisfy CodeQL and provide defense-in-depth, we'll ensure
        // it's available for all requests that might be browser-initiated.
        if (!csrfToken) {
            csrfToken = crypto.randomBytes(32).toString('hex');
            res.cookie(CSRF_COOKIE_NAME, csrfToken, {
                path: '/',
                httpOnly: false, // Must be readable by client JS
                secure: true,
                sameSite: COOKIE_SAME_SITE
            });
        }

        // 4. Add token to res.locals for EJS templates
        res.locals.csrfToken = csrfToken;

        // 5. Skip validation for safe methods
        const safeMethods = ['GET', 'HEAD', 'OPTIONS'];
        if (safeMethods.includes(req.method)) {
            return next();
        }

        // 6. Validate token for state-changing methods
        const headerToken = req.headers[CSRF_HEADER_NAME];
        
        // Also allow validation via body for traditional form submits (if needed)
        const bodyToken = req.body?._csrf;
        
        const submittedToken = headerToken || bodyToken;

        if (!submittedToken || submittedToken !== csrfToken) {
            logger.warn('[CSRF] Invalid or missing CSRF token', {
                method: req.method,
                path: req.path,
                ip: req.ip,
                hasCookie: !!csrfToken,
                hasHeader: !!headerToken,
                hasBody: !!bodyToken
            });
            
            return next(new AuthenticationError('Invalid or missing CSRF token', {
                code: 'CSRF_ERROR'
            }));
        }

        next();
    };
};
