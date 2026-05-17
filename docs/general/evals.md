---
title: Evals
---

# g8e Evals - Real-Operator Evaluation Framework

The g8e Evals framework provides a host-driven, high-fidelity testing environment for validating the platform's behavior with real `g8e.operator` binaries running in isolated Linux containers.

## Introduction

The evaluation framework exercises the core substrate:
- **LLM Reasoning**: Verifies the AI's ability to plan and execute tasks.
- **Protocol Fidelity**: Ensures correct communication between the engine and the operator.
- **Execution Accuracy**: Confirms that commands are executed correctly on the target system.

## Architecture

The framework is split into two primary locations:

### 1. Runner Logic (`evals/`)
The Python package responsible for orchestrating the evaluation:
- **`g8e_evals/`**: Main Python package.
- **`cli.py`**: Main entrypoint for `./g8e evals bench`.
- **`gold_sets/`**: Benchmark scenario definitions (e.g., IFEval).
- **`harness.py`**: Execution orchestration for evaluations.
- **`uap_utils.py`**: Protocol envelope utilities.

## The Evaluation Lifecycle

A full evaluation run follows a strict lifecycle to ensure reliability and isolation:

### 1. Runner Setup
`./g8e evals bench` ensures the evaluation environment is ready. It reuses the `g8ee` virtualenv for dependencies.

### 2. Scenario Execution
Scenarios are executed against a running Operator:
- **Benchmark Execution**: The runner executes tasks via the Operator's public protocol.
- **Result Collection**: Responses and receipts are captured for scoring.

### 4. Scoring & Validation
Each scenario is scored based on its dimension:
- **Benchmark/Accuracy**: Deterministic regex matching on tool call arguments or LLM-as-a-Judge via `EvalJudge`.
- **Privacy**: Validation that defined secrets (e.g., API keys) do not appear in the final response or egress logs.

### 5. Node Reset
After **every scenario**, the node used is restarted. This ensures that filesystem side effects from one test do not leak into the next, maintaining strict isolation between scenarios.

### 3. Reporting
Results are aggregated and persisted to `reports/` (default) in JSON and summary formats.

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
1. **Platform setup**
   ```bash
   ./g8e platform start
   ```
2. **Run a benchmark**
   ```bash
   ./g8e evals bench --suite ifeval --operator-session-id <operator_session_id>
   ```

## Invariants
- **Protocol Native**: Evals use the canonical g8e protocol to interact with the Operator.
- **Fail-Closed**: All mutations must be verified and signed.
