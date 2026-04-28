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


import { describe, it, expect, beforeEach, beforeAll, afterEach, vi } from 'vitest';
import { v4 as uuidv4 } from 'uuid';
import crypto from 'crypto';
import * as x509 from '@peculiar/x509';
import { mkdtempSync, rmSync, mkdirSync, writeFileSync } from 'fs';
import { join } from 'path';
import { tmpdir } from 'os';

// Configure @peculiar/x509 to use Node.js crypto
x509.cryptoProvider.set(crypto.webcrypto);

function pemToDer(pem, label) {
    const regex = new RegExp(`-----BEGIN ${label}-----([\\s\\S]*?)-----END ${label}-----`);
    const match = pem.match(regex);
    return Buffer.from(match[1].replace(/\s/g, ''), 'base64');
}

async function generateTestCA() {
    const { privateKey: caKeyPem, publicKey: caPubPem } = crypto.generateKeyPairSync('ec', {
        namedCurve: 'P-384',
        publicKeyEncoding: { type: 'spki', format: 'pem' },
        privateKeyEncoding: { type: 'pkcs8', format: 'pem' }
    });
    const caPublicKey = await crypto.webcrypto.subtle.importKey(
        'spki', pemToDer(caPubPem, 'PUBLIC KEY'),
        { name: 'ECDSA', namedCurve: 'P-384' }, true, ['verify']
    );
    const caPrivateKey = await crypto.webcrypto.subtle.importKey(
        'pkcs8', pemToDer(caKeyPem, 'PRIVATE KEY'),
        { name: 'ECDSA', namedCurve: 'P-384' }, false, ['sign']
    );
    const caCert = await x509.X509CertificateGenerator.createSelfSigned({
        serialNumber: crypto.randomBytes(8).toString('hex').toUpperCase(),
        name: 'CN=g8e Operator CA, O=g8e, C=US',
        notBefore: new Date(),
        notAfter: new Date(Date.now() + 10 * 365 * 24 * 60 * 60 * 1000),
        signingAlgorithm: { name: 'ECDSA', hash: 'SHA-384' },
        keys: { privateKey: caPrivateKey, publicKey: caPublicKey },
        extensions: [
            new x509.BasicConstraintsExtension(true, undefined, true),
            new x509.KeyUsagesExtension(x509.KeyUsageFlags.keyCertSign | x509.KeyUsageFlags.cRLSign, true)
        ]
    });
    return { caCertPem: caCert.toString('pem'), caKeyPem };
}

async function seedCA(sslDir) {
    const { caCertPem, caKeyPem } = await generateTestCA();
    mkdirSync(join(sslDir, 'ca'), { recursive: true });
    writeFileSync(join(sslDir, 'ca', 'ca.crt'), caCertPem);
    writeFileSync(join(sslDir, 'ca', 'ca.key'), caKeyPem);
    return { caCertPem, caKeyPem };
}

async function seedCAWithLegacyEcKey(sslDir) {
    const { caCertPem, caKeyPem } = await generateTestCA();
    const legacyEcKey = crypto.createPrivateKey({ key: caKeyPem, format: 'pem' })
        .export({ type: 'sec1', format: 'pem' });
    mkdirSync(join(sslDir, 'ca'), { recursive: true });
    writeFileSync(join(sslDir, 'ca', 'ca.crt'), caCertPem);
    writeFileSync(join(sslDir, 'ca', 'ca.key'), legacyEcKey);
    return { caCertPem, caKeyPem: legacyEcKey };
}

vi.mock('@g8ed/utils/logger.js', () => ({
    logger: {
        debug: vi.fn(),
        info: vi.fn(),
        warn: vi.fn(),
        error: vi.fn()
    }
}));

