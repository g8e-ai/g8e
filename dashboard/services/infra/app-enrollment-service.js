// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * AppEnrollmentService — Self-enrollment for the g8ed app identity.
 *
 * The dashboard authenticates to the gateway via its mTLS app cert for
 * server-to-server calls. This service runs at startup, before the Express
 * server listens, and establishes the dashboard's app identity (cert, key,
 * trust bundle) stored in its own runtime directory.
 *
 * Mirrors `ensemble/app/services/infra/app_enrollment_service.py`. Two
 * explicit operations, no hidden fallbacks:
 *
 * - `loadIdentity()` — read path. Loads an existing cert/key pair from disk
 *   and validates that the cert is not expired or near-expiry. Throws
 *   `ConfigurationError` if the cert or key is missing, if the cert is
 *   within the renewal threshold of expiry, or if the cert has no SPIFFE
 *   URI SAN. Does not touch the network.
 *
 * - `enroll()` — write path. Generates a CSR, enrolls with the gateway over
 *   HTTP, persists the cert/key/trust-bundle to disk, and returns an
 *   `AppIdentity`. Always contacts the gateway; never reads cached state.
 *
 * The caller (`server.js` startup) decides which to call. The service does
 * not hide that decision behind an `ensure*` method.
 */

import { promises as fs } from 'node:fs';
import path from 'node:path';
import { webcrypto, X509Certificate as NodeX509Cert } from 'node:crypto';
import x509Pkg from '@peculiar/x509';

const { Pkcs10CertificateRequestGenerator, Name } = x509Pkg;

// Renew when the cert is within this many days of expiry.
const _RENEWAL_THRESHOLD_DAYS = 7;
// App identity metadata.
const _APP_NAME = 'g8ed';
const _APP_TYPE = 'custom';
// HTTP timeout for the bootstrap surface (plain HTTP, no TLS).
const _HTTP_TIMEOUT_MS = 10_000;
// Well-known paths on the gateway's public HTTP bootstrap surface.
const _CA_BUNDLE_PATH = '/.well-known/g8e/pki/ca-bundle';
const _ENROLL_PATH = '/api/v1/pki/apps/enroll';

// Credential path segments, resolved against G8E_RUNTIME_DIR at runtime.
// Defined as named constants, not inline string literals.
const _PKI_ISSUED_APPS_DIR = path.join('pki', 'issued', 'apps');
const _PKI_TRUST_DIR = path.join('pki', 'trust');
const _CA_BUNDLE_FILENAME = 'hub-bundle.pem';

/**
 * ConfigurationError — raised when the enrollment service cannot resolve
 * its configuration or when a credential on disk is missing, expired, or
 * malformed. Distinct from network/transport errors, which are wrapped with
 * context via `ConfigurationError` as well but carry the underlying cause.
 */
export class ConfigurationError extends Error {
    constructor(message, { cause } = {}) {
        super(message);
        this.name = 'ConfigurationError';
        if (cause) this.cause = cause;
    }
}

/**
 * AppIdentity — resolved app identity for mTLS client configuration.
 *
 * @typedef {Object} AppIdentity
 * @property {string} app_id       - SPIFFE URI SAN from the cert (e.g. "spiffe://g8e.local/app/g8ed")
 * @property {string} cert_path    - Absolute path to the enrolled app cert (PEM, includes chain)
 * @property {string} key_path     - Absolute path to the enrolled app private key (PEM, PKCS#8)
 * @property {string} ca_cert_path - Absolute path to the gateway trust bundle (PEM)
 */

/**
 * Resolve the gateway's plain-HTTP bootstrap surface URL.
 *
 * Reads `G8E_GATEWAY_HTTP_URL` env var. Fail-closed: throws
 * `ConfigurationError` if unset — no silent default and no derivation.
 * The dashboard has no docker-internal HTTPS env var to derive from (its
 * `G8E_GATEWAY_URL` is browser-facing `localhost`), so the HTTP URL must
 * be explicit. Compose sets `http://g8eg:8080`.
 *
 * @returns {string} Base URL with trailing slash stripped.
 * @throws {ConfigurationError} If G8E_GATEWAY_HTTP_URL is unset.
 */
function _resolveGatewayHttpUrl() {
    const explicit = process.env.G8E_GATEWAY_HTTP_URL;
    if (!explicit) {
        throw new ConfigurationError(
            `AppEnrollmentService cannot resolve gateway HTTP URL: G8E_GATEWAY_HTTP_URL is not set`
        );
    }
    return explicit.replace(/\/+$/, '');
}

