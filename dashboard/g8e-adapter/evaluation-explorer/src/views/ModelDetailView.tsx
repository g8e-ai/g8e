// Model detail view. Per the plan: identity and role metadata, data-quality
// banner and limitations, overall metric summary, pass-rate confidence
// interval, suite-by-suite table, repeatability breakdown, agreement,
// latency/TTFT/throughput, token totals, terminal outcomes, every
// evaluation run involving the model, and comparison action.

import { Link, useParams, useSearchParams } from 'react-router-dom';
import { useActiveDatasetId } from '../state/dataset';
import { modelRecordKey, useStoreState } from '../state/store';
import {
  DetailRow,
  EmptyState,
  ErrorState,
  IntervalDisplay,
  MetricCard,
  OutcomeCounts,
  QualityBadge,
  SectionHeading,
  UnavailableValue,
  formatPercent,
  formatLatency,
  formatThroughput,
  formatTokens,
  formatNumber,
} from '../components/shared';
import { modelSuiteRows, roleLabel, runsForVariant } from './derived';

export function ModelDetailView() {
  const { variantId: routeVariant, datasetId: routeDataset } = useParams();
  const [params] = useSearchParams();
  // Single-segment URLs (#/models/<variant>) resolve the variant against
  // the active dataset rather than erroring.
  const variantId = routeVariant ?? routeDataset;
  const datasetParam = routeVariant ? routeDataset : undefined;
  const activeDatasetId = useActiveDatasetId(datasetParam ?? params.get('dataset') ?? undefined);
  const roleFilter = params.get('role');

  const model = useStoreState((state) => {
    if (!variantId) return undefined;
    if (roleFilter === 'primary' || roleFilter === 'assistant' || roleFilter === 'lite') {
      return state.models.get(modelRecordKey(activeDatasetId, variantId, roleFilter));
    }
    for (const candidate of state.models.values()) {
      if (candidate.dataset_id !== activeDatasetId || candidate.variant_id !== variantId) continue;
      if (candidate.pass_rate) return candidate;
    }
    for (const candidate of state.models.values()) {
      if (candidate.dataset_id === activeDatasetId && candidate.variant_id === variantId) return candidate;
    }
    return undefined;
  });
  const suites = useStoreState((state) =>
    Array.from(state.suites.values()).filter((s) => s.dataset_id === activeDatasetId),
  );
  const evaluations = useStoreState((state) =>
    Array.from(state.evaluations.values()).filter((e) => e.dataset_id === activeDatasetId),
  );
  const assignments = useStoreState((state) =>
    variantId
      ? Array.from(state.assignments.values()).filter(
          (a) => a.variant_id === variantId && a.dataset_id === activeDatasetId,
        )
      : [],
  );
  const connection = useStoreState((state) => state.connection);

  if (!variantId) {
    return <ErrorState message="No model selected." />;
  }
  if (!model) {
    return <EmptyState hasRecords={false} hasFilters={false} connection={connection} />;
  }

  const suiteRows = modelSuiteRows(model.variant_id, assignments, evaluations, suites);
  const modelRuns = runsForVariant(model.variant_id, assignments, evaluations);

  return (
    <div className="model-detail">
      <nav className="breadcrumb" aria-label="Breadcrumb">
        <Link to={`/models?dataset=${activeDatasetId}`}>Models</Link>
        <span aria-hidden="true">/</span>
        <span>{model.display_name}</span>
      </nav>

      <SectionHeading
        kicker="MODEL DETAIL"
        title={model.display_name}
        description={model.served_model_tag ? `Served as ${model.served_model_tag}` : undefined}
      />

      <div className="quality-banner">
        <QualityBadge state={model.quality_state} />
        {model.unavailable_reasons?.map((reason, i) => (
          <span key={i} className="unavailable-reason">{reason}</span>
        ))}
      </div>

      <section className="model-identity">
        <h2>Identity</h2>
        <dl>
          <DetailRow label="Variant ID">{model.variant_id}</DetailRow>
          <DetailRow label="Display name">{model.display_name}</DetailRow>
          <DetailRow label="Served tag">{model.served_model_tag ?? <UnavailableValue />}</DetailRow>
          <DetailRow label="Role">{roleLabel(model.role)}</DetailRow>
          <DetailRow label="Backend / provider">{model.backend_provider_class ?? <UnavailableValue />}</DetailRow>
          <DetailRow label="Quantization / weight">{model.quantization_weight_class ?? <UnavailableValue />}</DetailRow>
          <DetailRow label="Inventory only">{model.inventory_only ? 'Yes' : 'No'}</DetailRow>
          <DetailRow label="Evaluation coverage">{formatPercent(model.evaluation_coverage)}</DetailRow>
        </dl>
      </section>

      <section className="model-metrics">
        <h2>Overall metrics</h2>
        {model.pass_rate ? (
          <div className="metric-block">
            <div className="metric-label">Pass rate</div>
            <IntervalDisplay interval={model.pass_rate} label="pass rate" />
          </div>
        ) : (
          <p className="not-evaluated">Not evaluated. This model has no eligible observations in the selected dataset.</p>
        )}
        <div className="metric-grid">
          <MetricCard label="Agreement pairwise" metric={model.agreement_pairwise} formatter={formatPercent} />
          <MetricCard label="Agreement all-five" metric={model.agreement_all_five} formatter={formatPercent} />
          <MetricCard label="Latency p50" metric={model.latency_p50_ms} formatter={formatLatency} />
          <MetricCard label="Latency p95" metric={model.latency_p95_ms} formatter={formatLatency} />
          <MetricCard label="Throughput p50" metric={model.output_throughput_p50} formatter={formatThroughput} />
          <MetricCard label="Throughput p95" metric={model.output_throughput_p95} formatter={formatThroughput} />
          <MetricCard label="Input tokens" metric={model.input_tokens} formatter={formatTokens} />
          <MetricCard label="Output tokens" metric={model.output_tokens} formatter={formatTokens} />
          <MetricCard label="Thinking tokens" metric={model.thinking_tokens} formatter={formatTokens} />
          <MetricCard label="Cache tokens" metric={model.cache_tokens} formatter={formatTokens} />
        </div>
      </section>

      {model.repeatability ? (
        <section className="model-repeatability">
          <h2>Repeatability</h2>
          <ul className="repeatability-list">
            <li><span>Consistently correct</span><strong>{formatNumber(model.repeatability.consistently_correct)}</strong></li>
            <li><span>Consistently wrong</span><strong>{formatNumber(model.repeatability.consistently_wrong)}</strong></li>
            <li><span>Inconsistent</span><strong>{formatNumber(model.repeatability.inconsistent)}</strong></li>
            <li><span>Insufficient</span><strong>{formatNumber(model.repeatability.insufficient)}</strong></li>
          </ul>
        </section>
      ) : null}

      <section className="model-outcomes">
        <h2>Terminal outcomes</h2>
        <OutcomeCounts outcomes={model.terminal_outcomes} />
      </section>

      <section className="model-suites">
        <h2>Suite results</h2>
        <p className="disclosure">Rows include only assignments produced by this model. Failures and missing measurements remain visible beside the eligible pass-rate denominator.</p>
        {suiteRows.length === 0 ? (
          <p>No suite results for this model in the selected dataset.</p>
        ) : (
          <div className="data-table-wrap">
            <table className="suite-table">
              <thead>
                <tr>
                  <th scope="col">Suite</th>
                  <th scope="col">Total</th>
                  <th scope="col">Eligible</th>
                  <th scope="col">Passed</th>
                  <th scope="col">Pass rate</th>
                  <th scope="col">Model failures</th>
                  <th scope="col">Missing</th>
                  <th scope="col">Latency p50</th>
                  <th scope="col">Verifier</th>
                  <th scope="col">Quality</th>
                </tr>
              </thead>
              <tbody>
                {suiteRows.map((suite) => (
                  <tr key={suite.suite_id}>
                    <td>{suite.display_name}</td>
                    <td>{formatNumber(suite.total)}</td>
                    <td>{formatNumber(suite.eligible)}</td>
                    <td>{formatNumber(suite.passed)}</td>
                    <td>{suite.pass_rate !== undefined ? formatPercent(suite.pass_rate) : <UnavailableValue />}</td>
                    <td>{formatNumber(suite.model_failed)}</td>
                    <td>{formatNumber(suite.missing)}</td>
                    <td>{suite.latency_p50_ms !== undefined ? formatLatency(suite.latency_p50_ms) : <UnavailableValue />}</td>
                    <td>{suite.verifier_state ?? <UnavailableValue />}</td>
                    <td>{suite.quality_state ? <QualityBadge state={suite.quality_state} /> : <UnavailableValue />}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <section className="model-runs">
        <h2>Evaluation runs involving this model</h2>
        {modelRuns.length === 0 ? (
          <p>No runs involve this model in the selected dataset.</p>
        ) : (
          <ul className="run-list">
            {modelRuns.map((run) => (
              <li key={run.run_id}>
                <Link to={`/evaluations/${run.dataset_id}/${run.run_id}`}>
                  <span className="run-id">{run.run_id}</span>
                  <span className="run-suite">{run.suite_id}</span>
                  <span className="run-status">{run.lifecycle_state}</span>
                  <QualityBadge state={run.quality_state} />
                </Link>
              </li>
            ))}
          </ul>
        )}
      </section>

      {assignments.length > 0 ? (
        <section className="model-assignments">
          <h2>Assignments ({formatNumber(assignments.length)})</h2>
          <ul className="assignment-list">
            {assignments.slice(0, 20).map((asg) => (
              <li key={asg.assignment_id}>
                <Link to={`/evaluations/${asg.dataset_id}/${asg.run_id}/assignments/${asg.assignment_id}`}>
                  {asg.assignment_id} · {asg.task_id} · rep {asg.repetition}
                </Link>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
    </div>
  );
}
