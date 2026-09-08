# Compliance Alignment

**Document Version:** 2.1.7
**Last Updated:** 2026-09-08
**Platform:** g8e v2.1.7
**Maintained by:** Lateralus Labs, LLC.

## Purpose and claim boundary

This document identifies the machine-maintained sources for g8e compliance mappings and explains how verified evidence becomes assertion assessments, framework-control assessments, framework profiles, and rendered reports. It does not duplicate control status because status is derived for a specific assessment scope, evidence window, catalog version, and verifier version. The generated report for an assessment is the authoritative status record.

g8e does not claim certification, accreditation, authorization, legal compliance, auditor approval, or organization-wide maturity. A satisfied technical assertion reports the result of its declared evidence and verifier contract for one scope and time window. It does not satisfy customer-operated, inherited, organizational, legal, or assessor-dependent requirements. Missing, stale, malformed, unsupported, cross-scope, or untrusted evidence fails closed and remains explicit in generated analysis.

## Maintained sources of truth

| Source | Canonical location | Content |
| --- | --- | --- |
| Atomic assertions | [`protocol/constants/compliance/assertion-catalog.json`](../../protocol/constants/compliance/assertion-catalog.json) | Versioned framework-neutral technical assertions, responsibility, evidence requirements, grader and verifier references, evidence level, validation cycle, missing-evidence policy, and passing rule |
| Framework catalog | [`protocol/constants/compliance/framework-catalog.json`](../../protocol/constants/compliance/framework-catalog.json) | Versioned framework controls, responsibility, source references, support status, and support rationale |
| Reviewed crosswalk | [`protocol/constants/compliance/fedramp-nist-crosswalk.json`](../../protocol/constants/compliance/fedramp-nist-crosswalk.json) | Versioned mappings from framework controls to atomic assertions, including mapping type, evidence level, responsibility, reviewer, and review time |
| Demo scenario catalog | [`protocol/constants/compliance/demo-scenario-catalog.json`](../../protocol/constants/compliance/demo-scenario-catalog.json) | Versioned evidence-producing scenarios with assertion and framework-control references |
| Protocol contract | [`protocol/proto/g8e/compliance/v1/compliance.proto`](../../protocol/proto/g8e/compliance/v1/compliance.proto) | Typed scope, evidence, assessment, analysis, profile, report-manifest, signature, and verification records |
| Generated protocol reference | [`protocol/docs/reference/api/g8e/compliance/v1/index.md`](../../protocol/docs/reference/api/g8e/compliance/v1/index.md) | Generated field reference for the compliance protocol |
| FedRAMP KSI catalog | [`docs/reference/ksi-catalog.json`](./ksi-catalog.json) | Typed FedRAMP KSI definitions and method requirements used by the KSI evaluator |

The JSON catalogs are canonical compact documents with embedded identities, versions, and SHA-256 digests. The loader and validators in [`internal/services/compliance/catalog/`](../../internal/services/compliance/catalog/) reject malformed identities, versions, digests, responsibilities, support statuses, mappings, evidence levels, unsupported references, and duplicate records. Catalog totals and per-control support classifications are read from these files rather than copied into this page.

## Maintained framework scope

The canonical framework catalog and reviewed crosswalk currently cover FedRAMP 20x CR26 and NIST SP 800-53 Rev. 5. Their exact framework versions, controls, support classifications, responsibilities, and mappings are defined in the framework catalog and crosswalk linked above.

SOC 2 Trust Services Criteria, ISO/IEC 27001, HIPAA Security Rule, PCI DSS, GDPR, NIST SP 800-63B, and NSA Zero Trust Implementation Guidelines do not yet have reviewed canonical catalogs and crosswalks in this repository. g8e therefore does not emit framework-control assessments or framework profiles for them. Architecture features may be relevant to those standards, but relevance is not a machine-evaluated control outcome and is not presented as one here.

## Assessment pipeline

The proof-backed reporting path separates collection, verification, grading, analysis, profiling, and rendering:

1. Read-only importers decode persisted demo, eval, receipt, persistence, audit, commitment, ledger, KSI-history, build/configuration, and signed attestation evidence.
2. The evidence graph validates canonical digests, content addresses, references, prohibited cycles, assessed trust, encryption metadata, freshness, and scope, run, attempt, scenario, transaction, and evidence-window binding.
3. `assertion_assessment@1.0.0` evaluates atomic assertions only from verified, scope-bound evidence.
4. `framework_assessment@1.0.0` projects assertion assessments through the reviewed crosswalk without changing the underlying assertion outcomes.
5. `BuildComplianceAnalysis` creates the canonical cross-framework analysis, including evidence-window completeness, gaps, evidence links, limitations, findings, remediation, evidence resources, and explicit responsibility and outcome sections.
6. `BuildFrameworkProfiles` projects the canonical control assessments into one deterministic profile per catalog framework without re-grading or changing their outcomes.
7. The shared renderer emits canonical JSON, OSCAL JSON, Markdown, HTML, and CLI views from the same analysis.

Implementation boundaries are in [`internal/services/compliance/evidence/`](../../internal/services/compliance/evidence/), [`internal/services/compliance/report/`](../../internal/services/compliance/report/), and [`internal/services/compliance/oscal.go`](../../internal/services/compliance/oscal.go). The [Proof-Backed Compliance Evidence](./compliance-evidence.md) reference explains evidence levels, persisted evidence, independent verification, and remaining bundle work.

## Status semantics

Generated assertion and control assessments use typed outcomes rather than prose confidence labels:

| Status | Meaning |
| --- | --- |
| `satisfied` | Verified eligible evidence satisfies the complete declared rule for the assessed scope and window |
| `not_satisfied` | Verified evidence measures a failure of the declared rule |
| `not_applicable` | The typed applicability or missing-evidence policy excludes the assertion or control from satisfaction |
| `unverifiable` | Required evidence is missing, stale, malformed, unsupported, ambiguous, untrusted, or otherwise cannot support a result |
| `customer_attestation_required` | Platform evidence cannot satisfy the customer- or assessor-operated requirement |

Framework mappings retain `full`, `partial`, `supporting`, and `not_applicable` semantics. Partial and supporting mappings preserve explicit limitations and do not elevate supporting technical evidence into a full framework-control claim. Generated analyses also retain platform, customer, shared, inherited, assessor-required, planned, unsupported, failed, stale, and unverifiable sections.

## Generate and verify evidence

Validate persisted demo and eval evidence as one content-addressed graph:

```bash
g8e compliance evidence-graph verify \
  --demo-run <demo-run-id> \
  --eval-run <eval-run-id>
```

Generate and persist a signed report bundle from an explicit scope and evidence window. Repeat `--demo-run` and `--eval-run` as needed, select the `public` or `restricted` profile, and supply the dedicated compliance-report signing identity:

```bash
g8e compliance report generate \
  --scope-id <scope-id> \
  --demo-run <demo-run-id> \
  --eval-run <eval-run-id> \
  --window-start-unix-ms <inclusive-start> \
  --window-end-unix-ms <inclusive-end> \
  --report-id <report-id> \
  --profile public \
  --signing-metadata <signing-metadata.json> \
  --signing-private-key <signing-private-key.hex>
```

Verify the persisted bundle independently with external assessed report trust and, when signed source evidence is represented, separately assessed evidence-signer trust:

```bash
g8e compliance report verify <bundle-manifest.json> \
  --trust-policy <assessed-report-trust.json> \
  --evidence-trust <assessed-evidence-trust.json>
```

Verify one persisted demo run independently:

```bash
g8e compliance demo-run verify <run-id>
```

`compliance report generate` reads persisted demo and eval evidence plus explicit KSI, commitment, customer or assessor attestation, audit, ledger, and build/configuration inputs without mutating assessed state. It copies exact source bytes into canonical protected paths, assembles canonical analysis, framework profiles, and rendered formats into an immutable signed bundle, persists protected bodies, and writes the canonical descriptor last. `compliance report verify` is read-only and offline: it requires report trust outside the bundle, independently requires evidence-signer trust for represented signed sources, verifies directory integrity, protected bodies, checksum roots, and both signatures, replays every represented source route through the registered verifier or importer, compares reproduced evidence with signed analysis, and reproduces every renderer. The [v2.1.7 clean offline acceptance record](../release_notes/v2.1.x/v2.1.7-offline-acceptance.md) identifies the network-disabled environment, exact candidate and trust digests, successful verification report, and rejected source, renderer, and signature mutations.

