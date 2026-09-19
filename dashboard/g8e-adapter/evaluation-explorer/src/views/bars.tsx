// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Accessible hand-authored bar visualizations. Every bar carries its value
// as visible text and an aria-label; the adjacent table is the full
// screen-reader alternative. Unlike-unit metrics are never combined in one
// visual — each bar row shows a single proportion metric.

/** One horizontal bar for a 0..1 proportion with a visible text label. */
export function ProportionBar({
  value,
  label,
  color = 'var(--accent)',
}: {
  value: number;
  label: string;
  color?: string;
}) {
  const pct = Math.max(0, Math.min(100, value * 100));
  return (
    <div
      className="proportion-bar"
      role="img"
      aria-label={`${label}: ${pct.toFixed(1)} percent`}
      style={{ display: 'flex', alignItems: 'center', gap: '8px', minWidth: 0 }}
    >
      <div
        aria-hidden="true"
        style={{
          flex: '1 1 120px',
          height: '10px',
          background: 'var(--bg-elev2)',
          borderRadius: '4px',
          overflow: 'hidden',
          minWidth: '60px',
        }}
      >
        <div style={{ width: `${pct}%`, height: '100%', background: color }} />
      </div>
      <span style={{ fontFamily: 'var(--mono)', fontSize: '12px', whiteSpace: 'nowrap' }}>{label}</span>
    </div>
  );
}

export interface StackedSegment {
  key: string;
  label: string;
  count: number;
  color: string;
}

/** Stacked composition bar for repeatability classes, with a visible legend
 *  carrying every count so the visual is never the only source. */
export function StackedBar({ segments }: { segments: StackedSegment[] }) {
  const total = segments.reduce((acc, s) => acc + s.count, 0);
  const ariaLabel = segments.map((s) => `${s.label} ${s.count}`).join(', ');
  return (
    <div className="stacked-bar">
      <div
        role="img"
        aria-label={total === 0 ? 'No observations' : `Repeatability composition: ${ariaLabel}`}
        aria-hidden="false"
        style={{
          display: 'flex',
          height: '14px',
          borderRadius: '4px',
          overflow: 'hidden',
          background: 'var(--bg-elev2)',
        }}
      >
        {total === 0 ? null : (
          segments.map((s) =>
            s.count > 0 ? (
              <div
                key={s.key}
                aria-hidden="true"
                style={{ width: `${(s.count / total) * 100}%`, background: s.color }}
                title={`${s.label}: ${s.count}`}
              />
            ) : null,
          )
        )}
      </div>
      <ul
        className="stacked-legend"
        style={{ listStyle: 'none', padding: 0, margin: '8px 0 0', display: 'flex', gap: '14px', flexWrap: 'wrap' }}
      >
        {segments.map((s) => (
          <li key={s.key} style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '12px' }}>
            <span
              aria-hidden="true"
              style={{ width: '10px', height: '10px', borderRadius: '2px', background: s.color, display: 'inline-block' }}
            />
            <span style={{ color: 'var(--fg-muted)' }}>{s.label}</span>
            <strong>{s.count}</strong>
          </li>
        ))}
      </ul>
    </div>
  );
}
