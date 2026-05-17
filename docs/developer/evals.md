# g8e Evals Developer Guide

The evals harness drives the **real g8ee chat pipeline end-to-end** — Triage,
Dash/Sage, Tribunal, Auditor, Warden — captures every agent stage emitted on
the Operator's per-session SSE buffer, and folds the full agent trail into a
per-task receipt for offline replay.

This is the gold-standard evaluation path: the model under test exercises the
same code paths a real user hits via `./g8e chat send`. There is no longer a
stubbed "answer-only" SUT.

## Architecture Overview

```
evals/
├── g8e_evals/
│   ├── harness.py                # Task / Response / Score / SUTConfig
│   ├── transport.py              # AuthContext: canonical mTLS + cookie + headers
│   ├── sut/
│   │   ├── g8ee_chat.py          # G8eeChatSUT — drives /api/internal/chat
│   │   └── wire.py               # Typed SSE wire envelopes (parity with g8ee)
│   ├── agent_trail_renderer.py   # Live per-event CLI rendering
│   ├── receipts/
│   │   ├── collector.py          # Poll Operator /api/audit/receipts by tx_id
│   │   └── verify.py             # Offline Ed25519 signature verification
│   ├── benchmarks/ifeval/        # IFEval loader + verifier
│   ├── report/                   # Aggregation + Rich CLI summary
│   └── cli.py                    # Entry point: run, verify-receipts
├── tests/                        # Unit + contract tests
├── gold_sets/                    # Local benchmark data (gitignored)
└── reports/                      # Output artifacts (gitignored)
```

## Data Flow (per task)

1. `G8eeChatSUT.get_answer(task)` snapshots the Operator's SSE cursor
   (`GET /api/internal/sse/events?since_id=0`) so only events from this turn
   are consumed.
2. It POSTs `/api/internal/chat` on the g8ee Engine with `resource_creation.create_case=true`
   and per-role provider/model/api_key overrides from `SUTConfig`. g8ee creates
   a fresh case+investigation and runs the chat pipeline as a background task.
3. The SUT polls `GET /api/internal/sse/events?since_id=<cursor>` every
   `poll_interval_s` (default 0.25s) and accumulates `AgentTrailEvent` rows
   filtered by `investigation_id` until a terminal event or `idle_timeout_s`.
4. `text.chunk` payloads are concatenated into the final `answer` string.
5. The trail is scanned for `g8e.v1.ai.governance.warden.receipt.signed`. If
   present, its `transaction_hash` becomes the substrate `transaction_id` and
   `ReceiptCollector` polls `/api/audit/receipts?tx_id=...` for the signed
   ActionReceipt. Otherwise the response is `BindingType.UNBOUND` with
   `unbound_reason="answer-only turn (no Warden-signed ActionReceipt emitted)"`.
6. Per-task results (full agent trail + optional substrate receipt) are
   written to `reports/<suite>-<ts>/results.jsonl`.

## Prerequisites

The harness fails closed without the canonical mTLS + session credentials.

1. `./g8e platform start` — Operator + g8ee.
2. `./g8e login` — mints client cert/key, captures session id, exports the
   environment variables `scripts/cmd/evals.sh` re-exports for the bench:
   - `OPERATOR_SESSION_ID`, `USER_ID`
   - `G8E_CLI_CERT`, `G8E_CLI_KEY`, `G8E_TRUST_BUNDLE`
   - `G8EE_URL`, `G8E_INTERNAL_HTTP_URL`, `G8E_PKI_DIR`
3. **LLM API keys** must be available to g8ee for any provider you select.
   The chat request body forwards `llm_<role>_provider`, `llm_<role>_model`,
   `llm_<role>_api_key`, and `llm_<role>_endpoint` for the `primary`,
   `assistant`, and `lite` roles. The CLI performs a pre-flight check against
   `GET /api/internal/settings/user`: if a provider is selected but no key is
   configured (neither via CLI flag nor server-side settings), the run aborts
   with an actionable error before posting any tasks.

## Receipt Semantics

The Operator's audit vault keys ActionReceipts by **UAP envelope
`transaction_hash`**, *not* by g8ee `investigation_id`. The SUT respects this:

| Outcome                                                | `transaction_id`        | `binding`        |
|--------------------------------------------------------|-------------------------|------------------|
| Answer-only turn (no Tribunal→Warden mutation)         | `None`                  | `UNBOUND`        |
| Warden signed an ActionReceipt during the turn         | receipt `transaction_hash` | `RECEIPT_BOUND` |
| Pipeline failed (`iteration.failed`, dead-lettered, …) | substrate hash if any   | `UNBOUND`        |
| Idle timeout, no terminal event                        | substrate hash if any   | `UNBOUND`        |

Regression coverage: `evals/tests/test_g8ee_chat_sut.py` pins
`_extract_substrate_transaction_id` against trail shapes that previously
caused investigation ids to be promoted to substrate transaction ids.

## Receipt Verification

`g8e_evals.receipts.verify.verify_receipt_signature(receipt, warden_pub)`
performs offline Ed25519 verification:

- Loads the Warden public key from `${G8E_PKI_DIR:-.g8e/pki}/warden_pub.pem`
  (exported by `runListenMode` in `services/g8eo/cmd/g8eo/main.go`).
