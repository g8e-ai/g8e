# Constants System

## Overview

The g8e constants system maintains canonical constant definitions across the platform. Constants are defined in Go source files in `internal/constants/` and referenced by JSON schemas in `protocol/constants/` for protocol documentation and external consumers.

## Constant Categories

### Database Collections (`collections.go`)
Canonical collection names for the operator's embedded SQLite database:
- `CollectionUsers`, `CollectionWebSessions`, `CollectionOperatorSessions`, `CollectionCLISessions`
- `CollectionLoginAudit`, `CollectionAuthAdminAudit`, `CollectionAccountLocks`
- `CollectionOrganizations`, `CollectionOperators`, `CollectionOperatorUsage`
- `CollectionCases`, `CollectionInvestigations`, `CollectionTasks`
- And additional collections for agent activity, app policies, bound sessions, device links, etc.

### Event Types (`events.go`)
Typed event identifiers for the pub/sub system:
- App lifecycle events: `EventAppCaseCreated`, `EventAppCaseUpdated`, `EventAppTaskCreated`
- Operator events: `EventOperatorHeartbeat`, `EventOperatorCommandRequested`, `EventOperatorCommandCompleted`
- Governance events: `EventGovernanceEnvelopeReceived`, `EventGovernanceEnvelopeCommitted`
- Auth events: `EventAuthLoginRequested`, `EventAuthPasskeyRegistered`
- And hundreds of additional event types across all subsystems

### API Paths (`api_paths.go`)
HTTP route paths for the Gateway REST API:
- Authentication endpoints: `/api/v1/auth/login`, `/api/v1/auth/passkey/register`
- Operator management: `/api/v1/operators`, `/api/v1/operators/{id}/bind`
- Governance: `/api/v1/governance/envelope`, `/api/v1/governance/approve`
- And other API route definitions

### Channels (`channels.go`)
Pub/sub channel names for inter-component communication:
- Command channels: `ChannelOperatorCommand`, `ChannelOperatorResult`
- Heartbeat: `ChannelOperatorHeartbeat`
- Governance: `ChannelGovernanceEnvelope`, `ChannelGovernanceApproval`
- And other channel definitions

### Intents (`intents.go`)
Intent classification values for governance posture:
- Cloud provider intents: `IntentAWSRead`, `IntentAWSPowerUser`, `IntentGCPRead`
- Kubernetes intents: `IntentK8sRead`, `IntentK8sWrite`
- System intents: `IntentSystemRead`, `IntentSystemWrite`
- And other intent classifications

### Status Enums (`status.go`)
Internal enumeration constants:
- `UserRole`: `UserRoleUnspecified`, `UserRoleAdmin`, `UserRoleOperator`, `UserRoleUser`
- `OperatorStatus`: `OperatorStatusUnspecified`, `OperatorStatusOnline`, `OperatorStatusOffline`
- `ExecutionStatus`: `ExecutionStatusUnspecified`, `ExecutionStatusExecuting`, `ExecutionStatusCompleted`, `ExecutionStatusFailed`
- And other status enums

### Headers (`headers.go`)
HTTP header names used across the platform:
- Authentication headers: `HeaderAuthorization`, `HeaderXOperatorID`
- Session headers: `HeaderXWebSessionID`, `HeaderXOperatorSessionID`
- And other header constants

### Additional Constant Files
- `paths.go` - Filesystem paths for operator data, certificates, ledger
- `ports.go` - Network port numbers for Gateway, Operator services
- `exit_codes.go` - Process exit code constants
- `network.go` - Network-related constants
- `output.go` - Output format constants
- `mappings.go` - Mapping structures for protocol translation
- `auth.go` - Authentication-related constants
- `platform.go` - Platform-specific constants
- `prompts.go` - Prompt template identifiers
- `senders.go` - Message sender identifiers
- `pubsub.go` - Pub/sub protocol constants
- `document_ids.go` - Document ID prefixes
- `kv_keys.go` - Key-value store key patterns
- `env_vars.go` - Environment variable names
- `timestamp.go` - Go-specific format strings
- `agents.json` - Agent persona details (JSON reference)

## JSON Reference Files

The `protocol/constants/` directory contains JSON files that serve as reference documentation and external protocol definitions. These files mirror the Go constants and are used for:
- Protocol documentation generation
- External client SDK generation
- Cross-language protocol compatibility

Key JSON files:
- `collections.json` - Collection name definitions
- `events.json` - Event type definitions
- `channels.json` - Channel name definitions
- `intents.json` - Intent classification definitions
- `status.json` - Status enum definitions
- `headers.json` - HTTP header definitions
- `api_paths.json` - API path definitions
- And other reference JSON files

## Protocol Generation

### Generate Protocol Artifacts

```bash
make generate
```

This command generates Go Protobuf code from `.proto` files using Buf.

### Generate Python Protocol

```bash
make proto-python
```

Generates Python Protobuf code for the Python protocol SDK.

## CI Integration

Constants are validated in CI via the `G8E_STRICT_CONSTANTS_LINT` environment variable. When set, the test suite enforces that all constants are properly defined and referenced.

## Adding New Constants

1. **Add the constant** to the appropriate Go file in `internal/constants/`
2. **Update the corresponding JSON file** in `protocol/constants/` if the constant is part of the public protocol
3. **Run tests** to verify the constant is properly integrated
4. **Commit both** the Go source file and any updated JSON reference files

