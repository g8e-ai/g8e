# g8e Headless End-to-End UX Smoke Test

This runbook exercises the current headless Docker workflow from the repository root. It starts the gateway, enrolls an mTLS-only owner identity, starts and approves the operator, ensemble, and dashboard workloads, runs governed file and document mutations, inspects their audit records, verifies the operator-local CSV evidence report, and downloads the published platform binary. See the [g8ee documentation](../ensemble/index.md) for the ensemble architecture and the [g8ed documentation](../dashboard/index.md) for the dashboard component.

## What this proves

A successful run verifies that:

- The CLI exposes the documented command tree and MCP agent integrations.
- The default Docker profile starts the gateway, and the `bootstrapped` profile adds the operator, ensemble, and dashboard.
- The Linux AMD64 container binary reports that FIPS 140-3 approved mode is active and identifies its linked cryptographic module. Strict FIPS enforcement is a separate runtime setting and is off in the default image.
- Headless owner enrollment creates local mTLS credentials without opening a browser and prints the user and CLI session IDs.
- The enrolled owner can list and approve platform workload enrollment requests over mTLS.
- The ensemble can use its configured LLM provider to select `file_create`, submit the governed operation, and verify the resulting operator-side file through a governed read.
- Direct governed `DOCUMENT_UPDATE` requests create and partially merge a document while preserving untouched fields.
- The gateway exposes recent receipts and audit events over mTLS, and the operator exports its local persistent stores to CSV.
- The operator-local verification pass checks commitment-chain structure, canonical commitment hashes, Auditor signatures on commitments, the Git ledger root, file-mutation linkage, and receipt-to-commitment cross-links.
- The gateway publishes the image-baked platform binaries through its HTTP discovery endpoint.

This run does not verify browser passkey authentication, dashboard functionality beyond HTTP availability, strict FIPS enforcement, L2 or L3 enforcement postures, receipt signatures cryptographically, final receipt-persistence attestations, network-level data-flow claims, or key non-exportability.

## Prerequisites

- A Linux AMD64 host for the exact FIPS and downloaded-binary commands below. Other supported hosts can run most steps but must select their matching binary in the distribution check.
- Docker 24.0 or later and Docker Compose v2.
- `curl`, `file`, `awk`, and `grep`.
- The `g8e` CLI binary at `./g8e`. Run `make build` if it is absent.
- An Ollama endpoint only if the optional real-provider path is selected. The harness otherwise uses its deterministic fake provider by default.

Run all commands from the repository root. The cleanup commands in step 2 permanently delete the current unified-stack containers and volumes, including gateway PKI, operator vault data, receipts, commitments, and workload state. Export any evidence that must be retained before starting.

## Docker command classification

**Host network commands.** Commands such as `g8e operator list`, `g8e auth pending-platform-enrollments`, `g8e auth approve-platform-enrollment`, `g8e audit receipts`, `g8e audit events`, `g8e audit summary`, `g8e gw data operators`, and `g8e gw data audit list` read the host's local CLI configuration and mTLS credentials, then query the gateway at `localhost:8443`. They do not read the operator's Docker volume.

**Operator filesystem commands.** Commands such as `g8e vault status`, `g8e report all`, and `g8e compliance ksi` read persistent operator state directly. In this Docker topology that state is under `/root/.g8e/` in the operator volume, so run these commands as `docker exec g8e-operator /g8e <subcommand>`. Running them through the host binary inspects the host's unrelated runtime tree.

Use `g8e docker status`, not `g8e gw status`, for Docker service health. `g8e gw status` first attempts an authenticated gateway request and then checks a host-local PID file, neither of which represents the gateway container before enrollment.

Docker images build the binary from the current source tree. Rebuild with `./g8e docker build` after source changes and before starting the stack when the existing images may be stale.

## Governance posture

