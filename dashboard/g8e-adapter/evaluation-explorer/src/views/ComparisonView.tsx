// Comparison view. Multi-select comparison for two to four models using the
// same dataset and compatible metrics only. Never combines incompatible
// datasets or denominators.

import { useSearchParams, Link } from 'react-router-dom';
import { useActiveDatasetId } from '../state/dataset';
import { recordKey, useStoreState } from '../state/store';
import {
  EmptyState,
  ErrorState,
  IntervalDisplay,
  QualityBadge,
  SectionHeading,
  UnavailableValue,
  formatPercent,
  formatLatency,
  formatThroughput,
  formatTokens,
} from '../components/shared';

export function ComparisonView() {
  const [params] = useSearchParams();
  const activeDatasetId = useActiveDatasetId(params.get('dataset') ?? undefined);
  const modelIds = (params.get('models') ?? '').split(',').filter(Boolean);

  const models = useStoreState((state) =>
    modelIds
      .map((id) => state.models.get(recordKey(activeDatasetId, id)))
      .filter((m): m is NonNullable<typeof m> => m !== undefined),
  );

  if (modelIds.length < 2) {
    return (
      <div className="comparison-view">
        <SectionHeading
          kicker="COMPARISON"
          title="Compare models"
          description="Select two to four models from the model catalog to compare. Comparison uses one dataset and compatible metrics only."
        />
        <EmptyState hasRecords={false} hasFilters={false} connection="live" />
        <Link to={`/models?dataset=${activeDatasetId}`}>Go to model catalog</Link>
      </div>
    );
  }

  if (models.length < 2) {
    return <ErrorState message="Some selected models were not found in the active dataset." />;
  }

  const datasets = new Set(models.map((m) => m.dataset_id));
  if (datasets.size > 1) {
    return (
      <ErrorState message="Selected models come from different datasets. Comparison never combines incompatible datasets." />
    );
  }

  return (
    <div className="comparison-view">
      <SectionHeading
        kicker="COMPARISON"
        title="Model comparison"
        description={`Comparing ${models.length} models from the same dataset. This is not a superiority claim.`}
      />
      <Link to={`/models?dataset=${activeDatasetId}`}>Back to model catalog</Link>

      <section className="comparison-table">
        <h2>Side-by-side</h2>
        <table className="compare-table">
          <thead>
            <tr>
              <th scope="col">Metric</th>
              {models.map((m) => (
                <th key={m.variant_id} scope="col">
                  <Link to={`/models/${activeDatasetId}/${m.variant_id}`}>{m.display_name}</Link>
                  <QualityBadge state={m.quality_state} />
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            <tr>
              <th scope="row">Role</th>
              {models.map((m) => <td key={m.variant_id}>{m.role}</td>)}
            </tr>
            <tr>
              <th scope="row">Coverage</th>
              {models.map((m) => <td key={m.variant_id}>{formatPercent(m.evaluation_coverage)}</td>)}
            </tr>
            <tr>
              <th scope="row">Pass rate</th>
              {models.map((m) => (
                <td key={m.variant_id}>
                  {m.pass_rate ? <IntervalDisplay interval={m.pass_rate} label="pass rate" /> : <UnavailableValue />}
                </td>
              ))}
            </tr>
            <tr>
              <th scope="row">Agreement pairwise</th>
              {models.map((m) => (
                <td key={m.variant_id}>
                  {m.agreement_pairwise?.value !== undefined ? formatPercent(m.agreement_pairwise.value) : <UnavailableValue />}
                </td>
              ))}
            </tr>
            <tr>
              <th scope="row">Latency p50</th>
              {models.map((m) => (
                <td key={m.variant_id}>
                  {m.latency_p50_ms?.value !== undefined ? formatLatency(m.latency_p50_ms.value) : <UnavailableValue />}
                </td>
              ))}
            </tr>
            <tr>
              <th scope="row">Throughput p50</th>
              {models.map((m) => (
                <td key={m.variant_id}>
                  {m.output_throughput_p50?.value !== undefined ? formatThroughput(m.output_throughput_p50.value) : <UnavailableValue />}
                </td>
              ))}
            </tr>
            <tr>
              <th scope="row">Output tokens</th>
              {models.map((m) => (
                <td key={m.variant_id}>
                  {m.output_tokens?.value !== undefined ? formatTokens(m.output_tokens.value) : <UnavailableValue />}
                </td>
              ))}
            </tr>
          </tbody>
        </table>
      </section>
    </div>
  );
}
