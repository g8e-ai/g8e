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
 * VSOD Error Handler Middleware
 * 
 * Centralized error handling following VSO patterns
 */

import { logger } from '../utils/logger.js';
import { HttpStatusMessage, ErrorCategory } from '../constants/errors.js';
import { VSOError, InternalServerError, ValidationError } from '../services/error_service.js';

/**
 * VSOD Error Handler Middleware Factory
 * 
 * Centralized error handling following VSO patterns
 * 
 * @param {Object} options
 * @param {Object} options.config - Platform configuration
 */
export function createErrorHandlerMiddleware({ config }) {
    const environment = config?.environment || 'production';

    return (err, req, res, next) => {
        let vsoError;

        if (err instanceof VSOError) {
            vsoError = err;
        } else if (err.validationErrors) {
            // Handle model validation errors as 400s
            vsoError = new ValidationError(err.message, {
                details: { errors: err.validationErrors },
                cause: err,
                requestId: req.headers['x-request-id'],
                traceId: req.headers['x-trace-id']
            });
        } else {
            // Wrap generic errors
            vsoError = new InternalServerError(err.message || HttpStatusMessage.INTERNAL_SERVER_ERROR, {
                cause: err,
                requestId: req.headers['x-request-id'],
                traceId: req.headers['x-trace-id']
            });
        }

        // Log the error
        const logMetadata = {
            code: vsoError.code,
            category: vsoError.category,
            severity: vsoError.severity,
            url: req.url,
            method: req.method,
            requestId: vsoError.requestId,
            traceId: vsoError.traceId
        };

        if (vsoError.severity === 'critical' || vsoError.severity === 'high') {
            logger.error(`[${vsoError.code}] ${vsoError.message}`, { ...logMetadata, stack: err.stack });
            if (environment === 'test') {
                console.error(`[TEST-ERROR] ${vsoError.code}: ${vsoError.message}\n${err.stack}`);
            }
        } else {
            logger.warn(`[${vsoError.code}] ${vsoError.message}`, logMetadata);
            if (environment === 'test' && statusCode >= 500) {
                console.error(`[TEST-ERROR] ${vsoError.code}: ${vsoError.message}\n${err.stack}`);
            }
        }

        const statusCode = vsoError.getHttpStatus();
        const responseBody = vsoError.toVSOErrorResponse();

        // Ensure we don't leak stack traces in production if it's not a VSOError or if it's an internal error
        if (environment === 'production' && vsoError.category === ErrorCategory.INTERNAL) {
            responseBody.error.message = HttpStatusMessage.INTERNAL_SERVER_ERROR;
            if (responseBody.error.cause) {
                delete responseBody.error.cause;
            }
        }

        res.status(statusCode).json(responseBody);
    };
}