## Generated artifacts

Assessment results belong in generated artifacts, not this document. The repository currently retains these generated release-evidence projections:

- [v2.1.7 release evidence (Markdown)](../release_notes/v2.1.x/v2.1.7-compliance-evidence.md)
- [v2.1.7 release evidence (CSV)](../release_notes/v2.1.x/v2.1.7-compliance-evidence.csv)
- [v2.1.7 clean offline acceptance](../release_notes/v2.1.x/v2.1.7-offline-acceptance.md)
- [v2.1.5 release evidence (Markdown)](../release_notes/v2.1.x/v2.1.5-compliance-evidence.md)
- [v2.1.5 release evidence (CSV)](../release_notes/v2.1.x/v2.1.5-compliance-evidence.csv)
- [v2.1.4 release evidence (Markdown)](../release_notes/v2.1.x/v2.1.4-compliance-evidence.md)
- [v2.1.4 release evidence (CSV)](../release_notes/v2.1.x/v2.1.4-compliance-evidence.csv)
- [Current public README evidence index](../evidence/readme/current/index.json)

The release-evidence files aggregate live KSI results, KSI history inventory, and independently verified demo runs. They predate the current signed report-bundle implementation and are not substitutes for canonical `ComplianceAnalysis`, deterministic framework profiles, or offline bundle verification. Each generated artifact carries its own release, generation time, scope-related inputs, and claim boundaries; later evidence does not rewrite an earlier result.

## Responsibility boundaries

The framework catalog records responsibility per control. Generated reports preserve these categories:

- **Platform:** g8e-operated technical behavior can be evaluated from verified platform evidence.
- **Customer:** deployment, workforce, policy, physical, organizational, or business-process evidence is supplied and operated by the customer.
- **Shared:** platform evidence covers only the declared technical portion; customer evidence remains required for the customer-operated portion.
- **Inherited:** satisfaction depends on an external provider or inherited control with explicit assessed evidence.
- **Assessor:** an independent assessor or authority supplies the acceptance decision or attestation.

Public reports contain references and safe projections, not restricted plaintext. Restricted evidence uses authenticated encryption metadata, explicit authorization scope, and independently verified digests. A packaged public key is not trusted by inclusion; assessed trust metadata establishes verifier and signer trust.

## Interpreting alignment

Use the canonical files and generated output in this order:

1. Read the framework catalog to determine whether a control is mapped, planned, or unsupported and who owns it.
2. Read the reviewed crosswalk to identify the exact atomic assertions, mapping type, evidence requirement, and review metadata.
3. Read the generated framework-control assessment or framework profile for the assessed scope and window.
4. Resolve its assertion, evidence, finding, limitation, and remediation references through the canonical analysis and evidence graph.
5. Verify the source evidence and signatures independently before relying on the result.

Do not infer satisfaction from source-code presence, test names, demo terminal output, a successful process exit, a framework mentioned in prose, or a control appearing in the catalog. Only a verified generated assessment records an outcome.

## Related references

- [Proof-Backed Compliance Evidence](./compliance-evidence.md)
- [FIPS 140-3 Compliance](./fips140-3.md)
- [Governance Architecture](../architecture/governance.md)
- [Authentication Architecture](../architecture/auth.md)
- [Storage Architecture](../architecture/storage.md)
- [Protocol Specification](../../protocol/docs/spec.md)
- [FedRAMP Demo](../../demos/fedramp/README.md)
- [Healthcare Demo](../../demos/healthcare/README.md)
- [Finance Demo](../../demos/finance/README.md)
- [DHS Demo](../../demos/dhs/README.md)

## Contact

Security reports follow [`.github/SECURITY.md`](../../.github/SECURITY.md). Compliance inquiries can be sent to compliance@lateraluslabs.com.
