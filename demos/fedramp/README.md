# FedRAMP Sovereign Cloud Governance Demo

This demo shows how **g8e** provides governed, auditable cloud resource operations for a FedRAMP-authorized cloud service provider. Every provision, configure, destroy, and revert operation passes through the full 5-layer governance gauntlet before touching cloud infrastructure.

It maps to FedRAMP CR26 Key Security Indicators (KSI) for access control (AC), audit and accountability (AU), configuration management (CM), system and communications protection (SC), system and information integrity (SI), and critical requirements (CR). Doctrine detectors carry typed KSI IDs from the CR26 per-KSI catalog (`docs/reference/ksi-catalog.json`), providing traceability from governance enforcement to compliance evidence. The demo emits a KSI result artifact via `g8e compliance ksi --class C` and persists snapshots via `g8e compliance ksi-history`.

## The teaming model

g8e does **not** manage cloud resources directly. It provides the governance layer that a cloud service operator's existing automation calls through:

- **Sovereign control**: every cloud operation is admitted only after L1 doctrine, L2 consensus, and (where required) L3 notary approval.
- **Provable auditability**: every provision, destroy, revert, and denied attempt lands in a hash-chained Git ledger and SQLite audit vault.
- **Tamper-evident evidence**: audit trail wipe attempts are rejected at L1 admission before reaching the actuator.
- **Human-in-the-loop destruction**: resource destruction requires out-of-band authorizing official approval via WebAuthn passkey.

## What is real vs. display

This demo uses **real g8e enforcement**, no mock/fake doctrine, no fake MCP calls. The governance path is genuine:

| Component | Type | Description |
|---|---|---|
| **gateway** | REAL | g8e binary in `consensus` posture (notary for scenario 3) |
| **operator** | REAL | g8e binary with `--execution-vault`, executes governed `run_shell_command` calls |
| **agent-runtime** | REAL | g8e binary running `demos scenarios run` that submits genuine `GovernanceEnvelope`s |
| **cloudsvc** | REAL | Python HTTP server (the L5 actuator), records governed cloud operations to `operations.jsonl` |
| **cloudop.sh** | REAL | Wrapper script mounted at `/usr/local/bin/cloudop`, bridges operator execution to cloudsvc |
| **verify_ops.py** | REAL | Python script that queries cloudsvc to prove L5 actuation occurred |
| **bad-actor** | DISPLAY | Alpine echo loop, represents an adversary on the untrusted network |
| **observability** | DISPLAY | Alpine log tailer, read-only audit trail viewer |

The cloud resources are **synthetic**; the point is to demonstrate how g8e governs cloud operations (provision, configure, destroy, revert), not to manage real infrastructure.

## Coverage against FedRAMP CR26 KSI categories

| KSI category | KSI IDs | Demonstrated by |
|---|---|---|
| AC | KSI-IAM-05, KSI-IAM-07 | Scenario 1 (governed provisioning), Scenario 2 (unauthorized destruction blocked), Scenario 3 (destruction requires authorizing official) |
| AU | KSI-MLA-07 | Scenario 2 (audit trail destruction blocked), Scenario 5 (audit vault wipe blocked), every scenario (signed receipts to hash-chained ledger) |
| CM | KSI-CMT-01, KSI-SVC-04 | Scenario 1 (governed provisioning), Scenario 4 (governed configuration revert) |
| SC | KSI-SVC-03, KSI-CNA-01 | Scenario 2 (cross-domain destruction blocked), mTLS identity required for all submissions |
| SI | KSI-IAM-05 | Scenario 3 (privilege escalation detection via notary gating), integrity monitoring via hash-chained ledger |
| CR | KSI-MLA-07 | CR-26 audit trail integrity, tamper-evident ledger across all scenarios |

The category-level grouping in `target-data/ksi_categories.json` coexists with the typed per-KSI catalog at `docs/reference/ksi-catalog.json`. The category file provides demo grouping; the typed catalog is the source of truth for individual KSI IDs, automated methods, and evidence anchors.

## Network topology

Each network models a real isolation boundary:

