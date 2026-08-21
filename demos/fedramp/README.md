# FedRAMP Sovereign Cloud Governance Demo

This demo shows how **g8e** provides governed, auditable cloud resource operations for a FedRAMP-authorized cloud service provider. Every provision, configure, destroy, and revert operation passes through the full 5-layer governance gauntlet before touching cloud infrastructure.

It maps to FedRAMP CR26 Key Security Indicators (KSI) for access control (AC), audit and accountability (AU), configuration management (CM), system and communications protection (SC), system and information integrity (SI), and critical requirements (CR). Doctrine detectors carry typed KSI IDs from the CR26 per-KSI catalog (`docs/reference/ksi-catalog.json`), providing traceability from governance enforcement to compliance evidence. The demo emits a KSI result artifact via `g8e compliance ksi --class C` and persists snapshots via `g8e compliance ksi-history`.

## The teaming model

g8e does **not** manage cloud resources directly. It provides the governance layer that a cloud service operator's existing automation calls through:

- **Sovereign control**: every cloud operation is admitted only after L1 doctrine and L2 consensus.
- **Provable auditability**: every provision, destroy, revert, and denied attempt lands in a hash-chained Git ledger and SQLite audit vault.
- **Tamper-evident evidence**: audit trail wipe attempts are rejected at L1 admission before reaching the actuator.

## What is real vs. display

This demo uses **real g8e enforcement**, no mock/fake doctrine, no fake MCP calls. The governance path is genuine:

| Component | Type | Description |
|---|---|---|
| **gateway** | REAL | g8e binary in `consensus` posture |
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
| AC | KSI-IAM-05, KSI-IAM-07 | Scenario 1 (governed provisioning), Scenario 2 (unauthorized destruction blocked) |
| AU | KSI-MLA-07 | Scenario 2 (audit trail destruction blocked), Scenario 4 (audit vault wipe blocked), every scenario (signed receipts to hash-chained ledger) |
| CM | KSI-CMT-01, KSI-SVC-04 | Scenario 1 (governed provisioning), Scenario 3 (governed configuration revert) |
| SC | KSI-SVC-03, KSI-CNA-01 | Scenario 2 (cross-domain destruction blocked), mTLS identity required for all submissions |
| SI | KSI-IAM-05 | integrity monitoring via hash-chained ledger |
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
- **L3 notary** proofs are attached to envelopes and audited in the receipt, but do not gate admission.

### Tribunal bootstrap

The gateway boots with `--posture consensus --consensus-id fedramp-consensus --consensus-bootstrap /etc/g8e/consensus-bootstrap.json`. The bootstrap file defines a 3-member tribunal with distinct per-member Ed25519 seeds (`member_seeds`), each deriving an independent key pair. The gateway registers each member's public key as a TrustedSigner; the agent-runtime container reads the same bootstrap file to reconstruct the per-member private keys and sign L2 votes independently.

This makes the 2-of-3 quorum a real BFT quorum: each member signs with its own key, and a single compromised key cannot forge enough votes to meet quorum. The `RequireDistinct` flag ensures duplicate signer key IDs are rejected.

### Phased rollout

| Phase | Posture | What it demonstrates | Status |
|---|---|---|---|
| **Phase 1** | doctrine | L1 doctrine enforcement, L5 actuator execution, signed receipts | Complete |
| **Phase 2** | consensus | L2 BFT consensus as a fail-closed gate (quorum required) | **Active** |

All scenarios run under consensus posture; the gateway is not restarted mid-demo.

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

## Quick start

```bash
# from the repository root
make build

g8e demos start fedramp
g8e demos run fedramp        # runs all four scenarios
g8e demos run fedramp 3      # run a single scenario
g8e audit receipts           # inspect the audit trail / ledger
g8e demos clean fedramp
```

`g8e demos run` runs every scenario end-to-end with no human interaction. The gateway boots in consensus posture and stays there for the whole run; no posture switching, no host-CLI enrollment, no passkey ceremony. Notary scenarios (`fedramp-escalate`) remain in the harness registry for manual testing via `g8e demos scenarios run fedramp-escalate` against a manually-started demo with a manually-enrolled passkey, but the automated `demos run` orchestration excludes them.

### Owner-approved platform activation

After `g8e demos start fedramp`, the gateway is healthy but the operator and its dependent service (`agent-runtime`) remain not-ready until the operator's platform enrollment request is approved. `g8e demos start` prints the activation instructions automatically, including the demo gateway port and the exact `g8e auth approve-platform-enrollment <request-id>` command to run. The activation flow is:

```bash
# 1. Enroll the first owner (the demo gateway port is printed by `g8e demos start fedramp`).
./g8e auth enroll user -e https://localhost:<demo-https-port>

# 2. List pending platform enrollment requests.
./g8e auth pending-platform-enrollments

# 3. Approve the operator's request by exact request ID.
./g8e auth approve-platform-enrollment <operator-request-id> --yes

# 4. Wait for the operator and its dependents to become healthy.
g8e demos status fedramp
```

`g8e demos run fedramp` warns if the operator is not yet enrolled and prints the activation instructions before attempting to run scenarios.

