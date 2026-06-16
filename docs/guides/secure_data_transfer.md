---
title: Governed Data Migration
parent: Guides
---

# Governed Data Migration

> "The transfer happened. Here's the proof it should have."

Last Updated: 2026-06-16
Version: v1.1.3

---

## Who this is for

Organizations moving large volumes of sensitive data between storage systems — a SharePoint
on-premises farm to Microsoft 365, a file server to S3, an on-prem NAS to Azure Blob —
who need a tamper-evident audit record proving that the transfer was authorized, executed
correctly, and received by the destination **before** any compliance question is ever asked.

This guide is also for the **procurement and compliance reviewers** who evaluate migration
vendors: g8e does not replace your transfer tool (rclone, the SharePoint Migration Tool,
robocopy). It **governs** it. The transfer tool runs inside the g8e L5 Actuator; every byte
it moves was authorized by a signed `GovernanceEnvelope` before it left the source.

The forced choice migration projects face today is *speed **or** audit completeness*. g8e's
answer is **both**: fast, parallelized bulk transfer via best-in-class tools, with
cryptographic chain of custody at the source **and** the destination.

---

## The governing idea

**g8e governs the action, not the actor.**

A *connector* is a minimal application that knows how to describe a migration step as a
typed `GovernanceEnvelope`. The connector does not transfer data itself — it produces the
authorization record. The actual transfer tool (rclone, the SharePoint Migration Tool)
runs inside the **L5 Actuator**, which verifies the envelope against L1 Doctrine, L2
Consensus, and L3 human approval before executing anything.

This means:

- The transfer tool cannot run without a signed, verified envelope
- The connector cannot bypass L1 Doctrine (it has no direct execution rights)
- A poisoned or misconfigured connector produces at most a rejected envelope and a
  recorded violation — it cannot move data unilaterally

The transfer tool is interchangeable. The governance record is not.

**What g8e is not:** a universal storage API. Veeam, Commvault, and their peers own that
lane. g8e is a thin connector interface — two first-party connectors covering the vast
majority of enterprise migration targets — layered over a governance pipeline that makes
every transfer legally auditable.

---

## Two-Operator topology

A production governed migration uses **two Operators**: one at the source, one at the
destination. Each signs its own `ActionReceipt`. Both receipts together form the
cryptographic chain of custody — proof that the data left the source under authorization
and arrived at the destination under verification.

```
Source Domain                                   Destination Domain
┌──────────────────────────────┐               ┌──────────────────────────────┐
│  src-gateway  (PDPoint)      │               │  dst-gateway  (PDPoint)      │
│  src-operator (L5 Actuator)  │──── bytes ───▶│  dst-operator (L5 Actuator)  │
│  connector  ──▶ Envelope ───▶│               │  ◀── ingress verification    │
│  [source storage]            │               │  [destination storage]        │
└──────────────────────────────┘               └──────────────────────────────┘
        ↓ src ActionReceipt                            ↓ dst ActionReceipt
        └──────────────────────────────────────────────┘
                         Chain of Custody
```

Neither operator trusts the other implicitly. The source receipt proves authorized egress;
the destination receipt proves authorized ingress. An auditor with both receipts can prove
the transfer happened exactly once, under authorization, to the right destination.

A single-operator deployment is valid for internal migrations where source and destination
share a security domain. Add the destination operator when the migration crosses a trust
boundary — cloud tenant, agency boundary, contractor network.

---

## Connector interface

A connector is any enrolled process that can produce a valid `GovernanceEnvelope`. It must:

1. **Hold an enrolled mTLS identity** — `spiffe://g8e.local/app/<connector-name>`,
   obtained via CSR-based enrollment with the source gateway
2. **Reference a signed migration manifest** — a JSON document listing source paths,
   destination paths, classification, and justification, signed by the migration admin
3. **Submit one envelope per transfer action** — `MIGRATION_TRANSFER` action type, with
   `source_path`, `destination_path`, `connector`, and `manifest_id` in `intent_data`
4. **Record the ActionReceipt** — append the signed receipt to the migration log; submit
   to the destination gateway for ingress verification

The connector does **not** open connections to destination storage, read source files, or
write destination files. Those are L5 Actuator actions, executed by the Operator after
the envelope passes L1–L4.

A minimal envelope for a migration step:

