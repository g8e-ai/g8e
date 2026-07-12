# DHS Persistent Sovereign Capability Demo

This demo shows how **g8e** serves as the governed, sovereign data plane underneath a coalition **common operating picture (COP)** for protecting vulnerable populations and interdicting trafficking networks across the Western Hemisphere maritime approaches.

It maps directly to the ONIX Marketplace Challenge: *Persistent Sovereign Capability for Protection of Vulnerable Populations and Interdiction of Trafficking Networks*.

## The teaming model

g8e does **not** collect sensor data, build the fusion engine, or render the COP. Those are a partner prime's products. g8e provides what those products cannot on their own:

- **Sovereign control** — every byte stays under U.S. Government authority, enforced at the network/caveat boundary.
- **U.S.-person privacy & civil-liberties protection** — PII is tokenized on ingest and rehydrated on-host only under authority.
- **Provable auditability** — every ingest, release, inference, and destruction lands in a hash-chained Git ledger and SQLite audit vault.
- **Resilience** — governance continues in contested, comms-denied corridors with no cloud dependency.

## What is real vs. display

This demo uses **real g8e enforcement** — no mock/fake doctrine, no fake MCP calls. The governance path is genuine:

| Component | Type | Description |
|---|---|---|
| **gateway** | REAL | g8e binary in `consensus` posture (notary for scenario 2) |
| **operator** | REAL | g8e binary with `--execution-vault`, executes governed `run_shell_command` calls |
| **agent-coalition** | REAL | g8e binary running `demos scenarios run` that submits genuine `GovernanceEnvelope`s |
| **datasvc** | REAL | Python HTTP server (the L5 actuator) — records governed data operations to `operations.jsonl` |
| **dataop.sh** | REAL | Wrapper script mounted at `/usr/local/bin/dataop` — bridges operator execution to datasvc |
| **verify_ops.py** | REAL | Python script that queries datasvc to prove L5 actuation occurred |
| connector-dhs/mil/ic/partner | DISPLAY | Alpine echo loops — narrative only, no g8e identity |
| fusion-cop | DISPLAY | Alpine echo loop — represents the partner prime's COP product |
| coalition-datalink | DISPLAY | Alpine echo loop — severed in Scenario 3 to simulate comms denial |
| analyst-partner | DISPLAY | Alpine echo loop — represents a partner-nation analyst |
| bad-actor | DISPLAY | Alpine echo loop — represents an adversary on the untrusted network |
| observability | DISPLAY | Alpine log tailer — read-only audit trail viewer |

The data is **synthetic / mock-generated** — the point is to demonstrate how g8e *handles* data (use / transfer / store / retrieve / destroy), not to generate it.

## Coverage against the four Lines of Effort

| LOE | Title | Demonstrated by |
|---|---|---|
| LOE 1 | Persistent Data Collection & Coverage | Scenario 1 (multi-source ingest, NIPR/SIPR/Mission-Partner/partner-nation), Scenario 2 (secure cross-domain sharing) |
| LOE 2 | Sovereign Data Handling, Resilience & Continuity | Scenario 2 (sovereign release control), Scenario 3 (disconnected ops), Scenario 5 (governed destruction) |
| LOE 3 | Activity Characterization & Decision Support | Scenario 4 (governed cueing with data lineage) |
| LOE 4 | AI-Enhanced Decision Support & Predictive Analytics | Scenario 4 (privacy-gated predictive model, authority-bound cues) |

## Network topology

Each network models a real classification / sovereignty domain:

- **net_untrusted (10.60.0.0/24)** — partner-nation feed + analyst + bad actor (outside U.S. authority)
- **net_perimeter (10.61.0.0/24)** — Mission Partner shared COP environment + gateway public surface
- **net_internal (10.62.0.0/24)** — high-side USG source connectors (DHS/CBP, DoW EO/IR, IC SIGINT), the predictive-analytics agent, and the operator's outbound mTLS tunnel
- **net_secure (10.63.0.0/24)** — sovereign data vault (US-only) + operator actuator boundary
- **net_mgmt (10.64.0.0/24)** — out-of-band observability / audit tail

The operator is the only process on net_secure with the vault. Source connectors hold enrolled mTLS identities and submit `GovernanceEnvelope`s; the operator executes the governed data operation.

## Gateway posture: consensus (Phase 2)

The gateway currently runs in **consensus** posture — L2 BFT consensus is enforced as a fail-closed gate. Under consensus:
- **L1 doctrine** is enforced at admission (compiled-in threat detectors block dangerous commands).
- **L2 consensus** is enforced: ensemble Ed25519 votes must meet quorum for the transaction to be admitted. A veto (decision=false) blocks execution.
- **L3 notary** proofs are **attached to envelopes and audited in the receipt**, but do **not** gate admission (deferred to Phase 3).
- The operator executes admitted commands via `run_shell_command`, driving the `datasvc` actuator.
- Signed receipts are written to the hash-chained ledger for every admitted operation.

### Tribunal bootstrap

The gateway boots with `--posture consensus --tribunal-id dhs-tribunal --tribunal-bootstrap /etc/g8e/tribunal-bootstrap.json`. The bootstrap file seeds a `TribunalPolicy` and trusted signer from a deterministic Ed25519 seed (`ensemble-seed.hex`), shared with the `agent-coalition` container. This enables the harness to reconstruct the same private key and sign L2 votes that verify against the gateway's trusted signer registry.

### Phased rollout

