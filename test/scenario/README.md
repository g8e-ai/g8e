# Scenario-Based Integration Testing Framework

This framework provides table-driven integration tests for the g8e governance platform. It tests the real admission path (TransactionVerifier + Actuator) against a fixture matrix of security scenarios, asserting deterministic verdicts and diffing signed receipts against golden files.

## Architecture

```
test/scenario/
  fixtures/      // go:embed — the payload matrix
    security/forged_sig.json
    finance/wire_replay.json
  golden/        // signed-receipt snapshots
  scenario.go    // Scenario struct + loader
  runner.go      // fires a fixture at the REAL admission path
  report.go      // pretty trace (the theater)
  scenario_test.go // table-driven test with TestMain
```

## Running the Tests

### Prerequisites

Before running scenario integration tests, ensure:

1. The Gateway is running: `./g8e gw start`
2. You have authenticated with the Gateway: `./g8e auth login`

If you have recently run `./g8e gw clean`, you must re-authenticate before running tests, as the PKI hierarchy is regenerated and existing CLI credentials become invalid.

### Local Development

```bash
# Run scenario tests with verbose output (using g8e wrapper)
./g8e test scenario -v

# Run scenario tests with verbose output (direct go test)
go test -tags=integration -v -run TestScenarios ./test/scenario/...

# Run a specific scenario (using g8e wrapper)
./g8e test scenario --run l2_invalid -v

# Run a specific scenario (direct go test)
go test -tags=integration -v -run TestScenarios/l2_invalid ./test/scenario/...
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
  "name": "l2_invalid",
  "vertical": "gates",
  "narrative": "Envelope with forged L2 signature: rejected in consensus/notary (L2 enforced), accepted in doctrine (L2 audited only)",
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

The framework automatically diffs signed receipts against golden files in `test/scenario/golden/{scenario}_{mode}.golden.json`. When a scenario accepts an envelope, the receipt is serialized to JSON and compared against the golden snapshot. Golden files are auto-created if missing and auto-updated on mismatch.

## Database Persistence

The framework uses real SQLite databases (no mocks) to verify receipt persistence. This ensures the platform actually writes receipts to the audit store as expected in production.

### Database Setup

- **Setup**: `SetupTestDB()` initializes an in-memory SQLite database with the gateway schema
- **Teardown**: `TeardownTestDB()` closes the database connection after all tests complete
- **Lifecycle**: Database is created once in `TestMain()` and shared across all scenario tests

### Receipt Verification

- **Query Helper**: `QueryReceipt()` retrieves persisted receipts by transaction ID from the database
- **Assertion**: `AssertPersistedReceipt()` verifies that accepting scenarios persist receipts and rejecting scenarios do not
- **Integration**: The `OperatorGate` uses a real `TransactionAuditStore` backed by the test database

This approach follows the "no mocks" principle from `docs/guides/devs.md`, ensuring tests exercise the actual persistence path rather than mocked behavior.

## Current Scenarios

### Forge Anything (#6)

Security scenarios testing fundamental rejection criteria:

- **l2_invalid**: Forged L2 signature → reject
- **actual_replay**: Replayed nonce (store seeded) → reject
- **stale_state_root**: Stale state root → reject
- **l3_missing**: Missing L3 proof in notary mode → reject
- **tampered_receipt**: Valid envelope accepted, receipt signature tampered → tampering detected
- **malformed_payload**: Invalid protobuf payload structure → reject
- **empty_payload**: Missing payload field → reject

These are the CI backbone - trivially deterministic and fast. The `tampered_receipt` scenario specifically tests the "tamper-evident" property of signed receipts. The edge case fixtures (malformed_payload, empty_payload) ensure fail-closed behavior for malformed inputs.

## Future Scenarios

Planned scenarios from the original specification:

- **Same Knife (#1)**: One intent, three producer variants, assert identical verdict
- **Go Around It (#3)**: Assert the only mutation path is the Actuator
- **Runaway (#4)**: Doctrine forbidden-pattern fixture
- **Worm Enrolls (#5)**: CSR-based enrollment validation
- **Hand Me the Proof (#7)**: Receipt chain validation with scorecard
- **Pull the Cable (#2)**: Transport fault-injection (requires `-tags=integration,partition`)

## Viewing Receipts

Receipts are printed to the test output when running with the `-v` flag:

```bash
go test -tags=integration -v -run TestScenarios ./test/scenario/...
```

For accepted scenarios, the receipt includes:
- Transaction ID and hash
- Execution status and result summary
- State root before/after
- L2/L3 validation status
- Signer key ID and signature

Example output:
```
=== Scenario: l2_invalid (doctrine mode) ===
Vertical: gates
Narrative: Envelope with forged L2 signature: rejected in consensus/notary (L2 enforced), accepted in doctrine (L2 audited only)
Evidence: L2=true (key=797c07dc...), L3=false, signer=797c07dc...
Result: ACCEPTED
Receipt:
  Transaction ID: abc123...
  Transaction Hash: def456...
  Status: EXECUTION_STATUS_COMPLETED
  Result Summary: mock execution succeeded
  State Root Before: abc123def456
  State Root After: abc123def456
  Signer Key ID: 797c07dc...
  Signature: deadbeef...
  Gateway Signed: false
  L2 Status: L2_STATUS_REQUIRED_VALID
  L3 Status: L3_STATUS_REQUIRED_VALID
  Executed At: 1716624000000
```

## Viewing the Local Ledger and Audit Vault

The audit vault persists a git ledger and SQLite database at `.g8e/test-vault/{timestamp}-{test-name}/` for post-test inspection. The test logs the vault path when created:

```
Test vault created at: /home/bob/g8e/.g8e/test-vault/20260524-120000-TestScenarios
```

### Using the CLI to Inspect Test Vaults

The g8e CLI provides commands to inspect test vaults without requiring a running Operator:

```bash
# List all available test vaults
./g8e test review --list

# Show action receipts from a specific vault
./g8e test review --vault-path .g8e/test-vault/20260524-120000-TestScenarios --receipts

# Show git ledger from a specific vault
./g8e test review --vault-path .g8e/test-vault/20260524-120000-TestScenarios --ledger

# Execute custom SQL queries on the vault database
./g8e test review --vault-path .g8e/test-vault/20260524-120000-TestScenarios --query "SELECT * FROM action_receipts;"

# Inspect vault structure (list tables)
./g8e test review --vault-path .g8e/test-vault/20260524-120000-TestScenarios

# Clean old vaults (older than N days)
./g8e test review --clean-old 7

# Clean all vaults
./g8e test review --clean
```

### Manual Inspection

You can also manually inspect the vault using standard tools:

```bash
# Navigate to the test vault directory
cd .g8e/test-vault/{timestamp}-{test-name}

# View git log of audit events
cd ledger
git log --oneline

# View a specific commit's details
git show <commit-hash>

# View the full diff of a commit
git show <commit-hash> --stat

# Query the SQLite database directly
sqlite3 audit_vault.db ".tables"
sqlite3 audit_vault.db "SELECT * FROM action_receipts;"
```

The ledger contains all audit events written during the test, including transaction receipts and state changes. This allows detailed inspection of the audit trail after test completion.

Note: The test database uses in-memory storage that is cleaned up after test completion, but the audit vault ledger directory is preserved for manual inspection.

## The Theater

Under `-v`, the test prints a full verification trace including receipt details. The same test that gates the pipeline is the demo - no duplicate maintenance.
