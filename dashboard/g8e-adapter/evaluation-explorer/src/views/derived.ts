// Derived read-model helpers for the detail views. These compute display
// aggregates from normalized store records — never from transport payloads.
// Every derivation is pure and deterministic so Worker 7 can unit-test it.
//
// Honesty rules enforced here:
//   - A metric with no observed values is undefined (rendered Unavailable),
//     never zero.
//   - Model-to-run and model-to-suite joins go through
//     assignment_result.variant_id and evaluation_summary.suite_id. The
//     exploratory runs' declared model_role_mapping is a template default
//     (every run declares the same primary), so it is not a join key.
//   - Terminal failures stay in denominators according to the metric: the
//     pass-rate denominator counts assignments whose `pass` metric was
//     observed; model_failed assignments have no pass metric and are
//     reported separately, not silently dropped.

import type {
  AssignmentResult,
  CatalogSnapshot,
  EvaluationSummary,
  LiveEvent,
  ModelRole,
  ModelSummary,
  QualityState,
  SuiteSummary,
  TerminalStatus,
  VerifierState,
} from '../contract/types';

const FAILURE_TERMINAL_STATUSES = new Set<TerminalStatus>([
  'model_failed',
  'grader_failed',
  'invalid_evidence',
  'stopped',
]);

/** True when the assignment ended in a preserved failure outcome. */
export function isFailureTerminalStatus(status: TerminalStatus | 'running' | 'queued'): boolean {
  return FAILURE_TERMINAL_STATUSES.has(status as TerminalStatus);
}

/** Terminal assignment progress for live campaign runs. */
export function campaignTerminalProgress(run: EvaluationSummary): { done: number; total: number } {
  const done = run.assignment_completed + run.assignment_failed;
  const total = run.assignment_total > 0 ? run.assignment_total : done;
  return { done, total };
}

/** Lifecycle events for one assignment, oldest first. */
export function assignmentLifecycleEvents(
  assignmentId: string,
  events: LiveEvent[],
): LiveEvent[] {
  return events
    .filter((event) => event.assignment_id === assignmentId)
    .sort((a, b) => a.observed_at.localeCompare(b.observed_at));
}

/** Linear-interpolation percentile over an unsorted list of observed values. */
export function percentile(values: number[], p: number): number | undefined {
  const observed = values.filter((v) => Number.isFinite(v));
  if (observed.length === 0) return undefined;
  observed.sort((a, b) => a - b);
  const pos = (observed.length - 1) * p;
  const lower = Math.floor(pos);
  const upper = Math.ceil(pos);
  const lowerValue = observed[lower];
  const upperValue = observed[upper];
  if (lowerValue === undefined || upperValue === undefined) return undefined;
  if (lower === upper) return lowerValue;
  return lowerValue + (upperValue - lowerValue) * (pos - lower);
}

export function sumDefined(values: Array<number | undefined>): { total: number; observed: number } {
  let total = 0;
  let observed = 0;
  for (const v of values) {
    if (v !== undefined && Number.isFinite(v)) {
      total += v;
      observed += 1;
    }
  }
  return { total, observed };
}

/** The observed pass metric for an assignment: 1, 0, or undefined when the
 *  attempt had no graded metric (for example a model_failed terminal). */
export function assignmentPass(assignment: AssignmentResult): number | undefined {
  return assignment.metric_values.pass?.value;
}

/** True when the assignment has no provider resource observation at all:
 *  every resource_summary metric is absent or carries only a reason. */
export function resourceObservationMissing(assignment: AssignmentResult): boolean {
  const res = assignment.resource_summary;
  if (!res) return true;
  return [res.latency_ms, res.input_tokens, res.output_tokens, res.retries].every(
    (m) => m?.value === undefined,
  );
}

/** Distinct variants that produced at least one assignment in a run, with
 *  assignment counts, ordered by count then id for deterministic output. */
export function runParticipants(
  assignments: AssignmentResult[],
): Array<{ variant_id: string; count: number }> {
  const counts = new Map<string, number>();
  for (const a of assignments) {
    counts.set(a.variant_id, (counts.get(a.variant_id) ?? 0) + 1);
  }
  return Array.from(counts.entries())
    .map(([variant_id, count]) => ({ variant_id, count }))
    .sort((a, b) => b.count - a.count || a.variant_id.localeCompare(b.variant_id));
}

