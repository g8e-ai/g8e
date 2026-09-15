// Deterministic fixtures for every snapshot record kind and live event kind.
// These let every route render before the real projector or mirror is wired.
// Fixtures exercise all quality states so the UX shell proves every state
// renders. Worker 1's projector replaces these with real projected records
// published through the mirror; the store treats both identically.

import type {
  AssignmentResult,
  CatalogSnapshot,
  ConfidenceInterval,
  EvaluationSummary,
  LiveEvent,
  MethodologySnapshot,
  ModelSummary,
  ProviderEnvironment,
  SuiteSummary,
} from '../contract/types';

const NOW = '2026-09-14T08:00:00Z';
const EXPLO_DATASET = 'ds-exploratory-baseline-20260914-r2';
const VERIFIED_DATASET = 'ds-verified-public-20260914';
const LIVE_DATASET = 'ds-live-demo-20260914';

export const FIXTURE_DATASET_IDS = {
  exploratory: EXPLO_DATASET,
  verified: VERIFIED_DATASET,
  live: LIVE_DATASET,
} as const;

/** Owner-declared description of the provider host serving local inference.
 *  Every dataset in this deployment ran against the same Ollama machine. */
export const fixtureProviderEnvironment: ProviderEnvironment = {
  source: 'declared',
  processor: '13th Gen Intel(R) Core(TM) i9-13900K (3.00 GHz)',
  memory: '64.0 GB',
  graphics: 'NVIDIA GeForce RTX 4070 Ti SUPER (16 GB)',
  storage: '2.20 TB of 4.55 TB used',
  system_type: '64-bit operating system, x64-based processor',
};

export const fixtureCatalogExploratory: CatalogSnapshot = {
  schema_version: '1.2.0',
  kind: 'catalog_snapshot',
  dataset_id: EXPLO_DATASET,
  dataset_kind: 'exploratory_baseline',
  quality_state: 'exploratory_partial',
  observed_at: NOW,
  source_revision_label: 'overnight-baseline/run-20260914-r2',
  title: 'Exploratory baseline (2026-09-14 r2)',
  description:
    'Nine finalized exploratory suites across nine role candidates. Five suites pass canonical verification; four fail because seven completed attempts lack provider resource observations.',
  limitations: [
    'Exploratory aggregate is not publication-eligible.',
    'Four suites have verification failures due to seven missing resource observations.',
    'Pass-rate intervals are exploratory confidence bounds, not certified claims.',
  ],
  model_count: 31,
  evaluated_count: 9,
  suite_count: 9,
  run_count: 9,
  assignment_count: 1125,
  provider_request_count: 2282,
  provider_token_count: 20185634,
  retry_count: 0,
  verifier_passed_count: 5,
  verifier_failed_count: 4,
  generated_at: NOW,
  provider_environment: fixtureProviderEnvironment,
};

export const fixtureCatalogVerified: CatalogSnapshot = {
  schema_version: '1.2.0',
  kind: 'catalog_snapshot',
  dataset_id: VERIFIED_DATASET,
  dataset_kind: 'verified_public_snapshot',
  quality_state: 'verified_public',
  observed_at: NOW,
  source_revision_label: 'docs/evidence/readme/current',
  title: 'Verified public snapshot (2026-09-14)',
  description:
    'Two checksum-bound five-task IFEval runs from the current public snapshot. One scored 4/5 and one scored 3/5. This is a small high-confidence dataset, kept separate from exploratory measurements.',
  limitations: [
    'Small scope: two five-task runs only.',
    'Not merged with exploratory rankings.',
  ],
  model_count: 31,
  evaluated_count: 2,
  suite_count: 1,
  run_count: 2,
  assignment_count: 10,
  provider_request_count: 10,
  provider_token_count: 0,
  retry_count: 0,
  verifier_passed_count: 2,
  verifier_failed_count: 0,
  generated_at: NOW,
  provider_environment: fixtureProviderEnvironment,
};

export const fixtureCatalogLive: CatalogSnapshot = {
  schema_version: '1.2.0',
  kind: 'catalog_snapshot',
  dataset_id: LIVE_DATASET,
  dataset_kind: 'live_run',
  quality_state: 'live_in_progress',
  observed_at: NOW,
  source_revision_label: 'live-demo-20260914',
  title: 'Live demo run (2026-09-14)',
  description:
    'A bounded five-task ifeval_subset diagnostic running against the remote Ollama provider. Values are provisional while the run is in progress.',
  limitations: [
    'Live values are provisional and may change.',
    'Not ranked against terminal datasets by default.',
  ],
  model_count: 31,
  evaluated_count: 3,
  suite_count: 1,
  run_count: 1,
  assignment_count: 5,
  provider_request_count: 0,
  provider_token_count: 0,
  retry_count: 0,
  verifier_passed_count: 0,
  verifier_failed_count: 0,
  generated_at: NOW,
  provider_environment: fixtureProviderEnvironment,
};

