// Derives explorer aggregate view records (catalog, model, methodology) from
// campaign lifecycle and terminal result projections. Live runs do not yet
// publish these snapshots from the Go projector; the adapter materializes them
// during mirror ingest so Models, Compare, and Docs populate honestly.

import {
  VIEW_SCHEMA_VERSION,
  type CatalogSnapshot,
  type MethodologySnapshot,
  type ModelRole,
  type ModelSummary,
  type SnapshotRecord,
  type TerminalStatus,
} from '../contract/types';
const CAMPAIGN_SOURCE_REVISION = 'g8e-eval-campaign';

export interface VariantRoleBucket {
  variantId: string;
  role: ModelRole;
  scheduledIds: Set<string>;
  terminalIds: Set<string>;
  passed: number;
  failed: number;
  outcomes: Partial<Record<TerminalStatus, number>>;
}

const BENCHMARK_SUITE_ID = 'north-star-25';

export function variantRoleStatsKey(runId: string, variantId: string, role: ModelRole): string {
  return `${runId}:${variantId}:${role}`;
}

export function ensureVariantRoleBucket(
  variantRoleStats: Map<string, VariantRoleBucket>,
  runId: string,
  assignmentId: string,
  variantId: string,
  role: ModelRole,
): VariantRoleBucket {
  const key = variantRoleStatsKey(runId, variantId, role);
  let bucket = variantRoleStats.get(key);
  if (!bucket) {
    bucket = {
      variantId,
      role,
      scheduledIds: new Set(),
      terminalIds: new Set(),
      passed: 0,
      failed: 0,
      outcomes: {},
    };
    variantRoleStats.set(key, bucket);
  }
  bucket.scheduledIds.add(assignmentId);
  return bucket;
}

export function recordVariantRoleTerminal(
  variantRoleStats: Map<string, VariantRoleBucket>,
  runId: string,
  assignmentId: string,
  variantId: string,
  role: ModelRole,
  passed: boolean,
  terminalStatus: TerminalStatus,
): void {
  const bucket = ensureVariantRoleBucket(variantRoleStats, runId, assignmentId, variantId, role);
  if (bucket.terminalIds.has(assignmentId)) return;
  bucket.terminalIds.add(assignmentId);
  if (passed) {
    bucket.passed += 1;
  } else {
    bucket.failed += 1;
  }
  bucket.outcomes[terminalStatus] = (bucket.outcomes[terminalStatus] ?? 0) + 1;
}

interface RunProgressCounts {
  scheduled: number;
  matrixTotal: number;
  terminal: number;
  passed: number;
  failed: number;
}

export function buildCampaignAggregateRecords(
  variantRoleStats: Map<string, VariantRoleBucket>,
  runTotals: Map<string, RunProgressCounts>,
  runId: string,
  datasetId: string,
  observedAt: string,
): SnapshotRecord[] {
  const progress = runTotals.get(runId);
  if (!progress || (progress.matrixTotal === 0 && progress.scheduled === 0 && progress.terminal === 0)) {
    return [];
  }

  const records: SnapshotRecord[] = [];
  const catalog = buildCatalogSnapshot(variantRoleStats, runId, datasetId, observedAt, progress);
  records.push(catalog);

  for (const [key, bucket] of variantRoleStats.entries()) {
    if (!key.startsWith(`${runId}:`)) continue;
    records.push(buildModelSummary(datasetId, observedAt, bucket));
  }

  records.push(buildMethodologySnapshot(datasetId, observedAt));
  return records;
}

function buildCatalogSnapshot(
  variantRoleStats: Map<string, VariantRoleBucket>,
  runId: string,
  datasetId: string,
  observedAt: string,
  progress: RunProgressCounts,
): CatalogSnapshot {
  const variantIds = new Set<string>();
  for (const [key, bucket] of variantRoleStats.entries()) {
    if (!key.startsWith(`${runId}:`)) continue;
    variantIds.add(bucket.variantId);
  }

  const uniqueEvaluatedModels = new Set<string>();
  for (const [key, bucket] of variantRoleStats.entries()) {
    if (!key.startsWith(`${runId}:`) || bucket.terminalIds.size === 0) continue;
    uniqueEvaluatedModels.add(bucket.variantId);
  }

  return {
    schema_version: VIEW_SCHEMA_VERSION,
    kind: 'catalog_snapshot',
    dataset_id: datasetId,
    dataset_kind: 'live_run',
    quality_state: 'live_in_progress',
    observed_at: observedAt,
    source_revision_label: CAMPAIGN_SOURCE_REVISION,
    title: `Live smoke run (${runId})`,
    description:
      'Homogeneous full-pipeline model-role evaluation over the frozen 25-scenario agent benchmark catalog. Values are provisional while assignments are still executing.',
    limitations: [
      'Live values are provisional and update as assignments complete.',
      'Model aggregates reflect designated role responsibility inside the production chat pipeline, not a provider-only benchmark.',
      'Resource telemetry remains unavailable until provider-boundary observation is published.',
    ],
    model_count: variantIds.size,
    evaluated_count: uniqueEvaluatedModels.size,
    suite_count: 1,
    run_count: 1,
    assignment_count: progress.matrixTotal > 0 ? progress.matrixTotal : progress.scheduled,
    provider_request_count: progress.terminal,
    provider_token_count: 0,
    retry_count: 0,
    verifier_passed_count: 0,
    verifier_failed_count: 0,
    generated_at: observedAt,
  };
}

