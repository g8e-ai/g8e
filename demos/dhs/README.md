# DHS Persistent Sovereign Capability Demo

This demo shows how **g8e** serves as the governed, sovereign data plane underneath a coalition **common operating picture (COP)** for protecting vulnerable populations and interdicting trafficking networks across the Western Hemisphere maritime approaches.

It maps directly to the ONIX Marketplace Challenge: *Persistent Sovereign Capability for Protection of Vulnerable Populations and Interdiction of Trafficking Networks*.

## The teaming model

g8e does **not** collect sensor data, build the fusion engine, or render the COP. Those are a partner prime's products. g8e provides what those products cannot on their own:

- **Sovereign control**: every byte stays under U.S. Government authority, enforced at the network/caveat boundary.
- **U.S.-person privacy & civil-liberties protection**: PII is tokenized on ingest and rehydrated on-host only under authority.
- **Provable auditability**: every ingest, release, inference, and destruction lands in a hash-chained Git ledger and SQLite audit vault.
- **Resilience**: local governance, actuation, and evidence persistence continue while the simulated coalition datalink is detached; this scenario does not exercise cloud-model inference.

## What is real vs. display

This demo uses **real g8e enforcement**, no mock/fake doctrine, no fake MCP calls. The governance path is genuine:

| Component | Type | Description |
|---|---|---|
| **gateway** | REAL | g8e binary in `consensus` posture |
| **operator** | REAL | g8e binary with `--execution-vault`, executes governed `run_shell_command` calls |
| **agent-coalition** | REAL | g8e binary running `demos scenarios run` that submits genuine `GovernanceEnvelope`s |
| **datasvc** | REAL | Python HTTP server (the L5 actuator), records governed data operations to `operations.jsonl` |
| **dataop.sh** | REAL | Wrapper script mounted at `/usr/local/bin/dataop`, bridges operator execution to datasvc |
| **verify_ops.py** | REAL | Python script that queries datasvc to prove L5 actuation occurred |
| connector-dhs/mil/ic/partner | DISPLAY | Alpine echo loops, narrative only, no g8e identity |
| fusion-cop | DISPLAY | Alpine echo loop, represents the partner prime's COP product |
| coalition-datalink | DISPLAY | Alpine echo loop, severed in Scenario 2 to simulate comms denial |
| analyst-partner | DISPLAY | Alpine echo loop, represents a partner-nation analyst |
| bad-actor | DISPLAY | Alpine echo loop, represents an adversary on the untrusted network |
| observability | DISPLAY | Alpine log tailer, read-only audit trail viewer |

The data is **synthetic / mock-generated**; the point is to demonstrate how g8e *handles* data (use / transfer / store / retrieve / destroy), not to generate it.

## Coverage against the four Lines of Effort

| LOE | Title | Demonstrated by |
|---|---|---|
| LOE 1 | Persistent Data Collection & Coverage | Scenario 1 (multi-source ingest, NIPR/SIPR/Mission-Partner/partner-nation) |
| LOE 2 | Sovereign Data Handling, Resilience & Continuity | Scenario 2 (disconnected ops), Scenario 4 (governed destruction) |
| LOE 3 | Activity Characterization & Decision Support | Scenario 3 (governed cueing with data lineage) |
| LOE 4 | AI-Enhanced Decision Support & Predictive Analytics | Scenario 3 (privacy-gated predictive model, authority-bound cues) |

## Network topology

Each network models a real classification / sovereignty domain:

- **net_untrusted (10.60.0.0/24)**: partner-nation feed + analyst + bad actor (outside U.S. authority)
- **net_perimeter (10.61.0.0/24)**: Mission Partner shared COP environment + gateway public surface
- **net_internal (10.62.0.0/24)**: high-side USG source connectors (DHS/CBP, Military ISR, IC SIGINT), the predictive-analytics agent, and the operator's outbound mTLS tunnel
- **net_secure (10.63.0.0/24)**: sovereign data vault (US-only) + operator actuator boundary
- **net_mgmt (10.64.0.0/24)**: out-of-band observability / audit tail

