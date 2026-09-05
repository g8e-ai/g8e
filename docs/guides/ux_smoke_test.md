# g8e Headless End-to-End UX Smoke Test

This runbook walks a coding agent through the complete g8e user experience described in `README.md` and `docs/guides/getting_started.md`, using only CLI commands in a headless environment. It starts the unified Docker stack, performs a headless mTLS-only owner enrollment, brings up the operator, ensemble, and dashboard, approves the workload enrollments, and then drives a real AI call through the g8ee ensemble that produces a signed receipt in the audit vault. See the [g8ee documentation](../ensemble/index.md) for the ensemble architecture and the [g8ed documentation](../dashboard/index.md) for the dashboard component.

## What this proves

By the end of this run the agent will have verified that:

- The g8e binary is a single statically-linked executable that publishes platform binaries over the documented HTTP discovery endpoint.
- The unified Docker stack (`g8e docker start`) brings the gateway, operator, ensemble, and dashboard online and healthy, with the ensemble and dashboard HTTP surfaces responding directly.
- The container binary reports that FIPS 140-3 approved mode is active and identifies its validated cryptographic module at runtime.
- Headless `g8e auth enroll user --headless` mints mTLS credentials without a browser and emits a `User ID` and `CLI Session ID`.
- Platform workload enrollments for the operator, ensemble, and dashboard can be listed and approved from the CLI over mTLS.
- A real LLM call through the g8ee ensemble is translated into a typed MCP `tools/call`, routed through the five-layer governance pipeline, executed by the operator, persisted as a complete signed receipt in the SQLite audit store, and linked to the signed SQLite commitment chain inside the operator container.
- Governed `DOCUMENT_UPDATE` mutations pass through the same admission pipeline and preserve untouched fields during a partial merge.
- The receipts and audit events can be exported for archival, and the CSV evidence report independently checks ledger and cross-store integrity.

## Prerequisites

- Docker 24.0+ and Docker Compose v2.
- The `g8e` binary in the repository root. If it is not present, build it with `make build` and place it at `./g8e`.
- An Ollama endpoint reachable from the harness process, unless the deterministic fake provider is used.

If no real LLM is available, set `G8E_HARNESS_LLM_PROVIDER=fake` to run the scenario deterministically. That still exercises the platform and produces receipts, but it is not a real AI call.

## Docker command classification: network vs filesystem

This runbook runs the entire stack inside Docker containers. The `g8e` CLI commands fall into two categories, and only one category works from the host when the stack is in Docker:

**Network commands (run from the host).** These load CLI mTLS credentials and issue HTTPS requests to `localhost:8443`, which Docker port-forwards to the gateway container. They work from the host because they never touch the local `.g8e/` filesystem for state — they only read the CLI cert/key/trust-bundle files that `auth enroll user --headless` wrote in step 3. Examples: `g8e operator list`, `g8e auth pending-platform-enrollments`, `g8e auth approve-platform-enrollment`, `g8e audit receipts`, `g8e audit events`, `g8e audit summary`, `g8e gw data operators`, `g8e gw data audit list`.

**Filesystem commands (must run inside the container).** These read persistent state — the vault, audit SQLite databases, git ledger, replay store — directly from the `.g8e/` directory on disk. When the stack runs in Docker, that state lives inside the container's volume at `/root/.g8e/`, not on the host filesystem. Running these commands on the host reads an empty or stale `.g8e/` directory and produces wrong results (vault reports "not initialized", reports generate header-only CSVs, etc.). The binary inside the operator container is at `/g8e`. Run filesystem commands via `docker exec g8e-operator /g8e <subcommand>`. Examples: `g8e vault status`, `g8e report all`.

**Do not** run `g8e gw status` to check Docker gateway health. Its HTTP check loads CLI mTLS credentials via `api.NewClient`, which cannot exist before step 3 enrollment, and its fallback reads a local PID file that does not exist when the gateway runs inside Docker. Both paths report `State: STOPPED` even when the Docker container is healthy. Use `g8e docker status` instead.

**Rebuild Docker images after `make build`.** The Docker images bake the `g8e` binary in at build time. If you run `make build` to pick up source changes, you must also run `./g8e docker build` to update the images before `./g8e docker start`. A stale image can cause 404s on routes that exist in the new binary but not in the old one.

## Note on L2- and L3-enforcing postures

