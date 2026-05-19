---
title: Evals
---

# g8e Evals

Last Updated: 2026-05-18

The evals harness drives the **real g8ee chat pipeline end-to-end** — Triage, Dash/Sage, Tribunal, Auditor, Warden — captures every agent stage emitted on the Operator's per-session SSE buffer, and folds the full agent trail into a per-task receipt for offline replay. This is the gold-standard evaluation path: the model under test exercises the same code paths a real user hits via `./g8e chat send`.

---

## Architecture

```
evals/
├── g8e_evals/
│   ├── harness.py                # Task / Response / Score / SUTConfig
│   ├── transport.py              # AuthContext: mTLS + cookie + headers
│   ├── sut/
│   │   ├── g8ee_chat.py          # G8eeChatSUT — drives /api/internal/chat
│   │   └── wire.py               # Typed SSE wire envelopes (parity with g8ee)
│   ├── agent_trail_renderer.py   # Live per-event CLI rendering
│   ├── receipts/
│   │   ├── collector.py          # Poll Operator /api/audit/receipts by tx_id
│   │   └── verify.py             # Offline Ed25519 signature verification
│   ├── benchmarks/ifeval/        # IFEval loader + verifier
│   ├── report/                   # Aggregation + Rich CLI summary
│   └── cli.py                    # Entry point: bench, verify-receipts, list
├── tests/                        # Unit + contract tests
├── gold_sets/                    # Local benchmark data (gitignored)
└── reports/                      # Output artifacts (gitignored)
```

---

## Per-Task Data Flow

1. `G8eeChatSUT.get_answer(task)` snapshots the Operator's SSE cursor (`GET /api/internal/sse/events?since_id=0`) so only events from this turn are consumed.
2. SUT POSTs `/api/internal/chat` on g8ee with `resource_creation.create_case=true` and per-role provider/model/api-key overrides from `SUTConfig`. g8ee creates a fresh case+investigation and runs the chat pipeline as a background task.
3. SUT polls `GET /api/internal/sse/events?since_id=<cursor>` every `poll_interval_s` (default 0.25s), accumulating `AgentTrailEvent` rows filtered by `investigation_id` until a terminal event or `idle_timeout_s`.
4. `text.chunk` payloads concatenate into the final `answer` string.
5. The trail is scanned for `g8e.v1.ai.governance.warden.receipt.signed`. If present, its `transaction_hash` becomes the substrate `transaction_id` and `ReceiptCollector` polls `/api/audit/receipts?tx_id=...` for the signed `ActionReceipt`.
6. Per-task results (full agent trail + optional substrate receipt) are written to `reports/<suite>-<ts>/results.jsonl`.

---

## Receipt Semantics

The Operator's audit vault keys ActionReceipts by **UAP envelope `transaction_hash`**, not by g8ee `investigation_id`.

| Outcome | `transaction_id` | `binding` |
|---|---|---|
| Answer-only turn (no Tribunal→Warden mutation) | `None` | `UNBOUND` |
| Warden signed an ActionReceipt during the turn | receipt `transaction_hash` | `RECEIPT_BOUND` |
| Pipeline failed (`iteration.failed`, dead-lettered) | substrate hash if any | `UNBOUND` |
| Idle timeout, no terminal event | substrate hash if any | `UNBOUND` |

Regression coverage: `evals/tests/test_g8ee_chat_sut.py` pins `_extract_substrate_transaction_id` so investigation IDs are never promoted to substrate transaction IDs.

---

## Receipt Verification

`g8e_evals.receipts.verify.verify_receipt_signature(receipt, warden_pub)` performs offline Ed25519 verification:

- Loads the Warden public key from `${G8E_PKI_DIR:-.g8e/pki}/warden_pub.pem` (exported by Listen Mode startup in `services/g8eo/cmd/g8eo/main.go`).
- Verifies the signature over `transaction_hash + "|true"` with the receipt's declared `key_id` matching the Warden key id.

Inline verification runs during `bench`. To re-verify a saved report directory offline:

```bash
./g8e evals verify-receipts reports/ifeval-20260517-072900
```

---

## Prerequisites

The harness is fail-closed without canonical mTLS + session credentials:

1. `./g8e platform start` — Operator + g8ee.
2. `./g8e login` — mints client cert/key, captures session id, exports the env vars `scripts/cmd/evals.sh` re-exports for the bench:
   - `OPERATOR_SESSION_ID`, `USER_ID`
   - `G8E_CLI_CERT`, `G8E_CLI_KEY`, `G8E_TRUST_BUNDLE`
   - `G8EE_URL`, `G8E_INTERNAL_HTTP_URL`, `G8E_PKI_DIR`
