# Operation Config Schema Reference

The unified `./g8e eval` facade replaces the provisional flat `CampaignRunConfig` with versioned typed operation documents. This document describes the schema, fields, validation rules, and the relationship between configs, presets, and the Go facade.

## Schema version

All operation configs use `schema_version: "1.0.0"`. The schema version is shared between Go and Python through a contract-tested registry.

## Operation kinds

| Kind | Config type | Description |
| --- | --- | --- |
| `diagnostic` | `DiagnosticConfig` | Single-arm diagnostic: one model, one arm, one run. No campaign identity, assignment manifest, or randomized schedule. |
| `campaign` | `CampaignConfig` | Authoritative multi-arm, multi-cohort campaign: one campaign identity, one report directory, one assignment manifest, one randomized schedule, one final canonical analysis. |

## Common fields

Every config includes these fields:

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `schema_version` | `"1.0.0"` | yes | Pinned schema version. |
| `operation_kind` | `"diagnostic"` or `"campaign"` | yes | Selects the specialized config type. |
| `operation_id` | string | yes | Stable operation identifier. |
| `revision` | string | yes | Fresh revision identifier. |
| `suite` | string | yes | Benchmark suite ID (e.g., `ifeval_subset`). |
| `seed` | integer >= 0 | yes | Deterministic sampling seed. |
| `report_root` | string | yes | Repository-relative report root. Absolute and escaping paths are rejected. |
| `gold_set` | `AuthorityRef` | yes | Gold set authority reference. |
| `evidence_key` | `EvidenceKeyRef` | yes | Evidence key reference (path and key ID; never key bytes). |
| `provider_endpoint` | `ProviderEndpointRef` | yes | Provider endpoint class and reference without credentials. |
| `budget` | `BudgetCeilings` | yes | Provider budget ceilings. |
| `stop_conditions` | `StopConditions` | yes | Stop conditions (idle timeout, max duration). |
| `content_hash` | string (64 hex chars) | computed | SHA-256 over canonical JSON excluding `content_hash`. Verified on load. |

## Diagnostic-specific fields

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `model_variant_id` | string | yes | Model variant ID (e.g., `gemma4:e4b`). |
| `arm` | string | yes | Experiment arm (e.g., `ensemble_ungoverned`). |
| `sampling` | `SamplingPolicy` | no | Sampling policy (temperature, top_p, max_output_tokens). |
| `preregistration` | `AuthorityRef` | no | Preregistration authority for paired analysis. |
| `task_limit` | integer > 0 | no | Limit number of tasks. |
| `task_offset` | integer >= 0 | no | Task offset (default 0). |

## Campaign-specific fields

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `campaign_id` | string | yes | Campaign identifier. |
| `release_version` | string | yes | Release version. |
| `preregistration` | `AuthorityRef` | yes | Preregistration authority. |
| `profile` | `AuthorityRef` | yes | Campaign profile authority. |
| `model_registry` | `AuthorityRef` | yes | Model registry authority. |
| `model_tags` | `AuthorityRef` | no | Model tags authority. |
| `arms` | list[string] | yes | Experiment arms (min 1). |
| `cohort_ids` | list[string] | yes | Cohort IDs (min 1). |
| `repetitions` | integer > 0 | yes | Repetitions per cohort (default 1). |
| `task_offset` | integer >= 0 | no | Task offset (default 0). |
| `task_limit` | integer > 0 | no | Limit number of tasks. |
| `campaign_set_plan` | `AuthorityRef` | no | Campaign-set plan authority. |
| `replacement_rule` | `AuthorityRef` | no | Replacement rule authority. Requires `campaign_set_plan`. |
| `publication_eligible` | bool | yes | Whether the campaign is eligible for publication (default true). |

## Sub-models

### AuthorityRef

| Field | Type | Description |
| --- | --- | --- |
| `path` | string | Repository-relative path. Absolute and escaping paths are rejected. |
| `sha256` | string (64 hex chars) | SHA-256 content hash. |

