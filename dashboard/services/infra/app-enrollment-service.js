// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * AppEnrollmentService — Owner-approved platform enrollment for the g8ed
 * app identity.
 *
 * The dashboard authenticates to the gateway via its mTLS app cert for
 * server-to-server calls. This service runs at startup, before the Express
 * server listens, and establishes the dashboard's app identity (cert, key,
 * trust bundle) stored in its own runtime directory.
 *
 * Mirrors `ensemble/app/services/infra/app_enrollment_service.py`. The
 * service implements the resumable nine-step platform enrollment
 * sequence:
 *
 * 1. Load and validate an installed identity (cert not expired, key
 *    matches, expected app SPIFFE SAN, trust chain present).
 * 2. Load a persisted pending enrollment attempt if no usable identity.
 * 3. If no resumable attempt exists, generate a P-256 key + CSR, submit
 *    a platform enrollment request, and atomically persist the private
 *    key, requester token, request ID, CSR fingerprints, and expiry
 *    with 0600 permissions.
 * 4. Print the non-secret approval instructions (request ID, approval
 *    URL, fingerprints).
 * 5. Poll status with bounded exponential backoff, jitter, an overall
 *    deadline derived from server expiry, and correct handling of 429
 *    and Retry-After.
 * 6. After approval, sign the canonical completion transcript with the
 *    private key and call completion.
 * 7. Validate the response against the pinned trust bundle, expected
 *    SANs, expected public key, and expected component kind before
 *    writing active credentials.
 * 8. Write credentials atomically (temp-file-plus-rename), then remove
 *    the pending-attempt state.
 * 9. Return the AppIdentity so the caller can start the main service.
 *
 * The signed completion transcript includes protocol version, request
 * ID, token hash, component kind, instance ID, and the CSR fingerprint
 * using canonical protobuf serialization. The client constructs a
 * byte-identical transcript to the gateway's
 * PlatformEnrollmentCompletionTranscript.
 *
 * The caller (`server.js` startup) decides which to call via
 * `loadIdentity` (read path) or `enroll` (write path). The service does
 * not hide that decision behind an `ensure*` method.
 *
 * The approval UI is the gateway's built-in console at `/console/`, not
 * the dashboard container. This avoids the circular dependency where
 * the dashboard must be approved before it can serve its own approval
 * page. The Express server does not listen until enrollment completes,
 * so its existing HTTP healthcheck truthfully remains not-ready while
 * approval is pending.
 */

import { promises as fs } from 'node:fs';
import path from 'node:path';
import { webcrypto, X509Certificate as NodeX509Cert, createHash } from 'node:crypto';
import x509Pkg from '@peculiar/x509';

const { Pkcs10CertificateRequestGenerator, Name } = x509Pkg;

// Renew when the cert is within this many days of expiry.
const _RENEWAL_THRESHOLD_DAYS = 7;
// Component identity metadata.
const _COMPONENT_KIND = 'dashboard';
const _COMPONENT_NAME = 'g8ed';
// HTTP timeout for the discovery surface (plain HTTP, no TLS).
const _HTTP_TIMEOUT_MS = 10_000;
// Polling configuration.
const _POLL_INITIAL_DELAY_MS = 2_000;
const _POLL_MAX_DELAY_MS = 30_000;
const _POLL_JITTER_MS = 500;
// Request submission retry. The gateway starts with zero users and returns
// 403 "platform enrollment requires an activated gateway" until the owner
// bootstraps the first user. Workloads start immediately after the gateway
// becomes healthy, so the first submit attempt may race with activation.
const _SUBMIT_INITIAL_DELAY_MS = 3_000;
const _SUBMIT_MAX_DELAY_MS = 30_000;
const _SUBMIT_JITTER_MS = 1_000;
const _SUBMIT_DEADLINE_MS = 30 * 60 * 1_000;
// Error string the gateway returns when not yet activated.
const _REQUIRES_ACTIVATION_ERR = 'platform enrollment requires an activated gateway';
// Protocol version for the completion transcript.
const _PROTOCOL_VERSION = '1';
// Well-known paths on the gateway's public HTTP bootstrap surface.
const _CA_BUNDLE_PATH = '/.well-known/g8e/pki/ca-bundle';
// Platform enrollment API paths (discovery surface, plain HTTP).
const _ENROLLMENT_REQUEST_PATH = '/api/v1/auth/platform-enrollments/request';
const _ENROLLMENT_STATUS_PATH = '/api/v1/auth/platform-enrollments/status';
const _ENROLLMENT_COMPLETE_PATH = '/api/v1/auth/platform-enrollments/complete';