The operator holds the execution vault on net_secure. Source connectors hold enrolled mTLS identities and submit `GovernanceEnvelope`s; the operator executes the governed data operation.

## Gateway posture: consensus (Phase 2)

The gateway currently runs in **consensus** posture; L2 BFT consensus is enforced as a fail-closed gate. Under consensus:
- **L1 doctrine** is enforced at admission (compiled-in threat detectors block dangerous commands).
- **L2 consensus** is enforced: ensemble Ed25519 votes must meet quorum for the transaction to be admitted.
- **L3 notary** proofs are attached to envelopes and audited in the receipt, but do not gate admission.
- The operator executes admitted commands via `run_shell_command`, driving the `datasvc` actuator.
- Signed receipts are written to the hash-chained ledger for every admitted operation.

### Tribunal bootstrap

The gateway boots with `--posture consensus --consensus-id dhs-consensus --consensus-bootstrap /etc/g8e/consensus-bootstrap.json`. The bootstrap file defines a consensus policy with a per-member Ed25519 seed (`member_seeds`), deriving an independent key pair for each member. The gateway registers each member's public key as a TrustedSigner; the agent-coalition container reads the same bootstrap file to reconstruct the per-member private keys and sign L2 votes that verify against the gateway's trusted signer registry.

### Phased rollout

| Phase | Posture | What it demonstrates | Status |
|---|---|---|---|
| **Phase 1** | doctrine | L1 doctrine enforcement, L5 actuator execution, signed receipts, disconnected ops | Complete |
| **Phase 2** | consensus | L2 BFT consensus as a fail-closed gate (quorum required) | **Active** |

All scenarios run under consensus posture; the gateway is not restarted mid-demo.

## Port mappings

| Service | Port |
|---|---|
| Gateway HTTP | 8087 |
| Gateway HTTPS | 8450 |
| Console | https://localhost:8450/console/ |

## Doctrine rules

L1 doctrine is **compiled-in** threat detectors enforced by the Gateway at admission time, not JSON files loaded at runtime. The `doctrine/` directory is bind-mounted for reference/display, but the actual enforcement happens in Go code inside the gateway binary. The key L1 detector exercised in this demo is the **data-destruction threat detector**, which blocks `rm -rf` against audit/log paths at admission before the command ever reaches the operator.

The scenarios that exercise real L1 enforcement:
- **dhs-evidence-block**: `rm -rf /var/log/g8e` is rejected at L1 admission

## Quick start

```bash
# from the repository root
make build && cp g8e demos/bin/g8e

g8e demos start dhs
g8e demos run dhs        # runs all four scenarios
g8e demos run dhs 2      # run a single scenario
g8e audit receipts       # inspect the audit trail / ledger
g8e demos clean dhs
```

`g8e demos run` runs every scenario end-to-end with no human interaction. The gateway boots in consensus posture and stays there for the whole run; no posture switching, no host-CLI enrollment, no passkey ceremony. Notary scenarios (`dhs-release`) remain in the harness registry for manual testing via `g8e demos scenarios run dhs-release` against a manually-started demo with a manually-enrolled passkey, but the automated `demos run` orchestration excludes them.

### Owner-approved platform bootstrap

After `g8e demos start dhs`, the gateway is healthy but the operator and its dependent services (`agent-coalition`, `connector-dhs`, `connector-mil`, `connector-ic`) remain not-ready until the operator's platform enrollment request is approved. `g8e demos start` prints the bootstrap instructions automatically, including the demo gateway port and the exact `g8e auth approve-platform-enrollment <request-id>` command to run. The bootstrap flow is:

```bash
# 1. Enroll the first owner (the demo gateway port is printed by `g8e demos start dhs`).
./g8e auth enroll user -e localhost:8087 --port 8450

# 2. List pending platform enrollment requests.
./g8e auth pending-platform-enrollments -e localhost:8087 --port 8450

# 3. Approve the operator's request by exact request ID.
./g8e auth approve-platform-enrollment <operator-request-id> --yes -e localhost:8087 --port 8450

# 4. Wait for the operator and its dependents to become healthy.
g8e demos status dhs
```

