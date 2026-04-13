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
import { getInternalHttpClient } from '../clients/internal_http_client.js';
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
        this.internalHttpClient = internalHttpClient;
    }

    _getHttpClient() {
        if (this.internalHttpClient) return this.internalHttpClient;
        try {
            return getInternalHttpClient();
        } catch (e) {
            // During initialization, this may fail if called before Phase 5.
            // This is expected for the initial construction inside OperatorService.
            return null;
        }
    }

    async relayStopCommandToG8ee(g8eContext) {
        this._validateContext(g8eContext);
        const httpClient = this._getHttpClient();
        if (!httpClient) throw new Error('InternalHttpClient not initialized');
        
        const boundOperator = g8eContext.bound_operators?.[0];
        if (!boundOperator) throw new Error('No bound operator found in context for stop command');
        
        logger.info('[OPERATOR-RELAY] Relaying stop command to g8ee', {
            operator_id: boundOperator.operator_id,
            operator_session_id: boundOperator.operator_session_id?.substring(0, 12) + '...'
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
            operator_session_id: boundOperator.operator_session_id?.substring(0, 12) + '...',
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
            operator_session_id: boundOperator.operator_session_id?.substring(0, 20) + '...',
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

    _validateContext(g8eContext) {
        if (!g8eContext) {
            throw new Error('ENFORCEMENT VIOLATION: g8eContext is REQUIRED for g8ee calls');
        }
    }
}
