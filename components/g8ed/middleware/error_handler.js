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
 * g8ed Error Handler Middleware
 * 
 * Centralized error handling following g8e patterns
 */

import { logger } from '../utils/logger.js';
import { HttpStatusMessage, ErrorCategory } from '../constants/errors.js';
import { G8eError, InternalServerError, ValidationError } from '../services/error_service.js';

/**
 * g8ed Error Handler Middleware Factory
 * 
 * Centralized error handling following g8e patterns
 * 
 * @param {Object} options
 * @param {Object} options.config - Platform configuration
 */
export function createErrorHandlerMiddleware({ config }) {
    const environment = config?.environment || 'production';

    return (err, req, res, next) => {
        let g8eError;

        if (err instanceof G8eError) {
            g8eError = err;
        } else if (err.validationErrors) {
            // Handle model validation errors as 400s
            g8eError = new ValidationError(err.message, {
                details: { errors: err.validationErrors },
                cause: err,
                requestId: req.headers['x-request-id'],
                traceId: req.headers['x-trace-id']
            });
        } else {
            // Wrap generic errors
            g8eError = new InternalServerError(err.message || HttpStatusMessage.INTERNAL_SERVER_ERROR, {
                cause: err,
                requestId: req.headers['x-request-id'],
                traceId: req.headers['x-trace-id']
            });
        }

        // Log the error
        const logMetadata = {
            code: g8eError.code,
            category: g8eError.category,
            severity: g8eError.severity,
            url: req.url,
            method: req.method,
            requestId: g8eError.requestId,
            traceId: g8eError.traceId
        };

        if (g8eError.severity === 'critical' || g8eError.severity === 'high') {
            logger.error(`[${g8eError.code}] ${g8eError.message}`, { ...logMetadata, stack: err.stack });
            if (environment === 'test') {
                console.error(`[TEST-ERROR] ${g8eError.code}: ${g8eError.message}\n${err.stack}`);
            }
        } else {
            logger.warn(`[${g8eError.code}] ${g8eError.message}`, logMetadata);
            if (environment === 'test' && statusCode >= 500) {
                console.error(`[TEST-ERROR] ${g8eError.code}: ${g8eError.message}\n${err.stack}`);
            }
        }

        const statusCode = g8eError.getHttpStatus();
        const responseBody = g8eError.toG8eErrorResponse();

        // Ensure we don't leak stack traces in production if it's not a G8eError or if it's an internal error
        if (environment === 'production' && g8eError.category === ErrorCategory.INTERNAL) {
            responseBody.error.message = HttpStatusMessage.INTERNAL_SERVER_ERROR;
            if (responseBody.error.cause) {
                delete responseBody.error.cause;
            }
        }

        res.status(statusCode).json(responseBody);
    };
}