- **net_untrusted (10.70.0.0/24)**: bad actor (outside cloud authority)
- **net_perimeter (10.71.0.0/24)**: gateway public surface and console endpoint
- **net_internal (10.72.0.0/24)**: agent-runtime (g8e binary submitting GovernanceEnvelopes), operator outbound mTLS tunnel
- **net_secure (10.73.0.0/24)**: operator actuator boundary, cloudsvc (L5 actuator)
- **net_mgmt (10.74.0.0/24)**: out-of-band observability and audit tail

The operator holds the execution vault on net_secure. The agent-runtime holds enrolled mTLS credentials and submits `GovernanceEnvelope`s; the operator executes the governed cloud operation through the `cloudop.sh` wrapper.

## Gateway posture: consensus

The gateway runs in **consensus** posture; L2 BFT consensus is enforced as a fail-closed gate. Under consensus:
- **L1 doctrine** is enforced at admission (compiled-in threat detectors block dangerous commands).
- **L2 consensus** is enforced: ensemble Ed25519 votes must meet quorum for the transaction to be admitted.
- **L3 notary** proofs are attached to envelopes and audited in the receipt. Scenario 3 dynamically restarts the gateway in notary posture for the resource destruction flow.

### Tribunal bootstrap

The gateway boots with `--posture consensus --consensus-id fedramp-consensus --consensus-bootstrap /etc/g8e/consensus-bootstrap.json`. The bootstrap file defines a 3-member tribunal with distinct per-member Ed25519 seeds (`member_seeds`), each deriving an independent key pair. The gateway registers each member's public key as a TrustedSigner; the agent-runtime container reads the same bootstrap file to reconstruct the per-member private keys and sign L2 votes independently.

This makes the 2-of-3 quorum a real BFT quorum: each member signs with its own key, and a single compromised key cannot forge enough votes to meet quorum. The `RequireDistinct` flag ensures duplicate signer key IDs are rejected.

### Phased rollout

| Phase | Posture | What it demonstrates | Status |
|---|---|---|---|
| **Phase 1** | doctrine | L1 doctrine enforcement, L5 actuator execution, signed receipts | Complete |
| **Phase 2** | consensus | L2 BFT consensus as a fail-closed gate (quorum required) | **Active** |
| **Phase 3** | notary | L3 notary suspend/approve flow for resource destruction | **Active** (scenario 3) |

Scenario 3 dynamically restarts the gateway in notary posture, runs the escalate scenario, and restores consensus posture afterward. All other scenarios run under consensus.

## Port mappings

| Service | Port |
|---|---|
| Gateway HTTP | 8088 |
| Gateway HTTPS | 8451 |
| Console | https://localhost:8451/console/ |

## Doctrine rules

L1 doctrine is **compiled-in** threat detectors enforced by the Gateway at admission time, not JSON files loaded at runtime. The `doctrine/` directory is bind-mounted for reference/display and KSI category mapping, but the actual enforcement happens in Go code inside the gateway binary.

The scenarios that exercise real L1 enforcement:
- **fedramp-deny**: `rm -rf /var/cloudsvc` is rejected at L1 admission (the `destroy_rm_rf_system_dirs` detector fires)
- **fedramp-evidence-block**: `rm -rf /root/.g8e/data` is rejected at L1 admission (the `destroy_rm_rf_system_dirs` detector fires)
- **fedramp-escalate**: L3 notary suspend/approve flow gates resource destruction

## Quick start

```bash
# from the repository root
make build

g8e demos start fedramp
g8e demos run fedramp        # run all five scenarios
g8e demos run fedramp 3      # run a single scenario
g8e audit receipts           # inspect the audit trail / ledger
g8e demos clean fedramp
```

## FIPS 140-3 mode

