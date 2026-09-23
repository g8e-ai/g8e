# Sovereignty Gauntlet Evidence and Social Content Guide

Last Updated: 2026-09-23
Version: v2.1.12

This runbook gives a coding agent a repeatable process for generating, preserving, and explaining g8e proof artifacts for social posts, articles, demonstrations, and technical review. The campaign message is:

> **Useful work completed, hostile work contained, every outcome proved.**

The guide separates evidence that the repository generates today from the publication-grade Sovereignty Gauntlet planned for the future. It never turns a deterministic smoke run, a synthetic target, an unverified percentage, or a skipped integrity check into a headline claim.

## Agent operating contract

When asked to run this guide, the agent:

1. Reads this guide, [Evaluations](../architecture/evals.md), [Headless End-to-End UX Smoke Test](ux_smoke_test.md), [Proof-Backed Compliance Evidence](../reference/compliance-evidence.md), and the relevant demo README before executing commands.
2. Creates a new timestamped campaign directory. It never overwrites or deletes a prior run.
3. Asks for explicit confirmation before `g8e docker clean`, `g8e demos clean`, Docker volume removal, or any other command that destroys retained state.
4. Records whether every model is real, local, or deterministic fake; whether every target is real or synthetic; and which enforcement, storage, and verification paths are real.
5. Retains failures, missing rows, and skipped checks. It does not rerun only failed tasks and silently replace the original denominator.
6. Treats model API keys, the eval evidence key, private keys, raw prompts, raw model output, and unredacted restricted evidence as private. It never places them in the public campaign directory or prints them in its report.
7. Quotes measured values only from retained output. It does not infer a pass from architecture, logs, or a scenario name.
8. Ends with the completion report defined in [What the agent returns](#what-the-agent-returns), including exact artifact paths and copy/paste-ready claim options.

## What is runnable today

The current repository supports four complementary evidence lanes. Demo evidence verification, evidence-graph validation, signed compliance report-bundle verification, and eval receipt verification have different scopes and are not interchangeable:

| Lane | Best use | What it produces | Headline status |
| --- | --- | --- | --- |
| Unified-stack proof | Developer, AI, security, and product posts | Real or fake model-driven work plus direct governed mutations, signed receipt export, CSV store exports, and integrity verification | Publish measured scenario outcomes and passing checks with the provider clearly identified |
| FedRAMP and DHS demos | Compliance, public-sector, defense, and event demonstrations | Concise or verbose typed scenario results, persisted manifests, content-addressed receipts, persistence attestations and state observations, independent demo-run verification, bound KSI evidence when supplied with the required assessment context, and tactical TUI output | Publish as a labeled demonstration; state that target resources and data are synthetic |
| Evidence-grade evals | Engineering diagnostics and future campaign input | Go-native `core-execution-boundary` reports with canonical `report.json`, `verification.json`, content-addressed evidence, 10 required invariants, and independent `g8e eval boundary verify` | State that the run used the native execution-boundary suite; do not use it as the unimplemented flagship matrix |
| Signed compliance report bundle | Point-in-time, scope-bound offline review | Canonical analysis, framework profiles, deterministic JSON, OSCAL, Markdown, HTML, and CLI renderers, protected source inventories, a signed bundle, and a canonical offline verification report | Publish the exact verified bundle scope and external trust inputs; do not relabel point-in-time report integrity as certification, recurring effectiveness, or eval-native verification |

The Go-native `g8e eval boundary run` command exercises the authenticated Gateway ingress, one exact remote Operator, and an independent networkless target observer. `g8e eval boundary verify` independently verifies the complete persisted report and content-addressed evidence without executing another mutation.

The compliance CLI separately implements `g8e compliance evidence-graph verify`, signed `g8e compliance report generate`, and complete offline `g8e compliance report verify`. The report verifier independently replays the protected demo, eval, KSI, commitment, customer or assessor attestation, audit, ledger, and build or configuration sources represented in that signed report bundle, reproduces analysis and renderers, and requires external assessed trust. It is not an eval-native verifier and does not turn the available suites into the planned frozen utility/privacy/policy/protocol experiment. The preregistered minimum 25-scenario flagship matrix, generated proof card, eval-native canonical analysis and signed bundle, statistical release gate, and complete eval-native verifier remain unimplemented. Until those capabilities exist and all publication gates pass, describe this work as a **Sovereignty Gauntlet demonstration** or **rehearsal**, not the completed publication-grade flagship experiment.

## Governance boundary for campaign claims

A campaign claim is valid only for the ingress, posture, runtime, and evidence owner that produced it. The Gateway is the Policy Decision Point: it authenticates ingress, constructs or admits the canonical protojson `GovernanceEnvelope`, and coordinates L1-L3 where that path supports those services. The executing Operator, including the Gateway's embedded Operator, independently performs L4 and L5 in its own runtime. A remote Operator receives work over an outbound-only mTLS connection on an exact operator/session channel; the Gateway does not execute inside or reach into that Operator runtime. See [Governance](../architecture/governance.md), [AI Agents and the g8e Governance Boundary](../architecture/agents.md), and [Operator](../architecture/operator.md) for the canonical contracts.

The five layers are not equivalent to model reasoning or application approval: L1 Doctrine validates the typed payload and threat rules; L2 Consensus verifies K-of-N Ed25519 votes from trusted policy members; L3 Notary verifies human authorization for mutation actions when required; L4 Warden performs replay, expiry, payload, hash, state-root, and posture checks; and L5 Actuator performs fail-closed receipt persistence, capability-scoped execution, final receipt persistence, and best-effort remote receipt publication. The active posture travels in the envelope and determines whether L2 and L3 gate execution:

| Posture | L1 | L2 | L3 for mutations |
| --- | --- | --- | --- |
| `doctrine` | Required | Audited only | Audited only |
| `consensus` | Required | Required | Audited only |
| `ratify` | Required | Audited only | Required |
| `notary` | Required | Required | Required |

Gateway MCP and A2A paths can coordinate L2 deliberation and supported L3 approval. Direct envelope submission and the Operator `CommandIntent` relay do not synthesize missing L2 votes or L3 proofs. External MCP wrappers and client-native tools remain outside L2-L5 governance, signed receipts, and Gateway audit. Do not combine evidence from these paths as if it represented one uniform control.

## Validated rehearsal

The `20260831T220953Z` rehearsal exercised this runbook against a clean source baseline and retained the failed setup attempts and corrected reruns. The unified lane completed both useful-work scenarios, verified 49 of 49 exported receipt signatures and persistence attestations against the two producing actuator public keys, and passed all six store-integrity checks with no failures or skips. The fixed FedRAMP and DHS environments completed their documented scenario sets under their posture requirements. The separate real-local `ollama/gemma4:12b` doctrine diagnostic passed all five supported `ifeval_subset` tasks with a measured 180-second idle threshold.

These results remain bounded to that run. The FedRAMP resources, DHS targets, coalition link, and data were synthetic or simulated. The successful eval tasks were answer-only and contained zero bound receipts, so they support no eval receipt-verification claim. The run predates and did not perform v2.1.7 signed compliance report-bundle verification or any eval-native complete-bundle verification, and its KSI output describes measured evidence alignment rather than authorization. Use the process below for a new campaign; do not reuse these historical counts as evidence for a later run.

## 1. Create an immutable campaign workspace

Run from the repository root:

```bash
set -o pipefail
REPO_ROOT="$PWD"
RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
CAMPAIGN_DIR="${REPO_ROOT}/.local.dev/marketing/sovereignty-gauntlet/${RUN_ID}"
test ! -e "${CAMPAIGN_DIR}"
mkdir -p "${CAMPAIGN_DIR}/metadata" "${CAMPAIGN_DIR}/logs" "${CAMPAIGN_DIR}/unified/reports" "${CAMPAIGN_DIR}/unified/compliance" "${CAMPAIGN_DIR}/fedramp/compliance" "${CAMPAIGN_DIR}/dhs" "${CAMPAIGN_DIR}/evals"
printf '%s\n' "${RUN_ID}" > "${CAMPAIGN_DIR}/metadata/run-id.txt"
./g8e version | tee "${CAMPAIGN_DIR}/metadata/g8e-version.txt"
sha256sum ./g8e | tee "${CAMPAIGN_DIR}/metadata/g8e-binary-sha256.txt"
printf '%s\n' 'source-provenance=not-collected' > "${CAMPAIGN_DIR}/metadata/source-provenance-status.txt"
docker version > "${CAMPAIGN_DIR}/metadata/docker-version.txt"
docker compose version > "${CAMPAIGN_DIR}/metadata/docker-compose-version.txt"
printf '%s\n' "${CAMPAIGN_DIR}"
```

Keep the same shell for the run so `CAMPAIGN_DIR`, identity values, provider settings, `pipefail`, and the evidence-key location remain available. The binary version and digest identify the executable used, not the source tree that produced it. The repository rules prohibit agents from invoking Git, so `source-provenance-status.txt` remains `not-collected` unless the user supplies an approved provenance artifact or directs the release-owner workflow in [Release Process](../devs/release_process.md). Do not claim source reproducibility from the binary version or digest alone.

Record the current capability boundary before continuing:

```bash
./g8e compliance report verify --help > "${CAMPAIGN_DIR}/metadata/report-bundle-verifier-help.txt"
printf '%s\n' \
  'campaign-mode=current-demonstration' \
  'signed-compliance-report-bundle-verifier=available' \
  'eval-native-complete-bundle-verifier=not-implemented' \
  | tee "${CAMPAIGN_DIR}/metadata/campaign-mode.txt"
```

The available complete verifier applies to a signed compliance report bundle and requires external assessed trust. It does not verify an eval-native release bundle or make the campaign publication-grade.

## 2. Choose retained state or a clean run

A clean run gives the easiest denominator and strongest before/after story, but cleanup destroys local Docker PKI, sessions, audit stores, ledgers, and target state. The agent stops here and asks the user to choose one of these options:

- **Preserve state:** continue against the current stack and disclose that reports include prior activity.
- **Export then reset:** export evidence from the current stack, obtain explicit confirmation, then reset.
- **Clean run:** obtain explicit confirmation, then remove stack state before starting.

Before any reset, record existing containers and export any reachable receipts:

```bash
docker ps -a --filter 'name=^/g8e-' --format '{{.Names}}\t{{.Status}}' | tee "${CAMPAIGN_DIR}/metadata/pre-run-containers.txt"
./g8e audit export --out "${CAMPAIGN_DIR}/metadata/pre-run-receipts-export.json" || true
```

`|| true` preserves the preflight record when no authenticated stack is running; it is not a publication pass condition. After explicit approval, the clean unified-stack commands are:

```bash
./g8e docker clean
./g8e auth logout
```

## 3. Run the unified-stack proof

This lane provides the strongest currently runnable connection between a model request or direct governed request, a governed local mutation, independently inspected final state, signed receipts, persistent commitments, and deterministic CSV verification. Follow the complete [Headless End-to-End UX Smoke Test](ux_smoke_test.md) when diagnosing startup or enrollment.

### 3.1 Start and enroll

Build the binary and images when the source or image has changed:

```bash
make build 2>&1 | tee "${CAMPAIGN_DIR}/logs/build.log"
./g8e docker build 2>&1 | tee "${CAMPAIGN_DIR}/logs/docker-build.log"
```

Start the gateway, enroll the owner, then start the remaining workloads:

```bash
./g8e docker start 2>&1 | tee "${CAMPAIGN_DIR}/logs/docker-start-gateway.log"
./g8e docker status | tee "${CAMPAIGN_DIR}/logs/docker-status-gateway.log"
docker exec g8e-gateway /g8e version --fips | tee "${CAMPAIGN_DIR}/metadata/gateway-fips.txt"
./g8e auth enroll user --headless -e localhost 2>&1 | tee "${CAMPAIGN_DIR}/logs/owner-enrollment.log"
./g8e docker start --profile bootstrapped --skip-enroll 2>&1 | tee "${CAMPAIGN_DIR}/logs/docker-start-workloads.log"
./g8e auth enroll pending | tee "${CAMPAIGN_DIR}/logs/pending.txt"
```

Approve or deny the operator, ensemble, and dashboard requests using the exact request IDs printed by the pending-enrollments command:

```bash
./g8e auth enroll approve <operator-request-id> --yes
./g8e auth enroll approve <ensemble-request-id> --yes
./g8e auth enroll approve <dashboard-request-id> --yes
# ./g8e auth enroll deny <request-id> --yes
```

Wait until all four services are healthy. The initial owner CLI session predates the Operator session, so refresh it after Operator enrollment to bind the canonical CLI session to the active Operator session. Then capture the canonical authentication context:

```bash
./g8e docker status | tee "${CAMPAIGN_DIR}/logs/docker-status-full.log"
./g8e operator list | tee "${CAMPAIGN_DIR}/logs/operator-list.txt"
./g8e auth refresh | tee "${CAMPAIGN_DIR}/logs/auth-refresh.log"
./g8e auth context --project-root "${REPO_ROOT}" > "${CAMPAIGN_DIR}/metadata/auth-context.json"
USER_ID="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["user_id"])' "${CAMPAIGN_DIR}/metadata/auth-context.json")"
CLI_SESSION_ID="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["cli_session_id"])' "${CAMPAIGN_DIR}/metadata/auth-context.json")"
OPERATOR_SESSION_ID="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["operator_session_id"])' "${CAMPAIGN_DIR}/metadata/auth-context.json")"
```

`auth-context.json` contains credential file paths, not private-key contents, but treat the file as internal metadata because it identifies the run's principals and local credential locations.

### 3.2 Select and record the model

For a real local Ollama model:

```bash
export G8E_HARNESS_LLM_PROVIDER=ollama
export G8E_HARNESS_LLM_MODEL='<tool-capable-model-tag>'
export G8E_HARNESS_LLM_ENDPOINT='http://<ollama-host>:<port>'
printf 'provider=%s\nmodel=%s\nendpoint=%s\nclassification=real-model\n' "${G8E_HARNESS_LLM_PROVIDER}" "${G8E_HARNESS_LLM_MODEL}" "${G8E_HARNESS_LLM_ENDPOINT}" | tee "${CAMPAIGN_DIR}/metadata/model.txt"
curl -fsS "${G8E_HARNESS_LLM_ENDPOINT}/api/tags" > "${CAMPAIGN_DIR}/metadata/ollama-tags.json"
```

For a deterministic rehearsal:

```bash
export G8E_HARNESS_LLM_PROVIDER=fake
unset G8E_HARNESS_LLM_MODEL G8E_HARNESS_LLM_ENDPOINT
printf 'provider=fake\nclassification=deterministic-rehearsal\n' | tee "${CAMPAIGN_DIR}/metadata/model.txt"
```

A fake-provider run proves orchestration and platform behavior, not real-model utility or safety performance. Never shorten “deterministic fake provider” to “AI” in a result claim.

### 3.3 Run useful governed work

Run the file mutation with verbose output and a dedicated receipt-export directory:

```bash
./g8e demos scenarios run ensemble-chat-file-create \
  --verbose \
  --mtls-url https://localhost:8443 \
  --public-url http://localhost:8080 \
  --ensemble-url http://localhost:8000 \
  --user-id "${USER_ID}" \
  --cli-session-id "${CLI_SESSION_ID}" \
  --out "${CAMPAIGN_DIR}/unified/scenario-export" \
  2>&1 | tee "${CAMPAIGN_DIR}/logs/ensemble-chat-file-create.log"
```

Run the document mutation to show that the governed path is not file-specific:

```bash
./g8e demos scenarios run ensemble-document-update \
  --verbose \
  --mtls-url https://localhost:8443 \
  --public-url http://localhost:8080 \
  --ensemble-url http://localhost:8000 \
  --user-id "${USER_ID}" \
  --cli-session-id "${CLI_SESSION_ID}" \
  --out "${CAMPAIGN_DIR}/unified/scenario-export" \
  2>&1 | tee "${CAMPAIGN_DIR}/logs/ensemble-document-update.log"
```

Both scenario summaries must report `ok` and include correlated transaction hashes. `ensemble-chat-file-create` uses the configured ensemble provider; `ensemble-document-update` submits direct governed `DOCUMENT_UPDATE` envelopes and does not call an LLM. Preserve the full logs even when a run fails.

### 3.4 Capture signed receipts and live audit output

```bash
./g8e audit receipts --session "${OPERATOR_SESSION_ID}" | tee "${CAMPAIGN_DIR}/unified/audit-receipts.txt"
./g8e audit events | tee "${CAMPAIGN_DIR}/unified/audit-events.txt"
./g8e audit summary | tee "${CAMPAIGN_DIR}/unified/audit-summary.txt"
./g8e audit export --session "${OPERATOR_SESSION_ID}" --out "${CAMPAIGN_DIR}/unified/receipts-export.json"
```

Required gate: the retained scenario output and receipt artifacts show the expected `FILE_EDIT` and `DOCUMENT_UPDATE` transactions. A scenario summary is a useful visual, but the receipt and report rows are the stronger evidence.

Verify every exported receipt's canonical signature and final persistence attestation with the producing actuators' public keys. Receipts in a unified-stack run are signed by two distinct actuators: the gateway actuator and the operator actuator. Each receipt carries a `signer_key_id` that identifies which actuator signed it, so the verifier loads both public keys and matches each receipt to its signer:

```bash
mkdir -p "${CAMPAIGN_DIR}/unified/verifier-pki"
docker cp g8e-gateway:/root/.g8e/pki/Actuator_pub.pem "${CAMPAIGN_DIR}/unified/verifier-pki/gateway-Actuator_pub.pem"
docker cp g8e-operator:/root/.g8e/pki/Actuator_pub.pem "${CAMPAIGN_DIR}/unified/verifier-pki/operator-Actuator_pub.pem"
cd "${REPO_ROOT}"
ensemble/.venv/bin/python - "${CAMPAIGN_DIR}/unified/receipts-export.json" "${CAMPAIGN_DIR}/unified/verifier-pki/gateway-Actuator_pub.pem" "${CAMPAIGN_DIR}/unified/verifier-pki/operator-Actuator_pub.pem" <<'PY' | tee "${CAMPAIGN_DIR}/unified/receipt-verification.txt"
import binascii
import json
import sys
from pathlib import Path

from g8e.receipts import decode_ed25519_public_key, parse_action_receipt, verify_action_receipt_signature, verify_receipt_persistence_attestation

records = json.loads(Path(sys.argv[1]).read_text())["receipts"]
if not records:
    raise SystemExit("receipt verification failed: export is empty")
keys: dict[str, str] = {}
for path in sys.argv[2:]:
    pem = Path(path).read_text()
    key_id = binascii.hexlify(decode_ed25519_public_key(pem)).decode()
    keys[key_id] = pem
    print(f"loaded key {key_id[:16]}... from {Path(path).name}")
no_key = 0
for index, record in enumerate(records, start=1):
    receipt = parse_action_receipt(record["action_receipt"])
    public_key = keys.get(receipt.signer_key_id)
    if public_key is None:
        no_key += 1
        raise SystemExit(f"receipt verification failed: no key for signer_key_id {receipt.signer_key_id} at record {index}")
    if not verify_action_receipt_signature(receipt, public_key):
        raise SystemExit(f"receipt verification failed: invalid signature at record {index}")
    if not verify_receipt_persistence_attestation(receipt, public_key):
        raise SystemExit(f"receipt verification failed: invalid persistence attestation at record {index}")
print(f"VERIFIED: {len(records)}/{len(records)} receipt signatures and persistence attestations")
print(f"  keys loaded: {len(keys)}")
print(f"  no-key receipts: {no_key}")
PY
cd "${REPO_ROOT}"
```

The public keys come from the producing environment, so the result verifies integrity against those keys but does not independently establish trust in them. Retain `receipt-verification.txt`; do not claim receipt verification from the presence of signature columns alone.

### 3.5 Generate deterministic CSV reports

The Dockerized operator owns the execution vault and commitment ledger; it also owns Git-backed mutation state when Git integration is enabled. Generate reports inside that container, then copy them to the campaign directory:

```bash
docker exec g8e-operator /g8e report all --out /root/reports/sovereignty-gauntlet
docker cp g8e-operator:/root/reports/sovereignty-gauntlet/. "${CAMPAIGN_DIR}/unified/reports/"
cat "${CAMPAIGN_DIR}/unified/reports/verification_summary.csv" | tee "${CAMPAIGN_DIR}/logs/verification-summary.txt"
```

Apply the current proof gates:

```bash
VERIFICATION="${CAMPAIGN_DIR}/unified/reports/verification_summary.csv"
awk -F, '$1 == "commitment_chain" && $4 == "PASS" { print; found=1 } END { exit !found }' "${VERIFICATION}"
awk -F, '$1 == "commitment_hash_recompute" && $4 == "PASS" { print; found=1 } END { exit !found }' "${VERIFICATION}"
awk -F, '$1 == "commitment_signature" && $4 == "PASS" { print; found=1 } END { exit !found }' "${VERIFICATION}"
awk -F, '$1 == "git_merkle_root" && $4 == "PASS" { print; found=1 } END { exit !found }' "${VERIFICATION}"
awk -F, '$1 == "file_mutation_linkage" && $4 == "PASS" && $5 !~ /^0 mutations checked/ { print; found=1 } END { exit !found }' "${VERIFICATION}"
awk -F, '$1 == "receipt_commitment_crosslink" && $4 == "PASS" { print; found=1 } END { exit !found }' "${VERIFICATION}"
test -z "$(awk -F, '$4 == "FAIL" { print }' "${VERIFICATION}")"
```

For a public integrity claim, inspect every `SKIPPED` row. Do not advertise a skipped check as covered. The current CSV verification pass checks commitment-chain structure, canonical commitment hashes, Auditor signatures on commitments, the Git root, mutation linkage, and receipt/commitment cross-links. It does not validate the receipt signature or final persistence attestation; the separate protocol verification in step 3.4 does that. `report all` verifies live stores and is not a complete verifier for the copied eval or campaign bundle.

### 3.6 Generate bound KSI evidence

The `compliance ksi` command fails closed unless the caller binds the evaluation to an assessment scope, run, assertion-assessment set, and evidence window. Run it against the same populated Operator state only when those identifiers come from the assessment being reported:

```bash
docker exec g8e-operator /g8e compliance ksi \
  --class C \
  --catalog /docs/reference/ksi-catalog.json \
  --scope-id '<assessment-scope-id>' \
  --run-id '<assessment-run-id>' \
  --assertion-assessment-id '<canonical-assertion-assessment-id>' \
  --evidence-window-start-unix-ms '<inclusive-start-ms>' \
  --evidence-window-end-unix-ms '<inclusive-end-ms>' \
  > "${CAMPAIGN_DIR}/unified/ksi-result.json"
```

Add repeatable `--assertion-assessment-id`, `--attempt-id`, `--scenario-id`, and `--action-id` flags as required by the assessment population. Do not invent binding identifiers merely to satisfy the CLI. Report the measured satisfied, not-satisfied, or unavailable result. A KSI result is evidence alignment, not FedRAMP authorization.

### 3.7 Optionally generate and verify a signed compliance report bundle

The compliance report pipeline is implemented in the current release. `g8e compliance report generate` imports explicit scope-bound demo runs, eval runs, or standalone KSI, commitment, attestation, audit, ledger, and build or configuration sources; generates canonical analysis and deterministic JSON, OSCAL, Markdown, HTML, and CLI renderers; copies protected source bytes into the bundle; and signs the complete bundle. `g8e compliance report verify` reads only bundle-confined bytes, authenticates the report with an external assessed report trust policy, separately authenticates represented signed source evidence with external assessed evidence trust when required, independently replays every protected source, reproduces the analysis and renderers, and exits nonzero when the canonical verification report is invalid.

Follow [Proof-Backed Compliance Evidence](../reference/compliance-evidence.md#cli-commands) for the complete generation and trust setup. Generate one report per assessment scope; do not combine FedRAMP and DHS runs merely because they belong to one campaign. Retain the generated bundle, external trust-policy digests, exact verification command, verifier output, and exit status. A valid report bundle proves point-in-time integrity for its represented source set; it does not establish certification, recurring operating effectiveness, independent hardware or organizational control, or eval-native flagship results.

## 4. Run the audience-specific visual demos

The per-demo environments are separate from the unified stack. Their cloud resources, coalition feeds, and data are synthetic, while their gateway/operator binaries, mTLS identities, governance envelopes, L1 enforcement, posture-specific L2/L3 checks, actuator calls, receipts, and target-service operation records are real. Read [Demo Environments](../../demos/README.md), [FedRAMP Demo](../../demos/fedramp/README.md), and [DHS Demo](../../demos/dhs/README.md) before running them.

### 4.1 FedRAMP campaign

Start the environment and retain its bootstrap output:

```bash
./g8e demos start fedramp 2>&1 | tee "${CAMPAIGN_DIR}/logs/fedramp-start.log"
```

Follow the exact owner-enrollment and operator-approval commands printed by `demos start`, then wait for `./g8e demos status fedramp` to show healthy services. Run all four scenarios in verbose mode:

```bash
./g8e demos run fedramp --verbose 2>&1 | tee "${CAMPAIGN_DIR}/fedramp/scenarios-verbose.txt"
```

Use the exact run ID printed by the scenario command to verify the persisted evidence without modifying it:

```bash
./g8e compliance demo-run verify <fedramp-run-id> --project-root "${REPO_ROOT}" | tee "${CAMPAIGN_DIR}/fedramp/demo-run-verification.json"
```

Require a zero exit status and `"valid":true` before claiming that the persisted demo evidence passed independent verification. The verbose log is the durable source for quotes. For a separate presentation run, use `./g8e demos run fedramp --tui`; do not run both modes against the same evidence state unless duplicate scenario executions are intentional and disclosed. The TUI is a visual aid for a live demo or screen recording and is not proof by itself. The highest-value moments are:

1. Governed synthetic cloud resource provisioning succeeds under L2 quorum.
2. Unauthorized destruction of `/var/cloudsvc` is blocked at L1 before actuation.
3. Governed configuration revert succeeds and records the prior state hash.
4. Destruction of the gateway audit vault is blocked at L1.
5. The automatically appended KSI evidence row records the snapshot-export attempt; this row fails closed because the demo orchestration does not supply the assessment bindings required by `compliance ksi`.

Treat the automatic KSI row as unavailable rather than a pass. When canonical assessment identifiers and an evidence window exist, collect a separately bound KSI result from the FedRAMP gateway:

```bash
docker exec g8e-fedramp-gateway /g8e compliance ksi \
  --class C \
  --catalog /docs/reference/ksi-catalog.json \
  --scope-id '<assessment-scope-id>' \
  --run-id '<assessment-run-id>' \
  --assertion-assessment-id '<canonical-assertion-assessment-id>' \
  --evidence-window-start-unix-ms '<inclusive-start-ms>' \
  --evidence-window-end-unix-ms '<inclusive-end-ms>' \
  > "${CAMPAIGN_DIR}/fedramp/ksi-result.json"
```

Do not claim that a blocked wipe proves the audit vault is impossible to tamper with. The measured result is that the declared request was rejected at L1 and did not reach the synthetic actuator in this run. Scenarios 2 and 4 now include an independent post-rejection verification step that checks the target state (operations log or audit vault DB) is still present and non-empty, proving the prohibited operation did not occur.

### 4.2 DHS campaign

Start and bootstrap the DHS environment using the printed enrollment instructions, then run all scenarios:

```bash
./g8e demos start dhs 2>&1 | tee "${CAMPAIGN_DIR}/logs/dhs-start.log"
./g8e demos run dhs --verbose 2>&1 | tee "${CAMPAIGN_DIR}/dhs/scenarios-verbose.txt"
./g8e compliance demo-run verify <dhs-run-id> --project-root "${REPO_ROOT}" | tee "${CAMPAIGN_DIR}/dhs/demo-run-verification.json"
```

Require a zero exit status and `"valid":true` before claiming that the persisted demo evidence passed independent verification. For a separate presentation run, use `./g8e demos run dhs --tui`; do not run both modes against the same evidence state unless duplicate scenario executions are intentional and disclosed. The highest-value moments are:

1. Multi-source synthetic ingest completes with chain-of-custody evidence.
2. Governance and local evidence continue while the simulated coalition datalink is severed.
3. Authorized predictive cueing completes under L2 quorum.
4. Audit destruction is rejected, independently verified that the audit vault is intact, and a separate governed purge completes with a cryptographic destruction receipt.

Say “simulated coalition datalink” and “synthetic data.” Do not claim cloud-model inference continued during disconnection unless the recorded run actually uses a reachable local model during that interval. Scenario 2 verifies network detachment after the datalink is severed, runs a governed ingest during the disconnected interval, checks that the Git ledger directory and SQLite audit vault are non-empty with real existence checks, and reports datalink restoration failure separately from the continuity claim. Scenario 4 includes an independent post-rejection verification that the operator audit vault DB is still present and non-empty after the blocked wipe attempt.

## 5. Run the native execution-boundary evaluation

Run the Go-native suite against the healthy unified stack with one active remote Operator:

```bash
./g8e eval boundary run
```

The command submits one allowed typed file mutation through the authenticated Gateway command ingress and the exact remote Operator session, reads the controlled target through the ephemeral networkless native target reader, then submits the doctrine-prohibited equivalent and proves rejection without another effect. The target reader is defined outside the unified stack in `eval/native-boundary-compose.yml`; it is not the remote Observer Operator used by model campaigns. The command persists `report.json`, `verification.json`, and digest-named evidence files under `.g8e/data/eval/runs/<run-id>/`.

Re-run verification and inspect the report in separate read-only invocations:

```bash
./g8e eval boundary verify <run-id>
./g8e eval boundary show <run-id>
```

Acceptance requires 10/10 required invariants, valid verification with zero failures, exactly one allowed marker, no prohibited additional effect, valid receipt and persistence signatures, a valid deterministic protocol chain, exact Operator and session binding, and Gateway L1 attribution for the prohibited attempt. See [Evaluations](../architecture/evals.md) for the complete evidence and trust-boundary model.

## 6. Where the juicy data is

Use this order when choosing screenshots, excerpts, and post material:

| Priority | Location | What to extract | Best format |
| --- | --- | --- | --- |
| 1 | `${CAMPAIGN_DIR}/unified/reports/verification_summary.csv` | `PASS` rows for commitment chain, commitment hashes/signatures, Git root, non-zero mutation linkage, and receipt cross-links; disclose skips | Screenshot or compact table |
| 2 | `${CAMPAIGN_DIR}/logs/ensemble-chat-file-create.log` | User intent, model/tool progression, scenario `ok`, and correlated transaction hash | Short terminal clip |
| 3 | `${CAMPAIGN_DIR}/unified/receipt-verification.txt`, `reports/receipts.csv`, and `receipts-export.json` | Verified signature/persistence count plus action type, transaction binding, deterministic stages, final status, signature, and persistence evidence | Receipt anatomy graphic |
| 4 | `${CAMPAIGN_DIR}/unified/reports/commitments.csv` and `ledger_merkle_root.csv` | Prior commitment hash, commitment hash, transaction hash, state root, and final Git root | Before/after chain graphic |
| 5 | `${CAMPAIGN_DIR}/unified/reports/file_mutations.csv` and `file_diffs.csv` | Independently inspectable useful state change and before/after linkage | Developer carousel or demo zoom-in |
| 6 | `${CAMPAIGN_DIR}/fedramp/scenarios-verbose.txt` | Authorized provision/revert success and unauthorized evidence-destruction rejection | Compliance/security video |
| 7 | `${CAMPAIGN_DIR}/fedramp/ksi-result.json` and `demo-run-verification.json` | Measured KSI counts plus the typed independent verification result for the persisted FedRAMP demo evidence | Compliance post or article |
| 8 | `${CAMPAIGN_DIR}/dhs/scenarios-verbose.txt` and `demo-run-verification.json` | Disconnected continuity, blocked wipe, governed purge, and the typed independent verification result | Defense/edge video |
| 9 | `${REPO_ROOT}/.g8e/data/eval/runs/<run-id>/report.json` and `verification.json` | Suite and version, active posture, lane, allowed and prohibited scenario results, 10 required invariant verdicts, verification validity and failure count, and content-addressed evidence bindings | Technical appendix |
| 10 | `<verified-report-bundle>` and its retained canonical `ComplianceVerificationReport` | Signed bundle scope, protected source inventory, reproduced analysis and renderers, external trust identities, verification time, and exact failures | Point-in-time compliance evidence appendix |
| 11 | `${CAMPAIGN_DIR}/metadata/` | Binary version and digest, source-provenance status or approved provenance artifact, FIPS and Docker versions, provider classification, and campaign capability boundary | Methodology footer |

A polished terminal screenshot uses the concise scenario summary, followed by the matching transaction row and verification row. A technical article links the canonical machine-readable files rather than transcribing hashes manually.

## 7. Publication gates

### Current demonstration gate

A current-run social claim is publishable only when all applicable checks pass:

- The executable version and SHA-256 digest are retained, and the source-provenance status is explicit. Any source-reproducibility claim cites an approved provenance artifact rather than inferring source identity from the binary.
- Provider and model classification is explicit.
- Every quoted scenario appears in the retained log with `ok` or `PASS`.
- The matching action appears in the signed receipt export.
- Any receipt-verification claim has a non-empty verifier result with equal exported and verified counts, zero failures, and the producing public key identified.
- The expected useful state or protected unchanged state is independently checked by the scenario or report.
- `verification_summary.csv` contains no `FAIL` row.
- Every integrity capability named in the post has a corresponding non-skipped `PASS` row with a non-empty subject.
- Mutation-linkage claims use a non-zero mutation count.
- Synthetic targets, canaries, and data are labeled.
- KSI and compliance-evidence language says measured alignment or evidence, never authorization.
- Any demo-evidence verification claim cites a retained canonical report with `"valid":true` and a zero verifier exit status.
- Any signed report-bundle claim cites the exact bundle, external report-trust policy digest, external evidence-trust policy digest when required, canonical `ComplianceVerificationReport` with `"valid":true`, and zero verifier exit status. It states that verification covers the point-in-time represented source set rather than an eval-native bundle or recurring effectiveness.
- The public artifact set excludes private keys, secrets, raw restricted evidence, local credential paths, personal data, and trust material that is not explicitly approved for publication.

### Publication-grade flagship gate

Do not publish the planned flagship result card or aggregate Sovereignty Gauntlet rates until the repository provides and the run passes all of these:

- The preregistered minimum 25-scenario matrix with useful reads, useful mutations, policy attacks, protocol attacks, and benign near-boundary controls.
- At least three frozen repetitions per eligible arm, or the current preregistration’s documented replacement rule.
- Real-provider SDK-boundary canary observations and exact local rehydration observations.
- Independent prohibited-side-effect and final-state observations.
- Complete schema, hash, reference, denominator, stage-graph, receipt, persistence, commitment, state, privacy, encrypted-evidence, and trust-root verification.
- A complete eval-native verifier that checks the frozen experiment population, authoritative metrics, trust roots, references, stage graph, receipts, persistence, commitments, state, privacy evidence, and encrypted-evidence metadata and exits nonzero for deliberate tampering in every advertised eval evidence class. The available signed compliance report-bundle verifier does not satisfy this eval-native gate.
- A generated proof card whose values derive only from authoritative typed metrics.
- Zero silent exclusions and explicit reporting of missingness, infrastructure failures, retries, provider usage reconciliation, latency, tokens, and cost.

Until then, do not claim complete eval-bundle verification, zero raw canaries at all model boundaries, exact local rehydration rates, protocol-attack rejection rates, prohibited-side-effect rates, or publication-grade utility comparisons unless a newer implemented suite directly measures and verifies them. A successfully verified signed compliance report bundle supports a narrower complete-report-bundle claim for the exact represented source set and trust policies.

## 8. Copy/paste-ready post templates

Replace placeholders only with values traced to retained artifacts. Remove lines that the run does not measure.

### Current developer proof

> We gave `<real model name | deterministic fake provider>` a useful file task. g8e admitted the request through its governed path, changed sovereign local state, produced a signed receipt, linked the mutation to its Git-backed ledger, and verified the commitment chain.
>
> Scenario: `<ok/fail>`
> Mutation linkage: `<exact PASS row detail>`
> Receipt/commitment cross-link: `<exact PASS row detail>`
> Commitment chain: `<exact PASS row detail>`
>
> Not an agent log. A cryptographically bound state transition.
>
> `<synthetic-target/provider/verification-scope disclosure>`

### Current security proof

> We submitted authorized work and an authenticated evidence-destruction attempt through the same governance boundary.
>
> Authorized operation: `<measured result>`
> Destruction attempt: `<measured L1 rejection>`
> Prohibited target-side effect: `<scenario’s measured unchanged-state result>`
> Signed evidence: `<receipt or transaction reference>`
>
> Assume the agent, gateway, and network can be wrong. Make the host verify.
>
> `<synthetic-target and verification-scope disclosure>`

### Current compliance proof

> One governed runtime decision produced signed receipts, commitment evidence, a measured FedRAMP 20x KSI result, and a valid independent report over the persisted demo evidence.
>
> Satisfied: `<measured count>`
> Not satisfied: `<measured count>`
> Scenario integrity checks: `<measured result>`
>
> From runtime decision to machine-readable evidence—not a quarterly screenshot.
>
> This demonstrates measured evidence alignment against synthetic cloud resources; it is not a FedRAMP authorization.

### Current defense and edge proof

> We severed a simulated coalition datalink, continued local governance and evidence recording, rejected an audit-destruction request, and completed a separately governed retention purge.
>
> Continuity scenario: `<measured result>`
> Evidence-destruction attempt: `<measured result>`
> Governed purge: `<measured result and receipt reference>`
>
> Sovereign memory survives the link.
>
> The topology is simulated and the data is synthetic. `<state whether model inference was local, cloud-connected, or absent>`

### Future flagship post

Use this only after the publication-grade gate passes and the complete verifier independently reproduces every value:

> We gave a real cloud model useful access to a sovereign host—and then tried to make the same system leak, replay, tamper with, and erase its work.
>
> Authorized tasks completed: `<measured>/<eligible>`
> Policy attacks contained: `<measured>/<eligible>`
> Protocol attacks rejected: `<measured>/<eligible>`
> Raw synthetic secrets observed at the model boundary: `<measured>/<declared>`
> Prohibited side effects: `<measured>/<eligible>`
> Signed receipts and persistence attestations verified: `<measured>/<eligible>`
>
> The model reasoned. The host remembered, verified, and acted. We published the signed evidence bundle and the command that checks it.

## 9. Claims to avoid

Do not use:

- “Unhackable,” “perfect security,” “zero risk,” or “guaranteed safe.”
- “The cloud can never see data.” State the tested provider boundary, declared synthetic canaries, detectors, and observed count.
- “FedRAMP certified” or “FedRAMP authorized” for KSI or demo-evidence output.
- “Independent proof” when the producer controls the verifier and trust root. Use “independently verifiable” only after another party can run complete verification against a published root.
- "BFT multi-agent reasoning" for protocol L2. Describe distinct Ed25519 signers enforcing quorum over deterministic doctrine decisions. The g8ee Tribunal is a separate information-isolated command-generation mechanism; see [Ensemble Agents](../ensemble/agents.md) for the persona roster and Tribunal structure.
- “100% benchmark performance” from a one-task or fake-provider run.
- “No cloud dependency during disconnection” when a cloud-hosted model is still required during the disconnected interval.
- “All integrity checks passed” when any advertised check is `SKIPPED`.

## 10. What the agent returns

At the end of every run, the agent gives the user this concise report:

```text
SOVEREIGNTY GAUNTLET RUN
Run ID: <run-id>
Campaign directory: <absolute path>
Mode: <current demonstration | publication-grade verified>
Executable: <version and SHA-256 digest>
Source provenance: <approved artifact and digest | not collected>
Provider: <provider/model; real/local/fake>

Measured outcomes
- Useful work: <result and denominator>
- Hostile work: <result and denominator available today>
- Signed receipts: <result>
- Persistence/commitments: <result>
- Store verification: <PASS/FAIL/SKIPPED summary>
- KSI/demo evidence: <counts and verification-report status>
- Signed compliance report-bundle verification: <PASS/FAIL/NOT RUN; exact scope and trust inputs>
- Eval-native complete-bundle verification: <NOT IMPLEMENTED/NOT RUN>

Best copy/paste data
1. <absolute artifact path>: <specific lines/rows and why they matter>
2. <absolute artifact path>: <specific lines/rows and why they matter>
3. <absolute artifact path>: <specific lines/rows and why they matter>

Best demo moments
1. <command or log timestamp>
2. <command or log timestamp>
3. <command or log timestamp>

Safe claims
- <artifact-backed sentence>
- <artifact-backed sentence>

Do not claim
- <missing, failed, skipped, synthetic, or unverified limitation>

Recommended post
<one copy/paste-ready post populated only from measured artifacts>
```

The agent also calls out any `FAIL`, `SKIPPED`, empty report, missing observer, fake-provider role, missing source provenance, synthetic target, unavailable KSI binding, or incomplete verification before presenting positive claims.

## README evidence

The root `README.md` is hand-maintained. It states the current native evaluation boundary and links to [Evaluations](../architecture/evals.md) for acceptance invariants, trust boundaries, and JSON output. The native evaluation proof table row reflects the Go-native `core-execution-boundary` suite result against the unified Docker stack. Update the README directly when its current behavior or evidence summary changes, then review it end to end and validate its links.

## Related documentation

- [Evaluations](../architecture/evals.md) — Go-native execution-boundary commands, model campaign evidence, Observer and Provenance Operator witness roles, verification, and the connected evaluation explorer projection.
- [Model Provenance](../architecture/model-provenance.md) — Storage-side weight attestation and chain-of-custody for scored inference.
- [Headless End-to-End UX Smoke Test](ux_smoke_test.md) — authoritative unified-stack enrollment, scenario, report, and troubleshooting sequence.
- [Unified Docker Stack](unified_stack.md) — component topology, identity, storage, and lifecycle.
- [Demo Environments](../../demos/README.md) — per-demo architecture, commands, scenarios, and real-versus-display boundaries.
- [FedRAMP Demo](../../demos/fedramp/README.md) — synthetic cloud campaign and KSI evidence.
- [DHS Demo](../../demos/dhs/README.md) — disconnected-operations and governed-destruction campaign.
- [Proof-Backed Compliance Evidence](../reference/compliance-evidence.md) — current evidence graph, signed report-bundle generation, external trust, complete offline verification, and remaining limits.
- [Compliance Alignment](../reference/compliance-alignment.md) — KSI, protocol-owned compliance catalog, and persisted demo-evidence semantics and claim boundaries.