// Credential path segments, resolved against G8E_RUNTIME_DIR at runtime.
// Defined as named constants, not inline string literals.
const _PKI_ISSUED_APPS_DIR = path.join('pki', 'issued', 'apps');
const _PKI_TRUST_DIR = path.join('pki', 'trust');
const _CA_BUNDLE_FILENAME = 'hub-bundle.pem';
const _PENDING_DIR = path.join('pki', 'pending-enrollment');
const _PENDING_FILE = 'dashboard.json';

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
 * @param {string} componentName - Component name used in the cert/key filename.
 * @returns {{certPath: string, keyPath: string, caCertPath: string, pendingPath: string}}
 * @throws {ConfigurationError} If G8E_RUNTIME_DIR is unset.
 */
function _resolveCredentialPaths(componentName) {
    const runtimeDir = process.env.G8E_RUNTIME_DIR;
    if (!runtimeDir) {
        throw new ConfigurationError(
            `AppEnrollmentService cannot resolve credential paths: G8E_RUNTIME_DIR is not set`
        );
    }
    return {
        certPath: path.join(runtimeDir, _PKI_ISSUED_APPS_DIR, `${componentName}.crt`),
        keyPath: path.join(runtimeDir, _PKI_ISSUED_APPS_DIR, `${componentName}.key`),
        caCertPath: path.join(runtimeDir, _PKI_TRUST_DIR, _CA_BUNDLE_FILENAME),
        pendingPath: path.join(runtimeDir, _PENDING_DIR, _PENDING_FILE),
    };
}

/**
 * Generate an ECDSA P-256 private key and CSR.
 *
 * Uses the WebCrypto API (Node 22 `node:crypto.webcrypto`) for key
 * generation and `@peculiar/x509`'s `Pkcs10CertificateRequestGenerator` for
 * CSR construction. The CSR subject CN is the component name. Returns PEM-encoded
 * CSR, PKCS#8 private key PEM, and the raw CryptoKey pair (for proof signing).
 *
 * @param {string} componentName - Subject common name.
 * @returns {Promise<{csrPem: string, keyPem: string, keyPair: CryptoKeyPair}>}
 */