A FIPS-compliant compose variant is available at `compose.fips.yml`. It builds all g8e containers from `Dockerfile.fips` using the Go Cryptographic Module v1.0.0 (CMVP Cert #5247) with `GOFIPS140=v1.0.0` set at build time. The runtime image is `debian:12-slim` (vendor-affirmed OE).

```bash
# start the FIPS-mode demo
docker compose -f compose.fips.yml up -d

# verify FIPS mode is active in the gateway container
docker exec g8e-fedramp-fips-gateway /g8e version --fips

# run scenarios against the FIPS-mode gateway (ports 8089/8452)
g8e demos run fedramp

# teardown
docker compose -f compose.fips.yml down -v
```

The FIPS variant uses different host ports (8089/8452) and separate named volumes to avoid conflicts with the standard demo. All scenarios run identically under FIPS mode; the governance pipeline, PKI, and TLS stack use only FIPS-validated algorithms.

See [FIPS 140-3 Compliance](../../docs/reference/fips140-3.md) for the validated boundary, OE matrix, and build/runtime activation details.

## Scenarios

All scenarios run via `demos scenarios run`, a real g8e binary that submits genuine `GovernanceEnvelope`s over mTLS to the gateway. Scenarios use `MCPToolsCall` (Path A): the harness calls the MCP `tools/call` endpoint, the gateway builds the `GovernanceEnvelope` internally, runs L2 consensus deliberation via `LocalDeliberator`, and suspends transactions requiring L3 notary approval. For notary scenarios, the harness waits for human browser approval via `WaitForHumanApproval`, which subscribes to the gateway's SSE stream for `approval.completed` events and prints the approval URL for the human to complete the WebAuthn passkey ceremony in their browser. The gateway performs full real WebAuthn verification — no mock L3 bypass exists. The operator executes admitted commands via `run_shell_command`, driving the `cloudsvc` actuator through the `cloudop` wrapper.

### 1: Governed Cloud Resource Provisioning (AC, CM, AU)
**Scenario `fedramp-provision`**: A cloud operations agent submits a `GovernanceEnvelope` wrapping a `run_shell_command` that drives the Sovereign Cloud Service (L5 actuator). L1 doctrine admits the envelope; L2 consensus quorum is met and verified. The provision is executed and a signed receipt is written to the hash-chained ledger. The `cloudsvc` records a `PROVISION` operation.

### 2: Unauthorized Audit Trail Destruction Blocked (AC, SC)
**Scenario `fedramp-deny`**: A compromised operator tries to destroy the cloud operations ledger with `rm -rf /var/cloudsvc`. L1 doctrine rejects it at admission (the `destroy_rm_rf_system_dirs` detector fires). Even with valid L2 and L3 proofs attached, L1 is the hard gate and runs first. Nothing reaches the actuator.

### 3: Resource Destruction Requires Authorizing Official (SI, AC, AU)
**Scenario `fedramp-escalate`**: The gateway is restarted in **notary** posture. A resource destruction is submitted with L2 consensus only. Under notary posture the Gateway suspends the transaction pending an out-of-band L3 principal (authorizing official) approval. The scenario extracts the suspended transaction hash from the gateway's pending approvals API, then invokes `g8e approve`, which opens a browser for WebAuthn passkey approval of the exact transaction hash. Once approved, the destruction executes on the L5 actuator (cloudsvc records a `DESTROY` operation). The gateway is then restored to consensus posture.

### 4: Governed Configuration Revert (CM, AU)
**Scenario `fedramp-revert`**: A configuration revert is submitted to roll back a resource to its prior version. L1 doctrine admits the envelope; L2 consensus quorum is met. The revert is executed and the `cloudsvc` records a `REVERT` operation with the prior state hash. The revert appears in the ledger as an evidenced, attributed action.

### 5: Gateway Audit Vault Destruction Blocked (AU)
**Scenario `fedramp-evidence-block`**: A compromised operator tries to wipe the gateway audit vault with `rm -rf /root/.g8e/data`. L1 doctrine rejects it at admission (the `destroy_rm_rf_system_dirs` detector fires). Even with valid L2 and L3 proofs, L1 runs first. The audit vault is tamper-evident.

## Evidence export

After all scenarios, `g8e audit export` produces a single evidence bundle with all receipts. The bundle is tagged to CR26 KSI categories from `ksi_categories.json`. `g8e audit receipts` shows the full hash-chained ledger. Tampering with any record in a copy causes chain verification to fail.

The demo also emits a machine-readable KSI result artifact via `g8e compliance ksi --class C`, which evaluates KSIs against the live audit state and produces a binary result set. Snapshots are persisted to `.g8e/data/compliance/ksi-history/` via `g8e compliance ksi-history` for historical metrics retention. OSCAL `component-definition` and `assessment-results` artifacts are generated via `g8e compliance export --format oscal --class C`.

## License

Apache 2.0
