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

import { now } from '../../models/base.js';
import { OperatorStatusUpdatedEvent, OperatorStatusUpdatedData } from '../../models/sse_models.js';
import { logger } from '../../utils/logger.js';
import { sessionIdTag } from '../../utils/session_log.js';
import { OperatorStatus, OperatorType } from '../../constants/operator.js';
import { OperatorSessionRole, DeviceLinkError } from '../../constants/auth.js';
import { EventType } from '../../constants/events.js';
import { G8eHttpContext, BoundOperatorContext } from '../../models/request_models.js';
import { DEFAULT_OPERATOR_CONFIG } from '../../constants/operator_defaults.js';
import { getOperatorService } from '../initialization.js';

function sanitizeString(input, maxLength = 255) {
    if (!input || typeof input !== 'string') return '';
    return input
        .slice(0, maxLength)
        .replace(/[\x00-\x1F\x7F]/g, '')
        .replace(/[<>"'&]/g, (char) => {
            const escapeMap = { '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#x27;', '&': '&amp;' };
            return escapeMap[char] || char;
        })
        .trim();
}

function sanitizeFingerprint(input) {
    if (!input || typeof input !== 'string') return '';
    return input.replace(/[^0-9a-fA-F]/g, '').slice(0, 128).toLowerCase();
}

export class DeviceRegistrationService {
    /**
     * @param {Object} options
     * @param {Object} options.operatorService        - OperatorDataService instance
     * @param {Object} options.userService            - UserService instance
     * @param {Object} options.sseService             - SSEService instance
     * @param {Object} options.internalHttpClient     - InternalHttpClient instance
     */
    constructor({ operatorService, userService, sseService, internalHttpClient }) {
        if (!operatorService)        throw new Error('operatorService is required');
        if (!userService)            throw new Error('userService is required');
        if (!sseService)             throw new Error('sseService is required');
        if (!internalHttpClient)     throw new Error('internalHttpClient is required');

        this._operatorService        = operatorService;
        this._userService            = userService;
        this._sseService             = sseService;
        this._internalHttpClient     = internalHttpClient;
    }

    /**
     * Register a device against a specific operator slot.
     * Creates the operator session, claims or reconnects the slot, activates, and fires SSE.
     *
     * @param {{
     *   operator_id:   string,
     *   deviceInfo:    object,
     *   operator_type: string,
     *   g8eContext:    { web_session_id: string|null, user_id: string, organization_id: string|null },
     *   device_link_token: string,
     * }} params
     * @returns {Promise<{ success: boolean, operator_session_id?: string, operator_id?: string, api_key?: string, operator_cert?: string, operator_cert_key?: string, session?: object, error?: string }>}
     */
    async registerDevice({ operator_id: id, deviceInfo, operator_type = OperatorType.SYSTEM, g8eContext, device_link_token }) {
        const { user_id, web_session_id } = g8eContext;

        if (!deviceInfo.system_fingerprint) {
            return { success: false, error: DeviceLinkError.MISSING_FINGERPRINT };
        }

        const sanitized = {
            system_fingerprint: sanitizeFingerprint(deviceInfo.system_fingerprint),
            hostname:           sanitizeString(deviceInfo.hostname, 255),
            os:                 sanitizeString(deviceInfo.os, 32),
            arch:               sanitizeString(deviceInfo.arch, 32),
            username:           sanitizeString(deviceInfo.username, 255),
            ip_address:         deviceInfo.ip_address || null,
        };

        if (!sanitized.system_fingerprint) {
            return { success: false, error: DeviceLinkError.INVALID_FINGERPRINT };
        }

        const user = await this._userService.getUser(user_id);
        if (!user) {
            return { success: false, error: DeviceLinkError.USER_NOT_FOUND };
        }

        const result = await this._operatorService.relayRegisterDeviceLinkToG8ee({
            operator_id: id,
            user_id: user.id,
            organization_id: user.organization_id,
            operator_type: OperatorType.CLOUD,
            device_link_token,
            system_fingerprint: deviceInfo.system_fingerprint,
        }, g8eContext);

        if (!result.success) {
            return { success: false, error: result.error || DeviceLinkError.CLAIM_SLOT_FAILED };
        }

        const operator_session_id = result.operator_session_id;
        const operator_id = result.operator_id;

        logger.info('[DEVICE-REGISTRATION] Device registered for operator via g8ee', {
            operator_id,
            hostname:           sanitized.hostname,
            operator_session_id_tag: sessionIdTag(operator_session_id),
        });

        return {
            success: true,
            operator_session_id,
            operator_id: operator_id,
            api_key: result.api_key,
            operator_cert: result.operator_cert,
            operator_cert_key: result.operator_cert_key,
            session: result.session,
            config: DEFAULT_OPERATOR_CONFIG
        };
    }
}
