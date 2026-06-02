# Compliance Alignment Report

**Document Version:** 1.0  
**Last Updated:** 2026-06-02  
**Platform:** g8e v1.0.8  
**Maintained by:** Lateralus Labs

---

## Executive Summary

This document provides a comprehensive alignment of the g8e platform's security controls and governance mechanisms against major industry compliance frameworks. g8e is designed as a zero-trust execution platform for agentic infrastructure, implementing fail-closed verification, cryptographic proof chains, and local-first data sovereignty.

**Key Compliance Posture:**
- **SOC 2 Type II:** Strong alignment with Trust Services Criteria (Security, Availability, Confidentiality)
- **ISO 27001:2022:** Comprehensive coverage of Annex A controls
- **GDPR:** Data sovereignty by design with PII scrubbing and local processing
- **HIPAA:** Security Rule alignment with audit trails and access controls
- **NIST 800-53 Rev 5:** Moderate-to-High baseline coverage
- **PCI DSS 4.0:** Relevant controls for cardholder data environments
- **NSA Zero Trust Implementation Guidelines (ZIG):** Strong alignment with Discovery, Phase One, and Phase Two activities

---

## 1. SOC 2 Type II Alignment

### Trust Services Criteria Mapping

#### TSC - Security (CC)

| Control ID | Control Description | g8e Implementation | Evidence Location |
|------------|---------------------|-------------------|-------------------|
| **CC1.1** | Logical and physical access controls | mTLS with SPIFFE workload identity, WebAuthn L3 Notary | `internal/services/gateway/pki_controller.go`, `docs/architecture/auth.md` |
| **CC1.2** | Logical access security software | 5-layer verification pipeline (L1-L5) | `internal/services/governance/l4_warden.go` |
| **CC1.3** | Logical access to system components | Role-based session isolation (operator_session_id, cli_session_id, web_session_id) | `docs/architecture/g8e.md` |
| **CC1.4** | Logical access to stored data | Encrypted audit vault with optional encryption at rest | `internal/services/storage/audit_vault.go` |
| **CC1.5** | Authentication of external users | WebAuthn/FIDO2 hardware-bound authentication, mTLS certificate verification | `internal/services/governance/l3_notary.go` |
| **CC1.6** | Identification and authentication | SPIFFE URI SAN binding in certificates, Ed25519 signature verification | `protocol/workload_identity.go` |
| **CC1.7** | Logical access for support personnel | No standing privileges, JIT provisioning via CSR enrollment | `internal/services/gateway/pki_controller.go` |
| **CC1.8** | Management of access security | Certificate revocation with database-backed denylist and CRL | `internal/services/gateway/pki_controller.go` |
| **CC1.9** | Data transfer security | mTLS for all platform communication, outbound-only operator connections | `docs/architecture/auth.md` |
| **CC1.10** | Security monitoring | Audit vault with immutable git-backed ledger, signed ActionReceipts | `internal/services/storage/audit_vault.go` |
| **CC1.11** | Data disposal | Configurable retention policies (default 90 days), automated pruning | `internal/services/storage/audit_vault.go` |
| **CC1.12** | Security incident response | Coordinated disclosure policy via `security@lateraluslabs.com` | `SECURITY.md` |

#### TSC - Availability (A)

| Control ID | Control Description | g8e Implementation | Evidence Location |
|------------|---------------------|-------------------|-------------------|
| **A1.1** | Processing and recovery | Git-backed ledger for state recovery, replay protection via nonce | `internal/services/storage/ledger.go` |
| **A1.2** | Availability monitoring | Health checks via audit vault status, session tracking | `internal/services/storage/audit_vault.go` |
| **A1.3** | Data backup | Local-first storage with git commits per mutation | `internal/services/storage/ledger.go` |

#### TSC - Confidentiality (C)

| Control ID | Control Description | g8e Implementation | Evidence Location |
|------------|---------------------|-------------------|-------------------|
| **C1.1** | Confidentiality of information at rest | Optional encryption vault for audit content fields | `internal/services/storage/audit_vault.go` |
| **C1.2** | Confidentiality of information in transit | mTLS with TLS 1.3 for all platform communication | `docs/architecture/auth.md` |
| **C1.3** | Confidentiality of information during processing | Sovereignty Boundary Plane with PII/secret scrubbing before cloud transmission | `internal/services/sovereignty/boundary.go` |
| **C1.4** | Avoidance of unauthorized disclosure | Deterministic rehydration only at execution boundary | `internal/services/sovereignty/boundary.go` |

