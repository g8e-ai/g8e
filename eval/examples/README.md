# Evaluation artifacts (`eval/`)

Checked-in evaluation program data and templates live here. **Runtime** campaign state (provider freezes, per-model inventories, rollout progress) lives under `.g8e/eval/` on the campaign host and is gitignored.

## Checked-in program inventory

| Path | Purpose |
| --- | --- |
| `eval/base-model-inventory.json` | **Default genesis program inventory** — 45 init-campaign models (3 roles × 25 scenarios = **3375** cells when run homogeneously) |
| `eval/base-init-campaign-queue.json` | Template rollout queue for all 45 models (`status: pending`) |
| `eval/examples/init-campaign-queue.example.json` | Minimal queue shape reference |

The base inventory is the canonical 45-model genesis program set. Campaign ID: `eval-genesis-homogeneous`. Digests are a reference provider snapshot; re-freeze from your Ollama host before scored runs on release code. The rollout queue places the current Granite 4.2 and Qwen 3.5 small-model intake ahead of the alphabetical backlog.

### 45-model program set

| Family | Models |
| --- | --- |
| DeepSeek | `deepseek-r1:7b` |
| GLM | `milkey/GLM-4-9B-0414:Q4_K_M` |
| GPT-OSS | `gpt-oss:20b` |
| Gemma | `gemma2:9b`, `gemma3:270m`, `gemma3:1b`, `gemma3:4b`, `gemma4:e2b`, `gemma4:e4b` |
| Granite | `granite4.2:3b`, `granite4.2:8b`, `granite3.3:2b`, `granite3.3:8b` |
| Hermes | `hermes3:8b` |
| Llama | `llama3.1:8b`, `llama3.2:1b`, `llama3.2:3b`, `tinyllama:1.1b` |
| Mistral | `mistral:7b`, `ministral-3:3b`, `ministral-3:8b` |
| Phi | `phi4-mini:3.8b`, `phi4-mini-reasoning:3.8b` |
| Qwen | `pdevine/qwen3.6:27b-mtp-q4_K_M`, `qwen3:30b`, `qwen3.5:0.8b`, `qwen3.5:2b`, `qwen3.5:4b`, `qwen3.5:9b`, `qwen3:0.6b`, `qwen3:1.7b`, `qwen3:4b`, `qwen3:8b`, `qwen2.5:0.5b`, `qwen2.5:3b`, `qwen2.5:7b`, `qwen2.5-coder:7b` |
| SmolLM | `smollm2:135m`, `smollm2:360m`, `smollm2:1.7b`, `Impulse2000/smollm3:3b-q4_k_m` |
| Other | `Randomblock1/nemotron-nano:8b`, `sam860/LFM2:350m`, `sam860/LFM2:700m`, `sam860/LFM2:2.6b` |

Per-model campaigns use `eval-init-<variant_id>` (legacy exception: `gemma4:e4b` → `init-campaign`). Each single-model run is **75** cells.

## Runtime files (campaign host)

| Path | Purpose |
| --- | --- |
| `.g8e/data/eval/runs/<run-id>/` | **Canonical campaign evidence** — assignments, verification, publication state (survives gateway volume wipe) |
| `.g8e/eval/model-inventory.json` | Full provider freeze from `g8e eval models freeze` |
| `.g8e/eval/inventories/*.json` | Per-model campaign inventory files |
| `.g8e/eval/init-campaign-queue.json` | Rollout queue manifest |

Gateway-owned public mirror state (SSE feed, explorer datasets) lives in the Docker `g8e-gateway-data` volume only. It is **not** on the host `.g8e/` tree. After a gateway volume wipe, republish from host run artifacts (see [Wipe recovery](#wipe-recovery)).

## Typical workflow

```bash
# 1. Freeze provider inventory (recommended before scored runs on release code)
./g8e eval models freeze \
  --campaign-id eval-genesis-homogeneous \
  --ollama-endpoint "$G8E_OLLAMA_ENDPOINT" \
  --output .g8e/eval/model-inventory.json

./g8e eval models list

# 2. Build rollout queue from the base program inventory (default)
./g8e eval rollout init --materialize --merge

# Or reconcile live digests: freeze first, then filter runtime to base program tags
./g8e eval rollout init --from .g8e/eval/model-inventory.json --materialize --merge

./g8e eval rollout run --require-witness --skip-verified --skip-variant granite3-3-2b

# Or materialize one combined inventory for a multi-model smoke campaign
./g8e eval models materialize --tags qwen3:0.6b,qwen3:4b,gemma3:4b \
  --campaign-id eval-smoke-mini \
  --output .g8e/eval/inventories/eval-smoke-mini.json
```

Regenerate checked-in base files after changing the program model set:

```bash
go run ./.local.dev/tools/gen-base-model-inventory
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
| `g8e eval campaign export` | Export one run to a directory you choose (`--output-dir`) |
| `g8e eval campaign mirror restore` | Republish verified runs to the gateway-owned public mirror |

See [Unified Docker Stack Guide](../../docs/guides/unified_stack.md) and [Evaluations architecture](../../docs/architecture/evals.md) for full campaign operations.

## Wipe recovery

What survives depends on what you wipe:

| Wipe scope | Host `.g8e/data/eval/runs/` | Host `.g8e/eval/` queue | Gateway mirror volume |
| --- | --- | --- | --- |
| `docker init --clean` / gateway volume only | **Kept** | **Kept** | **Lost** |
| `./g8e docker clean` / full `.g8e` delete | **Lost** unless backed up | **Lost** unless backed up | **Lost** |

### After gateway volume wipe (most common)

Host evidence and queue remain. Restore the public mirror from verified queue entries:

```bash
curl -sf http://127.0.0.1:8082/bootstrap | jq '{freshness: .source_freshness, high_water: .snapshot.high_water_sequence}'

./g8e eval campaign mirror restore --queue
```

`docker init` runs this automatically when the queue and run artifacts are present. For one run:

```bash
./g8e eval campaign mirror restore --run-id eval-init-granite3-3-2b-1789754079
```

If `campaign publish` exports zero records after a mirror wipe (host `public-projection-state.json` still lists old idempotency keys), use `--force` on publish instead — see [Unified Docker Stack Guide](../../docs/guides/unified_stack.md#mirror-empty-after-docker-init---clean-but-host-run-artifacts-remain).

Runs marked `verified` in the queue but missing under `.g8e/data/eval/runs/<verified_run_id>/` cannot be mirror-restored. Mark them pending and re-run:

```bash
./g8e eval rollout mark --variant-id qwen3-0-6b --status pending --notes "redo after host artifact loss"
./g8e eval campaign start --queue qwen3-0-6b --publish --daemon --require-witness
```

### Archive evidence to a directory you choose

Canonical evidence stays under `.g8e/data/eval/runs/` during execution. Export a portable bundle any time:

```bash
./g8e eval campaign export --output-dir ./my-eval-archive/granite-reference eval-init-granite3-3-2b-1789754079
```

Back up `.g8e/data/eval/runs/` and `.g8e/eval/` together before a full platform wipe if you want to resume without re-executing inference.

### Custom queue or inventory paths

Rollout flags accept repo-relative or absolute paths:

```bash
./g8e eval rollout init --output /data/my-queue.json --inventory-dir /data/my-inventories --materialize --merge
./g8e eval rollout run --queue-file /data/my-queue.json --require-witness --skip-verified
```

## License

Source in this repository is licensed under the Business Source License 1.1 (BSL 1.1). It converts to Apache 2.0 on 2030-08-18. See the repository [LICENSE](../../LICENSE) for details.