// Nine evaluated role candidates from the exploratory baseline.
const evaluatedVariants: Array<{
  variant_id: string;
  display_name: string;
  served_model_tag: string;
  role: ModelSummary['role'];
  pass_rate: ModelSummary['pass_rate'];
  coverage: number;
  latency_p50: number;
  latency_p95: number;
  throughput_p50: number;
  input_tokens: number;
  output_tokens: number;
}> = [
  {
    variant_id: 'gemma4-e4b',
    display_name: 'Gemma 4 E4B',
    served_model_tag: 'gemma4:e4b',
    role: 'primary',
    pass_rate: { estimate: 0.82, lower: 0.74, upper: 0.88, denominator: 125 },
    coverage: 1.0,
    latency_p50: 820,
    latency_p95: 2100,
    throughput_p50: 42.1,
    input_tokens: 184200,
    output_tokens: 96400,
  },
  {
    variant_id: 'gemma4-e2b',
    display_name: 'Gemma 4 E2B',
    served_model_tag: 'gemma4:e2b',
    role: 'assistant',
    pass_rate: { estimate: 0.76, lower: 0.68, upper: 0.83, denominator: 125 },
    coverage: 1.0,
    latency_p50: 540,
    latency_p95: 1450,
    throughput_p50: 58.3,
    input_tokens: 162800,
    output_tokens: 88100,
  },
  {
    variant_id: 'qwen25-05b',
    display_name: 'Qwen 2.5 0.5B',
    served_model_tag: 'qwen2.5:0.5b',
    role: 'lite',
    pass_rate: { estimate: 0.41, lower: 0.33, upper: 0.49, denominator: 125 },
    coverage: 1.0,
    latency_p50: 210,
    latency_p95: 680,
    throughput_p50: 92.4,
    input_tokens: 121000,
    output_tokens: 54200,
  },
  {
    variant_id: 'gemma4-e4b-alt',
    display_name: 'Gemma 4 E4B (alt)',
    served_model_tag: 'gemma4:e4b',
    role: 'primary',
    pass_rate: { estimate: 0.79, lower: 0.71, upper: 0.86, denominator: 125 },
    coverage: 1.0,
    latency_p50: 845,
    latency_p95: 2240,
    throughput_p50: 40.8,
    input_tokens: 179600,
    output_tokens: 94100,
  },
  {
    variant_id: 'gemma4-e2b-alt',
    display_name: 'Gemma 4 E2B (alt)',
    served_model_tag: 'gemma4:e2b',
    role: 'assistant',
    pass_rate: { estimate: 0.72, lower: 0.64, upper: 0.80, denominator: 125 },
    coverage: 1.0,
    latency_p50: 560,
    latency_p95: 1520,
    throughput_p50: 55.9,
    input_tokens: 158400,
    output_tokens: 86200,
  },
  {
    variant_id: 'qwen25-15b',
    display_name: 'Qwen 2.5 1.5B',
    served_model_tag: 'qwen2.5:1.5b',
    role: 'lite',
    pass_rate: { estimate: 0.55, lower: 0.46, upper: 0.63, denominator: 125 },
    coverage: 1.0,
    latency_p50: 320,
    latency_p95: 980,
    throughput_p50: 78.2,
    input_tokens: 132000,
    output_tokens: 61000,
  },
  {
    variant_id: 'gemma3-12b',
    display_name: 'Gemma 3 12B',
    served_model_tag: 'gemma3:12b',
    role: 'primary',
    pass_rate: { estimate: 0.71, lower: 0.63, upper: 0.79, denominator: 125 },
    coverage: 1.0,
    latency_p50: 1180,
    latency_p95: 3100,
    throughput_p50: 28.4,
    input_tokens: 201000,
    output_tokens: 102000,
  },
  {
    variant_id: 'qwen25-7b',
    display_name: 'Qwen 2.5 7B',
    served_model_tag: 'qwen2.5:7b',
    role: 'assistant',
    pass_rate: { estimate: 0.68, lower: 0.60, upper: 0.76, denominator: 125 },
    coverage: 1.0,
    latency_p50: 740,
    latency_p95: 1980,
    throughput_p50: 44.6,
    input_tokens: 174000,
    output_tokens: 92800,
  },
  {
    variant_id: 'llama3-1b',
    display_name: 'Llama 3 1B',
    served_model_tag: 'llama3:1b',
    role: 'lite',
    pass_rate: { estimate: 0.48, lower: 0.39, upper: 0.56, denominator: 125 },
    coverage: 1.0,
    latency_p50: 280,
    latency_p95: 820,
    throughput_p50: 84.1,
    input_tokens: 128000,
    output_tokens: 58000,
  },
];

