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

import { describe, it, expect, beforeEach, vi } from 'vitest';
import fs from 'fs';
import path from 'path';
import { G8ENodeOperatorService } from '@g8ed/services/platform/g8ep_operator_service.js';
import { OperatorStatus } from '@g8ed/constants/operator.js';
import pathConstants from '/app/shared/constants/paths.json';

global.fetch = vi.fn();

function makeSettingsService(overrides = {}) {
    return {
        getPlatformSettings: vi.fn().mockResolvedValue({
            supervisor_port: '443',
            internal_auth_token: 'test-token',
            ...overrides,
        }),
        savePlatformSettings: vi.fn().mockResolvedValue({ success: true, saved: ['g8ep_operator_api_key'] }),
    };
}

const XML_OK = '<?xml version="1.0"?><methodResponse><params><param><value><boolean>1</boolean></value></param></params></methodResponse>';
const XML_ALREADY_STARTED = '<?xml version="1.0"?><methodResponse><fault><value><struct><member><name>faultCode</name><value><int>60</int></value></member><member><name>faultString</name><value><string>ALREADY_STARTED</string></value></member></struct></value></fault></methodResponse>';

function mockFetchOk() {
    fetch.mockResolvedValue({ ok: true, text: () => Promise.resolve(XML_OK) });
}

