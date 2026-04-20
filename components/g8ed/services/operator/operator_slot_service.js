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
import { OperatorStatus, HistoryEventType, OperatorType, CloudOperatorSubtype, DEFAULT_OPERATOR_SLOTS, DEFAULT_SLOT_COST } from '../../constants/operator.js';
import { OperatorDocument, HistoryEntry, SystemInfo, CertInfo, OperatorRefreshKeyResponse, OperatorSlotCreationResponse } from '../../models/operator_model.js';
import { now } from '../../models/base.js';
import { SourceComponent } from '../../constants/ai.js';
import { ApiKeyStatus, ApiKeyClientName, ApiKeyPermission, DeviceLinkError } from '../../constants/auth.js';
import crypto from 'crypto';
import { API_KEY_PREFIX } from '../../constants/operator_defaults.js';

export class OperatorSlotService {
    constructor({ operatorDataService, apiKeyService, certificateService, operatorSessionService }) {
        this.operatorDataService = operatorDataService;
        this.apiKeyService = apiKeyService;
        this.certificateService = certificateService;
        this.operatorSessionService = operatorSessionService;
    }

    async initializeOperatorSlots(userId, organizationId) {
        const existingOperators = await this.operatorDataService.queryOperatorsFresh([
            { field: 'user_id', operator: '==', value: userId }
        ]);

        const liveOperators = existingOperators.filter(op => op.status !== OperatorStatus.TERMINATED);
        const existingCount = liveOperators.length;
        const createdSlotIds = [];

        if (existingCount < DEFAULT_OPERATOR_SLOTS) {
            const hasG8eNode = liveOperators.some(op => op.is_g8ep === true);
            const slotsToCreate = DEFAULT_OPERATOR_SLOTS - existingCount;
            let g8eNodeAssigned = hasG8eNode;
            for (let i = 0; i < slotsToCreate; i++) {
                const slotNumber = existingCount + i + 1;
                const assignG8eNode = !g8eNodeAssigned;
                if (assignG8eNode) g8eNodeAssigned = true;
                const creationResponse = await this.createOperatorSlot({
                    userId,
                    organizationId,
                    slotNumber,
                    operatorType: OperatorType.CLOUD,
                    cloudSubtype: assignG8eNode ? CloudOperatorSubtype.G8E_POD : null,
                    namePrefix: 'operator',
                    isG8eNode: assignG8eNode,
                });
                if (creationResponse.success && creationResponse.operator_id) {
                    createdSlotIds.push(creationResponse.operator_id);
                }
            }
        }

        const allOperatorIds = liveOperators.map(op => op.operator_id).concat(createdSlotIds);
        return allOperatorIds;
    }

    async refreshOperatorApiKey(operatorId, userId, broadcastFn) {
        const oldOperator = await this.operatorDataService.getOperator(operatorId);
        if (!oldOperator) return OperatorRefreshKeyResponse.forFailure(DeviceLinkError.OPERATOR_NOT_FOUND);
        if (oldOperator.user_id !== userId) return OperatorRefreshKeyResponse.forFailure('Unauthorized');

        const ts = now();
        const oldApiKey = oldOperator.api_key;

        if (oldOperator.operator_session_id && this.operatorSessionService) {
            await this.operatorSessionService.endSession(oldOperator.operator_session_id).catch(() => {});
        }

        if (oldApiKey && this.apiKeyService) {
            await this.apiKeyService.revokeKey(oldApiKey);
        }

        if (oldOperator.operator_cert_serial && this.certificateService) {
            await this.certificateService.revokeCertificate(oldOperator.operator_cert_serial, 'api_key_refresh', operatorId).catch(() => {});
        }

        const terminateData = {
            status: OperatorStatus.TERMINATED,
            terminated_at: ts,
            updated_at: ts,
            operator_session_id: null,
            bound_web_session_id: null,
        };

        await this.operatorDataService.updateOperator(operatorId, terminateData);

        const slotNumber = oldOperator.slot_number;
        const newOperatorId = `${userId}_operator_${slotNumber}_${Date.now()}_${Math.random().toString(36).substring(7)}`;
        const newApiKey = this.generateOperatorApiKey(newOperatorId);

        let newCertInfo = CertInfo.empty();
        if (this.certificateService) {
            try {
                const certData = await this.certificateService.generateOperatorCertificate(newOperatorId, userId, oldOperator.organization_id);
                newCertInfo = CertInfo.fromCertData(certData);
            } catch (e) {}
        }

        const newOperatorDoc = OperatorDocument.forRefresh({
            newOperatorId,
            userId,
            organizationId: oldOperator.organization_id,
            name: oldOperator.name || `operator-${slotNumber}`,
            slotNumber,
            operatorType: oldOperator.operator_type || OperatorType.SYSTEM,
            cloudSubtype: oldOperator.cloud_subtype || null,
            isG8eNode: oldOperator.is_g8ep ?? false,
            slotCost: oldOperator.slot_cost ?? 1,
            newApiKey,
            certInfo: newCertInfo,
            oldOperatorId: operatorId,
            oldCertSerial: oldOperator.operator_cert_serial
        });

        await this.operatorDataService.createOperator(newOperatorId, newOperatorDoc);

        if (this.apiKeyService) {
            await this.apiKeyService.issueKey(newApiKey, {
                user_id: userId,
                organization_id: oldOperator.organization_id,
                operator_id: newOperatorId,
                client_name: ApiKeyClientName.OPERATOR,
                permissions: [ApiKeyPermission.OPERATOR_BOOTSTRAP, ApiKeyPermission.OPERATOR_HEARTBEAT, ApiKeyPermission.OPERATOR_DOWNLOAD],
                status: ApiKeyStatus.ACTIVE
            });
        }

        await broadcastFn(userId);

        return OperatorRefreshKeyResponse.forSuccess(newApiKey, newOperatorId);
    }

