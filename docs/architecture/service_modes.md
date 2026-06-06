# Service Modes

The g8e platform operates in two distinct modes: **Gateway Mode** and **Outbound Mode**. Each mode uses a different set of services, with some services shared across both modes.

## Mode-Specific Services

### Gateway Mode
Gateway mode is the operator platform mode that provides the central Policy Decision Point (PDP).

- **`GatewayModeService`** (`internal/services/gateway/gateway_service.go`)
  - Top-level orchestrator for gateway mode
  - Manages persistence, messaging, authentication, and enrollment
  - Only used in gateway mode
  - Dependencies: `CanonicalDBService`, `PubSubBroker`, `AuthService`, `PKIAuthority`, etc.

### Outbound Mode
Outbound mode is the host-side Policy Execution Point (PEP) and MCP server mode.

- **`G8eoService`** (`internal/services/g8eo.go`)
  - Top-level orchestrator for outbound mode
  - Manages execution, pub/sub dispatch, and governance layers
  - Only used in outbound mode
  - Dependencies: `BootstrapService`, `ExecutionService`, `PubSubCommandService`, etc.

## Shared Services

### `mcp.GatewayService`
- **Package**: `internal/services/mcp/gateway.go`
- **Purpose**: Handles MCP/A2A protocol translation and downstream dispatch
- **Usage**:
  - Gateway mode: Created by `GatewayModeService` as field `mcpGateway`
  - Outbound mode: Used by `PubSubCommandService` as field `mcpGateway`
- **Status**: Truly polymorphic - same implementation used in both modes
- **Dependencies**: `Responder`, `SuspendedTransactionStore`, `FieldPathRegistry`, etc.

### `CanonicalDBService`
- **Package**: `internal/services/gateway/gateway_db.go`
- **Purpose**: Unified SQLite persistence layer and canonical state root database
- **Usage**:
  - Gateway mode: Full database service for all gateway operations
  - Outbound mode: Used only for canonical state root calculation (line 125 in g8eo.go)
- **Status**: Reused with different scope per mode
- **Dependencies**: `sqliteutil.DB`, `AuditVaultService`, keystore

## Rationale for Shared vs Mode-Specific Services

### Shared Services
Services are shared across modes when:
1. They implement protocol handling that is mode-agnostic (e.g., `mcp.GatewayService`)
2. They provide canonical state that must be consistent across modes (e.g., `CanonicalDBService` for state roots)
3. The implementation is truly polymorphic and can operate in both contexts without modification

### Mode-Specific Services
Services are mode-specific when:
1. They orchestrate mode-specific workflows (e.g., `GatewayModeService` for gateway operations, `G8eoService` for outbound execution)
2. They depend on mode-specific infrastructure (e.g., `PubSubBroker` only exists in gateway mode)
3. Their purpose is fundamentally different between modes

## Naming Conventions

To avoid confusion between mode-specific and shared services:

- **Mode-specific services** use explicit mode names in their type names (e.g., `GatewayModeService`, `G8eoService`)
- **Shared services** use descriptive names that reflect their canonical purpose (e.g., `CanonicalDBService`, `mcp.GatewayService`)
- Package prefixes help distinguish services with similar names (e.g., `mcp.GatewayService` vs `gateway.GatewayModeService`)

## Cross-Mode Service Usage

When reading code that references a service, check:
1. The package prefix to identify which service is being used
2. The context (gateway mode vs outbound mode) to understand the service's scope
3. The service's documentation comments for mode-specific usage notes

For example:
- `mcp.GatewayService` is shared and used identically in both modes
- `CanonicalDBService` is shared but with different scope (full service in gateway mode, state root only in outbound mode)
- `GatewayModeService` is gateway-mode only and does not exist in outbound mode

## Migration History

The following services were renamed to clarify mode context:

- `gateway.GatewayService` → `gateway.GatewayModeService` (mode-specific)
- `gateway.GatewayDBService` → `gateway.CanonicalDBService` (shared, reflects canonical purpose)

See `docs/devs/codemap.md` for the complete service dependency tree with mode annotations.