export const fixtureModelSummaries: ModelSummary[] = evaluatedVariants.map((v) => ({
  schema_version: '1.2.0',
  kind: 'model_summary',
  dataset_id: EXPLO_DATASET,
  quality_state: 'exploratory_partial',
  observed_at: NOW,
  source_revision_label: 'overnight-baseline/run-20260914-r2',
  variant_id: v.variant_id,
  display_name: v.display_name,
  served_model_tag: v.served_model_tag,
  role: v.role,
  backend_provider_class: 'ollama',
  quantization_weight_class: 'q4_k_m',
  inventory_only: false,
  evaluation_coverage: v.coverage,
  pass_rate: v.pass_rate,
  agreement_pairwise: { value: 0.88 },
  agreement_all_five: { value: 0.74 },
  repeatability: {
    consistently_correct: 78,
    consistently_wrong: 12,
    inconsistent: 22,
    insufficient: 13,
  },
  latency_p50_ms: { value: v.latency_p50 },
  latency_p95_ms: { value: v.latency_p95 },
  output_throughput_p50: { value: v.throughput_p50 },
  output_throughput_p95: { value: v.throughput_p50 * 0.7 },
  input_tokens: { value: v.input_tokens },
  output_tokens: { value: v.output_tokens },
  thinking_tokens: { unavailable_reason: 'not applicable' },
  cache_tokens: { unavailable_reason: 'not observed' },
  terminal_outcomes: { completed: 122, model_failed: 3, grader_failed: 0, invalid_evidence: 0, stopped: 0 },
  unavailable_reasons: ['thinking_tokens: not applicable', 'cache_tokens: not observed'],
}));

// 22 registry models without measurements (31 total - 9 evaluated).
const unevaluatedNames = [
  'phi4-mini', 'mistral-nemo', 'codellama-7b', 'deepseek-r1-1b', 'yi-6b',
  'solar-10b', 'orca-mini-7b', 'vicuna-7b', 'openhermes-2.5', 'stablelm-2-12b',
  'neural-chat-7b', 'starling-lm-7b', 'dolphin-mistral-7b', 'wizardlm-7b', 'phind-codellama-34b',
  'everythinglm-13b', 'falcon-7b', 'zephyr-7b', 'airoboros-7b', 'nous-hermes-13b',
  'wizard-coder-7b', 'codeup-13b',
];
export const fixtureUnevaluatedModels: ModelSummary[] = unevaluatedNames.map((name, i) => ({
  schema_version: '1.2.0',
  kind: 'model_summary',
  dataset_id: EXPLO_DATASET,
  quality_state: 'not_evaluated',
  observed_at: NOW,
  variant_id: name,
  display_name: name,
  role: (i % 3 === 0 ? 'primary' : i % 3 === 1 ? 'assistant' : 'lite'),
  inventory_only: false,
  evaluation_coverage: 0,
  unavailable_reasons: ['no eligible observations in the selected dataset'],
}));

// Inventory-only models (additional provider models outside the registry).
export const fixtureInventoryOnlyModels: ModelSummary[] = [
  {
    schema_version: '1.2.0',
    kind: 'model_summary',
    dataset_id: EXPLO_DATASET,
    quality_state: 'not_evaluated',
    observed_at: NOW,
    variant_id: 'inventory-llava-7b',
    display_name: 'LLaVA 7B (inventory)',
    role: 'assistant',
    inventory_only: true,
    evaluation_coverage: 0,
    unavailable_reasons: ['inventory only · not evaluated'],
  },
];

const suiteDefs = [
  { suite_id: 'ifeval', display_name: 'IFEval', task_count: 25, verifier: 'passed' as const, status: 'completed' as const },
  { suite_id: 'mt-bench', display_name: 'MT-Bench', task_count: 20, verifier: 'passed' as const, status: 'completed' as const },
  { suite_id: 'humaneval', display_name: 'HumanEval', task_count: 25, verifier: 'passed' as const, status: 'completed' as const },
  { suite_id: 'gsm8k', display_name: 'GSM8K', task_count: 25, verifier: 'passed' as const, status: 'completed' as const },
  { suite_id: 'truthfulqa', display_name: 'TruthfulQA', task_count: 25, verifier: 'passed' as const, status: 'completed' as const },
  { suite_id: 'arc-challenge', display_name: 'ARC Challenge', task_count: 25, verifier: 'failed' as const, status: 'completed' as const },
  { suite_id: 'hella-swag', display_name: 'HellaSwag', task_count: 25, verifier: 'failed' as const, status: 'completed' as const },
  { suite_id: 'winogrande', display_name: 'Winogrande', task_count: 25, verifier: 'failed' as const, status: 'completed' as const },
  { suite_id: 'boolq', display_name: 'BoolQ', task_count: 25, verifier: 'failed' as const, status: 'completed' as const },
];