function buildModelSummary(datasetId: string, observedAt: string, bucket: VariantRoleBucket): ModelSummary {
  const scheduled = bucket.scheduledIds.size;
  const terminal = bucket.terminalIds.size;
  const coverage = scheduled > 0 ? terminal / scheduled : 0;
  const passEstimate = terminal > 0 ? bucket.passed / terminal : undefined;

  return {
    schema_version: VIEW_SCHEMA_VERSION,
    kind: 'model_summary',
    dataset_id: datasetId,
    quality_state: terminal > 0 ? 'live_in_progress' : 'not_evaluated',
    observed_at: observedAt,
    source_revision_label: CAMPAIGN_SOURCE_REVISION,
    variant_id: bucket.variantId,
    display_name: displayNameForVariant(bucket.variantId),
    served_model_tag: servedTagForVariant(bucket.variantId),
    role: bucket.role,
    backend_provider_class: 'ollama',
    inventory_only: terminal === 0,
    evaluation_coverage: coverage,
    pass_rate:
      passEstimate !== undefined
        ? {
            estimate: passEstimate,
            lower: passEstimate,
            upper: passEstimate,
            denominator: terminal,
          }
        : undefined,
    terminal_outcomes: terminal > 0
      ? {
          completed: bucket.outcomes.completed ?? 0,
          model_failed: bucket.outcomes.model_failed ?? 0,
          grader_failed: bucket.outcomes.grader_failed ?? 0,
          invalid_evidence: bucket.outcomes.invalid_evidence ?? 0,
          stopped: bucket.outcomes.stopped ?? 0,
        }
      : undefined,
    unavailable_reasons: terminal === 0 ? ['awaiting terminal assignments'] : undefined,
  };
}

function buildMethodologySnapshot(datasetId: string, observedAt: string): MethodologySnapshot {
  return {
    schema_version: VIEW_SCHEMA_VERSION,
    kind: 'methodology_snapshot',
    dataset_id: datasetId,
    quality_state: 'live_in_progress',
    observed_at: observedAt,
    source_revision_label: CAMPAIGN_SOURCE_REVISION,
    metric_definitions: [
      {
        key: 'pass_rate',
        name: 'Pass rate',
        unit: 'proportion',
        direction: 'higher_is_better',
        denominator: 'terminal homogeneous model-role assignments for the variant and designated role',
        missing_value_behavior: 'excluded until a terminal assignment exists; never rendered as zero',
        aggregation: 'mean over terminal assignments within the active live dataset',
        uncertainty_method: 'point estimate while the smoke campaign is in progress',
        explanation:
          'The fraction of terminal assignments that passed for one frozen model variant acting in one designated role through the production chat pipeline.',
      },
      {
        key: 'evaluation_coverage',
        name: 'Evaluation coverage',
        unit: 'proportion',
        direction: 'higher_is_better',
        denominator: 'scheduled assignments for the variant and designated role',
        missing_value_behavior: 'rendered as zero only when no assignments are scheduled',
        aggregation: 'terminal assignments divided by scheduled assignments',
        uncertainty_method: 'none (descriptive)',
        explanation:
          'How much of the scheduled smoke matrix has reached a terminal public result for this variant and role.',
      },
    ],
    suite_definitions: [
      {
        suite_id: BENCHMARK_SUITE_ID,
        display_name: 'Agent benchmark (25 scenarios)',
        task_count: 25,
        description:
          'Frozen 25-scenario catalog covering instruction adherence, tool use, analysis, routing, verification, security, recovery, and final response.',
      },
    ],
    limitations: [
      'Live smoke values are provisional until the campaign completes and verification runs.',
      'Homogeneous model-role and heterogeneous system leaderboards remain separate datasets.',
      'GPU and system efficiency metrics remain unavailable until provider-boundary observation is published.',
    ],
  };
}

function displayNameForVariant(variantId: string): string {
  return variantId
    .split('-')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ');
}

function servedTagForVariant(variantId: string): string {
  const parts = variantId.split('-');
  if (parts.length >= 2) {
    const family = parts.slice(0, -1).join('');
    const size = parts[parts.length - 1];
    return `${family}:${size}`;
  }
  return variantId;
}
