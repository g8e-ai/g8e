---
doc_id: ux_smoke_test
title: Host-Native Headless UX Smoke Test
audience: developers, evaluators, compliance auditors
status: current
last_updated: 2026-10-04
version: v2.3.0
owners:
  - internal/cli/cmd/demos/
  - internal/tools/agent_harness/
  - internal/services/reporting/
  - scripts/full.py
related:
  - docs/guides/getting_started.md
  - docs/ensemble/index.md
  - docs/devs/tests.md
  - docs/devs/release_process.md
when_to_read: Running localhost Gateway, Ensemble, and data-worker smoke scenarios; troubleshooting enrollment, operator binding, approvals, and local evidence reports.
do_not_use_for:
  - Platform coding invariants (docs/devs/devs.md)
  - Release acceptance (docs/devs/release_process.md)
  - Assessment authorization or certification
---

# Host-Native Headless UX Smoke Test

## Purpose

Run the Gateway daemon, Ensemble FastAPI process, and a dedicated data Operator directly on the host. Verify chat approval, governed file creation, document merge behavior, correlated receipts, read-back, and operator-local evidence. Run commands from the repository root. Reuse healthy services and approved identities when available; this procedure does not reset existing state.

## Quick index

- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Troubleshooting](#troubleshooting)
- [Evidence limits](#evidence-limits)
- [Links out](#links-out)

## Invariants

The default `doctrine` posture enforces L1, audits L2/L3, and keeps L4/L5 active. Headless owner enrollment does not exercise WebAuthn or strict consensus/notary enforcement.

The Gateway's `embedded-operator` binding is an in-process substrate. Ensemble file tools dispatch generic commands through the outbound worker subscription. Use an active remote **data** Operator for these tools. Provenance, observer, and inference workers have specialized roles and do not substitute for this worker.

Keep logical Operator ID, Operator session ID, and CLI session ID distinct. Enrollment approves a workload; `operator bind` selects the worker for the CLI and issues a replacement CLI session. Refresh saved identity values after binding.

Network audit commands query the Gateway. `report all` reads local stores, so point it explicitly at the data worker's runtime tree. `--working-dir` is the worker runtime root as well as its command-execution root; its vault and ledger live below `<working-dir>/.g8e`. Reporting against the repository's default runtime inspects Gateway state instead.

## Owned surfaces

| Behavior | Source |
| --- | --- |
| Daemon lifecycle and worker arguments | `./g8e gw --help`, `./g8e operator start --help` |
| Local Ensemble lifecycle | `./g8e ensemble --help`, `scripts/full.py`, `ensemble/app/serve.py` |
| Target selection and approval response | `internal/tools/agent_harness/client/audit.go`, `internal/tools/agent_harness/client/ensemble_approval.go` |
| Scenario assertions | `internal/tools/agent_harness/scenarios/ensemble.go` |
| CSV export and verification | `internal/services/reporting/`, `./g8e report all --help` |

## Procedures

### 1. Build and start the Gateway

Install prerequisites from [Getting Started](getting_started.md), including the Go toolchain required by `go.mod` and the Ensemble Python dependencies in the repository virtual environment. Build current source:

```bash
make build
./g8e version
./g8e gw start --cert-mode localhost --posture doctrine --http-port 8080 --https-port 8443
./g8e gw status
curl -fsS http://localhost:8080/api/v1/health
```

If the daemon already runs, inspect its status and launch configuration rather than starting another instance. Building the CLI does not replace a running daemon or worker. Restart services deliberately if their code changed.

For a new Gateway, enroll an owner:

```bash
./g8e auth enroll user --headless -e localhost
./g8e auth context
```

Headless enrollment creates an mTLS CLI identity. It does not create a browser passkey.

### 2. Start and approve the Ensemble

```bash
./g8e ensemble start
./g8e auth enroll pending
./g8e auth enroll approve <ensemble-request-id> --yes
./g8e ensemble status
curl -fsS http://127.0.0.1:8000/health
```

Approve only the request belonging to this workload; an already enrolled Ensemble needs no new approval. The lifecycle command runs local Python with `app.serve`, binds to `127.0.0.1:8000`, and uses `.local.dev/full/ensemble/.g8e` for its own runtime state. Inspect startup with `./g8e ensemble logs`.

### 3. Start, approve, and bind a data worker

In a separate terminal, leave the foreground worker running:

```bash
mkdir -p .local.dev/data-operator
./g8e operator start -e localhost --working-dir "$PWD/.local.dev/data-operator"
```

Default worker mode is data execution. Leave specialized inference and witness flags unset and keep Git integration enabled for ledger verification. Start the worker with current code: older binaries incorrectly placed runtime evidence under the launch directory even when `--working-dir` was set.

In the original terminal:

```bash
./g8e auth enroll pending
./g8e auth enroll approve <data-worker-request-id> --yes
./g8e operator list
./g8e operator show <data-worker-operator-id> --json
./g8e operator bind <data-worker-session-id> --yes
./g8e operator bind list
./g8e auth context
```

Verify the selected worker is `remote`, `active`, has runtime role `data`, and has the intended working directory. The worker terminal should show its pub/sub connection and command-channel subscription. Registration alone does not prove a command subscriber exists; the file scenario below tests delivery and execution.

### 4. Run the scenarios

The file scenario defaults to a deterministic fake model. It still traverses the Ensemble, SSE approval response, Gateway governance, worker command execution, receipt, and governed read-back paths. It does not exercise an external LLM.

```bash
./g8e demos scenarios run ensemble-chat-file-create \
  --mtls-url https://localhost:8443 \
  --public-url http://localhost:8080 \
  --ensemble-url http://127.0.0.1:8000 \
  --out ./reports/localhost-smoke/scenarios

./g8e demos scenarios run ensemble-document-update \
  --mtls-url https://localhost:8443 \
  --public-url http://localhost:8080 \
  --ensemble-url http://127.0.0.1:8000 \
  --cert "$PWD/.local.dev/full/ensemble/.g8e/pki/issued/apps/g8ee.crt" \
  --key "$PWD/.local.dev/full/ensemble/.g8e/pki/issued/apps/g8ee.key" \
  --ca "$PWD/.local.dev/full/ensemble/.g8e/pki/trust/g8eg-ca-bundle.pem" \
  --out ./reports/localhost-smoke/scenarios
```

The harness loads the current CLI identity and worker binding. `--operator-id` selects a logical ID and `--operator-session` selects a session; supplied constraints must resolve to one active registry record. A single target flag replaces the inherited pair and resolves its missing half. It does not change the server-side CLI binding; bind the CLI first.

Require both commands to exit zero with `ok` summaries. File creation requires a correlated completed `FILE_EDIT` receipt with a non-empty signature and exact governed read-back. Despite its name, the document scenario uses the enrolled Ensemble app certificate to submit two unbound `DOCUMENT_UPDATE` envelopes directly to the Gateway's platform-record path, creates a document, merges a partial patch, and verifies each synchronous signed receipt plus changed and preserved fields. It does not call the Ensemble or a model, and it must not target the remote data worker for this Gateway-local mutation.

Add `--verbose` to diagnose requests. Follow daemon logs with `./g8e gw logs --follow` and inspect the worker terminal.

### 5. Inspect and export recent audit data

Use the data worker session printed by `operator list`:

```bash
OPERATOR_SESSION_ID='<data-worker-session-id>'
./g8e audit receipts --session "$OPERATOR_SESSION_ID"
./g8e audit events --session "$OPERATOR_SESSION_ID" --limit 10
./g8e audit export --session "$OPERATOR_SESSION_ID" --out ./reports/localhost-smoke/audit-export.json
```

`audit receipts` has no `--limit` flag and returns the newest 50 matching records. The export contains the newest 100 matching receipts. Inspect document-update receipts in their executing session as well; the Gateway can execute document storage updates in its own process. These bounded queries supplement the scenarios' per-run receipt correlation.

### 6. Generate the data worker's evidence report

Choose a new output directory for each run and select the worker's local stores explicitly:

```bash
REPORT_DIR="$PWD/reports/localhost-smoke/operator-$(date -u +%Y%m%dT%H%M%SZ)"
./g8e report all \
  --runtime-dir "$PWD/.local.dev/data-operator/.g8e" \
  --data-dir "$PWD/.local.dev/data-operator/.g8e/data" \
  --ledger-dir "$PWD/.local.dev/data-operator/.g8e/data/ledger" \
  --out "$REPORT_DIR"
cat "$REPORT_DIR/verification_summary.csv"
```

Require `PASS` for `commitment_chain`, `git_merkle_root`, `file_mutation_linkage`, and `receipt_commitment_crosslink`. Mutation linkage must cover at least one mutation. Require zero `FAIL` and zero `SKIPPED` rows, and inspect `receipts.csv` and `commitments.csv` for the file run's `FILE_EDIT` evidence. A zero command exit or empty set of verification rows is insufficient. Missing stores or missing ledger evidence leave this smoke test incomplete.

Preserve the scenario results, audit export, report manifest, verification summary, and service logs together. Do not substitute Gateway-local CSV files for data-worker evidence.

### 7. Stop services started for this run

After archiving evidence:

```bash
./g8e operator stop "$OPERATOR_SESSION_ID"
./g8e ensemble stop
./g8e gw stop
```

Stop only services owned by this run. A targeted worker stop avoids stopping other local specialized workers. These commands preserve runtime state; do not delete PKI, vaults, or audit stores as part of the smoke test.

## Troubleshooting

- `dispatch: command delivered to no operator subscribers`: check the selected ID/session and CLI binding. The embedded substrate is not the outbound data command subscriber. Inspect the data worker's pub/sub connection, then retry after it subscribes.
- `witness operator: generic command execution is not permitted`: the selected worker has a witness role. Bind an active data worker.
- `401 G8E-1200`: check `auth context` after binding. Do not pass an Operator session UUID as the logical Operator ID or reuse the previous CLI session ID.
- `Proxy identity requires a valid Gateway signature`: use current harness code, which attaches the Operator session Bearer credential to approval responses. Check active session validation and Ensemble logs; proxy identity headers alone are insufficient.
- Target discovery rejects a missing, inactive, or ambiguous target: inspect `operator list` and select an active worker explicitly. The harness does not silently substitute a different worker for conflicting constraints.
- Enrollment is pending: approve the exact workload request and wait for enrollment and worker connection to finish before running scenarios.
- Approval POST fails: inspect the approval-listener error attached to the scenario failure, along with Ensemble logs. Only 2xx responses increment the approval count; requests honor scenario cancellation.
- Report lacks `FILE_EDIT` or has skipped verification: check the worker runtime path and completed mutation evidence. Do not call the run complete until the missing evidence is resolved.
- A worker started with an older binary may have written evidence under the shell's launch directory instead of `<working-dir>/.g8e`. Restart it with a current build and rerun the mutation; do not combine stores from different runtime roots.

## Evidence limits

A non-empty receipt signature is an assertion about the returned record, not independent signature verification. CSV verification checks commitment structure/signatures, Git roots, mutation linkage, and receipt-to-commitment cross-links; it does not independently verify receipt signatures or persistence attestations. Use the [release evidence workflow](../devs/release_process.md) for its required cryptographic and assessment checks.

This smoke run proves only the tested ingress paths in `doctrine` posture. It does not establish FedRAMP authorization, strict L2/L3 enforcement, non-exportability of vault keys, or every network data-flow property. Assessment-bound KSI evaluation requires legitimate scope identifiers and an evidence window; do not invent those to obtain a passing report.

## Links out

- [Getting Started](getting_started.md)
- [Ensemble Architecture](../ensemble/index.md)
- [Developer Guidelines](../devs/devs.md)
- [Testing Guide](../devs/tests.md)
- [Release Process](../devs/release_process.md)
- [Demo Harness](../../demos/README.md)
