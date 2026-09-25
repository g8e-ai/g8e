// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Diagnostics disclosure. Surfaces the known data-quality issues for the
// selected dataset — verifier-failed suites and assignments missing
// observations — so a user can see why some cells are insufficient rather
// than trusting a bare number. Rendered as a native <details> element:
// keyboard-accessible and visible without JavaScript.

import { Link } from 'react-router-dom';
import type { DiagnosticsInfo } from './derived';
import { formatNumber } from '../components/shared';

export function DiagnosticsDisclosure({
  diagnostics,
  datasetId,
}: {
  diagnostics: DiagnosticsInfo;
  datasetId: string;
}) {
  const issueCount = diagnostics.failedSuites.length;
  const missingCount = Math.max(diagnostics.flaggedMissing, diagnostics.missingResourceObservations);
  if (issueCount === 0 && missingCount === 0) return null;

  return (
    <details
      className="diagnostics-disclosure"
      data-testid="diagnostics-disclosure"
      style={{
        background: 'var(--bg-elev)',
        border: '1px solid var(--tone-warn)',
        borderRadius: 'var(--radius)',
        padding: '10px 14px',
        margin: '0 0 20px',
      }}
    >
      <summary style={{ cursor: 'pointer', fontWeight: 600, color: 'var(--tone-warn)' }}>
        Diagnostics: {formatNumber(issueCount)} verifier-failed {issueCount === 1 ? 'suite' : 'suites'},{' '}
        {formatNumber(missingCount)} {missingCount === 1 ? 'assignment' : 'assignments'} with missing or
        insufficient observations
      </summary>
      <div style={{ marginTop: '10px', fontSize: 'var(--text-lg)', color: 'var(--fg-muted)' }}>
        {diagnostics.failedSuites.length > 0 ? (
          <section aria-label="Verification failures" style={{ marginBottom: '12px' }}>
            <h3 style={{ fontSize: 'var(--text-lg)', margin: '0 0 6px', color: 'var(--fg)' }}>
              Suites that failed canonical verification
            </h3>
            <ul style={{ margin: 0, paddingLeft: '18px' }}>
              {diagnostics.failedSuites.map((suite) => (
                <li key={suite.suite_id} style={{ marginBottom: '6px' }}>
                  <strong style={{ color: 'var(--fg)' }}>{suite.display_name}</strong>
                  {suite.verifier_failure_summary ? (
                    <span> — {suite.verifier_failure_summary}</span>
                  ) : null}
                  {suite.run_ids.map((runId) => (
                    <span key={runId}>
                      {' '}
                      <Link to={`/evaluations/${datasetId}/${runId}`} style={{ fontFamily: 'var(--mono)', fontSize: 'var(--text-md)' }}>
                        {runId}
                      </Link>
                    </span>
                  ))}
                </li>
              ))}
            </ul>
          </section>
        ) : null}
        {diagnostics.flaggedByRun.length > 0 ? (
          <section aria-label="Missing observations by run" style={{ marginBottom: '12px' }}>
            <h3 style={{ fontSize: 'var(--text-lg)', margin: '0 0 6px', color: 'var(--fg)' }}>
              Missing or insufficient observations by run
            </h3>
            <ul style={{ margin: 0, paddingLeft: '18px' }}>
              {diagnostics.flaggedByRun.map((entry) => (
                <li key={entry.run_id}>
                  <Link to={`/evaluations/${datasetId}/${entry.run_id}`} style={{ fontFamily: 'var(--mono)', fontSize: 'var(--text-md)' }}>
                    {entry.suite_id ?? entry.run_id}
                  </Link>
                  <span> — {formatNumber(entry.count)} affected assignments</span>
                </li>
              ))}
            </ul>
          </section>
        ) : null}
        <p style={{ margin: 0 }}>
          Affected assignments keep their terminal outcome in the record but contribute no eligible
          metric values; insufficient repeatability cells and missing latencies trace back to these
          observations. Nothing here is hidden from the denominators — the counts above are the
          reason specific cells render as Unavailable or insufficient.
        </p>
      </div>
    </details>
  );
}
