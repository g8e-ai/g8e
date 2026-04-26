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

/**
 * Certificate Service - Per-Operator mTLS Certificate Management
 *
 * CA cert and key are read from /g8es/ssl/ca/ (mounted read-only from g8es).
 * Per-operator client certificates are signed by the CA using Node.js crypto.
 * CRL is maintained in-memory; revoked serials are tracked in Operator documents.
 *
 * Certificate Trust Model:
 *   g8e Operator CA (ECDSA P-384, 10-year validity)
 *   +-- Client Certificate (ECDSA P-384, per-operator, 365-day validity)
 *       +-- Used by: Individual g8eo Operator instances
 *       +-- CN contains operator_id for identification
 */

import { getInternalHttpClient } from '../clients/internal_http_client.js';
import { logger } from '../../utils/logger.js';
import { now, addSeconds } from '../../models/base.js';
import { GeneratedCertificate } from '../../models/operator_model.js';
import { CLIENT_CERT_VALIDITY_DAYS, DEFAULT_SSL_DIR, CERT_SUBJECT_ORG, CERT_SUBJECT_COUNTRY, CRL_ISSUER } from '../../constants/service_config.js';
import { G8eHttpContext } from '../../models/request_models.js';
import * as x509 from '@peculiar/x509';
import crypto from 'crypto';
import { join } from 'path';
import { readFileSync, existsSync } from 'fs';

x509.cryptoProvider.set(crypto.webcrypto);

class CertificateService {
    /**
     * @param {Object} options
     * @param {Object} options.bootstrapService - BootstrapService instance (for ssl_dir)
     */
    constructor({ bootstrapService } = {}) {
        if (!bootstrapService) throw new Error('CertificateService requires bootstrapService');
        this.initialized = false;
        this.caCert = null;
        this.caKey = null;
        this.sslDir = bootstrapService.getSslDir() || DEFAULT_SSL_DIR;
        this._revokedSerials = new Set();
    }

    async initialize() {
        if (this.initialized) {
            return;
        }

        // Try standard locations for CA cert and key
        const paths = [
            { cert: join(this.sslDir, 'ca.crt'), key: join(this.sslDir, 'ca.key') },
            { cert: join(this.sslDir, 'ca', 'ca.crt'), key: join(this.sslDir, 'ca', 'ca.key') }
        ];

        let foundCert = null;
        let foundKey = null;

        for (const p of paths) {
            if (existsSync(p.cert) && existsSync(p.key)) {
                foundCert = p.cert;
                foundKey = p.key;
                break;
            }
        }

        if (foundCert && foundKey) {
            this.caCert = readFileSync(foundCert, 'utf8');
            this.caKey = readFileSync(foundKey, 'utf8');
            logger.info('[CERT-SERVICE] CA certificate loaded', { path: foundCert });
        } else {
            logger.error('[CERT-SERVICE] CA certificate or key not found', {
                sslDir: this.sslDir,
                checked_paths: paths.map(p => p.cert)
            });
            throw new Error('CA certificate not found. Run scripts/security/manage-ssl.sh generate to generate certificates.');
        }

        this.initialized = true;
        logger.info('[CERT-SERVICE] Certificate service initialized', {
            ca_loaded: !!this.caCert
        });
    }

    async generateOperatorCertificate(operatorId, userId, organizationId) {
        if (!this.initialized) {
            await this.initialize();
        }

        logger.info('[CERT-SERVICE] Generating per-operator certificate', {
            operator_id: operatorId,
            user_id: userId
        });

        try {
            const { privateKey, publicKey } = crypto.generateKeyPairSync('ec', {
                namedCurve: 'P-384',
                publicKeyEncoding: { type: 'spki', format: 'pem' },
                privateKeyEncoding: { type: 'pkcs8', format: 'pem' }
            });

            const serial = crypto.randomBytes(16).toString('hex').toUpperCase();

            const notBefore = now();
            const notAfter = addSeconds(notBefore, CLIENT_CERT_VALIDITY_DAYS * 24 * 60 * 60);

            const subject = {
                CN: operatorId,
                OU: userId,
                O: CERT_SUBJECT_ORG,
                C: CERT_SUBJECT_COUNTRY
            };

            const cert = await this._signCertificate(publicKey, subject, serial, notBefore, notAfter);

            logger.info('[CERT-SERVICE] Operator certificate generated', {
                operator_id: operatorId,
                serial: serial.substring(0, 16) + '...',
                not_before: notBefore,
                not_after: notAfter
            });

            return new GeneratedCertificate({
                cert,
                key: privateKey,
                serial,
                not_before: notBefore,
                not_after: notAfter,
                subject,
            }).forWire();
        } catch (error) {
            logger.error('[CERT-SERVICE] Failed to generate Operator certificate', {
                operator_id: operatorId,
                error: error.message
            });
            throw error;
        }
    }

