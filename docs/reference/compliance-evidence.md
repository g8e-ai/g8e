# Proof-Backed Compliance Evidence

**Document Version:** 2.1.7
**Last Updated:** 2026-09-08
**Platform:** g8e v2.1.7
**Maintained by:** Lateralus Labs, LLC.

---

## Purpose

This document is the evidence companion to the [Compliance Alignment Report](./compliance-alignment.md). The alignment report maps g8e's security controls to external frameworks (SOC 2, ISO 27001, GDPR, HIPAA, NIST SP 800-53, PCI DSS, NIST SP 800-63B, NSA ZIG, FedRAMP 20x). This document describes the proof-backed reporting pipeline that binds those mappings to immutable, independently verifiable evidence rather than prose claims.

The pipeline answers five questions for every reported control:

1. What exact technical assertion does g8e make?
2. Which standards requirements map to that assertion, under which framework versions?
3. What immutable evidence proves or disproves that assertion for the assessed deployment and time window?
4. Which versioned verifier evaluated the evidence, and can an independent environment reproduce the result?
5. What remains outside g8e's scope or requires customer or assessor evidence?

---

## Claim boundaries

This document does not claim certification, accreditation, authorization, legal compliance, or auditor approval. It states which technical assertions were evaluated, in what scope, against which framework versions, using which evidence and verifier versions, at what time, with what result.

The pipeline distinguishes demonstrated technical control operation from organizational compliance, customer responsibility, and third-party certification. Platform tests cannot satisfy customer-operated process controls. A demo scenario that passes against a real stack demonstrates technical operation for one scope and one point in time; it does not establish operating effectiveness over an assessment period. Customer, shared, inherited, and assessor-dependent controls remain explicitly separated from platform-operated technical controls and never inherit a passing status from platform evidence.

---

## Architecture

The proof-backed reporting pipeline separates evidence collection, verification, grading, and rendering into distinct stages so that no renderer independently recomputes an assessment decision and no compliance result depends on terminal text or an unverified process exit code.

```text
Demo runner ───────────────┐
                           │
Eval runner ───────────────┼──> typed evidence collectors ──> evidence graph
                           │                                      │
Audit/ledger/KSI stores ───┘                                      ├──> artifact verifiers
                                                                  ├──> assertion graders
Framework catalogs/crosswalks ─────────────────────────────────────┤
                                                                  ├──> framework assessments
                                                                  ├──> release gates
                                                                  └──> canonical report record
                                                                         │
                                                                         ├──> OSCAL
                                                                         ├──> JSON
                                                                         ├──> Markdown
                                                                         ├──> HTML
                                                                         └──> CLI
```

Atomic g8e assertions are the source of behavioral truth. External framework controls map to those assertions through typed crosswalks. A single verified assertion can support multiple framework controls, while each framework control retains its own applicability, scope, responsibility, and evidence requirements. Demos, evals, KSI methods, OSCAL, Markdown, HTML, and CLI output all consume one immutable evidence graph; renderers project one canonical assessment record into each format rather than scraping terminal output or inferring results from filenames.

The pipeline fails closed with explicit uncertainty. Unknown framework versions, unknown assertion or verifier versions, missing trust roots, stale evidence, duplicate records, cross-run evidence, scope mismatches, malformed signatures, unresolved references, unsupported task shapes, and incomplete assessment periods produce non-passing results, and the report preserves the exact reason. The `unverifiable` result status records missing, stale, malformed, unsupported, ambiguous, or cryptographically invalid evidence without inventing a measured control failure.

---

## Atomic control assertions

The assertion catalog (`protocol/constants/compliance/assertion-catalog.json`) defines 13 atomic, framework-neutral assertions. Each assertion declares a category, component scope, responsibility, applicable action classes and arms, required evidence types, required grader and verifier references, minimum evidence level, validation cycle, missing-evidence policy, and passing rule. The catalog is content-addressed and versioned independently of the platform release.

