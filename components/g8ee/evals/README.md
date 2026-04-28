# g8e Evals - Real-Operator Evaluation Framework

This directory contains the new host-driven evals framework for testing g8e with real operator containers.

## Prerequisites

1. **Platform setup**
   ```bash
   ./g8e platform setup
   ./g8e platform start
   ```

2. **Authentication**
   - Generate a device link token from the g8e dashboard
   - Authenticate locally:
     ```bash
     ./g8e login --device-token dlk_xxx
     ```

## Quick Start

1. **Bring up the eval fleet**
   ```bash
   ./g8e evals up -d dlk_xxx
   # or
   ./g8e evals up --device-token dlk_xxx
   ```

2. **Check fleet status**
   ```bash
   ./g8e evals status
   ```

3. **View logs for a specific node**
   ```bash
   ./g8e evals logs eval-node-01
   ```

4. **Tear down the fleet**
   ```bash
   ./g8e evals down
   ```

## Architecture

The evals framework uses real Linux containers running real g8e operators:

- **eval-node containers**: Each runs a real g8e operator with device token authentication
- **g8e-network**: All nodes join the platform network to reach g8e.local
- **Host-driven runner**: Evals are launched from the Docker host, not inside test-runner containers

## File Layout

```
components/g8ee/evals/                    # Fleet assets + gold sets (data only)
├── README.md
├── docker-compose.evals.yml              # Eval fleet (N operator containers)
├── containers/
│   └── eval-node/
│       ├── Dockerfile
│       └── entrypoint.sh                 # Downloads + supervises operator
└── gold_sets/                            # Scenario JSON inputs

components/g8ee/app/evals/runner/         # Python runner package (app.evals.runner)
├── cli.py                                # ./g8e evals run entrypoint
├── fleet.py                              # compose up/down, health, discover
├── client.py                             # g8ed chat/approval/SSE client
├── scorer.py                             # judge + deterministic matchers
├── reporter.py                           # report rendering (text/CSV/JSON)
└── metrics.py                            # EvalRow / DimensionSummary / FullReport
```

## Migration Status

This framework superseded the previous `tests/evals/` pytest-based fixture, which has been removed. See `docs/testing.md` for the architectural rationale.

**Completed:**
- Eval-node container (`containers/eval-node/`)
- Compose fleet wrapper (`docker-compose.evals.yml`, `./g8e evals up|down|status|logs`)
- Host-driven runner (`app/evals/runner/cli.py`, `app/evals/runner/fleet.py`, `app/evals/runner/client.py`)
- Scorer + reporter (`app/evals/runner/scorer.py`, `app/evals/runner/reporter.py`, `app/evals/runner/metrics.py`)
- Full `./g8e evals run --gold-set <path>` entrypoint
- Privacy gold set / Sentinel scrubber drift coverage in `tests/unit/security/test_privacy_gold_set_placeholders.py`

## Resource Footprint

**Per eval-node container:**
- Base image: debian:stable-slim (~100MB)
- Operator binary: ~50MB (downloaded at runtime)
- Memory: ~200-300MB (operator + supervisor loop)
- CPU: Minimal idle, spikes during command execution

**Fleet of 3 nodes:**
- Total memory: ~600-900MB
- Total disk: ~300MB (base images) + ~150MB (operator binaries)
- Network: g8e-network internal traffic only

## Expected Run Times

**Fleet startup:** ~30-45 seconds
- Docker build: first run only (~20s)
- Container startup: ~5s per node
- Operator download: ~5-10s per node
- Operator bind: ~5-10s per node

**Scenario execution:** ~5-15 seconds per scenario
- Investigation creation: <1s
- LLM processing: 3-10s (varies by model and query complexity)
- Command execution: 1-3s
- Approval handling: <1s
- Node restart: ~2s

**Benchmark suite (13 scenarios):** ~2-3 minutes total
- Sequential execution on 3 nodes with round-robin assignment

**Smoke test (1 scenario):** ~30-45 seconds total
- Fleet startup + single scenario execution