---

## 2. ISO 27001:2022 Alignment

### Annex A Control Mapping

#### A.5 Organizational Security Policies
- **A.5.1 Policies for information security:** Security policy documented in `SECURITY.md`
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
- **A.8.3 Information classification:** Sovereignty Boundary Plane classifies sensitive data
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
- **A.9.10 Information disclosure restrictions:** Sovereignty scrubbing before external transmission

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
- **A.12.4 Logging and monitoring:** Audit vault with signed receipts
- **A.12.5 Log information protection:** Optional encryption at rest
- **A.12.6 Logging synchronization:** Local-first storage, no external dependency
- **A.12.7 Information leak prevention:** Sovereignty Boundary Plane scrubbing
- **A.12.8 Information deletion:** Configurable retention policies
- **A.12.9 Information backup:** Git ledger provides versioned backup

#### A.13 Communications Security
- **A.13.1 Network security controls:** mTLS with TLS 1.3
- **A.13.2 Security of information in transit:** mTLS for all platform communication
- **A.13.3 Information transfer policies:** Outbound-only operator connections

#### A.14 System Acquisition, Development, and Maintenance
- **A.14.1 Security requirements:** Fail-closed verification by design
- **A.14.2 Security in development:** Security testing in CI/CD (`.github/workflows/build-and-test.yml`)
- **A.14.3 Test data:** Separate test fixtures in `protocol/test-fixtures/`
- **A.14.4 Change management:** Versioned releases with changelog
- **A.14.5 Capacity management:** Configurable database size limits
- **A.14.6 Change control:** Git-based version control
- **A.14.7 Information on vulnerabilities:** Coordinated disclosure policy
- **A.14.8 Audit logging:** Comprehensive audit vault
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
- **A.17.3 Protection of records:** Audit vault with git ledger
- **A.17.4 Privacy and protection of PII:** Sovereignty Boundary Plane with PII scrubbing
- **A.17.5 Independent review:** Third-party security assessment (planned)

---

## 3. GDPR Alignment

### Data Protection Principles

| Principle | g8e Implementation | Evidence |
|-----------|-------------------|----------|
| **Lawfulness, fairness, transparency** | Local-first processing, user-controlled data | `internal/services/sovereignty/boundary.go` |
| **Purpose limitation** | Sovereignty scrubbing prevents data leakage to unintended systems | `internal/services/sovereignty/boundary.go` |
| **Data minimization** | Scrubbing removes PII before cloud transmission | `internal/services/sovereignty/boundary.go` |
| **Accuracy** | Immutable git-backed ledger with state roots | `internal/services/storage/ledger.go` |
| **Storage limitation** | Configurable retention policies (default 90 days) | `internal/services/storage/audit_vault.go` |
| **Integrity and confidentiality** | Encryption at rest, mTLS in transit, access controls | `internal/services/storage/audit_vault.go`, `docs/architecture/auth.md` |

### GDPR Rights Support

| Right | g8e Capability | Implementation |
|-------|----------------|----------------|
| **Right to access** | Local audit vault export | User can query local SQLite database |
| **Right to rectification** | Git ledger allows state rollback | `internal/services/storage/ledger.go` |
| **Right to erasure** | Configurable retention and pruning | `internal/services/storage/audit_vault.go` |
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
| **Security Incident Procedures** | Coordinated disclosure policy | `SECURITY.md` |
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
| **Audit Controls** | Comprehensive audit vault with signed receipts | `internal/services/storage/audit_vault.go` |
| **Integrity Controls** | State Merkle roots, git-backed ledger | `internal/services/storage/ledger.go` |
| **Transmission Security** | mTLS with TLS 1.3 for all communication | `docs/architecture/auth.md` |

### PHI Handling

- **PHI Scrubbing:** Sovereignty Boundary Plane scrubs PII/PHI patterns before cloud transmission
- **Local Processing:** All PHI processing occurs on customer infrastructure
- **Audit Trail:** Immutable audit logs track all PHI access

---

## 5. NIST 800-53 Rev 5 Alignment