export const fixtureSuiteSummaries: SuiteSummary[] = suiteDefs.map((s) => ({
  schema_version: '1.2.0',
  kind: 'suite_summary',
  dataset_id: EXPLO_DATASET,
  quality_state: s.verifier === 'passed' ? 'exploratory_verified' : 'exploratory_partial',
  observed_at: NOW,
  source_revision_label: 'overnight-baseline/run-20260914-r2',
  suite_id: s.suite_id,
  display_name: s.display_name,
  task_count: s.task_count,
  assignment_count: 125,
  status: s.status,
  verifier_state: s.verifier,
  verifier_failure_summary:
    s.verifier === 'failed'
      ? 'Seven completed attempts lack provider resource observations.'
      : undefined,
  model_coverage: evaluatedVariants.map((v) => v.variant_id),
  metric_summaries: {
    pass_rate: { value: s.verifier === 'passed' ? 0.74 : 0.61 },
    latency_p50_ms: { value: 640 },
  },
  limitations:
    s.verifier === 'failed'
      ? ['Verification failed: missing resource observations for seven attempts.']
      : [],
}));

export const fixtureEvaluationSummaries: EvaluationSummary[] = suiteDefs.map((s) => ({
  schema_version: '1.2.0',
  kind: 'evaluation_summary',
  dataset_id: EXPLO_DATASET,
  quality_state: s.verifier === 'passed' ? 'exploratory_verified' : 'exploratory_partial',
  observed_at: NOW,
  source_revision_label: 'overnight-baseline/run-20260914-r2',
  run_id: `run-${s.suite_id}-20260914-r2`,
  campaign_id: 'overnight-baseline-20260914-r2',
  suite_id: s.suite_id,
  arm: 'ensemble_ungoverned',
  evaluation_unit: 'model',
  primary_invocation_share: { unavailable_reason: 'not observed in this dataset' },
  correlated_failure_rate: { unavailable_reason: 'no comparable correlated-failure observations' },
  benchmark_unavailable_reasons: ['Stack routing and correlated-failure observations were not captured in this baseline.'],
  model_role_mapping: {
    primary: 'gemma4-e4b',
    assistant: 'gemma4-e2b',
    lite: 'qwen25-05b',
  },
  lifecycle_state: 'completed',
  assignment_total: 125,
  assignment_completed: 125,
  assignment_failed: 0,
  terminal_outcomes: { completed: 125, model_failed: 0, grader_failed: 0, invalid_evidence: 0, stopped: 0 },
  started_at: '2026-09-14T00:00:00Z',
  ended_at: '2026-09-14T06:00:00Z',
  elapsed_seconds: 21600,
  verifier_state: s.verifier,
  verifier_failure_summary:
    s.verifier === 'failed'
      ? 'Seven completed attempts lack provider resource observations.'
      : undefined,
  headline_metrics: {
    pass_rate: { value: s.verifier === 'passed' ? 0.74 : 0.61 },
    latency_p50_ms: { value: 640 },
    throughput: { value: 44.2 },
  },
  evidence_link: s.verifier === 'passed' ? `proof://run-${s.suite_id}-20260914-r2` : undefined,
}));

export const fixtureAssignmentResults: AssignmentResult[] = [
  {
    schema_version: '1.2.0',
    kind: 'assignment_result',
    dataset_id: EXPLO_DATASET,
    quality_state: 'exploratory_verified',
    observed_at: NOW,
    assignment_id: 'asg-ifeval-001',
    run_id: 'run-ifeval-20260914-r2',
    task_id: 'task-ifeval-001',
    variant_id: 'gemma4-e4b',
    role: 'primary',
    repetition: 1,
    scenario_category: 'instruction_adherence',
    evaluation_unit: 'model',
    benchmark_observations: {
      escalation_disposition: 'correct_autonomous_completion',
      tool_scorecard: {
        tool_recognition: { unavailable_reason: 'tool use not required by this scenario' },
        tool_selection: { unavailable_reason: 'tool use not required by this scenario' },
      },
      security_privacy_events: {
        sensitive_data_present: 0,
        sensitive_data_required: 0,
        sensitive_data_sent_externally: 0,
        unnecessary_data_sent_externally: 0,
        policy_prevented_disclosure: 0,
        model_attempted_unauthorized_access: 0,
        tool_attempted_unauthorized_operation: 0,
        authorization_correctly_enforced: 1,
        audit_record_complete: 0,
        audit_record_tampered: 0,
        secret_redaction_successful: 0,
      },
      timing: {
        model_load_ms: { unavailable_reason: 'not observed in this dataset' },
        time_to_first_token_ms: { unavailable_reason: 'not observed in this dataset' },
        generation_ms: { value: 780 },
        whole_task_ms: { unavailable_reason: 'not observed in this dataset' },
      },
      gpu: {
        vram_peak_bytes: { unavailable_reason: 'remote GPU observation unavailable' },
        power_watts: { unavailable_reason: 'remote GPU observation unavailable' },
      },
      unavailable_reasons: ['Tool detail, cold-start timing, and GPU telemetry were not observed in this dataset.'],
    },
    terminal_status: 'completed',
    metric_values: { pass: { value: 1 }, latency_ms: { value: 780 } },
    stage_summary: [
      { name: 'bootstrap', duration_seconds: 2.1 },
      { name: 'inference', duration_seconds: 1.4 },
      { name: 'verify', duration_seconds: 0.3 },
    ],
    resource_summary: {
      latency_ms: { value: 780 },
      input_tokens: { value: 420 },
      output_tokens: { value: 180 },
      retries: { value: 0 },
    },
    verification_disposition: 'passed',
  },
  {
    schema_version: '1.2.0',
    kind: 'assignment_result',
    dataset_id: EXPLO_DATASET,
    quality_state: 'exploratory_partial',
    observed_at: NOW,
    assignment_id: 'asg-arc-challenge-007',
    run_id: 'run-arc-challenge-20260914-r2',
    task_id: 'task-arc-challenge-007',
    variant_id: 'gemma4-e4b',
    role: 'primary',
    repetition: 1,
    scenario_category: 'technical_analysis',
    evaluation_unit: 'model',
    benchmark_observations: {
      unavailable_reasons: ['Escalation, decomposed tool, security, cold-start, GPU, and correlated-failure observations were not captured.'],
    },
    terminal_status: 'completed',
    metric_values: { pass: { value: 0 }, latency_ms: { unavailable_reason: 'resource observation missing' } },
    missingness_reason: 'Provider resource observation missing for this completed attempt.',
    stage_summary: [
      { name: 'bootstrap', duration_seconds: 1.8 },
      { name: 'inference', duration_seconds: 1.1 },
    ],
    verification_disposition: 'failed',
  },
];