/** Variants with at least one assignment in runs of the given suite. Used by
 *  the model-catalog suite filter. */
export function variantsInSuite(
  suiteId: string,
  assignments: AssignmentResult[],
  evaluations: EvaluationSummary[],
): Set<string> {
  const runIds = new Set(
    evaluations.filter((e) => e.suite_id === suiteId).map((e) => e.run_id),
  );
  const out = new Set<string>();
  for (const a of assignments) {
    if (runIds.has(a.run_id)) out.add(a.variant_id);
  }
  return out;
}

/** The runs that produced at least one assignment for a variant. This is
 *  the faithful model-to-run join (see module comment). */
export function runsForVariant(
  variantId: string,
  assignments: AssignmentResult[],
  evaluations: EvaluationSummary[],
): EvaluationSummary[] {
  const runIds = new Set(
    assignments.filter((a) => a.variant_id === variantId).map((a) => a.run_id),
  );
  return evaluations
    .filter((e) => runIds.has(e.run_id))
    .sort((a, b) => a.started_at?.localeCompare(b.started_at ?? '') ?? 0);
}

export interface ModelSuiteRow {
  suite_id: string;
  display_name: string;
  run_ids: string[];
  verifier_state?: VerifierState;
  quality_state?: QualityState;
  /** Assignments attributed to this variant in this suite. */
  total: number;
  /** Assignments with an observed pass metric (the displayed denominator). */
  eligible: number;
  passed: number;
  pass_rate?: number;
  model_failed: number;
  /** Assignments flagged with a missingness reason. */
  missing: number;
  latency_p50_ms?: number;
}

/** Per-suite results for one model, derived from its assignment records.
 *  Sorted by suite display name for deterministic ordering. */
export function modelSuiteRows(
  variantId: string,
  assignments: AssignmentResult[],
  evaluations: EvaluationSummary[],
  suites: SuiteSummary[],
): ModelSuiteRow[] {
  const suiteByRun = new Map(evaluations.map((e) => [e.run_id, e.suite_id]));
  const suiteMeta = new Map(suites.map((s) => [s.suite_id, s]));
  const groups = new Map<string, { runIds: Set<string>; items: AssignmentResult[] }>();
  for (const a of assignments) {
    if (a.variant_id !== variantId) continue;
    const suiteId = suiteByRun.get(a.run_id) ?? '';
    let group = groups.get(suiteId);
    if (!group) {
      group = { runIds: new Set(), items: [] };
      groups.set(suiteId, group);
    }
    group.runIds.add(a.run_id);
    group.items.push(a);
  }
  const rows: ModelSuiteRow[] = [];
  for (const [suiteId, group] of groups) {
    const meta = suiteMeta.get(suiteId);
    const passes = group.items.map(assignmentPass).filter((v): v is number => v !== undefined);
    const latencies = group.items
      .map((a) => a.resource_summary?.latency_ms?.value)
      .filter((v): v is number => v !== undefined);
    rows.push({
      suite_id: suiteId,
      display_name: meta?.display_name ?? (suiteId || 'Unknown suite'),
      run_ids: Array.from(group.runIds).sort(),
      verifier_state: meta?.verifier_state,
      quality_state: meta?.quality_state,
      total: group.items.length,
      eligible: passes.length,
      passed: passes.reduce((acc, v) => acc + v, 0),
      pass_rate: passes.length > 0 ? passes.reduce((acc, v) => acc + v, 0) / passes.length : undefined,
      model_failed: group.items.filter((a) => a.terminal_status === 'model_failed').length,
      missing: group.items.filter((a) => a.missingness_reason !== undefined && a.missingness_reason !== null).length,
      latency_p50_ms: percentile(latencies, 0.5),
    });
  }
  return rows.sort((a, b) => a.display_name.localeCompare(b.display_name));
}

export interface RunModelRow {
  variant_id: string;
  total: number;
  eligible: number;
  passed: number;
  pass_rate?: number;
  model_failed: number;
  missing: number;
  latency_p50_ms?: number;
  latency_p95_ms?: number;
  input_tokens?: number;
  output_tokens?: number;
  retries?: number;
}