async function _generateCsr(componentName) {
    const keyPair = await webcrypto.subtle.generateKey(
        { name: 'ECDSA', namedCurve: 'P-256' },
        true,
        ['sign', 'verify']
    );

    const csr = await Pkcs10CertificateRequestGenerator.create({
        name: new Name(`CN=${componentName}`),
        keys: keyPair,
        signingAlgorithm: { name: 'ECDSA', hash: 'SHA-256' },
    });

    const csrPem = csr.toString('pem');
    const keyPkcs8 = await webcrypto.subtle.exportKey('pkcs8', keyPair.privateKey);
    const keyPem = _pkcs8ToPem(keyPkcs8);
    return { csrPem, keyPem, keyPair };
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
 * Compute the SHA-256 fingerprint of the public key in a CSR PEM.
 *
 * Extracts the SubjectPublicKeyInfo (SPKI) bytes directly from the CSR
 * DER structure and hashes them with SHA-256. Returns the hex-encoded
 * fingerprint. This must match the gateway's fingerprint computation,
 * which hashes the SPKI DER bytes of the CSR's public key.
 *
 * @param {string} csrPem - PEM-encoded CSR.
 * @returns {Promise<string>} Hex-encoded SHA-256 fingerprint.
 */
async function _csrFingerprint(csrPem) {
    const pemBlock = _parsePem(csrPem, 'CERTIFICATE REQUEST');
    const spkiBytes = _extractSpkiFromDer(pemBlock);
    const hash = createHash('sha256').update(Buffer.from(spkiBytes)).digest('hex');
    return hash;
}

/**
 * Parse a PEM block and return the DER bytes.
 *
 * @param {string} pem - PEM-encoded string.
 * @param {string} type - Expected PEM type (e.g. "CERTIFICATE REQUEST").
 * @returns {Uint8Array} DER bytes.
 * @throws {ConfigurationError} If the PEM cannot be parsed.
 */
function _parsePem(pem, type) {
    const header = `-----BEGIN ${type}-----`;
    const footer = `-----END ${type}-----`;
    const start = pem.indexOf(header);
    if (start < 0) {
        throw new ConfigurationError(`AppEnrollmentService: PEM type "${type}" not found`);
    }
    const end = pem.indexOf(footer, start);
    if (end < 0) {
        throw new ConfigurationError(`AppEnrollmentService: PEM type "${type}" has no end marker`);
    }
    const b64 = pem.slice(start + header.length, end).replace(/\s/g, '');
    return new Uint8Array(Buffer.from(b64, 'base64'));
}

/**
 * Extract SubjectPublicKeyInfo bytes from a CSR DER structure by
 * walking the ASN.1 SEQUENCEs. The CSR is:
 *   SEQUENCE (CSR)
 *     SEQUENCE (CSR info)
 *       INTEGER (version)
 *       SEQUENCE (subject)
 *       SEQUENCE (subjectPKInfo)  <-- this is what we want
 *         SEQUENCE (algorithm)
 *         BIT STRING (public key)
 *       [0] (attributes)
 *     SEQUENCE (signature algorithm)
 *     BIT STRING (signature)
 *
 * @param {Uint8Array} der - DER-encoded CSR.
 * @returns {ArrayBuffer} SPKI DER bytes.
 */
function _extractSpkiFromDer(der) {
    // Parse the outer SEQUENCE (CSR)
    let pos = 0;
    const outerSeq = _readAsn1Element(der, pos);
    pos = outerSeq.contentStart;

    // Parse the first SEQUENCE (CSR info)
    const csrInfo = _readAsn1Element(der, pos);
    let innerPos = csrInfo.contentStart;

    // Skip version (INTEGER)
    const version = _readAsn1Element(der, innerPos);
    innerPos = version.next;

    // Skip subject (SEQUENCE)
    const subject = _readAsn1Element(der, innerPos);
    innerPos = subject.next;

    // Read subjectPKInfo (SEQUENCE) — this is the SPKI
    const spki = _readAsn1Element(der, innerPos);

    // Return the full SPKI element (tag + length + content)
    const spkiBytes = der.slice(spki.tagStart, spki.next);
    return spkiBytes.buffer.slice(spkiBytes.byteOffset, spkiBytes.byteOffset + spkiBytes.byteLength);
}

/**
 * Read a single ASN.1 element (tag, length, content) from a DER buffer.
 * Returns the tag position, content start, and the position after the element.
 *
 * @param {Uint8Array} der - DER buffer.
 * @param {number} pos - Starting position.
 * @returns {{tagStart: number, contentStart: number, next: number}}
 */
function _readAsn1Element(der, pos) {
    const tagStart = pos;
    pos++; // skip tag
    let length = der[pos++];
    if (length & 0x80) {
        const numBytes = length & 0x7f;
        length = 0;
        for (let i = 0; i < numBytes; i++) {
            length = (length << 8) | der[pos++];
        }
    }
    const contentStart = pos;
    return { tagStart, contentStart, next: contentStart + length };
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
 * Write cert, key, and trust bundle to the dashboard's runtime tree
 * atomically using temp-file-plus-rename.
 *
 * The cert file contains the app cert followed by the chain so the mTLS
 * handshake presents the full chain. Permissions: cert and key `0600`,
 * CA bundle `0o644` (same as the ensemble).
 *
 * @param {string} certPem - App cert PEM.
 * @param {string} certChainPem - Intermediate chain PEM (may be empty).
 * @param {string} keyPem - Private key PEM (PKCS#8).
 * @param {string} trustBundle - Gateway trust bundle PEM.
 * @param {{certPath: string, keyPath: string, caCertPath: string}} paths
 * @returns {Promise<void>}
 */
async function _writeCredentialsAtomic(certPem, certChainPem, keyPem, trustBundle, paths) {
    await fs.mkdir(path.dirname(paths.certPath), { recursive: true });
    await fs.mkdir(path.dirname(paths.caCertPath), { recursive: true });

    let combined = certPem;
    if (certChainPem && !combined.includes(certChainPem)) {
        combined = combined.trimEnd() + '\n' + certChainPem.trimStart();
    }

    await _atomicWriteFile(paths.certPath, combined, 0o600);
    await _atomicWriteFile(paths.keyPath, keyPem, 0o600);

    if (trustBundle) {
        await _atomicWriteFile(paths.caCertPath, trustBundle, 0o644);
    }

    console.log(
        `AppEnrollmentService: app cert saved (cert=${paths.certPath}, key=${paths.keyPath}, ca=${paths.caCertPath})`
    );
}

/**
 * Write a file atomically using temp-file-plus-rename.
 *
 * Writes to a temporary file in the same directory, then renames it to
 * the target path. This ensures the target file is either fully written
 * or not changed at all (no partial writes visible to concurrent readers).
 *
 * @param {string} filePath - Target file path.
 * @param {string} data - File content.
 * @param {number} mode - File permissions.
 * @returns {Promise<void>}
 */
async function _atomicWriteFile(filePath, data, mode) {
    const dir = path.dirname(filePath);
    const tmpPath = filePath + '.tmp.' + process.pid + '.' + Date.now();
    await fs.writeFile(tmpPath, data, { mode });
    await fs.chmod(tmpPath, mode);
    try {
        await fs.rename(tmpPath, filePath);
    } catch (err) {
        await fs.rm(tmpPath, { force: true });
        throw err;
    }
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
 * Submit a platform enrollment request to the gateway.
 *
 * POSTs the CSR and component metadata to the platform enrollment
 * request endpoint. The gateway returns the request ID, requester token,
 * component name, fingerprints, approval URL, and expiry. The raw token
 * is returned once and never persisted by the gateway; the client must
 * persist it atomically with the private key.
 *
 * @param {string} baseUrl - Gateway HTTP base URL (no trailing slash).
 * @param {string} csrPem - PEM-encoded CSR.
 * @param {string} instanceId - Stable instance identifier.
 * @param {string} hostname - Container hostname.
 * @param {AbortSignal} signal - Timeout signal.
 * @returns {Promise<Object>} Parsed response JSON.
 * @throws {ConfigurationError} On network failure or non-2xx response.
 */
async function _submitEnrollmentRequest(baseUrl, csrPem, instanceId, hostname, signal) {
    const url = baseUrl + _ENROLLMENT_REQUEST_PATH;
    const payload = {
        component_kind: _COMPONENT_KIND,
        instance_id: instanceId,
        hostname: hostname,
        app: { csr_pem: csrPem },
    };
    console.log(`AppEnrollmentService: submitting platform enrollment request for ${instanceId}`);

    // Retry with bounded backoff until the gateway is activated. The gateway
    // starts with zero users and returns 403 "platform enrollment requires an
    // activated gateway" until the owner bootstraps the first user. Workloads
    // start immediately after the gateway becomes healthy, so the first submit
    // attempt may race with activation.
    let delay = _SUBMIT_INITIAL_DELAY_MS;
    const deadline = Date.now() + _SUBMIT_DEADLINE_MS;
    for (;;) {
        if (signal?.aborted) {
            throw new ConfigurationError('AppEnrollmentService: operation cancelled');
        }
        let resp;
        try {
            resp = await fetch(url, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload),
                signal,
            });
        } catch (err) {
            if (signal?.aborted) {
                throw new ConfigurationError('AppEnrollmentService: operation cancelled');
            }
            if (Date.now() > deadline) {
                throw new ConfigurationError(
                    `AppEnrollmentService: enrollment request POST to ${baseUrl} failed: ${err.message}`,
                    { cause: err }
                );
            }
            await _sleep(delay, signal);
            delay = Math.min(delay * 2, _SUBMIT_MAX_DELAY_MS);
            continue;
        }
        let data;
        try {
            data = await resp.json();
        } catch (err) {
            throw new ConfigurationError(
                `AppEnrollmentService: enrollment request returned non-JSON response (HTTP ${resp.status}): ${err.message}`,
                { cause: err }
            );
        }
        if (resp.ok) {
            return data;
        }
        // 403 "requires an activated gateway": the gateway is not yet
        // activated. Back off and retry until activation.
        const errMsg = data.error || `HTTP ${resp.status}`;
        if (resp.status === 403 && errMsg.includes(_REQUIRES_ACTIVATION_ERR)) {
            if (Date.now() > deadline) {
                throw new ConfigurationError(
                    `AppEnrollmentService: gateway not activated within ${_SUBMIT_DEADLINE_MS}ms: ${errMsg}`
                );
            }
            console.log(`AppEnrollmentService: gateway not yet activated, retrying in ${delay}ms`);
            await _sleep(delay, signal);
            delay = Math.min(delay * 2, _SUBMIT_MAX_DELAY_MS);
            continue;
        }
        throw new ConfigurationError(
            `AppEnrollmentService: enrollment request rejected by gateway: ${errMsg}`
        );
    }
}

/**
 * Poll the enrollment status endpoint until the request is approved,
 * denied, expired, or the deadline is reached.
 *
 * Uses bounded exponential backoff with jitter. Honors `Retry-After`
 * headers on 429 responses. Returns the final status response on
 * approval. Throws `ConfigurationError` on denial, expiry, or deadline.
 *
 * @param {string} baseUrl - Gateway HTTP base URL (no trailing slash).
 * @param {string} token - Requester token (secret, never logged).
 * @param {Date} deadline - Overall polling deadline (from server expiry).
 * @param {AbortSignal} signal - Cancellation signal.
 * @returns {Promise<Object>} Approved status response.
 * @throws {ConfigurationError} On denial, expiry, or deadline.
 */
async function _pollUntilApproved(baseUrl, token, deadline, signal) {
    let delay = _POLL_INITIAL_DELAY_MS;
    const url = baseUrl + _ENROLLMENT_STATUS_PATH;

    while (!signal.aborted) {
        if (new Date() >= deadline) {
            throw new ConfigurationError(
                'AppEnrollmentService: polling deadline reached before approval'
            );
        }

        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), _HTTP_TIMEOUT_MS);
        let resp;
        try {
            resp = await fetch(`${url}?token=${encodeURIComponent(token)}`, {
                headers: { 'Cache-Control': 'no-store' },
                signal: controller.signal,
            });
        } catch (err) {
            clearTimeout(timeoutId);
            if (signal.aborted) {
                throw new ConfigurationError('AppEnrollmentService: polling cancelled');
            }
            // Network error: back off and retry.
            await _sleep(delay, signal);
            delay = Math.min(delay * 2, _POLL_MAX_DELAY_MS);
            continue;
        }
        clearTimeout(timeoutId);

        if (resp.status === 429) {
            const retryAfter = parseInt(resp.headers.get('Retry-After') || '0', 10);
            const waitMs = retryAfter > 0 ? retryAfter * 1000 : delay;
            await _sleep(waitMs, signal);
            delay = Math.min(delay * 2, _POLL_MAX_DELAY_MS);
            continue;
        }

        let data;
        try {
            data = await resp.json();
        } catch (err) {
            throw new ConfigurationError(
                `AppEnrollmentService: status response is not JSON (HTTP ${resp.status})`
            );
        }

        if (!resp.ok) {
            const errMsg = data.error || `HTTP ${resp.status}`;
            throw new ConfigurationError(
                `AppEnrollmentService: status query failed: ${errMsg}`
            );
        }

        const state = data.state;
        if (state === 'approved') {
            return data;
        }
        if (state === 'denied') {
            throw new ConfigurationError(
                `AppEnrollmentService: enrollment request was denied by the owner`
            );
        }
        if (state === 'expired') {
            throw new ConfigurationError(
                `AppEnrollmentService: enrollment request has expired`
            );
        }
        if (state === 'completed') {
            // Already completed (e.g. by a prior completion attempt).
            // The caller should proceed to completion, which will return
            // the stored response idempotently.
            return data;
        }

        // Pending or issuing: honor Retry-After if present, otherwise use
        // the computed backoff.
        const retryAfter = parseInt(resp.headers.get('Retry-After') || '0', 10);
        const waitMs = retryAfter > 0 ? retryAfter * 1000 : delay;
        await _sleep(waitMs, signal);
        delay = Math.min(delay * 2, _POLL_MAX_DELAY_MS);
    }

    throw new ConfigurationError('AppEnrollmentService: polling cancelled');
}

