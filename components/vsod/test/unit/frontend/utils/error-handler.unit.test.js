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

// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { HttpStatus, HTTP_STATUS_PATTERN } from '@vsod/public/js/constants/service-client-constants.js';
import { ApiErrorModel, ApiErrorType } from '@vsod/public/js/models/api-error-model.js';

let handleApiError;
let RateLimitError;
let notificationService;
let spyWarning;
let spyError;

beforeEach(async () => {
    vi.resetModules();

    window.serviceClient = undefined;
    window.handleApiError = undefined;

    ({ RateLimitError } = await import('@vsod/public/js/utils/service-client.js'));
    ({ notificationService } = await import('@vsod/public/js/utils/notification-service.js'));
    ({ handleApiError } = await import('@vsod/public/js/utils/error-handler.js'));

    spyWarning = vi.spyOn(notificationService, 'warning');
    spyError   = vi.spyOn(notificationService, 'error');
});

afterEach(() => {
    vi.restoreAllMocks();
});

describe('HttpStatus constants [UNIT]', () => {
    it('FORBIDDEN is 403', () => {
        expect(HttpStatus.FORBIDDEN).toBe(403);
    });

    it('UNAUTHORIZED is 401', () => {
        expect(HttpStatus.UNAUTHORIZED).toBe(401);
    });

    it('NOT_FOUND is 404', () => {
        expect(HttpStatus.NOT_FOUND).toBe(404);
    });

    it('INTERNAL_ERROR is 500', () => {
        expect(HttpStatus.INTERNAL_ERROR).toBe(500);
    });

    it('is frozen', () => {
        expect(Object.isFrozen(HttpStatus)).toBe(true);
    });
});

describe('HTTP_STATUS_PATTERN constant [UNIT]', () => {
    it('matches the ServiceClient HTTP error format', () => {
        expect(HTTP_STATUS_PATTERN.test('HTTP 403: Forbidden')).toBe(true);
    });

    it('captures the numeric status code in group 1', () => {
        const match = 'HTTP 500: Internal Server Error'.match(HTTP_STATUS_PATTERN);
        expect(match).not.toBeNull();
        expect(parseInt(match[1], 10)).toBe(HttpStatus.INTERNAL_ERROR);
    });

    it('matches every HttpStatus code', () => {
        expect(HTTP_STATUS_PATTERN.test(`HTTP ${HttpStatus.UNAUTHORIZED}: Unauthorized`)).toBe(true);
        expect(HTTP_STATUS_PATTERN.test(`HTTP ${HttpStatus.FORBIDDEN}: Forbidden`)).toBe(true);
        expect(HTTP_STATUS_PATTERN.test(`HTTP ${HttpStatus.NOT_FOUND}: Not Found`)).toBe(true);
        expect(HTTP_STATUS_PATTERN.test(`HTTP ${HttpStatus.INTERNAL_ERROR}: Internal Server Error`)).toBe(true);
    });

    it('does not match plain error messages without HTTP prefix', () => {
        expect(HTTP_STATUS_PATTERN.test('Network error')).toBe(false);
        expect(HTTP_STATUS_PATTERN.test('Timeout')).toBe(false);
        expect(HTTP_STATUS_PATTERN.test('')).toBe(false);
    });
});

