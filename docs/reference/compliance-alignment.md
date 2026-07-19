# Compliance Alignment Report

**Document Version:** 1.5.8  
**Last Updated:** 2026-07-19  
**Platform:** g8e v1.5.8  
**Maintained by:** Lateralus Labs, LLC.

---

## Executive Summary

This document provides a comprehensive alignment of the g8e platform's security controls and governance mechanisms against major industry compliance frameworks. g8e is designed as a zero-trust execution platform for agentic infrastructure, implementing fail-closed verification, cryptographic proof chains, and local-first data sovereignty.

**Key Compliance Posture:**
- **SOC 2 Type II:** Strong alignment with Trust Services Criteria (Security, Availability, Confidentiality)
- **ISO 27001:2022:** Comprehensive coverage of Annex A controls
- **GDPR:** Data sovereignty by design with PII scrubbing and local processing
- **HIPAA:** Security Rule alignment with audit trails and access controls
- **NIST SP 800-53 Rev 5.2.0:** Moderate-to-High baseline coverage
- **NIST SP 800-63B-4:** AAL2/AAL3 alignment with phishing-resistant authenticators
- **PCI DSS v4.0.1:** Relevant controls for cardholder data environments
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
| **CC1.10** | Security monitoring | Audit store with immutable git-backed ledger, signed ActionReceipts | `internal/services/storage/audit_store.go`, `internal/services/storage/ledger.go` |
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
- **A.17.2 Intellectual property rights:** Apache 2.0 license
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
| **AU-10** | Non-repudiation | Signed ActionReceipts with Ed25519 |
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
| **SC-13** | Use of cryptography | ECDSA P-256, Ed25519, SHA-256 |
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
| **1.2.3** | Secure configuration | Configurable governance postures (doctrine, consensus, notary); outbound mode defaults to notary |
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
| **Transaction Signatures** | Ed25519 | L2 Consensus, L5 Actuator | `internal/services/tribunal/service.go`, `internal/services/governance/l4_warden.go` |
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

## 10. Gap Analysis and Roadmap

### Current Strengths

- **Fail-closed verification pipeline** with 5-layer interlock
- **Cryptographic proof chains** with Ed25519 signatures
- **Local-first data sovereignty** with git-backed ledger
- **Comprehensive audit trail** with signed receipts
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
- **Python protocol test suite** (151 tests) validating constant accessors, enum generation, and model serialization
- **Python protocol conformance suite** (330 tests) enforcing parity between Go constants, Python runtime values, and canonical JSON SSOT files
- **Interactive gateway onboarding wizard** (`g8e gw start --interactive`) guiding users through network, security, agent, and review configuration steps
- **Anduril Lattice gRPC adapter** with OAuth2 client credentials authentication, gRPC retry with status code classification, and heartbeat interval validation
- **MCP tool interception verification** (`--verify` flag) ensuring agent tool-disabling configurations are correctly applied before agent launch
- **MCP gateway output scrubbing** preventing sensitive data leakage in downstream MCP audit logs
- **Agent support consolidation** to 4 supported agents (Claude Code, Codex, Goose, Gemini CLI) reducing attack surface
- **Governance interface extraction** separating verification concerns (replay store, signer store, tribunal store, app policy store, state root provider) into dedicated files
- **Protocol wire-format documentation** for EventType, AgentMode, and SessionEventWire ensuring consumer compatibility

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
- **Receipt signature verification:** The Gateway signs ActionReceipts with its Actuator Ed25519 private key, but no attested channel exists for distributing the public key to Engine instances. Consumers must obtain the public key out-of-band from the gateway PKI directory. An attested bootstrap flow is planned for a future release

---

## 11. Evidence Repository

### Documentation

| Document | Location | Purpose |
|---------|----------|---------|
| **Security Policy** | `.github/SECURITY.md` | Security posture and vulnerability reporting |
| **Architecture: Protocol** | `protocol/docs/spec.md` | Protocol specification and verification layers |
| **Architecture: Auth** | `docs/architecture/auth.md` | Authentication and authorization architecture |
| **Architecture: Operator** | `docs/architecture/operator.md` | Operator execution boundary |
| **Architecture: Gateway** | `docs/architecture/gateway.md` | Gateway policy decision point |
| **Architecture: Governance** | `docs/architecture/governance.md` | Governance postures and transaction process |
| **Position Paper** | `docs/core/position_paper.md` | Platform philosophy and security model |

### Code Evidence

| Component | Location | Compliance Relevance |
|-----------|----------|---------------------|
| **PKI Controller** | `internal/services/gateway/pki_controller.go` | Certificate issuance, revocation, CRL |
| **Audit Store** | `internal/services/storage/audit_store.go` | Audit logging, encryption, retention |
| **Sovereign Execution Boundary** | `internal/services/scrubbing/boundary.go` | PII scrubbing, data sovereignty |
| **L1 Doctrine** | `internal/services/governance/l1_doctrine.go` | Threat detection, input validation |
| **L2 Consensus** | `internal/services/tribunal/service.go` | L2 deliberation and Ed25519 consensus signing |
| **L3 Notary** | `internal/services/governance/l3_notary.go` | Two-layer human authorization (passkey plus mTLS transport) |
| **L4 Warden** | `internal/services/governance/l4_warden.go` | Pre-dispatch verification |
| **L5 Actuator** | `internal/services/governance/l5_actuator.go` | Execution boundary, signed receipts |
| **JIT Capabilities** | `internal/services/governance/capability.go` | Self-dissolving execution capability minting |
| **Privileged Route Registry** | `internal/services/gateway/gateway_auth.go` | App certificate governance envelope blocking |
| **Workload Identity** | `protocol/workload_identity.go` | SPIFFE identity specification |