    async createOperatorSlot(params) {
        const { userId, organizationId, slotNumber, operatorType, cloudSubtype, namePrefix, isG8eNode } = params;
        const operatorId = `${userId}_operator_${slotNumber}_${Date.now()}_${Math.random().toString(36).substring(7)}`;
        const apiKey = this.generateOperatorApiKey(operatorId);

        let certInfo = CertInfo.empty();
        if (this.certificateService) {
            try {
                const certData = await this.certificateService.generateOperatorCertificate(operatorId, userId, organizationId);
                certInfo = CertInfo.fromCertData(certData);
            } catch (certError) {
                logger.warn('[OPERATOR-SLOT] Certificate generation failed at slot creation', { operator_id: operatorId, error: certError.message });
            }
        }

        const operatorDoc = OperatorDocument.forSlot({
            operator_id: operatorId,
            userId,
            organizationId,
            namePrefix,
            slotNumber,
            operatorType,
            cloudSubtype,
            isG8eNode,
            operatorApiKey: apiKey,
            certInfo
        });

        const result = await this.operatorDataService.createOperator(operatorId, operatorDoc);
        if (!result.success) {
            logger.error('[OPERATOR-SLOT] Failed to create operator slot', { operator_id: operatorId });
            return OperatorSlotCreationResponse.forFailure('Failed to create operator document');
        }

        if (this.apiKeyService) {
            await this.apiKeyService.issueKey(apiKey, {
                user_id: userId,
                organization_id: organizationId,
                operator_id: operatorId,
                client_name: ApiKeyClientName.OPERATOR,
                permissions: [ApiKeyPermission.OPERATOR_BOOTSTRAP, ApiKeyPermission.OPERATOR_HEARTBEAT, ApiKeyPermission.OPERATOR_DOWNLOAD],
                status: ApiKeyStatus.ACTIVE
            }).catch(err => {
                logger.error('[OPERATOR-SLOT] Failed to issue API key for slot', { operator_id: operatorId, error: err.message });
            });
        }

        return OperatorSlotCreationResponse.forSuccess(operatorId);
    }

    /**
     * Claim an operator slot for an active session.
     * Authority for transitioning an AVAILABLE slot to ACTIVE.
     */
    async claimSlot(operatorId, { operator_session_id, bound_web_session_id, system_info, operator_type, status }) {
        const ts = now();
        const info = system_info instanceof SystemInfo
            ? system_info
            : SystemInfo.parse(system_info || {});

        const operator = await this.operatorDataService.getOperator(operatorId);
        
        const updateData = {
            status: status || OperatorStatus.ACTIVE,
            operator_session_id,
            bound_web_session_id,
            system_info: info,
            claimed: true,
            updated_at: ts,
            last_heartbeat: ts,
        };

        // Set first_deployed if not already set (first time operator becomes ACTIVE)
        if (operator && !operator.first_deployed) {
            updateData.first_deployed = ts;
        }

        if (operator_type) {
            updateData.operator_type = operator_type;
        }

        const historyEntry = new HistoryEntry({
            timestamp: ts,
            event_type: HistoryEventType.STATUS_CHANGED,
            summary: 'Operator slot claimed and activated via device registration',
            actor: SourceComponent.G8ED,
            details: {
                operator_session_id,
                bound_web_session_id,
                hostname: info.hostname,
                fingerprint: info.system_fingerprint
            }
        });

        // Atomic update to both state and history
        return await this.operatorDataService.updateOperator(operatorId, {
            ...updateData,
            $push: { history_trail: historyEntry.forDB() }
        });
    }

    generateOperatorApiKey(operatorId) {
        const operatorSuffix = operatorId.split('_').pop().substring(0, 8);
        const randomToken = crypto.randomBytes(32).toString('hex');
        return `${API_KEY_PREFIX}${operatorSuffix}_${randomToken}`;
    }

}
