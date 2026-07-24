# FedRAMP Sovereign Cloud Governance Demo

This demo shows how **g8e** provides governed, auditable cloud resource operations for a FedRAMP-authorized cloud service provider. Every provision, configure, destroy, and revert operation passes through the full 5-layer governance gauntlet before touching cloud infrastructure.

It maps to FedRAMP CR26 Key Security Indicators (KSI) for access control (AC), audit and accountability (AU), configuration management (CM), system and communications protection (SC), system and information integrity (SI), and critical requirements (CR).

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

| KSI category | Title | Demonstrated by |
|---|---|---|
| AC | Access Control | Scenario 1 (governed provisioning), Scenario 2 (unauthorized destruction blocked), Scenario 3 (destruction requires authorizing official) |
| AU | Audit and Accountability | Scenario 2 (audit trail destruction blocked), Scenario 5 (audit vault wipe blocked), every scenario (signed receipts to hash-chained ledger) |
| CM | Configuration Management | Scenario 1 (governed provisioning), Scenario 4 (governed configuration revert) |
| SC | System and Communications Protection | Scenario 2 (cross-domain destruction blocked), mTLS identity required for all submissions |
| SI | System and Information Integrity | Scenario 3 (privilege escalation detection via notary gating), integrity monitoring via hash-chained ledger |
| CR | Critical Requirements | CR-26 audit trail integrity, tamper-evident ledger across all scenarios |

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

The gateway boots with `--posture consensus --tribunal-id fedramp-tribunal --tribunal-bootstrap /etc/g8e/tribunal-bootstrap.json`. The bootstrap file seeds a `TribunalPolicy` and trusted signer from a deterministic Ed25519 seed (`ensemble-seed.hex`), shared with the `agent-runtime` container. This enables the harness to reconstruct the same private key and sign L2 votes that verify against the gateway's trusted signer registry.

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

## Scenarios

All scenarios run via `demos scenarios run`, a real g8e binary that submits genuine `GovernanceEnvelope`s over mTLS to the gateway. Under consensus posture, L1 doctrine is enforced at admission and L2 BFT consensus is enforced as a fail-closed gate. The operator executes admitted commands via `run_shell_command`, driving the `cloudsvc` actuator through the `cloudop` wrapper.

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

## License

Apache 2.0
