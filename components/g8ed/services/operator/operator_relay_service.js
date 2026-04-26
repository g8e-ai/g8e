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

import { logger } from '../../utils/logger.js';
import { sessionIdTag } from '../../utils/session_log.js';
import { ApiPaths } from '../../constants/api_paths.js';
import {
    StopOperatorRequest,
    OperatorSessionRegistrationRequest,
    DirectCommandRequest,
} from '../../models/request_models.js';

export class OperatorRelayService {
    constructor({
        internalHttpClient
    } = {}) {
        if (!internalHttpClient) {
            throw new Error('OperatorRelayService requires internalHttpClient');
        }
        this.internalHttpClient = internalHttpClient;
    }

    _getHttpClient() {
        return this.internalHttpClient;
    }

    async relayStopCommandToG8ee(g8eContext) {
        this._validateContext(g8eContext);
        const httpClient = this._getHttpClient();
        if (!httpClient) throw new Error('InternalHttpClient not initialized');
        
        const boundOperator = g8eContext.bound_operators?.[0];
        if (!boundOperator) throw new Error('No bound operator found in context for stop command');
        
        logger.info('[OPERATOR-RELAY] Relaying stop command to g8ee', {
            operator_id: boundOperator.operator_id,
            operator_session_id_tag: sessionIdTag(boundOperator.operator_session_id)
        });

        const request = new StopOperatorRequest({
            operator_id: boundOperator.operator_id,
            operator_session_id: boundOperator.operator_session_id,
            user_id: g8eContext.user_id,
        });

        return httpClient.request('g8ee', ApiPaths.g8ee.operatorsStop(), {
            method: 'POST',
            body: request.forWire(),
            g8eContext
        });
    }

    async deregisterOperatorSessionInG8ee(g8eContext) {
        this._validateContext(g8eContext);
        const httpClient = this._getHttpClient();
        if (!httpClient) throw new Error('InternalHttpClient not initialized');

        const boundOperator = g8eContext.bound_operators?.[0];
        if (!boundOperator) throw new Error('No bound operator found in context for deregistration');

        logger.info('[OPERATOR-RELAY] Deregistering operator session heartbeat subscription in g8ee', {
            operator_id: boundOperator.operator_id,
            operator_session_id_tag: sessionIdTag(boundOperator.operator_session_id),
        });

        const request = new OperatorSessionRegistrationRequest({
            operator_id: boundOperator.operator_id,
            operator_session_id: boundOperator.operator_session_id,
        });

        return httpClient.request('g8ee', ApiPaths.g8ee.operatorsDeregisterSession(), {
            method: 'POST',
            body: request.forWire(),
            g8eContext,
        });
    }

    async relayDirectCommandToG8ee(commandData, g8eContext) {
        this._validateContext(g8eContext);
        const httpClient = this._getHttpClient();
        if (!httpClient) throw new Error('InternalHttpClient not initialized');

        const boundOperator = g8eContext.bound_operators?.[0];
        if (!boundOperator) throw new Error('No bound operator found in context for direct command');
        
        const directCommandRequest = DirectCommandRequest.parse(commandData);

        logger.info('[OPERATOR-RELAY] Relaying direct command to operator via g8ee', {
            executionId: directCommandRequest.execution_id,
            operatorId: boundOperator.operator_id
        });

        return httpClient.request('g8ee', ApiPaths.g8ee.operatorDirectCommand(), {
            method: 'POST',
            body: directCommandRequest.forWire(),
            g8eContext
        });
    }

