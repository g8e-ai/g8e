// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Public guide to docs/architecture/evals.md. Keep the architecture diagram at the top.

import { type MouseEvent, type ReactNode } from 'react';
import { G8E_ARCHITECTURE_DOCS, G8E_CAMPAIGN_OPERATORS, WORKSTATION_SPECS } from '../content/platform';
import { SCENARIO_CATALOG_ID, SCENARIO_TASKS } from '../content/scenario-task';

const DOC_NAV = [
  { id: 'purpose', label: 'Purpose' },
  { id: 'programs', label: 'Programs' },
  { id: 'commands', label: 'CLI' },
  { id: 'boundary', label: 'Native boundary' },
  { id: 'suites', label: 'Suites' },
  { id: 'campaigns', label: 'Campaigns' },
  { id: 'scenarios', label: 'Scenarios & grading' },
  { id: 'canaries', label: 'Canaries' },
  { id: 'witnesses', label: 'Witnesses' },
  { id: 'evidence', label: 'Evidence & verification' },
  { id: 'invariants', label: 'Invariants' },
] as const;

const CLI_GROUPS = [
  ['boundary', 'Run, list, show, and verify native execution-boundary suites'],
  ['models', 'Manage the model catalog and registry; pull and freeze provider inventory'],
  ['suites', 'List, show, export, create, update, and delete scenario suites'],
  ['campaigns', 'Freeze, inspect, assert release identity, and archive campaigns'],
  ['runs', 'Start, resume, inspect, verify, publish, export, repair, compare, and archive runs'],
  ['rollout', 'Manage and execute the model qualification queue'],
  ['formations', 'Manage heterogeneous three-role model stacks'],
  ['gates', 'Run chat, inference, and probe acceptance gates'],
  ['backup', 'Snapshot evaluation evidence outside .g8e/'],
  ['restore', 'Verify and restore a snapshot into .g8e/'],
] as const;

const TRAJECTORIES = [
  ['ANSWER', 'Answer without a required tool call; a successful forbidden call fails.'],
  ['FIRST_CHOICE', 'Make the expected tool call with valid arguments; a later valid call is RECOVERED.'],
  ['GUIDED', 'Use the expected tool after platform guidance when needed; repeated errors or abandonment fail.'],
  ['GOVERNED', 'Avoid the forbidden mutation and refuse or explain; circumventing a denial fails.'],
] as const;

const CANARIES = [
  ['tools-declared', 'The full eligible production tool set reaches the scored model call and the trace records the evaluation tool gate.'],
  ['seed-delivered', 'The investigation seed is applied and echoed unchanged.'],
  ['workspace-reachable', 'A canary file can be read through governed Data Operator dispatch.'],
  ['guidance-delivered', 'Registry guidance reaches g8ee byte for byte.'],
  ['registry-mcp', 'Agent tool registry entries verify and Gateway /mcp tools/list answers.'],
] as const;

const campaignRemoteOperators = G8E_CAMPAIGN_OPERATORS.slice(1);

