// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Docs view — what's shipped, where this is headed, and how to read the feed.
// Static copy for first-time visitors; live methodology data for metrics,
// datasets, suites, and active limitations.

import { useEffect, useMemo, useState, type MouseEvent, type ReactNode } from 'react';
import { useStoreState } from '../state/store';
import { loadRuntimeConfig } from '../state/feed';
import { EmptyState, QualityBadge, StatTile } from '../components/shared';
import descriptorUrl from '../contract/descriptor.json?url';
import {
  G8E_ARCHITECTURE_DOCS,
  G8E_CAMPAIGN_OPERATORS,
  G8E_REPO_URL,
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
    label: 'Legacy public snapshot',
    defaultQuality: 'unavailable',
    compare: 'Historical · not current-standard verified',
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
  ['Verification incomplete', 'Data stays visible as Not fully verified'],
  ['Inventory models', 'inventory_only flag — not a quality state'],
] as const;

const EVALUATION_PROGRAMS = [
  {
    name: 'Execution boundary',
    id: 'core-execution-boundary@1.0.0',
    purpose: 'Proves that one governed mutation succeeds and its doctrine-prohibited equivalent fails closed through the same Gateway and remote Operator path.',
    path: 'Gateway ingress → L1–L3 admission → remote Operator L4/L5 → networkless target reader',
    excludes: 'No g8ee, model provider, campaign scheduler, or synthetic simulator',
  },
  {
    name: 'Model campaign',
    id: 'north-star-25@1.0.0',
    purpose: 'Scores real models through production chat, governed inference, host tools, provider telemetry, and storage-side model provenance.',
    path: 'g8ee chat → Gateway → bound Inference / Data Operators + independent witnesses',
    excludes: 'No direct Ollama calls from the campaign CLI or g8ee',
  },
] as const;

const EXECUTION_STAGES = [
  {
    label: 'Freeze',
    system: 'Campaign controller',
    detail: 'Binds the scenario catalog, model registry, role, repetitions, and exact Operator sessions before work starts.',
    output: 'Campaign + assignment identity',
  },
  {
    label: 'Admit',
    system: 'Gateway · PDP',
    detail: 'Authenticates ingress, constructs or verifies the GovernanceEnvelope, and applies posture-required L1–L3 policy.',
    output: 'State-bound envelope',
  },
  {
    label: 'Execute',
    system: 'Operator · PEP',
    detail: 'The exact bound Operator independently re-runs L1–L4, then performs L5 against its own runtime boundary.',
    output: 'Signed local receipt',
  },
  {
    label: 'Witness',
    system: 'Observer + Provenance',
    detail: 'Separate sessions bind GPU/RAM samples and model-weight hashes to the provider attempt without executor self-report.',
    output: 'Observation windows',
  },
  {
    label: 'Verify',
    system: 'Offline verifier',
    detail: 'Recomputes digests, signatures, bindings, populations, verdicts, and metrics without executing another mutation.',
    output: 'report.json + verification.json',
  },
  {
    label: 'Project',
    system: 'Public mirror',
    detail: 'Emits an allowlisted, public-safe projection. Private prompts, outputs, identities, paths, and receipt bodies stay owner-local.',
    output: 'Bootstrap + history + SSE',
  },
] as const;

const EVIDENCE_PROPERTIES = [
  ['Content addressed', 'Declared artifacts resolve by digest. Substitution, omission, contradiction, and undeclared evidence fail verification.'],
  ['Execution owned', 'The sovereign Operator stores authoritative local execution evidence; the Gateway receipt is a verified, best-effort mirror.'],
  ['Attempt bound', 'Telemetry and provenance windows bind to provider_attempt_id, not a model label or an inferred wall-clock interval.'],
  ['Population scoped', 'A passing report applies only to its exact run, campaign, catalog, registry, completed population, and verified population.'],
] as const;