/**
 * Resolve the absolute credential paths under G8E_RUNTIME_DIR.
 *
 * @param {string} appName - App name used in the cert/key filename.
 * @returns {{certPath: string, keyPath: string, caCertPath: string}}
 * @throws {ConfigurationError} If G8E_RUNTIME_DIR is unset.
 */
function _resolveCredentialPaths(appName) {
    const runtimeDir = process.env.G8E_RUNTIME_DIR;
    if (!runtimeDir) {
        throw new ConfigurationError(
            `AppEnrollmentService cannot resolve credential paths: G8E_RUNTIME_DIR is not set`
        );
    }
    return {
        certPath: path.join(runtimeDir, _PKI_ISSUED_APPS_DIR, `${appName}.crt`),
        keyPath: path.join(runtimeDir, _PKI_ISSUED_APPS_DIR, `${appName}.key`),
        caCertPath: path.join(runtimeDir, _PKI_TRUST_DIR, _CA_BUNDLE_FILENAME),
    };
}

/**
 * Generate an ECDSA P-256 private key and CSR.
 *
 * Uses the WebCrypto API (Node 22 `node:crypto.webcrypto`) for key
 * generation and `@peculiar/x509`'s `Pkcs10CertificateRequestGenerator` for
 * CSR construction. The CSR subject CN is the app name. Returns PEM-encoded
 * CSR and PKCS#8 private key.
 *
 * @param {string} appName - Subject common name.
 * @returns {Promise<{csrPem: string, keyPem: string}>}
 */
async function _generateCsr(appName) {
    const keyPair = await webcrypto.subtle.generateKey(
        { name: 'ECDSA', namedCurve: 'P-256' },
        true,
        ['sign', 'verify']
    );

    const csr = await Pkcs10CertificateRequestGenerator.create({
        name: new Name(`CN=${appName}`),
        keys: keyPair,
        signingAlgorithm: { name: 'ECDSA', hash: 'SHA-256' },
    });

    const csrPem = csr.toString('pem');
    const keyPkcs8 = await webcrypto.subtle.exportKey('pkcs8', keyPair.privateKey);
    const keyPem = _pkcs8ToPem(keyPkcs8);
    return { csrPem, keyPem };
}

/**
 * Encode an ArrayBuffer of PKCS#8 key bytes as a PEM PRIVATE KEY string.
 *
 * @param {ArrayBuffer} der
 * @returns {string}
 */
function _pkcs8ToPem(der) {
    const b64 = Buffer.from(der).toString('base64');
    const lines = b64.match(/.{1,64}/g) ?? [b64];
    return `-----BEGIN PRIVATE KEY-----\n${lines.join('\n')}\n-----END PRIVATE KEY-----\n`;
}

/**
 * Extract the SPIFFE app_id from a parsed cert's URI SAN.
 *
 * Node's `crypto.X509Certificate.subjectAltName` returns a string like
 * `URI:spiffe://g8e.local/app/g8ed` or `DNS:example.com, URI:spiffe://...`.
 * This function parses out the first `URI:` entry.
 *
 * @param {import('node:crypto').X509Certificate} cert
 * @returns {string} The first URI SAN value.
 * @throws {ConfigurationError} If the cert has no SubjectAlternativeName
 *   extension or no URI entry in the SAN.
 */
function _extractAppId(cert) {
    const san = cert.subjectAltName;
    if (!san) {
        throw new ConfigurationError(
            'AppEnrollmentService: app cert has no SubjectAlternativeName extension'
        );
    }
    const uriEntry = san.split(',').map(s => s.trim()).find(s => s.startsWith('URI:'));
    if (!uriEntry) {
        throw new ConfigurationError(
            'AppEnrollmentService: app cert has no URI SAN (SPIFFE app_id)'
        );
    }
    return uriEntry.slice('URI:'.length);
}

/**
 * Write cert, key, and trust bundle to the dashboard's runtime tree.
 *
 * The cert file contains the app cert followed by the chain so the mTLS
 * handshake presents the full chain. Permissions: cert and key `0o600`,
 * CA bundle `0o644` (same as the ensemble).
 *
 * @param {string} certPem - App cert PEM.
 * @param {string} certChainPem - Intermediate chain PEM (may be empty).
 * @param {string} keyPem - Private key PEM (PKCS#8).
 * @param {string} trustBundle - Gateway trust bundle PEM.
 * @param {{certPath: string, keyPath: string, caCertPath: string}} paths
 * @returns {Promise<void>}
 */