    async relayRegisterOperatorSessionToG8ee(g8eContext) {
        this._validateContext(g8eContext);
        const httpClient = this._getHttpClient();
        if (!httpClient) throw new Error('InternalHttpClient not initialized');

        const boundOperator = g8eContext.bound_operators?.[0];
        if (!boundOperator) throw new Error('No bound operator found in context for registration');

        logger.info('[OPERATOR-RELAY] Registering operator session heartbeat subscription in g8ee', {
            operator_id: boundOperator.operator_id,
            operator_session_id_tag: sessionIdTag(boundOperator.operator_session_id),
        });

        // Use the context fields for the request
        const request = new OperatorSessionRegistrationRequest({
            operator_id: boundOperator.operator_id,
            operator_session_id: boundOperator.operator_session_id,
        });

        // Non-blocking fire-and-forget for operator session registration
        // Errors are logged but do not block the main flow as registration is transient
        httpClient.request('g8ee', ApiPaths.g8ee.operatorsRegisterSession(), {
            method: 'POST',
            body: request.forWire(),
            g8eContext,
        }).catch(err => {
            logger.error('[OPERATOR-RELAY] Operator session registration failed (non-blocking)', {
                operatorId: boundOperator.operator_id,
                error: err.message
            });
        });

        return { success: true };
    }

    async relayApprovalResponseToG8ee(approvalData, g8eContext) {
        this._validateContext(g8eContext);
        const httpClient = this._getHttpClient();
        if (!httpClient) throw new Error('InternalHttpClient not initialized');

        logger.info('[OPERATOR-RELAY] Relaying operator approval response to g8ee', {
            approvalId: approvalData.approval_id,
            approved: approvalData.approved,
            caseId: g8eContext.case_id
        });

        return httpClient.request('g8ee', ApiPaths.g8ee.operatorApprovalRespond(), {
            method: 'POST',
            body: approvalData,
            g8eContext
        });
    }

    async relayPendingApprovalsFromG8ee(g8eContext) {
        this._validateContext(g8eContext);
        const httpClient = this._getHttpClient();
        if (!httpClient) throw new Error('InternalHttpClient not initialized');

        logger.info('[OPERATOR-RELAY] Fetching pending approvals from g8ee', {
            caseId: g8eContext.case_id,
            investigationId: g8eContext.investigation_id
        });

        return httpClient.request('g8ee', ApiPaths.g8ee.operatorApprovalPending(), {
            method: 'GET',
            g8eContext
        });
    }

    async relayCreateOperatorSlotToG8ee(params, g8eContext) {
        this._validateContext(g8eContext);
        const httpClient = this._getHttpClient();
        if (!httpClient) throw new Error('InternalHttpClient not initialized');

        logger.info('[OPERATOR-RELAY] Creating operator slot via g8ee', {
            user_id: params.user_id,
            slot_number: params.slot_number,
        });

        return httpClient.request('g8ee', ApiPaths.g8ee.operatorsCreateSlot(), {
            method: 'POST',
            body: params,
            g8eContext
        });
    }

    async relayClaimOperatorSlotToG8ee(params, g8eContext) {
        this._validateContext(g8eContext);
        const httpClient = this._getHttpClient();
        if (!httpClient) throw new Error('InternalHttpClient not initialized');

        logger.info('[OPERATOR-RELAY] Claiming operator slot via g8ee', {
            operator_id: params.operator_id,
        });

        return httpClient.request('g8ee', ApiPaths.g8ee.operatorsClaimSlot(), {
            method: 'POST',
            body: params,
            g8eContext
        });
    }

    async relayBindOperatorsToG8ee(params, g8eContext) {
        this._validateContext(g8eContext);
        const httpClient = this._getHttpClient();
        if (!httpClient) throw new Error('InternalHttpClient not initialized');

        logger.info('[OPERATOR-RELAY] Binding operators via g8ee', {
            operator_ids: params.operator_ids,
        });

        return httpClient.request('g8ee', ApiPaths.g8ee.operatorsBind(), {
            method: 'POST',
            body: params,
            g8eContext
        });
    }

