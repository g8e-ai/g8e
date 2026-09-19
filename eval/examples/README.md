# Evaluation runtime artifacts (`.g8e/eval/`)

This directory documents the **runtime** evaluation artifacts that live under `.g8e/eval/` on the campaign host. These paths are gitignored; checked-in templates and the public/private boundary live here in `eval/examples/`.

## Runtime files

| Path | Purpose |
| --- | --- |
| `.g8e/eval/model-inventory.json` | Full provider freeze from `g8e eval models freeze` |
| `.g8e/eval/inventories/*.json` | Per-model campaign inventory files |
| `.g8e/eval/init-campaign-queue.json` | Rollout queue manifest |

## Typical workflow

```bash
# 1. Freeze provider inventory
./g8e eval models freeze \
  --campaign-id eval-smoke \
  --ollama-endpoint "$G8E_OLLAMA_ENDPOINT" \
  --output .g8e/eval/model-inventory.json

./g8e eval models list

# 2. Build rollout queue (optional batch path)
./g8e eval rollout init --materialize --merge
./g8e eval rollout run --tier-a --skip-verified --skip-variant granite3-3-2b

# Or materialize one combined inventory for a multi-model smoke campaign
./g8e eval models materialize --tags qwen3:0.6b,qwen3:4b,gemma3:4b \
  --campaign-id eval-smoke-mini \
  --output .g8e/eval/inventories/eval-smoke-mini.json
```

## CLI reference

| Command | Purpose |
| --- | --- |
| `g8e eval models freeze` | Discover and freeze all models from the provider |
| `g8e eval models list` | List variants in a frozen inventory file |
| `g8e eval models materialize` | Write per-model or combined campaign inventory files |
| `g8e eval rollout init` | Build `.g8e/eval/init-campaign-queue.json` |
| `g8e eval rollout run` | Unattended rollout: start → verify for every queued model |
| `g8e eval rollout list` | Inspect queue entries |
| `g8e eval rollout next` | Show the next pending model |
| `g8e eval rollout mark` | Manually record verify progress for one entry |

See [Unified Docker Stack Guide](../../docs/guides/unified_stack.md) and [Evaluations architecture](../../docs/architecture/evals.md) for full campaign operations.