const PUBLIC_BOUNDARY = [
  ['Published', 'Scenario identity, closed grade metadata, grouped activity, bounded resource metrics, verification disposition, and approved SHA-256 bindings.'],
  ['Owner-local', 'Prompts, outputs, reasoning, private grade detail, principals, sessions, endpoints, filesystem paths, envelopes, and receipt bodies.'],
  ['Not implied', 'A public binding does not make its artifact public. Application-reported tool outcomes do not prove protocol authorization. Mirror availability is not verification evidence.'],
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

const campaignRemoteOperators = G8E_CAMPAIGN_OPERATORS.slice(1);

function operatorDiagramDetail(role: string): string {
  switch (role) {
    case 'Data Operator':
      return 'host tools · files · processes';
    case 'Inference Operator':
      return 'governed path to Ollama';
    case 'Observer Operator':
      return 'GPU + RAM witness';
    case 'Provenance Operator':
      return 'model-weight attestation';
    default:
      return 'outbound-only g8eo session';
  }
}

function ArchitectureDiagram() {
  const edgeNodes = [
    { id: 'publish', label: 'Publication', sub: 'public-safe projection' },
    { id: 'mirror', label: 'Mirror', sub: 'JSONL feed + SSE' },
    { id: 'cloudflare', label: 'Cloudflare', sub: 'tunnel to opendevops.ai' },
    { id: 'explorer', label: 'Explorer', sub: 'this browser' },
  ];

  return (
    <div className="docs-architecture" aria-label="g8e architecture from campaign operators to browser">
      <svg className="docs-architecture-svg" viewBox="0 0 900 540" role="img" aria-hidden="true">
        <defs>
          <marker id="docs-arch-arrow" markerWidth="8" markerHeight="8" refX="6" refY="4" orient="auto">
            <path d="M0,0 L8,4 L0,8 Z" fill="var(--accent)" />
          </marker>
        </defs>

        <g>
          {edgeNodes.map((node, i) => (
            <g key={node.id} transform={`translate(${36 + i * 210}, 22)`}>
              <rect width="156" height="72" rx="8" fill="var(--bg-elev2)" stroke="var(--border)" />
              <text x="78" y="30" textAnchor="middle" fill="var(--fg)" fontSize="14" fontWeight="700">
                {node.label}
              </text>
              <text x="78" y="50" textAnchor="middle" fill="var(--fg-dim)" fontSize="11">
                {node.sub}
              </text>
            </g>
          ))}
          {[0, 1, 2].map((i) => (
            <line
              key={i}
              x1={192 + i * 210}
              y1="58"
              x2={236 + i * 210}
              y2="58"
              stroke="var(--accent)"
              strokeWidth="2"
              markerEnd="url(#docs-arch-arrow)"
            />
          ))}
        </g>

        <line x1="450" y1="170" x2="450" y2="100" stroke="var(--accent)" strokeWidth="2" markerEnd="url(#docs-arch-arrow)" />

        <rect x="20" y="130" width="860" height="200" rx="12" fill="none" stroke="var(--border)" strokeWidth="2" strokeDasharray="6 4" />
        <text x="36" y="152" fill="var(--fg-dim)" fontSize="12" fontWeight="700" letterSpacing="0.4">
          HOME WORKSTATION · WINDOWS · DOCKER
        </text>

        <g transform="translate(40, 170)">
          <rect width="150" height="78" rx="8" fill="var(--bg-elev2)" stroke="var(--border)" />
          <text x="75" y="30" textAnchor="middle" fill="var(--fg)" fontSize="15" fontWeight="700">Ollama</text>
          <text x="75" y="50" textAnchor="middle" fill="var(--fg-dim)" fontSize="12">local model inference</text>
        </g>

        <g transform="translate(210, 170)">
          <rect width="500" height="78" rx="8" fill="var(--navy)" stroke="var(--accent)" strokeWidth="2" />
          <text x="250" y="28" textAnchor="middle" fill="var(--accent)" fontSize="16" fontWeight="800">g8e unified stack</text>
          <text x="250" y="48" textAnchor="middle" fill="var(--fg)" fontSize="12">
            Gateway · Operator · Ensemble · native eval
          </text>
          <text x="250" y="64" textAnchor="middle" fill="var(--fg-dim)" fontSize="11">
            governed execution, campaigns, and publication
          </text>
        </g>

        <g transform="translate(730, 170)">
          <rect width="130" height="78" rx="8" fill="var(--bg-elev2)" stroke="var(--border)" />
          <text x="65" y="30" textAnchor="middle" fill="var(--fg)" fontSize="15" fontWeight="700">Campaigns</text>
          <text x="65" y="50" textAnchor="middle" fill="var(--fg-dim)" fontSize="12">live + historical</text>
        </g>

        <g transform="translate(40, 258)">
          <rect width="820" height="64" rx="8" fill="var(--bg-elev2)" stroke="var(--border)" />
          <text x="12" y="15" fill="var(--fg-dim)" fontSize="10" fontWeight="700" letterSpacing="0.4">EVALUATION HOST</text>
          <line x1="0" y1="22" x2="820" y2="22" stroke="var(--border)" />
          <line x1="410" y1="22" x2="410" y2="64" stroke="var(--border)" />
          <line x1="0" y1="43" x2="820" y2="43" stroke="var(--border)" />
          {WORKSTATION_SPECS.map((spec, i) => {
            const x = (i % 2) * 410;
            const y = i < 2 ? 37 : 58;
            return (
              <g key={spec.label}>
                <text x={x + 12} y={y} fill="var(--fg-dim)" fontSize="9" fontWeight="700">{spec.label}</text>
                <text x={x + 68} y={y} fill="var(--fg)" fontSize="10">{spec.value}</text>
              </g>
            );
          })}
        </g>

        <rect x="20" y="360" width="860" height="156" rx="12" fill="none" stroke="var(--border)" strokeWidth="2" strokeDasharray="6 4" />
        <text x="36" y="506" fill="var(--fg-dim)" fontSize="12" fontWeight="700" letterSpacing="0.4">
          CAMPAIGN OPERATOR SESSIONS · OUTBOUND mTLS
        </text>
        <line x1="126" y1="350" x2="774" y2="350" stroke="var(--accent)" strokeWidth="2" opacity="0.65" />
        <line x1="450" y1="350" x2="450" y2="250" stroke="var(--accent)" strokeWidth="2" markerEnd="url(#docs-arch-arrow)" />

        {campaignRemoteOperators.map((operator, i) => {
          const x = 24 + i * 216;
          const center = 126 + i * 216;
          return (
            <g key={operator.role}>
              <line x1={center} y1="396" x2={center} y2="350" stroke="var(--accent)" strokeWidth="2" markerEnd="url(#docs-arch-arrow)" />
              <g transform={`translate(${x}, 396)`}>
                <rect width="204" height="96" rx="8" fill="var(--bg-elev2)" stroke="var(--border)" />
                <text x="102" y="28" textAnchor="middle" fill="var(--fg)" fontSize="14" fontWeight="700">
                  {operator.role}
                </text>
                <text x="102" y="48" textAnchor="middle" fill="var(--fg-dim)" fontSize="11">
                  {operator.wire} · remote session
                </text>
                <text x="102" y="70" textAnchor="middle" fill="var(--fg-dim)" fontSize="10">
                  {operatorDiagramDetail(operator.role)}
                </text>
              </g>
            </g>
          );
        })}
      </svg>

      <ol className="docs-architecture-steps">
        <li>
          <strong>Publication → mirror → Explorer</strong>
          <span>Public-safe campaign evidence travels through the mirror and Cloudflare to this read-only browser.</span>
        </li>
        <li>
          <strong>Home workstation</strong>
          <span>Windows host running the full g8e Docker stack and Ollama for on-prem model inference.</span>
        </li>
        <li>
          <strong>g8e unified stack</strong>
          <span>Gateway, Operator, Ensemble, and native eval campaigns execute and grade every benchmark locally.</span>
        </li>
        <li>
          <strong>Campaign operators</strong>
          <span>Data, Inference, Observer, and Provenance sessions connect outbound over mTLS with separate responsibilities.</span>
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
  { id: 'programs', label: 'Programs' },
  { id: 'execution', label: 'Execution path' },
  { id: 'benchmark', label: 'Benchmark' },
  { id: 'evidence', label: 'Evidence model' },
  { id: 'feed', label: 'Live contract' },
  { id: 'guarantees', label: 'UI invariants' },
  { id: 'reference', label: 'Reference' },
  { id: 'support', label: 'Support' },
] as const;

function scrollToSection(id: string, event: MouseEvent<HTMLAnchorElement>) {
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
        <div id="architecture">
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
        </div>

        <DocsSection id="overview" title="Evaluation architecture, not a model leaderboard">
          <div className="docs-overview-callout">
            <p className="docs-eyebrow">Engineering model</p>
            <h3>Measure the system that actually executes the work.</h3>
            <p>
              g8e evaluates models inside the production control plane: authenticated ingress, policy admission,
              session-bound execution, independent witness collection, content-addressed evidence, and offline
              verification. A score is useful only with a precise account of what ran, where it ran, and which
              claims the evidence supports.
            </p>
          </div>
          <div className="docs-principle-grid">
            <article>
              <span className="docs-principle-index">01</span>
              <strong>Production path</strong>
              <p>Scored inference traverses g8ee, the Gateway, and the exact Inference Operator. There is no provider API shortcut.</p>
            </article>
            <article>
              <span className="docs-principle-index">02</span>
              <strong>Sovereign execution</strong>
              <p>The Operator that can see or mutate a runtime owns L4/L5 execution and the authoritative local receipt.</p>
            </article>
            <article>
              <span className="docs-principle-index">03</span>
              <strong>Independent witnesses</strong>
              <p>Provider hardware and model weights are observed by separate least-privilege sessions, not the inference executor.</p>
            </article>
            <article>
              <span className="docs-principle-index">04</span>
              <strong>Scoped claims</strong>
              <p>Verification applies to one bound evidence population. Missing telemetry remains missing; it is never inferred or zero-filled.</p>
            </article>
          </div>
          <p className="docs-reading-note">
            <strong>Read the UI in this order:</strong> dataset → role → quality state → denominator → metric. Never compare values across dataset boundaries or treat a live run as terminal evidence.
          </p>
        </DocsSection>

        <DocsSection id="programs" title="Two programs, one evidence model">
          <div className="docs-program-grid">
            {EVALUATION_PROGRAMS.map((program, index) => (
              <article className="panel docs-program-card" key={program.id}>
                <div className="docs-program-head">
                  <span>0{index + 1}</span>
                  <code>{program.id}</code>
                </div>
                <h3>{program.name}</h3>
                <p>{program.purpose}</p>
                <dl>
                  <div>
                    <dt>Execution path</dt>
                    <dd>{program.path}</dd>
                  </div>
                  <div>
                    <dt>Scope boundary</dt>
                    <dd>{program.excludes}</dd>
                  </div>
                </dl>
              </article>
            ))}
          </div>
          <p className="docs-section-note">
            Both programs persist canonical evidence beneath <code>.g8e/data/eval/runs/</code>. Their verifiers recompute the evidence graph without performing another mutation.
          </p>
        </DocsSection>

        <DocsSection id="execution" title="From assignment to public projection">
          <DocsCard lede="The control path and evidence path advance together, but remain separate trust domains.">
            <ol className="docs-pipeline">
              {EXECUTION_STAGES.map((stage, index) => (
                <li key={stage.label}>
                  <span className="docs-pipeline-number">{String(index + 1).padStart(2, '0')}</span>
                  <div className="docs-pipeline-copy">
                    <div className="docs-pipeline-title">
                      <strong>{stage.label}</strong>
                      <span>{stage.system}</span>
                    </div>
                    <p>{stage.detail}</p>
                    <code>{stage.output}</code>
                  </div>
                </li>
              ))}
            </ol>
          </DocsCard>
          <DocsCard title="Campaign trust boundaries" lede="Each remote session uses the same g8e binary with a distinct capability set and outbound-only mTLS connection.">
            <div className="docs-operator-grid">
              {G8E_CAMPAIGN_OPERATORS.map((operator) => (
                <article key={operator.role} className={operator.wire === 'PDP' ? 'docs-operator-pdp' : undefined}>
                  <div>
                    <strong>{operator.role}</strong>
                    <code>{operator.wire}</code>
                  </div>
                  <p>{operator.detail}</p>
                </article>
              ))}
            </div>
            <p className="docs-card-foot">
              The Gateway coordinates work but does not collapse execution boundaries. Operators independently verify upstream proofs before a local side effect or witness publication.
            </p>
          </DocsCard>
        </DocsSection>

        <DocsSection id="benchmark" title="Benchmark design">
          <div className="docs-split">
            <DocsCard
              title="Frozen scenario catalog"
              lede={`${scenarioTotal} scenarios across ${SCENARIO_CATEGORIES.length} behavior categories, executed through the production inference and host-tool path.`}
            >
              <ScenarioChart />
            </DocsCard>
            <DocsCard title="Role-scoped candidates" lede="A candidate replaces one role at a time so the comparison keeps a stable system context.">
              <ul className="docs-role-cards">
                {MODEL_ROLES.map((role) => (
                  <li key={role.wire}>
                    <div className="docs-role-card-head">
                      <strong>{role.name}</strong>
                      <code>{role.wire}</code>
                    </div>
                    <span>{role.scope}</span>
                  </li>
                ))}
              </ul>
              <p className="docs-card-foot">
                Eligible tasks define the denominator; repetitions measure consistency without inflating pass rates. Rubric grades, tool scorecards, escalation disposition, security events, and timing publish only when observed.
              </p>
            </DocsCard>
          </div>
        </DocsSection>

        <DocsSection id="evidence" title="Evidence and verification model">
          <ul className="docs-evidence-grid">
            {EVIDENCE_PROPERTIES.map(([headline, detail]) => (
              <li key={headline}>
                <strong>{headline}</strong>
                <span>{detail}</span>
              </li>
            ))}
          </ul>
          <DocsCard title="The public boundary" lede="The Explorer is a signed spectator projection, not an audit database or execution authority.">
            <div className="docs-boundary-grid">
              {PUBLIC_BOUNDARY.map(([headline, detail], index) => (
                <article key={headline} className={`docs-boundary-${index}`}>
                  <span>{headline}</span>
                  <p>{detail}</p>
                </article>
              ))}
            </div>
            <p className="docs-card-foot">
              Full architecture:{' '}
              <a href={G8E_ARCHITECTURE_DOCS.overview} target="_blank" rel="noopener noreferrer">overview</a>
              {' · '}
              <a href={G8E_ARCHITECTURE_DOCS.governance} target="_blank" rel="noopener noreferrer">governance</a>
              {' · '}
              <a href={G8E_ARCHITECTURE_DOCS.operator} target="_blank" rel="noopener noreferrer">operator</a>
              {' · '}
              <a href={G8E_ARCHITECTURE_DOCS.evals} target="_blank" rel="noopener noreferrer">evaluations</a>
            </p>
          </DocsCard>
        </DocsSection>

        <DocsSection id="feed" title="Live methodology contract">
          <p className="docs-section-intro">
            This section renders the methodology snapshot currently accepted by the client. It is data, not hand-authored page copy, so the definitions and limitations track the connected mirror.
          </p>
          {methodology ? (
            <>
              <DocsCard title="Dataset partitions" lede="Quality and comparison policy are explicit for every partition. The Explorer never averages across them.">
                <DatasetTable catalogs={catalogs} />
              </DocsCard>

              <DocsCard title="Metric definitions" lede={`Accepted methodology snapshot · ${methodology.observed_at}`}>
                <MetricSpecTable metrics={methodology.metric_definitions} />
              </DocsCard>

              {methodology.suite_definitions.length > 0 ? (
                <DocsCard title="Active suite definitions" lede="Suites declared by the current methodology snapshot.">
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
                <DocsCard title="Declared limitations" lede="Known gaps travel with the feed and remain visible beside the measurements.">
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

        <DocsSection id="guarantees" title="Explorer invariants">
          <div className="docs-split docs-invariant-layout">
            <DocsCard title="Rendering contract" lede="Client rules that prevent visual convenience from changing the meaning of evidence.">
              <div className="table-scroll">
                <table className="docs-table docs-compact-table">
                  <thead>
                    <tr>
                      <th scope="col">Invariant</th>
                      <th scope="col">UI behavior</th>
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
            <DocsCard title="Quality is evidence scope" lede="A badge describes verification state, not universal model quality.">
              <ul className="docs-quality-list docs-quality-list-compact">
                {QUALITY_STATES.map((state) => (
                  <li key={state}>
                    <QualityBadge state={state} />
                    <code>{state}</code>
                  </li>
                ))}
              </ul>
              <p className="docs-card-foot">
                <code>exploratory_verified</code> applies only to the exact verified run-derived dataset and eligible variant/role aggregate. It does not imply complete optional telemetry or <code>verified_public</code> status.
              </p>
            </DocsCard>
          </div>
        </DocsSection>

        <DocsSection id="reference" title="Integrator reference">
          <DocsCard lede={`Frozen public view schema ${VIEW_SCHEMA_VERSION} · anonymous mirror reads only · no Gateway fallback or provider calls from the browser.`}>
            <div className="stat-grid docs-stat-grid">
              <StatTile label="Schema" value={VIEW_SCHEMA_VERSION} hint="Accepted view contract" />
              <StatTile label="Snapshots" value={SNAPSHOT_KINDS.length} hint="Durable record kinds" />
              <StatTile label="Live events" value={LIVE_EVENT_KINDS.length} hint="SSE lifecycle kinds" />
              <StatTile label="Feed types" value={FEED_RECORD_TYPES.length} hint="Mirror envelopes" />
            </div>
            <div className="docs-enum-grid docs-enum-grid-reference">
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

        <DocsSection id="support" title="Run it, inspect it, support it">
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
                Read the source
              </a>
              <a href={G8E_ARCHITECTURE_DOCS.evals} target="_blank" rel="noopener noreferrer">
                Evaluation architecture
              </a>
            </div>
          </DocsCard>
        </DocsSection>
      </div>
    </div>
  );
}