### Test Evidence

| Test Suite | Location | Coverage |
|------------|----------|----------|
| **PKI Tests** | `internal/services/gateway/pki_controller_test.go` | Certificate issuance, revocation |
| **Audit Store Tests** | `internal/services/storage/storagetest/audit_store_test.go` | Audit logging, encryption |
| **Sovereignty Tests** | `internal/services/scrubbing/boundary_test.go` | PII scrubbing, rehydration |
| **Governance Tests** | `internal/services/governance/*_test.go` | L1-L5 verification |
| **L3 Approval Pipeline Tests** | `internal/services/governance/l3_approval_pipeline_integration_test.go` | Full CLI to browser to approval pipeline |
| **Reporting Verification Tests** | `internal/services/reporting/verification_test.go` | Commitment chain, merkle root, receipt cross-link |
| **Integration Tests** | `test/*_test.go` | End-to-end security flows |

---

## 12. Contact and Support

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

## 13. Document Control

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-06-02 | Lateralus Labs | Initial compliance alignment report |
| 1.1 | 2026-06-12 | Lateralus Labs | Corrected Sovereign Execution Boundary evidence path (`internal/services/scrubbing/boundary.go`); updated platform version to v1.1.1 |
| 1.1.9 | 2026-06-23 | Lateralus Labs | Updated platform version to v1.1.9; replaced L2 Consensus references with Tribunal system (`internal/services/tribunal/service.go`); corrected DoD Zero Trust Reference Architecture naming; removed auto-approval references (removed in v1.1.9); corrected default posture configuration; fixed evidence repository paths to repository-relative format |
| 1.3.1 | 2026-06-28 | Lateralus Labs | Updated platform version to v1.3.1; updated L3 Notary description to reflect two-layer model (passkey authorization plus mTLS transport); added PrivilegedRouteRegistry and JIT capability minting evidence; added g8e Console SPA to current strengths; corrected `.github/SECURITY.md` references to repository-relative paths; added governance architecture documentation to evidence repository; added reporting verification and L3 approval pipeline test evidence |
| 1.3.5 | 2026-07-02 | Lateralus Labs | Updated platform version to v1.3.5; clarified zero runtime dependencies (statically linked binary, `CGO_ENABLED=0`); removed OpenSSL and Git from runtime dependency lists |
| 1.3.6 | 2026-07-03 | Lateralus Labs | Updated platform version to v1.3.6; added missing v1.3.5 document control entry |
| 1.3.11 | 2026-07-12 | Lateralus Labs | Updated platform version to v1.3.11; added one-time enrollment tokens, SSE-based L3 approval notifications, thread-safe dependency wiring, configurable CORS middleware, and 75% test coverage threshold to current strengths; verified all evidence paths against live codebase |
| 1.4.0 | 2026-07-12 | Lateralus Labs | Updated platform version to v1.4.0; added frontend enrollment (`g8e gui`), Cloudflare Tunnel management (`g8e tunnel`), L5 Actuator `ExecutionHandler` interface refactor, OpenAPI/Swagger annotations across gateway endpoints, consolidated web-cert trust scripts, `operator start` rename, and default TUI launch to current strengths |
| 1.5.0 | 2026-07-13 | Lateralus Labs | Updated platform version to v1.5.0; added unified Go module, Cosign/sigstore artifact signing, Gitleaks secret scanning, Go-licenses compliance, cross-OS CI matrix, Python protocol conformance suite (151 tests), Go performance benchmarks, and smoke test scripts to current strengths |
| 1.5.1 | 2026-07-14 | Lateralus Labs | Updated platform version to v1.5.1; added NIST SP 800-63B-4 alignment section; updated NIST SP 800-53 to Rev 5.2.0 with new controls SA-15(13), SA-24, SI-02(07); updated PCI DSS to v4.0.1; added ISO 27001 Amendment 1:2024 reference; corrected ZIG pillars to match DoW seven-pillar framework; added NIST SP 800-207A reference; updated DoD to DoW naming; added RuntimeFileService to current strengths; verified all evidence paths against live codebase |
| 1.5.8 | 2026-07-19 | Lateralus Labs | Updated platform version to v1.5.8; added interactive gateway onboarding wizard, Anduril Lattice gRPC adapter, MCP tool interception verification, MCP gateway output scrubbing, agent support consolidation, governance interface extraction, and protocol wire-format documentation to current strengths; corrected Python protocol test suite (151 tests) and conformance suite (330 tests) counts; updated test coverage to 75.9%; added receipt signature verification known limitation; verified all evidence paths against live codebase |

---

## Appendix A: Glossary

- **L1-L5:** 5-layer verification sequence (Doctrine, Consensus, Notary, Warden, Actuator)
- **Tribunal:** Enrolled agentic service responsible for L2 consensus deliberation and Ed25519 vote signing
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

---

*This document is maintained by Lateralus Labs and reflects the security posture of g8e as of the version date. For the most current information, refer to the latest version in the repository.*