`g8e demos run dhs` warns if the operator is not yet enrolled and prints the bootstrap instructions before attempting to run scenarios.

## Scenarios

All scenarios run via `demos scenarios run`, a real g8e binary that submits genuine `GovernanceEnvelope`s over mTLS to the gateway. Scenarios use `MCPToolsCall` (Path A): the harness calls the MCP `tools/call` endpoint, the gateway builds the `GovernanceEnvelope` internally, runs L2 consensus deliberation via `LocalDeliberator`, and admits or rejects the envelope at L1/L2. The operator executes admitted commands via `run_shell_command`, driving the `datasvc` actuator through the `dataop` wrapper.

Each scenario's stable identity, expected outcome, rejection layer, assertion references, framework-control references, required evidence, and terminal-state assertions are defined in the canonical demo scenario catalog at `protocol/constants/compliance/demo-scenario-catalog.json` (definitions `dhs-ingest@1.0.0`, `dhs-disconnected-operations@1.0.0`, `dhs-cue@1.0.0`, `dhs-destruction-block@1.0.0`, `dhs-destruction-purge@1.0.0`). The prose descriptions below are narrative context; the canonical definitions are authoritative for evidence-grade result production and compliance crosswalks.

### 1: Sovereign Multi-Source Ingest (chain-of-custody) (LOE 1)
**Scenario `dhs-ingest`**: A coalition source connector submits a `GovernanceEnvelope` wrapping a `run_shell_command` that drives the Sovereign Data Service (L5 actuator). L1 doctrine admits the envelope; L2 consensus quorum is met and verified. The ingest is executed and a signed receipt is written to the hash-chained ledger. The `datasvc` records an `INGEST` operation.

### 2: Resilient Disconnected Operations / Continuity of Coverage (LOE 2)
The Mission Partner datalink is severed with `docker network disconnect`, and the scenario verifies through `docker network inspect` that the display container is no longer attached to the perimeter network. A governed `dhs-ingest` runs through the local gateway during the disconnected interval, after which real existence and content checks verify the operator's Git ledger directory and SQLite audit database. Datalink restoration and reattachment are verified separately; a restoration failure is reported without rewriting the measured continuity result.

### 3: Governed Predictive Cueing (LOE 3 & 4)
**Scenario `dhs-cue`**: An authorized interdiction cue with L2 ensemble quorum (decision=true) is admitted and executed by the L5 actuator. The `datasvc` records a `CUE` operation.

### 4: Sovereign Destruction + tamper-evident audit (LOE 2)
**Scenario `dhs-evidence-block`**: A compromised connector tries to wipe the audit trail with `rm -rf /var/log/g8e`; L1 doctrine rejects it at admission (the data-destruction threat detector fires). Even with valid L2 and L3 proofs attached, L1 is the hard gate and runs first. The scenario independently verifies that the operator's canonical audit database remains present and non-empty after rejection; a failed check fails the scenario.
**Scenario `dhs-purge`**: A governed retention purge is admitted by L1 doctrine with L2 consensus quorum met, and the L5 actuator records a `PURGE` operation with a cryptographic destruction receipt written to the ledger.

## Compliance & sovereignty mapping (evaluation rubric)

| Rubric criterion | Where it shows up |
|---|---|
| Multi-source Data Integration | Scenario 1 |
| System Interoperability (NIPR/SIPR/Mission Partner/partner-nation) | Scenario 1 |
| Privacy & Civil-Liberties Compliance | Scenarios 1, 3, 4 |
| Data Sovereignty & Control | Scenarios 2, 4 |
| System Resilience & Scalability | Scenario 2 |
| Decision Support & Predictive Analytics | Scenario 3 |
| Auditability / Legal & Policy Adherence | every scenario (signed receipts → ledger) |

## License

Business Source License 1.1 (BSL 1.1). Converts to Apache 2.0 on 2030-08-18.
