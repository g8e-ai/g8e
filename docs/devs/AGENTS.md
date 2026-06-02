# g8e — AI Coding Agent Directives

> **Scope**: This document is the authoritative code quality contract for all AI-assisted development on the g8e platform. Every instruction here is non-negotiable. g8e is a production enterprise security platform — treat every change accordingly.

---

## Part I — Affirmative Directives

These are the rules. Follow them unconditionally. If a task cannot be completed while following them, say so before writing code.

---

### 1. Configuration & Constants

**Always** source configuration values from the established constants system.
- Port numbers → `internal/constants/ports.go` and `protocol/constants/ports.json`
- Collection names → `internal/constants/collections.go` and `protocol/constants/collections.json`
- Event names → `protocol/constants/events.json`
- Channel prefixes → `protocol/constants/channels.json`
- Doctrine rules → `protocol/constants/doctrine/doctrine_registry.json`

**Always** add new constants to the correct existing constants file. If no file exists for the category, create one under `internal/constants/` or `protocol/constants/` following the existing pattern, then register it.

**Always** pass configuration through the established config struct hierarchy. Trace the existing call chain before adding a parameter.

**Always** use the existing `paths.json` (at `internal/cli/config/paths.json`) and `config` loader (defined in `internal/config/config.go`) for file paths. The config loader is the single source of truth for runtime paths.

---

### 2. Architecture & Layer Boundaries

**Always** respect the five-layer execution boundary. L1 → L2 → L3 → L4 → L5 is a strict sequential pipeline. Each layer has one file under `internal/services/governance/`, one responsibility, and one call surface.

**Always** place governance logic in its correct layer file under `internal/services/governance/`:
- Static pattern analysis → `l1_doctrine.go`
- Signature verification → `l2_consensus.go`
- Human presence proof → `l3_notary.go`
- Pre-dispatch integrity → `l4_warden.go`
- Execution and receipt → `l5_actuator.go`

**Always** enforce fail-closed behavior. When a verification step is uncertain, reject. Default to rejection, never to permissiveness.

**Always** write the audit record before the side effect. The `AuditVaultService` write comes first. If the vault write fails, abort execution.

**Always** keep g8e Gateway (PDP) and g8e Operator (PEP) logic in their correct respective packages. g8e Gateway logic belongs in `internal/services/gateway/`. g8e Operator logic belongs in `internal/services/mcp/` and its governance pipeline. Cross-contamination is a protocol violation.

**Always** route new mutation types through the `GovernanceEnvelope`. No mutation reaches host state without a `transaction_hash`-verified envelope passing the full gauntlet.

---

### 3. The GovernanceEnvelope Contract

**Always** compute the `transaction_hash` as `SHA-256(canonical_fields)` and enforce `id == transaction_hash`. This is checked by `L4Warden` and must be pre-computed correctly at construction time.

**Always** embed session identity inside the envelope body. `operator_session_id`, `cli_session_id`, and `web_session_id` are envelope fields — they are never implicit or ambient context.

**Always** include `nonce`, `expires_at`, and `state_merkle_root` when constructing envelopes. Omitting any of these produces an envelope that L4 will reject.

**Always** use canonical JSON (`protojson`) for all wire formats. Binary protobuf is internal storage only.

**Always** sign receipts with the host-unique Ed25519 key. An `ActionReceipt` without a valid signature is not an `ActionReceipt`.

---

### 4. Identity & mTLS

**Always** use SPIFFE URI SANs for workload identity following the formats defined in `protocol/workload_identity.go`. Do not invent new identity formats.

**Always** enforce `tls.RequireAndVerifyClientCert` on port `8440`. The mTLS surface never downgrades.

**Always** check revocation on every handshake. Revocation is not a startup check — it is a per-connection check.

**Always** bind approvals to a `transaction_hash`, not a session. An L3 proof authorizes one exact transaction. It authorizes nothing else.

---

### 5. Error Handling

**Always** return typed, structured errors with context. Use `fmt.Errorf("component: action: %w", err)` wrapping throughout.

**Always** propagate the error code from the protocol error table (`ErrCodeInvalidEnvelope`, `ErrCodeHashMismatch`, etc.) when rejecting at a governance layer.

**Always** log the rejection reason and the transaction hash before returning a governance error. The audit trail must be complete even for rejected transactions.

**Always** treat a failed audit write as fatal. If the `AuditVaultService` cannot record an event, the operation must not proceed.

**Always** handle the zero-value case explicitly. A nil envelope, empty hash, or missing proof must produce a typed rejection, not a panic or silent pass-through.

---

### 6. Data Sovereignty

**Always** scrub sensitive data at the `L5Actuator` boundary before any output leaves the host. Use the scrubber in `internal/services/sovereignty/boundary.go`. Do not hand-roll token replacement inline.

**Always** split audit storage into Scrubbed Vault (AI-readable) and Raw Vault (human forensic). New audit write paths must go to both vaults with the correct scrubbing applied.

**Always** rehydrate tokens inside the `L5Actuator` only, at the instant of execution. Token rehydration never happens upstream of L5.

---

### 7. Binary & Dependency Hygiene

**Always** check that a new import is already present in `go.mod` before using it. The zero-dependency constraint is a hard architectural requirement.

**Always** prefer standard library over third-party for any new capability. If a third-party package is genuinely required, state the reason explicitly in the PR description before adding it.

**Always** ensure the g8e Node compiles to a single statically linked artifact (`CGO_ENABLED=0`). Nothing that breaks static compilation is acceptable.

---

### 8. Code Completeness

**Always** finish what you start in a single change. A function with a `TODO` body, an unreachable branch, or a stub return is a broken platform component, not a draft.

