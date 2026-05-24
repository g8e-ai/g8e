# g8ee (Ensemble) Internal Structure Codemap

## Overview

g8ee is a Python-based reference implementation of a g8e-compliant agentic ensemble. It implements a ReAct loop over a hierarchy of AI agents, reaches L2 consensus via heterogeneous models, forms and signs GovernanceEnvelope, and submits envelopes to the Gateway for admission.

```text
services/g8ee/
├── app/                         # Main application source
│   ├── __init__.py
│   ├── main.py                  # FastAPI application entry point
│   ├── dependencies.py          # Dependency injection setup
│   ├── errors.py                # Custom error definitions
│   ├── logging.py               # Logging configuration
│   │
│   ├── clients/                 # External service clients
│   │   ├── http_client.py       # HTTP client for external services
│   │   ├── db_client.py         # Database client
│   │   ├── pubsub_client.py     # Pub/sub client
│   │   ├── blob_client.py       # Blob storage client
│   │   └── kv_cache_client.py   # Key-value cache client
│   │
│   ├── constants/               # Application constants
│   │   └── paths.py             # Path resolution (from protocol/paths.json)
│   │
│   ├── db/                      # Database models and session management
│   │   ├── base.py              # SQLAlchemy base and session
│   │   └── models.py            # SQLAlchemy ORM models
│   │
│   ├── llm/                     # LLM integration layer
│   │   ├── llm_types.py         # LLM type definitions
│   │   └── providers/           # LLM provider implementations
│   │       ├── openai.py        # OpenAI provider
│   │       ├── anthropic.py     # Anthropic provider
│   │       └── ...
│   │
│   ├── middleware/              # FastAPI middleware
│   │   └── context.py           # RequestContext middleware
│   │
│   ├── models/                   # Pydantic data models
│   │   ├── agents/              # Agent-specific models
│   │   ├── personas/            # Agent persona definitions
│   │   └── ...                  # Other domain models
│   │
│   ├── prompts_data/            # Prompt templates and system messages
│   │   ├── core/                # Core prompt templates
│   │   ├── modes/               # Mode-specific prompts
│   │   ├── system/              # System messages
│   │   ├── tools/               # Tool descriptions
│   │   └── tribunal/            # Tribunal-specific prompts
│   │
│   ├── proto/                   # Generated protobuf Python code
│   │
│   ├── routers/                 # FastAPI route handlers
│   │   ├── chat_router.py       # Chat/triage endpoints
│   │   ├── internal_router.py   # Internal admin routes
│   │   ├── health_router.py     # Health check endpoints
│   │   └── ...                  # Other route modules
│   │
│   ├── security/                # Security utilities
│   │   └── ...
│   │
│   ├── services/                # Business logic layer
│   │   ├── ai/                  # AI/LLM orchestration
│   │   │   ├── auditor_service.py    # L2 signature generation
│   │   │   ├── chat_pipeline.py       # Chat orchestration
│   │   │   ├── generator.py           # LLM generation
│   │   │   ├── tribunal/              # Tribunal consensus (multi-stage)
│   │   │   │   ├── stages/            # Tribunal stages
│   │   │   │   │   ├── generation.py  # Generation stage
│   │   │   │   │   ├── auditor.py     # Auditor stage
│   │   │   │   │   └── warden.py      # Warden stage
│   │   │   │   └── emitter.py         # Tribunal event emitter
│   │   │   ├── agent.py               # Agent orchestration
│   │   │   ├── agent_tool_loop.py     # Agent tool execution loop
│   │   │   ├── tool_service.py        # Tool service
│   │   │   └── tools/                 # Tool implementations
│   │   │
│   │   ├── auth/                 # Authentication and authorization
│   │   │   └── ...
│   │   │
│   │   ├── cache/                # Caching layer
│   │   │   └── ...
│   │   │
│   │   ├── data/                 # Data access services
│   │   │   └── ...
│   │   │
│   │   ├── infra/                # Infrastructure services
│   │   │   └── ...
│   │   │
│   │   ├── investigation/        # Investigation management
│   │   │   └── investigation_service.py
│   │   │
│   │   ├── operator/             # Operator integration
│   │   │   ├── operator_data_service.py  # Operator data service
│   │   │   ├── operator_session_service.py # Operator session management
│   │   │   ├── command_service.py        # Command execution service
│   │   │   ├── execution_service.py       # Execution service
│   │   │   ├── file_service.py            # File operations service
│   │   │   ├── filesystem_service.py      # Filesystem service
│   │   │   ├── heartbeat_service.py       # Heartbeat monitoring
│   │   │   ├── approval_service.py        # Approval service
│   │   │   ├── intent_service.py          # Intent management
│   │   │   ├── pubsub_service.py          # Pub/sub service
│   │   │   └── stream_executor.py         # Streaming executor
│   │   │
│   │   └── storage/              # Storage abstraction
│   │       └── ...
│   │
│   └── utils/                    # Utility functions
│       └── ...
│
├── config/                       # Configuration files
│   ├── settings.json            # Application settings
│   ├── paths.json               # Path mappings (from protocol/)
│   └── ...
│
├── reports/                      # Report generation
│   └── evals/                   # Evaluation reports
│
├── tests/                        # Test suite
│   ├── conftest.py              # Pytest configuration
│   ├── fakes/                   # Fake implementations for testing
│   ├── integration/             # Integration tests
│   │   └── invariants/          # Invariant tests
│   ├── e2e/                     # End-to-end tests
│   └── ...                      # Other test modules
│
├── .venv/                        # Python virtual environment
├── pyproject.toml               # Python project configuration
├── requirements.txt             # Python dependencies
└── Makefile                     # Build targets
```