async function _writeCredentials(certPem, certChainPem, keyPem, trustBundle, paths) {
    await fs.mkdir(path.dirname(paths.certPath), { recursive: true });
    await fs.mkdir(path.dirname(paths.caCertPath), { recursive: true });

    let combined = certPem;
    if (certChainPem && !combined.includes(certChainPem)) {
        combined = combined.trimEnd() + '\n' + certChainPem.trimStart();
    }
    await fs.writeFile(paths.certPath, combined, { mode: 0o600 });
    await fs.chmod(paths.certPath, 0o600);

    await fs.writeFile(paths.keyPath, keyPem, { mode: 0o600 });
    await fs.chmod(paths.keyPath, 0o600);

    if (trustBundle) {
        await fs.writeFile(paths.caCertPath, trustBundle, { mode: 0o644 });
        await fs.chmod(paths.caCertPath, 0o644);
    }

    console.log(
        `AppEnrollmentService: app cert saved (cert=${paths.certPath}, key=${paths.keyPath}, ca=${paths.caCertPath})`
    );
}

/**
 * Fetch the gateway CA bundle from the public well-known endpoint.
 *
 * @param {string} baseUrl - Gateway HTTP base URL (no trailing slash).
 * @param {AbortSignal} signal - Timeout signal.
 * @returns {Promise<string>} PEM-encoded CA bundle.
 * @throws {ConfigurationError} On non-2xx response or network failure.
 */
async function _fetchCaBundle(baseUrl, signal) {
    const url = baseUrl + _CA_BUNDLE_PATH;
    console.log(`AppEnrollmentService: fetching CA bundle from ${url}`);
    let resp;
    try {
        resp = await fetch(url, { signal });
    } catch (err) {
        throw new ConfigurationError(
            `AppEnrollmentService: failed to fetch CA bundle from ${baseUrl}: ${err.message}`,
            { cause: err }
        );
    }
    if (!resp.ok) {
        throw new ConfigurationError(
            `AppEnrollmentService: failed to fetch CA bundle from ${baseUrl}: HTTP ${resp.status}`
        );
    }
    return resp.text();
}

/**
 * Submit the CSR to the gateway's public app enrollment endpoint.
 *
 * The gateway returns HTTP 201 with `{ success, app_cert, cert_chain,
 * trust_bundle, app_id, expires_at }` on success, or HTTP 400 with an
 * error body on rejection. Both paths are handled here.
 *
 * @param {string} baseUrl - Gateway HTTP base URL (no trailing slash).
 * @param {string} csrPem - PEM-encoded CSR.
 * @param {string} appName - App name for the enrollment request.
 * @param {string} appType - App type (one of: mcp-client, a2a-gateway, custom, consensus-member).
 * @param {AbortSignal} signal - Timeout signal.
 * @returns {Promise<Object>} Parsed enrollment response JSON.
 * @throws {ConfigurationError} On network failure, non-2xx response, or
 *   a 2xx response with `success: false`.
 */
async function _submitEnrollment(baseUrl, csrPem, appName, appType, signal) {
    const url = baseUrl + _ENROLL_PATH;
    const payload = { csr_pem: csrPem, app_name: appName, app_type: appType };
    console.log(`AppEnrollmentService: submitting enrollment for app=${appName}`);
    let resp;
    try {
        resp = await fetch(url, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
            signal,
        });
    } catch (err) {
        throw new ConfigurationError(
            `AppEnrollmentService: enrollment POST to ${baseUrl} failed: ${err.message}`,
            { cause: err }
        );
    }
    let data;
    try {
        data = await resp.json();
    } catch (err) {
        throw new ConfigurationError(
            `AppEnrollmentService: enrollment POST to ${baseUrl} returned non-JSON response (HTTP ${resp.status}): ${err.message}`,
            { cause: err }
        );
    }
    if (!resp.ok) {
        const errMsg = data.error || `HTTP ${resp.status}`;
        throw new ConfigurationError(
            `AppEnrollmentService: enrollment rejected by gateway: ${errMsg}`
        );
    }
    if (!data.success) {
        throw new ConfigurationError(
            `AppEnrollmentService: enrollment rejected by gateway: ${data.error || 'unknown error'}`
        );
    }
    return data;
}