| Assertion ID | Title | Minimum evidence level |
|--------------|-------|------------------------|
| `G8E-GOV-ALLOW-001` | Governed allow outcome | L3 |
| `G8E-GOV-BLOCK-001` | Governed block outcome | L3 |
| `G8E-AU-RECEIPT-001` | Receipt integrity | L3 |
| `G8E-AU-PERSIST-001` | Receipt persistence | L3 |
| `G8E-AU-CHAIN-001` | Protocol chain integrity | L3 |
| `G8E-AU-COMMIT-001` | Commitment integrity | L3 |
| `G8E-CM-STATE-001` | Independently observed state | L3 |
| `G8E-DLP-DETECT-001` | Sensitive-data detection | L3 |
| `G8E-DLP-BOUNDARY-001` | Model-boundary leakage | L3 |
| `G8E-DLP-REHYDRATE-001` | Local rehydration | L3 |
| `G8E-IA-MTLS-001` | Workload mTLS | L3 |
| `G8E-IA-NOTARY-001` | Notary authorization | L3 |
| `G8E-CRYPTO-FIPS-001` | Declared cryptographic mode | L3 |

Catalog identity: `g8e-control-assertions` v1.0.0, SHA-256 `bee48afa18a8c54983c8178718600c4f6b64d9c3f9007e65d6d4b1fabbaff1dd`.

The assertion families cover identity and workload authentication, access enforcement and least privilege, governed allow and block outcomes, exact rejection-layer attribution, receipt integrity and durable persistence, deterministic protocol-chain integrity, commitment and ledger integrity, configuration and file-state correctness, audit evidence preservation, replay and nonce and signer and state-root protections, sensitive-data detection and transformation, model-boundary leakage, local rehydration and token lifecycle, transport protection and cryptographic mode, and availability and recovery and fail-closed dependency behavior.

---

## Evidence levels

A result never receives a higher level merely because lower-level evidence exists. Source references cannot substitute for runtime evidence, demo terminal text cannot substitute for typed scenario records, and a single successful run cannot substitute for effectiveness over a required historical period.

| Level | Name | Meaning |
|-------|------|---------|
| L0 | Documented | Architecture, policy, or implementation reference describes the control. |
| L1 | Implemented | Focused source and test evidence verifies the implementation contract. |
| L2 | Deterministically evaluated | A registered versioned grader passes against typed evidence. |
| L3 | Demonstrated | A real-stack scenario passes with verified receipts and independently observed terminal state. |
| L4 | Continuously evidenced | Fresh historical evidence satisfies the control's declared validation cycle across the assessment window. |
| L5 | Externally attested | An independent assessor or authority accepts the evidence for the declared scope. |

Every assertion in the current catalog declares a minimum evidence level of L3, meaning a satisfied result requires a real-stack demonstration with verified receipts and independently observed terminal state. L4 and L5 are not yet produced by the pipeline; they require historical evidence collection and external attestation respectively.

---

## Framework crosswalk

The canonical crosswalk (`protocol/constants/compliance/fedramp-nist-crosswalk.json`) maps framework controls to atomic assertions. The initial crosswalk targets FedRAMP 20x (CR26-2026-06-24) and NIST SP 800-53 (rev5), the two frameworks for which the repository already has a 31-KSI catalog, Class A-D method requirements, validation cycles, a KSI evaluator, history, OSCAL export, typed doctrine linkages, and a real-stack demo.

Crosswalk identity: SHA-256 `d73eb7fa35d2f3bbd88510a000df6c35daf354f148d4f986feb6a7751e2ba2cd`.

The crosswalk contains 60 conservative `supporting` mappings. Every one of the 13 reviewed assertions has at least one mapping. The framework catalog (`protocol/constants/compliance/framework-catalog.json`) marks 34 controls as mapped and 97 as unsupported across the two frameworks:

| Framework | Version | Controls | Mapped | Unsupported |
|-----------|---------|----------|--------|-------------|
| FedRAMP 20x | CR26-2026-06-24 | 31 | 14 | 17 |
| NIST SP 800-53 | rev5 | 100 | 20 | 80 |
| **Total** | | **131** | **34** | **97** |

Each framework control carries typed `support_status` and `support_rationale` fields so that absence of a reviewed mapping is explicit rather than inferred. Customer-operated controls are called out in the support rationale. A `supporting` mapping cannot yield a full-control satisfaction claim without the other required assertions or attestations. Subsequent framework crosswalks (SOC 2, ISO 27001, HIPAA, PCI DSS, GDPR, NIST SP 800-63B, NSA ZIG) reuse the same assertions rather than introducing separate evidence engines; those crosswalks are planned and not yet in the canonical catalog.

---

## Evidence-grade demo scenarios

The demo scenario catalog (`protocol/constants/compliance/demo-scenario-catalog.json`) defines 14 evidence-grade scenarios across four demo environments. Each definition carries a stable scenario ID and version, display number, purpose, risk category, expected action classes, expected allow or block outcome, expected rejection layer where applicable, initial-state fixture reference, terminal-state assertions, required receipts and deterministic stages, assertion references, framework-control references, required evidence level, timeout, and failure policy.