## Agent Hierarchy

g8ee implements a multi-agent hierarchy with specialized roles:

### Triage Agent
- **Purpose**: Fast-path routing and complexity classification
- **Location**: `services/ai/chat_pipeline.py`
- **Responsibilities**:
  - Classify request complexity (simple vs complex)
  - Route to appropriate agent path
  - Handle benign diagnostic commands (auto-approval path)

### Sage Agent
- **Purpose**: Primary reasoner; proposes actions but cannot execute
- **Location**: `services/ai/generator.py`
- **Responsibilities**:
  - Generate proposed actions
  - Provide reasoning and context
  - Never directly executes mutations

### Tribunal (5-seat consensus)
- **Purpose**: L2 consensus via heterogeneous model ensemble
- **Location**: `services/ai/tribunal/` (multi-stage pipeline)
- **Responsibilities**:
  - Independent validation by 5 different models
  - k-of-n consensus on proposed actions
  - Ed25519 signature generation (L2)
- **Models**: Heterogeneous ensemble (Anthropic, OpenAI, local, etc.)

### Warden
- **Purpose**: Heuristic circuit breaker
- **Location**: `services/ai/tribunal/stages/warden.py`
- **Responsibilities**:
  - Two-strike circuit breaker
  - Heuristic risk assessment
  - Block suspicious patterns

### Auditor
- **Purpose**: History grounding and L2 signature generation
- **Location**: `services/ai/auditor_service.py` and `services/ai/tribunal/stages/auditor.py`
- **Responsibilities**:
  - Ground proposals in historical context
  - Generate Ed25519 signatures for consensus
  - Verify proposal alignment with doctrine

### Nemesis
- **Purpose**: Embedded adversary (recorded, never executed)
- **Location**: `services/ai/`
- **Responsibilities**:
  - Generate adversarial proposals for testing
  - Recorded in audit trail but never executed
  - Used for robustness validation

## Core Service Layer Breakdown

### `services/ai/chat_pipeline.py`
- **Purpose**: Orchestrate the full chat/triage pipeline
- **Flow**:
  1. Receive user request
  2. Triage classification
  3. Route to Sage or fast path
  4. Tribunal consensus (if complex)
  5. Warden validation
  6. Auditor signature
  7. Envelope formation
  8. Submit to Gateway

### `services/ai/generator.py`
- **Purpose**: LLM generation and response handling
- **Components**:
  - LLM provider abstraction
  - Streaming response handling
  - Tool call parsing
  - Generation configuration

### `services/ai/tribunal/`
- **Purpose**: Multi-model consensus orchestration (multi-stage pipeline)
- **Components**:
  - Model ensemble management
  - Parallel proposal generation
  - Consensus verification
  - k-of-n voting logic

### `services/ai/auditor_service.py`
- **Purpose**: L2 signature generation and history grounding
- **Components**:
  - Historical context retrieval
  - Proposal validation
  - Ed25519 signature generation
  - Reputation stake verification

### `services/operator/stream_executor.py`
- **Purpose**: Streaming execution of tool calls
- **Components**:
  - Async tool execution
  - Result streaming
  - Error handling
  - Timeout management