export class AppEnrollmentService {
    /**
     * @param {Object} [opts]
     * @param {string} [opts.appName] - App name (default "g8ed").
     * @param {string} [opts.appType] - App type (default "custom").
     */
    constructor({ appName = _APP_NAME, appType = _APP_TYPE } = {}) {
        this._appName = appName;
        this._appType = appType;
    }

    /**
     * Load an existing app identity from disk and validate it.
     *
     * Reads the cert and key from the dashboard's PKI tree, parses the cert
     * to check expiry, and extracts the SPIFFE app_id from the URI SAN.
     * Throws `ConfigurationError` if the cert or key file is missing, if the
     * cert cannot be parsed, if the cert is within the renewal threshold of
     * expiry, or if the cert has no SPIFFE URI SAN.
     *
     * Does not touch the network.
     *
     * @returns {Promise<AppIdentity>}
     */
    async loadIdentity() {
        const paths = _resolveCredentialPaths(this._appName);

        try {
            await fs.access(paths.certPath);
        } catch {
            throw new ConfigurationError(
                `AppEnrollmentService: app cert not found at ${paths.certPath}`
            );
        }
        try {
            await fs.access(paths.keyPath);
        } catch {
            throw new ConfigurationError(
                `AppEnrollmentService: app key not found at ${paths.keyPath}`
            );
        }

        const certPem = await fs.readFile(paths.certPath, 'utf8');
        let cert;
        try {
            cert = new NodeX509Cert(certPem);
        } catch (err) {
            throw new ConfigurationError(
                `AppEnrollmentService: failed to parse app cert at ${paths.certPath}: ${err.message}`,
                { cause: err }
            );
        }

        const expiry = cert.validToDate;
        const remainingMs = expiry.getTime() - Date.now();
        const thresholdMs = _RENEWAL_THRESHOLD_DAYS * 24 * 60 * 60 * 1000;
        if (remainingMs <= thresholdMs) {
            const remainingDays = Math.floor(remainingMs / (24 * 60 * 60 * 1000));
            throw new ConfigurationError(
                `AppEnrollmentService: app cert at ${paths.certPath} is within ${_RENEWAL_THRESHOLD_DAYS} days of expiry (expires ${expiry.toISOString()}, ${remainingDays} days remaining)`
            );
        }

        const appId = _extractAppId(cert);

        console.log(
            `AppEnrollmentService: loaded existing app cert (cert=${paths.certPath}, app_id=${appId}, expires=${expiry.toISOString()})`
        );

        return {
            app_id: appId,
            cert_path: paths.certPath,
            key_path: paths.keyPath,
            ca_cert_path: paths.caCertPath,
        };
    }

    /**
     * Enroll with the gateway and persist the resulting credentials.
     *
     * Generates a CSR, fetches the CA bundle, POSTs the enrollment request,
     * writes the cert/key/trust-bundle to disk, and returns an
     * `AppIdentity`. Always contacts the gateway; never reads cached state.
     * Throws `ConfigurationError` on any failure.
     *
     * @returns {Promise<AppIdentity>}
     */
    async enroll() {
        const baseUrl = _resolveGatewayHttpUrl();
        const paths = _resolveCredentialPaths(this._appName);
        const { csrPem, keyPem } = await _generateCsr(this._appName);

        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), _HTTP_TIMEOUT_MS);

        let trustBundle, enrollment;
        try {
            trustBundle = await _fetchCaBundle(baseUrl, controller.signal);
            enrollment = await _submitEnrollment(
                baseUrl,
                csrPem,
                this._appName,
                this._appType,
                controller.signal
            );
        } finally {
            clearTimeout(timeoutId);
        }

        const certPem = enrollment.app_cert || '';
        const certChainPem = enrollment.cert_chain || '';
        const responseBundle = enrollment.trust_bundle || '';
        const finalTrustBundle = responseBundle || trustBundle;
        const appId = enrollment.app_id || '';

        if (!certPem) {
            throw new ConfigurationError(
                'AppEnrollmentService: enrollment response missing app_cert field'
            );
        }

        await _writeCredentials(certPem, certChainPem, keyPem, finalTrustBundle, paths);

        console.log(
            `AppEnrollmentService: enrolled successfully (app_id=${appId}, app_name=${this._appName})`
        );

        return {
            app_id: appId,
            cert_path: paths.certPath,
            key_path: paths.keyPath,
            ca_cert_path: paths.caCertPath,
        };
    }
}
