// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * Tier 1 unit tests for AppEnrollmentService.
 *
 * Mirrors `ensemble/tests/unit/services/infra/app_enrollment_service_test.py`.
 * Covers the two explicit operations:
 *
 * - `loadIdentity()` — read path: loads an existing cert/key pair from disk,
 *   validates expiry, extracts the SPIFFE app_id from the URI SAN. Throws
 *   `ConfigurationError` if missing, expired, near-expiry, or malformed.
 *
 * - `enroll()` — write path: generates a CSR, fetches the CA bundle, POSTs the
 *   enrollment request to the gateway, writes credentials to disk, and returns
 *   an `AppIdentity`. Always contacts the gateway.
 *
 * HTTP traffic is intercepted via `vi.spyOn(global, 'fetch')`. Filesystem
 * state is isolated to a temp directory under `os.tmpdir()`.
 */

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { promises as fs } from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import { execSync } from 'node:child_process';
import { X509Certificate as NodeX509Cert } from 'node:crypto';
import { AppEnrollmentService, ConfigurationError } from '../../../../services/infra/app-enrollment-service.js';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

let _tmpDir;
let _runtimeDir;
let _origEnv;

async function _isolateRuntimeDir() {
    _tmpDir = await fs.mkdtemp(path.join(os.tmpdir(), 'g8ed-enroll-'));
    _runtimeDir = path.join(_tmpDir, 'runtime');
    await fs.mkdir(_runtimeDir, { recursive: true });
    process.env.G8E_RUNTIME_DIR = _runtimeDir;
    return _runtimeDir;
}

async function _cleanupRuntimeDir() {
    if (_tmpDir) {
        await fs.rm(_tmpDir, { recursive: true, force: true });
    }
}

/**
 * Generate a self-signed ECDSA P-256 cert + key PEM pair using openssl.
 *
 * The cert is not a real gateway-signed app cert, but AppEnrollmentService
 * only parses the not-after timestamp and the SPIFFE URI SAN — it does not
 * verify the issuer chain on the reuse path.
 *
 * @param {Object} opts
 * @param {number} opts.days - Validity in days from now.
 * @param {boolean} [opts.includeSan=true] - Include a SPIFFE URI SAN.
 * @param {string} [opts.appName='g8ed'] - Subject CN and SPIFFE app name.
 * @returns {Promise<{certPem: string, keyPem: string}>}
 */
async function _selfSignedCert({ days, includeSan = true, appName = 'g8ed' }) {
    const certPath = path.join(_tmpDir, `${appName}-${days}-${includeSan}.crt`);
    const keyPath = path.join(_tmpDir, `${appName}-${days}-${includeSan}.key`);
    const args = [
        'openssl', 'req', '-x509',
        '-newkey', 'ec', '-pkeyopt', 'ec_paramgen_curve:P-256',
        '-keyout', keyPath,
        '-out', certPath,
        '-days', String(days),
        '-nodes',
        '-subj', `/CN=${appName}`,
    ];
    if (includeSan) {
        args.push('-addext', `subjectAltName=URI:spiffe://g8e.local/app/${appName}`);
    }
    execSync(args.join(' '), { stdio: 'pipe' });
    const certPem = await fs.readFile(certPath, 'utf8');
    const keyPem = await fs.readFile(keyPath, 'utf8');
    return { certPem, keyPem };
}

/**
 * Write a pre-existing app cert/key pair into the isolated runtime tree.
 *
 * @param {string} certPem
 * @param {string} keyPem
 * @param {string} [appName='g8ed']
 * @returns {Promise<{certPath: string, keyPath: string}>}
 */
async function _writeExistingIdentity(certPem, keyPem, appName = 'g8ed') {
    const certDir = path.join(_runtimeDir, 'pki', 'issued', 'apps');
    await fs.mkdir(certDir, { recursive: true });
    const certPath = path.join(certDir, `${appName}.crt`);
    const keyPath = path.join(certDir, `${appName}.key`);
    await fs.writeFile(certPath, certPem);
    await fs.writeFile(keyPath, keyPem);
    return { certPath, keyPath };
}