Headless enrollment deliberately skips the WebAuthn/FIDO2 passkey ceremony. Because of that, this runbook stays in the default `doctrine` posture (L1 enforced, L2 and L3 audited). Fully proving L3 under `ratify` or `notary`, or L2 under `consensus` or `notary`, requires optional follow-up configuration: interactive passkey enrollment for L3 and a multi-member Ed25519 quorum for L2.

## The run

### 1. Verify the CLI

```bash
./g8e version
./g8e --help
./g8e mcp agent list
```

Expected: `g8e version` prints the version, build id, and platform; `g8e --help` prints the top-level command tree; and `g8e mcp agent list` prints the supported MCP agent integrations. This exercises the documented Secure MCP command surface and proves the "gateway and operator are a single static Go binary" statement from `README.md`.

### 2. Build and start the gateway

Every smoke-test run starts from clean Docker volumes and an empty local CLI identity. The reset deletes the existing gateway PKI and sessions, the operator vault and audit ledger, and all ensemble and dashboard state. Export any evidence that must be retained before proceeding, then run:

```bash
docker ps -a --filter 'name=^/g8e-' --format '{{.Names}}\t{{.Status}}'
./g8e docker clean
./g8e auth logout
docker ps -a --filter 'name=^/g8e-' --format '{{.Names}}\t{{.Status}}'
```

The first container listing records any state left by an earlier run. The final listing must produce no output. `docker clean` removes the Docker containers, volumes, and network but does not remove host-side CLI credentials; `auth logout` removes the local credentials so enrollment cannot accidentally reuse a certificate or session from the deleted gateway PKI.

If the Docker image is not already built, build it first. This step is only needed once.

```bash
./g8e docker build
```

Start the default compose profile, which brings up only the gateway:

```bash
./g8e docker start
```

Wait for the gateway container to be healthy, then run the FIPS 140-3 self-check against the binary baked into the image:

```bash
./g8e docker status
docker exec g8e-gateway /g8e version --fips
```

Expected: the `g8e-gateway` row shows `Up ... (healthy)` with ports `0.0.0.0:8080->8080/tcp, 0.0.0.0:8443->8443/tcp`. The FIPS self-check reports `FIPS 140-3 mode: enabled`, prints the validated module version, and exits zero. Run this assertion against the container binary because the host binary may not have been built with `GOFIPS140=v1.0.0`.

Do not use `./g8e gw status` here — see "Docker command classification" above for why it reports `STOPPED` in Docker mode.

### 3. Headless owner enrollment

```bash
./g8e auth enroll user --headless -e localhost
```

Expected: no browser opens. The command prints `User ID: <uuid>` and `CLI Session ID: <uuid>`. Copy both values; they are required for the scenario harness later. This is the headless enrollment path documented in `docs/guides/docker_gateway.md` and `docs/guides/unified_stack.md`.

### 4. Start the bootstrapped workloads

With the owner identity established, bring up the operator, ensemble, and dashboard without the interactive walkthrough:

```bash
./g8e docker start --profile bootstrapped --skip-enroll
```

Inspect the stack:

```bash
./g8e docker status
```

Expected: all four platform containers (`g8e-gateway`, `g8e-operator`, `g8e-ensemble`, `g8e-dashboard`) are present. They may still be starting while their platform enrollment requests are pending.

### 5. Approve platform workload enrollments

List pending requests:

```bash
./g8e auth pending-platform-enrollments
```

Approve each request by its exact request ID. The order does not matter, but ensemble, dashboard, operator is the conventional order used by the interactive `docker start --full` walkthrough:

```bash
./g8e auth approve-platform-enrollment <operator-request-id> --yes
./g8e auth approve-platform-enrollment <ensemble-request-id> --yes
./g8e auth approve-platform-enrollment <dashboard-request-id> --yes
```

This is the owner-approved platform enrollment protocol described in `docs/architecture/ensemble.md`. After approval the operator connects over its outbound-only mTLS tunnel and the ensemble and dashboard receive their SPIFFE app certificates.

### 6. Verify the full stack

Workloads poll for enrollment approval with backoff, so approval can take up to a few minutes to propagate. Repeat `./g8e docker status` until all four services report healthy before running the remaining checks.

```bash
./g8e docker status
./g8e operator list
./g8e gw data operators
docker exec g8e-operator /g8e vault status
OPERATOR_PORTS="$(docker port g8e-operator)"
test -z "${OPERATOR_PORTS}"
curl -fsS http://localhost:8000/health
curl -fsS -o /dev/null http://localhost:3000/
```

Expected:

