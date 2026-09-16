// Docs view — contract reference for engineers. Scannable tables, pipeline
// diagram, and live methodology snapshot data. No marketing copy.

import { useEffect, useMemo, useState } from 'react';
import { useStoreState } from '../state/store';
import { loadRuntimeConfig } from '../state/feed';
import { EmptyState, QualityBadge, SectionHeading, StatTile } from '../components/shared';
import { qualityStateLabel } from '../utils/feed-state';
import descriptorUrl from '../contract/descriptor.json?url';
import {
  DATASET_KINDS,
  FEED_RECORD_TYPES,
  LIVE_EVENT_KINDS,
  QUALITY_STATES,
  SCENARIO_CATEGORIES,
  SNAPSHOT_KINDS,
  VIEW_SCHEMA_VERSION,
  type CatalogSnapshot,
  type DatasetKind,
  type MethodologySnapshot,
  type QualityState,
} from '../contract/types';

const DATASET_KIND_META: Record<
  DatasetKind,
  { label: string; defaultQuality: QualityState; compare: string }
> = {
  exploratory_baseline: {
    label: 'Exploratory baseline',
    defaultQuality: 'exploratory_partial',
    compare: 'Model-role only · historical',
  },
  verified_public_snapshot: {
    label: 'Verified public snapshot',
    defaultQuality: 'verified_public',
    compare: 'Checksum-bound · publication-eligible',
  },
  live_run: {
    label: 'Live run',
    defaultQuality: 'live_in_progress',
    compare: 'Provisional · not ranked vs terminal',
  },
};

const SCENARIO_COUNTS: Record<(typeof SCENARIO_CATEGORIES)[number], number> = {
  instruction_adherence: 4,
  tool_selection: 4,
  tool_arguments: 3,
  technical_analysis: 4,
  routing_delegation: 3,
  verification: 2,
  security_policy: 2,
  recovery: 2,
  final_response: 1,
};

const ENGINEERING_RULES = [
  ['Missing metrics', 'Render Unavailable with reason — never zero'],
  ['Dataset mixing', 'Never average or rank across datasets'],
  ['Live values', 'Provisional until terminal + verification'],
  ['Denominator', 'Eligible tasks only — repetitions do not inflate rates'],
  ['Verification fail', 'Data stays visible under exploratory_partial'],
  ['Inventory models', 'inventory_only flag — not a quality state'],
] as const;

function formatEnum(value: string): string {
  return value.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
}

function scenarioLabel(category: (typeof SCENARIO_CATEGORIES)[number]): string {
  return formatEnum(category);
}

function PipelineDiagram() {
  const nodes = [
    { id: 'campaign', label: 'Campaign', sub: 'g8e eval campaign' },
    { id: 'publish', label: 'Publication', sub: 'public-safe projection' },
    { id: 'mirror', label: 'Mirror', sub: 'JSONL feed + SSE' },
    { id: 'explorer', label: 'Explorer', sub: 'read-only browser' },
  ];

  return (
    <div className="docs-pipeline" aria-label="Data flow from campaign to browser">
      <svg className="docs-pipeline-svg" viewBox="0 0 720 88" role="img" aria-hidden="true">
        <defs>
          <marker id="docs-arrow" markerWidth="8" markerHeight="8" refX="6" refY="4" orient="auto">
            <path d="M0,0 L8,4 L0,8 Z" fill="var(--accent)" />
          </marker>
        </defs>
        {[0, 1, 2].map((i) => (
          <line
            key={i}
            x1={150 + i * 180}
            y1="44"
            x2={210 + i * 180}
            y2="44"
            stroke="var(--accent)"
            strokeWidth="2"
            markerEnd="url(#docs-arrow)"
          />
        ))}
        {nodes.map((node, i) => (
          <g key={node.id} transform={`translate(${24 + i * 180}, 12)`}>
            <rect width="132" height="64" rx="8" fill="var(--bg-elev2)" stroke="var(--border)" />
            <text x="66" y="28" textAnchor="middle" fill="var(--fg)" fontSize="13" fontWeight="700">
              {node.label}
            </text>
            <text x="66" y="48" textAnchor="middle" fill="var(--fg-dim)" fontSize="10">
              {node.sub}
            </text>
          </g>
        ))}
      </svg>
      <ol className="docs-pipeline-steps">
        {nodes.map((node) => (
          <li key={node.id}>
            <strong>{node.label}</strong>
            <span>{node.sub}</span>
          </li>
        ))}
      </ol>
    </div>
  );
}

function ScenarioChart() {
  const max = Math.max(...Object.values(SCENARIO_COUNTS));
  const total = Object.values(SCENARIO_COUNTS).reduce((sum, n) => sum + n, 0);

  return (
    <div className="docs-scenario-chart" aria-label={`${total} scenarios across ${SCENARIO_CATEGORIES.length} categories`}>
      <ul className="docs-bar-list">
        {SCENARIO_CATEGORIES.map((category) => {
          const count = SCENARIO_COUNTS[category];
          return (
            <li key={category}>
              <span className="docs-bar-label">{scenarioLabel(category)}</span>
              <span className="docs-bar-track" aria-hidden="true">
                <span className="docs-bar-fill" style={{ width: `${(count / max) * 100}%` }} />
              </span>
              <span className="docs-bar-count">{count}</span>
            </li>
          );
        })}
      </ul>
      <p className="docs-chart-foot">
        <strong>{total}</strong> frozen scenarios · North Star catalog
      </p>
    </div>
  );
}