describe('CertificateService [UNIT - filesystem isolated]', { timeout: 30000 }, () => {
    let CertificateService;
    let certService;
    let tmpSslDir;

    beforeAll(async () => {
        const module = await import('@g8ed/services/platform/certificate_service.js');
        CertificateService = module.CertificateService;
    });

    beforeEach(async () => {
        vi.clearAllMocks();
        tmpSslDir = mkdtempSync(join(tmpdir(), 'g8e-ssl-'));
        await seedCA(tmpSslDir);
        certService = new CertificateService({ 
            bootstrapService: { getSslDir: () => tmpSslDir },
            internalHttpClient: { request: vi.fn().mockResolvedValue({ success: true }) }
        });
    });

    afterEach(() => {
        rmSync(tmpSslDir, { recursive: true, force: true });
    });

    describe('initialize', () => {
        it('should load CA from SSL_DIR/ca/ and set caCert and caKey', async () => {
            await certService.initialize();

            expect(certService.initialized).toBe(true);
            expect(certService.caCert).toContain('-----BEGIN CERTIFICATE-----');
            expect(certService.caKey).toContain('-----BEGIN PRIVATE KEY-----');
        });

        it('should throw if CA is not found in ssl_dir/ca/', async () => {
            rmSync(join(tmpSslDir, 'ca'), { recursive: true, force: true });
            await expect(certService.initialize()).rejects.toThrow('CA certificate not found');
        });

        it('should not re-initialize if already initialized', async () => {
            await certService.initialize();
            const caCert = certService.caCert;

            await certService.initialize();

            expect(certService.caCert).toBe(caCert);
        });

        it('should track revoked serials in memory', async () => {
            await certService.initialize();
            const serial = uuidv4().replace(/-/g, '').toUpperCase();
            await certService.revokeCertificate(serial, 'test', `op_${uuidv4()}`);

            expect(certService.isRevoked(serial)).toBe(true);
        });
    });

    describe('generateOperatorCertificate', () => {
        beforeEach(async () => {
            await certService.initialize();
        });

        it('should generate a certificate with correct Operator ID in subject', async () => {
            const operatorId = `op_test_${uuidv4()}`;
            const userId = `user_test_${uuidv4()}`;
            const orgId = `org_test_${uuidv4()}`;

            const result = await certService.generateOperatorCertificate(operatorId, userId, orgId);

            expect(result).toHaveProperty('cert');
            expect(result).toHaveProperty('key');
            expect(result).toHaveProperty('serial');
            expect(result).toHaveProperty('notBefore');
            expect(result).toHaveProperty('notAfter');
            expect(result).toHaveProperty('subject');
            expect(result.subject.CN).toBe(operatorId);
            expect(result.subject.OU).toBe(userId);
            expect(result.subject.O).toBe('g8e Operator');
        });

        it('should generate unique serial numbers for each certificate', async () => {
            const cert1 = await certService.generateOperatorCertificate(
                `op_test_${uuidv4()}`,
                `user_test_${uuidv4()}`,
                `org_test_${uuidv4()}`
            );
            const cert2 = await certService.generateOperatorCertificate(
                `op_test_${uuidv4()}`,
                `user_test_${uuidv4()}`,
                `org_test_${uuidv4()}`
            );

            expect(cert1.serial).not.toBe(cert2.serial);
            expect(cert1.serial.length).toBe(32);
            expect(cert2.serial.length).toBe(32);
        });

        it('should set correct validity period (365 days)', async () => {
            const result = await certService.generateOperatorCertificate(
                `op_test_${uuidv4()}`,
                `user_test_${uuidv4()}`,
                `org_test_${uuidv4()}`
            );

            const notBefore = new Date(result.notBefore);
            const notAfter = new Date(result.notAfter);
            const diffDays = (notAfter - notBefore) / (1000 * 60 * 60 * 24);

            expect(diffDays).toBeCloseTo(365, 0);
        });

        it('should generate valid PEM-formatted certificate and key', async () => {
            const result = await certService.generateOperatorCertificate(
                `op_test_${uuidv4()}`,
                `user_test_${uuidv4()}`,
                `org_test_${uuidv4()}`
            );

            expect(result.cert).toContain('-----BEGIN CERTIFICATE-----');
            expect(result.cert).toContain('-----END CERTIFICATE-----');
            expect(result.key).toContain('-----BEGIN');
            expect(result.key).toContain('-----END');
        });

        it('should generate certificate signed by the CA', async () => {
            const result = await certService.generateOperatorCertificate(
                `op_test_${uuidv4()}`,
                `user_test_${uuidv4()}`,
                `org_test_${uuidv4()}`
            );

            const cert = new x509.X509Certificate(result.cert);
            const caCert = new x509.X509Certificate(certService.caCert);
            const verified = await cert.verify({ publicKey: await caCert.publicKey.export() });
            expect(verified).toBe(true);
        });

        it('should auto-initialize if not already initialized', async () => {
            const uninitializedService = new CertificateService({ 
                bootstrapService: { getSslDir: () => tmpSslDir },
                internalHttpClient: { request: vi.fn().mockResolvedValue({ success: true }) }
            });

            const result = await uninitializedService.generateOperatorCertificate(
                `op_test_${uuidv4()}`,
                `user_test_${uuidv4()}`,
                `org_test_${uuidv4()}`
            );

            expect(result).toHaveProperty('cert');
            expect(uninitializedService.initialized).toBe(true);
        });

        it('should generate a valid certificate when CA key is in SEC1 (EC PRIVATE KEY) format', async () => {
            const sec1SslDir = mkdtempSync(join(tmpdir(), 'g8e-ssl-sec1-'));
            try {
                await seedCAWithLegacyEcKey(sec1SslDir);
                const service = new CertificateService({ 
                    bootstrapService: { getSslDir: () => sec1SslDir },
                    internalHttpClient: { request: vi.fn().mockResolvedValue({ success: true }) }
                });
                const result = await service.generateOperatorCertificate(
                    `op_test_${uuidv4()}`,
                    `user_test_${uuidv4()}`,
                    `org_test_${uuidv4()}`
                );
                expect(result).toHaveProperty('cert');
                expect(result.cert).toContain('-----BEGIN CERTIFICATE-----');
                expect(result).toHaveProperty('serial');
            } finally {
                rmSync(sec1SslDir, { recursive: true, force: true });
            }
        });
    });

    describe('revokeCertificate', () => {
        let testSerial;

        beforeEach(async () => {
            await certService.initialize();
            testSerial = uuidv4().replace(/-/g, '').toUpperCase();
        });

        it('should add serial to in-memory revoked set immediately', async () => {
            await certService.revokeCertificate(testSerial, 'api_key_refresh', `op_test_${uuidv4()}`);

            expect(certService.isRevoked(testSerial)).toBe(true);
        });

        it('should reflect revoked serial in getCRL()', async () => {
            await certService.revokeCertificate(testSerial, 'operator_deleted', `op_test_${uuidv4()}`);

            const crl = certService.getCRL();
            const entry = crl.revoked_certificates.find(e => e.serial === testSerial);
            expect(entry).toBeDefined();
        });
    });

    describe('isRevoked', () => {
        beforeEach(async () => {
            await certService.initialize();
        });

        it('should return true for revoked certificates (synchronous)', async () => {
            const serial = uuidv4().replace(/-/g, '').toUpperCase();
            await certService.revokeCertificate(serial, 'test', `op_test_${uuidv4()}`);

            expect(certService.isRevoked(serial)).toBe(true);
        });

        it('should return false for non-revoked certificates (synchronous)', async () => {
            const serial = uuidv4().replace(/-/g, '').toUpperCase();

            expect(certService.isRevoked(serial)).toBe(false);
        });
    });

    describe('getCRL', () => {
        beforeEach(async () => {
            await certService.initialize();
        });

        it('should return CRL data structure', () => {
            const crl = certService.getCRL();

            expect(crl).toHaveProperty('version');
            expect(crl).toHaveProperty('issuer');
            expect(crl).toHaveProperty('revoked_certificates');
            expect(crl.issuer).toBe('g8e Operator CA');
            expect(Array.isArray(crl.revoked_certificates)).toBe(true);
        });

        it('should return consistent data on multiple calls', () => {
            const crl1 = certService.getCRL();
            const crl2 = certService.getCRL();

            expect(crl1.version).toBe(crl2.version);
            expect(crl1.issuer).toBe(crl2.issuer);
        });
    });

    describe('getCACertificate', () => {
        beforeEach(async () => {
            await certService.initialize();
        });

        it('should return CA certificate in PEM format', async () => {
            const caCert = await certService.getCACertificate();

            expect(caCert).toContain('-----BEGIN CERTIFICATE-----');
            expect(caCert).toContain('-----END CERTIFICATE-----');
        });

        it('should return same certificate on multiple calls', async () => {
            const caCert1 = await certService.getCACertificate();
            const caCert2 = await certService.getCACertificate();

            expect(caCert1).toBe(caCert2);
        });
    });

    describe('extractSerialFromCert', () => {
        it('should return null for invalid certificate', () => {
            const serial = certService.extractSerialFromCert('not-a-certificate');
            expect(serial).toBeNull();
        });

        it('should return null for empty input', () => {
            const serial = certService.extractSerialFromCert('');
            expect(serial).toBeNull();
        });
    });

    describe('extractOperatorIdFromCert', () => {
        it('should return null for invalid certificate', () => {
            const operatorId = certService.extractOperatorIdFromCert('not-a-certificate');
            expect(operatorId).toBeNull();
        });

        it('should return null for empty input', () => {
            const operatorId = certService.extractOperatorIdFromCert('');
            expect(operatorId).toBeNull();
        });
    });

});
