// Docs view — what's shipped, where this is headed, and how to read the feed.
// Static copy for first-time visitors; live methodology data for metrics,
// datasets, suites, and active limitations.

import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { useStoreState } from '../state/store';
import { loadRuntimeConfig } from '../state/feed';
import { EmptyState, QualityBadge, StatTile } from '../components/shared';
import descriptorUrl from '../contract/descriptor.json?url';
import {
  G8E_ARCHITECTURE_DOCS,
  G8E_CAMPAIGN_OPERATORS,
  G8E_DIFFERENTIATORS,
  G8E_DIFFERENTIATORS_LEDE,
  G8E_MEASURED_TOGETHER,
  G8E_REPO_URL,
  G8E_STACK_COMPONENTS,
  GITHUB_SPONSORS_URL,
  PLATFORM_SITE_URL,
  SPONSORSHIP_LEDE,
  SPONSORSHIP_USES,
  WORKSTATION_SPECS,
} from '../content/platform';
import { MODEL_ROLES } from '../content/roles';
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

const SHIPPED_TODAY = [
  'A public Evaluation Explorer over a signed mirror feed — live campaign progress, historical runs, per-model results, and role-scoped comparison, all reconstructed from bootstrap, paginated history, and SSE.',
  'Model campaign benchmarks that run models through the production g8ee chat path: governed inference dispatch, tool and filesystem boundaries, and the Primary / Assistant / Lite role stack used in real workloads.',
  'A frozen 25-scenario agent benchmark catalog across nine behavior categories, graded on a real host boundary with rubric-based pass/fail, tool scorecards, escalation disposition, and timing telemetry when observed.',
  'Explicit quality-state labeling on every record — verified, exploratory, live-in-progress, or failed — so you can see what has passed integrity checks and what is still provisional.',
] as const;

const BUILDING_TOWARD = [
  'Checksum-bound verified public snapshots: publication-eligible results when a campaign completes and passes full verification.',
  'Independent provider-boundary observation for GPU and system-efficiency metrics, bound to inference attempts rather than inferred from latency.',
  'Heterogeneous system leaderboards that compare complete role stacks, separate from single-role model swaps.',
  'Broader model catalog coverage as campaigns scale — inventory-only entries today, measured candidates as runs complete.',
] as const;

const HOW_TO_INTERPRET = [
  ['Quality badges are the source of truth', 'Check the quality state on any row before drawing conclusions. verified_public means publication-eligible; exploratory_partial means measured but not yet verified; live_in_progress means still running.'],
  ['Metrics are scoped on purpose', 'Comparisons stay inside one dataset, one designated role, and one denominator. Model evaluations swap a single role candidate; system evaluations compare complete stacks.'],
  ['Missing telemetry is disclosed', 'When a metric was not observed, the UI shows Unavailable with a reason instead of a zero that would look like a measurement.'],
  ['Confidence intervals describe uncertainty', 'Bootstrap bounds express sampling variance over tasks. Overlapping intervals mean the data cannot separate the candidates — that is a feature, not a bug.'],
  ['Reproducibility lives in the report bundle', 'This mirror publishes aggregate results safe for anonymous reading. Signed operator evidence for full reproduction sits behind the evaluation pipeline, not in the browser.'],
] as const;

const SCENARIO_CATEGORY_BLURBS: Record<(typeof SCENARIO_CATEGORIES)[number], string> = {
  instruction_adherence: 'Follow constraints, formats, and stop conditions',
  tool_selection: 'Pick the right tool for the intent',
  tool_arguments: 'Populate schemas and semantic arguments correctly',
  technical_analysis: 'Interpret host and log evidence accurately',
  routing_delegation: 'Route work to the correct role or escalate',
  verification: 'Validate outputs before committing',
  security_policy: 'Respect authorization and data-handling policy',
  recovery: 'Recover from tool or execution failures',
  final_response: 'Synthesize a correct user-facing answer',
};

function formatEnum(value: string): string {
  return value.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
}

function scenarioLabel(category: (typeof SCENARIO_CATEGORIES)[number]): string {
  return formatEnum(category);
}

