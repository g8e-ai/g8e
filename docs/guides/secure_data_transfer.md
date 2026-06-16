---
title: Secure Data Transfer & Governed Pipelines
parent: Guides
---

# Secure Data Transfer & Governed Data Pipelines

Last Updated: 2026-06-15
Version: v1.1.1

---

## Who this is for

This guide is written for high-assurance operators — defense, intelligence, federal
civilian, healthcare, and financial institutions — that hold data they cannot let leave
the perimeter (classified, CUI, PHI, PCI, ITAR) but still want to put modern AI and
external applications to work against it.

The forced choice these organizations face today is *frontier reasoning **or** data
sovereignty*. g8e's answer is **both**: the model and the application reason over a
scrubbed projection of reality; the host remembers, verifies, and acts; and every byte
that moves does so through a fail-closed pipeline that writes a tamper-evident audit
record **before** the side effect occurs. See
[Position Paper §9](../core/position_paper.md) for the underlying argument.

This guide shows two things:

1. **How to move data and the g8e binary itself using OS-native tools** (`scp`,
   `ssh`, `robocopy`) — what crosses by raw copy, and how to keep that surface minimal
   and verifiable.
2. **How to turn every subsequent data touch — by an AI agent or an external
   application — into a governed, audited API call** through the same single `g8e`
   binary.

---

## The two planes

A correct g8e deployment cleanly separates two planes. Conflating them is the most
common security mistake.

| Plane | What crosses | Mechanism | Governance |
|---|---|---|---|
| **Transport / bootstrap plane** | The single static `g8e` binary, protocol schemas, and (optionally) a one-time data seed | OS-native: `scp`, `ssh`, `robocopy`, removable media | None at the wire — minimized to a single, checksum-verifiable artifact |
| **Governed data plane** | Every read, edit, copy, query, and command against sensitive data thereafter | `GovernanceEnvelope` over mTLS, via CLI / MCP / A2A / direct API | Full L1–L5 verification, signed receipts, hash-chained ledger |