- `g8e docker status` shows all services healthy.
- `g8e operator list` shows an operator with a SPIFFE session ID.
- `g8e gw data operators` shows the operator instance.
- `docker exec g8e-operator /g8e vault status` shows the encryption vault is initialized.
- The `docker port` assertion exits zero only when the operator publishes no ports.
- The ensemble health endpoint returns a successful response.
- The dashboard root returns a successful response.

`vault status` is a filesystem command (see "Docker command classification" above). It reads the vault header and key from `/root/.g8e/vault/` inside the operator container. Running `./g8e vault status` on the host reads the host's `.g8e/vault/`, which is empty in Docker mode, and falsely reports "not initialized". The `docker exec` form runs the operator container's `/g8e` binary against the container's volume.

These commands together prove that the gateway (PDP) and operator (PEP) are connected over outbound-only mTLS, that the operator does not listen on public ports, and that keys and audit state remain inside the container.

### 7. Configure a real LLM endpoint

This step is required. The scenario in step 8 cannot produce a `FILE_EDIT` receipt without a configured LLM provider, and the CSV reports in step 10 will be header-only if step 8 never writes a receipt. Export the three `G8E_HARNESS_*` variables in the same shell that will run step 8; child processes inherit them, but a separate terminal session does not.

```bash
export G8E_HARNESS_LLM_PROVIDER=ollama
export G8E_HARNESS_LLM_MODEL="<tool-capable-model-tag>"
export G8E_HARNESS_LLM_ENDPOINT="http://<ollama-host>:<port>"
```

If no real LLM endpoint is reachable, use the deterministic fake provider instead. It still drives the full governance pipeline and writes a signed `FILE_EDIT` receipt, so the reports in step 10 will populate:

```bash
export G8E_HARNESS_LLM_PROVIDER=fake
```

Replace both placeholders with the Ollama endpoint and a model that supports tool calls. If the model is not already pulled on the Ollama host, pull it from any client that can reach the host:

```bash
OLLAMA_HOST="${G8E_HARNESS_LLM_ENDPOINT}" ollama pull "${G8E_HARNESS_LLM_MODEL}"
```

Verify the model is visible:

```bash
curl -fsS "${G8E_HARNESS_LLM_ENDPOINT}/api/tags" | grep -F "\"name\":\"${G8E_HARNESS_LLM_MODEL}\""
```

Confirm the variables are set in the current shell before proceeding:

```bash
env | grep G8E_HARNESS
```

Expected: all three variables (or at least `G8E_HARNESS_LLM_PROVIDER`) appear. If nothing prints, step 8 will fail to call the LLM and no receipt will be written.

### 8. Run governed file and document mutations

The first scenario is the only source of `FILE_EDIT` receipts in the headless run. It sends a real AI call through the ensemble and must succeed before steps 9 and 10:

```bash
./g8e demos scenarios run ensemble-chat-file-create \
  --mtls-url https://localhost:8443 \
  --public-url http://localhost:8080 \
  --ensemble-url http://localhost:8000 \
  --user-id <user-id-from-step-3> \
  --cli-session-id <cli-session-id-from-step-3>
```

This causes the g8ee ensemble to send the prompt to the real LLM. The model selects the `file_create` tool; the call is translated into a `GovernanceEnvelope`, routed through the gateway's L1 doctrine gate, executed by the operator's L5 actuator, and written as a signed `FILE_EDIT` receipt. The `file_create` tool requires L3 notary approval before execution; in headless mode the harness starts an auto-approver that subscribes to the gateway SSE stream and approves the file-edit approval request on behalf of the harness persona. The harness prints a summary table with `ok` status for the scenario, and the notes include the correlated receipt's shortened transaction hash.

Run the document-update scenario against the same live stack to prove the admission and actuator paths generalize beyond file mutations:

```bash
./g8e demos scenarios run ensemble-document-update \
  --mtls-url https://localhost:8443 \
  --public-url http://localhost:8080 \
  --ensemble-url http://localhost:8000 \
  --user-id <user-id-from-step-3> \
  --cli-session-id <cli-session-id-from-step-3>
```

This scenario creates a document with `merge=false`, applies a partial update with `merge=true`, and reads the document back to prove the updated field changed while untouched fields survived. Both mutations pass through governance as `DOCUMENT_UPDATE` actions and exercise the document-store actuator path.

