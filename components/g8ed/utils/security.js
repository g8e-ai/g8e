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
 * Security Utilities
 * 
 * Helper functions for security-related operations like logging redaction,
 * CORS validation, etc. WebSession validation is handled by auth middleware.
 * 
 * ## g8e Cloud Operator Security Model
 * 
 * The g8e Cloud Operator for AWS implements a Just-In-Time (JIT) access
 * model with Zero Standing Privileges:
 * 
 * ### Core Principles:
 * 1. **Zero Standing Privileges** - Operator starts with minimal permissions
 * 2. **Just-In-Time Access** - Permissions granted only when needed
 * 3. **Intent-Based Execution** - AI requests permissions through natural conversation
 * 4. **Permission Boundaries** - Hard security ceiling prevents escalation
 * 5. **Complete Audit Trail** - All permission changes logged and tracked
 * 
 * ### Permission Flow:
 * 1. AI attempts action → Receives AccessDenied
 * 2. AI calls grant_intent_permission() with justification
 * 3. System displays approval card in Operator terminal
 * 4. User approves → System generates least-privilege IAM policy
 * 5. Policy applied via iam:PutRolePolicy → AI continues workflow
 * 
 * ### Security Boundaries:
 * - Blocks: AdministratorAccess, *Admin*, *FullAccess policies
 * - Scopes: WRITE actions to ManagedBy=g8e tagged resources
 * - Restricts: IAM actions to operator's own role only
 * 
 * ### Available Intents:
 * - ec2_discovery, ec2_management
 * - s3_read, s3_write
 * - lambda_discovery, lambda_invoke
 * - secrets_read, cloudwatch_logs
 * - And 25+ more service-specific intents
 */

import {
    SESSION_COOKIE_NAME,
    COOKIE_SAME_SITE,
    SESSION_ID_LOG_PREFIX_LENGTH,
} from '../constants/session.js';

export const EMAIL_REGEX = /[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}/;

export const PII_REDACT_MAX_DEPTH = 10;

export const PII_FIELDS = [
    'email',
    'toEmail',
    'customerEmail',
    'customer_email',
    'userEmail',
    'user_email',
    'name',
    'customerName',
    'customer_name',
    'userName',
    'user_name',
    'displayName',
    'display_name',
    'firstName',
    'first_name',
    'lastName',
    'last_name',
    'fullName',
    'full_name',
];

/**
 * Get the cookie domain for cross-subdomain sharing.
 * Enables cookies set on localhost to be sent to console.localhost.
 * 
 * @param {object} req - Express request object
 * @returns {string|undefined} - Cookie domain or undefined for host-only cookies
 */
export function getCookieDomain(req) {
    const host = (req.get ? req.get('host') : null) || req.hostname || req.headers?.host;
    if (!host) return undefined;
    
    const hostname = host.split(':')[0];
    
    if (hostname === 'localhost' || hostname === '127.0.0.1' || /^\d+\.\d+\.\d+\.\d+$/.test(hostname)) {
        return undefined;
    }
    
    if (hostname.endsWith('localhost')) {
        const parts = hostname.split('.');
        if (parts.length > 1) {
            // Return the direct parent domain
            // Example: console.localhost -> .localhost
            //          sub.console.localhost -> .console.localhost
            return '.' + parts.slice(1).join('.');
        }
        return '.' + hostname;
    }
    
    return undefined;
}

/**
 * Comprehensively clear web_session_id cookie from browser.
 * Clears all possible cookie variants (host-only, domain-scoped, parent domains)
 * to ensure stale cookies from previous sessions don't interfere.
 * 
 * @param {object} res - Express response object
 * @param {object} req - Express request object (to determine domain)
 */
export function clearSessionCookies(res, req) {
    const host = (req.get ? req.get('host') : null) || req.hostname || req.headers?.host;
    if (!host) return;

    const hostname = host.split(':')[0];
    const baseClearOptions = {
        path: '/',
        httpOnly: true,
        secure: true,
        sameSite: COOKIE_SAME_SITE
    };
    
    // 1. Clear host-only cookie
    res.clearCookie(SESSION_COOKIE_NAME, baseClearOptions);
    
    // 2. Clear domain cookies for each level (up to localhost)
    if (hostname.endsWith('localhost')) {
        const parts = hostname.split('.');
        // For sub.console.localhost, we clear:
        // .sub.console.localhost
        // .console.localhost
        // .localhost
        for (let i = 0; i < parts.length; i++) {
            const domain = '.' + parts.slice(i).join('.');
            res.clearCookie(SESSION_COOKIE_NAME, { ...baseClearOptions, domain });
        }
    } else {
        // For other domains, just clear the base domain cookie if one was found
        const cookieDomain = getCookieDomain(req);
        if (cookieDomain) {
            res.clearCookie(SESSION_COOKIE_NAME, { ...baseClearOptions, domain: cookieDomain });
        }
    }
}

/**
 * Redact sensitive session ID for logging (show only first 15 characters)
 * @param {string} webSessionId - WebSession ID to redact
 * @returns {string} - Redacted session ID
 */
export function redactWebSessionId(webSessionId) {
    if (!webSessionId || typeof webSessionId !== 'string') {
        return '[invalid]';
    }
    if (webSessionId.length <= SESSION_ID_LOG_PREFIX_LENGTH) {
        return webSessionId;
    }
    return webSessionId.substring(0, SESSION_ID_LOG_PREFIX_LENGTH) + '...';
}