The transport plane is used **once**, to stand the platform up. Everything an AI or an
application does afterward rides the governed data plane. Raw `scp`/`robocopy` of
*sensitive payloads* should be treated as a break-glass exception, and even then it is
best invoked **on-host through a governed shell command** so it lands in the audit trail
(see [§4](#4-governed-bulk-movement-scp-and-robocopy-on-host)).

---

## 1. Distribute the g8e binary with OS-native tools

g8e is a single, statically compiled binary with zero standing runtime dependencies —
which is exactly what makes it safe to push into a locked-down or air-gapped enclave. It
is the only thing you copy by hand, and it is fully verifiable by checksum.

### Linux / macOS (scp, ssh)

The CLI wraps `scp` and `ssh` directly so you can use your existing key-based auth and
SSH config:

```bash
# Copy the operator binary to a remote host (thin wrapper over scp; supports -P, -i, -C…)
./g8e operator scp deploy@enclave-host:/opt/g8e/g8e -i ~/.ssh/enclave_ed25519 -C

# Or stream-and-execute without leaving a copy on disk (good for ephemeral/air-gapped runs)
./g8e operator stream --hosts deploy@enclave-host -i ~/.ssh/enclave_ed25519

# Or deploy to several hosts and start them in the background
./g8e operator deploy --hosts host1,host2,host3 --background -i ~/.ssh/enclave_ed25519
```

Plain `scp` works identically if you prefer the raw tool — the binary is just a file:

```bash
scp -C -i ~/.ssh/enclave_ed25519 ./bin/g8e deploy@enclave-host:/opt/g8e/g8e
ssh deploy@enclave-host 'chmod +x /opt/g8e/g8e'
```

### Windows (robocopy)

On a Windows host or across an SMB share into the enclave, use `robocopy` to stage the
binary and schemas. `robocopy` gives you restartable, mirror-mode, logged copies — useful
for moving the binary plus the `protocol/` schema directory in one pass:

```bat
robocopy \\staging\g8e-release C:\g8e g8e.exe /Z /COPY:DAT /LOG:C:\g8e\transfer.log
robocopy \\staging\g8e-release\protocol C:\g8e\protocol /E /Z /LOG+:C:\g8e\transfer.log
```

`/Z` enables restartable mode for large/flaky links; `/COPY:DAT` preserves data,
attributes, and timestamps; `/LOG` gives you a transfer record to attach to your change
ticket.

### Verify the artifact before you trust it

Whatever tool moved the bytes, the binary is the attack surface — so verify it. After
transfer, confirm the checksum matches the release manifest:

```bash
# Linux/macOS
sha256sum /opt/g8e/g8e

# Windows
certutil -hashfile C:\g8e\g8e.exe SHA256
```

Once g8e is running, the same check is available as a **governed, audited** tool
(`fs_file_checksum`) so binary-integrity verification itself becomes part of the record —
see [§3](#3-replace-raw-file-operations-with-governed-equivalents).

### Air-gapped enclaves

For fully disconnected environments, compile on a connected staging host
(`make build`), then carry `bin/g8e` + `protocol/` across the gap by `scp`/`robocopy`/
removable media. At runtime g8e requires **zero** external network access: local PKI,
local SQLite state, local pub/sub, local WebAuthn bootstrap. Full procedure in the
[Air-Gap guide](./air_gap.md).

---

## 2. Stand up the governed boundary

Once the binary is on the host, bring up the Gateway (Policy Decision Point) and Operator
(Policy Execution Point). For a high-security posture, start in **notary mode**, which
strictly enforces all of L1 Doctrine, L2 Consensus, and L3 human-in-the-loop:

```bash
# Strictest posture: L1 + L2 + L3 all enforced (human signature required for mutations)
./g8e gw start --posture notary

# Authenticate the human operator; issues a short-lived mTLS cert bound to a SPIFFE user ID
./g8e auth login
```

The first human to authenticate becomes the **Platform Owner**. All other entities
(operators, AI clients, applications, additional users) must enroll via CSR-based
enrollment — there is no ambient trust and no standing credential to steal.

Enroll remote operators over mTLS via CSR (no inbound ports required on the host — the
operator dials out):

```bash
# On the enclave host, enroll with the gateway and obtain operator mTLS certs
./g8e gw security pki enroll -e <gateway-ip>
```

Confirm the boundary is live:

```bash
./g8e gw status
./g8e operator list      # operators currently connected to the gateway
```

At this point: nothing mutates host reality except through the L1→L2→L3→L4→L5 pipeline,
every attempt is recorded before execution, and raw data never leaves the `.g8e`
directory on the host.

---

## 3. Replace raw file operations with governed equivalents

This is the core of the model. Instead of an AI or an application reaching for the
filesystem (or `scp`-ing data out), it expresses **intent** as a typed
`GovernanceEnvelope`. The Operator verifies the proofs against live host state, executes
on-host, scrubs the output at the sovereignty boundary, and returns a **signed receipt**.

The AI sees only a scrubbed projection — secrets and regulated fields are replaced with
opaque tokens and rehydrated to real values **only at the moment of execution, on the
host where the data already lives**. The frontier model never takes custody.

### Canonical file/data event types

Applications and agents use these typed events (full list in
[Build Apps](./build_apps.md)):

| Operation | Event type |
|---|---|
| Read a file | `g8e.v1.operator.filesystem.read.requested` |
| Edit a file | `g8e.v1.operator.file.edit.requested` |
| List a directory | `g8e.v1.operator.filesystem.list.requested` |
| Grep / search | `g8e.v1.operator.filesystem.grep.requested` |
| File history / diff / restore | `g8e.v1.operator.file.history.fetch.requested`, `…file.diff.fetch.requested`, `…file.restore.requested` |
| Run a command | `g8e.v1.operator.command.requested` |

### Let an AI agent touch sensitive data — governed end to end

Launch the agent so that **all** of its I/O is forced through g8e and native tools are
disabled. The agent receives a short-lived delegated credential carrying *both* the app
identity and the accountable human's identity:

```bash
# Gateway must be running. Launches Claude with g8e as its ONLY MCP provider;
# Bash/Read/Write/Edit/WebSearch/WebFetch are disabled so nothing bypasses governance.
./g8e mcp agent run claude
```

Every tool call the agent makes is converted to a governed envelope, verified L1–L5, and
recorded against a per-agent SPIFFE identity (`spiffe://g8e.local/app/claude`) **and** the
human who launched it. A file the agent "reads" arrives scrubbed; a file it "edits" goes
through a two-phase, git-backed commit with `state_root_before`/`state_root_after`
captured around it, so the change is reversible and provable.

Governed integrity check (the audited replacement for the manual `sha256sum` in §1):

```bash
curl -X POST https://localhost:8443/mcp \
  --cert .g8e/pki/client.crt --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/call","id":1,
       "params":{"name":"fs_file_checksum","arguments":{"path":"/data/case-7788/intake.pdf"}}}'
```

---

## 4. Governed bulk movement: scp and robocopy on-host

When you genuinely need to move *files in bulk* (not the binary) — e.g. stage an evidence
set or replicate a dataset between governed hosts — do not run `scp`/`robocopy` out of
band. Invoke them **on the host, through a governed shell command**, so the transfer
itself becomes a verified, audited mutation:

```bash
# A governed shell command. In notary posture this suspends for human L3 approval
# before it runs, and emits a signed receipt after.
curl -X POST https://localhost:8443/mcp \
  --cert .g8e/pki/client.crt --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/call","id":1,
       "params":{"name":"run_shell_command","arguments":{
         "command":"scp -C /data/case-7788/*.pdf custodian@review-host:/intake/case-7788/"
       }}}'
```

On Windows the same pattern wraps `robocopy`:

```json
{"jsonrpc":"2.0","method":"tools/call","id":1,
 "params":{"name":"run_shell_command","arguments":{
   "command":"robocopy D:\\data\\case-7788 \\\\review-host\\intake\\case-7788 /E /Z /LOG:C:\\g8e\\xfer.log"
 }}}
```

What this buys you over raw `scp`/`robocopy`:

- **L1 Doctrine** screens the command against MITRE ATT&CK patterns and a denylist before
  it ever executes — a `rm -rf`, a `sudo`, or an exfiltration pattern is rejected.
- **L3 Notary** (in notary posture) suspends the transfer until a human signs the **exact
  transaction hash** with a hardware key — approval is bound to that one transfer, not a
  session.
- The command and its result land in the hash-chained ledger with a signed
  `ActionReceipt`. The transfer is now reconstructible and non-repudiable.

> Raw `scp`/`robocopy` run outside g8e move bytes with **no** intent verification, **no**
> human bond, and **no** audit record. Reserve them for the one-time binary distribution
> in §1; route sensitive-data movement through the governed path.

---

## 5. External applications via the API

This is the pattern for *every other* application that needs to reach the data — a case
management system, an ETL job, a SIEM connector, a custom analytics service. The
application never touches the host or the filesystem directly. It becomes a
**`GovernanceEnvelope` producer** and a **receipt consumer**, and rides the same
governed pipeline through the same `g8e` binary.

Three conditions make a governed pipeline possible:

1. **The application is enrolled** — it holds its own mTLS/SPIFFE identity
   (`spiffe://g8e.local/app/<app>`), obtained via CSR-based enrollment. It
   gets no ambient trust; the Gateway evaluates its envelopes with the same rigor as any
   stranger.
2. **A human session is live** — the user's CLI session (mTLS) or web session (WebAuthn)
   is active and unexpired. This is the accountable party.
3. **The operator is bound to that session** — the envelope carries both identities, so
   the action is attributable to *the app acting on behalf of the human*.

### 5.1 Enroll the application

**The mental model:** CSR-based enrollment is cryptographic identity proof. Instead of
sharing a secret (like an API key), the application generates its own key pair and asks
the Gateway to sign a certificate attesting "this public key belongs to this identity."
The Gateway acts as a Certificate Authority (CA). The act of starting the Gateway is
itself the Platform Owner's authorization — there are no standing invite codes,
pre-shared keys, or manual approval steps. The application then proves its identity on
every subsequent call by signing with its private key (via mTLS). No shared secrets, no
API keys to leak.

**The enrollment flow:**

1. **App generates key pair**: The app creates `private.key` and a Certificate Signing
   Request (CSR) that says "I want to be `spiffe://g8e.local/app/etl-service`"
2. **App submits CSR**: The app sends the CSR to the Gateway's enrollment endpoint
3. **Gateway validates and signs**: The Gateway (acting as CA) validates the CSR and
   issues a signed mTLS certificate
4. **App receives certificate**: The app gets `client.crt` (signed by the Gateway's CA)
   and uses it with `private.key` for all subsequent authentication
5. **Short-lived by design**: Certificates expire quickly (typically 1 day), so a
   compromised key has limited lifetime

```bash
# The app generates a CSR and submits it to the gateway for enrollment
# See [Connect Apps to Gateway §CSR-Based Enrollment](./connect_apps_to_gateway.md)
# for the full enrollment flow and API details.
```

The app receives `client.crt` / `client.key` and authenticates cryptographically on every
call — there is no API key to leak.

### 5.2 Bind the operator to the live human session

g8e uses a **delegated credential** model. The envelope (or, for `agent run`, the minted
certificate) carries *both* parties, and both are folded into the signed transaction
hash:

- `acting_app_id` → `spiffe://g8e.local/app/etl-service` (the delegate — drives policy at
  the TLS edge)
- `requestor_user_id` → `spiffe://g8e.local/user/<id>` (the delegator — the accountable
  human)
- `cli_session_id` / `operator_session_id` → the live session the action is bound to

Because both identities are part of the canonical hash, the receipt names *the human who
authorized it and the app that acted* — not just "a tool." The Gateway rejects the
envelope if the bound session is not live, so an enrolled app with no active human session
authorizes nothing.

### 5.3 Submit a governed envelope

The full, maximum-control path — the only customer-facing mutation API on the Gateway:

```bash
# 1) Fetch the current state root (binds the action to the world it was approved against)
curl -s https://localhost:8443/api/v1/health \
  --cert app.crt --key app.key | jq -r .state_merkle_root

# 2) POST the canonical-JSON envelope. id MUST equal the deterministic transaction_hash
#    computed over: action_type, target_resource, payload, state_merkle_root, nonce,
#    expires_at, intent_data, requestor_user_id, acting_app_id.
curl -X POST https://localhost:8443/api/v1/governance/envelopes \
  --cert app.crt --key app.key \
  -H "Content-Type: application/json" \
  -d @envelope.json
```

A minimal envelope skeleton (see [Build Apps](./build_apps.md) for the full field list and
the `GenerateMessageID` hashing rules):

```json
{
  "id": "<transaction_hash>",
  "event_type": "g8e.v1.operator.filesystem.read.requested",
  "action_type": "FS_READ",
  "target_resource": "/data/case-7788/intake.pdf",
  "payload": "<base64-protobuf>",
  "intent_data": { "path": "/data/case-7788/intake.pdf", "justification": "case review" },
  "state_merkle_root": "<state_root>",
  "nonce": "<unique-nonce>",
  "expires_at": "<rfc3339nano>",
  "cli_session_id": "<live-cli-session>",
  "operator_session_id": "<operator-session>",
  "requestor_user_id": "spiffe://g8e.local/user/<id>",
  "acting_app_id": "spiffe://g8e.local/app/etl-service",
  "transaction_hash": "<transaction_hash>",
  "protocol_version": "1.0",
  "governance": { "l1": { "validated": true, "violations": [] }, "l2": {}, "l3": {}, "gateway_signed": false }
}
```

Prefer not to build envelopes by hand? Submit through the Gateway's **MCP** or **A2A**
translation layer instead and let it construct the envelope and run L1 Doctrine for you:

```bash
# MCP: governed tool call
curl -X POST https://localhost:8443/mcp \
  --cert app.crt --key app.key -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/call","id":1,
       "params":{"name":"read_file","arguments":{"path":"/data/case-7788/intake.pdf"}}}'

# A2A: governed skill call
curl -X POST https://localhost:8443/api/v1/a2a/call \
  --cert app.crt --key app.key -H "Content-Type: application/json" \
  -d '{"skill_name":"file.read","payload_json":"{\"path\":\"/data/case-7788/intake.pdf\"}","execution_id":"task-1"}'
```

### 5.4 Out-of-band human approval

When the app submits a high-risk mutation without an L3 proof, the Gateway **suspends**
the transaction and returns an approval URL:

```
https://<gateway>:8443/approve/{tx_hash}
```

The accountable human opens it, authenticates with their passkey, and signs the exact
transaction. Approval is bound to that one action's hash and authorizes nothing else. The
CLI equivalent:

```bash
./g8e auth approve <transaction_hash>
```

The Gateway attaches the L3 proof, resumes verification, and the transaction proceeds —
producing a signed receipt like any other.

---

## 6. The audit trail

Every admitted action — and every *rejected* one — is written to a host-local, git-backed,
hash-chained ledger **before** the side effect, and sealed with an Ed25519-signed
`ActionReceipt`. This is your compliance evidence, and it never leaves the host except as
an explicit export you control.

```bash
# Per-session receipts (auto-discovers session if omitted)
./g8e audit receipts

# Per-agent / per-app activity (note the SPIFFE identity)
./g8e gateway data audit list   --operator-session-id spiffe://g8e.local/app/etl-service
./g8e gateway data audit summary --operator-session-id spiffe://g8e.local/app/etl-service

# Export the full signed bundle for archival
./g8e audit export --out ./receipts-export.json

# Generate a compliance report (JSON + Markdown)
./g8e audit report --out ./reports
```

Each receipt carries the typed intent, the proofs that authorized it, the `state_root`
before and after, the (scrubbed) result, and the chain link to its predecessor — so an
auditor can answer *who authorized this, on what basis, against what state, and prove it
later*, for every byte that moved.

---

## 7. End-to-end: an external app reads regulated data

Putting it together — an ETL service reads a CUI document for a human analyst, with full
governance:

1. **Bootstrap (once):** `scp` the verified `g8e` binary into the enclave; start the
   gateway in notary posture; the analyst runs `auth login` (becomes/authenticates as an
   accountable user).
2. **Enroll the app (once):** the service enrolls via CSR-based enrollment and holds an
   mTLS app identity (`spiffe://g8e.local/app/etl-service`).
3. **Live session:** the analyst's CLI session is active.
4. **Request:** the ETL service fetches the state root, builds a `FS_READ` envelope for
   `/data/cui/report.docx` carrying `acting_app_id` (the service) + `requestor_user_id`
   (the analyst) + the live session, and POSTs it.
5. **Govern:** L1 screens the path/intent; L4 checks nonce, expiry, and that the state
   root still matches; in notary posture the Gateway suspends and returns an approve URL.
6. **Approve:** the analyst signs the transaction hash with their passkey.
7. **Execute & scrub:** the Operator reads the file on-host, scrubs CUI markers at the
   sovereignty boundary, returns a tokenized projection, and writes a signed receipt.
8. **Evidence:** `g8e audit report` produces the record showing exactly who authorized
   the read, against which state, and what the app received.

An attempt to instead *exfiltrate* that document (e.g. a poisoned prompt telling the agent
to mail it out) is rejected at L1 and recorded as a blocked attempt.

### Run it live

The `secure-data` demo environment stands up this exact two-plane model — a sensitive
transfer set staged on a secure-tier host, a governed destination, and an isolated bad
actor — and exercises both the governed and the ungoverned path:

```bash
./g8e demos start secure-data
./g8e demos run   secure-data 1   # Governed Data Transfer with Signed Receipt
./g8e demos run   secure-data 2   # Out-of-Band Transfer Blocked
```

Scenario 1 walks the §4 governed-bulk-movement path (on-host `scp` through
`run_shell_command`, screened by L1, sealed with a signed receipt); scenario 2 is the
contrast — a raw out-of-band copy that is stopped by network isolation and rejected by the
`out_of_band_exfiltration` doctrine. The companion `gov` demo shows the same blocking for
CUI specifically:

```bash
./g8e demos run gov 1     # CUI Exfiltration Attempt Blocked
```

---

## 8. What you can tell your auditor

The mechanisms above map directly onto controls in the
[Compliance Alignment Report](../reference/compliance-alignment.md):

- **Data sovereignty / no custody transfer** — Sovereign Execution Boundary scrubs before
  egress; rehydration only at execution (NIST SC-7, GDPR data minimization, HIPAA
  transmission security).
- **No standing privileges** — JIT mTLS credentials via CSR-based enrollment (NIST
  AC-6/SI-14, NSA ZIG PAM).
- **Non-repudiable audit** — Ed25519-signed receipts on a git-backed ledger, written
  before the side effect (NIST AU-10, fail-closed AU-5).
- **Human accountability** — L3 WebAuthn/CLI signatures bound to the transaction hash, with
  both delegator and delegate in the signed record (NSA ZIG MFA, PCI 8.4).
- **Air-gap capable** — single binary, zero runtime external dependencies (NIST SC-22,
  ISO A.15.2).

---

## Next steps

- **[Build Apps](./build_apps.md)** — full envelope construction and the canonical hash algorithm
- **[Connect Apps to Gateway](./connect_apps_to_gateway.md)** — enrollment, sessions, MCP/A2A/API surfaces
- **[Air-Gap](./air_gap.md)** — disconnected deployment procedure
- **[Compliance Alignment](../reference/compliance-alignment.md)** — control-by-control mapping
- **[Position Paper](../core/position_paper.md)** — the architectural argument for commitments-not-custody