```json
{
  "id": "<transaction_hash>",
  "event_type": "g8e.v1.operator.migration.transfer.requested",
  "action_type": "MIGRATION_TRANSFER",
  "target_resource": "/sites/Legal/Documents/2024/contract-001.docx",
  "intent_data": {
    "source_path": "/sites/Legal/Documents/2024/contract-001.docx",
    "destination_path": "/sites/Legal-Archive/Documents/2024/contract-001.docx",
    "connector": "sharepoint-graph-api",
    "manifest_id": "SPO-MIGRATION-2026-001",
    "justification": "Legal hold migration — SharePoint On-Prem to M365"
  },
  "state_merkle_root": "<state_root>",
  "nonce": "<unique-nonce>",
  "expires_at": "<rfc3339nano>",
  "requestor_user_id": "spiffe://g8e.local/user/<migration-admin>",
  "acting_app_id": "spiffe://g8e.local/app/sharepoint-connector",
  "transaction_hash": "<transaction_hash>",
  "protocol_version": "1.0"
}
```

See [Build Apps](./build_apps.md) for the full field list and `GenerateMessageID` hashing
rules.

---

## First-party connectors

g8e ships two first-party connectors covering the majority of enterprise migration targets.

### rclone connector

**Coverage:** Amazon S3, Google Cloud Storage, Azure Blob, SMB/CIFS shares, SFTP, NFS,
and any other [rclone-supported backend](https://rclone.org/overview/).

The rclone connector enumerates the source tree, submits one `MIGRATION_TRANSFER` envelope
per batch, and the src-operator executes `rclone copyto` for each approved transfer.
rclone runs inside the L5 Actuator — the connector never touches source data directly.

```bash
# Configure source and destination remotes (once, on the source operator host)
./g8e migration connector rclone configure \
  --source s3:source-bucket \
  --destination azure:dest-container \
  --name s3-to-azure

# Plan: enumerate the manifest without transferring anything
./g8e migration connector rclone plan --name s3-to-azure --out migration-manifest.json

# Execute: submit envelopes; Operator runs rclone for each approved batch
./g8e migration connector rclone run --manifest migration-manifest.signed.json
```

### SharePoint connector

**Coverage:** SharePoint On-Premises → SharePoint Online, SharePoint → Azure Blob,
SharePoint → S3.

The SharePoint connector uses the Microsoft Graph API (`Sites.Read`, `Files.ReadWrite`)
for item-level operations and the SharePoint Migration API for parallelized package
ingestion on large libraries:

```bash
# Authenticate and configure the connector
./g8e migration connector sharepoint configure \
  --tenant contoso.onmicrosoft.com \
  --source https://sp-farm.corp/sites/Legal \
  --destination https://contoso.sharepoint.com/sites/Legal-Archive \
  --name spo-legal-migration

# Plan: enumerate source library, build manifest
./g8e migration connector sharepoint plan \
  --name spo-legal-migration \
  --out migration-manifest.json

# Execute: governed, receipted transfer
./g8e migration connector sharepoint run \
  --manifest migration-manifest.signed.json \
  --posture notary
```

---

## Running a governed migration

### 1. Stand up source and destination operators

On the source host:

```bash
./g8e gw start --posture notary
./g8e auth login   # migration admin becomes the accountable party
```

On the destination host:

```bash
./g8e gw start --posture notary
./g8e auth login
```

Register the destination gateway as a trusted peer so the source receipt can be submitted
for ingress verification:

```bash
# On source: register destination gateway
./g8e gw peer add --endpoint https://dst-gateway:8443 --name destination
```

### 2. Enroll the connector

The connector enrolls with the source gateway via CSR-based enrollment — same model as
any other enrolled application. It receives a short-lived mTLS cert and authenticates on
every envelope submission:

```bash
./g8e migration connector sharepoint enroll --gateway https://src-gateway:8443
# → issues spiffe://g8e.local/app/sharepoint-connector cert (1-day TTL)
```

No API key, no shared secret. A compromised cert expires within a day; the connector
cannot re-enroll without the migration admin's live session.

### 3. Sign the migration manifest

The manifest is the contract: every source path, every destination path, the data
classification, and the justification. The migration admin signs it before any transfer
begins. The `migration_manifest_required` doctrine rejects connectors that submit
envelopes referencing an unsigned manifest.

```bash
./g8e migration manifest sign \
  --manifest migration-manifest.json \
  --out migration-manifest.signed.json
```

### 4. Run the migration

```bash
./g8e migration connector sharepoint run \
  --manifest migration-manifest.signed.json \
  --posture notary
```

In notary posture the connector submits batches of envelopes. Each batch suspends for
human L3 approval — the accountable party signs the exact batch hash with their passkey:

```
Transfer batch 1/47 pending approval:
  48 files · 1.3 GB · Confidential // Legal Hold
  Approve at: https://src-gateway:8443/approve/a7f3d9c1...

./g8e auth approve a7f3d9c1
```

After approval, src-operator executes the transfers, writes a signed `ActionReceipt`,
and submits the receipt to dst-gateway for ingress verification. dst-operator confirms
arrival and writes its own receipt.

### 5. Verify chain of custody

```bash
# Source receipts
./g8e audit receipts --migration-id SPO-MIGRATION-2026-001

# Destination receipts
./g8e audit receipts \
  --gateway https://dst-gateway:8443 \
  --migration-id SPO-MIGRATION-2026-001

# Combined chain-of-custody report (JSON + Markdown)
./g8e migration report \
  --migration-id SPO-MIGRATION-2026-001 \
  --out ./migration-report/
```

The report shows every file transferred, the source receipt hash, the destination receipt
hash, the accountable human, and the manifest entry that authorized it.

---

## The audit trail

Every transfer — admitted or rejected — lands in the host-local, git-backed, hash-chained
ledger **before** the side effect, sealed with an Ed25519-signed `ActionReceipt`:

```bash
# Per-migration summary
./g8e audit report --migration-id SPO-MIGRATION-2026-001 --out ./reports

# Per-connector activity (by SPIFFE identity)
./g8e gateway data audit list \
  --operator-session-id spiffe://g8e.local/app/sharepoint-connector

# Export signed bundles for archival (both domains)
./g8e audit export --out ./audit-src.json
./g8e audit export --gateway https://dst-gateway:8443 --out ./audit-dst.json
```

Each receipt carries the typed intent, the manifest entry it was authorized against, the
L1–L4 proofs, the state root before and after, the transfer hash, and the chain link to
the previous receipt. An auditor can verify every file in the migration against the
original signed manifest.

---

## What you can tell your auditor

| Claim | Mechanism | Controls |
|---|---|---|
| Authorization before transfer | No byte moved without a signed, L1–L4 verified envelope and (in notary posture) a human WebAuthn signature bound to the exact manifest batch | NIST AC-3, CMMC 2.1.3 |
| Chain of custody | Source and destination operators each hold a signed, non-repudiable receipt; neither can be altered without breaking the hash chain | NIST AU-10, NIST SI-7 |
| Data classification preserved | Signed manifest carries the classification of every object; L1 Doctrine rejects transfers to destinations inconsistent with the classification | NIST SC-28, CMMC 3.13.16 |
| No standing privileges | Connector cert is 1-day TTL; migration admin session is live-only; no API key to revoke after the migration | NIST AC-6, NSA ZIG PAM |
| Connector cannot self-authorize | Connector produces envelopes; execution requires Operator L5 verification; a compromised connector cannot move data unilaterally | NIST SC-39 |

---

## Demo: governed SharePoint migration

The `secure-data` demo stands up the two-operator topology — source gateway + operator,
destination gateway + operator, an enrolled SharePoint connector, an rclone connector,
and an isolated bad actor — and exercises both the governed and ungoverned paths:

```bash
./g8e demos start secure-data
./g8e demos run   secure-data 1   # Governed Migration with Chain-of-Custody Receipts
./g8e demos run   secure-data 2   # Connector Bypass Attempt Blocked
./g8e demos run   secure-data 3   # Cross-Tenant Leak Doctrine Triggered
```

**Scenario 1:** The SharePoint connector submits a `MIGRATION_TRANSFER` envelope,
src-operator approves (notary posture: human signs the batch), executes the transfer,
and writes a receipt. The receipt is submitted to dst-gateway; dst-operator verifies
arrival and writes its own receipt. `./g8e migration report` produces the combined
chain-of-custody record.

**Scenario 2:** A connector attempts to `scp` data directly, bypassing the envelope
pipeline. Blocked at L1 by `connector_bypass_attempt` doctrine; recorded as a violation.

**Scenario 3:** A connector submits an envelope targeting an unregistered tenant URL.
Rejected by `cross_tenant_data_leak` doctrine; recorded and alerted.

---

## Bootstrap: distributing the g8e binary

The binary still travels by OS-native tools — this surface is intentional and used once.
Everything after it rides the governed data plane.

```bash
# Copy verified binary to source and destination hosts
scp -C -i ~/.ssh/enclave_ed25519 ./bin/g8e deploy@src-host:/opt/g8e/g8e
scp -C -i ~/.ssh/enclave_ed25519 ./bin/g8e deploy@dst-host:/opt/g8e/g8e

# Verify on both hosts before starting
sha256sum /opt/g8e/g8e   # match against release manifest
```

For fully disconnected environments, see the [Air-Gap guide](./air_gap.md).

---

## Next steps

- **[Build Apps](./build_apps.md)** — full envelope construction and the canonical hash algorithm
- **[Connect Apps to Gateway](./connect_apps_to_gateway.md)** — enrollment, sessions, MCP/A2A/API surfaces
- **[Air-Gap](./air_gap.md)** — disconnected deployment procedure
- **[Compliance Alignment](../reference/compliance-alignment.md)** — control-by-control mapping
- **[Position Paper](../core/position_paper.md)** — the architectural argument for commitments-not-custody