### `services/investigation/investigation_service.py`
- **Purpose**: Investigation lifecycle management
- **Components**:
  - Investigation creation and updates
  - Memory association
  - Event publishing
  - Context assembly

### `services/operator/operator_data_service.py`
- **Purpose**: Operator data management and communication
- **Components**:
  - Envelope formation
  - Gateway submission
  - Receipt handling
  - State synchronization

## Protocol Integration

### `app/proto/`
- **Purpose**: Generated protobuf Python code
- **Source**: Generated from `protocol/proto/`
- **Usage**: Envelope formation, receipt parsing, type definitions

### `app/constants/paths.py`
- **Purpose**: Path resolution from protocol constants
- **Source**: Reads from `protocol/constants/paths.json`
- **Usage**: Runtime path resolution for PKI, runtime, secrets

## Data Models

### `app/models/`
- **Purpose**: Pydantic data models for request/response validation
- **Categories**:
  - `agents/` - Agent-specific models
  - `personas/` - Agent persona definitions
  - Domain models for investigations, cases, etc.

### `app/db/models.py`
- **Purpose**: SQLAlchemy ORM models for persistence
- **Usage**: Investigation store, memory store, etc.

## Routing Layer

### `app/routers/`
- **Purpose**: FastAPI route handlers
- **Key Routes**:
  - `chat_router.py` - Chat/triage endpoints
  - `internal_router.py` - Internal admin routes
  - `health_router.py` - Health check endpoints
  - Other domain-specific routers

## Middleware

### `app/middleware/context.py`
- **Purpose**: RequestContext injection
- **Components**:
  - Request context extraction
  - User/session identification
  - Component source tracking
  - Context propagation to services

## Configuration

### `config/`
- **Purpose**: Application configuration
- **Files**:
  - `settings.json` - Application settings
  - `paths.json` - Path mappings (from protocol/)
- **Environment Variables**:
  - `G8E_PROTOCOL_DIR` - Protocol directory path
  - `G8E_PKI_DIR` - PKI directory path
  - `G8E_RUNTIME_DIR` - Runtime directory path

## Testing Structure

### Unit Tests
- **Location**: `tests/`
- **Focus**: Individual service logic
- **Fixtures**: `tests/fakes/` - Fake implementations

### Integration Tests
- **Location**: `tests/integration/`
- **Focus**: Service integration points
- **Invariant Tests**: `tests/integration/invariants/`

### End-to-End Tests
- **Location**: `tests/e2e/`
- **Focus**: Full request lifecycle
- **Dependencies**: Requires running Operator and Gateway

## Entry Points

### Main Application
- **Path**: `app/main.py`
- **Framework**: FastAPI
- **Startup**:
  1. Load configuration
  2. Initialize dependencies
  3. Connect to Operator
  4. Start HTTP server

## Critical Data Paths

### Chat Request Flow
```text
HTTP Request → routers/chat_router.py → services/ai/chat_pipeline.py
→ Triage → Sage → Tribunal → Warden → Auditor
→ Envelope formation → services/operator/operator_data_service.py
→ Gateway submission → Receipt handling → Response
```

### Investigation Flow
```text
Request → services/investigation/investigation_service.py
→ Investigation creation → Memory association
→ Event publishing → Operator sync → Response
```

### L2 Signature Flow
```
Tribunal consensus → services/ai/tribunal/stages/auditor.py
→ services/ai/auditor_service.py
→ Historical context retrieval → Proposal validation
→ Ed25519 signature generation → Envelope attachment
```

## Storage Layout

```
.g8e/
├── runtime/                     # Runtime state (if any)
└── investigations/              # Investigation storage (if local)
```

## Build Targets

```makefile
make build-g8ee          # Build/gather dependencies
make test-g8ee           # Run g8ee unit tests
make lint-g8ee           # Run linters
make clean-g8ee          # Clean build artifacts
```

## Key Invariants

1. **Protocol consumption**: All protobuf schemas from `protocol/proto/`
2. **No direct execution**: g8ee never executes mutations directly
3. **L2 consensus**: All mutations must pass Tribunal consensus
4. **Envelope formation**: All mutations wrapped in GovernanceEnvelope
5. **Gateway submission**: All envelopes submitted via Gateway
6. **Context propagation**: RequestContext propagated through all layers
7. **Dependency injection**: Services use DI for testability
