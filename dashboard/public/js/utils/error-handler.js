// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { RateLimitError } from './service-client.js';
import { notificationService } from './notification-service.js';
import { devLogger } from './dev-logger.js';
import { ApiErrorModel } from '../models/api-error-model.js';

export function handleApiError(error) {
    devLogger.error('[ErrorHandler] Handling API error:', error);

    const apiError = ApiErrorModel.fromError(error, RateLimitError);

    if (apiError.isWarning) {
        notificationService.warning(apiError.message);
    } else {
        notificationService.error(apiError.message);
    }
}

window.handleApiError = handleApiError;