**Always** wire new code into the call graph. A new function that nothing calls is dead weight.

**Always** update the relevant constants, registry, or schema file when adding a new event type, error code, collection, or doctrine rule. The file that defines the value and the file that uses it must stay in sync.

**Always** write the error path before the happy path. Confirm rejection behavior is correct first.

---

### 9. Go Style

**Always** follow standard Go idioms: errors are values, interfaces are small, packages are flat, names are precise.

**Always** name variables for what they contain, not their type. `txHash` not `hashString`. `envelope` not `env` when it is a `GovernanceEnvelope`.

**Always** place the authoritative struct definition in one file and import it everywhere else. No type duplication across packages.

**Always** keep functions under 60 lines. If a function is growing past that, it has more than one responsibility — split it.

**Always** use table-driven tests for governance layer logic. Each layer has deterministic behavior; enumerate the cases.

---

## Part II — Prohibitions

These are absolute. There are no exceptions and no context in which these are acceptable.

---

### Configuration

- **Never** read from environment variables at runtime. No `os.Getenv`, no `os.LookupEnv`, no `godotenv`, no `viper` env binding. Configuration comes from the config loader only.
- **Never** hardcode a port number, path, collection name, or event string inline in business logic. It belongs in a constants file.
- **Never** add a feature flag, kill switch, or debug toggle as a new environment variable. Use the existing posture/config system.
- **Never** leave a magic number or magic string without a named constant.

---

### Architecture

- **Never** create a new code path that bypasses any governance layer. There is no "fast path", "trusted caller", or "internal shortcut" through the gauntlet.
- **Never** allow the g8e Gateway to execute host mutations directly. Execution is the g8e Operator's exclusive domain.
- **Never** allow the g8e Operator to communicate with an Identity Provider directly. JWT validation and JIT provisioning are g8e Gateway responsibilities only.
- **Never** duplicate governance logic between g8e Gateway and g8e Operator. The shared governance layer in `internal/services/governance/` is the single implementation.
- **Never** introduce a new protocol format alongside the `GovernanceEnvelope`. There is one canonical wire format.
- **Never** store user credentials or long-lived session tokens in the g8e Operator. Zero standing privileges is an invariant.

---

### Code Quality

- **Never** commit a function that has a `TODO`, `FIXME`, `HACK`, or `panic("not implemented")` in it.
- **Never** leave dead code in the repository. Commented-out functions, unreachable branches, and unused exported symbols must be removed, not preserved "just in case."
- **Never** create parallel implementations of the same behavior. If you need to change how something works, change the existing implementation. Do not write a v2 alongside v1.
- **Never** silently swallow an error with `_ =` or an empty `catch` equivalent unless the error is provably irrelevant and that fact is documented inline.
- **Never** use `interface{}` or `any` as a function parameter or return type in business logic. Use the correct typed struct.
- **Never** add a global variable outside of a `var` block in a `constants` or `config` package. No mutable global state in service logic.

---

### Dependencies & Build

- **Never** add a new third-party dependency without explicit approval. Go modules that do not already exist in `go.mod` are off-limits without a justified exception.
- **Never** introduce `init()` functions in service packages. Initialization order is explicit and intentional.
- **Never** use `reflect` in hot paths (governance layer execution). Reflection in L1 field-option scanning is pre-existing and intentional; new uses are not.
- **Never** write code that requires `CGO_ENABLED=1`. The g8e Node is statically compiled.

---

### Security

- **Never** log a matched credential value, secret, or token, even at debug level. The L1 pattern match confirms presence; it does not echo content.
- **Never** normalize unicode or transform encoding before running L1 patterns. L1 runs on raw bytes first, then normalized forms. Reversing this order defeats obfuscation detection.
- **Never** auto-approve an L3 notary requirement for a mutation without an explicit App Policy (stored in the `app_policies` collection) containing an entry in `AutoApproveIntents` for that exact action type. Absence of a proof is a rejection, not an implicit pass.
- **Never** extend `expires_at` or re-use a `nonce` to retry a failed transaction. Construct a new envelope.
- **Never** allow raw vault data to be returned through any API surface. The scrubbed vault is the AI-facing surface. The raw vault is human-only.

---

### Process

- **Never** make changes to Mermaid diagrams in documentation files unless explicitly asked. Diagrams are architecture artifacts, not prose.
- **Never** rename an exported symbol, protobuf field, or JSON key without verifying all callers. These are wire-format contracts.
- **Never** change a default port without updating `ports.go`, `ports.json`, and all documentation that references it.
- **Never** introduce a behavior difference between gateway mode and Operator mode for a shared governance layer. The five layers behave identically regardless of which g8e Node mode is running them.

---

## Quick Reference

| When you need to... | Go to... |
|---|---|
| Add a new event type | `protocol/constants/events.json` |
| Add a port | `internal/constants/ports.go` + `protocol/constants/ports.json` |
| Add a collection | `internal/constants/collections.go` + `protocol/constants/collections.json` |
| Add a doctrine rule | `protocol/constants/doctrine/doctrine_registry.json` |
| Add an error code | Protocol error table in `docs/architecture/g8e.md`, then the error constants file at `internal/responder/responder.go` |
| Add a workload identity type | `protocol/workload_identity.go` |
| Add a new mutation type | Define proto payload → add event constant → implement in `internal/services/governance/l5_actuator.go` |
| Add a config value | Config struct and loaders at `internal/config/config.go` → `internal/cli/config/paths.json` if it's a path |
| Add a native tool | Create tool file in `internal/services/mcp/` implementing `NativeTool` interface → add to `RegisterNativeTools()` in `native_tool_registry.go` |

---

*g8e — Lateralus Labs. Apache 2.0.*