function ArchitectureDiagram() {
  const edgeNodes = [
    { id: 'publish', label: 'Publication', sub: 'public-safe projection' },
    { id: 'mirror', label: 'Mirror', sub: 'JSONL feed + SSE' },
    { id: 'cloudflare', label: 'Cloudflare', sub: 'tunnel to opendevops.ai' },
    { id: 'explorer', label: 'Explorer', sub: 'this browser' },
  ];

  return (
    <div className="docs-architecture" aria-label="g8e architecture from home workstation to browser">
      <svg className="docs-architecture-svg" viewBox="0 0 900 300" role="img" aria-hidden="true">
        <defs>
          <marker id="docs-arch-arrow" markerWidth="8" markerHeight="8" refX="6" refY="4" orient="auto">
            <path d="M0,0 L8,4 L0,8 Z" fill="var(--accent)" />
          </marker>
        </defs>

        <rect x="20" y="16" width="860" height="132" rx="12" fill="none" stroke="var(--border)" strokeWidth="2" strokeDasharray="6 4" />
        <text x="36" y="38" fill="var(--fg-dim)" fontSize="12" fontWeight="700" letterSpacing="0.4">
          HOME WORKSTATION · WINDOWS · DOCKER
        </text>

        <g transform="translate(40, 52)">
          <rect width="150" height="78" rx="8" fill="var(--bg-elev2)" stroke="var(--border)" />
          <text x="75" y="30" textAnchor="middle" fill="var(--fg)" fontSize="15" fontWeight="700">Ollama</text>
          <text x="75" y="50" textAnchor="middle" fill="var(--fg-dim)" fontSize="12">local model inference</text>
        </g>

        <g transform="translate(210, 52)">
          <rect width="500" height="78" rx="8" fill="var(--navy)" stroke="var(--accent)" strokeWidth="2" />
          <text x="250" y="28" textAnchor="middle" fill="var(--accent)" fontSize="16" fontWeight="800">g8e unified stack</text>
          <text x="250" y="48" textAnchor="middle" fill="var(--fg)" fontSize="12">
            Gateway · Operator · Ensemble · native eval
          </text>
          <text x="250" y="64" textAnchor="middle" fill="var(--fg-dim)" fontSize="11">
            governed execution, campaigns, and publication
          </text>
        </g>

        <g transform="translate(730, 52)">
          <rect width="130" height="78" rx="8" fill="var(--bg-elev2)" stroke="var(--border)" />
          <text x="65" y="30" textAnchor="middle" fill="var(--fg)" fontSize="15" fontWeight="700">Campaigns</text>
          <text x="65" y="50" textAnchor="middle" fill="var(--fg-dim)" fontSize="12">live + historical</text>
        </g>

        <line x1="450" y1="148" x2="450" y2="178" stroke="var(--accent)" strokeWidth="2" markerEnd="url(#docs-arch-arrow)" />

        {[0, 1, 2].map((i) => (
          <line
            key={i}
            x1={170 + i * 210}
            y1="224"
            x2={230 + i * 210}
            y2="224"
            stroke="var(--accent)"
            strokeWidth="2"
            markerEnd="url(#docs-arch-arrow)"
          />
        ))}

        {edgeNodes.map((node, i) => (
          <g key={node.id} transform={`translate(${36 + i * 210}, 188)`}>
            <rect width="156" height="72" rx="8" fill="var(--bg-elev2)" stroke="var(--border)" />
            <text x="78" y="30" textAnchor="middle" fill="var(--fg)" fontSize="14" fontWeight="700">
              {node.label}
            </text>
            <text x="78" y="50" textAnchor="middle" fill="var(--fg-dim)" fontSize="11">
              {node.sub}
            </text>
          </g>
        ))}
      </svg>

      <ol className="docs-architecture-steps">
        <li>
          <strong>Home workstation</strong>
          <span>Windows host running the full g8e Docker stack and Ollama for on-prem model inference.</span>
        </li>
        <li>
          <strong>g8e unified stack</strong>
          <span>Gateway, Operator, Ensemble, and native eval campaigns execute and grade every benchmark locally.</span>
        </li>
        <li>
          <strong>Publication + mirror</strong>
          <span>Campaign evidence is projected to public-safe fields and streamed as JSONL plus SSE from the gateway.</span>
        </li>
        <li>
          <strong>Cloudflare tunnel</strong>
          <span>The gateway public listener is exposed at opendevops.ai — you are reading live results from this PC.</span>
        </li>
        <li>
          <strong>Evaluation Explorer</strong>
          <span>This read-only browser validates every record against a frozen schema before rendering.</span>
        </li>
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
              <span className="docs-bar-label">
                <span className="docs-bar-name">{scenarioLabel(category)}</span>
                <span className="docs-bar-blurb">{SCENARIO_CATEGORY_BLURBS[category]}</span>
              </span>
              <span className="docs-bar-track" aria-hidden="true">
                <span className="docs-bar-fill" style={{ width: `${(count / max) * 100}%` }} />
              </span>
              <span className="docs-bar-count">{count}</span>
            </li>
          );
        })}
      </ul>
      <p className="docs-chart-foot">
        <strong>{total}</strong> frozen scenarios · versioned agent benchmark catalog
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