Catalog identity: `g8e-demo-scenarios` v1.1.0, SHA-256 `aa81ad66b05868580f35b2124f6cda77106a7fccf361a36d0bb8b24c9ffc4120`.

| Scenario ID | Demo | Display | Expected outcome |
|-------------|------|---------|------------------|
| `fedramp-provision` | FedRAMP | 1 | allowed |
| `fedramp-deny` | FedRAMP | 2 | blocked (L1) |
| `fedramp-revert` | FedRAMP | 3 | allowed |
| `fedramp-evidence-block` | FedRAMP | 4 | blocked (L1) |
| `dhs-ingest` | DHS | 1 | allowed |
| `dhs-disconnected-operations` | DHS | 2 | allowed |
| `dhs-cue` | DHS | 3 | allowed |
| `dhs-destruction-block` | DHS | 4a | blocked (L1) |
| `dhs-destruction-purge` | DHS | 4b | allowed |
| `finance-unauthorized-trade` | Finance | 1 | blocked (L1) |
| `healthcare-success` | Healthcare | 1 | allowed |
| `healthcare-gold-card` | Healthcare | 2 | allowed |
| `healthcare-sla-breach` | Healthcare | 3 | allowed |
| `healthcare-phi-blocked` | Healthcare | 4 | blocked (L1) |

Every framework-control reference in every definition has at least one canonical crosswalk mapping to an assertion carried by that definition. Contract tests lock this crosswalk consistency catalog-wide. The catalog is the source of truth for scenario identity, assertion mappings, and framework-control mappings; prose descriptions in the per-demo READMEs are narrative context rather than authoritative.

---

## Persisted demo evidence

Each demo run persists a typed `DemoManifest` and canonical `DemoScenarioResult` records under `.g8e/data/compliance/demo-evidence/<run-id>/`. The manifest binds scenario definitions, framework controls, automated and supported manual-notary execution lanes, required environment, and SHA-256 provenance for the demo compose file, doctrine, target data, and configuration. Scenario results bind run, scope, scenario, attempt, execution, transaction, investigation, receipt, protocol-chain, state-observation, assertion, and framework-control identities.

Demo runs persist canonical action receipts, final-persistence attestations, state observations, and typed metric evidence under SHA-256-addressed paths. Scenario and step records carry resolvable references to those artifacts. The healthcare gold-card and SLA scenarios additionally persist protocol-owned `DemoMetricEvidence` records that bind a registered `healthcare-threshold` grader, run, scope, scenario, source observation, subject, measured value, threshold, unit, comparison, reproduced grade, and evaluation time.

The governed MCP `tools/call` path accepts caller-supplied execution and investigation IDs, returns authoritative signed receipt references for completed calls and failed L1/L2 stages, and carries resumed receipt identity in human-approval completion events. The demo harness retains those identities, resolves canonical receipt and persistence bodies, and grades deterministic protocol chains without synthetic fallback references. Boundary-specific state collectors emit typed, fixture-bound, content-addressed observations independently of process exit status, so terminal claims are measured rather than inferred from command output.

---

## Independent demo-run verification

The `g8e compliance demo-run verify <run-id>` CLI command reads persisted demo evidence and emits a typed `ComplianceVerificationReport` as canonical JSON. The verifier is read-only, exits nonzero when the report is invalid, and never mutates assessed state.

The verifier independently checks:

- Manifest provenance digest matching against the real source tree
- Run, demo, scope, and scenario result correlation
- Canonical scenario result decoding
- Content-addressed receipt and persistence body resolution
- Cryptographic receipt and final-persistence signature verification against assessed Ed25519 keys
- Deterministic-stage protocol-chain grading
- State-observation body binding to scenario steps
- Typed metric source, grader, and grade reproduction
- Artifact directory integrity and root directory enforcement

Missing, malformed, duplicated, unexpected, cross-scope, stale, unsigned, checksum-mismatched, or unresolvable evidence invalidates the report. Demo, eval, standalone receipt and persistence, audit, commitment, ledger, KSI-history, build/configuration, and signed-attestation importers use shared evidence-graph and cryptographic verification primitives. Signed report-bundle generation copies explicit source bytes into canonical protected paths. Complete-bundle verification independently replays protected demo and eval inventories and standalone KSI, commitment, customer and assessor attestation, audit, ledger, and build/configuration sources through their registered importers, compares reproduced evidence resources with signed canonical analysis, and rejects every unknown artifact type without a route.