## FIPS 140-3 mode

The standard `./g8e demos start fedramp` invocation already builds with FIPS 140-3 approved mode enabled. The repo-root `Dockerfile` sets `GOFIPS140=v1.0.0` in the builder stage, linking the Go Cryptographic Module v1.0.0 (CMVP Cert #5247) into the binary. The runtime image is pinned to Debian GNU/Linux 12 (vendor-affirmed OE per CMVP Cert #5247 Table 3). There is no separate FIPS compose variant or FIPS Dockerfile — every demo and production deployment gets a FIPS-capable binary by default.

```bash
# start the demo (FIPS approved mode is active by default)
g8e demos start fedramp

# verify FIPS mode is active in the gateway container
docker exec g8e-fedramp-gateway /g8e version --fips

# run scenarios
g8e demos run fedramp
```

Approved mode is active but enforcement is OFF by default. This is the common production posture: non-approved primitives such as Ed25519 (consensus signing, actuator receipts, PKI) and ChaCha20-Poly1305 (SSH streaming) still work. Operators who need strict enforcement — rejecting non-approved primitives at runtime — set `GODEBUG=fips140=only` in the container environment. Add it to the `environment:` block of the relevant service in `compose.yml`:

```yaml
environment:
  - GODEBUG=fips140=only
  - G8E_GATEWAY_POSTURE=${G8E_GATEWAY_POSTURE:-consensus}
```

Then rebuild and restart:

```bash
GODEBUG=fips140=only docker exec g8e-fedramp-gateway /g8e version --fips
# reports "FIPS 140-3 mode: enabled", "FIPS enforcement: enabled", exits 0
```

All scenarios run identically under approved mode; the governance pipeline, PKI, and TLS stack use FIPS-validated algorithms for their validated operations.

See [FIPS 140-3 Compliance](../../docs/reference/fips140-3.md) for the validated boundary, OE matrix, and build/runtime activation details.

## Scenarios

All scenarios run via `demos scenarios run`, a real g8e binary that submits genuine `GovernanceEnvelope`s over mTLS to the gateway. Scenarios use `MCPToolsCall` (Path A): the harness calls the MCP `tools/call` endpoint, the gateway builds the `GovernanceEnvelope` internally, runs L2 consensus deliberation via `LocalDeliberator`, and admits or rejects the envelope at L1/L2. The operator executes admitted commands via `run_shell_command`, driving the `cloudsvc` actuator through the `cloudop` wrapper.

### 1: Governed Cloud Resource Provisioning (AC, CM, AU)
**Scenario `fedramp-provision`**: A cloud operations agent submits a `GovernanceEnvelope` wrapping a `run_shell_command` that drives the Sovereign Cloud Service (L5 actuator). L1 doctrine admits the envelope; L2 consensus quorum is met and verified. The provision is executed and a signed receipt is written to the hash-chained ledger. The `cloudsvc` records a `PROVISION` operation.

### 2: Unauthorized Audit Trail Destruction Blocked (AC, SC)
**Scenario `fedramp-deny`**: A compromised operator tries to destroy the cloud operations ledger with `rm -rf /var/cloudsvc`. L1 doctrine rejects it at admission (the `destroy_rm_rf_system_dirs` detector fires). Even with valid L2 and L3 proofs attached, L1 is the hard gate and runs first. Nothing reaches the actuator.

### 3: Governed Configuration Revert (CM, AU)
**Scenario `fedramp-revert`**: A configuration revert is submitted to roll back a resource to its prior version. L1 doctrine admits the envelope; L2 consensus quorum is met. The revert is executed and the `cloudsvc` records a `REVERT` operation with the prior state hash. The revert appears in the ledger as an evidenced, attributed action.

### 4: Gateway Audit Vault Destruction Blocked (AU)
**Scenario `fedramp-evidence-block`**: A compromised operator tries to wipe the gateway audit vault with `rm -rf /root/.g8e/data`. L1 doctrine rejects it at admission (the `destroy_rm_rf_system_dirs` detector fires). Even with valid L2 and L3 proofs, L1 runs first. The audit vault is tamper-evident.

## Evidence export

After all scenarios, `g8e audit export` produces a single evidence bundle with all receipts. The bundle is tagged to CR26 KSI categories from `ksi_categories.json`. `g8e audit receipts` shows the full hash-chained ledger. Tampering with any record in a copy causes chain verification to fail.

The demo also emits a machine-readable KSI result artifact via `g8e compliance ksi --class C`, which evaluates KSIs against the live audit state and produces a binary result set. This step runs automatically after all four scenarios complete — the demo orchestrator executes `g8e compliance ksi --class C --catalog /docs/reference/ksi-catalog.json` inside the gateway container, then verifies the snapshots via `verify_ops.py --ksi-result`. Snapshots are persisted to `/root/.g8e/data/compliance/ksi-history/` inside the gateway container. OSCAL `component-definition` and `assessment-results` artifacts are generated via `g8e compliance export --format oscal --class C`.

## License

Business Source License 1.1 (BSL 1.1). Converts to Apache 2.0 on 2030-08-18.