const DOC_NAV = [
  { id: 'overview', label: 'Overview' },
  { id: 'support', label: 'Support' },
  { id: 'benchmark', label: 'Benchmark' },
  { id: 'differentiators', label: 'Why g8e' },
  { id: 'architecture', label: 'Architecture' },
  { id: 'feed', label: 'Current feed' },
  { id: 'guarantees', label: 'Guarantees' },
  { id: 'reference', label: 'Reference' },
] as const;

function scrollToSection(id: string, event: React.MouseEvent<HTMLAnchorElement>) {
  event.preventDefault();
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth' });
}

function DocsSection({ id, title, children }: { id: string; title: string; children: ReactNode }) {
  return (
    <section id={id} className="docs-section" aria-labelledby={`${id}-title`}>
      <h2 id={`${id}-title`} className="docs-section-title">{title}</h2>
      <div className="docs-section-body">{children}</div>
    </section>
  );
}

function DocsCard({
  title,
  lede,
  children,
  variant,
}: {
  title?: string;
  lede?: string;
  children: ReactNode;
  variant?: 'roadmap' | 'sponsor';
}) {
  return (
    <article className={`panel docs-card${variant ? ` docs-card-${variant}` : ''}`}>
      {title ? <h3 className="docs-card-title">{title}</h3> : null}
      {lede ? <p className="docs-card-lede">{lede}</p> : null}
      {children}
    </article>
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
  const scenarioTotal = Object.values(SCENARIO_COUNTS).reduce((sum, n) => sum + n, 0);

  return (
    <div className="docs-layout">
      <aside className="docs-sidebar" aria-label="Documentation navigation">
        <p className="docs-sidebar-label">On this page</p>
        <nav>
          <ul className="docs-sidebar-nav">
            {DOC_NAV.map((item) => (
              <li key={item.id}>
                <a href={`#${item.id}`} onClick={(event) => scrollToSection(item.id, event)}>
                  {item.label}
                </a>
              </li>
            ))}
          </ul>
        </nav>
      </aside>

      <div className="docs-main">
        <header className="panel docs-hero">
          <h1 className="docs-hero-title">Evaluation Explorer</h1>
          <p className="docs-hero-lede">
            Public benchmarks for models running through OpenDevOps.ai — a live deployment of the{' '}
            <a href={G8E_REPO_URL} target="_blank" rel="noopener noreferrer">g8e</a> AI governance suite.
            Evaluations execute on a home workstation over Docker and Ollama, publish through the gateway
            mirror, and stream here over Cloudflare. Quality labels tell you exactly which stage each result
            is in: live, exploratory, or verified.
          </p>
        </header>

        <DocsSection id="overview" title="Overview">
          <div className="docs-split">
            <DocsCard title="Shipped today" lede="What you can use in this browser right now.">
              <ul className="docs-feature-list">
                {SHIPPED_TODAY.map((item) => (
                  <li key={item}>{item}</li>
                ))}
              </ul>
            </DocsCard>
            <DocsCard
              title="Building toward"
              lede="The evaluation program this Explorer surfaces as campaigns mature."
              variant="roadmap"
            >
              <ul className="docs-feature-list">
                {BUILDING_TOWARD.map((item) => (
                  <li key={item}>{item}</li>
                ))}
              </ul>
            </DocsCard>
          </div>
          <DocsCard title="How to interpret what you see" lede="Straight answers for careful readers.">
            <ul className="docs-trust-list">
              {HOW_TO_INTERPRET.map(([headline, detail]) => (
                <li key={headline}>
                  <strong>{headline}</strong>
                  <span>{detail}</span>
                </li>
              ))}
            </ul>
          </DocsCard>
        </DocsSection>

        <DocsSection id="support" title="Support OpenDevOps.ai">
          <DocsCard variant="sponsor" lede={SPONSORSHIP_LEDE}>
            <ul className="docs-feature-list">
              {SPONSORSHIP_USES.map((item) => (
                <li key={item}>{item}</li>
              ))}
            </ul>
            <div className="docs-sponsor-actions">
              <a
                className="docs-sponsor-button"
                href={GITHUB_SPONSORS_URL}
                target="_blank"
                rel="noopener noreferrer"
              >
                Sponsor on GitHub
              </a>
              <a href={G8E_REPO_URL} target="_blank" rel="noopener noreferrer">
                g8e repository
              </a>
            </div>
          </DocsCard>
        </DocsSection>

        <DocsSection id="benchmark" title="What we benchmark">
          <div className="docs-split">
            <DocsCard
              title="Scenario catalog"
              lede={`${scenarioTotal} frozen scenarios across ${SCENARIO_CATEGORIES.length} categories on a real host boundary through the production inference and tool stack.`}
            >
              <ScenarioChart />
            </DocsCard>
            <DocsCard title="Model roles" lede="Candidates compete per role — not by parameter count.">
              <ul className="docs-role-cards">
                {MODEL_ROLES.map((role) => (
                  <li key={role.wire}>
                    <strong>{role.name}</strong>
                    <code>{role.wire}</code>
                    <span>{role.scope}</span>
                  </li>
                ))}
              </ul>
              <p className="docs-card-foot">
                Each scenario grades pass/fail against rubric criteria. Escalation disposition, tool
                scorecards, security events, and timing telemetry publish when observed — otherwise they stay
                explicitly unavailable.
              </p>
            </DocsCard>
          </div>
        </DocsSection>

        <DocsSection id="differentiators" title="Why this evaluation is different">
          <DocsCard lede={G8E_DIFFERENTIATORS_LEDE}>
            <ul className="docs-trust-list">
              {G8E_DIFFERENTIATORS.map((item) => (
                <li key={item.headline}>
                  <strong>{item.headline}</strong>
                  <span>{item.detail}</span>
                </li>
              ))}
            </ul>
            <p className="docs-card-foot">
              Full platform architecture:{' '}
              <a href={G8E_ARCHITECTURE_DOCS.overview} target="_blank" rel="noopener noreferrer">overview</a>
              {' · '}
              <a href={G8E_ARCHITECTURE_DOCS.governance} target="_blank" rel="noopener noreferrer">governance</a>
              {' · '}
              <a href={G8E_ARCHITECTURE_DOCS.operator} target="_blank" rel="noopener noreferrer">operator</a>
              {' · '}
              <a href={G8E_ARCHITECTURE_DOCS.evals} target="_blank" rel="noopener noreferrer">evaluations</a>
            </p>
          </DocsCard>

          <div className="docs-split">
            <DocsCard
              title="Measured together — not in isolation"
              lede="Every model candidate runs through the same total package. Scores reflect the full governed path, not a stripped provider API call."
            >
              <ul className="docs-feature-list">
                {G8E_MEASURED_TOGETHER.map((item) => (
                  <li key={item}>{item}</li>
                ))}
              </ul>
            </DocsCard>
            <DocsCard
              title="Campaign operator topology"
              lede="One Gateway coordinates three enrolled remote Operator sessions — each an outbound-only g8e binary with its own evidence chain."
            >
              <ul className="docs-operator-list">
                {G8E_CAMPAIGN_OPERATORS.map((operator) => (
                  <li key={operator.role}>
                    <div className="docs-operator-head">
                      <strong>{operator.role}</strong>
                      <code>{operator.wire}</code>
                    </div>
                    <span>{operator.detail}</span>
                  </li>
                ))}
              </ul>
              <p className="docs-card-foot">
                Evidence for each session is written to the working directory where that binary was started — signed
                receipts, audit vault entries, and campaign artifacts under <code>.g8e/data/</code>. No root
                privilege required on the evaluation host.
              </p>
            </DocsCard>
          </div>
        </DocsSection>

        <DocsSection id="architecture" title="Architecture — powered by g8e">
          <DocsCard
            title="OpenDevOps.ai is a live g8e deployment"
            lede="This site is one working example of the g8e AI governance suite — not a separate benchmark product. Evaluations run on a home workstation, publish through the built-in gateway mirror, and reach your browser over Cloudflare. The results you see are live."
          >
            <p className="docs-architecture-intro">
              <a href={G8E_REPO_URL} target="_blank" rel="noopener noreferrer">
                g8e
              </a>{' '}
              governs AI data and execution at the edge: policy admission, host-bound operator execution,
              multi-agent reasoning, native evaluations, and a public mirror for anonymous read-only spectators.
              OpenDevOps.ai uses that stack end-to-end — Docker on a Windows workstation, Ollama for local models,
              and the gateway&apos;s Cloudflare tunnel to serve this explorer at{' '}
              <a href={PLATFORM_SITE_URL} target="_blank" rel="noopener noreferrer">opendevops.ai</a>.
              The suite&apos;s scope is much broader than this one surface; see the{' '}
              <a href={G8E_REPO_URL} target="_blank" rel="noopener noreferrer">g8e repository</a> for the full
              platform.
            </p>
            <ArchitectureDiagram />
            <p className="docs-card-foot">
              The publication layer projects only public-safe fields — aggregate metrics, quality states, and
              assignment outcomes. Prompts, outputs, and credentials stay in the signed report bundle behind
              the evaluation pipeline.
            </p>
          </DocsCard>

          <div className="docs-split">
            <DocsCard title="g8e components in this deployment" lede="Four platform surfaces, one governance boundary on the workstation.">
              <ul className="docs-stack-list">
                {G8E_STACK_COMPONENTS.map((component) => (
                  <li key={component.id}>
                    <strong>{component.label}</strong>
                    <span>{component.detail}</span>
                  </li>
                ))}
              </ul>
            </DocsCard>
            <DocsCard title="Evaluation host" lede="Hardware running the Docker stack and Ollama today.">
              <ul className="docs-spec-list">
                {WORKSTATION_SPECS.map((spec) => (
                  <li key={spec.label}>
                    <span className="docs-spec-label">{spec.label}</span>
                    <span className="docs-spec-value">{spec.value}</span>
                  </li>
                ))}
              </ul>
              <p className="docs-card-foot">
                Model campaigns execute against this host boundary through the production g8ee chat path — the same
                governed inference, tool, and filesystem stack used for real workloads, not a synthetic API shim.
              </p>
            </DocsCard>
          </div>
        </DocsSection>

        <DocsSection id="feed" title="Current feed">
          {methodology ? (
            <>
              <DocsCard title="Datasets" lede="Exploratory, verified, and live runs are labeled and compared separately.">
                <DatasetTable catalogs={catalogs} />
              </DocsCard>

              <DocsCard title="Metric definitions" lede={`Active methodology snapshot · ${methodology.observed_at}`}>
                <MetricSpecTable metrics={methodology.metric_definitions} />
              </DocsCard>

              {methodology.suite_definitions.length > 0 ? (
                <DocsCard title="Active suites" lede="Suites scheduled in the current feed.">
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
                </DocsCard>
              ) : null}

              {methodology.limitations.length > 0 ? (
                <DocsCard title="Active limitations" lede="Known gaps in the current feed.">
                  <ul className="docs-limitation-list">
                    {methodology.limitations.map((lim, i) => (
                      <li key={i}>{lim}</li>
                    ))}
                  </ul>
                </DocsCard>
              ) : null}
            </>
          ) : (
            <DocsCard>
              <EmptyState hasRecords={false} hasFilters={false} connection={connection} />
            </DocsCard>
          )}
        </DocsSection>

        <DocsSection id="guarantees" title="Rendering guarantees">
          <DocsCard lede="Rules the UI follows so displayed numbers stay faithful to the feed.">
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
          </DocsCard>
        </DocsSection>

        <DocsSection id="reference" title="Contract reference">
          <DocsCard lede={`For integrators · schema ${VIEW_SCHEMA_VERSION} · mirror-only · no provider calls from the browser.`}>
            <div className="stat-grid docs-stat-grid">
              <StatTile label="Schema" value={VIEW_SCHEMA_VERSION} hint="Frozen view contract" />
              <StatTile label="Snapshots" value={SNAPSHOT_KINDS.length} hint="Durable record kinds" />
              <StatTile label="Live events" value={LIVE_EVENT_KINDS.length} hint="SSE lifecycle kinds" />
              <StatTile label="Feed types" value={FEED_RECORD_TYPES.length} hint="Mirror transport envelope" />
            </div>
            <div className="docs-enum-grid">
              <section className="docs-enum-panel docs-enum-panel-bordered">
                <h3>Quality states</h3>
                <ul className="docs-quality-list">
                  {QUALITY_STATES.map((state) => (
                    <li key={state}>
                      <QualityBadge state={state} />
                      <code>{state}</code>
                    </li>
                  ))}
                </ul>
              </section>

              <section className="docs-enum-panel docs-enum-panel-bordered">
                <EnumChipTable title="Snapshot kinds" values={SNAPSHOT_KINDS} />
              </section>

              <section className="docs-enum-panel docs-enum-panel-bordered">
                <EnumChipTable title="Live event kinds" values={LIVE_EVENT_KINDS} />
              </section>
            </div>
            <DownloadsStrip />
          </DocsCard>
        </DocsSection>
      </div>
    </div>
  );
}