### EvidenceKeyRef

| Field | Type | Description |
| --- | --- | --- |
| `path` | string | Repository-relative path to the evidence key file. Absolute and escaping paths are rejected. |
| `key_id` | string | Expected key ID. Key bytes are never placed in config. |

### ProviderEndpointRef

| Field | Type | Description |
| --- | --- | --- |
| `provider` | string | Provider name (e.g., `ollama`, `openai`). |
| `endpoint_class` | string | Endpoint class identifier. Credentials remain in the supported environment. |

### BudgetCeilings

| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `max_requests` | integer > 0 | required | Maximum provider requests. |
| `max_tokens` | integer > 0 | required | Maximum provider tokens. |
| `max_usd` | float >= 0 | required | Maximum USD spend. Zero is valid for local-only providers. |
| `max_retries` | integer >= 0 | 1 | Maximum retries per task. |
| `concurrency` | integer > 0 | 1 | Concurrency level. |
| `min_free_disk_gb` | float >= 0 | none | Minimum free disk in GB. |

### StopConditions

| Field | Type | Description |
| --- | --- | --- |
| `idle_timeout_s` | float > 0 | Seconds without an SSE event before declaring a task idle. |
| `max_duration_s` | float > 0 | Optional whole-stream deadline. |

### SamplingPolicy

All fields are optional; `None` means the provider default applies.

| Field | Type | Description |
| --- | --- | --- |
| `temperature` | float [0.0, 2.0] | Sampling temperature. |
| `top_p` | float (0.0, 1.0] | Nucleus sampling threshold. |
| `max_output_tokens` | integer > 0 | Maximum output tokens. |

## Validation rules

- Unknown fields are rejected (`extra="forbid"`).
- Absolute and escaping repository-relative paths are rejected.
- Content hash is verified on load; a mismatch indicates tampering or drift.
- Zero or negative budget values are rejected.
- `replacement_rule` requires `campaign_set_plan`.
- Campaign `arms` and `cohort_ids` must be non-empty.
- `repetitions` must be positive.

## Platform-owned fields (not in config)

The following fields are injected by the Go facade from the selected repository/runtime context and never appear in user-authored config:

- `auth_project_root`
- `g8e_cli`
- `operator_url`
- `g8ee_url`
- Canonical trust-bundle paths
- Default Gateway HTTP/HTTPS URLs
- Ensemble URL
- Runtime directory
- `g8e` binary path and SHA-256
- Platform version

## Presets

Repository-owned presets supply policy defaults (suite, task population, arm, repetition, sampling, budget ceilings, stop conditions). They never supply authority, owner approval, evidence keys, provider endpoints, model identities, report roots, or operation IDs.

### Available diagnostic presets

| Name | Description |
| --- | --- |
| `ifeval-five-task` | Five-task ifeval_subset diagnostic against one model and one arm. Budget: 30 requests, 491520 tokens, 0 USD. |

### Available campaign presets

| Name | Description |
| --- | --- |
| `opendevops-development` | Development campaign over ifeval_subset with direct and ensemble_ungoverned arms. Three repetitions, concurrency one. Budget: 100 requests, 1000000 tokens, 0 USD. |
| `ifeval-full-120` | Full 120-task ifeval_subset campaign across all 25 instruction types. Three repetitions. Budget: 500 requests, 5000000 tokens, 0 USD. |

## Draft generation

`./g8e eval diagnostic draft` and `./g8e eval campaign draft` create a new config at a path that does not exist. They consume a named preset and explicit concise overrides. Draft generation performs no provider call, model pull, report-root creation, lease issuance, or publication.

The generated config has no active lease and cannot be started; lease lifecycle remains a separate typed record so the content-addressed config does not mutate during authorization.

## Example files

- `ensemble/evals/examples/configs/diagnostic-example.json`
- `ensemble/evals/examples/configs/campaign-example.json`
