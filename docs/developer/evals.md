# g8e Evals Developer Guide

This guide covers the evals system architecture, development workflow, and integration with the g8e platform.

## Architecture Overview

The evals system lives at the repository root `evals/` and is decoupled from any specific application layer.

```
evals/
├── g8e_evals/
│   ├── harness.py          # Core data structures (Task, Response, Score)
│   ├── sut/
│   │   ├── answer_only.py  # SUT-A: Single-turn QA with EVAL_ANSWER signature
│   │   └── tool_use.py     # SUT-B: Multi-turn agents with tool-call receipts
│   ├── receipts/
│   │   ├── collector.py    # Fetch receipts from Operator /api/audit/receipts
│   │   └── verify.py       # Offline Ed25519 signature verification
│   ├── benchmarks/
│   │   └── ifeval/
│   │       ├── loader.py   # Load IFEval gold set
│   │       └── verifier.py # Deterministic constraint verification
│   ├── report/
│   │   ├── aggregate.py    # Per-benchmark stats and pass^k
│   │   └── cli_renderer.py # Rich console output
│   └── cli.py              # Main entry point (run, verify-receipts)
├── gold_sets/              # Local benchmark data (gitignored)
├── reports/                # Output artifacts (gitignored)
├── pyproject.toml          # Independent Python environment
└── README.md
```

### Data Flow

1. **Task Loading**: Benchmark loader reads tasks from gold set
2. **Execution**: SUT calls LLM API and submits EVAL_ANSWER envelope to Operator
3. **Receipt Collection**: Receipt collector fetches signed receipts from audit vault
4. **Verification**: Receipt verifier checks Ed25519 signature using Warden public key
5. **Scoring**: Benchmark verifier evaluates response against constraints
6. **Output**: Results written to JSON with receipt verification status

## EVAL_ANSWER Action Type

The evals system uses the `EVAL_ANSWER` action type, an observability-only action that does not require L3 approval. It allows the evals harness to submit answers for cryptographic attestation without triggering human approval workflows.

### Protobuf Schema

```protobuf
message EvalAnswerRequested {
  string prompt_id = 1;
  string benchmark = 2;
  string answer = 3;
  string model = 4;
}
```

### Registration

- Added to `protocol/proto/operator.proto`
- Event registered in `protocol/constants/events.json` as `Operator.Eval.AnswerRequested`
- Mapped in `services/g8eo/internal/mappings/action_types.go`
- Payload decoder in `services/g8eo/internal/services/governance/transaction_verifier.go`
- Registered in known action types for pubsub, listen, and chaos tester
- No-op execution handler in Warden via `services/g8eo/internal/services/pubsub/pubsub_commands.go`

## Warden Public Key Export

The Warden's Ed25519 public key is exported at Operator bootstrap to enable offline receipt verification.

### Export Location

- **PEM**: `.g8e/pki/warden_pub.pem` - Standard PEM-encoded public key
- **JSON**: `.g8e/pki/warden_pub.json` - Contains `key_id`, `public_key` (hex), and `algorithm`

### Implementation

Export occurs in `services/g8eo/cmd/g8eo/main.go` in the `runListenMode` function after loading the Warden signing key:

```go
wardenPub := wardenPriv.Public().(ed25519.PublicKey)
if err := exportWardenPublicKey(cfg.PKIDir, wardenPub, wardenKeyID, logger); err != nil {
    logger.Error("Failed to export Warden public key", "error", err)
    os.Exit(constants.ExitConfigError)
}
```

The `exportWardenPublicKey` function writes both PEM and JSON formats with nil-safe logging for test compatibility.

### Testing

Unit tests in `services/g8eo/cmd/g8eo/warden_pub_export_test.go` verify:
- PEM file creation with correct header
- JSON file creation with correct structure
- Directory creation if PKI directory doesn't exist

## Receipt Verification

Receipt verification ensures that EVAL_ANSWER actions were actually processed by the Warden and signed with the correct key.

### Verification Steps

1. Load Warden public key from PEM or JSON file
2. Extract signature from receipt
3. Verify transaction ID matches receipt
4. Verify transaction hash matches receipt
5. Verify key ID matches Warden public key
6. Verify Ed25519 signature over `transaction_hash + "|true"`

### Implementation

The `ReceiptVerifier` class in `g8e_evals/receipts/verify.py` handles verification:

```python
verifier = ReceiptVerifier(pki_dir=".g8e/pki")
verified, error = verifier.verify_receipt(
    receipt,
    transaction_id="tx-123",
    transaction_hash="hash-abc",
)
```

### Self-Checks

In receipt mode, the CLI performs self-checks:
- Fails if `receipt_verified=False` without an error reason
- Fails if `transaction_id` is missing when `receipt_verified=True`
- Fails if `transaction_hash` is missing when `receipt_verified=True`

## Benchmark Development

### Adding a New Benchmark

