// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * Tier 1 unit tests for the g8eg client mTLS constructor wiring.
 *
 * Verifies the forward-compatible mTLS configuration path added by the
 * dashboard app enrollment plan:
 *
 * - `g8egHttpClient` builds an undici `Agent` dispatcher with
 *   `connect: { cert, key, ca }` when `clientCertPath`/`clientKeyPath` are
 *   provided and present on disk, and passes it as `dispatcher` on each
 *   `fetch` call. Falls back to no dispatcher when the paths are missing
 *   or absent on disk.
 * - `g8egPubSubClient._buildTLSOptions()` returns `{ cert, key, ca }` when
 *   all three paths are present on disk, `{ cert, key }` when only the
 *   client pair is present, `{ ca }` when only the CA is present, and `{}`
 *   when nothing is configured.
 *
 * No network. Filesystem fixtures are written to a temp directory.
 */

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { promises as fs } from 'node:fs';
import path from 'node:path';
import os from 'node:os';

vi.mock('../../../../utils/logger.js', () => ({
    logger: {
        info: () => {},
        warn: () => {},
        error: () => {},
        debug: () => {},
    },
}));

import { g8egHttpClient } from '../../../../services/clients/g8eg_http_client.js';
import { g8egPubSubClient } from '../../../../services/clients/g8eg_pubsub_client.js';

let _tmpDir;

async function _writeCert(name, content = '-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----\n') {
    const p = path.join(_tmpDir, name);
    await fs.writeFile(p, content, 'utf8');
    return p;
}

beforeEach(async () => {
    _tmpDir = await fs.mkdtemp(path.join(os.tmpdir(), 'g8ed-mtls-'));
});

afterEach(async () => {
    if (_tmpDir) {
        await fs.rm(_tmpDir, { recursive: true, force: true });
    }
});

describe('g8egHttpClient mTLS dispatcher wiring', () => {
    it('constructs an undici Agent dispatcher when cert/key paths exist on disk', async () => {
        const certPath = await _writeCert('client.crt');
        const keyPath = await _writeCert('client.key', '-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----\n');
        const caPath = await _writeCert('ca.pem');

        const client = new g8egHttpClient({
            listenUrl: 'https://g8eg:8443',
            clientCertPath: certPath,
            clientKeyPath: keyPath,
            caCertPath: caPath,
        });

        expect(client._dispatcher).not.toBeNull();
        // undici Agent instances expose a `connect` property; assert the
        // dispatcher is an Agent-shaped object rather than null.
        expect(typeof client._dispatcher).toBe('object');
    });

    it('falls back to null dispatcher when cert/key paths are not provided', () => {
        const client = new g8egHttpClient({
            listenUrl: 'https://g8eg:8443',
        });

        expect(client._dispatcher).toBeNull();
    });

    it('falls back to null dispatcher when the cert file is missing on disk', async () => {
        const keyPath = await _writeCert('client.key');
        const client = new g8egHttpClient({
            listenUrl: 'https://g8eg:8443',
            clientCertPath: path.join(_tmpDir, 'does-not-exist.crt'),
            clientKeyPath: keyPath,
        });

        expect(client._dispatcher).toBeNull();
    });

    it('falls back to null dispatcher when the key file is missing on disk', async () => {
        const certPath = await _writeCert('client.crt');
        const client = new g8egHttpClient({
            listenUrl: 'https://g8eg:8443',
            clientCertPath: certPath,
            clientKeyPath: path.join(_tmpDir, 'does-not-exist.key'),
        });

        expect(client._dispatcher).toBeNull();
    });

    it('constructs a dispatcher with only cert/key when caCertPath is unset', async () => {
        const certPath = await _writeCert('client.crt');
        const keyPath = await _writeCert('client.key', '-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----\n');

        const client = new g8egHttpClient({
            listenUrl: 'https://g8eg:8443',
            clientCertPath: certPath,
            clientKeyPath: keyPath,
        });

        expect(client._dispatcher).not.toBeNull();
    });

    it('passes the dispatcher as the fetch dispatcher option on request', async () => {
        const certPath = await _writeCert('client.crt');
        const keyPath = await _writeCert('client.key', '-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----\n');

        const client = new g8egHttpClient({
            listenUrl: 'https://g8eg:8443',
            clientCertPath: certPath,
            clientKeyPath: keyPath,
        });
        const dispatcher = client._dispatcher;

        // Replace global fetch with a spy that records the options it was
        // called with, then invoke the client. The dispatcher must be
        // passed through.
        const originalFetch = global.fetch;
        const calls = [];
        global.fetch = (url, opts) => {
            calls.push({ url, opts });
            return Promise.resolve({
                ok: true,
                status: 200,
                text: () => Promise.resolve('{"ok":true}'),
            });
        };
        try {
            await client.get('/ping');
        } finally {
            global.fetch = originalFetch;
        }

        expect(calls).toHaveLength(1);
        expect(calls[0].opts.dispatcher).toBe(dispatcher);
    });

    it('does not pass a dispatcher option when no mTLS config is provided', async () => {
        const client = new g8egHttpClient({ listenUrl: 'https://g8eg:8443' });

        const originalFetch = global.fetch;
        const calls = [];
        global.fetch = (url, opts) => {
            calls.push({ url, opts });
            return Promise.resolve({
                ok: true,
                status: 200,
                text: () => Promise.resolve('{"ok":true}'),
            });
        };
        try {
            await client.get('/ping');
        } finally {
            global.fetch = originalFetch;
        }

        expect(calls).toHaveLength(1);
        expect(calls[0].opts.dispatcher).toBeUndefined();
    });
});

