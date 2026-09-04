# Compliance Alignment Report

**Document Version:** 2.1.3
**Last Updated:** 2026-09-02
**Platform:** g8e v2.1.3
**Maintained by:** Lateralus Labs, LLC.

---

## Executive Summary

This document provides a comprehensive alignment of the g8e platform's security controls and governance mechanisms against major industry compliance frameworks. g8e is designed as a zero-trust execution platform for agentic infrastructure, implementing fail-closed verification, cryptographic proof chains, and local-first data sovereignty.

This document maps g8e's security controls to external frameworks. The [Proof-Backed Compliance Evidence](./compliance-evidence.md) document describes the pipeline that binds the FedRAMP 20x and NIST SP 800-53 mappings to immutable, independently verifiable evidence and the roadmap for extending that binding to the remaining frameworks.

**Key Compliance Posture:**
- **SOC 2 Type II:** Strong alignment with Trust Services Criteria (Security, Availability, Confidentiality)
- **ISO 27001:2022:** Comprehensive coverage of Annex A controls
- **GDPR:** Data sovereignty by design with PII scrubbing and local processing
- **HIPAA:** Security Rule alignment with audit trails and access controls
- **NIST SP 800-53 Rev 5.2.0:** Moderate-to-High baseline coverage
- **NIST SP 800-63B-4:** AAL2/AAL3 alignment with phishing-resistant authenticators
- **PCI DSS v4.0.1:** Relevant controls for cardholder data environments
- **FedRAMP 20x (CR26):** Machine-readable Key Security Indicators (KSIs), protocol-owned control catalogs, persisted demo evidence verification, and COSAiS overlay alignment
- **NSA Zero Trust Implementation Guidelines (ZIG):** Strong alignment with Discovery, Phase One, and Phase Two activities

---

## 1. SOC 2 Type II Alignment

### Trust Services Criteria Mapping

#### TSC - Security (CC)

| Control ID | Control Description | g8e Implementation | Evidence Location |
|------------|---------------------|-------------------|-------------------|
| **CC1.1** | Logical and physical access controls | mTLS with SPIFFE workload identity, WebAuthn L3 Notary | `internal/services/gateway/pki_controller.go`, `docs/architecture/auth.md` |
| **CC1.2** | Logical access security software | 5-layer verification pipeline (L1-L5) | `internal/services/governance/l4_warden.go` |
| **CC1.3** | Logical access to system components | Role-based session isolation (operator_session_id, cli_session_id, web_session_id) | `protocol/models/conversation.json` |
| **CC1.4** | Logical access to stored data | Encrypted audit store with mandatory encryption at rest | `internal/services/storage/audit_store.go` |
| **CC1.5** | Authentication of external users | Two-layer L3 Notary: passkey authorization (WebAuthn/FIDO2) plus mTLS transport authentication for CLI callers | `internal/services/governance/l3_notary.go` |
| **CC1.6** | Identification and authentication | SPIFFE URI SAN binding in certificates, Ed25519 signature verification | `protocol/workload_identity.go` |
| **CC1.7** | Logical access for support personnel | No standing privileges, JIT provisioning via CSR enrollment | `internal/services/gateway/pki_controller.go` |
| **CC1.8** | Management of access security | Certificate revocation with database-backed denylist and CRL | `internal/services/gateway/pki_controller.go` |
| **CC1.9** | Data transfer security | mTLS for all platform communication, outbound-only operator connections | `docs/architecture/auth.md` |
| **CC1.10** | Security monitoring | Complete signed receipts in the SQLite audit store, signed SQLite commitment chain, and git-backed file-mutation snapshots | `internal/services/storage/audit_store.go`, `internal/services/storage/commitment_ledger.go`, `internal/services/storage/ledger.go` |
| **CC1.11** | Data disposal | Configurable retention policies (default 90 days), automated pruning | `internal/services/storage/audit_store.go` |
| **CC1.12** | Security incident response | Coordinated disclosure policy via `security@lateraluslabs.com` | `.github/SECURITY.md` |

#### TSC - Availability (A)

| Control ID | Control Description | g8e Implementation | Evidence Location |
|------------|---------------------|-------------------|-------------------|
| **A1.1** | Processing and recovery | Git-backed ledger for state recovery, replay protection via nonce | `internal/services/storage/ledger.go` |
| **A1.2** | Availability monitoring | Health checks via audit store status, session tracking | `internal/services/storage/audit_store.go` |
| **A1.3** | Data backup | Local-first storage with git commits per mutation | `internal/services/storage/ledger.go` |

#### TSC - Confidentiality (C)

| Control ID | Control Description | g8e Implementation | Evidence Location |
|------------|---------------------|-------------------|-------------------|
| **C1.1** | Confidentiality of information at rest | Mandatory encryption vault for audit content fields | `internal/services/storage/audit_store.go` |
| **C1.2** | Confidentiality of information in transit | mTLS with TLS 1.3 for all platform communication | `docs/architecture/auth.md` |
| **C1.3** | Confidentiality of information during processing | Sovereign Execution Boundary with PII/secret scrubbing before cloud transmission | `internal/services/scrubbing/boundary.go` |
| **C1.4** | Avoidance of unauthorized disclosure | Deterministic rehydration only at execution boundary | `internal/services/scrubbing/boundary.go` |

---

## 2. ISO 27001:2022 Alignment

ISO/IEC 27001:2022 (with Amendment 1:2024) is the current version of the standard. Amendment 1:2024, published February 2024, adds climate change considerations to Clauses 4.1 and 4.2. The transition deadline for organizations certified to ISO 27001:2013 was October 31, 2025; all 2013 certifications are now expired.

### Annex A Control Mapping

#### A.5 Organizational Security Policies
- **A.5.1 Policies for information security:** Security policy documented in `.github/SECURITY.md`
- **A.5.2 Review of the policies:** Versioned documentation with changelog

#### A.6 Organization of People, Roles, and Responsibilities
- **A.6.1 Information security roles and responsibilities:** Defined workload identities via SPIFFE
- **A.6.2 Segregation of duties:** 5-layer verification prevents single-point bypass

#### A.7 Human Resource Security
- **A.7.1 Screening:** Background checks for security team (internal process)
- **A.7.2 Terms and conditions of employment:** Confidentiality agreements (internal process)

#### A.8 Asset Management
- **A.8.1 Inventory of assets:** Operator session tracking, ledger state tracking
- **A.8.2 Acceptable use policy:** Documented in security policy
- **A.8.3 Information classification:** Sovereign Execution Boundary classifies sensitive data
- **A.8.5 Management of removable media:** No external media handling by platform

#### A.9 Access Control
- **A.9.1 Access control policy:** Role-based session isolation
- **A.9.2 Access to networks and network services:** mTLS enforcement
- **A.9.3 User registration and de-registration:** CSR-based enrollment with revocation
- **A.9.4 User access provisioning:** JIT provisioning via certificate issuance
- **A.9.5 Management of access rights:** Certificate validity periods (7 days for leaves, 90 days for serving)
- **A.9.6 Authentication of information system users:** WebAuthn/FIDO2, mTLS certificates
- **A.9.7 Access to system functions and data:** 5-layer verification pipeline
- **A.9.8 Management of privileged access rights:** No standing privileges, per-transaction authorization
- **A.9.9 Secret authentication information:** Encrypted vault for secrets storage
- **A.9.10 Information disclosure restrictions:** Sensitive data scrubbing before external transmission

#### A.10 Cryptography
- **A.10.1 Cryptographic controls:** ECDSA P-256 for certificates, Ed25519 for signatures, SHA-256 for hashing
- **A.10.2 Key management:** PKI hierarchy with root/intermediate CA separation

#### A.11 Physical and Environmental Security
- **A.11.1 Physical security perimeters:** Platform runs on customer infrastructure (customer responsibility)
- **A.11.2 Equipment:** Customer-provided hardware (customer responsibility)

#### A.12 Operations Security
- **A.12.1 Operational procedures:** Documented in architecture docs
- **A.12.2 Protection from malware:** L1 Doctrine with MITRE ATT&CK pattern detection
- **A.12.3 Backup:** Git-backed ledger with per-mutation commits
- **A.12.4 Logging and monitoring:** Audit store with signed receipts
- **A.12.5 Log information protection:** Mandatory encryption at rest
- **A.12.6 Logging synchronization:** Local-first storage, no external dependency
- **A.12.7 Information leak prevention:** Sovereign Execution Boundary scrubbing
- **A.12.8 Information deletion:** Configurable retention policies
- **A.12.9 Information backup:** Git ledger provides versioned backup

#### A.13 Communications Security
- **A.13.1 Network security controls:** mTLS with TLS 1.3
- **A.13.2 Security of information in transit:** mTLS for all platform communication
- **A.13.3 Information transfer policies:** Outbound-only operator connections

