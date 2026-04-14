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

import { FrontendBaseModel, F } from './base.js';
import { HttpStatus, HTTP_STATUS_PATTERN } from '../constants/service-client-constants.js';

export const ApiErrorType = Object.freeze({
    RATE_LIMIT:     'rate_limit',
    FORBIDDEN:      'forbidden',
    UNAUTHORIZED:   'unauthorized',
    NOT_FOUND:      'not_found',
    SERVER_ERROR:   'server_error',
    UNKNOWN:        'unknown',
});

export class ApiErrorModel extends FrontendBaseModel {
    static fields = {
        type:       { type: F.string,  required: true },
        status:     { type: F.number,  default: null },
        message:    { type: F.string,  required: true },
        isWarning:  { type: F.boolean, default: false },
    };

    static fromError(error, RateLimitErrorClass) {
        if (error instanceof RateLimitErrorClass) {
            return new ApiErrorModel({
                type:      ApiErrorType.RATE_LIMIT,
                status:    429,
                message:   error.message || 'Too many requests. Please wait a moment and try again.',
                isWarning: true,
            });
        }

        const statusMatch = error.message && error.message.match(HTTP_STATUS_PATTERN);
        const status = statusMatch ? parseInt(statusMatch[1], 10) : null;

        if (status === HttpStatus.FORBIDDEN) {
            return new ApiErrorModel({
                type:    ApiErrorType.FORBIDDEN,
                status,
                message: 'Access denied. Please sign in again.',
            });
        }
        if (status === HttpStatus.UNAUTHORIZED) {
            return new ApiErrorModel({
                type:    ApiErrorType.UNAUTHORIZED,
                status,
                message: 'WebSession expired. Please sign in again.',
            });
        }
        if (status === HttpStatus.NOT_FOUND) {
            return new ApiErrorModel({
                type:    ApiErrorType.NOT_FOUND,
                status,
                message: 'Resource not found.',
            });
        }
        if (status === HttpStatus.INTERNAL_ERROR) {
            return new ApiErrorModel({
                type:    ApiErrorType.SERVER_ERROR,
                status,
                message: 'Server error. Please try again later.',
            });
        }

        return new ApiErrorModel({
            type:    ApiErrorType.UNKNOWN,
            status,
            message: error.message || 'An unexpected error occurred. Please try again.',
        });
    }
}