describe('handleApiError [UNIT - jsdom]', () => {
    describe('RateLimitError handling', () => {
        it('calls notificationService.warning for RateLimitError', () => {
            handleApiError(new RateLimitError('Too many requests. Please wait a moment and try again.'));
            expect(spyWarning).toHaveBeenCalledOnce();
            expect(spyError).not.toHaveBeenCalled();
        });

        it('uses the error message as the warning text', () => {
            handleApiError(new RateLimitError('Custom rate limit message'));
            expect(spyWarning).toHaveBeenCalledWith('Custom rate limit message');
        });

        it('falls back to default message when RateLimitError message is empty', () => {
            handleApiError(new RateLimitError(''));
            expect(spyWarning).toHaveBeenCalledWith('Too many requests. Please wait a moment and try again.');
        });

        it('does not call notificationService.error for RateLimitError', () => {
            handleApiError(new RateLimitError('rate limited'));
            expect(spyError).not.toHaveBeenCalled();
        });
    });

    describe(`HTTP ${HttpStatus.FORBIDDEN} handling`, () => {
        it('shows access denied error', () => {
            handleApiError(new Error(`HTTP ${HttpStatus.FORBIDDEN}: Forbidden`));
            expect(spyError).toHaveBeenCalledWith('Access denied. Please sign in again.');
        });

        it('does not call warning', () => {
            handleApiError(new Error(`HTTP ${HttpStatus.FORBIDDEN}: Forbidden`));
            expect(spyWarning).not.toHaveBeenCalled();
        });

        it('calls error exactly once', () => {
            handleApiError(new Error(`HTTP ${HttpStatus.FORBIDDEN}: Forbidden`));
            expect(spyError).toHaveBeenCalledOnce();
        });
    });

    describe(`HTTP ${HttpStatus.UNAUTHORIZED} handling`, () => {
        it('shows session expired error', () => {
            handleApiError(new Error(`HTTP ${HttpStatus.UNAUTHORIZED}: Unauthorized`));
            expect(spyError).toHaveBeenCalledWith('WebSession expired. Please sign in again.');
        });

        it('calls error exactly once', () => {
            handleApiError(new Error(`HTTP ${HttpStatus.UNAUTHORIZED}: Unauthorized`));
            expect(spyError).toHaveBeenCalledOnce();
        });
    });

    describe(`HTTP ${HttpStatus.NOT_FOUND} handling`, () => {
        it('shows resource not found error', () => {
            handleApiError(new Error(`HTTP ${HttpStatus.NOT_FOUND}: Not Found`));
            expect(spyError).toHaveBeenCalledWith('Resource not found.');
        });

        it('calls error exactly once', () => {
            handleApiError(new Error(`HTTP ${HttpStatus.NOT_FOUND}: Not Found`));
            expect(spyError).toHaveBeenCalledOnce();
        });
    });

    describe(`HTTP ${HttpStatus.INTERNAL_ERROR} handling`, () => {
        it('shows server error', () => {
            handleApiError(new Error(`HTTP ${HttpStatus.INTERNAL_ERROR}: Internal Server Error`));
            expect(spyError).toHaveBeenCalledWith('Server error. Please try again later.');
        });

        it('calls error exactly once', () => {
            handleApiError(new Error(`HTTP ${HttpStatus.INTERNAL_ERROR}: Internal Server Error`));
            expect(spyError).toHaveBeenCalledOnce();
        });
    });

    describe('unrecognized error handling', () => {
        it('shows the raw error message for an unrecognized HTTP status', () => {
            handleApiError(new Error('HTTP 502: Bad Gateway'));
            expect(spyError).toHaveBeenCalledWith('HTTP 502: Bad Gateway');
        });

        it('shows the raw error message for a non-HTTP error', () => {
            handleApiError(new Error('Network connection failed'));
            expect(spyError).toHaveBeenCalledWith('Network connection failed');
        });

        it('falls back to generic message when error has no message', () => {
            handleApiError(new Error(''));
            expect(spyError).toHaveBeenCalledWith('An unexpected error occurred. Please try again.');
        });

        it('calls error exactly once for unrecognized errors', () => {
            handleApiError(new Error('Something went wrong'));
            expect(spyError).toHaveBeenCalledOnce();
        });

        it('does not call warning for unrecognized errors', () => {
            handleApiError(new Error('Something went wrong'));
            expect(spyWarning).not.toHaveBeenCalled();
        });
    });

    describe('window global assignment', () => {
        it('assigns handleApiError to window', () => {
            expect(typeof window.handleApiError).toBe('function');
        });

        it('window.handleApiError is the exported function', () => {
            expect(window.handleApiError).toBe(handleApiError);
        });

        it('showErrorNotification is not assigned to window', () => {
            expect(window.showErrorNotification).toBeUndefined();
        });
    });
});