#### A.14 System Acquisition, Development, and Maintenance
- **A.14.1 Security requirements:** Fail-closed verification by design
- **A.14.2 Security in development:** Security testing in CI/CD (`.github/workflows/build-and-test.yml`)
- **A.14.3 Test data:** Separate test fixtures in `test/fixtures/`
- **A.14.4 Change management:** Versioned releases with changelog
- **A.14.5 Capacity management:** Configurable database size limits
- **A.14.6 Change control:** Git-based version control
- **A.14.7 Information on vulnerabilities:** Coordinated disclosure policy
- **A.14.8 Audit logging:** Comprehensive audit store
- **A.14.9 System documentation:** Architecture docs, protocol specs

#### A.15 Supplier Relationships
- **A.15.1 Information security in supplier relationships:** No third-party SaaS dependencies for core platform
- **A.15.2 Supply chain security:** Self-hosted, air-gap capable

#### A.16 Information Security Incident Management
- **A.16.1 Management of information security incidents:** Coordinated disclosure policy
- **A.16.2 Reporting information security events:** `security@lateraluslabs.com`
- **A.16.3 Management of information security weaknesses:** Vulnerability reporting process

#### A.17 Information Security Compliance
- **A.17.1 Identification of applicable laws and requirements:** This compliance alignment document
- **A.17.2 Intellectual property rights:** Business Source License 1.1 (BSL 1.1), converts to Apache 2.0 on 2030-08-18
- **A.17.3 Protection of records:** Audit store with git ledger
- **A.17.4 Privacy and protection of PII:** Sovereign Execution Boundary with PII scrubbing
- **A.17.5 Independent review:** Third-party security assessment (planned)

---

## 3. GDPR Alignment

### Data Protection Principles

| Principle | g8e Implementation | Evidence |
|-----------|-------------------|----------|
| **Lawfulness, fairness, transparency** | Local-first processing, user-controlled data | `internal/services/scrubbing/boundary.go` |
| **Purpose limitation** | Sensitive data scrubbing prevents data leakage to unintended systems | `internal/services/scrubbing/boundary.go` |
| **Data minimization** | Scrubbing removes PII before cloud transmission | `internal/services/scrubbing/boundary.go` |
| **Accuracy** | Immutable git-backed ledger with state roots | `internal/services/storage/ledger.go` |
| **Storage limitation** | Configurable retention policies (default 90 days) | `internal/services/storage/audit_store.go` |
| **Integrity and confidentiality** | Encryption at rest, mTLS in transit, access controls | `internal/services/storage/audit_store.go`, `docs/architecture/auth.md` |

### GDPR Rights Support

| Right | g8e Capability | Implementation |
|-------|----------------|----------------|
| **Right to access** | Local audit store export | User can query local SQLite database |
| **Right to rectification** | Git ledger allows state rollback | `internal/services/storage/ledger.go` |
| **Right to erasure** | Configurable retention and pruning | `internal/services/storage/audit_store.go` |
| **Right to data portability** | SQLite export capability | User controls local data directory |
| **Right to object** | Local-first architecture gives user control | All data stored on user's infrastructure |

### Data Processing Roles

- **Data Controller:** The organization deploying g8e (customer)
- **Data Processor:** g8e platform (local execution boundary)
- **Data Sovereignty:** All processing occurs on customer infrastructure; platform vendor acts as stateless relay

---

## 4. HIPAA Alignment

### Security Rule (45 CFR Part 164 Subpart C)

#### Administrative Safeguards

| Standard | Implementation | Evidence |
|----------|----------------|----------|
| **Security Management Process** | 5-layer verification, L1 Doctrine with threat detection | `internal/services/governance/l1_doctrine.go` |
| **Assigned Security Responsibility** | Defined roles via SPIFFE workload identity | `protocol/workload_identity.go` |
| **Workforce Security** | JIT provisioning, no standing privileges | `internal/services/gateway/pki_controller.go` |
| **Information Access Management** | Session-based access control, mTLS enforcement | `docs/architecture/auth.md` |
| **Security Awareness and Training** | Documentation for secure deployment | `docs/guides/` |
| **Security Incident Procedures** | Coordinated disclosure policy | `.github/SECURITY.md` |
| **Contingency Plan** | Git-backed ledger for recovery | `internal/services/storage/ledger.go` |
| **Evaluation** | Automated testing in CI/CD | `.github/workflows/build-and-test.yml` |

#### Physical Safeguards

| Standard | Implementation | Evidence |
|----------|----------------|----------|
| **Facility Access Controls** | Customer-controlled infrastructure | Customer responsibility |
| **Workstation Use** | No standing privileges, per-transaction authorization | Platform design |
| **Workstation Security** | mTLS, certificate-based authentication | `docs/architecture/auth.md` |

#### Technical Safeguards

| Standard | Implementation | Evidence |
|----------|----------------|----------|
| **Access Control** | mTLS with SPIFFE identity, session isolation | `docs/architecture/auth.md` |
| **Audit Controls** | Comprehensive audit store with signed receipts | `internal/services/storage/audit_store.go`, `internal/services/storage/ledger.go` |
| **Integrity Controls** | State Merkle roots, git-backed ledger | `internal/services/storage/ledger.go` |
| **Transmission Security** | mTLS with TLS 1.3 for all communication | `docs/architecture/auth.md` |

### PHI Handling

- **PHI Scrubbing:** Sovereign Execution Boundary scrubs PII/PHI patterns before cloud transmission
- **Local Processing:** All PHI processing occurs on customer infrastructure
- **Audit Trail:** Immutable audit logs track all PHI access

---

## 5. NIST SP 800-53 Rev 5.2.0 Alignment

NIST SP 800-53 Release 5.2.0 (August 27, 2025) adds three new controls: SA-15(13) (Logging Syntax), SA-24 (Design for Cyber Resiliency), and SI-02(07) (Root Cause Analysis). These controls address software update security and cyber resiliency by design, responding to Executive Order 14306.

### Moderate-to-High Baseline Controls

#### Access Control (AC)

| Control | g8e Implementation | Evidence |
|---------|-------------------|----------|
| **AC-1** | Access control policy | `.github/SECURITY.md` |
| **AC-2** | Account management | CSR-based enrollment, session tracking |
| **AC-3** | Access enforcement | 5-layer verification pipeline, PrivilegedRouteRegistry blocks app certificates from submitting governance envelopes | `internal/services/gateway/gateway_auth.go` |
| **AC-6** | Least privilege | JIT provisioning, self-dissolving execution capabilities, no standing privileges | `internal/services/governance/capability.go` |
| **AC-7** | Successful/failed access attempts | Audit store logging |
| **AC-8** | System use notification | Session identifiers in envelopes |
| **AC-11** | Session lock | Session-based isolation |
| **AC-12** | Session termination | Configurable session timeouts |
| **AC-14** | Permitted actions without identification | No auto-approval; all transactions require posture-appropriate verification |
| **AC-17** | Remote access | mTLS required for all remote connections |
| **AC-18** | Wireless access | Customer-controlled infrastructure |
| **AC-19** | Access control for mobile devices | Windows/macOS/Linux support parity |
| **AC-20** | Use of external information systems | Sensitive data scrubbing before external transmission |

#### Audit and Accountability (AU)