1. Create benchmark directory: `g8e_evals/benchmarks/<name>/`
2. Implement `loader.py` with a class that loads tasks:
   ```python
   class MyBenchmarkLoader:
       def load_tasks(self, limit: int | None = None) -> List[Task]:
           # Load tasks from gold set
   ```
3. Implement `verifier.py` with a class that verifies responses:
   ```python
   class MyBenchmarkVerifier:
       def verify(self, response: Response) -> Score:
           # Verify response against constraints
   ```
4. Add benchmark name to CLI choices in `g8e_evals/cli.py`
5. Add integration tests in `tests/test_<name>_smoke.py`

### IFEval Constraints

The IFEval verifier supports:

- **Length constraints**: Word count, character count, sentence count
- **Forbidden words**: Reject responses containing specific words
- **Required words**: Require responses to contain specific words
- **Format constraints**: List, JSON, uppercase, lowercase
- **Regex constraints**: Match against regular expression patterns

## CLI Usage

### Running Evaluations

```bash
# Using the g8e CLI wrapper
./g8e evals bench --suite ifeval --mode receipt \
    --operator-session-id $OPERATOR_SESSION_ID \
    --operator-id op-1

# Baseline mode
./g8e evals bench --suite ifeval --mode baseline --model openai:gpt-4o
```

### Offline Verification

```bash
./g8e evals verify-receipts --results reports/ifeval-20260516-200000
```

## Testing

### Unit Tests

Run unit tests for the evals package:

```bash
cd evals
pytest tests/ -v
```

### Integration Tests

Integration tests require a running Operator in listen mode:

```bash
# Start Operator in listen mode
./g8e platform start

# Run evals with real Operator
./g8e evals bench \
  --suite ifeval \
  --mode receipt \
  --operator-session-id $(cat .g8e/session_id) \
  --limit 5
```

## Output Artifacts

### results.json

Contains individual task results with fields:

```json
{
  "task": {
    "prompt_id": "ifeval_001",
    "prompt": "...",
    "benchmark": "ifeval",
    "metadata": {...}
  },
  "response": {
    "answer": "...",
    "model": "openai:gpt-4",
    "metadata": {"latency_ms": 1234}
  },
  "score": {
    "passed": true,
    "score": 1.0,
    "reason": "All constraints satisfied"
  },
  "receipt_verified": true,
  "receipt_error": null,
  "transaction_id": "tx-123",
  "transaction_hash": "hash-abc",
  "latency_ms": 1234,
  "error": null
}
```

### summary.json

Contains aggregate statistics:

```json
{
  "total_tasks": 10,
  "completed_tasks": 10,
  "passed_tasks": 8,
  "failed_tasks": 2,
  "error_tasks": 0,
  "receipt_verified_count": 10,
  "receipt_failed_count": 0,
  "average_score": 0.8,
  "mode": "receipt"
}
```

## PKI Integration

The evals system integrates with the g8e PKI infrastructure:

- **Trust bundle**: `.g8e/pki/trust/hub-bundle.pem` for TLS verification.
  Resolved by `g8e_evals.tls.resolve_trust_bundle()` in this order:
  1. `G8E_TRUST_BUNDLE` (explicit path override)
  2. `${G8E_PKI_DIR:-.g8e/pki}/trust/hub-bundle.pem`

  TLS verification is mandatory; if no bundle is found the harness fails
  closed rather than silently disabling verification.
- **Client certificates**: `.g8e/pki/client.crt` and `.g8e/pki/client.key` for mTLS authentication
- **Warden public key**: `.g8e/pki/warden_pub.pem` and `.g8e/pki/warden_pub.json` for receipt verification

See `docs/g8eo/pki.md` for complete PKI documentation.

## Troubleshooting

### Warden Public Key Not Found

If the evals harness cannot find the Warden public key:

1. Ensure the Operator is running in listen mode
2. Check that `.g8e/pki/warden_pub.pem` or `.g8e/pki/warden_pub.json` exists
3. Verify the PKI directory path is correct (default: `.g8e/pki`)

### Receipt Verification Failures

If receipt verification fails:

1. Check that the Operator audit vault contains receipts for the evaluated tasks
2. Verify the Warden public key matches the one used to sign the receipts
3. Ensure transaction IDs and hashes match between results and receipts

### LLM Integration

The `AnswerOnlySUT` currently uses placeholder LLM calls. To integrate with a real LLM provider:

1. Implement `_call_llm()` in `g8e_evals/sut/answer_only.py`
2. Add API key configuration
3. Handle rate limiting and retry logic
4. Add proper error handling for API failures

## Future Work

- Real LLM integration (OpenAI, Anthropic, etc.)
- Additional benchmarks (SimpleQA, GPQA, etc.)
- Parallel task execution
- Receipt caching to reduce Operator load
- Web UI for viewing results
- CI/CD integration for automated eval runs
