# g8ee — g8e Ensemble

g8ee is the first g8e-compatible agentic ensemble: a reference AI reasoning system for g8e infrastructure operations. It is a first-party component of the g8e platform, shipped in-tree under `ensemble/` alongside the gateway/operator Go binary and the operator dashboard (g8ed). It connects to the g8e gateway over mTLS, submits signed `GovernanceEnvelope` transactions through the five-layer admission pipeline, and derives context from the hash-chained ledger. See [docs/architecture/ensemble.md](../docs/architecture/ensemble.md) for the architecture and role of the ensemble in the platform.

## Getting Started

### Prerequisites

- Python 3.12+ (per `pyproject.toml`; the `requires-python` field is the source of truth)
- A running g8e gateway and operator (see the repo-root [Getting Started guide](../docs/guides/getting_started.md) or bring up the whole stack with `docker compose up` from the repo root)

### Setup (development)

The ensemble depends on the in-tree `protocol/python/` package. Install the protocol package first, then the ensemble in editable mode:

```bash
# From the g8e repo root
python3 -m venv .venv
source .venv/bin/activate

# Install the in-tree g8e protocol package first
pip install protocol/python

# Install the ensemble with dev/test extras
pip install -e "ensemble[dev,test]"

# Or install from the ensemble requirements.txt
pip install -r ensemble/requirements.txt

# Copy environment template and configure
cp ensemble/.env.example ensemble/.env
# Edit ensemble/.env with your configuration

# Generate protobuf stubs from g8e protocol definitions
make -C ensemble proto
```

### Running the Application

The g8ee application requires connection to g8e operator services (DB, KV, PubSub, Blob, HTTP) and TLS certificates for secure communication.

**Prerequisites for running:**
- g8e operator services running and accessible
- TLS certificates mounted at expected paths (configured via BootstrapService)
- Valid operator session credentials

**Start the application:**

```bash
# Using uvicorn directly
uvicorn app.main:app --host 0.0.0.0 --port 8443 --reload

# Or run the module directly
python -m app.main
```

The application will:
- Start on port 8443 (HTTPS)
- Connect to operator services on startup
- Load platform settings from the operator
- Initialize all domain services

**Configuration:**
- Local bootstrap settings are loaded from the operator volume
- Platform settings are merged from the operator on startup
- TLS certificate paths are configured via the BootstrapService

### Running in Docker

The ensemble ships as a Docker image built from `ensemble/Dockerfile` (build context: g8e repo root). The repo-root `docker-compose.yml` includes an `ensemble` service that brings it up alongside the gateway and operator:

```bash
# From the g8e repo root — brings up gateway, operator, ensemble, and dashboard
docker compose up
```

### Running Tests

```bash
# Unit tests (no external dependencies)
pytest tests/ -v -m "not ai_integration and not requires_web_search and not requires_api and not e2e"

# Evals tests
pytest evals/tests/ -v

# Full test suite
pytest tests/ evals/tests/ -v
```

### Development

```bash
# Lint with Ruff
ruff check .

# Format with Ruff
ruff format .

# Type check with Pyright
pyright

# Run pre-commit hooks (if installed)
pre-commit run --all-files

# Install pre-commit hooks
pre-commit install
```

## Project Structure

```
ensemble/
├── app/                 # Main application code (FastAPI, services, models)
│   ├── clients/         # External service clients
│   ├── constants/       # Application constants
│   ├── db/              # Database models and operations
│   ├── llm/             # LLM provider implementations
│   ├── models/          # Pydantic models
│   ├── protocol/        # Protocol definitions
│   ├── routers/         # FastAPI route handlers
│   ├── services/        # Business logic services
│   └── utils/           # Utility functions
├── tests/               # Unit and integration tests (Tier 1, co-located)
├── evals/               # Evaluation suite and benchmarks
├── config/              # Configuration files
├── scripts/             # Utility scripts
├── pyproject.toml       # Python project configuration
├── .env.example         # Environment variables template
├── CONTRIBUTING.md      # Contribution guidelines
├── CHANGELOG.md         # Changelog
├── Dockerfile           # Docker image (build context: g8e repo root)
└── README.md            # This file
```

## Development Extras

The project supports optional dependency groups for different use cases:

- `dev`: Development tools (ruff, pyright, pre-commit)
- `test`: Testing framework (pytest, pytest-cov, etc.)
- `docs`: Documentation tools (mkdocs, mkdocs-material)

Install with:
```bash
pip install -e ".[dev,test,docs]"
```

## Contributing

g8ee is a first-party component of the g8e platform. See [CONTRIBUTING.md](CONTRIBUTING.md) for ensemble-specific guidelines, and the repo-root [CONTRIBUTING.md](../.github/CONTRIBUTING.md) for platform-wide contribution guidelines.

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for the ensemble changelog. Platform-wide changes are documented in the repo-root [release notes](../docs/release_notes/).

## License

Business Source License 1.1 (BSL 1.1). Converts to Apache 2.0 on 2030-08-18. See the [LICENSE](LICENSE) file in this directory and the repo-root [LICENSE](../LICENSE) file for details.