### Moderate-to-High Baseline Controls

#### Access Control (AC)

| Control | g8e Implementation | Evidence |
|---------|-------------------|----------|
| **AC-1** | Access control policy | `SECURITY.md` |
| **AC-2** | Account management | CSR-based enrollment, session tracking |
| **AC-3** | Access enforcement | 5-layer verification pipeline |
| **AC-6** | Least privilege | JIT provisioning, no standing privileges |
| **AC-7** | Successful/failed access attempts | Audit vault logging |
| **AC-8** | System use notification | Session identifiers in envelopes |
| **AC-11** | Session lock | Session-based isolation |
| **AC-12** | Session termination | Configurable session timeouts |
| **AC-14** | Permitted actions without identification | Auto-approval for benign diagnostics only |
| **AC-17** | Remote access | mTLS required for all remote connections |
| **AC-18** | Wireless access | Customer-controlled infrastructure |
| **AC-19** | Access control for mobile devices | Windows/macOS/Linux support parity |
| **AC-20** | Use of external information systems | Sovereignty scrubbing before external transmission |

#### Audit and Accountability (AU)

| Control | g8e Implementation | Evidence |
|---------|-------------------|----------|
| **AU-1** | Audit and accountability policy | `SECURITY.md` |
| **AU-2** | Audit events | Comprehensive event logging in audit vault |
| **AU-3** | Audit record content | Events include timestamp, user, action, result |
| **AU-4** | Audit storage retention | Configurable retention (default 90 days) |
| **AU-5** | Audit response to processing failures | Fail-closed: execution aborted if audit fails |
| **AU-6** | Audit review, analysis, and reporting | Query capabilities via SQLite |
| **AU-7** | Audit reduction and report generation | Truncation for large outputs |
| **AU-8** | Time stamps | All events include UTC timestamps |
| **AU-9** | Protection of audit information | Optional encryption at rest |
| **AU-10** | Non-repudiation | Signed ActionReceipts with Ed25519 |
| **AU-11** | Audit record retention | Configurable retention policies |
| **AU-12** | Audit generation | All mutations generate audit records |

#### System and Communications Protection (SC)

| Control | g8e Implementation | Evidence |
|---------|-------------------|----------|
| **SC-1** | System and communications protection policy | `SECURITY.md` |
| **SC-7** | Boundary protection | Sovereignty Boundary Plane |
| **SC-8** | Transmission confidentiality | mTLS with TLS 1.3 |
| **SC-9** | Transmission integrity | mTLS with certificate verification |
| **SC-10** | Network disconnect | Outbound-only operator connections |
| **SC-11** | Trusted path | mTLS with SPIFFE identity |
| **SC-12** | Cryptographic key establishment and management | PKI hierarchy with root/intermediate CA |
| **SC-13** | Use of cryptography | ECDSA P-256, Ed25519, SHA-256 |
| **SC-17** | Public key infrastructure certificates | Certificate issuance, revocation, CRL |
| **SC-18** | Mobile code | Single binary, no external dependencies |
| **SC-20** | Secure name/address resolution | g8e.local canonical alias with internal translation |
| **SC-21** | Domain name services | Customer-controlled DNS |
| **SC-22** | Architecture and provisioning | Local-first, air-gap capable |
| **SC-23** | Session authenticity | Session-based isolation with cryptographic binding |

#### System and Information Integrity (SI)

| Control | g8e Implementation | Evidence |
|---------|-------------------|----------|
| **SI-1** | System and information integrity policy | `SECURITY.md` |
| **SI-2** | Flaw remediation | Coordinated disclosure, automated dependency scanning |
| **SI-3** | Malicious code protection | L1 Doctrine with MITRE ATT&CK detection |
| **SI-4** | System monitoring | Audit vault, signed receipts |
| **SI-5** | Security alerts | Session-based event routing |
| **SI-6** | Integrity verification | State Merkle roots, hash-based transaction binding |
| **SI-7** | Software, firmware, and information integrity | Git-backed ledger with per-mutation commits |
| **SI-8** | Spam protection | Not applicable (infrastructure platform) |
| **SI-10** | Information input validation | L1 Doctrine with input validation framework |
| **SI-11** | Error handling | Fail-closed verification, error categorization |
| **SI-12** | Information output handling | Sovereignty scrubbing before output |
| **SI-13** | Predictable failure prevention | Fail-closed design |
| **SI-14** | Non-persistence | JIT credentials, no standing privileges |
| **SI-16** | Memory protection | Go memory safety |
| **SI-17** | Fail-safe procedures | Fail-closed verification pipeline |

