# Evals

## Overview

g8ee includes an evaluation suite for measuring agent performance across benchmarks. Evals are separate from the standard test suite and may require AI provider access.

## Structure

```
evals/
├── g8e_evals/
│   ├── benchmarks/    # Benchmark definitions
│   ├── receipts/      # Evaluation run receipts
│   ├── report/        # Generated reports
│   └── ...
├── tests/             # Eval-specific tests
└── Dockerfile         # Containerized eval environment
```

## Running Evals

```bash
# Run eval tests
pytest evals/tests/ -v

# Run the eval suite directly
python -m evals
```

## Benchmarks

Benchmarks measure:

- Agent reasoning quality
- Consensus accuracy
- Protocol compliance
- Provider-specific performance

## Related

- [Testing](tests.md) — Standard test suite
- [Development](devs.md) — Dev setup
- [Thinking](thinking.md) — Reasoning quality evaluation