---

## Demonstrated evidence at v2.1.3

At the v2.1.3 release boundary, all 14 canonical demo scenarios produce evidence-grade results against real Compose topologies with mTLS, governed wire paths, and independent target-state collectors. The FedRAMP, finance, DHS, and healthcare stacks are enrolled and healthy, and their current-binary real-stack runs persist canonical manifests, scenario results, receipt bodies, final-persistence attestations, state-observation artifacts, and (for healthcare gold-card and SLA) typed metric evidence bodies.

Independent `g8e compliance demo-run verify` verification reports `valid: true` with zero failures for:

- All four FedRAMP scenarios (`fedramp-provision`, `fedramp-deny`, `fedramp-revert`, `fedramp-evidence-block`)
- The finance scenario (`finance-unauthorized-trade`)
- All four DHS scenarios (`dhs-ingest`, `dhs-disconnected-operations`, `dhs-cue`, `dhs-destruction-block` + `dhs-destruction-purge` under one run ID)
- All four healthcare scenarios (`healthcare-success`, `healthcare-gold-card`, `healthcare-sla-breach`, `healthcare-phi-blocked`), including the two metric-bearing gold-card and SLA runs

This demonstrates L3 evidence (real-stack scenario with verified receipts and independently observed terminal state) for the 13 atomic assertions exercised by those scenarios. It does not demonstrate L4 (continuous effectiveness over an assessment window) or L5 (external attestation), both of which require infrastructure not yet built.

---

## What is not yet available

Canonical analysis, signed report-bundle generation, protected-source replay, and offline complete-bundle verification are available. The [v2.1.7 clean offline acceptance record](../release_notes/v2.1.x/v2.1.7-offline-acceptance.md) identifies the candidate, bundle, external trust, isolated environment, successful verification report, and four rejected mutation classes. The following capabilities remain outstanding:

- **Eval-native signed release bundle** — Eval runs replay when represented in a signed compliance report bundle. A separate eval-native canonical analysis, statistical release gate, signing format, and complete eval verifier remain deferred.
- **Historical effectiveness evidence** — Recurring evidence collection, assertion and control history stores, release-gate profiles, failed-run denominator preservation, and version bridge assessments are not implemented. Canonical analysis calculates point-in-time evidence-window completeness but does not establish operating effectiveness over a recurring period.
- **Additional framework crosswalks** — Only FedRAMP 20x and NIST SP 800-53 have canonical framework definitions and a reviewed crosswalk. SOC 2, ISO 27001, HIPAA, PCI DSS, GDPR, NIST SP 800-63B, and NSA ZIG do not receive generated framework-control assessments.
- **Manual notary real-topology verification** — The `fedramp-escalate` and `dhs-release` passkey flows are implemented and unit-tested but have not been run against their real notary topologies.

---

## Offline trust and replay contract

Report-signing trust and source-evidence trust are independent inputs. `--trust-policy` authenticates the signed report manifest and checksum root. `--evidence-trust` supplies protocol-owned assessed Ed25519 signer records for commitment and customer or assessor attestation replay. Both policies remain outside the bundle; the verifier rejects an in-bundle policy, a reused path, non-canonical input, invalid key material or digest, duplicate keys, revoked or expired assessment, and scope-ineligible trust.

The verifier reads all source bodies through the bundle root, enforces the signed directory inventory and artifact limits, and does not discover evidence or trust from the working directory or runtime state. It reproduces demo and eval source-verification reports, reimports every standalone source, compares every reproduced protocol evidence reference with protected analysis, reproduces every renderer, and fails closed for missing, orphaned, duplicated, transplanted, substituted, malformed, unsupported, or unassessed evidence. One report has one assessment scope; generation includes only source records bound to that scope.

## Roadmap

Phases 0 through 5 are complete for in-tree implementation, verification, and the recorded v2.1.7 clean offline acceptance lane. The remaining roadmap begins with eval-native release evidence, then recurring operating-effectiveness evidence and additional reviewed framework catalogs and crosswalks.

**Phase 6 — Recurring evidence, release gates, and historical effectiveness.** This phase adds scheduled evidence collection, assertion and control history, release-gate profiles, failed-run denominator preservation, and bridge assessments for version changes.

