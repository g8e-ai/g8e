// Inline side-by-side comparison for two to four models from the same dataset.

import { Link } from 'react-router-dom';
import { modelComparisonId } from '../state/store';
import { roleLabel } from '../views/derived';
import {
  IntervalDisplay,
  QualityBadge,
  UnavailableValue,
  formatPercent,
  formatLatency,
  formatThroughput,
  formatTokens,
} from './shared';
import type { ModelSummary } from '../contract/types';

export function ModelComparisonPanel({
  models,
  datasetId,
  onClear,
}: {
  models: ModelSummary[];
  datasetId: string;
  onClear: () => void;
}) {
  return (
    <section className="comparison-panel" aria-labelledby="comparison-heading">
      <div className="comparison-panel-header">
        <div>
          <h2 id="comparison-heading">Side-by-side comparison</h2>
          <p className="comparison-panel-lede">
            Comparing {models.length} models from the same dataset. This is not a superiority claim.
          </p>
        </div>
        <button type="button" className="comparison-clear-btn" onClick={onClear}>
          Clear comparison
        </button>
      </div>

      <div className="table-scroll">
        <table className="compare-table">
          <caption className="sr-only">Model comparison for {models.length} selected models</caption>
          <thead>
            <tr>
              <th scope="col">Metric</th>
              {models.map((m) => (
                <th key={modelComparisonId(m)} scope="col">
                  <Link to={`/models/${datasetId}/${m.variant_id}?role=${m.role}`}>{m.display_name}</Link>
                  <QualityBadge state={m.quality_state} />
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            <tr>
              <th scope="row">Role</th>
              {models.map((m) => <td key={modelComparisonId(m)}>{roleLabel(m.role)}</td>)}
            </tr>
            <tr>
              <th scope="row">Coverage</th>
              {models.map((m) => <td key={modelComparisonId(m)}>{formatPercent(m.evaluation_coverage)}</td>)}
            </tr>
            <tr>
              <th scope="row">Pass rate</th>
              {models.map((m) => (
                <td key={modelComparisonId(m)}>
                  {m.pass_rate ? <IntervalDisplay interval={m.pass_rate} label="pass rate" /> : <UnavailableValue />}
                </td>
              ))}
            </tr>
            <tr>
              <th scope="row">Agreement pairwise</th>
              {models.map((m) => (
                <td key={modelComparisonId(m)}>
                  {m.agreement_pairwise?.value !== undefined ? formatPercent(m.agreement_pairwise.value) : <UnavailableValue />}
                </td>
              ))}
            </tr>
            <tr>
              <th scope="row">Latency p50</th>
              {models.map((m) => (
                <td key={modelComparisonId(m)}>
                  {m.latency_p50_ms?.value !== undefined ? formatLatency(m.latency_p50_ms.value) : <UnavailableValue />}
                </td>
              ))}
            </tr>
            <tr>
              <th scope="row">Throughput p50</th>
              {models.map((m) => (
                <td key={modelComparisonId(m)}>
                  {m.output_throughput_p50?.value !== undefined ? formatThroughput(m.output_throughput_p50.value) : <UnavailableValue />}
                </td>
              ))}
            </tr>
            <tr>
              <th scope="row">Output tokens</th>
              {models.map((m) => (
                <td key={modelComparisonId(m)}>
                  {m.output_tokens?.value !== undefined ? formatTokens(m.output_tokens.value) : <UnavailableValue />}
                </td>
              ))}
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  );
}