| Phase | Posture | What it demonstrates | Status |
|---|---|---|---|
| **Phase 1** | doctrine | L1 doctrine enforcement, L5 actuator execution, signed receipts, disconnected ops | Complete |
| **Phase 2** | consensus | L2 BFT consensus as a fail-closed gate (quorum required, veto blocks execution) | **Active** |
| **Phase 3** | notary | L3 notary suspend/approve flow for cross-domain release | **Active** (scenario 2) |

Scenario 2 dynamically restarts the gateway in notary posture, runs the release scenario, and restores consensus posture afterward. All other scenarios run under consensus.

## Port mappings

| Service | Port |
|---|---|
| Gateway HTTP | 8087 |
| Gateway HTTPS | 8450 |
| Console | https://localhost:8450/console/ |

## Doctrine rules

L1 doctrine is **compiled-in** threat detectors enforced by the Gateway at admission time — not JSON files loaded at runtime. The `doctrine/` directory is bind-mounted for reference/display, but the actual enforcement happens in Go code inside the gateway binary. The key L1 detector exercised in this demo is the **data-destruction threat detector**, which blocks `rm -rf` against audit/log paths at admission before the command ever reaches the operator.

The scenarios that exercise real L1 enforcement:
- **dhs-evidence-block**: `rm -rf /var/log/g8e` is rejected at L1 admission
- **dhs-cue-veto**: L2 consensus veto (decision=false) rejects an unauthorized cue
- **dhs-release**: L3 notary suspend/approve flow gates cross-domain release

## Quick start

```bash
# from the repository root
make build && cp g8e demos/bin/g8e

g8e demos start dhs
g8e demos run dhs        # run all five scenarios
g8e demos run dhs 2      # run a single scenario
g8e demos audit dhs      # inspect the audit trail / ledger
g8e demos clean dhs
```

## Scenarios

All scenarios run via `demos scenarios run` — a real g8e binary that submits genuine `GovernanceEnvelope`s over mTLS to the gateway. Under consensus posture (Phase 2), L1 doctrine is enforced at admission and L2 BFT consensus is enforced as a fail-closed gate (quorum required, veto blocks execution). L3 notary proofs are attached and audited in the receipt but do not gate admission. The operator executes admitted commands via `run_shell_command`, driving the `datasvc` actuator through the `dataop` wrapper.

### 1 — Sovereign Multi-Source Ingest (chain-of-custody) (LOE 1)
**Scenario `dhs-ingest`**: A coalition source connector submits a `GovernanceEnvelope` wrapping a `run_shell_command` that drives the Sovereign Data Service (L5 actuator). L1 doctrine admits the envelope; L2 consensus quorum is met and verified. The ingest is executed and a signed receipt is written to the hash-chained ledger. The `datasvc` records an `INGEST` operation.

### 2 — Cross-Domain Release requires Notary authority (LOE 1 & 2)
**Scenario `dhs-release`**: The gateway is restarted in **notary** posture. A cross-domain release is submitted with L2 consensus only. Under notary posture the Gateway suspends the transaction pending an out-of-band L3 principal (release authority) approval. The scenario extracts the suspended transaction hash from the gateway database, then invokes `g8e approve` — which opens a browser for WebAuthn passkey approval of the exact transaction hash. Once approved, the release executes on the L5 actuator (datasvc records a `RELEASE` operation). The gateway is then restored to consensus posture.

### 3 — Resilient Disconnected Operations / Continuity of Coverage (LOE 2)
The Mission Partner datalink is severed (docker network disconnect). Governance continues locally — a `dhs-ingest` scenario runs through the gateway with the datalink down. Every decision is committed to the Git-backed ledger and SQLite audit vault on the operator. The datalink is restored afterward. No cloud required.

### 4 — Governed Predictive Cueing (quorum vs veto) (LOE 3 & 4)
**Scenario `dhs-cue`**: An authorized interdiction cue with L2 ensemble quorum (decision=true) is admitted and executed by the L5 actuator. The `datasvc` records a `CUE` operation.
**Scenario `dhs-cue-veto`**: The same cue with L2 consensus decision=false is vetoed at quorum — the operator fails closed (≥400 response). No actuator row is produced. This demonstrates that L2 BFT consensus is a real fail-closed gate, not just an audit annotation.

### 5 — Sovereign Destruction + tamper-proof audit (LOE 2)
**Scenario `dhs-evidence-block`**: A compromised connector tries to wipe the audit trail with `rm -rf /var/log/g8e` — L1 doctrine rejects it at admission (the data-destruction threat detector fires). Even with valid L2 + L3 proofs attached, L1 is the hard gate and runs first.
**Scenario `dhs-purge`**: A governed retention purge is admitted by L1 doctrine with L2 consensus quorum met, and the L5 actuator records a `PURGE` operation with a cryptographic destruction receipt written to the ledger.

## Compliance & sovereignty mapping (evaluation rubric)

| Rubric criterion | Where it shows up |
|---|---|
| Multi-source Data Integration | Scenario 1 |
| System Interoperability (NIPR/SIPR/Mission Partner/partner-nation) | Scenarios 1 & 2 |
| Privacy & Civil-Liberties Compliance | Scenarios 1, 4, 5 |
| Data Sovereignty & Control | Scenarios 2, 3, 5 |
| System Resilience & Scalability | Scenario 3 |
| Decision Support & Predictive Analytics | Scenario 4 |
| Auditability / Legal & Policy Adherence | every scenario (signed receipts → ledger) |

## License

Apache 2.0