/**
 * Build a mock fetch that responds to CA bundle and enrollment endpoints.
 *
 * @param {Object} opts
 * @param {string} [opts.caBundle='CA-BUNDLE-PEM']
 * @param {Object} [opts.enrollmentResponse]
 * @param {number} [opts.enrollmentStatus=201]
 * @param {number} [opts.caStatus=200]
 * @param {Error} [opts.enrollError] - Throw from the enrollment endpoint.
 * @returns {vi.Spy} The fetch spy.
 */
function _mockFetch({
    caBundle = 'CA-BUNDLE-PEM',
    enrollmentResponse = {
        success: true,
        app_id: 'spiffe://g8e.local/app/g8ed',
        app_cert: 'FAKE-APP-CERT-PEM',
        cert_chain: 'FAKE-CHAIN-PEM',
        trust_bundle: 'FAKE-RESPONSE-BUNDLE',
        expires_at: '2099-01-01T00:00:00Z',
    },
    enrollmentStatus = 201,
    caStatus = 200,
    enrollError = null,
} = {}) {
    const spy = vi.spyOn(global, 'fetch').mockImplementation(async (url, opts) => {
        const urlStr = String(url);
        if (urlStr.endsWith('/.well-known/g8e/pki/ca-bundle')) {
            if (caStatus !== 200) {
                return new Response('unavailable', { status: caStatus });
            }
            return new Response(caBundle, { status: 200 });
        }
        if (urlStr.endsWith('/api/v1/pki/apps/enroll')) {
            if (enrollError) throw enrollError;
            return new Response(JSON.stringify(enrollmentResponse), {
                status: enrollmentStatus,
                headers: { 'Content-Type': 'application/json' },
            });
        }
        return new Response('not found', { status: 404 });
    });
    return spy;
}

// ---------------------------------------------------------------------------
// Tests: loadIdentity (read path)
// ---------------------------------------------------------------------------

describe('AppEnrollmentService.loadIdentity', () => {
    beforeEach(async () => {
        _origEnv = { ...process.env };
        await _isolateRuntimeDir();
    });
    afterEach(async () => {
        vi.restoreAllMocks();
        for (const k of Object.keys(process.env)) {
            if (!(k in _origEnv)) delete process.env[k];
        }
        Object.assign(process.env, _origEnv);
        await _cleanupRuntimeDir();
    });

    it('loads an existing valid cert and extracts the SPIFFE app_id', async () => {
        const { certPem, keyPem } = await _selfSignedCert({ days: 90 });
        const { certPath, keyPath } = await _writeExistingIdentity(certPem, keyPem);

        const service = new AppEnrollmentService();
        const identity = await service.loadIdentity();

        expect(identity.cert_path).toBe(certPath);
        expect(identity.key_path).toBe(keyPath);
        expect(identity.ca_cert_path).toBe(path.join(_runtimeDir, 'pki', 'trust', 'hub-bundle.pem'));
        expect(identity.app_id).toBe('spiffe://g8e.local/app/g8ed');
    });

    it('raises ConfigurationError when the cert file is missing', async () => {
        const service = new AppEnrollmentService();
        await expect(service.loadIdentity()).rejects.toThrow(ConfigurationError);
        await expect(service.loadIdentity()).rejects.toThrow('app cert not found');
    });

    it('raises ConfigurationError when the key file is missing', async () => {
        const { certPem } = await _selfSignedCert({ days: 90 });
        const certDir = path.join(_runtimeDir, 'pki', 'issued', 'apps');
        await fs.mkdir(certDir, { recursive: true });
        await fs.writeFile(path.join(certDir, 'g8ed.crt'), certPem);
        // No key file written.

        const service = new AppEnrollmentService();
        await expect(service.loadIdentity()).rejects.toThrow('app key not found');
    });

    it('raises ConfigurationError when the cert is within the renewal threshold', async () => {
        const { certPem, keyPem } = await _selfSignedCert({ days: 3 });
        await _writeExistingIdentity(certPem, keyPem);

        const service = new AppEnrollmentService();
        await expect(service.loadIdentity()).rejects.toThrow(ConfigurationError);
        await expect(service.loadIdentity()).rejects.toThrow('within 7 days of expiry');
    });

    it('raises ConfigurationError when the cert has no SubjectAlternativeName', async () => {
        const { certPem, keyPem } = await _selfSignedCert({ days: 90, includeSan: false });
        await _writeExistingIdentity(certPem, keyPem);

        const service = new AppEnrollmentService();
        await expect(service.loadIdentity()).rejects.toThrow('no SubjectAlternativeName');
    });

    it('raises ConfigurationError when the cert file is unparseable', async () => {
        await _writeExistingIdentity('not a cert', 'not a key');

        const service = new AppEnrollmentService();
        await expect(service.loadIdentity()).rejects.toThrow('failed to parse app cert');
    });
});