    async revokeCertificate(serial, reason, operatorId, actorContext) {
        logger.info('[CERT-SERVICE] Relaying certificate revocation to g8ee', {
            serial: serial.substring(0, 16) + '...',
            reason,
            operator_id: operatorId
        });

        try {
            // Authority: g8ee (Engine) owns CRL management
            const internalHttpClient = getInternalHttpClient();
            
            // Ensure we have a valid G8eHttpContext for the relay
            let g8eContext = actorContext;
            if (!g8eContext || !(g8eContext instanceof G8eHttpContext)) {
                g8eContext = new G8eHttpContext({
                    source_component: 'g8ed',
                    user_id: 'system', // Fallback if no actor context provided
                    organization_id: 'system'
                });
            }

            const response = await internalHttpClient.request('g8ee', '/api/internal/auth/revoke-cert', {
                method: 'POST',
                body: { serial, reason, operator_id: operatorId },
                g8eContext
            });

            if (response.success) {
                logger.info('[CERT-SERVICE] Certificate revocation relayed successfully', { serial });
                return true;
            } else {
                const error = new Error(response.error || 'Failed to revoke certificate via g8ee');
                error.cause = response;
                throw error;
            }
        } catch (error) {
            logger.error('[CERT-SERVICE] Failed to relay certificate revocation', {
                serial: serial.substring(0, 16) + '...',
                error: error.message
            });
            throw error;
        }
    }

    isRevoked(serial) {
        // Authority: g8ee. g8ed should not maintain a shadow CRL.
        // For performance, this would ideally hit a local TTL cache.
        // For now, we log and return false to force the transition to g8ee authority.
        logger.warn('[CERT-SERVICE] isRevoked called on g8ed (authority shifted to g8ee)', { serial });
        return false;
    }

    getCRL() {
        // Authority: g8ee. 
        logger.warn('[CERT-SERVICE] getCRL called on g8ed (authority shifted to g8ee)');
        return {
            version: 1,
            issuer: CRL_ISSUER,
            revoked_certificates: [],
        };
    }

    async getCACertificate() {
        return this.caCert;
    }

    extractSerialFromCert(certPem) {
        try {
            const cert = new crypto.X509Certificate(certPem);
            return cert.serialNumber;
        } catch (error) {
            logger.warn('[CERT-SERVICE] Failed to extract serial from certificate', {
                error: error.message
            });
            return null;
        }
    }

    extractOperatorIdFromCert(certPem) {
        try {
            const cert = new crypto.X509Certificate(certPem);
            const subject = cert.subject;
            const cnMatch = subject.match(/CN=([^,\n]+)/);
            return cnMatch ? cnMatch[1] : null;
        } catch (error) {
            logger.warn('[CERT-SERVICE] Failed to extract Operator ID from certificate', {
                error: error.message
            });
            return null;
        }
    }

    // ========================================
    // Private Methods
    // ========================================

    async _signCertificate(publicKeyPem, subject, serial, notBefore, notAfter) {
        const publicKeyDer = this._pemToDer(publicKeyPem, 'PUBLIC KEY');
        const publicKey = await crypto.webcrypto.subtle.importKey(
            'spki',
            publicKeyDer,
            { name: 'ECDSA', namedCurve: 'P-384' },
            true,
            ['verify']
        );

        const normalizedCaKey = this._convertEcKeyToPkcs8(this.caKey);
        const caPrivateKeyDer = this._pemToDer(normalizedCaKey, 'PRIVATE KEY');
        const caPrivateKey = await crypto.webcrypto.subtle.importKey(
            'pkcs8',
            caPrivateKeyDer,
            { name: 'ECDSA', namedCurve: 'P-384' },
            false,
            ['sign']
        );

        const caCert = new x509.X509Certificate(this.caCert);

        const subjectStr = subject.OU
            ? `CN=${subject.CN}, OU=${subject.OU}, O=${subject.O}, C=${subject.C}`
            : `CN=${subject.CN}, O=${subject.O}, C=${subject.C}`;

        const cert = await x509.X509CertificateGenerator.create({
            serialNumber: serial,
            subject: subjectStr,
            issuer: caCert.subject,
            notBefore: notBefore,
            notAfter: notAfter,
            signingAlgorithm: { name: 'ECDSA', hash: 'SHA-384' },
            publicKey: publicKey,
            signingKey: caPrivateKey,
            extensions: [
                new x509.BasicConstraintsExtension(false, undefined, true),
                new x509.KeyUsagesExtension(
                    x509.KeyUsageFlags.digitalSignature | x509.KeyUsageFlags.keyEncipherment,
                    true
                ),
                new x509.ExtendedKeyUsageExtension(['1.3.6.1.5.5.7.3.2'], false)
            ]
        });

        return cert.toString('pem');
    }

    _pemToDer(pem, label) {
        const labelsToTry = label === 'PRIVATE KEY'
            ? ['PRIVATE KEY', 'EC PRIVATE KEY']
            : [label];

        for (const l of labelsToTry) {
            const regex = new RegExp(`-----BEGIN ${l}-----([\\s\\S]*?)-----END ${l}-----`);
            const match = pem.match(regex);
            if (match) {
                const base64 = match[1].replace(/\s/g, '');
                return Buffer.from(base64, 'base64');
            }
        }

        throw new Error(`Invalid PEM format for ${label}`);
    }

    _convertEcKeyToPkcs8(ecKeyPem) {
        try {
            const key = crypto.createPrivateKey({ key: ecKeyPem, format: 'pem' });
            return key.export({ type: 'pkcs8', format: 'pem' });
        } catch {
            return ecKeyPem;
        }
    }

}

export { CertificateService };