describe('g8egPubSubClient _buildTLSOptions mTLS wiring', () => {
    it('returns cert, key, and ca when all three paths exist on disk', async () => {
        const certPath = await _writeCert('client.crt');
        const keyPath = await _writeCert('client.key', '-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----\n');
        const caPath = await _writeCert('ca.pem');

        const client = new g8egPubSubClient({
            pubsubUrl: 'wss://g8eg:8443',
            clientCertPath: certPath,
            clientKeyPath: keyPath,
            caCertPath: caPath,
        });

        const opts = client._buildTLSOptions();
        expect(opts.cert).toBeDefined();
        expect(opts.key).toBeDefined();
        expect(opts.ca).toBeDefined();
    });

    it('returns cert and key without ca when only the client pair is configured', async () => {
        const certPath = await _writeCert('client.crt');
        const keyPath = await _writeCert('client.key', '-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----\n');

        const client = new g8egPubSubClient({
            pubsubUrl: 'wss://g8eg:8443',
            clientCertPath: certPath,
            clientKeyPath: keyPath,
        });

        const opts = client._buildTLSOptions();
        expect(opts.cert).toBeDefined();
        expect(opts.key).toBeDefined();
        expect(opts.ca).toBeUndefined();
    });

    it('returns only ca when only caCertPath is configured (legacy path)', async () => {
        const caPath = await _writeCert('ca.pem');

        const client = new g8egPubSubClient({
            pubsubUrl: 'wss://g8eg:8443',
            caCertPath: caPath,
        });

        const opts = client._buildTLSOptions();
        expect(opts.ca).toBeDefined();
        expect(opts.cert).toBeUndefined();
        expect(opts.key).toBeUndefined();
    });

    it('returns an empty object when no TLS paths are configured', () => {
        const client = new g8egPubSubClient({
            pubsubUrl: 'wss://g8eg:8443',
        });

        expect(client._buildTLSOptions()).toEqual({});
    });

    it('omits cert/key when the cert file is missing on disk but ca is present', async () => {
        const keyPath = await _writeCert('client.key', '-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----\n');
        const caPath = await _writeCert('ca.pem');

        const client = new g8egPubSubClient({
            pubsubUrl: 'wss://g8eg:8443',
            clientCertPath: path.join(_tmpDir, 'missing.crt'),
            clientKeyPath: keyPath,
            caCertPath: caPath,
        });

        const opts = client._buildTLSOptions();
        expect(opts.cert).toBeUndefined();
        expect(opts.key).toBeUndefined();
        expect(opts.ca).toBeDefined();
    });

    it('forwards clientCertPath and clientKeyPath through duplicate()', async () => {
        const certPath = await _writeCert('client.crt');
        const keyPath = await _writeCert('client.key', '-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----\n');
        const caPath = await _writeCert('ca.pem');

        const client = new g8egPubSubClient({
            pubsubUrl: 'wss://g8eg:8443',
            clientCertPath: certPath,
            clientKeyPath: keyPath,
            caCertPath: caPath,
        });
        const dup = client.duplicate();

        expect(dup.clientCertPath).toBe(certPath);
        expect(dup.clientKeyPath).toBe(keyPath);
        expect(dup.caCertPath).toBe(caPath);
    });
});
