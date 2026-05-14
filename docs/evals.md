---
title: Evals
---

# g8e Evals - Real-Operator Evaluation Framework

The g8e Evals framework provides a host-driven, high-fidelity testing environment for validating the platform's behavior with real `g8e.operator` binaries running in isolated Linux containers.

## Introduction

Unlike standard unit tests, the evals framework exercises the entire stack:
- **LLM Reasoning**: Verifies the AI's ability to plan and execute tasks.
- **Protocol Fidelity**: Ensures correct communication between `g8ed` and the operator.
- **Security & Privacy**: Validates that sensitive data is scrubbed and approvals are enforced.
- **Execution Accuracy**: Confirms that commands are executed correctly on a real filesystem.

## Architecture

The framework is split into two primary locations:

### 1. Fleet Assets (`components/g8ee/evals/`)
This directory contains the infrastructure definition for the evaluation fleet:
- **`docker-compose.evals.yml`**: Defines a dynamically scalable fleet of nodes (e.g., `evals-eval-node-1`).
- **`containers/eval-node/`**: Contains the `Dockerfile` and `entrypoint.sh` for the operator environment.
- **`gold_sets/`**: JSON scenario definitions (Accuracy, Benchmark, Privacy).

### 2. Runner Logic (`components/g8ee/app/evals/runner/`)
The Python package responsible for orchestrating the evaluation:
- **`cli.py`**: Main entrypoint for `./g8e evals run`.
- **`fleet.py`**: Manages the lifecycle (up/down/restart) of Docker containers.
- **`client.py`**: Asynchronous client for `g8ed` (Chat, Investigations, SSE stream, Approvals).
- **`scorer.py`**: Implements deterministic regex matching, LLM-as-a-Judge, and privacy validation.
- **`metrics.py`**: Data models for evaluation results and reporting.

## The Evaluation Lifecycle

A full evaluation run follows a strict lifecycle to ensure reliability and isolation:

### 1. Fleet Startup and Operator Deployment
`./g8e evals deploy -d <token>` brings up 3 `eval-node` containers and authenticates them with the platform using a dashboard-issued device link token. Each node:
- Downloads the latest `g8e.operator` binary from the platform.
- Initializes a realistic filesystem state (e.g., `/var/log/app/app.log`).
- Starts the operator with the provided device token.

Before `./g8e evals run` can execute scenarios, those operators must be bound to the human's active web session in the dashboard. This web-session binding is required proof of human presence.

### 3. Scenario Execution
Scenarios are executed sequentially across the fleet:
- **Investigation Creation**: A new investigation is created for each scenario.
- **Chat Interaction**: The runner sends the `user_query` and streams events (text, tool calls, approvals).
- **Auto-Approval**: The runner automatically approves any `approval_required` events to allow execution to proceed.
- **Result Collection**: Final response text and all tool calls are captured for scoring.

### 4. Scoring & Validation
Each scenario is scored based on its dimension:
- **Benchmark/Accuracy**: Deterministic regex matching on tool call arguments or LLM-as-a-Judge via `EvalJudge`.
- **Privacy**: Validation that defined secrets (e.g., API keys) do not appear in the final response or egress logs.

### 5. Node Reset
After **every scenario**, the node used is restarted. This ensures that filesystem side effects from one test do not leak into the next, maintaining strict isolation between scenarios.

### 6. Reporting
Results are aggregated into a `FullReport` and persisted to `reports/evals/` in three formats:
- **Text**: Summary table for console output.
- **JSON**: Complete structured data for CI integration.
- **CSV**: Flattened results for spreadsheet analysis.

## Usage

### Prerequisites
1. **Platform setup**
   ```bash
   ./g8e platform setup
   ./g8e platform start
   ```
2. **Authentication**
   - Generate a device link token from the g8e dashboard.
   - Keep the dashboard open so you can bind the eval operators to your web session.

### Quick Start
1. **Deploy and authenticate eval operators**
   ```bash
   ./g8e evals deploy -d dlk_xxx
   ```
2. **Bind the eval operators in the dashboard**
   Select the newly connected eval operators in the web UI and bind them to your active web session.
3. **Check fleet status**
   ```bash
   ./g8e evals status
   ```
4. **View logs for a specific node**
   ```bash
   ./g8e evals logs evals-eval-node-1
   ```
5. **Run a gold set**
   ```bash
   ./g8e evals run --gold-set components/g8ee/evals/gold_sets/benchmark.json
   ```
6. **Tear down**
   ```bash
   ./g8e evals down
   ```

## Invariants
- **Real Binaries**: Evals always run the actual `g8e.operator` binary, never a mock.
- **Human Presence**: Evals cannot run from a device link token. `evals run` requires operators that have been bound to a human web session.
- **Isolation**: Nodes are restarted between scenarios to prevent state bleed.
- **Runner Container**: The runner executes in `g8ee-test-runner` with Docker socket access, orchestrating eval containers through Docker Compose.
