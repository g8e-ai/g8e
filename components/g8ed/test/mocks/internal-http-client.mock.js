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

import { vi } from 'vitest';

/**
 * MockInternalHttpClient — unit-test stand-in for InternalHttpClient.
 */
export class MockInternalHttpClient {
    constructor() {
        // Mock nested structure used by some legacy services (e.g., CertificateService)
        this._bootstrapService = {
            _operatorClient: {
                post: vi.fn().mockResolvedValue({ success: true })
            }
        };

        // g8ee endpoints
        this.sendChatMessage = vi.fn();
        this.deleteCase = vi.fn();
        this.stopAIProcessing = vi.fn();
        this.recordTriageAnswer = vi.fn();
        this.skipTriageQuestions = vi.fn();
        this.timeoutTriageQuestions = vi.fn();
        this.generateApiKey = vi.fn();
        this.revokeCertificate = vi.fn();
        this.syncUserSettings = vi.fn();
        this.terminateOperator = vi.fn();
        this.stopOperator = vi.fn();
        this.respondToApproval = vi.fn();
        this.sendDirectCommand = vi.fn();
        this.registerOperatorSession = vi.fn();

        // g8eo (SUBSTRATE) endpoints
        this.listOperators = vi.fn().mockResolvedValue({ success: true, operators: [] });
        this.rotateOperatorApiKey = vi.fn().mockResolvedValue({ success: true });
        this.terminateOperatorSubstrate = vi.fn().mockResolvedValue({ success: true });
        this.bindOperators = vi.fn().mockResolvedValue({ success: true });
        this.unbindOperators = vi.fn().mockResolvedValue({ success: true });
        this.setTargetContext = vi.fn().mockResolvedValue({ success: true });
        this.listDeviceLinks = vi.fn().mockResolvedValue({ success: true, links: [] });
        this.createDeviceLink = vi.fn().mockResolvedValue({ success: true, token: 'dlk_mock_token' });
        this.deleteDeviceLink = vi.fn().mockResolvedValue({ success: true });

        // g8eo PASSKEY endpoints
        this.passkeyRegisterChallenge = vi.fn().mockResolvedValue({ success: true, challenge: 'mock_reg_challenge' });
        this.passkeyRegisterVerify = vi.fn().mockResolvedValue({ success: true });
        this.passkeyAuthChallenge = vi.fn().mockResolvedValue({ success: true, challenge: 'mock_auth_challenge' });
        this.passkeyAuthVerify = vi.fn().mockResolvedValue({ success: true, session_id: 'mock_session_id' });
        this.passkeyCredentials = vi.fn().mockResolvedValue({ success: true, credentials: [] });
        this.passkeyRevokeCredential = vi.fn().mockResolvedValue({ success: true });

        this.healthCheck = vi.fn().mockResolvedValue({ g8ee: { status: 'healthy' } });
    }

    _reset() {
        vi.clearAllMocks();
    }
}

/**
 * Create a MockInternalHttpClient instance.
 * @returns {MockInternalHttpClient}
 */
export function createMockInternalHttpClient() {
    return new MockInternalHttpClient();
}
