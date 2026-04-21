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
import { SystemInfo } from '../../models/operator_model.js';
import { OperatorStatusUpdatedEvent, OperatorStatusUpdatedData } from '../../models/sse_models.js';
import { logger } from '../../utils/logger.js';
import { OperatorStatus, OperatorType } from '../../constants/operator.js';
import { OperatorSessionRole, DeviceLinkError } from '../../constants/auth.js';
import { EventType } from '../../constants/events.js';
import { G8eHttpContext, BoundOperatorContext } from '../../models/request_models.js';

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
     * @param {Object} options.operatorSessionService - OperatorSessionService instance
     * @param {Object} options.userService            - UserService instance
     * @param {Object} options.sseService             - SSEService instance
     * @param {Object} options.internalHttpClient     - InternalHttpClient instance
     * @param {Object} options.sessionAuthListener    - SessionAuthListener instance
     */
    constructor({ operatorService, operatorSessionService, userService, sseService, internalHttpClient, sessionAuthListener }) {
        if (!operatorService)        throw new Error('operatorService is required');
        if (!operatorSessionService) throw new Error('operatorSessionService is required');
        if (!userService)            throw new Error('userService is required');
        if (!sseService)             throw new Error('sseService is required');
        if (!internalHttpClient)     throw new Error('internalHttpClient is required');
        if (!sessionAuthListener)    throw new Error('sessionAuthListener is required');

        this._operatorService        = operatorService;
        this._operatorSessionService = operatorSessionService;
        this._userService            = userService;
        this._sseService             = sseService;
        this._internalHttpClient     = internalHttpClient;
        this._sessionAuthListener    = sessionAuthListener;
    }

    /**
     * Register a device against a specific operator slot.
     * Creates the operator session, claims or reconnects the slot, activates, and fires SSE.
     *
     * @param {{
     *   id:   string,
     *   deviceInfo:    object,
     *   operator_type: string,
     *   g8eContext:    { web_session_id: string|null, user_id: string, organization_id: string|null },
     * }} params
     * @returns {Promise<{ success: boolean, operator_session_id?: string, id?: string, system_info?: object, error?: string }>}
     */
    async registerDevice({ id, deviceInfo, operator_type = OperatorType.SYSTEM, g8eContext }) {
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

        const operator = await this._operatorService.getOperator(id);
        if (!operator) {
            return { success: false, error: DeviceLinkError.OPERATOR_NOT_FOUND };
        }

        const user = await this._userService.getUser(user_id);
        if (!user) {
            return { success: false, error: DeviceLinkError.USER_NOT_FOUND };
        }

        const sessionData = {
            user_id: user.id,
            user_data: {
                email:           user.email,
                name:            user.name,
                picture:         user.profile_picture,
                id:              user.id,
                organization_id: user.organization_id,
                roles:           user.roles || [OperatorSessionRole.OPERATOR],
            },
            id,
        };

        const session = await this._operatorSessionService.createOperatorSession(sessionData);
        const operator_session_id = session.id;

        const system_info = SystemInfo.parse({
            system_fingerprint: sanitized.system_fingerprint,
            hostname:           sanitized.hostname,
            os:                 sanitized.os,
            architecture:       sanitized.arch,
            current_user:       sanitized.username,
        });

        const claimed = await this._operatorService.claimOperatorSlot(id, {
            operator_session_id,
            bound_web_session_id: web_session_id,
            system_info,
            operator_type,
        });

        if (!claimed) {
            await this._operatorSessionService.endSession(operator_session_id);
            return { success: false, error: DeviceLinkError.CLAIM_SLOT_FAILED };
        }

        await this._userService.updateUserOperator(user_id, id, OperatorStatus.ACTIVE);

        const relayContext = G8eHttpContext.parse({
            web_session_id:  web_session_id || operator_session_id,
            user_id,
            organization_id: g8eContext.organization_id || null,
            bound_operators: [
                BoundOperatorContext.parse({
                    id,
                    operator_session_id,
                    bound_web_session_id: web_session_id,
                    status:        OperatorStatus.ACTIVE,
                    operator_type,
                    system_info,
                })
            ],
        });

        if (web_session_id) {
            const event = OperatorStatusUpdatedEvent.parse({
                type: EventType.OPERATOR_STATUS_UPDATED_ACTIVE,
                data: OperatorStatusUpdatedData.parse({
                    id,
                    status: OperatorStatus.ACTIVE,
                    system_info,
                }),
                timestamp: now(),
            });
            
            await this._sseService.publishEvent(web_session_id, event);

            try {
                await this._operatorService.relayRegisterOperatorSessionToG8ee(relayContext);
            } catch (hbError) {
                logger.warn('[DEVICE-REGISTRATION] Failed to register operator session heartbeat subscription', {
                    id,
                    error: hbError.message,
                });
            }
        }

        logger.info('[DEVICE-REGISTRATION] Device registered for operator', {
            id,
            hostname:           sanitized.hostname,
            operator_session_id: operator_session_id.substring(0, 12) + '...',
        });

        this._sessionAuthListener.listen({
            operator_session_id,
            id,
            user_id,
            organization_id: g8eContext.organization_id || null,
        });

        return { success: true, operator_session_id, id, system_info };
    }
}