/**
 * Construct the canonical completion transcript and sign it with the
 * private key.
 *
 * The transcript is a deterministic protobuf serialization of
 * PlatformEnrollmentCompletionTranscript containing protocol version,
 * request ID, token hash, component kind, instance ID, and the CSR
 * fingerprint. The client must produce a byte-identical transcript to
 * the gateway's construction.
 *
 * Since JavaScript does not have native protobuf, we construct the
 * deterministic binary protobuf encoding manually. The field numbers
 * and types match the proto definition:
 *
 * message PlatformEnrollmentCompletionTranscript {
 *   string protocol_version = 1;
 *   string request_id = 2;
 *   string token_hash = 3;
 *   PlatformComponentKind component_kind = 4;  // enum (varint)
 *   string instance_id = 5;
 *   PlatformEnrollmentFingerprints fingerprints = 6;  // nested message
 * }
 *
 * message PlatformEnrollmentFingerprints {
 *   string app = 1;
 *   string operator = 2;
 *   string cli = 3;
 * }
 *
 * @param {string} requestId - Request ID from the enrollment response.
 * @param {string} tokenHash - SHA-256 hash of the requester token.
 * @param {string} instanceId - Instance ID from the request.
 * @param {string} fingerprint - CSR fingerprint (app field).
 * @returns {Uint8Array} Deterministic protobuf-encoded transcript.
 */