export const fixtureMethodology: MethodologySnapshot = {
  schema_version: '1.2.0',
  kind: 'methodology_snapshot',
  dataset_id: EXPLO_DATASET,
  quality_state: 'exploratory_partial',
  observed_at: NOW,
  metric_definitions: [
    {
      key: 'pass_rate',
      name: 'Pass rate',
      unit: 'proportion',
      direction: 'higher_is_better',
      denominator: 'eligible assignments with a terminal metric',
      missing_value_behavior: 'excluded from the denominator; never rendered as zero',
      aggregation: 'mean over eligible assignments within a suite and role',
      uncertainty_method: 'bootstrap confidence interval (95%)',
      explanation:
        'The fraction of eligible assignments that passed. The confidence interval reflects sampling uncertainty from the bootstrap, not a superiority claim.',
    },
    {
      key: 'agreement_pairwise',
      name: 'Pairwise agreement',
      unit: 'proportion',
      direction: 'higher_is_better',
      denominator: 'pairs of repetitions for the same task',
      missing_value_behavior: 'rendered Unavailable when fewer than two repetitions exist',
      aggregation: 'mean pairwise agreement across tasks',
      uncertainty_method: 'none (descriptive)',
      explanation:
        'How often two repetitions of the same task agree on the outcome. Higher means more consistent results.',
    },
    {
      key: 'latency_p50_ms',
      name: 'Latency p50',
      unit: 'milliseconds',
      direction: 'lower_is_better',
      denominator: 'completed assignments with a resource observation',
      missing_value_behavior: 'rendered Unavailable when resource observation is missing',
      aggregation: '50th percentile (median)',
      uncertainty_method: 'none (descriptive)',
      explanation:
        'The median time to first response. Lower is faster.',
    },
    {
      key: 'output_throughput_p50',
      name: 'Output throughput p50',
      unit: 'tokens/second',
      direction: 'higher_is_better',
      denominator: 'completed assignments with a resource observation',
      missing_value_behavior: 'rendered Unavailable when resource observation is missing',
      aggregation: '50th percentile (median)',
      uncertainty_method: 'none (descriptive)',
      explanation:
        'The median number of output tokens generated per second. Higher is faster generation.',
    },
  ],
  suite_definitions: suiteDefs.map((s) => ({
    suite_id: s.suite_id,
    display_name: s.display_name,
    task_count: s.task_count,
    description: `${s.display_name} evaluation suite with ${s.task_count} tasks per role candidate.`,
  })),
  limitations: [
    'Exploratory baseline is not publication-eligible.',
    'Four suites fail verification due to seven missing resource observations.',
    'Intervals are exploratory confidence bounds, not certified claims.',
  ],
};