**Phase 7 — Expanded framework and domain coverage.** This phase extends reviewed catalogs and crosswalks beyond the current FedRAMP 20x and NIST SP 800-53 foundation and adds framework drift tooling and expanded customer and assessor attestation support.

---

## CLI commands

The compliance CLI exposes evaluation, evidence-graph verification, demo verification, signed report-bundle generation, and offline report-bundle verification:

- `g8e compliance ksi --class C --scope-id <scope> --run-id <run> --window-start-unix-ms <start> --window-end-unix-ms <end>` evaluates KSIs with an explicit assessment binding and prints the result set as JSON.
- `g8e compliance ksi-history --ksi <id>` reads historical evaluation snapshots for a specific KSI.
- `g8e compliance overlay --overlay-dir <dir>` inspects and validates AI control overlay catalogs.
- `g8e compliance demo-run verify <run-id> [--project-root <dir>]` independently verifies one persisted demo run and emits a canonical `ComplianceVerificationReport`.
- `g8e compliance evidence-graph verify --demo-run <run-id> --eval-run <run-id>` imports and validates persisted runs as one typed content-addressed graph.
- `g8e compliance report generate --scope-id <scope> [--demo-run <run-id>] [--eval-run <run-id>] [standalone source flags] --window-start-unix-ms <start> --window-end-unix-ms <end> --report-id <report-id> --profile <public|restricted> --signing-metadata <path> --signing-private-key <path> [--evidence-trust <path>]` imports explicit scope-bound evidence, copies exact demo, eval, KSI, commitment, attestation, audit, ledger, and build/configuration source bytes into canonical protected paths, grades the evidence, constructs canonical analysis and framework profiles, renders every supported format, signs the complete bundle, persists it under the runtime report tree, and prints the descriptor path. Signed standalone sources require the external assessed evidence trust input during generation.
- `g8e compliance report verify <bundle-manifest.json> --trust-policy <assessed-report-trust.json> [--evidence-trust <assessed-evidence-trust.json>]` verifies protected bodies, directory inventory, signed roots, external assessed trust, independent source replay, protected analysis equality, and deterministic renderer reproduction offline and emits a canonical `ComplianceVerificationReport`. Evidence trust is required whenever commitment or customer or assessor attestation evidence is represented.

---

## Evidence repository

### Canonical catalogs

| Catalog | Location | Identity |
|---------|----------|----------|
| Assertion catalog | `protocol/constants/compliance/assertion-catalog.json` | `g8e-control-assertions` v1.0.0 |
| Framework catalog | `protocol/constants/compliance/framework-catalog.json` | FedRAMP 20x CR26-2026-06-24, NIST SP 800-53 rev5 |
| Crosswalk | `protocol/constants/compliance/fedramp-nist-crosswalk.json` | 60 supporting mappings |
| Demo scenario catalog | `protocol/constants/compliance/demo-scenario-catalog.json` | `g8e-demo-scenarios` v1.1.0 |
| KSI catalog | `docs/reference/ksi-catalog.json` | 31 KSIs across 10 categories |
| COSAiS overlays | `docs/reference/cosais-overlays.json` | 5 overlay entries (draft) |

### Code evidence

| Component | Location | Purpose |
|-----------|----------|---------|
| Compliance catalogs | `protocol/constants/compliance/`, `internal/services/compliance/catalog/` | Digest-verified assertion, framework, crosswalk, and demo-scenario definitions with fail-closed validation |
| Evidence import and verification | `internal/services/compliance/evidence/` | Read-only typed importers, content-addressed evidence graph validation, cryptographic verification, assertion and framework grading, canonical analysis, and deterministic framework profiles |
| Compliance report service | `internal/services/compliance/report/` | Canonical analysis orchestration and shared JSON, OSCAL, Markdown, HTML, and CLI rendering |
| Compliance CLI | `internal/cli/cmd/compliance.go` | KSI evaluation/history, overlay validation, demo verification, evidence-graph verification, release evidence, and canonical report generation |
| KSI evaluator | `internal/services/compliance/ksi_evaluator.go` | Scope-bound KSI outcomes from typed cryptographic, grader, state, commitment, ledger, and historical evidence methods |
| KSI history store | `internal/services/compliance/ksi_history.go` | KSI snapshot persistence and historical metrics retention |
| COSAiS overlay loader | `internal/services/compliance/overlay_loader.go` | AI control overlay ingestion and KSI reference validation |
| OSCAL renderer | `internal/services/compliance/oscal.go` | Deterministic OSCAL component-definition and assessment-results generation from canonical analysis with resolvable content-addressed evidence resources |
| Receipt evidence resolution | `internal/tools/agent_harness/scenarios/receipt_evidence.go` | Canonical receipt query, final-persistence waiting, signature verification, and content-addressed evidence propagation |
| Protocol-chain grader | `internal/tools/agent_harness/scenarios/protocol_chain_grader.go` | Deterministic-stage normalization and protocol-chain grading |
| Demo manifest builder | `internal/cli/cmd/demo_manifest.go` | Typed `DemoManifest` construction, provenance hashing, and canonical persistence |
| Demo evidence persistence | `internal/cli/cmd/demo_evidence_persist.go` | Canonical receipt and persistence body persistence as resolvable runtime evidence artifacts |