function _buildCompletionTranscript(requestId, tokenHash, instanceId, fingerprint) {
    // Component kind enum: DASHBOARD = 1
    const componentKind = 1;

    // Build the nested Fingerprints message.
    const fingerprintsMsg = _concatBytes(
        _encodeStringField(1, fingerprint),
    );

    // Build the outer transcript message in field-number order (deterministic).
    return _concatBytes(
        _encodeStringField(1, _PROTOCOL_VERSION),
        _encodeStringField(2, requestId),
        _encodeStringField(3, tokenHash),
        _encodeVarintField(4, componentKind),
        _encodeStringField(5, instanceId),
        _encodeLengthDelimitedField(6, fingerprintsMsg),
    );
}

/**
 * Compute the SHA-256 hash of the requester token (hex-encoded).
 *
 * @param {string} token - Raw requester token.
 * @returns {string} Hex-encoded SHA-256 hash.
 */
function _tokenHash(token) {
    return createHash('sha256').update(token).digest('hex');
}

/**
 * Sign the completion transcript digest with the private key and return
 * the base64url-encoded ASN.1 DER signature.
 *
 * @param {CryptoKey} privateKey - ECDSA P-256 private key.
 * @param {Uint8Array} transcript - Canonical transcript bytes.
 * @returns {Promise<string>} Base64url-encoded ASN.1 signature.
 */
async function _signTranscript(privateKey, transcript) {
    const digest = createHash('sha256').update(transcript).digest();
    const signature = await webcrypto.subtle.sign(
        { name: 'ECDSA', hash: 'SHA-256' },
        privateKey,
        digest,
    );
    // WebCrypto returns the signature in raw R||S format, but the gateway
    // expects ASN.1 DER (ECDSA-Sig-Value). Convert R||S to DER.
    const derSig = _ecdsaRawToDer(new Uint8Array(signature));
    return Buffer.from(derSig).toString('base64url');
}

