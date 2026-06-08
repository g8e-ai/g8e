# g8e-protocol

Official Python protocol library for the g8e zero-trust execution platform. Provides protocol constants, Pydantic models, and protobuf definitions for building g8e-compatible clients and services.

## Installation

```bash
pip install g8e-protocol
```

## Usage

### Constants

```python
from g8e.constants import EVENTS, STATUS, COLLECTIONS, ComponentName

# Access protocol constants
print(EVENTS["command"]["requested"])
print(COLLECTIONS["operators"])
print(ComponentName.G8EO)
```

### Models

```python
from g8e.models import RequestContext, BoundOperator

context = RequestContext(
    web_session_id="web-123",
    user_id="user-456",
    source_component=ComponentName.CLIENT,
    bound_operators=[
        BoundOperator(operator_id="op-789", operator_session_id="session-abc")
    ]
)
```

### Headers

```python
from g8e.constants import (
    HTTP_AUTHORIZATION_HEADER,
    WEB_SESSION_ID_HEADER,
    CLI_SESSION_ID_HEADER,
    OPERATOR_ID_HEADER,
)

# Use protocol header constants
headers = {
    HTTP_AUTHORIZATION_HEADER: "Bearer token",
    WEB_SESSION_ID_HEADER: "web-123",
    CLI_SESSION_ID_HEADER: "cli-456",
    OPERATOR_ID_HEADER: "op-789",
}
```

## Components

- **constants.py**: Runtime loader for JSON protocol constants (events, status, collections, headers, etc.)
- **models/**: Pydantic models for protocol data structures (RequestContext, BoundOperator, etc.)
- **generated/**: Protobuf-generated Python code (via buf)

## Protocol Versioning

This package follows semantic versioning. Major version changes indicate breaking protocol changes. Minor version changes add new protocol features. Patch version changes include bug fixes and non-breaking enhancements.

## License

Apache License 2.0 - see LICENSE file for details.

## Contributing

Protocol changes require coordination across all g8e components. Submit protocol change proposals via GitHub issues with clear justification and impact analysis.

## Support

For protocol questions and support, open a GitHub issue or visit https://github.com/g8e-ai/g8e
