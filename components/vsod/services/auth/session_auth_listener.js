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
import { logger } from '../../utils/logger.js';
import { SessionAuthResponse } from '../../models/request_models.js';
import { ApiKeyError } from '../../constants/auth.js';
import { SESSION_AUTH_LISTEN_TTL_MS } from '../../constants/auth.js';
import { PubSubChannel } from '../../constants/channels.js';
import { DEFAULT_OPERATOR_CONFIG } from '../../constants/operator_defaults.js';

export class SessionAuthListener {
    /**
     * @param {Object} options
     * @param {Object} options.pubSubClient           - VSODBPubSubClient instance
     * @param {Object} options.operatorSessionService - OperatorSessionService instance
     * @param {Object} options.operatorService        - OperatorDataService instance
     */
    constructor({ pubSubClient, operatorSessionService, operatorService }) {
        if (!pubSubClient)           throw new Error('pubSubClient is required');
        if (!operatorSessionService) throw new Error('operatorSessionService is required');
        if (!operatorService)        throw new Error('operatorService is required');

        this._pubSubClient           = pubSubClient;
        this._operatorSessionService = operatorSessionService;
        this._operatorService        = operatorService;
    }

    /**
     * Subscribe to the session auth channel for this operator_session_id and
     * respond with bootstrap config once. Called fire-and-forget after device registration.
     *
     * @param {{ operator_session_id: string, operator_id: string, user_id: string, organization_id: string|null }} vsoContext
     */
    listen(vsoContext) {
        const { operator_session_id, operator_id, user_id, organization_id } = vsoContext;
        const sessionHash     = crypto.createHash('sha256').update(operator_session_id).digest('hex');
        const authChannel     = `${PubSubChannel.AUTH_PUBLISH_SESSION_PREFIX}${sessionHash}`;
        const responseChannel = `${PubSubChannel.AUTH_RESPONSE_SESSION_PREFIX}${sessionHash}`;

        let subscriber = null;
        const cleanup = () => { try { subscriber?.terminate(); } catch (_) {} };
        const timer = setTimeout(cleanup, SESSION_AUTH_LISTEN_TTL_MS);

        this._pubSubClient.duplicate().then(sub => {
            subscriber = sub;

            subscriber.on('message', async (channel, _data) => {
                if (channel !== authChannel) return;
                clearTimeout(timer);

                try {
                    const sessionData = await this._operatorSessionService.validateSession(operator_session_id);

                    if (!sessionData || !sessionData.is_active) {
                        await subscriber.publish(responseChannel, new SessionAuthResponse({
                            success: false,
                            error:   'Session not found or expired',
                        }).forKV());
                        return;
                    }

                    const operator = await this._operatorService.getOperator(operator_id);
                    const api_key  = operator?.api_key || null;

                    await subscriber.publish(responseChannel, new SessionAuthResponse({
                        success:            true,
                        operator_session_id,
                        operator_id,
                        user_id,
                        organization_id,
                        api_key,
                        config:             { ...DEFAULT_OPERATOR_CONFIG },
                        operator_cert:      null,
                        operator_cert_key:  null,
                    }).forKV());

                    logger.info('[SESSION-AUTH-LISTENER] Auth response published', {
                        operator_id,
                        operator_session_id: operator_session_id.substring(0, 12) + '...',
                    });
                } catch (err) {
                    logger.error('[SESSION-AUTH-LISTENER] Failed to handle session auth request', { error: err.message });
                    try {
                        await subscriber.publish(responseChannel, new SessionAuthResponse({
                            success: false,
                            error:   ApiKeyError.AUTH_FAILED,
                        }).forKV());
                    } catch (_) {}
                } finally {
                    cleanup();
                }
            });

            return subscriber.subscribe(authChannel);
        }).then(() => {
            logger.info('[SESSION-AUTH-LISTENER] Listening for session auth', {
                operator_id,
                channel: authChannel,
            });
        }).catch(err => {
            clearTimeout(timer);
            cleanup();
            logger.error('[SESSION-AUTH-LISTENER] Failed to set up session auth listener', { error: err.message });
        });
    }
}
