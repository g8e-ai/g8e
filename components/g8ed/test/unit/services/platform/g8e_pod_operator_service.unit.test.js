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
import { G8ENodeOperatorService } from '@g8ed/services/platform/g8e_pod_operator_service.js';
import { OperatorStatus } from '@g8ed/constants/operator.js';

global.fetch = vi.fn();

function makeSettingsService(overrides = {}) {
    return {
        getPlatformSettings: vi.fn().mockResolvedValue({
            supervisor_port: '443',
            internal_auth_token: 'test-token',
            ...overrides,
        }),
        savePlatformSettings: vi.fn().mockResolvedValue({ success: true, saved: ['g8e_pod_operator_api_key'] }),
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
        service = new G8ENodeOperatorService({ settingsService, operatorService });
    });

    describe('constructor', () => {
        it('throws if settingsService is missing', () => {
            expect(() => new G8ENodeOperatorService({ operatorService }))
                .toThrow('G8ENodeOperatorService requires settingsService');
        });
    });

    describe('getG8ENodeOperatorForUser', () => {
        it('returns operator and active status', async () => {
            const mockOp = { operator_id: 'op_123', status: OperatorStatus.ACTIVE };
            operatorService.queryOperators.mockResolvedValue([mockOp]);

            const result = await service.getG8ENodeOperatorForUser('user_123');

            expect(result.operator.operator_id).toBe('op_123');
            expect(result.alreadyActive).toBe(true);
            expect(operatorService.queryOperators).toHaveBeenCalledWith(
                expect.arrayContaining([
                    { field: 'user_id', operator: '==', value: 'user_123' },
                    { field: 'is_g8e_pod', operator: '==', value: true },
                ])
            );
        });

        it('returns null if no slot found', async () => {
            operatorService.queryOperators.mockResolvedValue([]);
            const result = await service.getG8ENodeOperatorForUser('user_123');
            expect(result).toBeNull();
        });
    });

    describe('launchG8ENodeOperator', () => {
        it('persists API key to platform_settings then starts supervisor program via XML-RPC', async () => {
            mockFetchOk();

            await service.launchG8ENodeOperator('sk-test-key');

            expect(settingsService.savePlatformSettings).toHaveBeenCalledWith({
                g8e_pod_operator_api_key: 'sk-test-key',
            });
            expect(fetch).toHaveBeenCalledWith(
                expect.stringContaining(':443/RPC2'),
                expect.objectContaining({
                    method: 'POST',
                    body: expect.stringContaining('supervisor.startProcess'),
                })
            );
        });

        it('uses supervisor_port from platform settings', async () => {
            settingsService.getPlatformSettings.mockResolvedValue({
                supervisor_port: '9999',
                internal_auth_token: 'tok',
            });
            mockFetchOk();

            await service.launchG8ENodeOperator('sk-test-key');

            expect(fetch).toHaveBeenCalledWith(
                'http://g8ep:9999/RPC2',
                expect.any(Object)
            );
        });

        it('restarts if start fails (already running)', async () => {
            fetch
                .mockResolvedValueOnce({ ok: true, text: () => Promise.resolve(XML_ALREADY_STARTED) })
                .mockResolvedValueOnce({ ok: true, text: () => Promise.resolve(XML_OK) })
                .mockResolvedValueOnce({ ok: true, text: () => Promise.resolve(XML_OK) });

            await service.launchG8ENodeOperator('sk-test-key');

            expect(settingsService.savePlatformSettings).toHaveBeenCalledOnce();
            expect(fetch).toHaveBeenCalledWith(
                expect.any(String),
                expect.objectContaining({ body: expect.stringContaining('supervisor.stopProcess') })
            );
            expect(fetch).toHaveBeenCalledWith(
                expect.any(String),
                expect.objectContaining({ body: expect.stringContaining('supervisor.startProcess') })
            );
        });

        it('throws a specific error when savePlatformSettings fails', async () => {
            settingsService.savePlatformSettings.mockRejectedValue(new Error('g8es write timeout'));

            await expect(service.launchG8ENodeOperator('sk-test-key'))
                .rejects.toThrow('Failed to persist operator API key to platform_settings: g8es write timeout');

            expect(fetch).not.toHaveBeenCalled();
        });

        it('reads platform settings once per launch even on restart path', async () => {
            fetch
                .mockResolvedValueOnce({ ok: true, text: () => Promise.resolve(XML_ALREADY_STARTED) })
                .mockResolvedValueOnce({ ok: true, text: () => Promise.resolve(XML_OK) })
                .mockResolvedValueOnce({ ok: true, text: () => Promise.resolve(XML_OK) });

            await service.launchG8ENodeOperator('sk-test-key');

            expect(settingsService.getPlatformSettings).toHaveBeenCalledTimes(1);
        });
    });

    describe('relaunchG8ENodeOperatorForUser', () => {
        it('stops, resets, persists new key, and launches', async () => {
            const mockOp = { operator_id: 'op_123', status: OperatorStatus.ACTIVE };
            operatorService.queryOperators.mockResolvedValue([mockOp]);
            operatorService.resetOperator.mockResolvedValue({ success: true, operator: { api_key: 'new-key' } });
            mockFetchOk();

            const result = await service.relaunchG8ENodeOperatorForUser('user_123');

            expect(result.success).toBe(true);
            expect(fetch).toHaveBeenCalledWith(
                expect.any(String),
                expect.objectContaining({ body: expect.stringContaining('supervisor.stopProcess') })
            );
            expect(operatorService.resetOperator).toHaveBeenCalledWith('op_123');
            expect(settingsService.savePlatformSettings).toHaveBeenCalledWith({
                g8e_pod_operator_api_key: 'new-key',
            });
            expect(fetch).toHaveBeenCalledWith(
                expect.any(String),
                expect.objectContaining({ body: expect.stringContaining('supervisor.startProcess') })
            );
        });

        it('returns failure when no g8ep slot found', async () => {
            operatorService.queryOperators.mockResolvedValue([]);

            const result = await service.relaunchG8ENodeOperatorForUser('user_123');

            expect(result.success).toBe(false);
            expect(result.error).toMatch(/no g8ep operator slot/i);
        });

        it('returns failure when reset has no API key', async () => {
            const mockOp = { operator_id: 'op_123', status: OperatorStatus.ACTIVE };
            operatorService.queryOperators.mockResolvedValue([mockOp]);
            operatorService.resetOperator.mockResolvedValue({ success: true, operator: {} });
            mockFetchOk();

            const result = await service.relaunchG8ENodeOperatorForUser('user_123');

            expect(result.success).toBe(false);
            expect(result.error).toMatch(/no api key/i);
        });
    });

    describe('activateG8ENodeOperatorForUser', () => {
        it('skips launch when operator is already active', async () => {
            const mockOp = { operator_id: 'op_123', status: OperatorStatus.ACTIVE };
            operatorService.queryOperators.mockResolvedValue([mockOp]);

            await service.activateG8ENodeOperatorForUser('user_123', null, 'sess_1');

            expect(settingsService.savePlatformSettings).not.toHaveBeenCalled();
            expect(fetch).not.toHaveBeenCalled();
        });

        it('launches when operator slot is available with API key', async () => {
            const mockOp = { operator_id: 'op_123', status: 'AVAILABLE', api_key: 'sk-key' };
            operatorService.queryOperators.mockResolvedValue([mockOp]);
            mockFetchOk();

            await service.activateG8ENodeOperatorForUser('user_123', null, 'sess_1');

            expect(settingsService.savePlatformSettings).toHaveBeenCalledWith({
                g8e_pod_operator_api_key: 'sk-key',
            });
            expect(fetch).toHaveBeenCalled();
        });

        it('does not throw when activation fails', async () => {
            operatorService.queryOperators.mockRejectedValue(new Error('DB down'));

            await expect(
                service.activateG8ENodeOperatorForUser('user_123', null, 'sess_1')
            ).resolves.toBeUndefined();
        });
    });

    describe('g8ep script validation', () => {
        const scriptsDir = path.resolve('/app/components/g8ep/scripts');
        const g8esUrlPattern = /https:\/\/g8es(?::(\d+))?/g;

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
            const urls = extractG8esUrls(path.join(scriptsDir, 'fetch-key-and-run.sh'));
            expect(urls.length).toBeGreaterThan(0);
            for (const { url, port, line } of urls) {
                expect(port, `${url} at line ${line} must specify port 9000`).toBe('9000');
            }
        });

        it('fetch-key-and-run.sh does not pass --ca-url to the operator binary', () => {
            const content = fs.readFileSync(path.join(scriptsDir, 'fetch-key-and-run.sh'), 'utf-8');
            expect(content).not.toMatch(/--ca-url/);
        });

        it('fetch-key-and-run.sh downloads binary from blob store when not present locally', () => {
            const content = fs.readFileSync(path.join(scriptsDir, 'fetch-key-and-run.sh'), 'utf-8');
            expect(content).toMatch(/BLOB_URL="https:\/\/g8es:9000\/blob\/operator-binary"/);
            expect(content).toMatch(/_fetch_binary/);
            expect(content).toMatch(/if \[ ! -x "\$\{OPERATOR_BINARY\}" \]/);
        });

        it('fetch-key-and-run.sh passes --working-dir /home/g8e to the operator binary', () => {
            const content = fs.readFileSync(path.join(scriptsDir, 'fetch-key-and-run.sh'), 'utf-8');
            expect(content).toMatch(/--working-dir \/home\/g8e/);
        });

        it('fetch-key-and-run.sh passes --no-git to the operator binary', () => {
            const content = fs.readFileSync(path.join(scriptsDir, 'fetch-key-and-run.sh'), 'utf-8');
            expect(content).toMatch(/--no-git/);
        });

        it('fetch-key-and-run.sh forwards G8E_LOG_LEVEL as --log flag', () => {
            const content = fs.readFileSync(path.join(scriptsDir, 'fetch-key-and-run.sh'), 'utf-8');
            expect(content).toMatch(/LOG_LEVEL="\$\{G8E_LOG_LEVEL:-info\}"/);
            expect(content).toMatch(/--log "\$LOG_LEVEL"/);
        });

        it('fetch-key-and-run.sh exports G8E_OPERATOR_PUBSUB_URL when set', () => {
            const content = fs.readFileSync(path.join(scriptsDir, 'fetch-key-and-run.sh'), 'utf-8');
            expect(content).toMatch(/PUBSUB_URL="\$\{G8E_OPERATOR_PUBSUB_URL:-\}"/);
            expect(content).toMatch(/export G8E_OPERATOR_PUBSUB_URL="\$PUBSUB_URL"/);
        });
    });
});
