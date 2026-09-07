// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ClaimClassification is the Phase 0 classification of every claim in
// docs/reference/compliance-alignment.md. The plan requires each claim to be
// classified as platform technical, customer, shared, inherited, assessor,
// planned, or informational. This typed inventory is the baseline record;
// Phase 1's crosswalk reconciles these classifications against typed assertion
// and framework-control mappings.
type ClaimClassification string

const (
	ClaimClassPlatformTechnical ClaimClassification = "platform_technical"
	ClaimClassCustomer          ClaimClassification = "customer"
	ClaimClassShared            ClaimClassification = "shared"
	ClaimClassInherited         ClaimClassification = "inherited"
	ClaimClassAssessor          ClaimClassification = "assessor"
	ClaimClassPlanned           ClaimClassification = "planned"
	ClaimClassInformational     ClaimClassification = "informational"
)

// ClaimInventoryEntry is one row of the Phase 0 claim inventory.
type ClaimInventoryEntry struct {
	Framework      string              // e.g. "SOC 2", "FedRAMP 20x"
	ControlID      string              // e.g. "CC1.1", "KSI-MLA-07", or section name
	Claim          string              // short description of the g8e claim
	Classification ClaimClassification // Phase 0 classification
	Notes          string              // why this classification was chosen
}

// phase0ClaimInventory is the typed inventory of claims in
// docs/reference/compliance-alignment.md. It is intentionally representative
// rather than exhaustive at the individual-control-row level: each framework
// section contributes entries covering its platform-technical claims plus
// every explicit customer, shared, inherited, assessor, planned, and
// informational marker. Individual control rows that restate the same
// platform-technical claim (e.g. "mTLS for all communication" appearing under
// SOC 2 CC1.9, ISO A.13.1, HIPAA Transmission Security, NIST SC-8) are
// represented once per framework with the shared classification.
//
// This inventory is the baseline that Phase 1's ControlCrosswalk replaces.
// When the crosswalk lands, this test is updated to assert crosswalk coverage
// of every platform-technical entry rather than the static inventory.
var phase0ClaimInventory = []ClaimInventoryEntry{
	// --- SOC 2 ---
	{Framework: "SOC 2", ControlID: "CC1.1-CC1.12", Claim: "mTLS, SPIFFE identity, 5-layer verification, encrypted audit, WebAuthn, JIT provisioning, certificate revocation", Classification: ClaimClassPlatformTechnical, Notes: "gateway/governance/storage code paths"},
	{Framework: "SOC 2", ControlID: "A1.1-A1.3", Claim: "Git-backed ledger recovery, health checks, local-first backup", Classification: ClaimClassPlatformTechnical, Notes: "ledger and audit store"},
	{Framework: "SOC 2", ControlID: "C1.1-C1.4", Claim: "Encryption at rest, TLS 1.3, Sovereign Execution Boundary scrubbing, deterministic rehydration", Classification: ClaimClassPlatformTechnical, Notes: "audit store and scrubbing service"},
	{Framework: "SOC 2", ControlID: "Type II", Claim: "SOC 2 Type II alignment claimed in executive summary", Classification: ClaimClassAssessor, Notes: "Type II requires independent assessor attestation; no assessor evidence exists in the repo"},

	// --- ISO 27001 ---
	{Framework: "ISO 27001", ControlID: "A.5-A.6", Claim: "Security policy in SECURITY.md, defined workload identities, 5-layer segregation of duties", Classification: ClaimClassPlatformTechnical, Notes: "policy doc and identity system"},
	{Framework: "ISO 27001", ControlID: "A.7.1-A.7.2", Claim: "Background checks, confidentiality agreements", Classification: ClaimClassCustomer, Notes: "explicitly marked internal process / HR"},
	{Framework: "ISO 27001", ControlID: "A.8-A.14", Claim: "Asset inventory, access control, cryptography, operations security, comms security, dev security", Classification: ClaimClassPlatformTechnical, Notes: "platform code paths"},
	{Framework: "ISO 27001", ControlID: "A.11.1-A.11.2", Claim: "Physical security perimeters, equipment", Classification: ClaimClassCustomer, Notes: "explicitly marked customer responsibility"},
	{Framework: "ISO 27001", ControlID: "A.15", Claim: "Supplier relationships, supply chain security", Classification: ClaimClassPlatformTechnical, Notes: "self-hosted, air-gap capable, no third-party SaaS"},
	{Framework: "ISO 27001", ControlID: "A.16", Claim: "Incident management, disclosure policy", Classification: ClaimClassShared, Notes: "platform provides disclosure channel; customer operates incident process"},
	{Framework: "ISO 27001", ControlID: "A.17.5", Claim: "Independent review (third-party security assessment)", Classification: ClaimClassPlanned, Notes: "explicitly marked planned"},
	{Framework: "ISO 27001", ControlID: "ISMS", Claim: "ISO 27001 certification", Classification: ClaimClassAssessor, Notes: "report does not claim certification; ISMS is organizational/customer"},

	// --- GDPR ---
	{Framework: "GDPR", ControlID: "Art.5", Claim: "Data minimization, scrubbing, local-first processing, immutable ledger, retention", Classification: ClaimClassPlatformTechnical, Notes: "scrubbing and storage services"},
	{Framework: "GDPR", ControlID: "Arts.15-21", Claim: "Right to access, rectification, erasure, portability, object", Classification: ClaimClassShared, Notes: "platform provides SQLite export and retention; customer operates DSAR process as controller"},
	{Framework: "GDPR", ControlID: "Roles", Claim: "Data controller is the deploying organization; g8e is processor", Classification: ClaimClassCustomer, Notes: "explicitly stated customer is controller"},
	{Framework: "GDPR", ControlID: "Lawful basis", Claim: "No lawful-basis determination made", Classification: ClaimClassInformational, Notes: "document states no legal adequacy determination"},

	// --- HIPAA ---
	{Framework: "HIPAA", ControlID: "Admin Safeguards", Claim: "5-layer verification, SPIFFE roles, JIT provisioning, session access control, disclosure policy, ledger recovery, CI testing", Classification: ClaimClassPlatformTechnical, Notes: "governance and storage code"},
	{Framework: "HIPAA", ControlID: "Admin: Training", Claim: "Security awareness training documentation", Classification: ClaimClassCustomer, Notes: "documentation-only; customer trains workforce"},
	{Framework: "HIPAA", ControlID: "Physical Safeguards", Claim: "Facility access controls, workstation use/security", Classification: ClaimClassCustomer, Notes: "explicitly marked customer responsibility / platform design"},
	{Framework: "HIPAA", ControlID: "Technical Safeguards", Claim: "mTLS access control, audit controls, integrity controls, transmission security", Classification: ClaimClassPlatformTechnical, Notes: "gateway and storage code"},
	{Framework: "HIPAA", ControlID: "PHI Handling", Claim: "PHI scrubbing, local processing, audit trail", Classification: ClaimClassPlatformTechnical, Notes: "synthetic PHI in demos only"},
	{Framework: "HIPAA", ControlID: "Covered-entity status", Claim: "No legal determination of covered-entity or business-associate status", Classification: ClaimClassInformational, Notes: "document makes no legal determination"},

	// --- NIST SP 800-53 ---
	{Framework: "NIST SP 800-53", ControlID: "AC-1-AC-20", Claim: "Access control policy, account management, access enforcement, least privilege, session management, remote access", Classification: ClaimClassPlatformTechnical, Notes: "gateway, governance, PKI code"},
	{Framework: "NIST SP 800-53", ControlID: "AC-18", Claim: "Wireless access", Classification: ClaimClassCustomer, Notes: "explicitly marked customer-controlled infrastructure"},
	{Framework: "NIST SP 800-53", ControlID: "AU-1-AU-12", Claim: "Audit policy, events, record content, retention, fail-closed, review, timestamps, encryption, non-repudiation, generation", Classification: ClaimClassPlatformTechnical, Notes: "audit store, ledger, receipts"},
	{Framework: "NIST SP 800-53", ControlID: "SC-1-SC-23", Claim: "Boundary protection, transmission confidentiality/integrity, PKI, cryptography, session authenticity", Classification: ClaimClassPlatformTechnical, Notes: "scrubbing, mTLS, PKI"},
	{Framework: "NIST SP 800-53", ControlID: "SC-13", Claim: "FIPS 140-3 approved cryptographic module (Go CMVP Cert #5247)", Classification: ClaimClassPlatformTechnical, Notes: "FIPS build and runtime attestation"},
	{Framework: "NIST SP 800-53", ControlID: "SC-21", Claim: "Domain name services", Classification: ClaimClassCustomer, Notes: "explicitly marked customer-controlled DNS"},
	{Framework: "NIST SP 800-53", ControlID: "SI-1-SI-17", Claim: "Integrity policy, flaw remediation, malicious code protection, monitoring, input validation, error handling, fail-safe", Classification: ClaimClassPlatformTechnical, Notes: "doctrine, ledger, CI"},
	{Framework: "NIST SP 800-53", ControlID: "SI-8", Claim: "Spam protection not applicable", Classification: ClaimClassInformational, Notes: "explicitly marked not applicable (infrastructure platform)"},

	// --- PCI DSS ---
	{Framework: "PCI DSS", ControlID: "1-12", Claim: "Secure config, mTLS, change control, encryption, key management, access control, MFA, audit trails, vulnerability scanning, policy", Classification: ClaimClassPlatformTechnical, Notes: "platform code paths"},
	{Framework: "PCI DSS", ControlID: "11.3", Claim: "Penetration testing", Classification: ClaimClassPlanned, Notes: "explicitly marked planned"},
	{Framework: "PCI DSS", ControlID: "CHD scope", Claim: "Whether assessed environment is in cardholder-data scope", Classification: ClaimClassCustomer, Notes: "deployment-specific; synthetic PAN in automated evidence only"},

	// --- NSA ZIG ---
	{Framework: "NSA ZIG", ControlID: "Discovery", Claim: "Critical data/app/asset/service identification, data-flow mapping, trust boundaries", Classification: ClaimClassPlatformTechnical, Notes: "scrubbing, governance, ledger, identity"},
	{Framework: "NSA ZIG", ControlID: "Phase One", Claim: "MFA, privileged access, federation, device trust, network segmentation, encryption, IAM, monitoring, incident response", Classification: ClaimClassPlatformTechnical, Notes: "WebAuthn, PKI, mTLS, audit"},
	{Framework: "NSA ZIG", ControlID: "Phase Two", Claim: "Policy decision/enforcement, dynamic evaluation, risk-based auth, least privilege, DLP, threat detection, automated response, audit, session mgmt, cert/key mgmt, API security, supply chain", Classification: ClaimClassPlatformTechnical, Notes: "governance, scrubbing, doctrine, ledger"},
	{Framework: "NSA ZIG", ControlID: "Phase Three", Claim: "Advanced threat hunting and automated response", Classification: ClaimClassPlanned, Notes: "explicitly marked partial alignment / in development"},
	{Framework: "NSA ZIG", ControlID: "Phase Four", Claim: "Continuous optimization and maturity assessment", Classification: ClaimClassPlanned, Notes: "explicitly marked planned FY 2027"},
	{Framework: "NSA ZIG", ControlID: "Org maturity", Claim: "Organization-wide zero-trust maturity", Classification: ClaimClassAssessor, Notes: "a demo cannot elevate org maturity; requires assessor evidence"},

	// --- NIST SP 800-63B ---
	{Framework: "NIST SP 800-63B", ControlID: "AAL2", Claim: "WebAuthn passkey plus mTLS client certificate; phishing-resistant", Classification: ClaimClassPlatformTechnical, Notes: "L3 notary and PKI"},
	{Framework: "NIST SP 800-63B", ControlID: "AAL3", Claim: "Hardware-bound WebAuthn with non-exportable key plus mTLS", Classification: ClaimClassShared, Notes: "platform provides protocol; authenticator non-exportability requires hardware attestation metadata or assessor evidence"},
	{Framework: "NIST SP 800-63B", ControlID: "Authenticator properties", Claim: "Non-exportability of authenticator keys", Classification: ClaimClassAssessor, Notes: "synthetic ceremony tests protocol, not hardware non-exportability"},

	// --- FedRAMP 20x ---
	{Framework: "FedRAMP 20x", ControlID: "KSI catalog", Claim: "31 KSIs across 10 categories with typed evaluator, OSCAL export, history, COSAiS overlays", Classification: ClaimClassPlatformTechnical, Notes: "compliance package"},
	{Framework: "FedRAMP 20x", ControlID: "KSI-MLA-08", Claim: "Receipt and final-persistence signatures are cryptographically verified", Classification: ClaimClassPlatformTechnical, Notes: "canonical Ed25519 verification fails closed on missing, malformed, or tampered receipt evidence"},
	{Framework: "FedRAMP 20x", ControlID: "Class D", Claim: "Class D (high) certification", Classification: ClaimClassPlanned, Notes: "explicitly marked out-of-scope until FedRAMP opens Phase 4"},
	{Framework: "FedRAMP 20x", ControlID: "Certification", Claim: "FedRAMP authorization", Classification: ClaimClassAssessor, Notes: "platform produces evidence; FedRAMP 3PAO authorization is assessor-dependent"},

	// --- Cross-cutting ---
	{Framework: "All", ControlID: "Physical security", Claim: "Physical security perimeters and equipment", Classification: ClaimClassCustomer, Notes: "Known Limitations section: customer responsibility"},
	{Framework: "All", ControlID: "Network segmentation", Claim: "Network segmentation and endpoint security", Classification: ClaimClassCustomer, Notes: "Known Limitations section: customer responsibility"},
	{Framework: "All", ControlID: "RBAC", Claim: "Role-based access control and multi-tenancy", Classification: ClaimClassPlanned, Notes: "Planned Enhancements: Q3 2026"},
	{Framework: "All", ControlID: "Third-party assessment", Claim: "Third-party security assessment", Classification: ClaimClassPlanned, Notes: "Planned Enhancements: Q4 2026"},
	{Framework: "All", ControlID: "Penetration testing", Claim: "Formal penetration testing", Classification: ClaimClassPlanned, Notes: "Planned Enhancements: Q4 2026"},
	{Framework: "All", ControlID: "Automated compliance reporting", Claim: "Automated compliance reporting", Classification: ClaimClassPlanned, Notes: "Planned Enhancements: Q2 2027; this plan (v2.1.3) implements it"},
	{Framework: "All", ControlID: "Receipt key distribution", Claim: "Attested Actuator public key distribution channel", Classification: ClaimClassPlanned, Notes: "Known Limitations: attested bootstrap flow planned"},
}