/** Per-variant results inside one run, derived from its assignments. */
export function runModelRows(assignments: AssignmentResult[]): RunModelRow[] {
  const groups = new Map<string, AssignmentResult[]>();
  for (const a of assignments) {
    const list = groups.get(a.variant_id) ?? [];
    list.push(a);
    groups.set(a.variant_id, list);
  }
  const rows: RunModelRow[] = [];
  for (const [variant_id, items] of groups) {
    const passes = items.map(assignmentPass).filter((v): v is number => v !== undefined);
    const latencies = items
      .map((a) => a.resource_summary?.latency_ms?.value)
      .filter((v): v is number => v !== undefined);
    const input = sumDefined(items.map((a) => a.resource_summary?.input_tokens?.value));
    const output = sumDefined(items.map((a) => a.resource_summary?.output_tokens?.value));
    const retries = sumDefined(items.map((a) => a.resource_summary?.retries?.value));
    rows.push({
      variant_id,
      total: items.length,
      eligible: passes.length,
      passed: passes.reduce((acc, v) => acc + v, 0),
      pass_rate: passes.length > 0 ? passes.reduce((acc, v) => acc + v, 0) / passes.length : undefined,
      model_failed: items.filter((a) => a.terminal_status === 'model_failed').length,
      missing: items.filter((a) => a.missingness_reason !== undefined && a.missingness_reason !== null).length,
      latency_p50_ms: percentile(latencies, 0.5),
      latency_p95_ms: percentile(latencies, 0.95),
      input_tokens: input.observed > 0 ? input.total : undefined,
      output_tokens: output.observed > 0 ? output.total : undefined,
      retries: retries.observed > 0 ? retries.total : undefined,
    });
  }
  return rows.sort((a, b) => (b.pass_rate ?? -1) - (a.pass_rate ?? -1) || a.variant_id.localeCompare(b.variant_id));
}

export interface ResourceSummary {
  observed: number;
  missing: number;
  latency_p50_ms?: number;
  latency_p95_ms?: number;
  input_tokens?: number;
  output_tokens?: number;
  retries?: number;
}

/** Resource-observation aggregate over a set of assignments. `observed` and
 *  `missing` count assignments by whether any resource metric was observed,
 *  so the UI can disclose coverage honestly. */
export function resourceSummary(assignments: AssignmentResult[]): ResourceSummary {
  const latencies = assignments
    .map((a) => a.resource_summary?.latency_ms?.value)
    .filter((v): v is number => v !== undefined);
  const input = sumDefined(assignments.map((a) => a.resource_summary?.input_tokens?.value));
  const output = sumDefined(assignments.map((a) => a.resource_summary?.output_tokens?.value));
  const retries = sumDefined(assignments.map((a) => a.resource_summary?.retries?.value));
  const missing = assignments.filter(resourceObservationMissing).length;
  return {
    observed: assignments.length - missing,
    missing,
    latency_p50_ms: percentile(latencies, 0.5),
    latency_p95_ms: percentile(latencies, 0.95),
    input_tokens: input.observed > 0 ? input.total : undefined,
    output_tokens: output.observed > 0 ? output.total : undefined,
    retries: retries.observed > 0 ? retries.total : undefined,
  };
}

export interface DiagnosticsInfo {
  /** Suites whose canonical verifier failed, with the runs that produced them. */
  failedSuites: Array<{
    suite_id: string;
    display_name: string;
    verifier_failure_summary?: string;
    run_ids: string[];
  }>;
  /** Assignments carrying a missingness reason. */
  flaggedMissing: number;
  /** Assignments with no provider resource observation at all. */
  missingResourceObservations: number;
  /** Count of flagged-missing assignments per run. */
  flaggedByRun: Array<{ run_id: string; suite_id?: string; count: number }>;
}

/** Diagnostics for the selected dataset: which suites failed verification
 *  and where observations are missing, so insufficient cells are explained. */