3. **LLM API keys** must be available to g8ee for any provider you select. The chat request body forwards `llm_<role>_provider`, `llm_<role>_model`, `llm_<role>_api_key`, and `llm_<role>_endpoint` for `primary`, `assistant`, and `lite`. The CLI performs a pre-flight check against `GET /api/internal/settings/user`; missing keys abort the run before any tasks post.

---

## CLI Usage

```bash
./g8e evals bench \
    --suite ifeval \
    --provider openai --model gpt-4o-mini \
    --primary-api-key "$OPENAI_API_KEY" \
    --limit 5
```

Useful flags:

- `--verbose-text` — stream the agent's response inline as chunks arrive.
- `--idle-timeout` — seconds without any SSE event before declaring idle (default 180s).
- `--assistant-provider/--assistant-model`, `--lite-provider/--lite-model` — per-role overrides for Tribunal/Auditor and lightweight calls.
- `--mode baseline|receipt` — baseline mode runs without binding; receipt mode requires `--operator-session-id` and `--operator-id`.

---

## Output Artifacts

Each run writes `reports/<suite>-<ts>/`:

- `results.jsonl` — one row per task with full `agent_trail`, `event_counts_by_type`, `terminal_event`, `case_id`, `investigation_id`, assembled `answer`, optional `substrate_receipt`, and benchmark score.
- `summary.json` — aggregate stats from `report/aggregate.py`.

---

## Wire Contract

SSE payloads are parsed via `g8e_evals.sut.wire.SSEWireEnvelope` (Pydantic), mirroring `services/g8ee/app/models/events.py`. The contract test in `evals/tests/test_sse_wire.py` fails loudly if the publisher's shape drifts.

`G8E_STRICT_SSE=true` enables `extra="forbid"` on `SSEWireEnvelope` and `SSEEventBody` and raises on any schema mismatch. Use this during development to catch protocol changes early.

---

## Auth Wiring Parity

`evals/g8e_evals/transport.py::AuthContext` is the single source of truth for mTLS + `g8e_session` cookie + `X-G8E-*` context headers. It must stay in lockstep with `scripts/cmd/common.sh::_g8ee_curl` / `_append_g8e_context_headers`. The contract test `evals/tests/test_auth_wiring_parity.py` enforces shell↔Python parity.

---

## Evaluation Dimensions

| Dimension | Purpose | Gold set |
|---|---|---|
| **Accuracy** | LLM-as-a-judge grading against expected behavior and required concepts. | `evals/gold_sets/accuracy.json` (planned) |
| **Benchmark / Safety** | Deterministic regex matching on tool-call payloads, including refusal scenarios. | `evals/gold_sets/benchmark.json` (planned) |
| **Privacy** | Sentinel PII redaction across egress layers. Validates `[PII]`, `[AWS_KEY]`, `[AWS_SECRET]`, `[URL_WITH_CREDENTIALS]`, `[CONN_STRING]`, `[CREDENTIAL_REFERENCE]` placeholders match `app/security/sentinel_scrubber.py`. | `evals/gold_sets/privacy.json` (planned) |
| **IFEval** | Instruction-Following Evaluation benchmark. | `evals/gold_sets/ifeval/input_data.jsonl` |

---

## Adding a New Benchmark

1. Create `g8e_evals/benchmarks/<name>/` with a `loader.py` (yields `Task`) and a `verifier.py` (returns `Score`).
2. Add `<name>` to the `--suite` choices in `g8e_evals/cli.py`.
3. Add an integration smoke test under `evals/tests/test_<name>_smoke.py`.

---

## Known Limitation: Polling, Not Streaming

`G8eeChatSUT._drain_events` busy-polls the Operator's per-session SSE replay buffer. Consequences:

- Wall-clock latency floor of one `poll_interval_s` per observed event.
- Long Sage ReAct turns issue thousands of GETs while waiting for the terminal event.

Acceptable for v1; the long-term fix is to consume an Operator-native `text/event-stream` endpoint. Tracked by the `TODO(evals)` in the SUT module docstring.

---

## Troubleshooting

| Symptom | Action |
|---|---|
| `AuthenticationError: missing OPERATOR_SESSION_ID / USER_ID` | Re-run `./g8e login`. |
| `Pre-flight validation failed: missing API key for primary provider` | Pass `--primary-api-key` (and equivalents) or configure the keys in g8ee user settings. |
| All tasks `UNBOUND` with `answer-only turn` | Expected for IFEval-style prompts that never trigger a Warden mutation. Receipt binding only occurs when the agent stack escalates a typed mutation through Tribunal→Warden. |
| Receipt verification failure | Confirm `.g8e/pki/warden_pub.pem` was exported by the running Operator and `--operator-url` points at the same instance. |

See also: [Protocol](protocol.md), [Operator](operator.md), [g8ee Service](g8ee_service.md), [Tests](tests.md).