Required outcome: both summary tables show `ok`, and their notes include correlated transaction hashes. If either table shows `fail` or `error`, or no transaction hash appears, stop and diagnose before continuing. The most common causes for the file scenario are: the `G8E_HARNESS_*` variables from step 7 are not set in this shell; the Ollama endpoint is unreachable or the model is not pulled; or the stack went down between step 6 and step 8. Re-run `g8e docker status` and `env | grep G8E_HARNESS` to confirm both are healthy.

To see the governance pipeline in detail, run with verbose output and watch the gateway logs in a second terminal:

```bash
./g8e demos scenarios run ensemble-chat-file-create --verbose \
  --mtls-url https://localhost:8443 \
  --public-url http://localhost:8080 \
  --ensemble-url http://localhost:8000 \
  --user-id <user-id> \
  --cli-session-id <cli-session-id>
```

```bash
./g8e docker logs -f gateway
```

### 9. Inspect the receipts and audit trail

After the scenario prints `ok`, capture the operator session ID from `g8e operator list` and query the vault:

```bash
./g8e audit receipts
./g8e audit events
./g8e audit summary
./g8e gw data audit list --operator-session-id <operator-session-id>
./g8e audit export --session <operator-session-id> --out ./receipts-export.json
grep -q '<file-transaction-hash-from-step-8>' ./receipts-export.json
grep -q '<document-transaction-hash-from-step-8>' ./receipts-export.json
```

Expected: the transaction hashes from both scenarios appear in the receipts and events tables with `EXECUTING` or `COMPLETED` status. At least one row has action type `FILE_EDIT`, and the document scenario produces `DOCUMENT_UPDATE` rows. The export command writes the full signed-receipt bundle, and both `grep` assertions exit zero, proving that the bundle contains the correlated scenario receipts and can be archived separately from the running platform. Together with the report checks in step 10, this verifies the three-store architecture: complete receipts in the SQLite audit store, signed attestations in the SQLite commitment chain, and governed file snapshots in the git-backed ledger.

`g8e audit receipts` queries the gateway API with a default limit of 50 receipts. In environments with frequent background heartbeats or long polling, the `FILE_EDIT` receipt can be pushed past the 50-row window before this step runs. If the default query does not show the receipt, scope the query to the operator session so the `FILE_EDIT` row is reliably retrieved:

```bash
./g8e audit receipts --session <operator-session-id>
```

Gate check before step 10: confirm both mutation types exist. If `g8e audit receipts` shows only `HEARTBEAT` and `PLATFORM_ENROLLMENT_*` rows, step 8 did not produce a mutation and step 10 will generate mostly empty CSVs. Do not proceed to step 10; return to step 7 and step 8.

```bash
./g8e audit receipts | grep FILE_EDIT
./g8e audit receipts | grep DOCUMENT_UPDATE
```

Expected: both commands print at least one matching line. If either grep returns nothing, its scenario failed or did not run. If a scenario reported `ok` but its receipt is absent, the receipt was likely pushed past the default 50-row window; re-run the receipt query with `--session <operator-session-id>` as shown above.

### 10. Generate and verify a CSV evidence report

`report all` is a filesystem command (see "Docker command classification" above). It reads the audit SQLite databases, vault, and git ledger directly from `.g8e/` on disk. When the stack runs in Docker, that state lives inside the operator container's volume. Run it inside the container:

```bash
docker exec g8e-operator /g8e report all
```

The CSV files are written to `reports/<timestamp>/` inside the container at `/root/reports/<timestamp>/`. To inspect them on the host, copy the directory out. Use the `/.` source suffix and a trailing slash on the destination so the contents land flat at `./reports/<timestamp>/` regardless of whether `./reports` already exists on the host; without the suffix, `docker cp` nests the tree under `./reports/reports/<timestamp>/` when the destination directory is present:

```bash
docker cp g8e-operator:/root/reports/. ./reports/
REPORT_DIR="$(ls -dt ./reports/*/ | head -n 1)"
cat "${REPORT_DIR}/verification_summary.csv"
awk -F, '$1 == "commitment_chain" && $4 == "PASS" { print; found=1 } END { exit !found }' "${REPORT_DIR}/verification_summary.csv"
awk -F, '$1 == "git_merkle_root" && $4 == "PASS" { print; found=1 } END { exit !found }' "${REPORT_DIR}/verification_summary.csv"
awk -F, '$1 == "file_mutation_linkage" && $4 == "PASS" && $5 !~ /^0 mutations checked/ { print; found=1 } END { exit !found }' "${REPORT_DIR}/verification_summary.csv"
awk -F, '$1 == "receipt_commitment_crosslink" && $4 == "PASS" { print; found=1 } END { exit !found }' "${REPORT_DIR}/verification_summary.csv"
test -z "$(awk -F, '$4 == "FAIL" { print }' "${REPORT_DIR}/verification_summary.csv")"
```