describe('G8ENodeOperatorService [UNIT]', () => {
    let operatorService;
    let settingsService;
    let internalHttpClient;
    let service;

    beforeEach(() => {
        vi.clearAllMocks();
        operatorService = {
            queryOperators: vi.fn(),
            collectionName: 'operators',
            resetOperator: vi.fn(),
            getOperator: vi.fn(),
        };
        settingsService = makeSettingsService();
        internalHttpClient = {
            activateG8EPOperator: vi.fn(),
            relaunchG8EPOperator: vi.fn(),
        };
        service = new G8ENodeOperatorService({ settingsService, operatorService, internalHttpClient });
    });

    describe('constructor', () => {
        it('throws if settingsService is missing', () => {
            expect(() => new G8ENodeOperatorService({ operatorService }))
                .toThrow('G8ENodeOperatorService requires settingsService');
        });
    });

    describe('getG8ENodeOperatorForUser', () => {
        it('returns operator and active status', async () => {
            const mockOp = { id: 'op_123', user_id: 'user_123', status: OperatorStatus.ACTIVE };
            operatorService.queryOperators.mockResolvedValue([mockOp]);

            const result = await service.getG8ENodeOperatorForUser('user_123');

            expect(result.operator.id).toBe('op_123');
            expect(result.alreadyActive).toBe(true);
            expect(operatorService.queryOperators).toHaveBeenCalledWith(
                expect.arrayContaining([
                    { field: 'user_id', operator: '==', value: 'user_123' },
                    { field: 'is_g8ep', operator: '==', value: true },
                ])
            );
        });

        it('returns null if no slot found', async () => {
            operatorService.queryOperators.mockResolvedValue([]);
            const result = await service.getG8ENodeOperatorForUser('user_123');
            expect(result).toBeNull();
        });
    });

    describe('relaunchG8ENodeOperatorForUser', () => {
        it('delegates to internalHttpClient.relaunchG8EPOperator', async () => {
            const mockResult = { success: true, operator_id: 'op_123' };
            internalHttpClient.relaunchG8EPOperator.mockResolvedValue(mockResult);

            const result = await service.relaunchG8ENodeOperatorForUser('user_123');

            expect(result).toEqual(mockResult);
            expect(internalHttpClient.relaunchG8EPOperator).toHaveBeenCalledWith('user_123');
        });
    });

    describe('activateG8ENodeOperatorForUser', () => {
        it('delegates to internalHttpClient.activateG8EPOperator', async () => {
            internalHttpClient.activateG8EPOperator.mockResolvedValue({ success: true });

            await service.activateG8ENodeOperatorForUser('user_123', null, 'sess_1');

            expect(internalHttpClient.activateG8EPOperator).toHaveBeenCalledWith('user_123');
        });

        it('does not throw when activation fails', async () => {
            internalHttpClient.activateG8EPOperator.mockRejectedValue(new Error('Internal Server Error'));

            await expect(
                service.activateG8ENodeOperatorForUser('user_123', null, 'sess_1')
            ).resolves.toBeUndefined();
        });
    });

    describe('g8ep script validation', () => {
        const scriptsDir = pathConstants.g8ep.scripts_dir;
        const g8esUrlPattern = /https:\/\/g8es(?::(\d+))?/g;
        const scriptPath = path.join(scriptsDir, 'fetch-key-and-run.sh');

        beforeAll(() => {
            if (!fs.existsSync(scriptPath)) {
                console.warn(`[SKIP] g8ep script validation tests: ${scriptPath} not found in test environment`);
            }
        });

        function extractG8esUrls(filePath) {
            const content = fs.readFileSync(filePath, 'utf-8');
            const urls = [];
            let match;
            while ((match = g8esUrlPattern.exec(content)) !== null) {
                urls.push({ url: match[0], port: match[1] || null, line: content.substring(0, match.index).split('\n').length });
            }
            return urls;
        }

        it('fetch-key-and-run.sh uses port 9000 for all g8es URLs', () => {
            if (!fs.existsSync(scriptPath)) {
                console.warn('[SKIP] fetch-key-and-run.sh not found in test environment');
                return;
            }
            const urls = extractG8esUrls(path.join(scriptsDir, 'fetch-key-and-run.sh'));
            expect(urls.length).toBeGreaterThan(0);
            for (const { url, port, line } of urls) {
                expect(port, `${url} at line ${line} must specify port 9000`).toBe('9000');
            }
        });

        it('fetch-key-and-run.sh does not pass --ca-url to the operator binary', () => {
            if (!fs.existsSync(scriptPath)) {
                console.warn('[SKIP] fetch-key-and-run.sh not found in test environment');
                return;
            }
            const content = fs.readFileSync(path.join(scriptsDir, 'fetch-key-and-run.sh'), 'utf-8');
            expect(content).not.toMatch(/--ca-url/);
        });

        it('fetch-key-and-run.sh downloads binary from blob store and tracks metadata', () => {
            if (!fs.existsSync(scriptPath)) {
                console.warn('[SKIP] fetch-key-and-run.sh not found in test environment');
                return;
            }
            const content = fs.readFileSync(path.join(scriptsDir, 'fetch-key-and-run.sh'), 'utf-8');
            expect(content).toMatch(/BLOB_URL="https:\/\/g8es:9000\/blob\/operator-binary"/);
            expect(content).toMatch(/OPERATOR_META="\/home\/g8e\/g8e.operator.meta"/);
            expect(content).toMatch(/_fetch_metadata/);
            expect(content).toMatch(/_fetch_binary/);
            expect(content).toMatch(/metadata changed/);
        });

        it('fetch-key-and-run.sh passes --working-dir /home/g8e to the operator binary', () => {
            if (!fs.existsSync(scriptPath)) {
                console.warn('[SKIP] fetch-key-and-run.sh not found in test environment');
                return;
            }
            const content = fs.readFileSync(path.join(scriptsDir, 'fetch-key-and-run.sh'), 'utf-8');
            expect(content).toMatch(/--working-dir \/home\/g8e/);
        });

        it('fetch-key-and-run.sh passes --no-git to the operator binary', () => {
            if (!fs.existsSync(scriptPath)) {
                console.warn('[SKIP] fetch-key-and-run.sh not found in test environment');
                return;
            }
            const content = fs.readFileSync(path.join(scriptsDir, 'fetch-key-and-run.sh'), 'utf-8');
            expect(content).toMatch(/--no-git/);
        });

        it('fetch-key-and-run.sh forwards G8E_LOG_LEVEL as --log flag', () => {
            if (!fs.existsSync(scriptPath)) {
                console.warn('[SKIP] fetch-key-and-run.sh not found in test environment');
                return;
            }
            const content = fs.readFileSync(path.join(scriptsDir, 'fetch-key-and-run.sh'), 'utf-8');
            expect(content).toMatch(/LOG_LEVEL="\$\{G8E_LOG_LEVEL:-info\}"/);
            expect(content).toMatch(/--log "\$LOG_LEVEL"/);
        });

        it('fetch-key-and-run.sh exports G8E_OPERATOR_PUBSUB_URL when set', () => {
            if (!fs.existsSync(scriptPath)) {
                console.warn('[SKIP] fetch-key-and-run.sh not found in test environment');
                return;
            }
            const content = fs.readFileSync(path.join(scriptsDir, 'fetch-key-and-run.sh'), 'utf-8');
            expect(content).toMatch(/PUBSUB_URL="\$\{G8E_OPERATOR_PUBSUB_URL:-\}"/);
            expect(content).toMatch(/export G8E_OPERATOR_PUBSUB_URL="\$PUBSUB_URL"/);
        });
    });
});