    async relayUnbindOperatorsToG8ee(params, g8eContext) {
        this._validateContext(g8eContext);
        const httpClient = this._getHttpClient();
        if (!httpClient) throw new Error('InternalHttpClient not initialized');

        logger.info('[OPERATOR-RELAY] Unbinding operators via g8ee', {
            operator_ids: params.operator_ids,
        });

        return httpClient.request('g8ee', ApiPaths.g8ee.operatorsUnbind(), {
            method: 'POST',
            body: params,
            g8eContext
        });
    }

    async relayAuthenticateOperatorToG8ee(params, g8eContext) {
        this._validateContext(g8eContext);
        const httpClient = this._getHttpClient();
        if (!httpClient) throw new Error('InternalHttpClient not initialized');

        logger.info('[OPERATOR-RELAY] Authenticating operator via g8ee', {
            auth_mode: params.auth_mode,
        });

        return httpClient.request('g8ee', ApiPaths.g8ee.operatorsAuthenticate(), {
            method: 'POST',
            body: params,
            g8eContext
        });
    }

    async relayValidateOperatorSessionToG8ee(operatorSessionId, g8eContext) {
        this._validateContext(g8eContext);
        const httpClient = this._getHttpClient();
        if (!httpClient) throw new Error('InternalHttpClient not initialized');

        logger.info('[OPERATOR-RELAY] Validating operator session via g8ee', {
            operator_session_id_tag: sessionIdTag(operatorSessionId),
        });

        return httpClient.request('g8ee', ApiPaths.g8ee.operatorsValidateSession(), {
            method: 'POST',
            body: { operator_session_id: operatorSessionId },
            g8eContext
        });
    }

    async relayRefreshOperatorSessionToG8ee(operatorSessionId, g8eContext) {
        this._validateContext(g8eContext);
        const httpClient = this._getHttpClient();
        if (!httpClient) throw new Error('InternalHttpClient not initialized');

        logger.info('[OPERATOR-RELAY] Refreshing operator session via g8ee', {
            operator_session_id_tag: sessionIdTag(operatorSessionId),
        });

        return httpClient.request('g8ee', ApiPaths.g8ee.operatorsRefreshSession(), {
            method: 'POST',
            body: { operator_session_id: operatorSessionId },
            g8eContext
        });
    }

    async relayEndOperatorSessionToG8ee(operatorSessionId, g8eContext) {
        this._validateContext(g8eContext);
        const httpClient = this._getHttpClient();
        if (!httpClient) throw new Error('InternalHttpClient not initialized');

        logger.info('[OPERATOR-RELAY] Ending operator session via g8ee', {
            operator_session_id_tag: sessionIdTag(operatorSessionId),
        });

        return httpClient.request('g8ee', ApiPaths.g8ee.operatorsDeregisterSession(), {
            method: 'POST',
            body: { operator_session_id: operatorSessionId },
            g8eContext,
        });
    }

    async relayTerminateOperatorToG8ee(operatorId, g8eContext) {
        this._validateContext(g8eContext);
        const httpClient = this._getHttpClient();
        if (!httpClient) throw new Error('InternalHttpClient not initialized');

        logger.info('[OPERATOR-RELAY] Terminating operator via g8ee', {
            operator_id: operatorId,
        });

        return httpClient.request('g8ee', ApiPaths.g8ee.operatorsTerminate(), {
            method: 'POST',
            body: { operator_id: operatorId },
            g8eContext
        });
    }

    async relayListenSessionAuthToG8ee(params, g8eContext) {
        this._validateContext(g8eContext);
        const httpClient = this._getHttpClient();
        if (!httpClient) throw new Error('InternalHttpClient not initialized');

        logger.info('[OPERATOR-RELAY] Starting session auth listener via g8ee', {
            operator_id: params.operator_id,
        });

        return httpClient.request('g8ee', ApiPaths.g8ee.operatorsListenSessionAuth(), {
            method: 'POST',
            body: params,
            g8eContext
        });
    }

    _validateContext(g8eContext) {
        if (!g8eContext) {
            throw new Error('ENFORCEMENT VIOLATION: g8eContext is REQUIRED for g8ee calls');
        }
    }
}