Expected: the command writes deterministic CSV files for the audit vault, receipts, commitment ledger, Git ledger, document stores, and secrets stores. Each `awk` command prints its matching `PASS` row, the file-mutation row reports a non-zero mutation count, and the final command exits zero only when no verification row reports `FAIL`. These checks capture a non-empty Git ledger root, validate mutation-to-ledger linkage, recompute the commitment chain, verify every Auditor Ed25519 signature, compare signed attestations with their structured columns, and cross-link every commitment to its receipt.

The `file_diffs.csv` and `file_mutations.csv` reports are populated by the `FILE_EDIT` receipt from step 8. The `events.csv`, `executions.csv`, and `replay_nonces.csv` reports contain evidence from operator-executed governance paths. `commitments.csv` contains signed commitments for operator-local L5 executions, including the file mutation and heartbeats, and `receipts.csv` contains the matching operator-local receipts with readable protobuf status names. The gateway audit queries in step 9 contain the `DOCUMENT_UPDATE` receipts because the unified gateway executes those document-store actions in process. If the file-specific reports contain only their header row or `commitments.csv` has no `FILE_EDIT` row, the file scenario did not complete through the operator.

### 11. Verify the binary distribution endpoint

```bash
curl -fSLO http://localhost:8080/.well-known/g8e/bin/g8e-linux-amd64
chmod +x ./g8e-linux-amd64
./g8e-linux-amd64 version
test "$(./g8e-linux-amd64 version)" = "$(docker exec g8e-gateway /g8e version)"
```

Expected: the gateway serves a runnable static `g8e` binary over the plain HTTP discovery port. The downloaded binary prints valid build metadata, and the comparison exits zero only when its version, build ID, build time, and platform match the binary currently running in the gateway container. This proves the "gateway hosts and serves these binaries via `/.well-known/g8e/bin/{filename}`" statement from `README.md` and `docs/guides/getting_started.md`. Clean up the downloaded binary afterwards.

### 12. Optional: evaluate KSIs

The compliance command reads the live audit, commitment, and Git ledger stores, so run it inside the operator container after the governed scenarios have populated those stores:

```bash
docker exec g8e-operator /g8e compliance ksi \
  --class C \
  --catalog /docs/reference/ksi-catalog.json > ./ksi-result.json
grep -q '"class": "C"' ./ksi-result.json
grep -q '"evidence":' ./ksi-result.json
```

Expected: the KSI command emits a Class C result set with evidence from the live post-run stores. Individual KSIs may report `not-satisfied` when the smoke environment lacks the history or configuration required by that indicator. The superseded flat OSCAL export is unavailable, and proof-backed bundle generation is not yet available.

### 13. Optional: governance posture sweep

The headless run above uses the default `doctrine` posture. To exercise `ratify` or `notary`, enroll with a passkey in a browser. To exercise `consensus` or `notary`, provide a `consensus-bootstrap.json` file. The optional commands are:

```bash
./g8e auth enroll user -e localhost
./g8e docker start --profile bootstrapped --skip-enroll
# approve operator, ensemble, and dashboard again
```

Then run a scenario under `consensus`, `ratify`, or `notary`. The `ratify` and `notary` scenarios require a human in the loop for L3 approval, while the `consensus` and `notary` scenarios require L2 consensus configuration. These optional scenarios are not part of the headless run.

### 14. Stop and clean up

To stop the stack while preserving state:

```bash
./g8e docker stop
```

To remove all state, including PKI, vault, and audit ledger:

```bash
./g8e docker clean
```

## README claim-to-verification mapping