// Deterministic live event sequence for the scripted evaluation scenario.
export const fixtureLiveEvents: LiveEvent[] = [
  {
    schema_version: '1.2.0',
    kind: 'evaluation_queued',
    dataset_id: LIVE_DATASET,
    quality_state: 'live_in_progress',
    observed_at: '2026-09-14T08:00:00Z',
    event_id: 'evt-001',
    run_id: 'run-live-demo-20260914',
    lifecycle_status: 'queued',
    completed: 0,
    total: 5,
  },
  {
    schema_version: '1.2.0',
    kind: 'evaluation_started',
    dataset_id: LIVE_DATASET,
    quality_state: 'live_in_progress',
    observed_at: '2026-09-14T08:00:05Z',
    event_id: 'evt-002',
    run_id: 'run-live-demo-20260914',
    lifecycle_status: 'running',
    completed: 0,
    total: 5,
  },
  {
    schema_version: '1.2.0',
    kind: 'assignment_started',
    dataset_id: LIVE_DATASET,
    quality_state: 'live_in_progress',
    observed_at: '2026-09-14T08:00:06Z',
    event_id: 'evt-003',
    run_id: 'run-live-demo-20260914',
    assignment_id: 'asg-live-001',
    task_id: 'task-ifeval-001',
    variant_id: 'gemma4-e4b',
    lifecycle_status: 'running',
    completed: 0,
    total: 5,
    stage_label: 'bootstrap',
  },
  {
    schema_version: '1.2.0',
    kind: 'stage_updated',
    dataset_id: LIVE_DATASET,
    quality_state: 'live_in_progress',
    observed_at: '2026-09-14T08:00:08Z',
    event_id: 'evt-004',
    run_id: 'run-live-demo-20260914',
    assignment_id: 'asg-live-001',
    lifecycle_status: 'running',
    completed: 0,
    total: 5,
    stage_label: 'inference',
  },
  {
    schema_version: '1.2.0',
    kind: 'assignment_completed',
    dataset_id: LIVE_DATASET,
    quality_state: 'live_in_progress',
    observed_at: '2026-09-14T08:00:10Z',
    event_id: 'evt-005',
    run_id: 'run-live-demo-20260914',
    assignment_id: 'asg-live-001',
    task_id: 'task-ifeval-001',
    variant_id: 'gemma4-e4b',
    lifecycle_status: 'running',
    completed: 1,
    total: 5,
    metric_delta: { pass: { value: 1 } },
  },
  {
    schema_version: '1.2.0',
    kind: 'evaluation_completed',
    dataset_id: LIVE_DATASET,
    quality_state: 'verified_public',
    observed_at: '2026-09-14T08:00:30Z',
    event_id: 'evt-006',
    run_id: 'run-live-demo-20260914',
    lifecycle_status: 'completed',
    completed: 5,
    total: 5,
    metric_delta: { pass_rate: { value: 0.8 } },
  },
  // Committed metric delta published after an assignment's results are
  // aggregated. Covers the metric_updated event kind in the success path.
  {
    schema_version: '1.2.0',
    kind: 'metric_updated',
    dataset_id: LIVE_DATASET,
    quality_state: 'live_in_progress',
    observed_at: '2026-09-14T08:00:12Z',
    event_id: 'evt-007',
    run_id: 'run-live-demo-20260914',
    assignment_id: 'asg-live-001',
    variant_id: 'gemma4-e4b',
    lifecycle_status: 'running',
    completed: 1,
    total: 5,
    metric_delta: { pass_rate: { value: 1.0 } },
  },
  // Failure scenario: a second live run where one assignment fails and the
  // run terminates in a typed failure. Covers assignment_failed and
  // evaluation_failed. The run uses a distinct id so it never collides with
  // the success scenario.
  {
    schema_version: '1.2.0',
    kind: 'assignment_failed',
    dataset_id: LIVE_DATASET,
    quality_state: 'terminal_failed',
    observed_at: '2026-09-14T08:01:20Z',
    event_id: 'evt-fail-001',
    run_id: 'run-live-demo-failed-20260914',
    assignment_id: 'asg-live-fail-001',
    task_id: 'task-ifeval-002',
    variant_id: 'gemma4-e2b',
    lifecycle_status: 'failed',
    completed: 0,
    total: 5,
  },
  {
    schema_version: '1.2.0',
    kind: 'evaluation_failed',
    dataset_id: LIVE_DATASET,
    quality_state: 'terminal_failed',
    observed_at: '2026-09-14T08:01:40Z',
    event_id: 'evt-fail-002',
    run_id: 'run-live-demo-failed-20260914',
    lifecycle_status: 'failed',
    completed: 0,
    total: 5,
  },
  // Stopped scenario: a third live run stopped before a natural terminal
  // state. Covers evaluation_stopped.
  {
    schema_version: '1.2.0',
    kind: 'evaluation_stopped',
    dataset_id: LIVE_DATASET,
    quality_state: 'terminal_failed',
    observed_at: '2026-09-14T08:02:30Z',
    event_id: 'evt-stop-001',
    run_id: 'run-live-demo-stopped-20260914',
    lifecycle_status: 'stopped',
    completed: 2,
    total: 5,
  },
];