// ---------------------------------------------------------------------------
// Tests: enroll (write path)
// ---------------------------------------------------------------------------

describe('AppEnrollmentService.enroll', () => {
    beforeEach(async () => {
        _origEnv = { ...process.env };
        await _isolateRuntimeDir();
        process.env.G8E_GATEWAY_HTTP_URL = 'http://g8e.local:8080';
    });
    afterEach(async () => {
        vi.restoreAllMocks();
        for (const k of Object.keys(process.env)) {
            if (!(k in _origEnv)) delete process.env[k];
        }
        Object.assign(process.env, _origEnv);
        await _cleanupRuntimeDir();
    });

    it('enrolls and writes credentials to disk', async () => {
        const fetchSpy = _mockFetch();

        const service = new AppEnrollmentService();
        const identity = await service.enroll();

        expect(identity.app_id).toBe('spiffe://g8e.local/app/g8ed');
        expect(identity.cert_path).toBe(path.join(_runtimeDir, 'pki', 'issued', 'apps', 'g8ed.crt'));
        expect(identity.key_path).toBe(path.join(_runtimeDir, 'pki', 'issued', 'apps', 'g8ed.key'));
        expect(identity.ca_cert_path).toBe(path.join(_runtimeDir, 'pki', 'trust', 'hub-bundle.pem'));

        // Both HTTP calls fired: CA bundle fetch + enrollment POST.
        const calledUrls = fetchSpy.mock.calls.map(c => String(c[0]));
        expect(calledUrls.some(u => u.endsWith('/.well-known/g8e/pki/ca-bundle'))).toBe(true);
        expect(calledUrls.some(u => u.endsWith('/api/v1/pki/apps/enroll'))).toBe(true);

        // The enrollment POST carried the app_name and a CSR.
        const enrollCall = fetchSpy.mock.calls.find(c => String(c[0]).endsWith('/api/v1/pki/apps/enroll'));
        const body = JSON.parse(enrollCall[1].body);
        expect(body.app_name).toBe('g8ed');
        expect(body.app_type).toBe('custom');
        expect(body.csr_pem).toContain('BEGIN CERTIFICATE REQUEST');

        // Credentials were written to disk with the expected content.
        const certOnDisk = await fs.readFile(identity.cert_path, 'utf8');
        expect(certOnDisk).toContain('FAKE-APP-CERT-PEM');
        const keyOnDisk = await fs.readFile(identity.key_path, 'utf8');
        expect(keyOnDisk).toContain('BEGIN PRIVATE KEY');
        const caOnDisk = await fs.readFile(identity.ca_cert_path, 'utf8');
        // The enrollment response's trust_bundle takes precedence over the
        // well-known-fetched bundle.
        expect(caOnDisk).toBe('FAKE-RESPONSE-BUNDLE');
    });

    it('uses the well-known CA bundle when the response omits trust_bundle', async () => {
        _mockFetch({
            enrollmentResponse: {
                success: true,
                app_id: 'spiffe://g8e.local/app/g8ed',
                app_cert: 'FAKE-APP-CERT-PEM',
                cert_chain: '',
                trust_bundle: '',
                expires_at: '2099-01-01T00:00:00Z',
            },
        });

        const service = new AppEnrollmentService();
        const identity = await service.enroll();

        const caOnDisk = await fs.readFile(identity.ca_cert_path, 'utf8');
        expect(caOnDisk).toBe('CA-BUNDLE-PEM');
    });

    it('writes cert file with the chain appended', async () => {
        _mockFetch({
            enrollmentResponse: {
                success: true,
                app_id: 'spiffe://g8e.local/app/g8ed',
                app_cert: '-----BEGIN CERTIFICATE-----\nAAA\n-----END CERTIFICATE-----\n',
                cert_chain: '-----BEGIN CERTIFICATE-----\nBBB\n-----END CERTIFICATE-----\n',
                trust_bundle: 'CA-BUNDLE',
                expires_at: '2099-01-01T00:00:00Z',
            },
        });

        const service = new AppEnrollmentService();
        const identity = await service.enroll();

        const certOnDisk = await fs.readFile(identity.cert_path, 'utf8');
        expect(certOnDisk).toContain('AAA');
        expect(certOnDisk).toContain('BBB');
    });

    it('sets file permissions: cert and key 0600, CA bundle 0644', async () => {
        _mockFetch();

        const service = new AppEnrollmentService();
        const identity = await service.enroll();

        const certStat = await fs.stat(identity.cert_path);
        const keyStat = await fs.stat(identity.key_path);
        const caStat = await fs.stat(identity.ca_cert_path);
        // Mask to permission bits only.
        expect(certStat.mode & 0o777).toBe(0o600);
        expect(keyStat.mode & 0o777).toBe(0o600);
        expect(caStat.mode & 0o777).toBe(0o644);
    });

    it('raises ConfigurationError when the gateway rejects enrollment (HTTP 400)', async () => {
        _mockFetch({
            enrollmentStatus: 400,
            enrollmentResponse: { success: false, error: 'bad csr' },
        });

        const service = new AppEnrollmentService();
        await expect(service.enroll()).rejects.toThrow(ConfigurationError);
        await expect(service.enroll()).rejects.toThrow('enrollment rejected');
    });

    it('raises ConfigurationError when the CA bundle fetch fails', async () => {
        _mockFetch({ caStatus: 503 });

        const service = new AppEnrollmentService();
        await expect(service.enroll()).rejects.toThrow(ConfigurationError);
        await expect(service.enroll()).rejects.toThrow('failed to fetch CA bundle');
    });

    it('raises ConfigurationError when the enrollment POST has a network error', async () => {
        _mockFetch({ enrollError: new TypeError('connection refused') });

        const service = new AppEnrollmentService();
        await expect(service.enroll()).rejects.toThrow(ConfigurationError);
        await expect(service.enroll()).rejects.toThrow('enrollment POST');
    });

    it('raises ConfigurationError when the response is missing app_cert', async () => {
        _mockFetch({
            enrollmentResponse: {
                success: true,
                app_id: 'spiffe://g8e.local/app/g8ed',
                app_cert: '',
                cert_chain: '',
                trust_bundle: '',
                expires_at: '2099-01-01T00:00:00Z',
            },
        });

        const service = new AppEnrollmentService();
        await expect(service.enroll()).rejects.toThrow('missing app_cert');
    });
});

