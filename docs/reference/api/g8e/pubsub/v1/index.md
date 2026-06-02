# PubSub Service Documentation

The pubsub service implements the operator-side command dispatch and response handling for the g8e platform. The service receives inbound commands from the gateway via the pub/sub channel and dispatches them to specialized service handlers.

## Service Architecture

The `PubSubCommandService` in `internal/services/pubsub/pubsub_commands.go` acts as the central dispatcher. It maintains a registry of event type handlers and routes inbound `PubSubCommandMessage` instances to the appropriate first-class service.

### Core Services

The pubsub service coordinates the following specialized handlers:

- **AuditService** (`internal/services/pubsub/audit_service.go`) - Records LFAA audit events for user messages, AI messages, and direct terminal commands to the audit vault
- **CommandService** (`internal/services/pubsub/command_service.go`) - Handles command execution requests, cancellation requests, and periodic status updates during long-running commands
- **HeartbeatService** (`internal/services/pubsub/heartbeat_service.go`) - Builds heartbeat payloads, handles heartbeat requests, and manages the automatic heartbeat scheduler
- **FileOpsService** (`internal/services/pubsub/file_ops_service.go`) - Processes file edit, filesystem list, filesystem grep, and filesystem read requests
- **HistoryService** (`internal/services/pubsub/history_service.go`) - Handles log retrieval, execution history, file history, file restore, and file diff requests
- **PortService** (`internal/services/pubsub/port_service.go`) - Processes port connectivity check requests

### Message Flow

Inbound messages follow this sequence:

1. The `PubSubCommandService` receives a `PubSubCommandMessage` from the pub/sub client
2. The message contains an `EventType` field that identifies the operation
3. The dispatcher routes the message to the registered handler for that event type
4. The handler unmarshals the protobuf payload from `msg.Payload`
5. The handler executes the operation and publishes a result via the `ResultsPublisher`

### Governance Integration

The pubsub service integrates with the five-layer governance pipeline:

- **L1 Doctrine** - Technical bedrock validation via `governance.L1Doctrine`
- **L2 Consensus** - Multi-agent signature verification via `governance.L2Consensus`
- **L3 Notary** - Human-in-the-loop authorization via `governance.L3Notary`
- **L4 Warden** - Pre-dispatch verification gating via `governance.L4Warden`
- **L5 Actuator** - Isolated boundary tool dispatch via `governance.L5Actuator`

The governance services are initialized in `initializeGovernance` and require a `SignerStore`, `ReplayStore`, and `StateRootProvider` to operate correctly.

## Protocol Schema

The pubsub protocol uses protobuf messages defined in `protocol/proto/g8e/pubsub/v1/pubsub.proto`.

### PubSubMessage

The `PubSubMessage` message carries outbound commands from the gateway to the operator.

| Field | Type | Description |
| ----- | ---- | ----------- |
| action | string | The action type identifier |
| channel | string | The pub/sub channel name |
| data | bytes | The protobuf-encoded payload |

### PubSubEvent

The `PubSubEvent` message carries event notifications from the operator to the gateway.

| Field | Type | Description |
| ----- | ---- | ----------- |
| type | string | The event type identifier |
| channel | string | The pub/sub channel name |
| pattern | string | The subscription pattern |
| data | bytes | The protobuf-encoded payload |

## Event Types

The pubsub service handles the following event types defined in the constants system:

- `Event.Operator.HeartbeatRequested` - Handled by `HeartbeatService.HandleRequest`
- `Event.Operator.Command.Requested` - Handled by `CommandService.HandleExecutionRequest`
- `Event.Operator.Command.CancelRequested` - Handled by `CommandService.HandleCancelRequest`
- `Event.Operator.FileEdit.Requested` - Handled by `FileOpsService.HandleFileEditRequest`
- `Event.Operator.FsList.Requested` - Handled by `FileOpsService.HandleFsListRequest`
- `Event.Operator.FsRead.Requested` - Handled by `FileOpsService.HandleFsReadRequest`
- `Event.Operator.FsGrep.Requested` - Handled by `FileOpsService.HandleFsGrepRequest`
- `Event.Operator.PortCheck.Requested` - Handled by `PortService.HandlePortCheckRequest`
- `Event.Operator.FetchLogs.Requested` - Handled by `HistoryService.HandleFetchLogsRequest`
- `Event.Operator.FetchHistory.Requested` - Handled by `HistoryService.HandleFetchHistoryRequest`
- `Event.Operator.FetchFileHistory.Requested` - Handled by `HistoryService.HandleFetchFileHistoryRequest`
- `Event.Operator.RestoreFile.Requested` - Handled by `HistoryService.HandleRestoreFileRequest`
- `Event.Operator.ShutdownRequested` - Handled by `PubSubCommandService.handleShutdownRequest`
- `Event.Operator.Audit.UserMsg` - Handled by `AuditService.HandleUserMsgRequest`
- `Event.Operator.Audit.AIMsg` - Handled by `AuditService.HandleAIMsgRequest`
- `Event.Operator.Audit.Command` - Handled by `AuditService.HandleDirectCmdRequest`

## Constants

Field constants for pubsub messages are defined in `internal/constants/pubsub.go` and `protocol/constants/pubsub.json`:

- `PubSubFieldAction` - "action"
- `PubSubFieldChannel` - "channel"
- `PubSubFieldData` - "data"
- `PubSubFieldMessage` - "message"
- `PubSubFieldPattern` - "pattern"
- `PubSubFieldType` - "type"
- `PubSubFieldSender` - "sender"