- Verifies the signature over `transaction_hash + "|true"` with the receipt's
  declared `key_id` matching the Warden key id.

The CLI runs verification inline during `run`, and the `verify-receipts`
subcommand re-verifies a saved report directory offline:

```bash
./g8e evals verify-receipts reports/ifeval-20260517-072900
```

## CLI Usage

```bash
./g8e evals bench \
    --suite ifeval \
    --provider openai --model gpt-4o-mini \
    --primary-api-key "$OPENAI_API_KEY" \
    --limit 5
```

Useful flags:

- `--verbose-text` — stream the agent's response text inline as chunks arrive.
- `--idle-timeout` — seconds without any SSE event before declaring idle (default 180s).
- `--assistant-provider/--assistant-model`, `--lite-provider/--lite-model` —
  per-role overrides for Tribunal/Auditor and lightweight calls.

## Output Artifacts

Each run writes `reports/<suite>-<ts>/`:

- `results.jsonl` — one row per task. Each row contains the full
  `agent_trail`, `event_counts_by_type`, `terminal_event`, `case_id`,
  `investigation_id`, the assembled `answer`, optional `substrate_receipt`
  (only when Warden signed), and the benchmark score.
- `summary.json` — aggregate stats produced by `report/aggregate.py`.

## Wire Contract

SSE payloads are parsed via `g8e_evals.sut.wire.SSEWireEnvelope` (Pydantic),
which mirrors `services/g8ee/app/models/events.py`'s `SessionEventWire` /
`BackgroundEventWire`. The contract test in `evals/tests/test_sse_wire.py`
fails loudly if the publisher's shape drifts so the bench cannot silently
break.

## Auth Wiring Parity

`evals/g8e_evals/transport.py::AuthContext` is the single source of truth for
mTLS + `g8e_session` cookie + `X-G8E-*` context headers. It must stay in
lockstep with `scripts/cmd/common.sh::_g8ee_curl` /
`_append_g8e_context_headers`. The contract test in
`evals/tests/test_auth_wiring_parity.py` enforces that the headers shell and
Python emit do not diverge.

## Known Limitation: Polling, Not Streaming

`G8eeChatSUT._drain_events` busy-polls the Operator's per-session SSE *replay
buffer* (`GET /api/internal/sse/events?since_id=...`). Consequences:

- Wall-clock latency floor of one `poll_interval_s` per observed event.
- Long Sage ReAct turns issue thousands of GETs while waiting for the
  terminal event.

Acceptable for v1; the long-term fix is to consume an Operator-native
`text/event-stream` endpoint. Tracked by the `TODO(evals)` in the SUT module
docstring.

## Testing

```bash
# Unit + contract tests (pytest in the g8ee venv, which has g8e-evals installed):
services/g8ee/.venv/bin/python -m pytest evals/tests/ -v

# End-to-end smoke against a live platform:
./g8e platform start
./g8e login
./g8e evals bench --suite ifeval --limit 1 --primary-api-key "$OPENAI_API_KEY" \
    --provider openai --model gpt-4o-mini
```

A successful smoke run shows live `ITERATION/start`, `TRIAGE`, `THINKING`,
`TEXT`, `ITERATION/text.completed` lines, `agent_events > 0` in the per-task
summary, and a non-empty `agent_trail` in `results.jsonl`.

## PKI Integration

- **Trust bundle**: `${G8E_TRUST_BUNDLE}` or
  `${G8E_PKI_DIR:-.g8e/pki}/trust/hub-bundle.pem`. Resolved by
  `g8e_evals.tls.resolve_trust_bundle()`. The harness fails closed if the
  bundle is missing rather than silently disabling verification.
- **Client mTLS**: `G8E_CLI_CERT` / `G8E_CLI_KEY` (minted by `./g8e login`).
- **Warden public key**: `.g8e/pki/warden_pub.pem` (PEM) and
  `.g8e/pki/warden_pub.json` (JSON with `key_id`, hex `public_key`,
  `algorithm`).

See `docs/g8eo/pki.md` for the full PKI story.

## Adding a New Benchmark

1. Create `g8e_evals/benchmarks/<name>/` with a `loader.py` (yields `Task`)
   and a `verifier.py` (returns `Score`).
2. Add `<name>` to the `--suite` choices in `g8e_evals/cli.py`.
3. Add an integration smoke test under `evals/tests/test_<name>_smoke.py`.

## Troubleshooting

- **AuthenticationError: missing OPERATOR_SESSION_ID / USER_ID** — re-run
  `./g8e login`.
- **Pre-flight validation failed: missing API key for primary provider** —
  pass `--primary-api-key` (and equivalents for assistant/lite) or configure
  the keys in g8ee user settings before running.
- **All tasks `UNBOUND` with `answer-only turn`** — expected for IFEval-style
  prompts that never trigger a Warden mutation. Receipt binding only occurs
  when the agent stack escalates a typed mutation through Tribunal→Warden.
- **Receipt verification failure** — confirm `.g8e/pki/warden_pub.pem` was
  exported by the running Operator and that `--operator-url` points at the
  same instance.