### Test evidence

| Test suite | Location | Coverage |
|------------|----------|----------|
| Compliance catalog tests | `internal/services/compliance/catalog/*_test.go` | Canonical catalog digests, reference contracts, crosswalk consistency, and validation |
| Compliance evidence tests | `internal/services/compliance/evidence/*_test.go` | Demo evidence verification, tamper detection, and fail-closed mutation cases |
| Demo-run verifier CLI tests | `internal/cli/cmd/compliance_demo_run_verify_test.go` | Verifier acceptance, argument validation, fail-closed reporting, and factory-error regression |
| Demo manifest tests | `internal/cli/cmd/demo_manifest_evidence_test.go` | Manifest construction, provenance hashing, framework control union, and canonical persistence |
| Demo receipt evidence tests | `internal/cli/cmd/demo_receipt_evidence_persist_test.go` | Canonical receipt and persistence body persistence and content-address resolvability |
| Receipt evidence signature tests | `internal/tools/agent_harness/scenarios/receipt_evidence_signature_verification_test.go` | Independent receipt and persistence signature verification against assessed signer keys |
| Protocol-chain grader tests | `internal/tools/agent_harness/scenarios/protocol_chain_grader_test.go` | Deterministic-stage normalization, verified and rejected chain validation, and required-stage grading |
| Python compliance suite | `protocol/python/tests/test_compliance.py` | Canonicalization vector match, non-canonical JSON rejection, Phase 1 and Phase 2 records, and packaged catalog byte parity |
| Phase 0 regression and inventory | `internal/services/compliance/*_phase0_test.go`, `internal/services/compliance/compliance_claim_inventory_test.go`, `internal/services/compliance/ksi_method_classification_test.go` | Baseline gap documentation, claim inventory, and KSI method classification |

---

## Relationship to Compliance Alignment

[Compliance Alignment](./compliance-alignment.md) identifies the canonical catalog and crosswalk sources, defines status and responsibility semantics, links generated evidence, and explains how to resolve an assessment. It does not maintain a parallel prose control matrix or current status table. This document describes the evidence production, verification, grading, and reporting architecture behind those generated outcomes.

---

## Contact and support

For compliance evidence questions, audit support, or independent verification assistance, contact compliance@lateraluslabs.com. Security vulnerability reporting follows the coordinated disclosure policy in `.github/SECURITY.md`.

---

## Document control

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 2.1.3 | 2026-09-02 | Lateralus Labs | Initial proof-backed compliance evidence document; documented the protocol-owned assertion, framework, crosswalk, and demo-scenario catalogs, 14 evidence-grade demo scenarios, independent demo-run verification, demonstrated evidence at the v2.1.3 release boundary, and the Phase 3-7 roadmap |
| 2.1.6 | 2026-09-07 | Lateralus Labs | Reconciled completed evidence-graph, KSI, canonical analysis, OSCAL, shared renderer, control-section, and framework-profile capabilities; retained complete signed bundles, recurring effectiveness, and expanded frameworks as outstanding work |
| 2.1.7 | 2026-09-08 | Lateralus Labs | Completed protected demo, eval, KSI, commitment, attestation, audit, ledger, and build/configuration source replay; documented distinct external report and evidence trust, fail-closed offline verification, and the remaining clean-environment, eval-native, recurring-effectiveness, and framework limits |

---

*This document is maintained by Lateralus Labs and reflects the proof-backed compliance evidence pipeline of g8e as of the version date. It does not claim certification, accreditation, authorization, or legal compliance. For the framework control mappings, refer to the [Compliance Alignment Report](./compliance-alignment.md).*