| Control | g8e Implementation | Evidence |
|---------|-------------------|----------|
| **AU-1** | Audit and accountability policy | `.github/SECURITY.md` |
| **AU-2** | Audit events | Comprehensive event logging in audit store |
| **AU-3** | Audit record content | Events include timestamp, user, action, result |
| **AU-4** | Audit storage retention | Configurable retention (default 90 days) |
| **AU-5** | Audit response to processing failures | Fail-closed: execution aborted if audit fails |
| **AU-6** | Audit review, analysis, and reporting | Query capabilities via SQLite |
| **AU-7** | Audit reduction and report generation | Truncation for large outputs |
| **AU-8** | Time stamps | All events include UTC timestamps |
| **AU-9** | Protection of audit information | Mandatory encryption at rest |
| **AU-10** | Non-repudiation | Signed ActionReceipts with Ed25519; evidence anchored to KSI-MLA-07 via the typed KSI model (see [FedRAMP 20x Alignment](#10-fedramp-20x-cr26-alignment)) |
| **AU-11** | Audit record retention | Configurable retention policies |
| **AU-12** | Audit generation | All mutations generate audit records |

#### System and Communications Protection (SC)

| Control | g8e Implementation | Evidence |
|---------|-------------------|----------|
| **SC-1** | System and communications protection policy | `.github/SECURITY.md` |
| **SC-7** | Boundary protection | Sovereign Execution Boundary |
| **SC-8** | Transmission confidentiality | mTLS with TLS 1.3 |
| **SC-9** | Transmission integrity | mTLS with certificate verification |
| **SC-10** | Network disconnect | Outbound-only operator connections |
| **SC-11** | Trusted path | mTLS with SPIFFE identity |
| **SC-12** | Cryptographic key establishment and management | PKI hierarchy with root/intermediate CA |
| **SC-13** | Use of cryptography | g8e uses the Go Cryptographic Module v1.0.0 (CMVP Cert #5247, CAVP A6650) in FIPS 140-3 approved mode. See [FIPS 140-3 Compliance](./fips140-3.md) for the validated boundary, OE matrix, and build/runtime activation details. |
| **SC-17** | Public key infrastructure certificates | Certificate issuance, revocation, CRL |
| **SC-18** | Mobile code | Single binary, no runtime dependencies (build-time only) |
| **SC-20** | Secure name/address resolution | g8e.local canonical alias with internal translation |
| **SC-21** | Domain name services | Customer-controlled DNS |
| **SC-22** | Architecture and provisioning | Local-first, air-gap capable |
| **SC-23** | Session authenticity | Session-based isolation with cryptographic binding |

#### System and Information Integrity (SI)

| Control | g8e Implementation | Evidence |
|---------|-------------------|----------|
| **SI-1** | System and information integrity policy | `.github/SECURITY.md` |
| **SI-2** | Flaw remediation | Coordinated disclosure, automated dependency scanning |
| **SI-3** | Malicious code protection | L1 Doctrine with MITRE ATT&CK detection |
| **SI-4** | System monitoring | Audit store, signed receipts |
| **SI-5** | Security alerts | Session-based event routing |
| **SI-6** | Integrity verification | State Merkle roots, hash-based transaction binding |
| **SI-7** | Software, firmware, and information integrity | Git-backed ledger with per-mutation commits |
| **SI-8** | Spam protection | Not applicable (infrastructure platform) |
| **SI-10** | Information input validation | L1 Doctrine with input validation framework |
| **SI-11** | Error handling | Fail-closed verification, error categorization |
| **SI-12** | Information output handling | Sensitive data scrubbing before output |
| **SI-13** | Predictable failure prevention | Fail-closed design |
| **SI-14** | Non-persistence | JIT credentials, self-dissolving capabilities, no standing privileges | `internal/services/governance/capability.go` |
| **SI-16** | Memory protection | Go memory safety |
| **SI-17** | Fail-safe procedures | Fail-closed verification pipeline |
| **SA-15(13)** | Logging syntax | Structured audit event format in audit store with typed event fields | `internal/services/storage/audit_store.go` |
| **SA-24** | Design for cyber resiliency | Fail-closed verification pipeline with state recovery via git-backed ledger | `internal/services/storage/ledger.go` |
| **SI-02(07)** | Root cause analysis | Coordinated disclosure policy with vulnerability tracking | `.github/SECURITY.md` |

---

## 6. PCI DSS v4.0.1 Alignment

PCI DSS v4.0.1 (published June 11, 2024) is the current active version. PCI DSS v4.0 was retired December 31, 2024. The 51 future-dated requirements introduced in v4.0 became effective March 31, 2025, including universal MFA, expanded encryption requirements, and continuous monitoring.

### Relevant Controls for Cardholder Data Environments

| Requirement | g8e Implementation | Evidence |
|-------------|-------------------|----------|
| **1.2.1** | Business need justification | Outbound-only operator connections |
| **1.2.3** | Secure configuration | Configurable governance postures (doctrine, consensus, ratify, notary); outbound mode defaults to notary |
| **1.3** | Secure data flows | mTLS for all platform communication |
| **2.1** | Change control processes | Git-based version control |
| **2.2** | Configuration standards | Documented in architecture docs |
| **2.3** | Encryption of non-console administrative access | mTLS for all access |
| **3.1** | Keep cardholder data storage to minimum | Sensitive data scrubbing removes PII |
| **3.2** | Protect stored cardholder data | Mandatory encryption at rest |
| **3.3** | Mask PAN when displayed | Sensitive data scrubbing patterns |
| **3.4** | Render PAN unreadable | Sensitive data scrubbing patterns |
| **3.5** | Protect cryptographic keys | PKI hierarchy with key separation |
| **4.1** | Use strong cryptography | ECDSA P-256, Ed25519, SHA-256 |
| **4.2** | Secure cryptographic key distribution | mTLS for certificate distribution |
| **6.2** | Develop secure software | Security testing in CI/CD |
| **7.1** | Limit access to system components | Session-based access control |
| **7.2** | Establish access control system | mTLS with SPIFFE identity |
| **7.3** | Access to system components | 5-layer verification |
| **8.1** | Identify and authenticate access to system components | WebAuthn/FIDO2, mTLS certificates |
| **8.2** | Authentication factors | Hardware-bound WebAuthn, certificate-based mTLS |
| **8.3** | Secure authentication processes | Fail-closed verification |
| **8.4** | Multi-factor authentication | WebAuthn provides MFA |
| **8.5** | Secure authentication for non-console access | mTLS required |
| **9.1** | Use effective logging | Comprehensive audit store |
| **9.2** | Protect audit trails | Mandatory encryption at rest |
| **9.3** | Secure audit trails | Signed ActionReceipts |
| **9.4** | Review audit trails | Query capabilities via SQLite |
| **10.1** | Implement audit trails | All mutations generate audit records |
| **10.2** | Audit trail automation | Automatic logging in audit store |
| **10.3** | Audit trail retention | Configurable retention policies |
| **10.4** | Audit trail review | Query capabilities |
| **10.5** | Audit trail reconstruction | Git-backed ledger |
| **10.6** | Audit trail integrity | Signed receipts, state roots |
| **10.7** | Audit trail retention | Configurable retention |
| **11.1** | Implement security controls | 5-layer verification |
| **11.2** | Perform vulnerability scans | Automated dependency scanning |
| **11.3** | Implement penetration testing | Security assessment (planned) |
| **11.4** | Use intrusion detection systems | L1 Doctrine threat detection |
| **11.5** | Deploy intrusion prevention systems | L1 Doctrine hard gates |
| **11.6** | Verify security controls | Automated testing in CI/CD |
| **12.1** | Maintain security policy | `.github/SECURITY.md` |
| **12.2** | Risk assessment | This compliance alignment document |
| **12.3** | Security awareness | Documentation for secure deployment |
| **12.4** | Incident response | Coordinated disclosure policy |
| **12.5** | Vulnerability management | Coordinated disclosure, automated scanning |
| **12.6** | Security updates | Versioned releases with changelog |
| **12.7** | Security information | Architecture documentation |
| **12.8** | Security policies | `.github/SECURITY.md` |

---

## 7. NSA Zero Trust Implementation Guidelines (ZIG) Alignment

### Overview

The NSA Zero Trust Implementation Guidelines (ZIG) provide a five-phase approach to achieving Target-level Zero Trust maturity, aligned with the Department of War (DoW) Zero Trust Reference Architecture (v2.0, July 2022) and NIST guidance. The NSA released the Primer and Discovery Phase in January 2026, followed by Phase One and Phase Two in January 2026. An interactive ZIG webpage was launched in May 2026. g8e demonstrates strong alignment with the Discovery, Phase One, and Phase Two guidelines.

**ZIG Phases:**
- **Discovery Phase:** Identify critical data, applications, assets, and services (DAAS)
- **Phase One:** Establish secure foundation (36 activities, 30 capabilities)
- **Phase Two:** Integrate core zero trust tools (41 activities, 34 capabilities)
- **Phase Three:** (Planned) Advanced zero trust integration
- **Phase Four:** (Planned) Optimization and continuous improvement

### Discovery Phase Alignment

| ZIG Activity | Description | g8e Implementation | Evidence |
|--------------|-------------|-------------------|----------|
| **Identify Critical Data** | Catalog sensitive data and classification | Sovereign Execution Boundary with PII/secret detection | `internal/services/scrubbing/boundary.go` |
| **Identify Critical Applications** | Map application dependencies and data flows | GovernanceEnvelope protocol with transaction tracking | `protocol/proto/g8e/operator/v1/operator.proto` |
| **Identify Critical Assets** | Inventory infrastructure components | Operator session tracking, ledger state | `internal/services/storage/ledger.go` |
| **Identify Critical Services** | Catalog services and communication patterns | SPIFFE workload identity registry | `protocol/workload_identity.go` |
| **Map Data Flows** | Document data movement across boundaries | Sensitive data scrubbing before external transmission | `internal/services/scrubbing/boundary.go` |
| **Establish Trust Boundaries** | Define security perimeters | 5-layer verification pipeline (L1-L5) | `internal/services/governance/l4_warden.go` |

### Phase One Alignment: Secure Foundation

| ZIG Capability | Description | g8e Implementation | Evidence |
|----------------|-------------|-------------------|----------|
| **Multi-Factor Authentication** | Hardware-bound authentication for human users | WebAuthn/FIDO2 with L3 Notary | `internal/services/governance/l3_notary.go` |
| **Privileged Access Management** | JIT provisioning, no standing privileges | CSR-based enrollment with short-lived certificates | `internal/services/gateway/pki_controller.go` |
| **Federation and User Credentialing** | Cross-domain identity federation | SPIFFE URI SAN binding in certificates | `protocol/workload_identity.go` |
| **Device Trust Verification** | Cryptographic device identity | mTLS certificate verification | `docs/architecture/auth.md` |
| **Network Segmentation** | Micro-segmentation via mTLS | Per-connection mTLS enforcement | `docs/architecture/auth.md` |
| **Encryption in Transit** | TLS 1.3 for all communication | mTLS with TLS 1.3 | `docs/architecture/auth.md` |
| **Encryption at Rest** | Protect stored data | Mandatory encryption vault for audit content | `internal/services/storage/audit_store.go` |
| **Identity and Access Management** | Centralized identity control | SPIFFE workload identity system | `protocol/workload_identity.go` |
| **Continuous Monitoring** | Real-time security monitoring | Audit store with signed receipts | `internal/services/storage/audit_store.go` |
| **Incident Response** | Coordinated disclosure and response | Coordinated disclosure policy | `.github/SECURITY.md` |

### Phase Two Alignment: Core Zero Trust Tools

| ZIG Capability | Description | g8e Implementation | Evidence |
|----------------|-------------|-------------------|----------|
| **Policy Decision Point** | Centralized authorization decisions | Gateway as policy decision point | `docs/architecture/gateway.md` |
| **Policy Enforcement Point** | Distributed enforcement | L4 Warden pre-dispatch verification | `internal/services/governance/l4_warden.go` |
| **Dynamic Policy Evaluation** | Context-aware authorization | 5-layer verification with real-time checks | `internal/services/governance/l1_doctrine.go` |
| **Risk-Based Authentication** | Adaptive authentication based on risk | L3 Notary with hardware-bound auth | `internal/services/governance/l3_notary.go` |
| **Least Privilege Access** | Minimum necessary access | JIT provisioning, per-transaction authorization | `internal/services/gateway/pki_controller.go` |
| **Micro-Segmentation** | Fine-grained network segmentation | mTLS with workload identity | `docs/architecture/auth.md` |
| **Data Loss Prevention** | Prevent unauthorized data exfiltration | Sovereign Execution Boundary scrubbing | `internal/services/scrubbing/boundary.go` |
| **Threat Detection** | Identify malicious activity | L1 Doctrine with MITRE ATT&CK patterns | `internal/services/governance/l1_doctrine.go` |
| **Automated Response** | Automated containment of threats | Fail-closed verification pipeline | `internal/services/governance/l5_actuator.go` |
| **Audit Logging** | Comprehensive audit trail | Git-backed ledger | `internal/services/storage/ledger.go` |
| **Session Management** | Secure session handling | Session-based isolation (operator_session_id, cli_session_id, web_session_id) | `protocol/docs/spec.md` |
| **Certificate Management** | PKI lifecycle management | Root/intermediate CA hierarchy, CRL | `internal/services/gateway/pki_controller.go` |
| **Key Management** | Secure key storage and rotation | PKI hierarchy with key separation | `internal/services/gateway/pki_controller.go` |
| **API Security** | Secure API communication | mTLS for all platform APIs | `docs/architecture/auth.md` |
| **Supply Chain Security** | Verify software integrity | Git-based version control, signed releases | `protocol/proto/g8e/operator/v1/operator.proto` |

### ZIG Pillars Alignment

The ZIG framework organizes 152 activities from the DoW Zero Trust Strategy across seven pillars. g8e implements these pillars as follows:

| Pillar | ZIG Focus | g8e Implementation |
|--------|-----------|-------------------|
| **User** | Strong authentication, identity management | WebAuthn/FIDO2, SPIFFE workload identity, mTLS |
| **Device** | Device trust and posture | Certificate-based device identity, mTLS verification |
| **Application & Workload** | Application security, data protection | GovernanceEnvelope, Sovereign Execution Boundary |
| **Data** | Data classification, loss prevention | PII/secret scrubbing, encryption at rest |
| **Network & Environment** | Network segmentation, encryption | mTLS everywhere, micro-segmentation |
| **Automation & Orchestration** | Automated response, policy enforcement | Fail-closed verification pipeline, JIT capability minting |
| **Visibility & Analytics** | Continuous monitoring, audit logging | Audit store with signed receipts, git-backed ledger |

### NIST SP 800-207A Alignment

NIST SP 800-207A (September 2023) extends SP 800-207 with a zero trust architecture model for cloud-native applications in multi-cloud environments. It explicitly references SPIFFE as a platform for enforcing application-level policies based on service identities. g8e's SPIFFE workload identity system and mTLS enforcement align with this model.

### Gap Analysis: ZIG Phases Three and Four

| Phase | Status | Notes |
|-------|--------|-------|
| **Phase Three** | Partial alignment | Advanced threat hunting and automated response in development |
| **Phase Four** | Planned | Continuous optimization and maturity assessment planned for FY 2027 |

---

## 8. NIST SP 800-63B-4 Alignment

NIST SP 800-63B-4 (July 2025) supersedes SP 800-63B (2020) and defines technical requirements for three Authentication Assurance Levels (AALs). This revision introduces phishing-resistant authenticator requirements at AAL2, non-exportable key requirements at AAL3, normative guidance for syncable authenticators, and session monitoring (continuous authentication) guidelines.

### Authentication Assurance Level Mapping

| AAL | SP 800-63B-4 Requirement | g8e Implementation | Evidence |
|-----|---------------------------|-------------------|----------|
| **AAL2** | Two distinct authentication factors; phishing-resistant option required | WebAuthn/FIDO2 passkey (possession factor) plus mTLS client certificate (transport factor); WebAuthn is phishing-resistant by design | `internal/services/governance/l3_notary.go`, `docs/architecture/auth.md` |
| **AAL3** | Phishing-resistant authenticator with non-exportable key; two factors | Hardware-bound WebAuthn authenticator with non-exportable private key (platform authenticator or security key); mTLS certificate as second factor | `internal/services/governance/l3_notary.go`, `internal/services/gateway/pki_controller.go` |

### Key Requirement Alignment

| Requirement | g8e Implementation | Evidence |
|-------------|-------------------|----------|
| **Phishing-resistant authentication** | WebAuthn/FIDO2 uses public-key cryptography with origin binding, preventing credential phishing | `internal/services/governance/l3_notary.go` |
| **Non-exportable authenticator keys** | Platform authenticators (Windows Hello, Touch ID) and FIDO2 security keys store private keys in hardware-protected storage | `docs/architecture/auth.md` |
| **Multi-factor authentication** | Passkey authorization (L3 Notary) plus mTLS transport authentication provides two distinct factors | `internal/services/governance/l3_notary.go` |
| **Session monitoring** | Audit store tracks all session activity with signed receipts; session-based isolation with operator, CLI, and web session IDs | `internal/services/storage/audit_store.go` |
| **Reauthentication** | Per-transaction authorization via L3 Notary; no persistent sessions for mutations | `internal/services/governance/l3_notary.go` |
| **Authenticator lifecycle** | Passkey registration and revocation via g8e Console; certificate enrollment and revocation via PKI controller | `internal/services/gateway/pki_controller.go` |

---

## 9. Security Control Summary

### Cryptographic Controls

| Control | Algorithm | Purpose | Implementation |
|---------|-----------|---------|----------------|
| **Certificate Authority** | ECDSA P-256 | PKI hierarchy | `internal/services/gateway/pki_controller.go` |
| **Workload Identity** | SPIFFE URI SAN | Identity binding | `protocol/workload_identity.go` |
| **Transaction Signatures** | Ed25519 | L2 Consensus, L5 Actuator | `internal/services/consensus/service.go`, `internal/services/governance/l4_warden.go` |
| **Hash Functions** | SHA-256 | Transaction hash, state roots | `protocol/proto/g8e/operator/v1/operator.proto` |
| **Transport Security** | TLS 1.3 | mTLS for all communication | `docs/architecture/auth.md` |
| **Human Authentication** | WebAuthn/FIDO2 | L3 Notary | `internal/services/governance/l3_notary.go` |

### Access Control Mechanisms

| Mechanism | Type | Validity | Evidence |
|-----------|------|----------|----------|
| **Operator Certificate** | Leaf | 7 days | `docs/architecture/auth.md` |
| **CLI Certificate** | Leaf | 7 days | `docs/architecture/auth.md` |
| **Gateway Serving Certificate** | Serving | 90 days | `docs/architecture/auth.md` |
| **Gateway Peer Certificate** | Peer | 90 days | `docs/architecture/auth.md` |
| **Root CA** | Root | 3650 days | `docs/architecture/auth.md` |
| **Intermediate CAs** | Intermediate | 3650 days | `docs/architecture/auth.md` |

### Data Protection Mechanisms

| Mechanism | Scope | Implementation |
|-----------|-------|----------------|
| **Encryption at Rest** | Audit store content fields | `internal/services/storage/audit_store.go` |
| **Encryption in Transit** | All platform communication | mTLS with TLS 1.3 |
| **PII Scrubbing** | Outbound data | `internal/services/scrubbing/boundary.go` |
| **State Binding** | Transaction state | State Merkle roots |
| **Replay Protection** | Transaction nonces | `internal/services/governance/l4_warden.go` |
| **Audit Trail** | All mutations | `internal/services/storage/audit_store.go` |

---

## 10. FedRAMP 20x (CR26) Alignment

FedRAMP's Consolidated Rules for 2026 (CR26) introduce the 20x path, which replaces static System Security Plans with binary-resolution, machine-readable Key Security Indicators (KSIs) that regenerate on demand. g8e produces the raw evidence 20x consumes: signed `ActionReceipts` (Ed25519), a hash-chained git ledger, a SQLite audit vault, and the LFAA retrieval surface. The compliance package (`internal/services/compliance/`) provides typed KSI models, a binary KSI evaluator, historical snapshot retention, COSAiS overlay ingestion, protocol-owned assertion and framework catalogs, and independent verification of persisted demo evidence runs. The typed OSCAL renderer remains available to the package, but the CLI does not expose the superseded flat live-state export or the planned proof-backed report bundle generator. The [Proof-Backed Compliance Evidence](./compliance-evidence.md) document describes the assertion catalog, framework crosswalk, evidence-grade demo scenarios, and independent demo-run verification that bind the FedRAMP 20x and NIST SP 800-53 mappings to immutable evidence.

### Certification Classes

CR26 renames legacy impact levels into Certification Classes A through D for the 20x path:

| Class | Approximate legacy level | 20x status | Automation expectation per KSI |
|-------|--------------------------|-----------|--------------------------------|
| **Class A** | New, mature program entry | Finalized | MAY automate |
| **Class B** | Low | Finalized | SHOULD automate, at least 1 automated method |
| **Class C** | Moderate | Finalized (Phase 2 pilot) | MUST automate, at least 2 automated methods plus historical metrics |
| **Class D** | High | No 20x path (Phase 4, FY27 est.) | MUST automate, at least 4 automated methods (future) |

Class C is the realistic 20x target for g8e-backed offerings. g8e targets Class C as the design point and treats Class D as out-of-scope until FedRAMP opens Phase 4.

### Key Security Indicators (KSIs)

The typed KSI catalog (`docs/reference/ksi-catalog.json`) defines 31 KSIs across 10 categories (CED, CMT, CNA, IAM, INR, MLA, PIY, RCP, SVC, TPR), seeded from the CR26 reference and cross-checked against RFC-0006 and RFC-0014. Each KSI binds to one or more automated methods that derive binary status (satisfied, not_satisfied, not_applicable) from live g8e state. The KSI evaluator (`internal/services/compliance/ksi_evaluator.go`) enforces minimum method counts per certification class and fails closed if insufficient.

Machine-based resources validate at least every 7 days; non-machine resources at least every 3 months (PVA standard, RFC-0017). The evaluator checks staleness and fails closed for Class C if a KSI exceeds its validation cycle.

### Compliance evidence commands

The OSCAL model and renderer remain in `internal/services/compliance/oscal.go`, but the legacy flat live-state export command is removed because it does not produce content-addressed, scope-bound evidence. Proof-backed report bundle generation is not yet available. The protocol-owned compliance foundation instead defines digest-verified assertion, framework, crosswalk, and demo-scenario catalogs plus typed scope, evidence, assessment, manifest, and verification records.

CLI commands:
- `g8e compliance ksi --class C` evaluates KSIs and prints the result set as JSON.
- `g8e compliance ksi-history --ksi <id>` reads historical evaluation snapshots for a specific KSI.
- `g8e compliance overlay --overlay-dir <dir>` inspects and validates AI control overlay catalogs.
- `g8e compliance demo-run verify <run-id> [--project-root <dir>]` reads `.g8e/data/compliance/demo-evidence/<run-id>/`, verifies its canonical manifest, scenario results, provenance, content-addressed artifacts, signatures, protocol chains, state observations, healthcare threshold metrics, and directory integrity, then emits a canonical `ComplianceVerificationReport`. The verifier is read-only and exits nonzero when the report is invalid.

### Historical Metrics Retention

Class C requires historical metrics including KSI status over time. The KSI history store (`internal/services/compliance/ksi_history.go`) persists `KSIResultSet` snapshots to `.g8e/data/compliance/ksi-history/` via `RuntimeFileService` on each evaluation cycle. The `g8e compliance ksi-history --ksi <id>` CLI command reads the chronological series for a specific KSI. Snapshots are pruned after a 90-day retention period.

### COSAiS Overlay Alignment

NIST's Control Overlays for Securing AI Systems (COSAiS) provide AI-specific control overlays at concept-paper stage. The overlay loader (`internal/services/compliance/overlay_loader.go`) ingests overlay JSON from a configurable directory (`--overlay-dir`), validates structure, detects duplicate IDs, and checks that KSI overlay references resolve to catalog entries. A placeholder catalog (`docs/reference/cosais-overlays.json`) seeds 5 overlay entries from the COSAiS concept paper use cases, all with `status: draft` until NIST finalizes.

### Doctrine and Detector KSI Linkage

L1 doctrine entries carry typed `ksi_ids`, `control_ids`, and `overlay_ids` fields that project into emitted `ThreatSignal` values. This allows each blocked or detected event to carry its KSI and control projection, providing traceability from governance enforcement to compliance evidence. The FedRAMP demo doctrine (`demos/fedramp/doctrine/fedramp_doctrine.json`) maps all 5 detectors to specific KSI IDs and NIST 800-53 controls.

### FedRAMP Demo KSI Mapping

The FedRAMP sovereign cloud governance demo maps doctrine detectors to typed KSI IDs:

| Doctrine detector | KSI IDs | Controls |
|-------------------|---------|----------|
| `fedramp-cr26-destruction` | KSI-MLA-07 | AU-2, AU-6 |
| `fedramp-ac2-unauthorized-destroy` | KSI-IAM-07 | AC-2 |
| `fedramp-si4-privilege-escalation` | KSI-IAM-05 | SI-4 |
| `fedramp-sc8-cross-domain-transfer` | KSI-SVC-03, KSI-CNA-01 | SC-8 |
| `fedramp-cm7-unauthorized-config` | KSI-CMT-01, KSI-SVC-04 | CM-7 |

See [FedRAMP Demo](../../demos/fedramp/README.md) for the full demo documentation.

### References

- [FedRAMP 20x](https://www.fedramp.gov/20x/)
- [FedRAMP CR26 Certification](https://preview.fedramp.gov/2026/providers/20x/rules/fedramp-certification/)
- [FedRAMP 20x Key Security Indicators](https://preview.fedramp.gov/2026/reference/20x/c/key-security-indicators/)
- [RFC-0006 (Phase One KSIs)](https://www.fedramp.gov/rfcs/0006/)
- [RFC-0014 (Phase Two KSIs)](https://www.fedramp.gov/rfcs/0014/)
- [RFC-0017 (Persistent Validation and Assessment)](https://www.fedramp.gov/rfcs/0017/)
- [NIST COSAiS Project](https://csrc.nist.gov/projects/cosais/)

---

## 11. Gap Analysis and Roadmap

### Current Strengths

- **Fail-closed verification pipeline** with 5-layer interlock
- **Cryptographic proof chains** with Ed25519 signatures
- **Local-first data sovereignty** with git-backed ledger
- **Comprehensive audit trail** with complete canonical signed receipts whose signatures bind deterministic governance-stage evidence
- **Durable-persistence proof** with a final Ed25519 attestation binding each receipt signature to its audit record and persistence timestamp
- **Signed commitment chain** with insertion-ordered atomic append and verification of signatures, structured columns, receipt cross-links, and file-mutation linkage
- **Cross-language offline verification** through protocol-owned Go/Python canonicalization vectors and Python receipt and persistence-attestation helpers
- **mTLS everywhere** with SPIFFE workload identity
- **Hardware-bound human authentication** via WebAuthn/FIDO2
- **PII/secret scrubbing** before cloud transmission
- **Certificate revocation** with CRL and database denylist
- **Air-gap capable** with no runtime dependencies (all dependencies resolved at build time)
- **g8e Console SPA** for browser-based passkey registration and transaction approval
- **PrivilegedRouteRegistry** blocking app certificates from governance envelope submission
- **JIT capability minting** with self-dissolving execution scopes
- **One-time enrollment tokens** replacing raw session identifiers in browser enrollment URLs with cryptographically random, 5-minute-TTL tokens consumed on first use
- **SSE-based L3 approval notifications** replacing polling with real-time event subscriptions for passkey approval delivery
- **Thread-safe dependency wiring** via atomic pointers for late-bound gateway dependencies, eliminating data races in concurrent request handling
- **Configurable CORS middleware** validating request origins against an allowlist for cross-origin browser access
- **75.9% test coverage** (75% threshold enforced in CI/CD) across gateway, governance, storage, and CLI subsystems
- **RuntimeFileService abstraction** (`internal/services/fs`) as canonical `.g8e/` file I/O layer, replacing direct `os.*` calls across gateway, keystore, PKI, and CLI subsystems
- **Unified Go module** merging the separate protocol module into the root module, eliminating version skew between protocol and platform
- **Cosign/sigstore artifact signing** providing cryptographic signatures for release binaries
- **Gitleaks secret scanning** in CI/CD preventing credential leakage in source code
- **Go-licenses license compliance** auditing transitive dependencies for license compatibility
- **Cross-OS CI matrix** (Ubuntu, macOS, Windows) ensuring platform parity
- **Python protocol test suite** validating constant accessors, generated protobuf messages, enum generation, model serialization, canonical receipt signatures, and persistence attestations
- **Python protocol conformance suite** enforcing parity between Go constants, Python runtime values, canonical JSON SSOT files, and shared cryptographic vectors
- **Interactive gateway onboarding wizard** (`g8e gw start --interactive`) guiding users through network, security, agent, and review configuration steps
- **Anduril Lattice gRPC adapter** with OAuth2 client credentials authentication, gRPC retry with status code classification, and heartbeat interval validation
- **MCP tool interception verification** (`--verify` flag) ensuring agent tool-disabling configurations are correctly applied before agent launch
- **MCP gateway output scrubbing** preventing sensitive data leakage in downstream MCP audit logs
- **Agent support** for 5 agents (Claude Code, Codex, Goose, Gemini CLI, Devin CLI) with per-agent MCP config wiring; 4 of 5 (claude, codex, goose, gemini) support native tool disabling for full governance interception
- **Governance interface extraction** separating verification concerns (replay store, signer store, consensus store, app policy store, state root provider) into dedicated files
- **Protocol wire-format documentation** for EventType, AgentMode, and SessionEventWire ensuring consumer compatibility
- **Cross-language governance hash parity** ensuring Go and Python produce identical transaction hash digests through aligned canonicalization logic
- **Enrollment token TOCTOU fix** with atomic conditional update preventing race condition exploitation during token consumption
- **SSE resilience improvements** with configurable heartbeat interval, multi-line SSE data parsing fix, and pubsub reconnect backoff reset
- **Governance JSON schema** providing canonical protocol validation for GovernanceEnvelope structures
- **Gateway HTTP handler decomposition** into SSEController, HealthController, and GovernanceController for single-responsibility routing
- **CanonicalDBService Stores struct extraction** enabling direct dependency injection of individual store services
- **L2 consensus interface decoupling** with L4 Warden depending on generic `L2ConsensusPolicyStore` instead of concrete store implementation
- **AuthController decomposition** into BootstrapController, EnrollmentTokenController, UserController, and SessionController for focused authentication workflows
- **PasskeyOrchestrator extraction** from PasskeyHandler for centralized passkey workflow management
- **L1 doctrine threat detector expansion** with credential leak and privilege escalation pattern detection
- **MCP gateway ThreatScanner interface** delegating threat scanning to a dedicated interface for modular security enforcement
- **L5 Actuator decomposition** with fail-closed payload rehydration at the execution boundary
- **MCP gateway AuditEventRecorder interface** for structured audit event recording in downstream MCP logs
- **Error sentinel migration** to `internal/constants` for consistent error handling across all subsystems
- **ScrubbingService constructor fail-closed** on token load failure, preventing startup with missing scrubbing rules
- **"Tribunal" to "Consensus" rename** across CLI flags, env vars, API paths, Go types, and documentation for naming consistency
- **DBController split** into DataController, AuditController, and SignerController for single-responsibility data management
- **L2 consensus logic extraction** from L4 Warden into dedicated `l2_consensus.go` for separation of concerns
- **Configurable doctrine directory** (`--doctrine-dir` CLI flag) for external doctrine rules and custom threat patterns
- **Gateway construction refactor** to single-phase construction with lazy forwarding adapters and per-controller dependency injection, eliminating late-bound dependency injection
- **Dead code removal** (107 unreachable functions removed from production and test code)
- **FedRAMP sovereign cloud governance demo** demonstrating compliance posture for federal workloads
- **FedRAMP 20x (CR26) alignment** with typed KSI evaluation, historical metrics retention, COSAiS overlay ingestion, protocol-owned compliance catalogs, persisted demo evidence, and fail-closed independent demo-run verification
- **EnrollmentCoordinator** replacing the scattered CLI enrollment transport (`BootstrapWithURL`/`CLIEnroll`/`ReEnroll`/`EnrollWithGateway`/`AutoRenewCertificate`) with a single state machine owning the enrollment lifecycle
- **Human-approved CLI recovery flow** for new CLIs against an existing gateway, using a one-time approval (browser Console SPA or mTLS-enrolled CLI via `g8e auth approve-recovery`) and opaque proof-of-possession token
- **mTLS-protected CLI certificate rotation endpoint** enabling in-band certificate renewal without re-enrollment
- **OS trust-store installation before browser launch** during `auth enroll user`, with a blocking browser-restart gate after trust-store changes
- **Posture-aware passkey enrollment** requiring passkeys for ratify and notary postures (optional for doctrine and consensus)
- **Audit store hard dependency** for the MCP gateway, making audit recording a construction-time requirement rather than a late-bound optional
- **Passkey proof verification decoupling** from the L3 notary interface, separating proof verification from notary authorization logic
- **Construction-phase consensus wiring** eliminating late-bound `**T` pointers and `atomic.Pointer` cells — consensus is injected directly at construction via `GatewayModeDeps`, with no `SetConsensusService` mutator
- **Typed audit response models** narrowing HTTP audit payload shapes to typed structs instead of raw maps
- **RootCACommonName constant centralization** extracting the root-CA subject common name into a single typed constant across gateway certificate generation, OS trust-store installation, and stale-anchor enumeration
- **Headless CLI enrollment** via `--headless` flag on `g8e auth enroll` enabling CLI-only identities (mTLS without browser or WebAuthn passkey ceremony) for automated CI and remote server environments
- **mTLS-based CLI recovery approval** via `g8e auth approve-recovery <token>` subcommand and `POST /api/v1/auth/cli/recovery/approve-cli` (`RouteAuthMTLS`) endpoint, allowing enrolled CLIs to approve or deny pending recovery requests over mTLS
- **Platform logging consolidation** under `internal/services/logging` with unified `g8e.log` file lifecycle, structured slog logging, and `RuntimeFileService` streaming append and read support (`OpenForAppend`/`OpenForRead`)
- **Pub/sub `PSUBSCRIBE` glob-pattern subscriptions** on the gateway broker with Redis-compatible channel matching and fail-closed topic ACLs confining wildcards to subscriber-owned operator segments
- **Typed MCP request and result structs** replacing ad-hoc JSON unmarshaling and generic maps across native tools
- **Re-unified complete platform monorepo (v2.0.0)** shipping the Go gateway and operator platform, in-tree Python/FastAPI ensemble (`ensemble/` — g8ee), and Node.js/Express dashboard (`dashboard/` — g8ed) with unified end-to-end Docker Compose orchestration. See the [g8ee documentation](../ensemble/index.md) and the [g8ed documentation](../dashboard/index.md) for the first-party component details.

### Planned Enhancements

| Enhancement | Target Standard | Timeline |
|-------------|----------------|----------|
| **RBAC & Multi-tenancy** | SOC 2, ISO 27001 | Q3 2026 |
| **Third-party security assessment** | SOC 2 Type II, ISO 27001 | Q4 2026 |
| **Formal penetration testing** | PCI DSS, NIST 800-53 | Q4 2026 |
| **Enhanced logging analytics** | SOC 2, HIPAA | Q1 2027 |
| **Automated compliance reporting** | All frameworks | Q2 2027 |

### Known Limitations

- **Customer Responsibility:** Physical security, network segmentation, and endpoint security are customer responsibilities
- **No Cloud SaaS:** Platform is designed for local deployment; cloud SaaS version not available
- **No RBAC:** Role-based access control is in development
- **No External Audits:** Third-party security assessments are planned but not yet completed
- **Receipt verification key distribution:** Go and Python consumers can canonically verify ActionReceipt signatures and final persistence attestations, but no attested channel distributes the Actuator public key to external consumers. Consumers must establish trust out of band when obtaining the exported key from the gateway or operator PKI directory. An attested bootstrap flow is planned for a future release

---

## 12. Evidence Repository

### Documentation

| Document | Location | Purpose |
|---------|----------|---------|
| **Security Policy** | `.github/SECURITY.md` | Security posture and vulnerability reporting |
| **Architecture: Protocol** | `protocol/docs/spec.md` | Protocol specification and verification layers |
| **Architecture: Auth** | `docs/architecture/auth.md` | Authentication and authorization architecture |
| **Architecture: Operator** | `docs/architecture/operator.md` | Operator execution boundary |
| **Architecture: Gateway** | `docs/architecture/gateway.md` | Gateway policy decision point |
| **Architecture: Governance** | `docs/architecture/governance.md` | Governance postures and transaction process |
| **Architecture: Encryption** | `docs/architecture/encryption.md` | Encryption architecture and certificate management |
| **Architecture: Consensus** | `docs/architecture/consensus.md` | L2 Consensus architecture and signing |
| **Architecture: Storage** | `docs/architecture/storage.md` | Storage and audit architecture |
| **Position Paper** | `docs/core/position_paper.md` | Platform philosophy and security model |

### Code Evidence

| Component | Location | Compliance Relevance |
|-----------|----------|---------------------|
| **PKI Controller** | `internal/services/gateway/pki_controller.go` | Certificate issuance, revocation, CRL |
| **Audit Store** | `internal/services/storage/audit_store.go` | Searchable receipt columns, complete canonical receipt persistence and export, audit logging, encryption, retention |
| **Commitment Ledger** | `internal/services/storage/commitment_ledger.go` | Atomic insertion-ordered signed hash chain and durable receipt linkage |
| **Receipt Protocol** | `protocol/action_receipt_canonicalization_test.go`, `protocol/python/g8e/receipts.py`, `protocol/vectors/` | Cross-language canonical receipt and persistence-attestation verification |
| **Reporting Verification** | `internal/services/reporting/verification.go` | Receipt signatures, persistence attestations, commitment signatures and columns, cross-links, mutation linkage, and Git Merkle roots |
| **Sovereign Execution Boundary** | `internal/services/scrubbing/boundary.go` | PII scrubbing, data sovereignty |
| **L1 Doctrine** | `internal/services/governance/l1_doctrine.go` | Threat detection, input validation |
| **L2 Consensus** | `internal/services/consensus/service.go` | L2 deliberation and Ed25519 consensus signing |
| **L3 Notary** | `internal/services/governance/l3_notary.go` | Two-layer human authorization (passkey plus mTLS transport) |
| **L4 Warden** | `internal/services/governance/l4_warden.go` | Pre-dispatch verification |
| **L5 Actuator** | `internal/services/governance/l5_actuator.go` | Execution boundary, signed receipts |
| **JIT Capabilities** | `internal/services/governance/capability.go` | Self-dissolving execution capability minting |
| **Privileged Route Registry** | `internal/services/gateway/gateway_auth.go` | App certificate governance envelope blocking |
| **L2 Consensus Policy Store** | `internal/services/governance/l2_consensus.go` | L2 consensus interface and policy store decoupling |
| **MCP Gateway ThreatScanner** | `internal/services/mcp/gateway.go` | Threat scanning delegation and audit event recording |
| **KSI Catalog** | `docs/reference/ksi-catalog.json` | Typed per-KSI model with 31 KSIs across 10 categories |
| **KSI Evaluator** | `internal/services/compliance/ksi_evaluator.go` | Binary KSI status derivation from live audit state |
| **OSCAL Renderer Model** | `internal/services/compliance/oscal.go` | Typed OSCAL component-definition and assessment-results generation retained for the proof-backed bundle renderer |
| **KSI History Store** | `internal/services/compliance/ksi_history.go` | KSI snapshot persistence and historical metrics retention |
| **COSAiS Overlay Loader** | `internal/services/compliance/overlay_loader.go` | AI control overlay ingestion and KSI reference validation |
| **Compliance Catalogs** | `protocol/constants/compliance/`, `internal/services/compliance/catalog/` | Digest-verified assertion, framework, crosswalk, and demo-scenario definitions with fail-closed validation |
| **Demo Evidence Verifier** | `internal/services/compliance/evidence/`, `internal/cli/cmd/compliance_demo_run.go` | Read-only verification of persisted demo manifests, scenario results, provenance, content-addressed artifacts, signatures, protocol chains, state observations, and healthcare threshold metrics |
| **Compliance CLI** | `internal/cli/cmd/compliance.go` | `ksi`, `ksi-history`, `overlay`, and `demo-run verify` subcommands |
| **Log Service** | `internal/services/logging/log_service.go` | Platform logging lifecycle, streaming file append and read |
| **Enrollment Coordinator** | `internal/cli/auth/enrollment.go` | CLI enrollment state machine for browser and headless flows |
| **CLI Recovery Controller** | `internal/services/gateway/cli_recovery_controller.go` | CLI recovery approval via browser and mTLS endpoints |
| **Workload Identity** | `protocol/workload_identity.go` | SPIFFE identity specification |

### Test Evidence

| Test Suite | Location | Coverage |
|------------|----------|----------|
| **PKI Tests** | `internal/services/gateway/pki_controller_test.go` | Certificate issuance, revocation |
| **Audit Store Tests** | `internal/services/storage/storagetest/audit_store_test.go` | Complete canonical receipt round trips, list/export records, audit logging, and encryption |
| **Receipt Canonicalization Tests** | `protocol/action_receipt_canonicalization_test.go`, `protocol/python/tests/test_receipts.py`, `protocol/vectors/` | Shared Go/Python receipt, stage-evidence, signature, and persistence-attestation vectors |
| **Persistence Attestation Tests** | `internal/services/governance/l5_actuator_test.go`, `internal/services/governance/l5_actuator_integration_test.go` | Fail-closed final receipt persistence and signed durable association |
| **Commitment Concurrency Tests** | `internal/services/storage/commitment_ledger_concurrency_integration_test.go` | Concurrent ledger instances serialize chain-head selection and append |
| **Sovereignty Tests** | `internal/services/scrubbing/boundary_test.go` | PII scrubbing, rehydration |
| **Governance Tests** | `internal/services/governance/*_test.go` | L1-L5 verification |
| **L3 Approval Pipeline Tests** | `internal/services/governance/l3_approval_pipeline_integration_test.go` | Full CLI to browser to approval pipeline |
| **Reporting Verification Tests** | `internal/services/reporting/verification_test.go` | Commitment ordering, signatures and structured columns, receipt signatures and persistence attestations, receipt cross-links, mutation linkage, and Git Merkle roots |
| **Logging Tests** | `internal/services/logging/*_test.go` | LogService, handler, formatting, and stream file tests |
| **Integration Tests** | `test/*_test.go` | End-to-end security flows |
| **Compliance Tests** | `internal/services/compliance/*_test.go`, `internal/services/compliance/catalog/*_test.go`, `internal/services/compliance/evidence/*_test.go`, `internal/cli/cmd/compliance_demo_run_verify_test.go` | KSI model, evaluator, OSCAL model, history store, overlay loader, canonical catalogs, crosswalk contracts, assessment validation, demo evidence persistence, and fail-closed verifier tamper cases |

---

## 13. Contact and Support

### Security Contact

- **Email:** security@lateraluslabs.com
- **Vulnerability Reporting:** Coordinated disclosure policy in `.github/SECURITY.md`
- **Response Time:** 48 hours acknowledgment, 5 business days initial assessment

### General Contact

- **Email:** hello@lateraluslabs.com
- **Website:** https://lateraluslabs.com
- **Documentation:** https://github.com/g8e-ai/g8e

### Compliance Inquiries

For specific compliance questions or audit support, contact:
- **Email:** compliance@lateraluslabs.com

---

## 14. Document Control

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-06-02 | Lateralus Labs | Initial compliance alignment report |
| 1.1 | 2026-06-12 | Lateralus Labs | Corrected Sovereign Execution Boundary evidence path (`internal/services/scrubbing/boundary.go`); updated platform version to v1.1.1 |
| 1.1.9 | 2026-06-23 | Lateralus Labs | Updated platform version to v1.1.9; replaced L2 Consensus references with Consensus system (`internal/services/consensus/service.go`); corrected DoD Zero Trust Reference Architecture naming; removed auto-approval references (removed in v1.1.9); corrected default posture configuration; fixed evidence repository paths to repository-relative format |
| 1.3.1 | 2026-06-28 | Lateralus Labs | Updated platform version to v1.3.1; updated L3 Notary description to reflect two-layer model (passkey authorization plus mTLS transport); added PrivilegedRouteRegistry and JIT capability minting evidence; added g8e Console SPA to current strengths; corrected `.github/SECURITY.md` references to repository-relative paths; added governance architecture documentation to evidence repository; added reporting verification and L3 approval pipeline test evidence |
| 1.3.5 | 2026-07-02 | Lateralus Labs | Updated platform version to v1.3.5; clarified zero runtime dependencies (statically linked binary, `CGO_ENABLED=0`); removed OpenSSL and Git from runtime dependency lists |
| 1.3.6 | 2026-07-03 | Lateralus Labs | Updated platform version to v1.3.6; added missing v1.3.5 document control entry |
| 1.3.11 | 2026-07-12 | Lateralus Labs | Updated platform version to v1.3.11; added one-time enrollment tokens, SSE-based L3 approval notifications, thread-safe dependency wiring, configurable CORS middleware, and 75% test coverage threshold to current strengths; verified all evidence paths against live codebase |
| 1.4.0 | 2026-07-12 | Lateralus Labs | Updated platform version to v1.4.0; added frontend enrollment (`g8e gui`), Cloudflare Tunnel management (`g8e tunnel`), L5 Actuator `ExecutionHandler` interface refactor, OpenAPI/Swagger annotations across gateway endpoints, consolidated web-cert trust scripts, `operator start` rename, and default TUI launch to current strengths |
| 1.5.0 | 2026-07-13 | Lateralus Labs | Updated platform version to v1.5.0; added unified Go module, Cosign/sigstore artifact signing, Gitleaks secret scanning, Go-licenses compliance, cross-OS CI matrix, Python protocol conformance suite (151 tests), Go performance benchmarks, and smoke test scripts to current strengths |
| 1.5.1 | 2026-07-14 | Lateralus Labs | Updated platform version to v1.5.1; added NIST SP 800-63B-4 alignment section; updated NIST SP 800-53 to Rev 5.2.0 with new controls SA-15(13), SA-24, SI-02(07); updated PCI DSS to v4.0.1; added ISO 27001 Amendment 1:2024 reference; corrected ZIG pillars to match DoW seven-pillar framework; added NIST SP 800-207A reference; updated DoD to DoW naming; added RuntimeFileService to current strengths; verified all evidence paths against live codebase |
| 1.5.8 | 2026-07-19 | Lateralus Labs | Updated platform version to v1.5.8; added interactive gateway onboarding wizard, Anduril Lattice gRPC adapter, MCP tool interception verification, MCP gateway output scrubbing, agent support consolidation, governance interface extraction, and protocol wire-format documentation to current strengths; corrected Python protocol test suite (151 tests) and conformance suite (330 tests) counts; updated test coverage to 75.9%; added receipt signature verification known limitation; verified all evidence paths against live codebase |
| 1.6.8 | 2026-07-31 | Lateralus Labs | Updated platform version to v1.6.8; added cross-language governance hash parity, enrollment token TOCTOU fix, SSE resilience improvements, governance JSON schema, gateway HTTP handler decomposition, CanonicalDBService Stores struct extraction, L2 consensus interface decoupling, AuthController decomposition, PasskeyOrchestrator extraction, L1 doctrine threat detector expansion, MCP gateway ThreatScanner interface, L5 Actuator decomposition with fail-closed rehydration, MCP gateway AuditEventRecorder interface, error sentinel migration, ScrubbingService constructor fail-closed, "Tribunal" to "Consensus" rename, DBController split, L2 consensus logic extraction, configurable doctrine directory, gateway construction refactor to InitHTTPHandler(), dead code removal (107 functions), and FedRAMP sovereign cloud governance demo to current strengths; updated Python protocol conformance suite count from 330 to 420; added encryption, consensus, and storage architecture docs to evidence repository; added L2 Consensus Policy Store and MCP Gateway ThreatScanner to code evidence; verified all evidence paths against live codebase |
| 1.7.0 | 2026-08-02 | Lateralus Labs | Added FedRAMP 20x (CR26) alignment section with Certification Classes A-D, typed KSI model (31 KSIs), OSCAL evidence export, historical metrics retention, COSAiS overlay alignment, and doctrine KSI linkage; updated AU-10 non-repudiation claim to reference KSI-MLA-07; added compliance package code evidence and test evidence entries |
| 1.7.1 | 2026-08-10 | Lateralus Labs | Updated platform version to v1.7.1; updated Current Strengths gateway construction entry to reflect single-phase construction with lazy forwarding adapters and per-controller dependency injection (replacing the prior two-phase InitHTTPHandler pattern); verified all evidence paths against live codebase |
| 1.7.2 | 2026-08-14 | Lateralus Labs | Updated platform version to v1.7.2; added EnrollmentCoordinator state machine, human-approved CLI recovery flow, and mTLS-protected CLI certificate rotation endpoint to Current Strengths; noted removal of the --tpm flag (file-backed EC P-256 keys on all platforms); verified all evidence paths against live codebase |
| 1.7.3 | 2026-08-14 | Lateralus Labs | Updated platform version to v1.7.3; noted RootCACommonName constant centralization (internal refactor, no on-wire or trust-store behavior change); verified all evidence paths against live codebase |
| 1.7.4 | 2026-08-15 | Lateralus Labs | Updated platform version to v1.7.4; noted passkey enrollment is required only for notary posture (optional for doctrine and consensus); noted single always-FIPS Dockerfile consolidation and demo orchestration simplification; verified all evidence paths against live codebase |
| 1.7.5 | 2026-08-16 | Lateralus Labs | Updated platform version to v1.7.5; added audit store hard dependency for MCP gateway, passkey proof verification decoupling from L3 notary interface, and atomic.Pointer adoption for late-bound consensus service to Current Strengths; added typed audit response models to Current Strengths; corrected agent support count from 4 to 5 to reflect Devin CLI addition in v1.6.7; verified all evidence paths against live codebase |
| 1.7.6 | 2026-08-16 | Lateralus Labs | Reconciled architecture, guide, and reference documentation; aligned Python protocol package constants with Go SSOT; fixed PyPI publish verification workflow |
| 1.7.7 | 2026-08-18 | Lateralus Labs | Added headless CLI enrollment (`--headless`), mTLS recovery approval (`g8e auth approve-recovery` and `POST /api/v1/auth/cli/recovery/approve-cli`), and updated project license to Business Source License 1.1 (BSL 1.1) with 2030-08-18 Change Date |
| 1.7.8 | 2026-08-19 | Lateralus Labs | Added platform logging consolidation (`internal/services/logging`, `g8e.log`, `OpenForAppend`/`OpenForRead`), `PSUBSCRIBE` glob-pattern subscriptions on gateway pubsub with fail-closed topic ACLs, typed MCP native tool request/result structs, and `docs/architecture/overview.md` |
| 2.0.0 | 2026-08-25 | Lateralus Labs | Updated platform version to v2.0.0; documented polyglot monorepo reunification shipping Go platform, in-tree Python ensemble (`ensemble/`), Node.js dashboard (`dashboard/`), unified Docker compose orchestration, and updated test suite metrics |
| 2.1.0 | 2026-08-30 | Lateralus Labs | Added deterministic stage-evidence receipt binding, signed durable-persistence attestations, atomic signed commitment chaining, full canonical receipt persistence and export, cross-language offline verification, and expanded code and test evidence |
| 2.1.2 | 2026-08-31 | Lateralus Labs | Added ratify posture with L1/L3 enforcement and audited L2, clarified posture-required fail-closed verification, and reconciled posture-dependent passkey and consensus requirements |
| 2.1.3 | 2026-09-02 | Lateralus Labs | Added protocol-owned compliance catalogs and records, persisted typed evidence across the healthcare, finance, DHS, and FedRAMP demos, and read-only fail-closed demo-run verification; reconciled OSCAL CLI availability and proof-backed bundle limitations |

---

## Appendix A: Glossary

- **L1-L5:** 5-layer verification sequence (Doctrine, Consensus, Notary, Warden, Actuator)
- **Consensus:** Enrolled agentic service responsible for L2 consensus deliberation and Ed25519 vote signing
- **mTLS:** Mutual Transport Layer Security
- **SPIFFE:** Secure Production Identity Framework For Everyone
- **SAN:** Subject Alternative Name
- **PKI:** Public Key Infrastructure
- **CA:** Certificate Authority
- **CSR:** Certificate Signing Request
- **CRL:** Certificate Revocation List
- **UEI:** Uniform Element Identifier (token placeholder)
- **JIT:** Just-In-Time provisioning
- **PII:** Personally Identifiable Information
- **PHI:** Protected Health Information
- **SOC:** System and Organization Controls
- **GDPR:** General Data Protection Regulation
- **HIPAA:** Health Insurance Portability and Accountability Act
- **NIST:** National Institute of Standards and Technology
- **PCI DSS:** Payment Card Industry Data Security Standard
- **ISO:** International Organization for Standardization
- **DoD:** Department of Defense
- **DoW:** Department of War (formerly Department of Defense)
- **ZIG:** Zero Trust Implementation Guidelines
- **AAL:** Authentication Assurance Level
- **KSI:** Key Security Indicator (FedRAMP 20x binary-resolution compliance metric)
- **OSCAL:** Open Security Controls Assessment Language (NIST machine-readable compliance format)
- **COSAiS:** Control Overlays for Securing AI Systems (NIST AI-specific control overlays)
- **CR26:** FedRAMP Consolidated Rules for 2026
- **20x:** FedRAMP 20x path (KSI-based, machine-readable compliance assessment)

---

*This document is maintained by Lateralus Labs and reflects the security posture of g8e as of the version date. For the most current information, refer to the latest version in the repository.*