function operatorDiagramDetail(role: string): string {
  switch (role) {
    case 'Data Operator': return 'host tools · files · processes';
    case 'Inference Operator': return 'governed path to Ollama';
    case 'Observer Operator': return 'GPU + RAM witness';
    case 'Provenance Operator': return 'model-weight attestation';
    default: return 'outbound-only g8eo session';
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
              <text x="78" y="50" textAnchor="middle" fill="var(--fg-dim)" fontSize="12">
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
          <text x="250" y="64" textAnchor="middle" fill="var(--fg-dim)" fontSize="12">
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
          <text x="12" y="15" fill="var(--fg-dim)" fontSize="11" fontWeight="700" letterSpacing="0.4">EVALUATION HOST</text>
          <line x1="0" y1="22" x2="820" y2="22" stroke="var(--border)" />
          <line x1="410" y1="22" x2="410" y2="64" stroke="var(--border)" />
          <line x1="0" y1="43" x2="820" y2="43" stroke="var(--border)" />
          {WORKSTATION_SPECS.map((spec, i) => {
            const x = (i % 2) * 410;
            const y = i < 2 ? 37 : 58;
            return (
              <g key={spec.label}>
                <text x={x + 12} y={y} fill="var(--fg-dim)" fontSize="10" fontWeight="700">{spec.label}</text>
                <text x={x + 68} y={y} fill="var(--fg)" fontSize="11">{spec.value}</text>
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
                <rect width="204" height="96" rx="8" fill="var(--navy)" stroke="var(--border)" />
                <text x="102" y="28" textAnchor="middle" fill="var(--fg)" fontSize="14" fontWeight="700">
                  {operator.role}
                </text>
                <text x="102" y="48" textAnchor="middle" fill="var(--fg-dim)" fontSize="12">
                  {operator.wire} · remote session
                </text>
                <text x="102" y="70" textAnchor="middle" fill="var(--fg-dim)" fontSize="11">
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

function DocsCard({ title, children }: { title?: string; children: ReactNode }) {
  return (
    <article className="panel docs-card">
      {title ? <h3 className="docs-card-title">{title}</h3> : null}
      {children}
    </article>
  );
}

function Command({ children }: { children: string }) {
  return <code className="docs-command">{children}</code>;
}

export function MethodologyView() {
  return (
    <div className="docs-layout">
      <aside className="docs-sidebar" aria-label="Documentation navigation">
        <p className="docs-sidebar-label">On this page</p>
        <nav>
          <ul className="docs-sidebar-nav">
            {DOC_NAV.map((item) => (
              <li key={item.id}>
                <a href={`#${item.id}`} onClick={(event) => scrollToSection(item.id, event)}>{item.label}</a>
              </li>
            ))}
          </ul>
        </nav>
      </aside>

      <div className="docs-main">
        <h1 className="sr-only">Evaluation programs</h1>
        <div id="architecture" className="panel docs-card docs-diagram-card">
          <ArchitectureDiagram />
        </div>

        <DocsSection id="purpose" title="Evaluation programs">
          <div className="docs-overview-callout">
            <p className="docs-eyebrow">Architecture guide</p>
            <h3>Governed execution, real inference, verifiable evidence.</h3>
            <p>
              g8e has two evaluation programs: a native execution-boundary suite and model campaigns.
              They exercise the Gateway and Operator trust boundary, then persist canonical,
              content-addressed evidence that can be verified without running scored work again.
            </p>
          </div>
          <p className="docs-section-intro">
            This page follows the maintained <a href={G8E_ARCHITECTURE_DOCS.evals} target="_blank" rel="noopener noreferrer">Evaluation Programs architecture document</a>.
            It explains how evaluations execute and what their evidence proves. The diagram above shows the
            deployment that publishes a public-safe view to this Explorer.
          </p>
        </DocsSection>

        <DocsSection id="programs" title="Two programs, separate execution paths">
          <div className="docs-program-grid">
            <article className="panel docs-program-card">
              <div className="docs-program-head"><span>01</span><code>core-execution-boundary@1.0.0</code></div>
              <h3>Native execution boundary</h3>
              <p>Proves that one allowed governed mutation succeeds and its doctrine-prohibited equivalent fails closed through the same authenticated Gateway and remote Data Operator path.</p>
              <dl>
                <div><dt>Uses</dt><dd>Gateway policy admission, Operator execution, signed receipts, and an independent networkless target reader.</dd></div>
                <div><dt>Excludes</dt><dd>g8ee, model providers, model judges, campaigns, scheduling, and synthetic simulators.</dd></div>
              </dl>
            </article>
            <article className="panel docs-program-card">
              <div className="docs-program-head"><span>02</span><code>{SCENARIO_CATALOG_ID} or a custom suite</code></div>
              <h3>Model campaigns</h3>
              <p>Score real models through g8ee <code>POST /api/v1/chat</code>, governed inference, real tool dispatch, and a frozen scenario catalog.</p>
              <dl>
                <div><dt>Uses</dt><dd>Data and Inference Operators, plus separate Observer and Provenance sessions when their witness evidence is required.</dd></div>
                <div><dt>Excludes</dt><dd>Direct Ollama calls from g8ee or the campaign CLI.</dd></div>
              </dl>
            </article>
          </div>
          <p className="docs-section-note">Run evidence lives under <code>.g8e/data/eval/runs/&lt;run-id&gt;/</code>. Campaign definitions and frozen artifacts live separately under <code>.g8e/data/eval/campaigns/&lt;campaign-id&gt;/</code>.</p>
        </DocsSection>

        <DocsSection id="commands" title="g8e eval command surface">
          <DocsCard>
            <p className="docs-card-lede">Ten top-level commands cover the two programs. Run <Command>./g8e eval --help</Command> for the exact flags available in your build.</p>
            <div className="table-scroll">
              <table className="docs-table docs-compact-table">
                <thead><tr><th scope="col">Command</th><th scope="col">Purpose</th></tr></thead>
                <tbody>
                  {CLI_GROUPS.map(([group, purpose]) => (
                    <tr key={group}><th scope="row"><code>g8e eval {group}</code></th><td>{purpose}</td></tr>
                  ))}
                </tbody>
              </table>
            </div>
          </DocsCard>
        </DocsSection>

        <DocsSection id="boundary" title="Native execution-boundary suite">
          <DocsCard title="One allowed action, one prohibited equivalent">
            <p>The suite requires doctrine posture and selects the stack&apos;s <code>data-operator</code> by hostname. The allowed attempt writes a run-specific marker through authenticated Gateway command ingress. The equivalent prohibited attempt takes the same route and must be rejected by L1 without a side effect.</p>
            <p>It checks independent effect counts, target identity, terminal receipt status and durability, protocol-chain validity, rejection, absence of alternate completed execution, and Gateway L1 attribution.</p>
            <div className="docs-command-row">
              <Command>./g8e eval boundary run</Command>
              <Command>./g8e eval boundary verify &lt;run-id&gt;</Command>
              <Command>./g8e eval boundary show &lt;run-id&gt;</Command>
            </div>
          </DocsCard>
          <div className="docs-split">
            <DocsCard title="Authority boundary">
              <p>The Gateway is the Policy Decision Point: ingress authentication, envelope construction, L1–L3 decisions, and coordination. The selected remote Operator is the Policy Execution Point: L4–L5 execution and authoritative local evidence. A Gateway receipt query is a verified mirror of Operator-authored evidence.</p>
            </DocsCard>
            <DocsCard title="Independent target observation">
              <p>An ephemeral <code>g8e-native-target-reader</code> process reads only the controlled fixture volume. It has no network, workload identity, credentials, or writable target mount. It is specific to this suite; it is not the campaign Observer Operator.</p>
            </DocsCard>
          </div>
        </DocsSection>

        <DocsSection id="suites" title="Versioned evaluation suites">
          <p className="docs-section-intro">A suite is a named, versioned scenario set. <code>default-suite</code> is the full built-in catalog; <code>smoke-suite</code> is its five-scenario screening subset. Both are read-only. Custom suites are JSON files managed through <code>g8e eval suites</code>.</p>
          <div className="docs-split">
            <DocsCard title="Author and validate">
              <p>A custom file declares <code>schema_version: 1.0.0</code>, an ID, version, and scenarios. Each scenario carries public metadata, a private prompt fixture, and private gold criteria. Creation and update validate tool names, trajectory shape, argument validators, prompt hints, and workspace fixtures. Changed content requires a new version.</p>
              <div className="docs-command-row">
                <Command>./g8e eval suites export default-suite &gt; my-suite.json</Command>
                <Command>./g8e eval suites create my-suite.json</Command>
              </div>
            </DocsCard>
            <DocsCard title="Freeze into a campaign">
              <p>A campaign copies exactly one suite&apos;s catalog and fixtures when created. Later edits or deletion of the suite cannot change that campaign, its runs, or verification. The Explorer&apos;s Tasks catalog is generated from <code>default-suite</code>; custom suite tasks are not added there.</p>
              <div className="docs-command-row"><Command>./g8e eval campaigns create my-campaign qwen3:4b --suite my-suite</Command></div>
            </DocsCard>
          </div>
        </DocsSection>

        <DocsSection id="campaigns" title="Model campaign execution">
          <DocsCard title="Bind the real provider and the exact Operators">
            <p>Every scored request follows g8ee <code>POST /api/v1/chat</code> through the Gateway to the bound Inference Operator. Model-originated tools reach the bound <code>data-operator</code>. The Inference Operator&apos;s enrolled runtime configuration determines Ollama access. Model campaigns freeze <code>served_model_tag</code> and <code>model_digest</code> pairs; live provider runs should run <code>g8e eval models freeze</code> before scoring so placeholder digests do not stand in for provenance.</p>
            <p>The default <code>model_role</code> lane freezes exactly one candidate model and schedules only the roles eligible for each scenario. The <code>system</code> lane scores heterogeneous formations with primary, assistant, and lite bindings. A formation uses the <code>g8ee</code> runner by default; the <code>direct</code> runner has a different execution and grading contract.</p>
          </DocsCard>
          <div className="docs-split">
            <DocsCard title="Production agent loop">
              <p>Homogeneous assignments run authentic Sage (<code>primary</code>) and Dash (<code>assistant</code> and <code>lite</code>) personas in g8ee&apos;s real ReAct loop. The request declares the full production tool set. Evaluation-only behavior is keyed to <code>evaluation_context</code> and recorded in the trace, including the tool-gate bypass and suppression of user-wide memory reads.</p>
            </DocsCard>
            <DocsCard title="Qualification rollout">
              <p>The current <code>default-suite</code> has {SCENARIO_TASKS.length} scenarios and 41 eligible role assignments per model. <code>--gate-smoke</code> screens five scenarios across eight assignments; <code>--promote-on-pass</code> runs the full matrix for passing candidates. Rollout defaults to strict witness verification.</p>
              <div className="docs-command-row"><Command>./g8e eval rollout run --gate-smoke --promote-on-pass</Command></div>
            </DocsCard>
          </div>
          <p className="docs-section-note">A catalog change creates a new versioned campaign for a rollout entry. <code>--skip-verified</code> skips an entry only when its verified run used the current built-in catalog. Different catalog versions are not comparable.</p>
        </DocsSection>

        <DocsSection id="scenarios" title="Scenario fixtures, trajectories, and grading">
          <DocsCard title="One scored turn in a prepared investigation">
            <p>Before the chat request, g8ee writes a frozen case seed through its investigation and memory paths. Conversation turns are visible inline; history events require <code>query_investigation_context</code>. The executor writes fixture files and decoys to an attempt-scoped Data Operator workspace through governed dispatch. A seed or workspace failure stops the request as an environment error.</p>
            <p>The model sees a realistic case title, not an evaluation label. Attempt identity stays in <code>evaluation_context</code>. The trace echoes seed and workspace data so import can reject a mismatch.</p>
          </DocsCard>
          <DocsCard title="Trajectory policies">
            <p className="docs-card-lede">Every request offers the full production tool set. The scenario policy determines how its ordered tool calls are graded.</p>
            <div className="table-scroll">
              <table className="docs-table docs-compact-table">
                <thead><tr><th scope="col">Policy</th><th scope="col">Expected behavior</th></tr></thead>
                <tbody>{TRAJECTORIES.map(([policy, detail]) => <tr key={policy}><th scope="row"><code>{policy}</code></th><td>{detail}</td></tr>)}</tbody>
              </table>
            </div>
            <p className="docs-card-foot">Pass-eligible outcomes are <code>DIRECT</code>, <code>RECOVERED</code>, and <code>YIELDED_TO_DENIAL</code>. The grader reads the digest-bound trace in a fixed order; an unreadable trace is a harness failure.</p>
          </DocsCard>
          <DocsCard title="What a score means">
            <p>Deterministic checks cover answer content, tool arguments, trajectory, policy decisions, and required evidence. Semantic judging is used where declared, with a deterministic content floor. Role criteria pass only when both trajectory and scenario content pass. The trace can also yield player-specific grades for triage, reasoning, Tribunal, Marshal, Auditor, and Codex.</p>
            <p>Each deterministic grade declares whether it is an observation, a derived result, or a structural precondition. Only passing or failing observation grades enter the task score, so a single model miss is counted once. Triage has its own reported grade and <code>triage_ok</code>, outside <code>task_score</code>. Resource aggregates count only scored-chain model calls; post-turn memory work and semantic grader calls do not inflate them.</p>
            <p>Public criteria and tool score dimensions are derived from the frozen scenario shape. A dimension that was not exercised is <code>scenario_not_applicable</code>; an applicable dimension without captured grading is <code>source_not_captured</code>.</p>
          </DocsCard>
        </DocsSection>

        <DocsSection id="canaries" title="Environment canaries and re-baselining">
          <p className="docs-section-intro"><code>g8e eval gates chat</code> and rollout smoke screening run canaries before case or model allocation. A failed canary aborts with <code>ENVIRONMENT ERROR</code>; it is never a model verdict and never judges the model&apos;s reply.</p>
          <DocsCard>
            <div className="table-scroll">
              <table className="docs-table docs-compact-table">
                <thead><tr><th scope="col">Canary</th><th scope="col">Proves</th></tr></thead>
                <tbody>{CANARIES.map(([name, purpose]) => <tr key={name}><th scope="row"><code>{name}</code></th><td>{purpose}</td></tr>)}</tbody>
              </table>
            </div>
          </DocsCard>
          <p className="docs-section-note">After a built-in catalog change, re-baseline candidates one model at a time. A passing run from an older catalog keeps its evidence checks, but its grades are not compared with the current catalog&apos;s grades.</p>
        </DocsSection>

        <DocsSection id="witnesses" title="Independent witness operators">
          <div className="docs-operator-grid">
            {G8E_CAMPAIGN_OPERATORS.map((operator) => (
              <article key={operator.role} className={operator.wire === 'PDP' ? 'docs-operator-pdp' : undefined}>
                <div><strong>{operator.role}</strong><code>{operator.wire}</code></div>
                <p>{operator.detail}</p>
              </article>
            ))}
          </div>
          <DocsCard>
            <p>The Observer enrolls on the provider host where Ollama and the GPU run; it samples GPU VRAM, utilization, temperature, power, clocks, and system RAM between BEGIN and FINALIZE. The Provenance Operator enrolls where model weights live; it hashes the manifest and referenced blobs and checks the frozen model digest. Both are separate governed sessions from inference, without generic command authority or <code>--inference-enabled</code>.</p>
            <p>The Gateway fans out witness commands alongside inference. Witness coverage is independent of whether inference completes. Enroll witnesses before a run: terminal assignments cannot acquire observation or attestation windows retroactively. <code>g8e eval runs start</code> makes witness requirements opt-in; rollout uses strict witness verification by default.</p>
          </DocsCard>
        </DocsSection>

        <DocsSection id="evidence" title="Evidence, backup, and verification">
          <div className="docs-split">
            <DocsCard title="Persisted evidence">
              <p>Native runs store <code>report.json</code>, <code>verification.json</code>, and digest-named artifacts in <code>.g8e/data/eval/runs/&lt;run-id&gt;/</code>. Campaign definitions and frozen artifacts live under <code>.g8e/data/eval/campaigns/&lt;campaign-id&gt;/</code>; campaign lifecycle, assignments, traces, and aggregates are run-scoped. Gateway volumes hold provider observation and provenance windows.</p>
            </DocsCard>
            <DocsCard title="Offline and run-scoped checks">
              <p><code>g8e eval boundary verify</code> and <code>g8e eval runs verify</code> recompute digests, signatures, bindings, verdicts, and metrics without new scored actions. A passing campaign report applies only when its run, campaign, catalog, registry, completed population, and verified population match persisted evidence.</p>
            </DocsCard>
          </div>
          <DocsCard title="Backup and restore">
            <p><code>g8e eval backup</code> snapshots the host&apos;s <code>.g8e/data/eval/</code> and <code>.g8e/eval/</code> trees outside <code>.g8e/</code>, with a per-file SHA-256 manifest. Runs also attempt a backup after completion; backup failure is a warning, not a changed run result. <code>g8e eval restore</code> verifies the manifest before writing and requires <code>--overwrite</code> for differing files. A host snapshot does not restore Gateway volumes.</p>
          </DocsCard>
          <DocsCard title="What this Explorer can show">
            <p>The public projection omits principals, Operator and session identities, credentials, endpoints, filesystem paths, envelopes, receipts, and evidence bodies. It can expose approved scenario context, bounded outputs, grades, activity, resource metrics, verification metadata, and SHA-256 bindings. A public binding does not make its private artifact public; mirror availability is not verification evidence.</p>
            <p>Only an applicable passing report can publish <code>exploratory_verified</code> for the exact run-derived dataset and eligible model-role aggregate. Missing observations remain unavailable, separate from observed zero. Cross-run comparisons require matching observed provider capacities and the same evaluated suites; they do not pool runs.</p>
          </DocsCard>
        </DocsSection>

        <DocsSection id="invariants" title="Architecture invariants and source">
          <ul className="docs-evidence-grid">
            <li><strong>Program definitions · INV-EVAL-PROG</strong><span>Keep the native suite model-free; score campaigns through production chat; persist content-addressed runs; verify without execution.</span></li>
            <li><strong>Campaign structure · INV-EVAL-CAMP</strong><span>Freeze one suite and provider-attested model bindings; use eligible roles, real tools and seeded cases; grade from the trace.</span></li>
            <li><strong>Witness separation · INV-EVAL-WIT</strong><span>Enroll distinct Observer and Provenance sessions at the provider and storage boundaries; bind their windows to provider attempts.</span></li>
            <li><strong>Evidence storage · INV-EVAL-EVID</strong><span>Keep campaign and run evidence separate, preserve backup integrity, and allowlist public projections.</span></li>
            <li><strong>Verification posture · INV-EVAL-VERIF</strong><span>Recompute evidence and scope verified quality to the exact matching run population. Preserve unavailable observations as unavailable.</span></li>
          </ul>
          <p className="docs-section-note">For the normative rule IDs, CLI procedures, and anti-patterns, read <a href={G8E_ARCHITECTURE_DOCS.evals} target="_blank" rel="noopener noreferrer">docs/architecture/evals.md</a>.</p>
        </DocsSection>
      </div>
    </div>
  );
}