---

## 6. PCI DSS 4.0 Alignment

### Relevant Controls for Cardholder Data Environments

| Requirement | g8e Implementation | Evidence |
|-------------|-------------------|----------|
| **1.2.1** | Business need justification | Outbound-only operator connections |
| **1.2.3** | Secure configuration | Default security posture (Notary mode) |
| **1.3** | Secure data flows | mTLS for all platform communication |
| **2.1** | Change control processes | Git-based version control |
| **2.2** | Configuration standards | Documented in architecture docs |
| **2.3** | Encryption of non-console administrative access | mTLS for all access |
| **3.1** | Keep cardholder data storage to minimum | Sovereignty scrubbing removes PII |
| **3.2** | Protect stored cardholder data | Optional encryption at rest |
| **3.3** | Mask PAN when displayed | Sovereignty scrubbing patterns |
| **3.4** | Render PAN unreadable | Sovereignty scrubbing patterns |
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
| **9.1** | Use effective logging | Comprehensive audit vault |
| **9.2** | Protect audit trails | Optional encryption at rest |
| **9.3** | Secure audit trails | Signed ActionReceipts |
| **9.4** | Review audit trails | Query capabilities via SQLite |
| **10.1** | Implement audit trails | All mutations generate audit records |
| **10.2** | Audit trail automation | Automatic logging in audit vault |
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
| **12.1** | Maintain security policy | `SECURITY.md` |
| **12.2** | Risk assessment | This compliance alignment document |
| **12.3** | Security awareness | Documentation for secure deployment |
| **12.4** | Incident response | Coordinated disclosure policy |
| **12.5** | Vulnerability management | Coordinated disclosure, automated scanning |
| **12.6** | Security updates | Versioned releases with changelog |
| **12.7** | Security information | Architecture documentation |
| **12.8** | Security policies | `SECURITY.md` |

---

## 7. NSA Zero Trust Implementation Guidelines (ZIG) Alignment

### Overview

The NSA Zero Trust Implementation Guidelines (ZIG) provide a five-phase approach to achieving Target-level Zero Trust maturity, aligned with the Department of War (DoW) Zero Trust Framework and NIST guidance. g8e demonstrates strong alignment with the Discovery, Phase One, and Phase Two guidelines.

**ZIG Phases:**
- **Discovery Phase:** Identify critical data, applications, assets, and services (DAAS)
- **Phase One:** Establish secure foundation (36 activities, 30 capabilities)
- **Phase Two:** Integrate core zero trust tools (41 activities, 34 capabilities)
- **Phase Three:** (Planned) Advanced zero trust integration
- **Phase Four:** (Planned) Optimization and continuous improvement

### Discovery Phase Alignment

| ZIG Activity | Description | g8e Implementation | Evidence |
|--------------|-------------|-------------------|----------|
| **Identify Critical Data** | Catalog sensitive data and classification | Sovereignty Boundary Plane with PII/secret detection | `internal/services/sovereignty/boundary.go` |
| **Identify Critical Applications** | Map application dependencies and data flows | GovernanceEnvelope protocol with transaction tracking | `docs/architecture/g8e.md` |
| **Identify Critical Assets** | Inventory infrastructure components | Operator session tracking, ledger state | `internal/services/storage/ledger.go` |
| **Identify Critical Services** | Catalog services and communication patterns | SPIFFE workload identity registry | `protocol/workload_identity.go` |
| **Map Data Flows** | Document data movement across boundaries | Sovereignty scrubbing before external transmission | `internal/services/sovereignty/boundary.go` |
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
| **Encryption at Rest** | Protect stored data | Optional encryption vault for audit content | `internal/services/storage/audit_vault.go` |
| **Identity and Access Management** | Centralized identity control | SPIFFE workload identity system | `protocol/workload_identity.go` |
| **Continuous Monitoring** | Real-time security monitoring | Audit vault with signed receipts | `internal/services/storage/audit_vault.go` |
| **Incident Response** | Coordinated disclosure and response | Coordinated disclosure policy | `SECURITY.md` |

