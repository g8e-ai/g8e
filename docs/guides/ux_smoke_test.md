---
doc_id: ux_smoke_test
title: Headless End-to-End UX Smoke Test
audience: developers, evaluators, compliance auditors
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - docs/guides/
  - internal/cli/cmd/demos/
  - internal/services/reporting/
related:
  - docs/guides/unified_stack.md
  - docs/guides/getting_started.md
  - docs/guides/docker_gateway.md
  - docs/ensemble/index.md
  - docs/architecture/console.md
  - demos/README.md
  - docs/guides/governance_posture.md
when_to_read: Running end-to-end validation of the full g8e platform stack (gateway, operator, ensemble, console) in a Docker environment; troubleshooting platform enrollment or governance operations; auditing audit-trail integrity and CSV evidence generation.
do_not_use_for:
  - Platform coding invariants (docs/devs/devs.md)
  - Runtime and package ownership (docs/devs/codemap.md)
  - Test selection and CI scope (docs/devs/tests.md)
  - Release execution (docs/devs/release_process.md)
  - Diagnostics and recovery (docs/devs/troubleshooting.md)
---

# Headless End-to-End UX Smoke Test

## Purpose

Exercises the current headless Docker workflow from the repository root. The runbook starts the gateway, enrolls an mTLS-only owner identity, starts and approves the operator and ensemble workloads, runs governed file and document mutations, inspects their audit records, verifies the operator-local CSV evidence report, and downloads the published platform binary. The suite is deterministic by default and reproducible without external dependencies; optional real-model paths exercise the full ensemble pipeline against live Ollama endpoints.

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Prerequisites](#prerequisites)
- [Procedures](#procedures)
- [Claim-to-check mapping](#claim-to-check-mapping-and-limits)
- [Troubleshooting](#troubleshooting)
- [Links out](#links-out)

## Invariants

| ID | Claim | Verification Method |
| --- | --- | --- |
| INV-SMOKE-01 | CLI exposes documented command tree and lists MCP integrations | `./g8e --help` and `./g8e mcp agent list` exit zero and show all subcommands |
| INV-SMOKE-02 | Default Docker profile starts gateway; `bootstrapped` profile adds operator and ensemble | `./g8e docker status` reports all services healthy after `docker compose up -d` and profile selection |
| INV-SMOKE-03 | Linux AMD64 binary reports FIPS 140-3 approved mode and linked module | `docker exec g8e-gateway /g8e version --fips` prints `FIPS 140-3 mode: enabled` and module version |
| INV-SMOKE-04 | Headless owner enrollment creates mTLS credentials without browser and prints UUIDs | `./g8e auth enroll user --headless -e localhost` produces User ID and CLI Session ID |
| INV-SMOKE-05 | Owner can list and approve platform workload enrollment requests over mTLS | `./g8e auth enroll pending` and `./g8e auth enroll approve <id> --yes` complete successfully |
| INV-SMOKE-06 | Ensemble executes file_create tool via LLM, obtains signed receipt, and verified read-back succeeds | `./g8e demos scenarios run ensemble-chat-file-create` completes with `ok` status and non-empty signature |
| INV-SMOKE-07 | Direct DOCUMENT_UPDATE requests create and merge documents while preserving untouched fields | `./g8e demos scenarios run ensemble-document-update` completes with `ok` status and both fields verified |
| INV-SMOKE-08 | Gateway exposes recent receipts, events, and audit data over mTLS; operator exports to CSV | `./g8e audit receipts`, `./g8e audit events`, and `./g8e audit export` complete successfully |
| INV-SMOKE-09 | Operator CSV verification pass validates commitment chain, hashes, signatures, Git root, mutations, and cross-links | `docker exec g8e-data-operator /g8e report all` generates `verification_summary.csv` with all checks `PASS` |
| INV-SMOKE-10 | Gateway publishes platform binaries via HTTP discovery at `/.well-known/g8e/bin/g8e-<os>-<arch>` | `curl http://localhost:8080/.well-known/g8e/bin/g8e-linux-amd64` downloads executable matching gateway version |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Docker Compose unified stack | `docker-compose.yml` | Gateway at port 8443 (mTLS), 8080 (HTTP); operator, ensemble, inference operator services with correct volume mounts |
| Smoke test runbook | `docs/guides/ux_smoke_test.md` | End-to-end procedural walkthrough with claim mapping |
| CLI command tree | `./g8e --help` | All subcommands listed; demo scenarios and audit commands present |
| Report CSV generation | `internal/services/reporting/` | All required files in `internal/constants/paths.go` |
| Scenario definitions | `demos/scenarios/` | `ensemble-chat-file-create` and `ensemble-document-update` scenarios defined |

## Procedures

### Prerequisites

- A Linux AMD64 host for FIPS and binary-matching checks below. Other supported hosts can run most steps and must select their matching `g8e-<os>-<arch>` binary in the distribution check.
- Docker 24.0 or later and Docker Compose v2.
- Shell commands: `curl`, `file`, `awk`, `grep`, `docker`, `docker-compose`.
- The `g8e` CLI binary at `./g8e`. Run `make build` if absent.
- An Ollama endpoint and working language model only if the optional real-provider path is selected in step 7. The harness uses its deterministic fake provider by default.
- Run all commands from the repository root.

**Warning:** Cleanup commands in step 2 permanently delete unified-stack containers and volumes, including gateway PKI, operator vault, audit data, receipts, commitments, and workload state. Export any evidence required before starting.

### Command classification

**Host-network commands** read the host's local CLI configuration and mTLS credentials, then query the gateway at `localhost:8443`. These include `g8e operator list`, `g8e auth enroll pending`, `g8e auth enroll approve`, `g8e auth enroll deny`, `g8e audit receipts`, `g8e audit events`, `g8e audit summary`, `g8e gw data operators`, `g8e gw data audit list`, and `g8e gw data store list`. They do not access the operator Docker volume.

**Operator-filesystem commands** read persistent operator state directly from `/root/.g8e/` in the operator volume. Run these as `docker exec g8e-data-operator /g8e <subcommand>`. These include `g8e vault status`, `g8e report all`, and `g8e compliance ksi`. Running them through the host binary inspects the host's unrelated runtime tree.

**Docker status.** Use `g8e docker status` for unified-stack service health, not `g8e gw status`. The latter attempts an authenticated gateway request and checks a host-local PID file, neither of which represents the gateway container before enrollment.

**Image rebuilding.** Docker images build the binary from the current source tree. Rebuild with `./g8e docker build` after source code changes and before restarting the stack if existing images may be stale.

### Governance posture

The root Docker Compose configuration starts the gateway in the default `doctrine` posture: L1 enforced, L2 and L3 audited, L4 and L5 active. Headless enrollment skips WebAuthn passkey registration, so this runbook does not test `ratify`, `consensus`, or `notary` enforcement. Refer to [Governance Posture Setup](./getting_started.md) for alternative configurations.

### Step 1: Verify the CLI

```bash
./g8e version
./g8e --help
./g8e mcp agent list
file ./g8e
```

Expected: `g8e version` prints the version, build ID, build time, and platform; the help command prints the top-level command tree; `g8e mcp agent list` lists the supported integrations; and `file` identifies the Linux binary as statically linked.

### Step 2: Reset, build, and start the gateway

Record any containers left by a previous run, remove the unified-stack containers and volumes, and remove the host's local CLI credential material:

```bash
docker ps -a --filter 'name=^/g8e-' --format '{{.Names}}\t{{.Status}}'
./g8e docker clean --yes
./g8e auth logout
docker ps -a --filter 'name=^/g8e-' --format '{{.Names}}\t{{.Status}}'
```

The final container listing is empty. `docker clean` removes containers, volumes, and the network. `auth logout` removes local CLI credentials but preserves the host trust bundle and does not revoke the deleted gateway-side session.

Build the images if they are absent or stale:

```bash
./g8e docker build
```

Start the default profile, which contains only the gateway:

```bash
./g8e docker start
./g8e docker status
```

Wait until `g8e-gateway` reports healthy. Then inspect the FIPS state of the Linux AMD64 binary in the image:

```bash
docker exec g8e-gateway /g8e version --fips
```

Expected: the output contains `FIPS 140-3 mode: enabled`, `FIPS enforcement: disabled`, and the module version. The command also prints a warning that non-approved primitives are not rejected while enforcement is off, then exits zero.

### Step 3: Enroll the headless owner

```bash
./g8e auth enroll user --headless -e localhost
```

Expected: no browser opens. The command prints `User ID: <uuid>` and `CLI Session ID: <uuid>`. Save both values for step 8. The resulting identity authenticates CLI mTLS requests but cannot authenticate to the Console SPA.

### Step 4: Start the remaining workloads

```bash
./g8e docker start --profile bootstrapped --skip-enroll
./g8e docker status
```

Expected: the gateway, operator, and ensemble containers are present. The workloads can remain unready while their enrollment requests await approval.

### Step 5: Review the workload enrollments

List the pending requests:

```bash
./g8e auth enroll pending

# Reject a request instead of approving it:
# ./g8e auth enroll deny <request-id> --yes
```

Approve each operator and ensemble request by its exact request ID:

```bash
./g8e auth enroll approve <operator-request-id> --yes
./g8e auth enroll approve <ensemble-request-id> --yes
```

The order does not affect the approval protocol. Each workload polls until its request is approved and then completes certificate enrollment.

### Step 6: Verify the full stack

Repeat `./g8e docker status` until all four services report healthy, then run:

```bash
./g8e operator list
./g8e gw data operators
docker exec g8e-data-operator /g8e vault status
OPERATOR_PORTS="$(docker port g8e-data-operator)"
test -z "${OPERATOR_PORTS}"
curl -fsS http://localhost:8000/health
curl -fsSk -o /dev/null https://localhost:8443/console/
```

Expected:

- `g8e operator list` shows the operator and its session ID. Save the full session ID for step 9.
- `g8e gw data operators` shows the registered operator instance.
- `vault status` reports that the operator vault is initialized and unlocked.
- The `docker port` assertion exits zero because the operator publishes no host ports.
- The ensemble health endpoint and the Gateway console (`/console/`) return successful responses.

These checks establish service availability, owner-visible operator registration, the absence of published operator ports, and operator-local vault initialization. They do not inspect traffic contents or prove that key material never leaves the operator.

### Step 7: Select the LLM provider

For a deterministic smoke test, no provider variables are required. The harness defaults to the fake provider and model, which exercises the ensemble, governance, operator execution, receipt, and read-back paths without an external model.

To include a real Ollama call, save its endpoint on the console's Inference
page, then export the provider and model selection in the shell that runs step 8:

```bash
export G8E_HARNESS_LLM_PROVIDER=ollama
export G8E_HARNESS_LLM_MODEL='<tool-capable-model-tag>'
OLLAMA_ENDPOINT='http://<ollama-host>:<port>'
curl -fsS "${OLLAMA_ENDPOINT}/api/tags" | grep -F "\"name\":\"${G8E_HARNESS_LLM_MODEL}\""
```

The model must support tool calls and be available at the endpoint. If the real provider is unavailable, return to the deterministic path with:

```bash
export G8E_HARNESS_LLM_PROVIDER=fake
unset G8E_HARNESS_LLM_MODEL
```

### Step 8: Run governed file and document mutations

Run the ensemble file-creation scenario with the IDs printed during enrollment:

```bash
./g8e demos scenarios run ensemble-chat-file-create \
  --mtls-url https://localhost:8443 \
  --public-url http://localhost:8080 \
  --ensemble-url http://localhost:8000 \
  --user-id <user-id-from-step-3> \
  --cli-session-id <cli-session-id-from-step-3>
```

The scenario sends a chat request to the ensemble. The selected provider chooses `file_create`; an SSE-connected CLI auto-approver approves the file-edit request; the governed operation executes on the operator; and the harness accepts only a correlated `COMPLETED` `FILE_EDIT` receipt with a non-empty signature. It then performs a governed `read_file` call and checks the unique file content. With the fake provider this is deterministic; with Ollama it includes a real model call.

Run the document merge regression scenario against the same stack:

```bash
./g8e demos scenarios run ensemble-document-update \
  --mtls-url https://localhost:8443 \
  --public-url http://localhost:8080 \
  --ensemble-url http://localhost:8000 \
  --user-id <user-id-from-step-3> \
  --cli-session-id <cli-session-id-from-step-3>
```

Despite its historical `ensemble-` name, this scenario does not call the ensemble or an LLM. It submits two `DOCUMENT_UPDATE` envelopes directly to the gateway admission API, first creating a document with `merge=false` and then applying a partial patch with `merge=true`. Governed reads verify both the changed field and the untouched fields.

Required outcome: both summaries report `ok`. The notes show correlated receipt transaction IDs and successful read-back checks. A scenario command exits nonzero if its scenario fails.

For detailed request and response output, add `--verbose`. Follow gateway logs in another terminal with:

```bash
./g8e docker logs -f g8e-gateway
```

### Step 9: Inspect and export recent audit data

Set the operator session ID saved in step 6:

```bash
export OPERATOR_SESSION_ID='<operator-session-id>'
```

Query the gateway over mTLS and archive the recent receipt response:

```bash
./g8e audit receipts --session "${OPERATOR_SESSION_ID}"
./g8e audit events --session "${OPERATOR_SESSION_ID}" --limit 100
./g8e audit summary --session "${OPERATOR_SESSION_ID}"
./g8e gw data audit list --operator-session-id "${OPERATOR_SESSION_ID}" --limit 100
./g8e audit export --session "${OPERATOR_SESSION_ID}" --out ./receipts-export.json
grep -q 'FILE_EDIT' ./receipts-export.json
grep -q 'DOCUMENT_UPDATE' ./receipts-export.json
```

Expected: the receipt table contains `COMPLETED` mutation records, the event and summary commands return operator-scoped audit data, and both action-type assertions pass. The scenario itself provides the stronger per-run correlation and side-effect checks.

`audit receipts` returns the newest 50 matching records because the CLI does not expose receipt pagination. `audit export` returns the newest 100 matching records. Run this step immediately after the scenarios; the export is a bounded recent-receipt archive, not an unbounded export of the entire database.

### Step 10: Generate and verify the operator CSV report

Choose an explicit output directory so the host copy cannot select stale output from an earlier run:

```bash
REPORT_NAME="ux-smoke-$(date -u +%Y%m%dT%H%M%SZ)"
docker exec g8e-data-operator /g8e report all --out "/root/reports/${REPORT_NAME}"
mkdir -p "./reports/${REPORT_NAME}"
docker cp "g8e-data-operator:/root/reports/${REPORT_NAME}/." "./reports/${REPORT_NAME}/"
REPORT_DIR="./reports/${REPORT_NAME}"
cat "${REPORT_DIR}/verification_summary.csv"
awk -F, '$1 == "commitment_chain" && $4 == "PASS" { print; found=1 } END { exit !found }' "${REPORT_DIR}/verification_summary.csv"
awk -F, '$1 == "git_merkle_root" && $4 == "PASS" { print; found=1 } END { exit !found }' "${REPORT_DIR}/verification_summary.csv"
awk -F, '$1 == "file_mutation_linkage" && $4 == "PASS" && $5 !~ /^0 mutations checked/ { print; found=1 } END { exit !found }' "${REPORT_DIR}/verification_summary.csv"
awk -F, '$1 == "receipt_commitment_crosslink" && $4 == "PASS" { print; found=1 } END { exit !found }' "${REPORT_DIR}/verification_summary.csv"
test -z "$(awk -F, '$4 == "FAIL" || $4 == "SKIPPED" { print }' "${REPORT_DIR}/verification_summary.csv")"
grep -q 'FILE_EDIT' "${REPORT_DIR}/receipts.csv"
grep -q 'FILE_EDIT' "${REPORT_DIR}/commitments.csv"
```

Expected: `report all` exits zero and writes `receipts.csv`, `sessions.csv`, `events.csv`, `file_mutations.csv`, `commitments.csv`, `executions.csv`, `file_diffs.csv`, `replay_nonces.csv`, `suspended_transactions.csv`, Git ledger CSVs, `verification_summary.csv`, and `manifest.csv` when their stores are available. The checks require a non-empty file mutation, a captured Git root, a valid commitment chain, receipt-to-commitment linkage, no failed or skipped verification rows, and `FILE_EDIT` records in both operator-local receipts and commitments.

The CSV verifier checks commitment structure and signatures, not receipt signatures or final receipt-persistence attestations. The gateway executes the document updates in its own process, so the operator-local CSV files contain the `FILE_EDIT` execution but do not necessarily contain the gateway-local `DOCUMENT_UPDATE` records inspected in step 9.

### Step 11: Verify binary distribution

The following filename and equality check target Linux AMD64, matching the gateway container:

```bash
curl -fsS -o ./g8e-linux-amd64 http://localhost:8080/.well-known/g8e/bin/g8e-linux-amd64
chmod +x ./g8e-linux-amd64
file ./g8e-linux-amd64
./g8e-linux-amd64 version
test "$(./g8e-linux-amd64 version)" = "$(docker exec g8e-gateway /g8e version)"
rm -f ./g8e-linux-amd64
```

Expected: the downloaded file is a runnable statically linked Linux AMD64 binary, and its version, build ID, build time, and platform match the binary running in the gateway container. Other supported clients can download the corresponding `g8e-<os>-<arch>` filename, but its platform line will not equal the Linux AMD64 container output.

### Step 12 (optional): Evaluate KSIs with an assessment binding

`compliance ksi` is not an unscoped smoke command. It fails closed unless the caller supplies identifiers and an evidence window from a real assessment. If those values exist, run the evaluation against the operator-local stores:

```bash
docker exec g8e-data-operator /g8e compliance ksi \
  --class C \
  --catalog /docs/reference/ksi-catalog.json \
  --scope-id '<assessment-scope-id>' \
  --run-id '<assessment-run-id>' \
  --assertion-assessment-id '<canonical-assertion-assessment-id>' \
  --evidence-window-start-unix-ms '<inclusive-start-ms>' \
  --evidence-window-end-unix-ms '<inclusive-end-ms>' \
  > ./ksi-result.json
grep -q '"class": "C"' ./ksi-result.json
grep -q '"results":' ./ksi-result.json
```

Repeat `--assertion-assessment-id`, `--attempt-id`, `--scenario-id`, and `--action-id` as required by the assessment population. Do not invent identifiers to satisfy the CLI. A result reports evidence alignment for the declared scope; it does not establish FedRAMP authorization.

### Step 13: Stop or clean up

Stop containers while preserving volumes:

```bash
./g8e docker stop
```

Permanently remove all unified-stack containers and volumes:

```bash
./g8e docker clean --yes
```

The second command deletes the gateway PKI, operator vault and audit data, and all workload state.

## Troubleshooting

- `g8e docker status` shows `starting`: wait and retry. If a container exits, inspect it with `./g8e docker logs <service-name>`.
- Enrollment fails after a reset: run `./g8e auth logout` to remove stale local CLI credentials, then retry against the new gateway PKI.
- A network command returns `404 page not found`: rebuild stale images with `./g8e docker build`, restart the stack, and compare `./g8e version` with `docker exec g8e-gateway /g8e version`.
- Pending enrollments are empty while workloads remain unready: wait for the workloads to submit their requests, then inspect `./g8e docker logs g8e-data-operator`, or `./g8e docker logs ensemble`.
- The file scenario fails with Ollama: confirm the endpoint and model, or switch to `G8E_HARNESS_LLM_PROVIDER=fake` and unset the model and endpoint variables.
- A scenario reports `ok` but a mutation is absent from `audit receipts`: the newest-50 API window may have advanced. Check the newest-100 `audit export` immediately, and rely on the scenario's own correlated receipt and read-back result for its per-run assertion.
- `report all` lacks `FILE_EDIT` rows: the file scenario did not complete on the operator, or the report was run against the wrong filesystem. Re-run the scenario, then execute `/g8e report all` inside `g8e-data-operator`.
- A verification row is `SKIPPED`: the corresponding store or Git ledger was unavailable. Treat the smoke run as incomplete and inspect the operator logs instead of reporting the check as covered.
- The downloaded binary does not execute: select the filename matching the host OS and architecture. The exact comparison in step 11 applies only to Linux AMD64.

## Claim-to-check mapping and limits

| Claim | Proof | Limitation |
| --- | --- | --- |
| Gateway and Operator use one static Go binary | `file ./g8e` and `file ./g8e-linux-amd64` both report `ELF 64-bit LSB executable, x86-64, statically linked`; downloaded binary version matches gateway container version. | Does not reproduce the release build or inspect its provenance. |
| FIPS 140-3 approved mode is observable at runtime | `docker exec g8e-gateway /g8e version --fips` prints `FIPS 140-3 mode: enabled`. | Default image reports enforcement `disabled`; this is not strict FIPS-only operation. |
| Secure MCP governs ensemble-selected file tool | File scenario selects `file_create`, obtains a `COMPLETED` receipt with non-empty signature, and performs governed read-back. | Fake-provider path does not call an external model. Use real Ollama provider for full LLM exercise. |
| Operator is outbound-only | Operator registers with gateway; `docker port g8e-data-operator` output is empty. | Validates published ports only, not every socket or packet. |
| Mutations traverse governance pipeline | Scenarios require completed receipts for `FILE_EDIT` and `DOCUMENT_UPDATE`. | Doctrine enforces L1 while L2 and L3 audit; does not prove other postures. |
| Operator-local evidence links receipts, commitments, mutations | `report all` verification pass validates commitment chain, Git root, mutation linkage, receipt cross-links. | CSV verifier does not verify receipt signatures or persistence attestations. |
| Raw data remains on operator | Governed file executes and reads back through operator; operator state in Docker volume. | Smoke test does not capture network traffic; does not independently prove broader data-flow claim. |
| Vault keys remain under owner control | `vault status` confirms operator vault initialized and unlocked. | Status does not prove non-exportability or absence from network traffic. |
| Governance posture configurations exist | [Governance posture documentation](./getting_started.md) lists configurations. | Smoke run executes only default `doctrine` posture. |
| Continuous compliance evidence evaluates over live stores | Correctly bound KSI command evaluates operator stores. | Smoke run does not create legitimate assessment binding; KSI output is not authorization. |

## Anti-patterns

- Mixing host and operator commands: do not run `./g8e report all` on the host; use `docker exec g8e-data-operator /g8e report all`.
- Stale images: do not skip `./g8e docker build` after source changes; rebuilt images from the current tree are required.
- Assuming audit state persists: data removed by `./g8e docker clean` includes gateway PKI and operator vault. Export evidence before cleanup.
- Running scenarios before enrollment: workload enrollments must be approved before scenarios execute; `./g8e auth enroll pending` and `./g8e auth enroll approve` are prerequisites.
- Interpreting CSV verification as cryptographic proof: the verifier checks hash chains and signatures on commitments; it does not verify receipt signatures or persistence attestations independently.
- Testing postures this smoke run does not cover: `ratify`, `consensus`, `notary` require browser WebAuthn enrollment and explicit posture configuration. Use [governance posture setup](./getting_started.md) for those paths.

## Links out

- [Getting Started Guide](./getting_started.md) — Initial platform setup and governance posture configuration.
- [Unified Docker Stack](./unified_stack.md) — Docker Compose topology, volumes, and service coordination.
- [Docker Gateway](./docker_gateway.md) — Gateway lifecycle management and container operations.
- [Ensemble Architecture](../ensemble/index.md) — Ensemble component design and LLM integration.
- [Console Architecture](../architecture/console.md) — the Gateway-served browser console.
- [Demo Harness](../../demos/README.md) — Scenario definitions and evaluation framework.
- [Sovereignty Gauntlet](./sovereignty_gauntlet.md) — Advanced multi-region and cross-enrollment testing.