function DatasetTable({ catalogs }: { catalogs: CatalogSnapshot[] }) {
  const byKind = useMemo(() => {
    const map = new Map<DatasetKind, CatalogSnapshot>();
    for (const catalog of catalogs) {
      map.set(catalog.dataset_kind, catalog);
    }
    return map;
  }, [catalogs]);

  return (
    <div className="table-scroll">
      <table className="docs-table">
        <thead>
          <tr>
            <th scope="col">Kind</th>
            <th scope="col">Dataset ID</th>
            <th scope="col">Quality</th>
            <th scope="col">Coverage</th>
            <th scope="col">Policy</th>
          </tr>
        </thead>
        <tbody>
          {DATASET_KINDS.map((kind) => {
            const meta = DATASET_KIND_META[kind];
            const catalog = byKind.get(kind);
            return (
              <tr key={kind}>
                <th scope="row">{meta.label}</th>
                <td>
                  <code>{catalog?.dataset_id ?? '—'}</code>
                </td>
                <td>
                  <QualityBadge state={catalog?.quality_state ?? meta.defaultQuality} />
                </td>
                <td>
                  {catalog ? (
                    <>
                      {catalog.evaluated_count}/{catalog.model_count} models ·{' '}
                      {catalog.assignment_count} assignments
                    </>
                  ) : (
                    '—'
                  )}
                </td>
                <td>{meta.compare}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function MetricSpecTable({ metrics }: { metrics: MethodologySnapshot['metric_definitions'] }) {
  return (
    <div className="table-scroll">
      <table className="docs-table docs-metric-table">
        <thead>
          <tr>
            <th scope="col">Metric</th>
            <th scope="col">Unit</th>
            <th scope="col">Direction</th>
            <th scope="col">Denominator</th>
            <th scope="col">Missing</th>
            <th scope="col">Uncertainty</th>
          </tr>
        </thead>
        <tbody>
          {metrics.map((metric) => (
            <tr key={metric.key}>
              <th scope="row">
                {metric.name}
                <span className="docs-metric-key">{metric.key}</span>
              </th>
              <td>{metric.unit}</td>
              <td>{formatEnum(metric.direction)}</td>
              <td>{metric.denominator}</td>
              <td>{metric.missing_value_behavior}</td>
              <td>{metric.uncertainty_method}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function EnumChipTable({ title, values }: { title: string; values: readonly string[] }) {
  return (
    <div className="docs-enum-panel">
      <h3>{title}</h3>
      <ul className="docs-enum-list">
        {values.map((value) => (
          <li key={value}>
            <code>{value}</code>
          </li>
        ))}
      </ul>
    </div>
  );
}

function DownloadsStrip() {
  const [origin, setOrigin] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    loadRuntimeConfig()
      .then((config) => {
        if (!cancelled) setOrigin(config.mirror_origin);
      })
      .catch(() => {
        if (!cancelled) setOrigin(null);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const links = [
    { label: 'View schema', href: descriptorUrl },
    { label: 'Bootstrap', href: origin ? `${origin}/bootstrap` : undefined },
    { label: 'History', href: origin ? `${origin}/history` : undefined },
    { label: 'SSE stream', href: origin ? `${origin}/stream` : undefined },
  ];

  return (
    <ul className="docs-link-strip">
      {links.map((link) => (
        <li key={link.label}>
          {link.href ? (
            <a href={link.href} target="_blank" rel="noopener noreferrer">
              {link.label}
            </a>
          ) : (
            <span className="docs-link-disabled">{link.label}</span>
          )}
        </li>
      ))}
    </ul>
  );
}

export function MethodologyView() {
  const methodology = useStoreState((state) => state.methodology);
  const catalogs = useStoreState((state) => Array.from(state.catalogs.values()));
  const connection = useStoreState((state) => state.connection);

  if (!methodology) {
    return (
      <div className="docs-view">
        <SectionHeading kicker="DOCS" title="Evaluation contract" />
        <EmptyState hasRecords={false} hasFilters={false} connection={connection} />
      </div>
    );
  }

  return (
    <div className="docs-view">
      <SectionHeading
        kicker="DOCS"
        title="Evaluation contract"
        description={`Public read model for OpenDevOps.ai model campaigns. Schema ${VIEW_SCHEMA_VERSION} · mirror-only · no provider calls from the browser.`}
      />

      <div className="stat-grid docs-stat-grid">
        <StatTile label="Schema" value={VIEW_SCHEMA_VERSION} hint="Frozen view contract" />
        <StatTile label="Snapshots" value={SNAPSHOT_KINDS.length} hint="Durable record kinds" />
        <StatTile label="Live events" value={LIVE_EVENT_KINDS.length} hint="SSE lifecycle kinds" />
        <StatTile label="Feed types" value={FEED_RECORD_TYPES.length} hint="Mirror transport envelope" />
      </div>

      <section className="panel docs-panel">
        <div className="panel-head">
          <h2>Publication pipeline</h2>
          <p className="panel-note">Campaign evidence → public projection → anonymous mirror</p>
        </div>
        <PipelineDiagram />
      </section>

      <div className="docs-grid">
        <section className="panel docs-panel">
          <div className="panel-head">
            <h2>Metric spec</h2>
            <p className="panel-note">From active methodology snapshot</p>
          </div>
          <MetricSpecTable metrics={methodology.metric_definitions} />
        </section>

        <section className="panel docs-panel">
          <div className="panel-head">
            <h2>Datasets</h2>
            <p className="panel-note">Never mixed in compare or aggregates</p>
          </div>
          <DatasetTable catalogs={catalogs} />
        </section>
      </div>

      <div className="docs-grid docs-grid-wide">
        <section className="panel docs-panel">
          <div className="panel-head">
            <h2>Scenario catalog</h2>
            <p className="panel-note">Frozen North Star matrix by category</p>
          </div>
          <ScenarioChart />
        </section>

        <section className="panel docs-panel">
          <div className="panel-head">
            <h2>Roles</h2>
            <p className="panel-note">Candidates compete per role — not by parameter count</p>
          </div>
          <div className="table-scroll">
            <table className="docs-table docs-compact-table">
              <thead>
                <tr>
                  <th scope="col">Role</th>
                  <th scope="col">Wire</th>
                  <th scope="col">Scope</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <th scope="row">Primary</th>
                  <td><code>primary</code></td>
                  <td>Task owner · plans · delegates · synthesizes</td>
                </tr>
                <tr>
                  <th scope="row">Assistant</th>
                  <td><code>assistant</code></td>
                  <td>Bounded technical work for Primary</td>
                </tr>
                <tr>
                  <th scope="row">Light</th>
                  <td><code>lite</code></td>
                  <td>Constrained decisions or escalate</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
      </div>

      <section className="panel docs-panel">
        <div className="panel-head">
          <h2>Engineering rules</h2>
          <p className="panel-note">Fail-closed behaviors enforced in validators and UI</p>
        </div>
        <div className="table-scroll">
          <table className="docs-table docs-compact-table">
            <thead>
              <tr>
                <th scope="col">Rule</th>
                <th scope="col">Behavior</th>
              </tr>
            </thead>
            <tbody>
              {ENGINEERING_RULES.map(([rule, behavior]) => (
                <tr key={rule}>
                  <th scope="row">{rule}</th>
                  <td>{behavior}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <div className="docs-enum-grid">
        <section className="panel docs-panel">
          <div className="panel-head">
            <h2>Quality states</h2>
          </div>
          <ul className="docs-quality-list">
            {QUALITY_STATES.map((state) => (
              <li key={state}>
                <QualityBadge state={state} />
                <code>{state}</code>
                <span>{qualityStateLabel(state)}</span>
              </li>
            ))}
          </ul>
        </section>

        <section className="panel docs-panel">
          <EnumChipTable title="Snapshot kinds" values={SNAPSHOT_KINDS} />
        </section>

        <section className="panel docs-panel">
          <EnumChipTable title="Live event kinds" values={LIVE_EVENT_KINDS} />
        </section>
      </div>

      {methodology.suite_definitions.length > 0 ? (
        <section className="panel docs-panel">
          <div className="panel-head">
            <h2>Suites</h2>
          </div>
          <div className="table-scroll">
            <table className="docs-table docs-compact-table">
              <thead>
                <tr>
                  <th scope="col">Suite</th>
                  <th scope="col">ID</th>
                  <th scope="col">Tasks</th>
                  <th scope="col">Description</th>
                </tr>
              </thead>
              <tbody>
                {methodology.suite_definitions.map((suite) => (
                  <tr key={suite.suite_id}>
                    <th scope="row">{suite.display_name}</th>
                    <td><code>{suite.suite_id}</code></td>
                    <td>{suite.task_count}</td>
                    <td>{suite.description}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      ) : null}

      {methodology.limitations.length > 0 ? (
        <section className="panel docs-panel docs-limitations">
          <div className="panel-head">
            <h2>Active limitations</h2>
            <p className="panel-note">From methodology snapshot · {methodology.observed_at}</p>
          </div>
          <ul className="docs-limitation-list">
            {methodology.limitations.map((lim, i) => (
              <li key={i}>{lim}</li>
            ))}
          </ul>
        </section>
      ) : null}

      <section className="panel docs-panel">
        <div className="panel-head">
          <h2>Machine-readable contract</h2>
          <p className="panel-note">descriptor.json · mirror endpoints</p>
        </div>
        <DownloadsStrip />
      </section>
    </div>
  );
}