### Phase Two Alignment: Core Zero Trust Tools

| ZIG Capability | Description | g8e Implementation | Evidence |
|----------------|-------------|-------------------|----------|
| **Policy Decision Point** | Centralized authorization decisions | Gateway as policy decision point | `docs/architecture/gateway.md` |
| **Policy Enforcement Point** | Distributed enforcement | L4 Warden pre-dispatch verification | `internal/services/governance/l4_warden.go` |
| **Dynamic Policy Evaluation** | Context-aware authorization | 5-layer verification with real-time checks | `internal/services/governance/l1_doctrine.go` |
| **Risk-Based Authentication** | Adaptive authentication based on risk | L3 Notary with hardware-bound auth | `internal/services/governance/l3_notary.go` |
| **Least Privilege Access** | Minimum necessary access | JIT provisioning, per-transaction authorization | `internal/services/gateway/pki_controller.go` |
| **Micro-Segmentation** | Fine-grained network segmentation | mTLS with workload identity | `docs/architecture/auth.md` |
| **Data Loss Prevention** | Prevent unauthorized data exfiltration | Sovereignty Boundary Plane scrubbing | `internal/services/sovereignty/boundary.go` |
| **Threat Detection** | Identify malicious activity | L1 Doctrine with MITRE ATT&CK patterns | `internal/services/governance/l1_doctrine.go` |
| **Automated Response** | Automated containment of threats | Fail-closed verification pipeline | `internal/services/governance/l5_actuator.go` |
| **Audit Logging** | Comprehensive audit trail | Audit vault with git-backed ledger | `internal/services/storage/audit_vault.go` |
| **Session Management** | Secure session handling | Session-based isolation (operator_session_id, cli_session_id, web_session_id) | `docs/architecture/g8e.md` |
| **Certificate Management** | PKI lifecycle management | Root/intermediate CA hierarchy, CRL | `internal/services/gateway/pki_controller.go` |
| **Key Management** | Secure key storage and rotation | PKI hierarchy with key separation | `internal/services/gateway/pki_controller.go` |
| **API Security** | Secure API communication | mTLS for all platform APIs | `docs/architecture/auth.md` |
| **Supply Chain Security** | Verify software integrity | Git-based version control, signed releases | `docs/architecture/g8e.md` |

### ZIG Pillars Alignment

The NSA ZIG framework aligns with the DoW Zero Trust pillars. g8e implements these pillars as follows:

| Pillar | ZIG Focus | g8e Implementation |
|--------|-----------|-------------------|
| **Identity** | Strong authentication, identity management | WebAuthn/FIDO2, SPIFFE workload identity, mTLS |
| **Devices** | Device trust and posture | Certificate-based device identity, mTLS verification |
| **Network** | Network segmentation, encryption | mTLS everywhere, micro-segmentation |
| **Applications** | Application security, data protection | GovernanceEnvelope, Sovereignty Boundary Plane |
| **Data** | Data classification, loss prevention | PII/secret scrubbing, encryption at rest |
| **Infrastructure** | Secure infrastructure deployment | Local-first, air-gap capable, git-backed state |

### Gap Analysis: ZIG Phases Three and Four

| Phase | Status | Notes |
|-------|--------|-------|
| **Phase Three** | Partial alignment | Advanced threat hunting and automated response in development |
| **Phase Four** | Planned | Continuous optimization and maturity assessment planned for 2027 |

---

## 8. Security Control Summary

### Cryptographic Controls

| Control | Algorithm | Purpose | Implementation |
|---------|-----------|---------|----------------|
| **Certificate Authority** | ECDSA P-256 | PKI hierarchy | `internal/services/gateway/pki_controller.go` |
| **Workload Identity** | SPIFFE URI SAN | Identity binding | `protocol/workload_identity.go` |
| **Transaction Signatures** | Ed25519 | L2 Consensus, L5 Actuator | `internal/services/governance/l2_consensus.go` |
| **Hash Functions** | SHA-256 | Transaction hash, state roots | `docs/architecture/g8e.md` |
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
| **Encryption at Rest** | Audit vault content fields | `internal/services/storage/audit_vault.go` |
| **Encryption in Transit** | All platform communication | mTLS with TLS 1.3 |
| **PII Scrubbing** | Outbound data | `internal/services/sovereignty/boundary.go` |
| **State Binding** | Transaction state | State Merkle roots |
| **Replay Protection** | Transaction nonces | `internal/services/governance/l4_warden.go` |
| **Audit Trail** | All mutations | `internal/services/storage/audit_vault.go` |