// ---------------------------------------------------------------------------
// Tests: gateway HTTP URL resolution
// ---------------------------------------------------------------------------

describe('AppEnrollmentService gateway HTTP URL resolution', () => {
    beforeEach(() => {
        _origEnv = { ...process.env };
    });
    afterEach(() => {
        vi.restoreAllMocks();
        for (const k of Object.keys(process.env)) {
            if (!(k in _origEnv)) delete process.env[k];
        }
        Object.assign(process.env, _origEnv);
    });

    it('uses G8E_GATEWAY_HTTP_URL when set (strips trailing slash)', async () => {
        process.env.G8E_GATEWAY_HTTP_URL = 'http://g8e.local:8080/';
        process.env.G8E_RUNTIME_DIR = '/tmp/test';
        const fetchSpy = _mockFetch();

        const service = new AppEnrollmentService();
        await service.enroll();

        const caUrl = String(fetchSpy.mock.calls[0][0]);
        expect(caUrl).toBe('http://g8e.local:8080/.well-known/g8e/pki/ca-bundle');
    });

    it('raises ConfigurationError when G8E_GATEWAY_HTTP_URL is unset (fail-closed, no derivation)', async () => {
        delete process.env.G8E_GATEWAY_HTTP_URL;
        process.env.G8E_RUNTIME_DIR = '/tmp/test';

        const service = new AppEnrollmentService();
        await expect(service.enroll()).rejects.toThrow(ConfigurationError);
        await expect(service.enroll()).rejects.toThrow('G8E_GATEWAY_HTTP_URL is not set');
    });
});