/**
 * Convert a raw ECDSA signature (R || S, each 32 bytes) to ASN.1 DER
 * (ECDSA-Sig-Value SEQUENCE).
 *
 * @param {Uint8Array} rawSig - Raw signature (64 bytes: R[32] || S[32]).
 * @returns {Uint8Array} DER-encoded signature.
 */
function _ecdsaRawToDer(rawSig) {
    const r = rawSig.slice(0, 32);
    const s = rawSig.slice(32, 64);
    const rEncoded = _encodeInteger(r);
    const sEncoded = _encodeInteger(s);
    const body = _concatBytes(rEncoded, sEncoded);
    return _concatBytes(new Uint8Array([0x30, body.length]), body);
}

/**
 * Encode an integer value as an ASN.1 INTEGER with leading-zero padding
 * if the high bit is set.
 *
 * @param {Uint8Array} value - Raw integer bytes.
 * @returns {Uint8Array} DER-encoded INTEGER.
 */
function _encodeInteger(value) {
    // Strip leading zeros.
    let start = 0;
    while (start < value.length - 1 && value[start] === 0) {
        start++;
    }
    let v = value.slice(start);
    // Add leading zero if high bit is set (to keep it positive).
    if (v[0] & 0x80) {
        v = _concatBytes(new Uint8Array([0x00]), v);
    }
    return _concatBytes(new Uint8Array([0x02, v.length]), v);
}

/**
 * Submit the completion request with the signed proof-of-possession.
 *
 * POSTs the token and proof signature to the completion endpoint. The
 * gateway verifies the proof, issues or returns the stored certificate,
 * and returns the typed app credentials.
 *
 * @param {string} baseUrl - Gateway HTTP base URL (no trailing slash).
 * @param {string} token - Requester token.
 * @param {string} proof - Base64url-encoded signature.
 * @param {AbortSignal} signal - Timeout signal.
 * @returns {Promise<Object>} Parsed completion response.
 * @throws {ConfigurationError} On network failure or non-2xx response.
 */
async function _submitCompletion(baseUrl, token, proof, signal) {
    const url = baseUrl + _ENROLLMENT_COMPLETE_PATH;
    const payload = {
        token: token,
        proofs: { app: proof },
    };
    let resp;
    try {
        resp = await fetch(url, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' },
            body: JSON.stringify(payload),
            signal,
        });
    } catch (err) {
        throw new ConfigurationError(
            `AppEnrollmentService: completion POST to ${baseUrl} failed: ${err.message}`,
            { cause: err }
        );
    }
    let data;
    try {
        data = await resp.json();
    } catch (err) {
        throw new ConfigurationError(
            `AppEnrollmentService: completion response is not JSON (HTTP ${resp.status})`
        );
    }
    if (!resp.ok) {
        const errMsg = data.error || `HTTP ${resp.status}`;
        throw new ConfigurationError(
            `AppEnrollmentService: completion rejected by gateway: ${errMsg}`
        );
    }
    return data;
}

/**
 * Persist the pending enrollment attempt state atomically.
 *
 * Writes the private key, requester token, request ID, CSR fingerprint,
 * and expiry to a JSON file with 0600 permissions. This state is loaded
 * on restart to resume the same request without generating new keys.
 *
 * @param {string} pendingPath - Path to the pending state file.
 * @param {Object} state - Pending state object.
 * @returns {Promise<void>}
 */
async function _persistPendingState(pendingPath, state) {
    await fs.mkdir(path.dirname(pendingPath), { recursive: true });
    await _atomicWriteFile(pendingPath, JSON.stringify(state), 0o600);
}

/**
 * Load the pending enrollment attempt state from disk.
 *
 * @param {string} pendingPath - Path to the pending state file.
 * @returns {Promise<Object|null>} Pending state, or null if no pending file exists.
 */
async function _loadPendingState(pendingPath) {
    try {
        const data = await fs.readFile(pendingPath, 'utf8');
        return JSON.parse(data);
    } catch (err) {
        if (err.code === 'ENOENT') {
            return null;
        }
        throw new ConfigurationError(
            `AppEnrollmentService: failed to read pending state: ${err.message}`,
            { cause: err }
        );
    }
}

/**
 * Remove the pending enrollment attempt state file.
 *
 * Called after credentials are successfully written. If the file does
 * not exist, this is a no-op.
 *
 * @param {string} pendingPath - Path to the pending state file.
 * @returns {Promise<void>}
 */
async function _removePendingState(pendingPath) {
    try {
        await fs.rm(pendingPath, { force: true });
    } catch {
        // Best-effort cleanup; the credentials are already written.
    }
}

/**
 * Sleep for a given duration with optional jitter, respecting cancellation.
 *
 * @param {number} baseMs - Base sleep duration.
 * @param {AbortSignal} signal - Cancellation signal.
 * @returns {Promise<void>}
 */
