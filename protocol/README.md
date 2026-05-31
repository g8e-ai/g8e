# g8e Protocol

Official protocol library for the g8e zero-trust execution platform. Provides typed protobuf definitions, constants, models, and workload identity helpers for building g8e-compatible clients and services.

## Installation

### Go

```bash
go get github.com/g8e-ai/g8e/protocol
```

### Python

```bash
pip install g8e-protocol
```

## Components

### Go Package

- **proto/g8e/common/v1**: Core governance envelope and metadata types (L1/L2/L3)
- **proto/g8e/operator/v1**: Operator service definitions and payloads
- **proto/g8e/pubsub/v1**: Pub/sub message types
- **workload_identity.go**: SPIFFE workload identity generation and validation

### Python Package

- **g8e_protocol.constants**: Runtime loader for JSON protocol constants
- **g8e_protocol.models**: Pydantic models for protocol data structures
- **g8e_protocol.generated**: Protobuf-generated Python code (via buf)

## Usage

### Go - Workload Identity

```go
package main

import (
    "fmt"
    "github.com/g8e-ai/g8e/protocol"
)

func main() {
    wid := protocol.NewWorkloadIdentity()
    
    // Generate operator SPIFFE ID
    spiffeID := wid.OperatorSPIFFEID("org-123", "op-456", "session-789")
    fmt.Println(spiffeID) // spiffe://g8e.local/operator/org-123/op-456/session-789
    
    // Validate SPIFFE ID
    if wid.MatchesOperator(spiffeID, "org-123", "op-456", "session-789") {
        fmt.Println("Valid operator identity")
    }
}
```

### Go - Governance Envelope

```go
package main

import (
    "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
    "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

func main() {
    envelope := &commonv1.GovernanceEnvelope{
        Id: "txn-123",
        EventType: "g8e.v1.operator.command.requested",
        OperatorId: "op-456",
        OperatorSessionId: "session-789",
        Governance: &commonv1.GovernanceMetadata{
            L1: &commonv1.L1Metadata{
                Validated: true,
            },
        },
    }
    
    cmd := &operatorv1.CommandRequested{
        Command: "ls -la",
        ExecutionId: "exec-123",
        Justification: "List directory contents",
    }
}
```

### Python - Constants

```python
from g8e_protocol.constants import EVENTS, STATUS, COLLECTIONS, ComponentName

# Access protocol constants
print(EVENTS["command"]["requested"])
print(COLLECTIONS["operators"])
print(ComponentName.G8EO)
```

### Python - Models

```python
from g8e_protocol.models import RequestContext, BoundOperator

context = RequestContext(
    web_session_id="web-123",
    user_id="user-456",
    source_component=ComponentName.CLIENT,
    bound_operators=[
        BoundOperator(operator_id="op-789", operator_session_id="session-abc")
    ]
)
```

## Protocol Versioning

This package follows semantic versioning. Major version changes indicate breaking protocol changes. Minor version changes add new protocol features. Patch version changes include bug fixes and non-breaking enhancements.

## License

Apache License 2.0 - see LICENSE file for details.

## Contributing

Protocol changes require coordination across all g8e components. Submit protocol change proposals via GitHub issues with clear justification and impact analysis.

## Support

For protocol questions and support, open a GitHub issue or visit https://github.com/g8e-ai/g8e