// ---------------------------------------------------------------------------
// Tests: G8E_RUNTIME_DIR resolution
// ---------------------------------------------------------------------------

describe('AppEnrollmentService G8E_RUNTIME_DIR resolution', () => {
    beforeEach(() => {
        _origEnv = { ...process.env };
        process.env.G8E_GATEWAY_HTTP_URL = 'http://g8e.local:8080';
    });
    afterEach(() => {
        vi.restoreAllMocks();
        for (const k of Object.keys(process.env)) {
            if (!(k in _origEnv)) delete process.env[k];
        }
        Object.assign(process.env, _origEnv);
    });

    it('raises ConfigurationError when G8E_RUNTIME_DIR is unset', async () => {
        delete process.env.G8E_RUNTIME_DIR;

        const service = new AppEnrollmentService();
        await expect(service.loadIdentity()).rejects.toThrow('G8E_RUNTIME_DIR is not set');
    });
});

// ---------------------------------------------------------------------------
// Tests: CSR generation
// ---------------------------------------------------------------------------

describe('AppEnrollmentService CSR generation', () => {
    beforeEach(() => {
        _origEnv = { ...process.env };
        process.env.G8E_GATEWAY_HTTP_URL = 'http://g8e.local:8080';
        process.env.G8E_RUNTIME_DIR = '/tmp/test';
    });
    afterEach(() => {
        vi.restoreAllMocks();
        for (const k of Object.keys(process.env)) {
            if (!(k in _origEnv)) delete process.env[k];
        }
        Object.assign(process.env, _origEnv);
    });

    it('generates a CSR with the app name as CN and a P-256 key', async () => {
        const fetchSpy = _mockFetch();

        const service = new AppEnrollmentService({ appName: 'myapp' });
        await service.enroll();

        const enrollCall = fetchSpy.mock.calls.find(c => String(c[0]).endsWith('/api/v1/pki/apps/enroll'));
        const body = JSON.parse(enrollCall[1].body);
        expect(body.app_name).toBe('myapp');
        expect(body.csr_pem).toContain('BEGIN CERTIFICATE REQUEST');

        // Parse the CSR to verify the key type and subject.
        const csrPem = body.csr_pem;
        const csrFile = path.join(os.tmpdir(), `g8ed-csr-${Date.now()}.pem`);
        await fs.writeFile(csrFile, csrPem);
        try {
            const csrResult = execSync(`openssl req -in ${csrFile} -noout -text`, {
                stdio: ['pipe', 'pipe', 'pipe'],
            }).toString();
            expect(csrResult).toContain('Subject: CN = myapp');
            expect(csrResult).toContain('prime256v1');
        } finally {
            await fs.unlink(csrFile);
        }
    });
});