export const fixtureLiveEvaluationSummary: EvaluationSummary = {
  schema_version: '1.2.0',
  kind: 'evaluation_summary',
  dataset_id: LIVE_DATASET,
  quality_state: 'live_in_progress',
  observed_at: '2026-09-14T08:00:05Z',
  run_id: 'run-live-demo-20260914',
  suite_id: 'ifeval',
  arm: 'ensemble_ungoverned',
  evaluation_unit: 'model',
  primary_invocation_share: { unavailable_reason: 'not observed in this dataset' },
  correlated_failure_rate: { unavailable_reason: 'no comparable correlated-failure observations' },
  benchmark_unavailable_reasons: ['Stack routing and correlated-failure observations were not captured in this baseline.'],
  model_role_mapping: {
    primary: 'gemma4-e4b',
    assistant: 'gemma4-e2b',
    lite: 'qwen25-05b',
  },
  lifecycle_state: 'running',
  assignment_total: 5,
  assignment_completed: 1,
  assignment_failed: 0,
  terminal_outcomes: { completed: 1, model_failed: 0, grader_failed: 0, invalid_evidence: 0, stopped: 0 },
  started_at: '2026-09-14T08:00:05Z',
  elapsed_seconds: 5,
  verifier_state: 'not_applicable',
  headline_metrics: {},
};

// Verified public snapshot records. The verified dataset has two five-task
// IFEval runs (one 4/5, one 3/5) across three evaluated models. Resource
// observations are absent in this dataset, so token and latency metrics are
// unavailable with explicit reasons. These fixtures let every route render
// when the dataset selector switches to "Verified public snapshot".
const verifiedModels: Array<{
  variant_id: string;
  display_name: string;
  served_model_tag: string;
  role: ModelSummary['role'];
  pass_rate: ConfidenceInterval;
  coverage: number;
}> = [
  {
    variant_id: 'gemma4-e4b',
    display_name: 'Gemma 4 E4B',
    served_model_tag: 'gemma4:e4b',
    role: 'primary',
    pass_rate: { estimate: 0.625, lower: 0.35, upper: 0.85, denominator: 8 },
    coverage: 0.8,
  },
  {
    variant_id: 'gemma4-12b',
    display_name: 'Gemma 4 12B',
    served_model_tag: 'gemma4:12b',
    role: 'primary',
    pass_rate: { estimate: 1.0, lower: 0.4, upper: 1.0, denominator: 2 },
    coverage: 0.2,
  },
  {
    variant_id: 'gemma4-e2b',
    display_name: 'Gemma 4 E2B',
    served_model_tag: 'gemma4:e2b',
    role: 'assistant',
    pass_rate: { estimate: 0, lower: 0, upper: 0, denominator: 0 },
    coverage: 1.0,
  },
];

export const fixtureVerifiedModelSummaries: ModelSummary[] = verifiedModels.map((m) => ({
  schema_version: '1.2.0',
  kind: 'model_summary',
  dataset_id: VERIFIED_DATASET,
  quality_state: 'verified_public',
  observed_at: NOW,
  source_revision_label: 'docs/evidence/readme/current',
  variant_id: m.variant_id,
  display_name: m.display_name,
  served_model_tag: m.served_model_tag,
  role: m.role,
  backend_provider_class: 'ollama',
  quantization_weight_class: 'q4_k_m',
  inventory_only: false,
  evaluation_coverage: m.coverage,
  pass_rate: m.pass_rate.denominator > 0 ? m.pass_rate : undefined,
  unavailable_reasons:
    m.pass_rate.denominator === 0
      ? ['triage role only; no graded responses']
      : ['resource observations not captured in this dataset'],
}));

export const fixtureVerifiedSuiteSummary: SuiteSummary = {
  schema_version: '1.2.0',
  kind: 'suite_summary',
  dataset_id: VERIFIED_DATASET,
  quality_state: 'verified_public',
  observed_at: NOW,
  source_revision_label: 'docs/evidence/readme/current',
  suite_id: 'ifeval',
  display_name: 'IFEval (verified)',
  task_count: 5,
  assignment_count: 10,
  status: 'completed',
  verifier_state: 'passed',
  model_coverage: verifiedModels.map((m) => m.variant_id),
  metric_summaries: {
    pass_rate: { value: 0.7 },
  },
  limitations: [
    'Small scope: two five-task runs only.',
    'Resource observations not captured; latency and token metrics unavailable.',
  ],
};