export function datasetDiagnostics(
  suites: SuiteSummary[],
  evaluations: EvaluationSummary[],
  assignments: AssignmentResult[],
): DiagnosticsInfo {
  const runsBySuite = new Map<string, string[]>();
  for (const e of evaluations) {
    const list = runsBySuite.get(e.suite_id) ?? [];
    list.push(e.run_id);
    runsBySuite.set(e.suite_id, list);
  }
  const failedSuites = suites
    .filter((s) => s.verifier_state === 'failed')
    .map((s) => ({
      suite_id: s.suite_id,
      display_name: s.display_name,
      verifier_failure_summary: s.verifier_failure_summary,
      run_ids: (runsBySuite.get(s.suite_id) ?? []).sort(),
    }))
    .sort((a, b) => a.display_name.localeCompare(b.display_name));

  const flagged = new Map<string, number>();
  let flaggedMissing = 0;
  let missingResourceObservations = 0;
  for (const a of assignments) {
    const hasReason = a.missingness_reason !== undefined && a.missingness_reason !== null;
    const noObs = resourceObservationMissing(a);
    if (hasReason) flaggedMissing += 1;
    if (noObs) missingResourceObservations += 1;
    if (hasReason || noObs) {
      flagged.set(a.run_id, (flagged.get(a.run_id) ?? 0) + 1);
    }
  }
  const suiteByRun = new Map(evaluations.map((e) => [e.run_id, e.suite_id]));
  return {
    failedSuites,
    flaggedMissing,
    missingResourceObservations,
    flaggedByRun: Array.from(flagged.entries())
      .map(([run_id, count]) => ({ run_id, suite_id: suiteByRun.get(run_id), count }))
      .sort((a, b) => b.count - a.count || a.run_id.localeCompare(b.run_id)),
  };
}

/** Shape of the optional working-selection block on the exploratory
 *  catalog_snapshot. Proposed contract addition for Worker 0; the view reads
 *  it defensively so nothing is fabricated while the field is absent. */
export interface WorkingSelection {
  selection_id?: string;
  selection_policy?: string;
  publication_eligible?: boolean;
  roles: Partial<Record<ModelRole, string>>;
  limitations: string[];
}

function isWorkingSelectionShape(value: unknown): value is {
  selection_id?: string;
  selection_policy?: string;
  publication_eligible?: boolean;
  roles?: Record<string, unknown>;
  limitations?: unknown[];
} {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return false;
  const v = value as Record<string, unknown>;
  if (v.roles !== undefined && (typeof v.roles !== 'object' || v.roles === null || Array.isArray(v.roles))) return false;
  if (v.limitations !== undefined && !Array.isArray(v.limitations)) return false;
  return true;
}

/** Extract the recorded working configuration from a catalog record, when
 *  the dataset carries it. Returns undefined when absent — the UI must not
 *  recompute the selection and label it as the recorded one. */
export function readWorkingSelection(catalog: CatalogSnapshot | undefined): WorkingSelection | undefined {
  if (!catalog) return undefined;
  const raw = (catalog as unknown as Record<string, unknown>).working_selection;
  if (!isWorkingSelectionShape(raw)) return undefined;
  const roles: Partial<Record<ModelRole, string>> = {};
  for (const [role, variant] of Object.entries(raw.roles ?? {})) {
    if ((role === 'primary' || role === 'assistant' || role === 'lite') && typeof variant === 'string') {
      roles[role] = variant;
    }
  }
  return {
    selection_id: typeof raw.selection_id === 'string' ? raw.selection_id : undefined,
    selection_policy: typeof raw.selection_policy === 'string' ? raw.selection_policy : undefined,
    publication_eligible: typeof raw.publication_eligible === 'boolean' ? raw.publication_eligible : undefined,
    roles,
    limitations: (raw.limitations ?? []).filter((l): l is string => typeof l === 'string'),
  };
}

/** Other repetitions of the same task for the same variant within the run —
 *  the repeatability context for an assignment detail page. */
export function siblingRepetitions(
  assignment: AssignmentResult,
  assignments: AssignmentResult[],
): AssignmentResult[] {
  return assignments
    .filter(
      (a) =>
        a.assignment_id !== assignment.assignment_id &&
        a.run_id === assignment.run_id &&
        a.task_id === assignment.task_id &&
        a.variant_id === assignment.variant_id &&
        a.dataset_id === assignment.dataset_id,
    )
    .sort((a, b) => a.repetition - b.repetition);
}