| README claim | How this runbook proves it |
|---|---|
| "g8e is a sovereign execution platform that delivers frontier AI reasoning to the edge without surrendering data custody" | The ensemble scenario sends the prompt to a real LLM, but the actual `file_create` execution and receipt are produced by the operator inside the container and stored in the operator's local audit vault. |
| "The gateway and operator are a single static Go binary" | `./g8e version` and the `/.well-known/g8e/bin/` endpoint. |
| "FIPS mode is verifiable at runtime via `g8e version --fips`" | `docker exec g8e-gateway /g8e version --fips` verifies approved mode against the FIPS-built container binary. |
| Continuous compliance evidence | The optional `compliance ksi` command evaluates live stores and emits a typed KSI result set. |
| "Secure MCP" | `g8e mcp agent list` exercises the documented MCP agent-integration command surface; the ensemble file scenario drives a typed MCP `tools/call` through governance. |
| "docker compose up from the repo root brings up the whole stack" | `g8e docker start` followed by `g8e docker status` showing the gateway, operator, ensemble, and dashboard, plus direct HTTP probes of the ensemble and dashboard. |
| "The operator initiates a single outbound mTLS tunnel to the gateway. It listens on no ports" | `g8e operator list` shows the operator session; capturing `docker port g8e-operator` and asserting an empty result explicitly verifies that the container publishes no ports. |
| "Every mutation must clear a five-layer admission pipeline" | The `FILE_EDIT` and `DOCUMENT_UPDATE` receipts in `g8e audit receipts` and the gateway logs show admission and execution across file and document actuator paths. |
| Complete receipts use the SQLite audit store, signed commitments use the SQLite hash chain, and governed file snapshots use the git-backed ledger | `g8e audit receipts`, `g8e audit events`, and the `report all` receipt-signature, persistence-attestation, commitment-chain, structured-column, ledger-root, mutation-linkage, and receipt-cross-link checks. |
| "Raw data remains on the host" | The operator's `.g8e/vault/` and `.g8e/data/` directories are inside the operator container's volume; the gateway and ensemble receive only hashes, signatures, and tokenized projections. |
| "Vault keys are owned by the data owner and never shared with the gateway or cloud provider" | `docker exec g8e-operator /g8e vault status` shows the vault is initialized; keys are never transmitted in the scenario. |
| "Four posture configurations: doctrine, consensus, ratify, notary" | The default `doctrine` run and the optional `consensus`, `ratify`, and `notary` posture sweep, with each posture's L2 and L3 prerequisites documented. |

## Troubleshooting

- `g8e docker status` shows the gateway as `Up ... (starting)` or the container is absent: the gateway is still starting or failed to launch. Wait a few seconds and retry. If the container is missing, check `docker logs g8e-gateway` for startup errors. Do not use `g8e gw status` to diagnose a Docker gateway — see "Docker command classification" above.
- `auth enroll` fails: stale `.g8e/` state from a previous run may conflict. Stop the stack with `g8e docker stop` and clean with `g8e docker clean` or `g8e gw clean` before restarting.
- A network command returns `404 page not found` for a route that exists in the local binary: the Docker image is stale. The Docker images bake the `g8e` binary in at build time. Run `./g8e docker build` to rebuild the images with the current binary, then `./g8e docker stop && ./g8e docker start` to restart. Verify with `docker exec g8e-gateway /g8e version` that the container binary matches the local `./g8e version`.
- `demos scenarios run` fails with a model or endpoint error: the Ollama model is not pulled or the endpoint is unreachable. Switch to `G8E_HARNESS_LLM_PROVIDER=fake` for a smoke run, or pull the model with `ollama pull`.
- `demos scenarios run` fails with no LLM configured: the `G8E_HARNESS_*` variables were not exported in the shell running the scenario. Re-export them (or set `G8E_HARNESS_LLM_PROVIDER=fake`) in the same shell, confirm with `env | grep G8E_HARNESS`, and re-run the scenario.
- `docker exec g8e-operator /g8e report all` produces no `FILE_EDIT` rows in `receipts.csv`, `commitments.csv`, `file_mutations.csv`, or `file_diffs.csv`: step 8 did not complete the governed file mutation. `verification_summary.csv` reports "0 mutations checked", though heartbeat receipts and commitments can still make the other reports non-empty. Return to step 7, confirm the `G8E_HARNESS_*` variables are set, re-run step 8 until `g8e audit receipts | grep FILE_EDIT` returns a match, then re-run the report command inside the container. Do not run `./g8e report all` on the host; it reads the host's empty `.g8e/` directory and always produces header-only CSVs in Docker mode.
- `pending-platform-enrollments` is empty but workloads are not healthy: the operator or ensemble may still be starting. Wait and run `g8e auth pending-platform-enrollments` again, then check `g8e docker logs -f operator`.
- The downloaded `g8e-linux-amd64` cannot execute on the current architecture: use the filename matching the workstation's OS and architecture (`g8e-darwin-arm64`, `g8e-windows-amd64.exe`, etc.).

## See also

- `docs/guides/getting_started.md`
- `docs/guides/unified_stack.md`
- `docs/guides/docker_gateway.md`
- `docs/architecture/ensemble.md`
- `demos/README.md`