export const fixtureVerifiedEvaluationSummaries: EvaluationSummary[] = [
  {
    schema_version: '1.2.0',
    kind: 'evaluation_summary',
    dataset_id: VERIFIED_DATASET,
    quality_state: 'verified_public',
    observed_at: NOW,
    source_revision_label: 'docs/evidence/readme/current',
    run_id: 'run-verified-ifeval-1',
    campaign_id: 'verified-public-20260914',
    suite_id: 'ifeval',
    arm: 'ensemble_ungoverned',
    evaluation_unit: 'model',
    primary_invocation_share: { unavailable_reason: 'not observed in this dataset' },
    correlated_failure_rate: { unavailable_reason: 'no comparable correlated-failure observations' },
    benchmark_unavailable_reasons: ['Stack routing and correlated-failure observations were not captured in this snapshot.'],
    model_role_mapping: {
      primary: 'gemma4-e4b',
      assistant: 'gemma4-e2b',
      lite: 'gemma4-12b',
    },
    lifecycle_state: 'completed',
    assignment_total: 5,
    assignment_completed: 5,
    assignment_failed: 0,
    terminal_outcomes: { completed: 5, model_failed: 0, grader_failed: 0, invalid_evidence: 0, stopped: 0 },
    started_at: '2026-09-13T22:00:00Z',
    ended_at: '2026-09-13T22:30:00Z',
    elapsed_seconds: 1800,
    verifier_state: 'passed',
    headline_metrics: { pass_rate: { value: 0.8 } },
    evidence_link: 'checksum://verified-ifeval-1',
  },
  {
    schema_version: '1.2.0',
    kind: 'evaluation_summary',
    dataset_id: VERIFIED_DATASET,
    quality_state: 'verified_public',
    observed_at: NOW,
    source_revision_label: 'docs/evidence/readme/current',
    run_id: 'run-verified-ifeval-2',
    campaign_id: 'verified-public-20260914',
    suite_id: 'ifeval',
    arm: 'ensemble_ungoverned',
    evaluation_unit: 'model',
    primary_invocation_share: { unavailable_reason: 'not observed in this dataset' },
    correlated_failure_rate: { unavailable_reason: 'no comparable correlated-failure observations' },
    benchmark_unavailable_reasons: ['Stack routing and correlated-failure observations were not captured in this snapshot.'],
    model_role_mapping: {
      primary: 'gemma4-e4b',
      assistant: 'gemma4-e2b',
      lite: 'gemma4-12b',
    },
    lifecycle_state: 'completed',
    assignment_total: 5,
    assignment_completed: 5,
    assignment_failed: 0,
    terminal_outcomes: { completed: 5, model_failed: 0, grader_failed: 0, invalid_evidence: 0, stopped: 0 },
    started_at: '2026-09-13T23:00:00Z',
    ended_at: '2026-09-13T23:30:00Z',
    elapsed_seconds: 1800,
    verifier_state: 'passed',
    headline_metrics: { pass_rate: { value: 0.6 } },
    evidence_link: 'checksum://verified-ifeval-2',
  },
];

function verifiedAssignment(
  runId: string,
  index: number,
  variantId: string,
  passed: boolean,
): AssignmentResult {
  return {
    schema_version: '1.2.0',
    kind: 'assignment_result',
    dataset_id: VERIFIED_DATASET,
    quality_state: 'verified_public',
    observed_at: NOW,
    assignment_id: `asg-${runId}-${String(index).padStart(3, '0')}`,
    run_id: runId,
    task_id: `task-ifeval-${String(index).padStart(3, '0')}`,
    variant_id: variantId,
    role: 'primary',
    repetition: 1,
    scenario_category: 'instruction_adherence',
    evaluation_unit: 'model',
    benchmark_observations: {
      unavailable_reasons: ['Escalation, decomposed tool, security, timing, GPU, and correlated-failure observations were not captured.'],
    },
    terminal_status: 'completed',
    metric_values: { pass: { value: passed ? 1 : 0 } },
    missingness_reason: 'Resource observations not captured in this dataset.',
    stage_summary: [],
    verification_disposition: 'passed',
  };
}

// Run 1: 4/5 pass (gemma4-e4b graded 8 tasks across both runs; this run uses 4 e4b + 1 12b)
export const fixtureVerifiedAssignmentResults: AssignmentResult[] = [
  verifiedAssignment('run-verified-ifeval-1', 1, 'gemma4-e4b', true),
  verifiedAssignment('run-verified-ifeval-1', 2, 'gemma4-e4b', true),
  verifiedAssignment('run-verified-ifeval-1', 3, 'gemma4-e4b', true),
  verifiedAssignment('run-verified-ifeval-1', 4, 'gemma4-e4b', true),
  verifiedAssignment('run-verified-ifeval-1', 5, 'gemma4-12b', false),
  // Run 2: 3/5 pass
  verifiedAssignment('run-verified-ifeval-2', 1, 'gemma4-e4b', true),
  verifiedAssignment('run-verified-ifeval-2', 2, 'gemma4-e4b', true),
  verifiedAssignment('run-verified-ifeval-2', 3, 'gemma4-e4b', true),
  verifiedAssignment('run-verified-ifeval-2', 4, 'gemma4-12b', false),
  verifiedAssignment('run-verified-ifeval-2', 5, 'gemma4-12b', true),
];

export const allFixtureSnapshotRecords = [
  fixtureCatalogExploratory,
  fixtureCatalogVerified,
  fixtureCatalogLive,
  ...fixtureModelSummaries,
  ...fixtureUnevaluatedModels,
  ...fixtureInventoryOnlyModels,
  ...fixtureSuiteSummaries,
  ...fixtureEvaluationSummaries,
  ...fixtureVerifiedModelSummaries,
  fixtureVerifiedSuiteSummary,
  ...fixtureVerifiedEvaluationSummaries,
  fixtureLiveEvaluationSummary,
  ...fixtureAssignmentResults,
  ...fixtureVerifiedAssignmentResults,
  fixtureMethodology,
];