describe('ApiErrorModel.fromError [UNIT]', () => {
    it('classifies RateLimitError as rate_limit with isWarning true', () => {
        const err = new RateLimitError('Too many requests');
        const model = ApiErrorModel.fromError(err, RateLimitError);
        expect(model.type).toBe(ApiErrorType.RATE_LIMIT);
        expect(model.status).toBe(429);
        expect(model.isWarning).toBe(true);
        expect(model.message).toBe('Too many requests');
    });

    it('falls back to default rate limit message when RateLimitError message is empty', () => {
        const err = new RateLimitError('');
        const model = ApiErrorModel.fromError(err, RateLimitError);
        expect(model.type).toBe(ApiErrorType.RATE_LIMIT);
        expect(model.message).toBe('Too many requests. Please wait a moment and try again.');
    });

    it(`classifies HTTP ${HttpStatus.FORBIDDEN} as forbidden`, () => {
        const model = ApiErrorModel.fromError(new Error(`HTTP ${HttpStatus.FORBIDDEN}: Forbidden`), RateLimitError);
        expect(model.type).toBe(ApiErrorType.FORBIDDEN);
        expect(model.status).toBe(HttpStatus.FORBIDDEN);
        expect(model.isWarning).toBe(false);
        expect(model.message).toBe('Access denied. Please sign in again.');
    });

    it(`classifies HTTP ${HttpStatus.UNAUTHORIZED} as unauthorized`, () => {
        const model = ApiErrorModel.fromError(new Error(`HTTP ${HttpStatus.UNAUTHORIZED}: Unauthorized`), RateLimitError);
        expect(model.type).toBe(ApiErrorType.UNAUTHORIZED);
        expect(model.status).toBe(HttpStatus.UNAUTHORIZED);
        expect(model.isWarning).toBe(false);
        expect(model.message).toBe('WebSession expired. Please sign in again.');
    });

    it(`classifies HTTP ${HttpStatus.NOT_FOUND} as not_found`, () => {
        const model = ApiErrorModel.fromError(new Error(`HTTP ${HttpStatus.NOT_FOUND}: Not Found`), RateLimitError);
        expect(model.type).toBe(ApiErrorType.NOT_FOUND);
        expect(model.status).toBe(HttpStatus.NOT_FOUND);
        expect(model.isWarning).toBe(false);
        expect(model.message).toBe('Resource not found.');
    });

    it(`classifies HTTP ${HttpStatus.INTERNAL_ERROR} as server_error`, () => {
        const model = ApiErrorModel.fromError(new Error(`HTTP ${HttpStatus.INTERNAL_ERROR}: Internal Server Error`), RateLimitError);
        expect(model.type).toBe(ApiErrorType.SERVER_ERROR);
        expect(model.status).toBe(HttpStatus.INTERNAL_ERROR);
        expect(model.isWarning).toBe(false);
        expect(model.message).toBe('Server error. Please try again later.');
    });

    it('classifies an unrecognized HTTP status as unknown with raw message', () => {
        const model = ApiErrorModel.fromError(new Error('HTTP 502: Bad Gateway'), RateLimitError);
        expect(model.type).toBe(ApiErrorType.UNKNOWN);
        expect(model.status).toBe(502);
        expect(model.isWarning).toBe(false);
        expect(model.message).toBe('HTTP 502: Bad Gateway');
    });

    it('classifies a non-HTTP error as unknown with raw message', () => {
        const model = ApiErrorModel.fromError(new Error('Network connection failed'), RateLimitError);
        expect(model.type).toBe(ApiErrorType.UNKNOWN);
        expect(model.status).toBeNull();
        expect(model.isWarning).toBe(false);
        expect(model.message).toBe('Network connection failed');
    });

    it('falls back to generic message when error has no message', () => {
        const model = ApiErrorModel.fromError(new Error(''), RateLimitError);
        expect(model.type).toBe(ApiErrorType.UNKNOWN);
        expect(model.message).toBe('An unexpected error occurred. Please try again.');
    });

    it('returns an ApiErrorModel instance', () => {
        const model = ApiErrorModel.fromError(new Error('test'), RateLimitError);
        expect(model).toBeInstanceOf(ApiErrorModel);
    });

    it('ApiErrorType is frozen', () => {
        expect(Object.isFrozen(ApiErrorType)).toBe(true);
    });
});