The root Docker Compose configuration starts the gateway in the default `doctrine` posture. L1 is enforced, L2 and L3 are audited, and L4 and L5 remain active. Headless enrollment skips WebAuthn passkey registration, so this runbook does not test `ratify`, `consensus`, or `notary` enforcement. Follow the [governance posture setup](./getting_started.md#governance-postures) for those configurations.

## The run

### 1. Verify the CLI

```bash
./g8e version
./g8e --help
./g8e mcp agent list
file ./g8e
```

Expected: `g8e version` prints the version, build ID, build time, and platform; the help command prints the top-level command tree; `g8e mcp agent list` lists the supported integrations; and `file` identifies the Linux binary as statically linked.

### 2. Reset, build, and start the gateway

Record any containers left by a previous run, remove the unified-stack containers and volumes, and remove the host's local CLI credential material:

```bash
docker ps -a --filter 'name=^/g8e-' --format '{{.Names}}\t{{.Status}}'
./g8e docker clean
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

### 3. Enroll the headless owner

```bash
./g8e auth enroll user --headless -e localhost
```

Expected: no browser opens. The command prints `User ID: <uuid>` and `CLI Session ID: <uuid>`. Save both values for step 8. The resulting identity authenticates CLI mTLS requests but cannot authenticate to the Console SPA.

### 4. Start the remaining workloads

```bash
./g8e docker start --profile bootstrapped --skip-enroll
./g8e docker status
```

Expected: the gateway, operator, ensemble, and dashboard containers are present. The three workloads can remain unready while their enrollment requests await approval.

### 5. Approve the workload enrollments

List the pending requests:

```bash
./g8e auth pending-platform-enrollments
```

Approve each operator, ensemble, and dashboard request by its exact request ID:

```bash
./g8e auth approve-platform-enrollment <operator-request-id> --yes
./g8e auth approve-platform-enrollment <ensemble-request-id> --yes
./g8e auth approve-platform-enrollment <dashboard-request-id> --yes
```

The order does not affect the approval protocol. Each workload polls until its request is approved and then completes certificate enrollment.

### 6. Verify the full stack

Repeat `./g8e docker status` until all four services report healthy, then run:

```bash
./g8e operator list
./g8e gw data operators
docker exec g8e-operator /g8e vault status
OPERATOR_PORTS="$(docker port g8e-operator)"
test -z "${OPERATOR_PORTS}"
curl -fsS http://localhost:8000/health
curl -fsS -o /dev/null http://localhost:3000/
```

Expected:

- `g8e operator list` shows the operator and its session ID. Save the full session ID for step 9.
- `g8e gw data operators` shows the registered operator instance.
- `vault status` reports that the operator vault is initialized and unlocked.
- The `docker port` assertion exits zero because the operator publishes no host ports.
- The ensemble health endpoint and dashboard root return successful responses.

These checks establish service availability, owner-visible operator registration, the absence of published operator ports, and operator-local vault initialization. They do not inspect traffic contents or prove that key material never leaves the operator.

### 7. Select the LLM provider

For a deterministic smoke test, no provider variables are required. The harness defaults to the fake provider and model, which exercises the ensemble, governance, operator execution, receipt, and read-back paths without an external model.

To include a real Ollama call, export all three values in the shell that runs step 8:

```bash
export G8E_HARNESS_LLM_PROVIDER=ollama
export G8E_HARNESS_LLM_MODEL='<tool-capable-model-tag>'
export G8E_HARNESS_LLM_ENDPOINT='http://<ollama-host>:<port>'
curl -fsS "${G8E_HARNESS_LLM_ENDPOINT}/api/tags" | grep -F "\"name\":\"${G8E_HARNESS_LLM_MODEL}\""
```

The model must support tool calls and be available at the endpoint. If the real provider is unavailable, return to the deterministic path with:

```bash
export G8E_HARNESS_LLM_PROVIDER=fake
unset G8E_HARNESS_LLM_MODEL G8E_HARNESS_LLM_ENDPOINT
```

### 8. Run governed file and document mutations

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

### 9. Inspect and export recent audit data

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

### 10. Generate and verify the operator CSV report

Choose an explicit output directory so the host copy cannot select stale output from an earlier run:

```bash
REPORT_NAME="ux-smoke-$(date -u +%Y%m%dT%H%M%SZ)"
docker exec g8e-operator /g8e report all --out "/root/reports/${REPORT_NAME}"
mkdir -p "./reports/${REPORT_NAME}"
docker cp "g8e-operator:/root/reports/${REPORT_NAME}/." "./reports/${REPORT_NAME}/"
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

### 11. Verify binary distribution

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

### 12. Optional: evaluate KSIs with an assessment binding

`compliance ksi` is not an unscoped smoke command. It fails closed unless the caller supplies identifiers and an evidence window from a real assessment. If those values exist, run the evaluation against the operator-local stores:

```bash
docker exec g8e-operator /g8e compliance ksi \
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

### 13. Stop or clean up

Stop containers while preserving volumes:

```bash
./g8e docker stop
```

Permanently remove all unified-stack containers and volumes:

```bash
./g8e docker clean
```

The second command deletes the gateway PKI, operator vault and audit data, and all workload state.

## Claim-to-check mapping and limits

| Claim | Check | Limit |
|---|---|---|
| The Gateway and Operator use one static Go binary | `file` checks the CLI and downloaded binary; the downloaded binary matches `/g8e` in the gateway container; the operator uses the same image. | This does not reproduce the release build or inspect its provenance. |
| FIPS approved mode is observable at runtime | `docker exec g8e-gateway /g8e version --fips`. | The default image reports enforcement off; this is not strict FIPS-only operation. |
| Secure MCP governs an ensemble-selected file tool | The file scenario selects `file_create`, obtains a completed receipt, and performs governed read-back. | The fake-provider path does not call an external model. |
| The operator is outbound-only | The operator registers with the gateway and `docker port g8e-operator` is empty. | This checks published ports, not every socket or packet. |
| Mutations traverse the governance pipeline | The scenarios require completed governed receipts for `FILE_EDIT` and `DOCUMENT_UPDATE`. | Doctrine enforces L1 while L2 and L3 are audited; this run does not prove other postures. |
| Operator-local evidence links receipts, commitments, and file mutations | `report all` checks the commitment chain, Git root, mutation linkage, and receipt cross-links. | The CSV verifier does not verify receipt signatures or persistence attestations. |
| Raw data remains on the host | The governed file is executed and read back through the operator, and operator state is stored in its volume. | This smoke test does not capture network traffic and therefore does not independently prove the broader data-flow claim. |
| Vault keys remain under owner control | `vault status` confirms operator-local vault initialization. | Vault status does not prove non-exportability or absence from network traffic. |
| Four posture configurations exist | The linked posture guide documents the configurations. | This run executes only the default doctrine posture. |
| Continuous compliance evidence can be evaluated | A correctly bound optional KSI command evaluates the live operator stores. | The smoke run does not create a legitimate assessment binding by itself, and KSI output is not an authorization. |

## Troubleshooting

- `g8e docker status` shows `starting`: wait and retry. If a container exits, inspect it with `./g8e docker logs <service-name>`.
- Enrollment fails after a reset: run `./g8e auth logout` to remove stale local CLI credentials, then retry against the new gateway PKI.
- A network command returns `404 page not found`: rebuild stale images with `./g8e docker build`, restart the stack, and compare `./g8e version` with `docker exec g8e-gateway /g8e version`.
- Pending enrollments are empty while workloads remain unready: wait for the workloads to submit their requests, then inspect `./g8e docker logs g8e-operator`, `./g8e docker logs ensemble`, or `./g8e docker logs dashboard`.
- The file scenario fails with Ollama: confirm the endpoint and model, or switch to `G8E_HARNESS_LLM_PROVIDER=fake` and unset the model and endpoint variables.
- A scenario reports `ok` but a mutation is absent from `audit receipts`: the newest-50 API window may have advanced. Check the newest-100 `audit export` immediately, and rely on the scenario's own correlated receipt and read-back result for its per-run assertion.
- `report all` lacks `FILE_EDIT` rows: the file scenario did not complete on the operator, or the report was run against the wrong filesystem. Re-run the scenario, then execute `/g8e report all` inside `g8e-operator`.
- A verification row is `SKIPPED`: the corresponding store or Git ledger was unavailable. Treat the smoke run as incomplete and inspect the operator logs instead of reporting the check as covered.
- The downloaded binary does not execute: select the filename matching the host OS and architecture. The exact comparison in step 11 applies only to Linux AMD64.

## See also

- [Getting Started](./getting_started.md)
- [Unified Docker Stack](./unified_stack.md)
- [Docker Gateway](./docker_gateway.md)
- [Ensemble Architecture](../architecture/ensemble.md)
- [Demo Harness](../../demos/README.md)
- [Sovereignty Gauntlet](./sovereignty_gauntlet.md)