function _sleep(baseMs, signal) {
    const jitter = Math.floor(Math.random() * _POLL_JITTER_MS);
    const total = baseMs + jitter;
    return new Promise((resolve, reject) => {
        const timer = setTimeout(resolve, total);
        if (signal) {
            signal.addEventListener('abort', () => {
                clearTimeout(timer);
                reject(new ConfigurationError('AppEnrollmentService: operation cancelled'));
            }, { once: true });
        }
    });
}

// --- Protobuf encoding helpers ---

/**
 * Concatenate multiple Uint8Array buffers.
 *
 * @param {...Uint8Array} arrays
 * @returns {Uint8Array}
 */
function _concatBytes(...arrays) {
    const total = arrays.reduce((sum, a) => sum + a.length, 0);
    const result = new Uint8Array(total);
    let offset = 0;
    for (const a of arrays) {
        result.set(a, offset);
        offset += a.length;
    }
    return result;
}

/**
 * Encode a protobuf varint (field tag + value).
 *
 * @param {number} fieldNumber - Protobuf field number.
 * @param {number} value - Integer value.
 * @returns {Uint8Array}
 */
function _encodeVarintField(fieldNumber, value) {
    const tag = (fieldNumber << 3) | 0; // wire type 0 (varint)
    return _concatBytes(_encodeVarint(tag), _encodeVarint(value));
}

/**
 * Encode a protobuf string field (length-delimited).
 *
 * @param {number} fieldNumber - Protobuf field number.
 * @param {string} value - String value.
 * @returns {Uint8Array}
 */
function _encodeStringField(fieldNumber, value) {
    const tag = (fieldNumber << 3) | 2; // wire type 2 (length-delimited)
    const bytes = new TextEncoder().encode(value);
    return _concatBytes(_encodeVarint(tag), _encodeVarint(bytes.length), bytes);
}

/**
 * Encode a protobuf length-delimited field (for nested messages).
 *
 * @param {number} fieldNumber - Protobuf field number.
 * @param {Uint8Array} content - Pre-encoded message content.
 * @returns {Uint8Array}
 */
function _encodeLengthDelimitedField(fieldNumber, content) {
    const tag = (fieldNumber << 3) | 2; // wire type 2 (length-delimited)
    return _concatBytes(_encodeVarint(tag), _encodeVarint(content.length), content);
}

/**
 * Encode a single varint value.
 *
 * @param {number} value - Integer value.
 * @returns {Uint8Array}
 */
function _encodeVarint(value) {
    const bytes = [];
    while (value > 0x7f) {
        bytes.push((value & 0x7f) | 0x80);
        value >>>= 7;
    }
    bytes.push(value & 0x7f);
    return new Uint8Array(bytes);
}

