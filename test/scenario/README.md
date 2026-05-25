# Scenario-Based Integration Testing Framework

This framework provides table-driven integration tests for the g8e governance substrate. It tests the real admission path (TransactionVerifier + Actuator) against a fixture matrix of security scenarios, asserting deterministic verdicts and diffing signed receipts against golden files.

## Architecture

```
test/scenario/
  fixtures/      // go:embed — the payload matrix
    security/forged_sig.json
    finance/wire_replay.json
  golden/        // signed-receipt snapshots (TODO)
  scenario.go    // Scenario struct + loader
  runner.go      // fires a fixture at the REAL admission path
  report.go      // pretty trace (the theater)
  scenario_test.go // table-driven test with TestMain
```

## Running the Tests

### Local Development

```bash
# Run scenario tests with verbose output
go test -tags=integration -v -run TestScenarios ./test/scenario/...

# Run a specific scenario
go test -tags=integration -v -run TestScenarios/forge_signature ./test/scenario/...
```

### CI Pipeline

The scenario tests run in a separate CI job (`test-scenarios`) to keep the main test suite fast:

```yaml
- name: Run scenario integration tests
  run: |
    go test -tags=integration -v -run TestScenarios ./test/scenario/...
```

## Scenario Structure

A scenario is pure data defined in JSON:

```json
{
  "name": "forge_signature",
  "vertical": "security",
  "narrative": "Envelope with forged L2 signature should be rejected",
  "intent": <GovernanceEnvelope JSON bytes>,
  "evidence": {
    "l2_signature_present": true,
    "l2_key_id": "tribunal_1",
    "l3_proof_present": false,
    "signer_id": "tribunal_1"
  },
  "expect": {
    "doctrine": {
      "verdict": "reject",
      "reject_reason": "TX_QUORUM_L2_SIG_INVALID",
      "l2_valid": false,
      "l3_valid": false
    },
    "consensus": { ... },
    "notary": { ... }
  }
}
```

### Fields

- **name**: Unique identifier for the scenario
- **vertical**: Domain category (security, finance, etc.)
- **narrative**: Human-readable description
- **intent**: Raw GovernanceEnvelope JSON bytes (the mutation payload)
- **evidence**: Which governance proofs are present
- **expect**: Expected outcome per governance mode (doctrine, consensus, notary)

## Governance Modes

The framework tests three governance postures:

- **doctrine**: L1 (Doctrine) validation only (L2 and L3 not required)
- **consensus**: L1 (Doctrine) + L2 validation (no L3 required)
- **notary**: L1 (Doctrine) + L2 + L3 validation

Each mode has different requirements for L2 signatures and L3 proofs. Doctrine is the minimal posture, accepting any envelope that passes L1 validation.

## Deterministic Testing

The framework uses injectable dependencies to ensure deterministic results:

- **Clock**: Fixed time (2026-05-24 12:00:00 UTC) for expiry checks
- **StateRoot**: Fixed state root ("abc123def456") for state binding
- **ReplayStore**: In-memory store for nonce replay protection
- **Signers**: Generated test ED25519 keypairs for L2 verification

This prevents flaky tests due to wall time or state drift.

## Adding New Scenarios

1. Create a JSON file in `test/scenario/fixtures/{vertical}/{name}.json`
2. Define the envelope payload in the `intent` field
3. Specify which governance proofs are present in `evidence`
4. Define expected outcomes for each mode in `expect`
5. Run the tests to verify

## Golden File Diffing

The framework automatically diffs signed receipts against golden files in `test/scenario/golden/{scenario}_{mode}.golden.json`. When a scenario accepts an envelope, the receipt is serialized to JSON and compared against the golden snapshot. Set `G8E_UPDATE_GOLDEN=1` to refresh golden files after intentional changes.

## Database Persistence

The framework uses real SQLite databases (no mocks) to verify receipt persistence. This ensures the substrate actually writes receipts to the audit store as expected in production.

### Database Setup

- **Setup**: `SetupTestDB()` initializes an in-memory SQLite database with the gateway schema
- **Teardown**: `TeardownTestDB()` closes the database connection after all tests complete
- **Lifecycle**: Database is created once in `TestMain()` and shared across all scenario tests

### Receipt Verification

- **Query Helper**: `QueryReceipt()` retrieves persisted receipts by transaction ID from the database
- **Assertion**: `AssertPersistedReceipt()` verifies that accepting scenarios persist receipts and rejecting scenarios do not
- **Integration**: The `OperatorGate` uses a real `TransactionAuditStore` backed by the test database

This approach follows the "no mocks" principle from `docs/devs.md`, ensuring tests exercise the actual persistence path rather than mocked behavior.

## Current Scenarios

### Forge Anything (#6)

Four security scenarios testing fundamental rejection criteria:

- **forge_signature**: Forged L2 signature → reject
- **replay_nonce**: Replayed nonce → reject
- **stale_state_root**: Stale state root → reject
- **tampered_receipt**: Tampered receipt signature → reject

These are the CI backbone - trivially deterministic and fast.

## Future Scenarios

Planned scenarios from the original specification:

- **Same Knife (#1)**: One intent, three producer variants, assert identical verdict
- **Go Around It (#3)**: Assert the only mutation path is the Actuator
- **Runaway (#4)**: Doctrine forbidden-pattern fixture
- **Worm Enrolls (#5)**: Device-link token validation
- **Hand Me the Proof (#7)**: Receipt chain validation with scorecard
- **Pull the Cable (#2)**: Transport fault-injection (requires `-tags=integration,partition`)

## The Theater

Under `-v`, the test prints a full gauntlet trace:

```
=== Scenario: forge_signature (doctrine mode) ===
Vertical: security
Narrative: Envelope with forged L2 signature should be rejected
Evidence: L2=true (key=tribunal_1), L3=false, signer=tribunal_1
Result: REJECTED - TX_QUORUM_L2_SIG_INVALID
```

The same test that gates the pipeline is the demo - no duplicate maintenance.
