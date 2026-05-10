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

// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { MockServiceClient } from '@test/mocks/mock-browser-env.js';
import { EventType } from '@g8ed/constants/events.js';
import fs from 'fs';
import path from 'path';

const CONSOLE_EJS_PATH = path.resolve(__dirname, '../../../../views/console.ejs');
const consoleEjsSource = fs.readFileSync(CONSOLE_EJS_PATH, 'utf8');

describe('Console Page [FRONTEND - jsdom]', () => {
    let serviceClient;
    let originalWindow;

    beforeEach(() => {
        serviceClient = new MockServiceClient();
        originalWindow = { ...window };
        window.serviceClient = serviceClient;
    });

    afterEach(() => {
        vi.restoreAllMocks();
    });

    describe('Static Analysis — console.ejs source', () => {
        it('should never use "new EventType(" — must use "new EventSource("', () => {
            const scriptBlocks = consoleEjsSource.match(/<script[\s\S]*?<\/script>/gi) || [];
            for (const block of scriptBlocks) {
                expect(block).not.toMatch(/new\s+EventType\s*\(/);
            }
        });

        it('should use EventSource for SSE connections', () => {
            const scriptBlocks = consoleEjsSource.match(/<script[\s\S]*?<\/script>/gi) || [];
            const hasEventSource = scriptBlocks.some(block => /new\s+EventSource\s*\(/.test(block));
            expect(hasEventSource).toBe(true);
        });

        it('should reference the correct wire protocol event type for log connected', () => {
            expect(consoleEjsSource).toContain(EventType.PLATFORM_CONSOLE_LOG_CONNECTED_CONFIRMED);
        });

        it('should reference the correct wire protocol event type for log entry', () => {
            expect(consoleEjsSource).toContain(EventType.PLATFORM_CONSOLE_LOG_ENTRY_RECEIVED);
        });

        it('should not use short-form event type checks like "connected" or "log"', () => {
            const scriptBlocks = consoleEjsSource.match(/<script[\s\S]*?<\/script>/gi) || [];
            for (const block of scriptBlocks) {
                expect(block).not.toMatch(/data\.type\s*===?\s*['"]connected['"]/);
                expect(block).not.toMatch(/data\.type\s*===?\s*['"]log['"]/);
            }
        });

        it('should not use polling (setInterval) for log data', () => {
            const scriptBlocks = consoleEjsSource.match(/<script[\s\S]*?<\/script>/gi) || [];
            for (const block of scriptBlocks) {
                const withoutComments = block.replace(/\/\/.*$/gm, '').replace(/\/\*[\s\S]*?\*\//g, '');
                expect(withoutComments).not.toMatch(/setInterval\s*\([^)]*log/i);
            }
        });
    });

    describe('apiFetch response restructuring', () => {
        function createApiFetch(sc) {
            return async function apiFetch(path, options = {}) {
                const method = (options.method || 'GET').toUpperCase();
                let res;
                if (method === 'POST') {
                    res = await sc.post('g8ed', path, options.body ? JSON.parse(options.body) : null);
                } else {
                    res = await sc.get('g8ed', path);
                }
                const json = await res.json();
                const { success, message, error, ...rest } = json;
                return { success, message, error, data: rest };
            };
        }

        it('should restructure flat backend response into { success, data } shape', async () => {
            serviceClient.setResponse('g8ed', '/api/console/overview', {
                ok: true,
                status: 200,
                json: async () => ({
                    success: true,
                    message: 'Overview fetched successfully',
                    timestamp: '2026-01-01T00:00:00.000Z',
                    users: { total: 10 },
                    operators: { total: 5 },
                    sessions: { total: 3 },
                    cache: { hitRate: 0.8 },
                    system: { overall: 'healthy' }
                })
            });

            const apiFetch = createApiFetch(serviceClient);
            const result = await apiFetch('/api/console/overview');

            expect(result.success).toBe(true);
            expect(result.message).toBe('Overview fetched successfully');
            expect(result.data).toBeDefined();
            expect(result.data.users).toEqual({ total: 10 });
            expect(result.data.operators).toEqual({ total: 5 });
            expect(result.data.sessions).toEqual({ total: 3 });
            expect(result.data.cache).toEqual({ hitRate: 0.8 });
            expect(result.data.system).toEqual({ overall: 'healthy' });
        });

        it('should not nest success/message/error inside data', async () => {
            serviceClient.setResponse('g8ed', '/api/console/overview', {
                ok: true,
                status: 200,
                json: async () => ({
                    success: true,
                    message: 'OK',
                    error: null,
                    users: { total: 1 }
                })
            });

            const apiFetch = createApiFetch(serviceClient);
            const result = await apiFetch('/api/console/overview');

            expect(result.data.success).toBeUndefined();
            expect(result.data.message).toBeUndefined();
            expect(result.data.error).toBeUndefined();
            expect(result.data.users).toEqual({ total: 1 });
        });

        it('should restructure KV scan response correctly', async () => {
            serviceClient.setResponse('g8ed', '/api/console/kv/scan?pattern=*&cursor=0&count=50', {
                ok: true,
                status: 200,
                json: async () => ({
                    success: true,
                    message: null,
                    pattern: '*',
                    cursor: '0',
                    keys: ['key1', 'key2'],
                    count: 2,
                    has_more: false
                })
            });

            const apiFetch = createApiFetch(serviceClient);
            const result = await apiFetch('/api/console/kv/scan?pattern=*&cursor=0&count=50');

            expect(result.success).toBe(true);
            expect(result.data.keys).toEqual(['key1', 'key2']);
            expect(result.data.cursor).toBe('0');
            expect(result.data.count).toBe(2);
            expect(result.data.has_more).toBe(false);
        });

        it('should restructure DB query response correctly', async () => {
            const docs = [{ id: 'doc1', name: 'test' }];
            serviceClient.setResponse('g8ed', '/api/console/db/query?collection=users&limit=50', {
                ok: true,
                status: 200,
                json: async () => ({
                    success: true,
                    message: null,
                    collection: 'users',
                    documents: docs,
                    count: 1,
                    limit: 50
                })
            });

            const apiFetch = createApiFetch(serviceClient);
            const result = await apiFetch('/api/console/db/query?collection=users&limit=50');

            expect(result.data.documents).toEqual(docs);
            expect(result.data.count).toBe(1);
        });

        it('should restructure DB collections response correctly', async () => {
            serviceClient.setResponse('g8ed', '/api/console/db/collections', {
                ok: true,
                status: 200,
                json: async () => ({
                    success: true,
                    message: null,
                    collections: ['users', 'operators', 'sessions']
                })
            });

            const apiFetch = createApiFetch(serviceClient);
            const result = await apiFetch('/api/console/db/collections');

            expect(result.data.collections).toEqual(['users', 'operators', 'sessions']);
        });

        it('should restructure component health response correctly', async () => {
            serviceClient.setResponse('g8ed', '/api/console/components/health', {
                ok: true,
                status: 200,
                json: async () => ({
                    success: true,
                    message: 'Component health fetched successfully',
                    overall: 'healthy',
                    timestamp: '2026-01-01T00:00:00.000Z',
                    components: { g8es: { status: 'healthy' } }
                })
            });

            const apiFetch = createApiFetch(serviceClient);
            const result = await apiFetch('/api/console/components/health');

            expect(result.data.overall).toBe('healthy');
            expect(result.data.timestamp).toBe('2026-01-01T00:00:00.000Z');
            expect(result.data.components).toEqual({ g8es: { status: 'healthy' } });
        });

        it('should restructure login audit response correctly', async () => {
            serviceClient.setResponse('g8ed', '/api/console/metrics/login-audit', {
                ok: true,
                status: 200,
                json: async () => ({
                    success: true,
                    message: 'Login audit stats fetched successfully',
                    total: 100,
                    successful: 80,
                    failed: 15,
                    locked: 3,
                    anomalies: 2,
                    byHour: {}
                })
            });

            const apiFetch = createApiFetch(serviceClient);
            const result = await apiFetch('/api/console/metrics/login-audit');

            expect(result.success).toBe(true);
            expect(result.data.total).toBe(100);
            expect(result.data.successful).toBe(80);
            expect(result.data.failed).toBe(15);
            expect(result.data.locked).toBe(3);
            expect(result.data.anomalies).toBe(2);
        });

        it('should restructure AI stats response correctly', async () => {
            serviceClient.setResponse('g8ed', '/api/console/metrics/ai', {
                ok: true,
                status: 200,
                json: async () => ({
                    success: true,
                    message: 'AI stats fetched successfully',
                    totalInvestigations: 200,
                    activeInvestigations: 20,
                    completedInvestigations: 180
                })
            });

            const apiFetch = createApiFetch(serviceClient);
            const result = await apiFetch('/api/console/metrics/ai');

            expect(result.data.totalInvestigations).toBe(200);
            expect(result.data.activeInvestigations).toBe(20);
        });

        it('should handle POST requests', async () => {
            serviceClient.setResponse('g8ed', '/api/console/cache/clear', {
                ok: true,
                status: 200,
                json: async () => ({
                    success: true,
                    message: 'Metrics cache cleared'
                })
            });

            const apiFetch = createApiFetch(serviceClient);
            const result = await apiFetch('/api/console/cache/clear', { method: 'POST' });

            expect(result.success).toBe(true);
            expect(result.message).toBe('Metrics cache cleared');
            const postCalls = serviceClient.getRequestLog().filter(r => r.method === 'POST');
            expect(postCalls.length).toBe(1);
        });
    });

    describe('SSE event type wire protocol contract', () => {
        it('should define PLATFORM_CONSOLE_LOG_ENTRY_RECEIVED with correct wire value', () => {
            expect(EventType.PLATFORM_CONSOLE_LOG_ENTRY_RECEIVED).toBe('g8e.v1.platform.console.log.entry.received');
        });

        it('should define PLATFORM_CONSOLE_LOG_CONNECTED_CONFIRMED with correct wire value', () => {
            expect(EventType.PLATFORM_CONSOLE_LOG_CONNECTED_CONFIRMED).toBe('g8e.v1.platform.console.log.connected.confirmed');
        });

        it('should match the event types used in console.ejs connectLogs', () => {
            const connectedType = EventType.PLATFORM_CONSOLE_LOG_CONNECTED_CONFIRMED;
            const entryType = EventType.PLATFORM_CONSOLE_LOG_ENTRY_RECEIVED;

            expect(consoleEjsSource).toContain(`data.type === '${connectedType}'`);
            expect(consoleEjsSource).toContain(`data.type === '${entryType}'`);
        });
    });

    describe('SSE connectLogs behavior', () => {
        let mockEventSource;
        let eventSourceInstances;

        beforeEach(() => {
            eventSourceInstances = [];
            mockEventSource = vi.fn().mockImplementation(function(url, options) {
                this.url = url;
                this.withCredentials = options?.withCredentials ?? false;
                this.readyState = 0;
                this.onmessage = null;
                this.onerror = null;
                this.close = vi.fn();
                eventSourceInstances.push(this);
            });
            window.EventSource = mockEventSource;

            document.body.innerHTML = `
                <select id="logs-level"><option value="info" selected>Info</option></select>
                <span id="logs-status-badge"></span>
                <div id="logs-container"></div>
            `;
        });

        function getConnectLogs() {
            let logsEventSource = null;
            let logsCount = 0;
            let logsPaused = false;

            function esc(str) { return String(str ?? ''); }

            function appendLogEntry(entry) {
                if (logsPaused) return;
                const container = document.getElementById('logs-container');
                const row = document.createElement('div');
                row.className = 'console-log-row';
                row.textContent = entry.message || '';
                container.appendChild(row);
                logsCount++;
            }

            function connectLogs() {
                const level = document.getElementById('logs-level').value;
                const badge = document.getElementById('logs-status-badge');

                if (logsEventSource) {
                    logsEventSource.close();
                    logsEventSource = null;
                }

                badge.textContent = 'Connecting...';
                badge.className = 'console-realtime-badge';

                const container = document.getElementById('logs-container');
                container.innerHTML = '';
                logsCount = 0;

                const es = new EventSource(`/api/console/logs/stream?level=${encodeURIComponent(level)}&limit=200`, { withCredentials: true });
                logsEventSource = es;

                es.onmessage = (event) => {
                    try {
                        const data = JSON.parse(event.data);
                        if (data.type === 'g8e.v1.platform.console.log.connected.confirmed') {
                            badge.textContent = 'Live';
                            badge.className = 'console-realtime-badge';
                        } else if (data.type === 'g8e.v1.platform.console.log.entry.received') {
                            appendLogEntry(data.entry);
                        }
                    } catch (_) {}
                };

                es.onerror = () => {
                    badge.textContent = 'Disconnected';
                    badge.className = 'console-realtime-badge badge-unhealthy';
                    es.close();
                    logsEventSource = null;
                };
            }

            return { connectLogs, getEventSource: () => logsEventSource, getLogsCount: () => logsCount };
        }

        it('should create EventSource with correct URL and withCredentials', () => {
            const { connectLogs } = getConnectLogs();
            connectLogs();

            expect(mockEventSource).toHaveBeenCalledTimes(1);
            const es = eventSourceInstances[0];
            expect(es.url).toBe('/api/console/logs/stream?level=info&limit=200');
            expect(es.withCredentials).toBe(true);
        });

        it('should set badge to "Connecting..." on start', () => {
            const { connectLogs } = getConnectLogs();
            connectLogs();

            const badge = document.getElementById('logs-status-badge');
            expect(badge.textContent).toBe('Connecting...');
        });

        it('should set badge to "Live" on connected confirmed event', () => {
            const { connectLogs } = getConnectLogs();
            connectLogs();

            const es = eventSourceInstances[0];
            es.onmessage({
                data: JSON.stringify({
                    type: EventType.PLATFORM_CONSOLE_LOG_CONNECTED_CONFIRMED,
                    timestamp: new Date().toISOString(),
                    buffered: 0
                })
            });

            const badge = document.getElementById('logs-status-badge');
            expect(badge.textContent).toBe('Live');
        });

        it('should append log entry on log entry received event', () => {
            const { connectLogs } = getConnectLogs();
            connectLogs();

            const es = eventSourceInstances[0];
            es.onmessage({
                data: JSON.stringify({
                    type: EventType.PLATFORM_CONSOLE_LOG_ENTRY_RECEIVED,
                    entry: { level: 'info', message: 'test log message', timestamp: new Date().toISOString() }
                })
            });

            const container = document.getElementById('logs-container');
            expect(container.children.length).toBe(1);
            expect(container.children[0].textContent).toBe('test log message');
        });

        it('should ignore events with unknown type', () => {
            const { connectLogs } = getConnectLogs();
            connectLogs();

            const es = eventSourceInstances[0];
            es.onmessage({
                data: JSON.stringify({ type: 'unknown.event.type', entry: { message: 'ignored' } })
            });

            const container = document.getElementById('logs-container');
            expect(container.children.length).toBe(0);
            const badge = document.getElementById('logs-status-badge');
            expect(badge.textContent).toBe('Connecting...');
        });

        it('should set badge to "Disconnected" on error', () => {
            const { connectLogs } = getConnectLogs();
            connectLogs();

            const es = eventSourceInstances[0];
            es.onerror();

            const badge = document.getElementById('logs-status-badge');
            expect(badge.textContent).toBe('Disconnected');
            expect(badge.className).toContain('badge-unhealthy');
            expect(es.close).toHaveBeenCalled();
        });

        it('should close previous EventSource when reconnecting', () => {
            const { connectLogs } = getConnectLogs();
            connectLogs();
            const firstEs = eventSourceInstances[0];

            connectLogs();
            expect(firstEs.close).toHaveBeenCalled();
            expect(eventSourceInstances.length).toBe(2);
        });

        it('should clear logs container on reconnect', () => {
            const { connectLogs } = getConnectLogs();
            connectLogs();

            const es = eventSourceInstances[0];
            es.onmessage({
                data: JSON.stringify({
                    type: EventType.PLATFORM_CONSOLE_LOG_ENTRY_RECEIVED,
                    entry: { level: 'info', message: 'old entry' }
                })
            });

            expect(document.getElementById('logs-container').children.length).toBe(1);

            connectLogs();
            expect(document.getElementById('logs-container').children.length).toBe(0);
        });

        it('should use the selected level in the EventSource URL', () => {
            const select = document.getElementById('logs-level');
            const errorOpt = document.createElement('option');
            errorOpt.value = 'error';
            errorOpt.textContent = 'Error';
            select.appendChild(errorOpt);
            select.value = 'error';

            const { connectLogs } = getConnectLogs();
            connectLogs();

            const es = eventSourceInstances[0];
            expect(es.url).toBe('/api/console/logs/stream?level=error&limit=200');
        });

        it('should handle multiple log entries in sequence', () => {
            const { connectLogs, getLogsCount } = getConnectLogs();
            connectLogs();

            const es = eventSourceInstances[0];
            for (let i = 0; i < 5; i++) {
                es.onmessage({
                    data: JSON.stringify({
                        type: EventType.PLATFORM_CONSOLE_LOG_ENTRY_RECEIVED,
                        entry: { level: 'info', message: `msg ${i}` }
                    })
                });
            }

            const container = document.getElementById('logs-container');
            expect(container.children.length).toBe(5);
            expect(getLogsCount()).toBe(5);
        });

        it('should handle malformed JSON gracefully without throwing', () => {
            const { connectLogs } = getConnectLogs();
            connectLogs();

            const es = eventSourceInstances[0];
            expect(() => {
                es.onmessage({ data: 'not-valid-json{{{' });
            }).not.toThrow();

            const container = document.getElementById('logs-container');
            expect(container.children.length).toBe(0);
        });
    });

    describe('Backend/Frontend response shape contract', () => {
        it('should verify overview response shape is consumed via data property', () => {
            const overviewFn = consoleEjsSource.match(/async function loadOverview\(\)[\s\S]*?(?=\n {8}async function|\n {8}function)/);
            expect(overviewFn).not.toBeNull();
            const src = overviewFn[0];
            expect(src).toContain('json.data');
            expect(src).not.toMatch(/json\.users(?!\s*\?)/);
            expect(src).not.toMatch(/json\.operators(?!\s*\?)/);
        });

        it('should verify component health response is consumed via data property', () => {
            const healthFn = consoleEjsSource.match(/async function loadComponentHealth\(\)[\s\S]*?(?=\n {8}let kvCursor)/);
            expect(healthFn).not.toBeNull();
            const src = healthFn[0];
            expect(src).toContain('json.data');
        });

        it('should verify KV scan response is consumed via data property', () => {
            const kvFn = consoleEjsSource.match(/async function kvScan\([\s\S]*?(?=\n {8}async function kvViewKey)/);
            expect(kvFn).not.toBeNull();
            const src = kvFn[0];
            expect(src).toContain('json.data');
        });

        it('should verify DB query response is consumed via data property', () => {
            const dbFn = consoleEjsSource.match(/async function dbQuery\(\)[\s\S]*?(?=\n {8}function dbViewDocument)/);
            expect(dbFn).not.toBeNull();
            const src = dbFn[0];
            expect(src).toContain('json.data');
        });

        it('should verify populateCollections response is consumed via data.collections', () => {
            const colFn = consoleEjsSource.match(/async function populateCollections\(\)[\s\S]*?(?=\n {8}async function dbQuery)/);
            expect(colFn).not.toBeNull();
            const src = colFn[0];
            expect(src).toContain('json.data.collections');
        });
    });
});