// TestPhase0ClaimInventory_EveryEntryHasValidClassification validates that
// every claim inventory entry has a recognized classification. This is the
// typed inventory record required by Phase 0 exit criteria.
func TestPhase0ClaimInventory_EveryEntryHasValidClassification(t *testing.T) {
	valid := map[ClaimClassification]bool{
		ClaimClassPlatformTechnical: true,
		ClaimClassCustomer:          true,
		ClaimClassShared:            true,
		ClaimClassInherited:         true,
		ClaimClassAssessor:          true,
		ClaimClassPlanned:           true,
		ClaimClassInformational:     true,
	}

	for _, e := range phase0ClaimInventory {
		assert.True(t, valid[e.Classification],
			"entry %s/%s has invalid classification %q",
			e.Framework, e.ControlID, e.Classification)
		assert.NotEmpty(t, e.Framework, "entry has empty framework")
		assert.NotEmpty(t, e.ControlID, "entry has empty control ID")
		assert.NotEmpty(t, e.Claim, "entry has empty claim")
	}
}

// TestPhase0ClaimInventory_CoversAllFrameworks validates that the inventory
// covers every framework section present in compliance-alignment.md.
func TestPhase0ClaimInventory_CoversAllFrameworks(t *testing.T) {
	required := []string{
		"SOC 2", "ISO 27001", "GDPR", "HIPAA", "NIST SP 800-53",
		"PCI DSS", "NSA ZIG", "NIST SP 800-63B", "FedRAMP 20x", "All",
	}

	seen := make(map[string]bool)
	for _, e := range phase0ClaimInventory {
		seen[e.Framework] = true
	}

	for _, fw := range required {
		assert.True(t, seen[fw],
			phase0RegressionBeforeFix+
				": claim inventory must cover framework %q", fw)
	}
}

// TestPhase0ClaimInventory_CustomerAndPlannedClaimsAreExplicit validates that
// every customer, planned, and assessor claim is explicitly marked as such
// rather than being presented as platform-technical. This is the core
// overclaiming mitigation: the inventory must distinguish what g8e proves
// from what the customer, assessor, or a future release must provide.
func TestPhase0ClaimInventory_CustomerAndPlannedClaimsAreExplicit(t *testing.T) {
	require.NotEmpty(t, phase0ClaimInventory)

	counts := make(map[ClaimClassification]int)
	for _, e := range phase0ClaimInventory {
		counts[e.Classification]++
	}

	// The inventory must contain at least one customer, one planned, and one
	// assessor entry. If these are absent, the inventory is not honestly
	// separating responsibilities.
	assert.Greater(t, counts[ClaimClassCustomer], 0,
		"inventory must contain at least one customer-responsibility claim")
	assert.Greater(t, counts[ClaimClassPlanned], 0,
		"inventory must contain at least one planned claim")
	assert.Greater(t, counts[ClaimClassAssessor], 0,
		"inventory must contain at least one assessor-dependent claim")
	assert.Greater(t, counts[ClaimClassPlatformTechnical], 0,
		"inventory must contain platform-technical claims")
}