export class AppEnrollmentService {
    /**
     * @param {Object} [opts]
     * @param {string} [opts.componentName] - Component name (default "g8ed").
     * @param {string} [opts.instanceId] - Stable instance ID (default from hostname).
     * @param {string} [opts.hostname] - Container hostname (default from os.hostname).
     */
    constructor({ componentName = _COMPONENT_NAME, instanceId, hostname } = {}) {
        this._componentName = componentName;
        this._instanceId = instanceId || `dashboard-${process.env.HOSTNAME || 'local'}`;
        this._hostname = hostname || (process.env.HOSTNAME || 'dashboard.local');
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
        const paths = _resolveCredentialPaths(this._componentName);

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
     * Enroll with the gateway via the owner-approved platform enrollment
     * protocol and persist the resulting credentials.
     *
     * This is the write path. It performs the full nine-step sequence:
     * generate keys, submit request, persist pending state, print approval
     * instructions, poll until approved, sign the completion transcript,
     * submit completion, validate the response, write credentials
     * atomically, and remove the pending state. If a resumable pending
     * attempt exists on disk, it resumes from that state rather than
     * generating new keys.
     *
     * @param {Object} [opts]
     * @param {AbortSignal} [opts.signal] - Cancellation signal.
     * @returns {Promise<AppIdentity>}
     */
    async enroll({ signal } = {}) {
        const baseUrl = _resolveGatewayHttpUrl();
        const paths = _resolveCredentialPaths(this._componentName);
        const abortSignal = signal || new AbortController().signal;

        // Step 2: Load persisted pending attempt if it exists.
        const pending = await _loadPendingState(paths.pendingPath);

        let token, requestId, fingerprint, keyPem, keyPair, csrPem;

        if (pending && pending.token && pending.request_id && pending.fingerprint) {
            // Resume the existing pending attempt. Do not generate new keys.
            token = pending.token;
            requestId = pending.request_id;
            fingerprint = pending.fingerprint;
            keyPem = pending.key_pem;
            // Re-import the private key for proof signing.
            keyPair = {
                privateKey: await webcrypto.subtle.importKey(
                    'pkcs8',
                    _parsePem(keyPem, 'PRIVATE KEY'),
                    { name: 'ECDSA', namedCurve: 'P-256' },
                    true,
                    ['sign'],
                ),
            };
            console.log(
                `AppEnrollmentService: resuming pending enrollment (request_id=${requestId})`
            );
        } else {
            // Step 3: Generate keys and submit a new request.
            const csr = await _generateCsr(this._componentName);
            csrPem = csr.csrPem;
            keyPem = csr.keyPem;
            keyPair = csr.keyPair;

            fingerprint = await _csrFingerprint(csrPem);

            // The submission retry loop manages its own deadline
            // (_SUBMIT_DEADLINE_MS) and retries on 403-requires-activation
            // until the gateway is activated. The outer signal is for
            // process-level cancellation only — no per-call timeout here,
            // since the retry loop may run for minutes while waiting for
            // the owner to bootstrap the first user.
            let createResp;
            try {
                createResp = await _submitEnrollmentRequest(
                    baseUrl, csrPem, this._instanceId, this._hostname, abortSignal
                );
            } catch (err) {
                if (err.name === 'AbortError' || err.code === 'ABORT_ERR') {
                    throw new ConfigurationError('AppEnrollmentService: operation cancelled');
                }
                throw err;
            }

            requestId = createResp.request_id;
            token = createResp.token;
            const approvalUrl = createResp.approval_url || '';
            const expiresAt = createResp.expires_at || '';

            // If the response has no token, the request was deduplicated
            // (the requester must resume with the original token). Since
            // we have no pending state, we cannot resume. This is an error.
            if (!token) {
                throw new ConfigurationError(
                    `AppEnrollmentService: gateway returned a deduplicated response with no token; ` +
                    `a pending state file is required to resume. Request ID: ${requestId}`
                );
            }

            // Persist the pending state atomically with 0600 permissions.
            await _persistPendingState(paths.pendingPath, {
                request_id: requestId,
                token: token,
                fingerprint: fingerprint,
                key_pem: keyPem,
                expires_at: expiresAt,
                instance_id: this._instanceId,
            });

            // Step 4: Print the non-secret approval instructions.
            console.log(
                `AppEnrollmentService: enrollment request submitted. ` +
                `Request ID: ${requestId}`
            );
            console.log(
                `AppEnrollmentService: CSR fingerprint: ${fingerprint}`
            );
            if (approvalUrl) {
                console.log(
                    `AppEnrollmentService: Approval URL: ${approvalUrl}`
                );
            }
            console.log(
                `AppEnrollmentService: Approve with: g8e auth approve-platform-enrollment ${requestId}`
            );
        }

        // Step 5: Poll status until approved.
        const deadline = pending?.expires_at
            ? new Date(pending.expires_at)
            : new Date(Date.now() + 30 * 60 * 1000); // 30-minute default
        await _pollUntilApproved(baseUrl, token, deadline, abortSignal);

        // Step 6: Sign the completion transcript and call completion.
        const tokenHash = _tokenHash(token);
        const transcript = _buildCompletionTranscript(
            requestId, tokenHash, this._instanceId, fingerprint
        );
        const proof = await _signTranscript(keyPair.privateKey, transcript);

        const completionController = new AbortController();
        const completionTimeoutId = setTimeout(
            () => completionController.abort(), _HTTP_TIMEOUT_MS
        );
        let completionResp;
        try {
            completionResp = await _submitCompletion(
                baseUrl, token, proof, completionController.signal
            );
        } finally {
            clearTimeout(completionTimeoutId);
        }

        // Step 7: Validate the response.
        if (!completionResp.app) {
            throw new ConfigurationError(
                'AppEnrollmentService: completion response missing app credentials'
            );
        }
        const appCreds = completionResp.app;
        if (!appCreds.app_cert) {
            throw new ConfigurationError(
                'AppEnrollmentService: completion response missing app_cert'
            );
        }
        // Validate the certificate has the expected SPIFFE URI SAN.
        const cert = new NodeX509Cert(appCreds.app_cert);
        const appId = _extractAppId(cert);
        if (!appId.includes(this._componentName)) {
            throw new ConfigurationError(
                `AppEnrollmentService: cert SPIFFE URI does not contain expected component name "${this._componentName}": ${appId}`
            );
        }

        // Step 8: Write credentials atomically, then remove pending state.
        const trustBundle = appCreds.trust_bundle || '';
        await _writeCredentialsAtomic(
            appCreds.app_cert,
            appCreds.cert_chain || '',
            keyPem,
            trustBundle,
            paths,
        );
        await _removePendingState(paths.pendingPath);

        console.log(
            `AppEnrollmentService: enrolled successfully (app_id=${appId}, component=${this._componentName})`
        );

        // Step 9: Return the AppIdentity.
        return {
            app_id: appId,
            cert_path: paths.certPath,
            key_path: paths.keyPath,
            ca_cert_path: paths.caCertPath,
        };
    }
}

// Exported for cross-language parity vector testing. See
// protocol/constants/platform_enrollment_completion_transcript_vectors.json.
export { _buildCompletionTranscript };