---

## 9. Gap Analysis and Roadmap

### Current Strengths

- **Fail-closed verification pipeline** with 5-layer interlock
- **Cryptographic proof chains** with Ed25519 signatures
- **Local-first data sovereignty** with git-backed ledger
- **Comprehensive audit trail** with signed receipts
- **mTLS everywhere** with SPIFFE workload identity
- **Hardware-bound human authentication** via WebAuthn/FIDO2
- **PII/secret scrubbing** before cloud transmission
- **Certificate revocation** with CRL and database denylist
- **Air-gap capable** with no external dependencies

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

---

## 10. Evidence Repository

### Documentation

| Document | Location | Purpose |
|---------|----------|---------|
| **Security Policy** | `/SECURITY.md` | Security posture and vulnerability reporting |
| **Architecture: Protocol** | `/docs/architecture/g8e.md` | Protocol specification and verification layers |
| **Architecture: Auth** | `/docs/architecture/auth.md` | Authentication and authorization architecture |
| **Architecture: Operator** | `/docs/architecture/operator.md` | Operator execution boundary |
| **Architecture: Gateway** | `/docs/architecture/gateway.md` | Gateway policy decision point |
| **Position Paper** | `/docs/core/position_paper.md` | Platform philosophy and security model |

### Code Evidence

| Component | Location | Compliance Relevance |
|-----------|----------|---------------------|
| **PKI Controller** | `/internal/services/gateway/pki_controller.go` | Certificate issuance, revocation, CRL |
| **Audit Vault** | `/internal/services/storage/audit_vault.go` | Audit logging, encryption, retention |
| **Sovereignty Boundary** | `/internal/services/sovereignty/boundary.go` | PII scrubbing, data sovereignty |
| **L1 Doctrine** | `/internal/services/governance/l1_doctrine.go` | Threat detection, input validation |
| **L2 Consensus** | `/internal/services/governance/l2_consensus.go` | Cryptographic verification |
| **L3 Notary** | `/internal/services/governance/l3_notary.go` | Human authorization |
| **L4 Warden** | `/internal/services/governance/l4_warden.go` | Pre-dispatch verification |
| **L5 Actuator** | `/internal/services/governance/l5_actuator.go` | Execution boundary, signed receipts |
| **Workload Identity** | `/protocol/workload_identity.go` | SPIFFE identity specification |

### Test Evidence

| Test Suite | Location | Coverage |
|------------|----------|----------|
| **PKI Tests** | `/internal/services/gateway/pki_controller_test.go` | Certificate issuance, revocation |
| **Audit Vault Tests** | `/internal/services/storage/audit_vault_test.go` | Audit logging, encryption |
| **Sovereignty Tests** | `/internal/services/sovereignty/boundary_test.go` | PII scrubbing, rehydration |
| **Governance Tests** | `/internal/services/governance/*_test.go` | L1-L5 verification |
| **Integration Tests** | `/test/*_test.go` | End-to-end security flows |

---

## 11. Contact and Support

### Security Contact

- **Email:** security@lateraluslabs.com
- **Vulnerability Reporting:** Coordinated disclosure policy in `SECURITY.md`
- **Response Time:** 48 hours acknowledgment, 5 business days initial assessment

### General Contact

- **Email:** hello@lateraluslabs.com
- **Website:** https://lateraluslabs.com
- **Documentation:** https://github.com/g8e-ai/g8e

### Compliance Inquiries

For specific compliance questions or audit support, contact:
- **Email:** compliance@lateraluslabs.com

---

## 12. Document Control

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-06-02 | Lateralus Labs | Initial compliance alignment report |

---

## Appendix A: Glossary

- **L1-L5:** 5-layer verification sequence (Doctrine, Consensus, Notary, Warden, Actuator)
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

---

*This document is maintained by Lateralus Labs and reflects the security posture of g8e as of the version date. For the most current information, refer to the latest version in the repository.*