/** Sum of observed provider retries for a variant — used by the model
 *  catalog retries column (model_summary carries no retry field). */
export function modelRetries(variantId: string, assignments: AssignmentResult[]): number | undefined {
  const r = sumDefined(
    assignments.filter((a) => a.variant_id === variantId).map((a) => a.resource_summary?.retries?.value),
  );
  return r.observed > 0 ? r.total : undefined;
}

/** Display label for a catalog role key. */
export function roleLabel(role: string): string {
  if (role === 'lite') return 'Light';
  return role.charAt(0).toUpperCase() + role.slice(1);
}

export interface ModelRoleLeaderboardRow {
  rank: number;
  model: ModelSummary;
  terminal: number;
  passed: number;
  failed: number;
  pass_rate?: number;
  coverage: number;
  latency_p50_ms?: number;
  throughput_p50?: number;
}

/** Homogeneous model-role leaderboard rows from measured model summaries.
 *  Inventory-only entries and variants without terminal assignments are excluded. */
export function modelRoleLeaderboardRows(
  models: ModelSummary[],
  role: ModelRole | 'all' = 'all',
): ModelRoleLeaderboardRow[] {
  const measured = models.filter((model) => {
    if (model.inventory_only || !model.pass_rate) return false;
    if (role !== 'all' && model.role !== role) return false;
    return true;
  });
  measured.sort(
    (a, b) =>
      (b.pass_rate?.estimate ?? -1) - (a.pass_rate?.estimate ?? -1) ||
      a.display_name.localeCompare(b.display_name) ||
      a.variant_id.localeCompare(b.variant_id),
  );
  return measured.map((model, index) => {
    const terminal = model.pass_rate?.denominator ?? 0;
    const passed = terminal > 0 ? Math.round((model.pass_rate?.estimate ?? 0) * terminal) : 0;
    const failed = Math.max(0, terminal - passed);
    return {
      rank: index + 1,
      model,
      terminal,
      passed,
      failed,
      pass_rate: model.pass_rate?.estimate,
      coverage: model.evaluation_coverage,
      latency_p50_ms: model.latency_p50_ms?.value,
      throughput_p50: model.output_throughput_p50?.value,
    };
  });
}

export interface SystemLeaderboardRow {
  rank: number;
  run: EvaluationSummary;
  stack_id: string;
  pass_rate?: number;
  primary_invocation_share?: number;
  correlated_failure_rate?: number;
  terminal: number;
  scheduled: number;
}

/** Heterogeneous system leaderboard rows from system evaluation summaries.
 *  Homogeneous model-role runs are excluded so denominators never mix. */
export function systemLeaderboardRows(evaluations: EvaluationSummary[]): SystemLeaderboardRow[] {
  const systemRuns = evaluations.filter(
    (run) =>
      run.evaluation_unit === 'system' ||
      run.arm.includes('heterogeneous') ||
      Boolean(run.stack_id),
  );
  systemRuns.sort(
    (a, b) =>
      (b.headline_metrics.pass_rate?.value ?? -1) - (a.headline_metrics.pass_rate?.value ?? -1) ||
      (a.stack_id ?? a.run_id).localeCompare(b.stack_id ?? b.run_id),
  );
  return systemRuns.map((run, index) => ({
    rank: index + 1,
    run,
    stack_id: run.stack_id ?? run.run_id,
    pass_rate: run.headline_metrics.pass_rate?.value,
    primary_invocation_share: run.primary_invocation_share?.value,
    correlated_failure_rate: run.correlated_failure_rate?.value,
    terminal: run.assignment_completed + run.assignment_failed,
    scheduled: run.assignment_total,
  }));
}

/** The distinct start dates (YYYY-MM-DD) across runs, for the date filter. */
export function runStartDays(evaluations: EvaluationSummary[]): string[] {
  const days = new Set<string>();
  for (const e of evaluations) {
    if (e.started_at && e.started_at.length >= 10) days.add(e.started_at.slice(0, 10));
  }
  return Array.from(days).sort();
}
