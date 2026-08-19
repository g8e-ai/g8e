# Getting Started

## Prerequisites

- Python 3.14+
- g8e operator services running and accessible (DB, KV, PubSub, Blob, HTTP)
- TLS certificates mounted at expected paths (configured via BootstrapService)
- Valid operator session credentials

## Installation

```bash
# Create virtual environment
python3 -m venv venv
source venv/bin/activate

# Install dependencies
pip install -e ".[dev,test]"

# Or install from requirements.txt
pip install -r requirements.txt

# Copy environment template and configure
cp .env.example .env
# Edit .env with your configuration
```

## Running the Application

```bash
# Using uvicorn directly
uvicorn app.main:app --host 0.0.0.0 --port 8443 --reload

# Or run the module directly
python -m app.main
```

The application starts on port 8443 (HTTPS), connects to operator services on startup, loads platform settings from the operator, and initializes all domain services.

## Configuration

- Local bootstrap settings are loaded from the operator volume
- Platform settings are merged from the operator on startup
- TLS certificate paths are configured via the BootstrapService

## Next Steps

- [Architecture](architecture.md) — Understand the system design
- [Governance](governance.md) — Learn about the 3-layer governance model
- [Development](devs.md) — Set up your development environment
