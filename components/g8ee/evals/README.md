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
components/g8ee/evals/
├── README.md
├── docker-compose.evals.yml              # Eval fleet (N operator containers)
├── containers/
│   └── eval-node/
│       ├── Dockerfile
│       └── entrypoint.sh                 # Downloads + supervises operator
└── runner/                               # TODO: Step 3+
    ├── cli.py                            # ./g8e evals run entrypoint
    ├── fleet.py                          # compose up/down, health, discover
    ├── client.py                         # g8ed chat/approval/SSE client
    ├── scorer.py                         # judge + deterministic matchers
    ├── reporter.py                       # (move from tests/evals/reporter.py)
    └── metrics.py                        # (move from tests/evals/metrics.py)
```

## Migration Status

This framework supersedes the broken `tests/evals/real_operator_fixture.py` implementation. See `docs/benchmarking/evals.md` for the complete design document and implementation plan.

**Completed Steps:**
- Step 0: Deleted broken real_operator_fixture.py
- Step 1: Created eval-node container
- Step 2: Created docker-compose.evals.yml and CLI wrappers
- Step 3: Runner skeleton + g8ed client
- Step 4: Scorer + reporter port
- Step 5: Full runner loop
- Step 6: Regression coverage (smoke test added)

**Remaining Steps:**
- Step 7: Docs + deprecation notes

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
