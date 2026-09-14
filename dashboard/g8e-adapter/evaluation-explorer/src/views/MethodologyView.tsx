// Methodology view. Explains datasets, suite definitions, metric direction
// and denominator, task as independent unit, bootstrap interval, repetitions,
// agreement, repeatability, quality states, missingness, exploratory
// limitations, and why live values are provisional. User-focused, links to
// deeper g8e documentation rather than making the user parse release plans.

import { useStoreState } from '../state/store';
import { EmptyState, QualityBadge, SectionHeading } from '../components/shared';
import { qualityStateLabel } from '../utils/feed-state';
import { QUALITY_STATES } from '../contract/types';

const evaluationQuestions = [
  ['Capability', 'Can the model do the job?', 'Task accuracy, instruction following, reasoning, and tool selection.'],
  ['Protocol', 'Can the model behave inside the protocol?', 'Schemas, routing, handoffs, retries, escalation, and recovery.'],
  ['Governance', 'Can the platform govern it safely?', 'Policy enforcement, data exposure, privilege boundaries, and audit completeness.'],
  ['Local value', 'Is it worth running locally?', 'Cold start, latency, VRAM, throughput, tokens, power, and cost-equivalent efficiency.'],
] as const;

const roleDefinitions = [
  ['Primary', 'Owns the task, plans, delegates, synthesizes evidence, and decides what happens next.', 'Investigate why a service is failing and produce a support response with evidence.'],
  ['Assistant', 'Performs bounded technical work for the Primary.', 'Inspect these logs and identify the probable failure.'],
  ['Light', 'Makes extremely constrained, cheap, high-volume decisions or escalates.', 'Does this result satisfy the request? YES or NO.'],
] as const;

const scenarioPlan = [
  ['Instruction adherence', 4, 'Follows explicit constraints exactly.'],
  ['Tool selection', 4, 'Recognizes when a tool is needed and chooses the right one.'],
  ['Tool arguments', 3, 'Produces valid structured arguments with correct semantics.'],
  ['Technical analysis', 4, 'Analyzes logs, network output, errors, and configuration.'],
  ['Routing and delegation', 3, 'Exercises Primary, Assistant, and Light handoffs.'],
  ['Verification', 2, 'Determines whether another agent actually satisfied the task.'],
  ['Security and policy', 2, 'Refuses or blocks prohibited operations.'],
  ['Recovery', 2, 'Handles tool failure, malformed responses, and unavailable resources.'],
  ['Final response', 1, 'Communicates the supported result clearly.'],
] as const;

const escalationMetrics = [
  ['Correct autonomous completion', 'Solved the task correctly without escalation.'],
  ['Correct escalation', 'Recognized that a stronger role was necessary.'],
  ['False escalation', 'Used stronger compute when the assigned role could have solved the task.'],
  ['Missed escalation', 'Attempted work beyond the role capability and failed.'],
  ['Escalation efficiency', 'Minimized stronger-model use without reducing task accuracy.'],
] as const;

const toolMetrics = [
  'Tool recognition',
  'Tool selection',
  'Argument schema',
  'Argument semantics',
  'Permission compliance',
  'Result interpretation',
  'Follow-up decision',
  'Unnecessary tool calls',
  'Looping',
  'Recovery',
] as const;

const securityEvents = [
  'Sensitive data present',
  'Sensitive data required',
  'Sensitive data sent externally',
  'Unnecessary data sent externally',
  'Policy prevented disclosure',
  'Model attempted unauthorized access',
  'Tool attempted unauthorized operation',
  'Authorization correctly enforced',
  'Audit record complete',
  'Audit record tampered',
  'Secret redaction successful',
] as const;

const telemetryGroups = [
  ['Identity', 'run, campaign, scenario, model, revision, family, parameter count, quantization, and assigned role'],
  ['Generation', 'temperature, top-p, seed, context window, prompt template, input tokens, output tokens, and total tokens'],
  ['Timing', 'model load, time to first token, generation duration, total latency, whole-task duration, and tokens per second'],
  ['Resources', 'VRAM before and peak, system RAM peak, GPU utilization, temperature, power draw, and clock'],
  ['Protocol', 'tool calls, successful and invalid calls, retries, escalations, handoffs, and final status'],
  ['Scores', 'task, tool, policy, verification, repeatability, and correlated-failure observations'],
] as const;

export function MethodologyView() {
  const methodology = useStoreState((state) => state.methodology);
  const connection = useStoreState((state) => state.connection);

  if (!methodology) {
    return (
      <div className="methodology-view">
        <SectionHeading kicker="METHODOLOGY" title="How evaluations are measured" />
        <EmptyState hasRecords={false} hasFilters={false} connection={connection} />
      </div>
    );
  }

  return (
    <div className="methodology-view">
      <SectionHeading
        kicker="METHODOLOGY"
        title="How evaluations are measured"
        description="Plain-language definitions of the metrics, datasets, and quality states shown in this site."
      />

      <section className="methodology-program-status">
        <p><strong>Current data:</strong> the dataset selector exposes the measured exploratory baseline, checksum-bound public snapshot, and observed live runs. The design below defines the broader first publication campaign; unmeasured dimensions remain unavailable and are never inferred from pass rate.</p>
      </section>

      <section className="methodology-questions">
        <h2>Four evaluation questions</h2>
        <p>No composite score can explain capability, protocol behavior, governance, and local efficiency at the same time. The first published dataset reports these as separate dimensions.</p>
        <div className="benchmark-card-grid">
          {evaluationQuestions.map(([label, question, measures], index) => (
            <article key={label} className="benchmark-card">
              <span>{String(index + 1).padStart(2, '0')} · {label}</span>
              <h3>{question}</h3>
              <p>{measures}</p>
            </article>
          ))}
        </div>
      </section>

      <section className="methodology-roles">
        <h2>Roles are responsibilities</h2>
        <p>Models compete for every role they can technically perform. Parameter count does not preassign a role.</p>
        <div className="table-scroll">
          <table className="benchmark-table">
            <thead><tr><th scope="col">Role</th><th scope="col">Responsibility</th><th scope="col">Typical task</th></tr></thead>
            <tbody>
              {roleDefinitions.map(([role, responsibility, task]) => (
                <tr key={role}><th scope="row">{role}</th><td>{responsibility}</td><td>{task}</td></tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <section className="methodology-scenario-plan">
        <h2>Initial 25-scenario campaign</h2>
        <p>Breadth comes before hundreds of near-duplicate variants. Small Light decisions, bounded Assistant analyses, and agentic Primary investigations all contribute to the same campaign.</p>
        <div className="table-scroll">
          <table className="benchmark-table scenario-table">
            <thead><tr><th scope="col">Category</th><th scope="col">Scenarios</th><th scope="col">Purpose</th></tr></thead>
            <tbody>
              {scenarioPlan.map(([category, count, purpose]) => (
                <tr key={category}><th scope="row">{category}</th><td>{count}</td><td>{purpose}</td></tr>
              ))}
            </tbody>
            <tfoot><tr><th scope="row">Total</th><td>25</td><td>One broad, inspectable first campaign.</td></tr></tfoot>
          </table>
        </div>
      </section>

      <section className="methodology-leaderboards">
        <h2>Model and system leaderboards</h2>
        <div className="benchmark-card-grid leaderboard-grid">
          <article className="benchmark-card"><span>MODEL EVALUATION</span><h3>One candidate, one role</h3><p>Compares model competency when a candidate runs alone as Primary, Assistant, or Light.</p></article>
          <article className="benchmark-card"><span>SYSTEM EVALUATION</span><h3>One heterogeneous stack</h3><p>Compares end-to-end task completion, routing, governance, and compute use across a Primary → Assistant → Light pipeline.</p></article>
        </div>
        <p>System results also report Primary invocation share and correlated failure rate. A stack can improve task completion while reserving the largest model for genuine escalations.</p>
      </section>

      <section className="methodology-escalation">
        <h2>Escalation quality</h2>
        <p>A small model succeeds when it solves a bounded task or correctly requests a stronger role. Confidently attempting work beyond its capability is a missed escalation.</p>
        <dl className="scorecard-list">
          {escalationMetrics.map(([metric, meaning]) => <div key={metric}><dt>{metric}</dt><dd>{meaning}</dd></div>)}
        </dl>
      </section>

      <section className="methodology-tools">
        <h2>Tool-calling scorecard</h2>
        <p>A successful HTTP response is not a complete tool score. Each call is decomposed so a failure identifies recognition, selection, schema, semantics, authorization, interpretation, decision, waste, looping, or recovery.</p>
        <ul className="metric-chip-list">
          {toolMetrics.map((metric) => <li key={metric}>{metric}</li>)}
        </ul>
      </section>

      <section className="methodology-security-events">
        <h2>Event-based security and privacy</h2>
        <p>Security and privacy derive from observed events rather than an opaque rating. Public summaries expose counts and denominators while restricted values remain outside the public projection.</p>
        <ul className="metric-chip-list">
          {securityEvents.map((event) => <li key={event}>{event}</li>)}
        </ul>
      </section>

      <section className="methodology-telemetry">
        <h2>Complete inference telemetry</h2>
        <p>Each inference and protocol event binds enough identity, configuration, timing, resource, routing, and outcome data to reproduce comparisons and explain failures.</p>
        <dl className="telemetry-list">
          {telemetryGroups.map(([group, fields]) => <div key={group}><dt>{group}</dt><dd>{fields}.</dd></div>)}
        </dl>
        <p className="methodology-callout"><strong>Cold and warm costs stay separate.</strong> Model load time, time to first token, generation duration, and whole-task duration are recorded independently because model swapping can dominate a local pipeline.</p>
      </section>

      <section className="methodology-profile">
        <h2>Published as a profile</h2>
        <p>The first release reports Task Accuracy, Tool Reliability, Instruction Fidelity, Security, Privacy, Escalation Quality, Recovery, Repeatability, Token Efficiency, and Latency Efficiency separately. A composite OpenDevOps score waits until the dimensions and weighting have public evidence.</p>
      </section>

      <section className="methodology-metrics">
        <h2>Current metric definitions</h2>
        <dl>
          {methodology.metric_definitions.map((metric) => (
            <div key={metric.key} className="metric-def">
              <dt>{metric.name}</dt>
              <dd>
                <p className="metric-explanation">{metric.explanation}</p>
                <ul className="metric-meta">
                  <li><span>Unit</span><strong>{metric.unit}</strong></li>
                  <li><span>Direction</span><strong>{metric.direction.replace(/_/g, ' ')}</strong></li>
                  <li><span>Denominator</span><strong>{metric.denominator}</strong></li>
                  <li><span>Missing value</span><strong>{metric.missing_value_behavior}</strong></li>
                  <li><span>Aggregation</span><strong>{metric.aggregation}</strong></li>
                  <li><span>Uncertainty</span><strong>{metric.uncertainty_method}</strong></li>
                </ul>
              </dd>
            </div>
          ))}
        </dl>
      </section>

      <section className="methodology-suites">
        <h2>Suite definitions</h2>
        <ul className="suite-def-list">
          {methodology.suite_definitions.map((suite) => (
            <li key={suite.suite_id}>
              <strong>{suite.display_name}</strong>
              <span>{suite.task_count} tasks</span>
              <p>{suite.description}</p>
            </li>
          ))}
        </ul>
      </section>

      <section className="methodology-quality">
        <h2>Quality states</h2>
        <p>Every dataset, run, metric, and task carries one of these visible states. Quality state is data, not decorative copy.</p>
        <ul className="quality-state-list">
          {QUALITY_STATES.map((state) => (
            <li key={state}>
              <QualityBadge state={state} />
              <span>{qualityStateLabel(state)}</span>
            </li>
          ))}
        </ul>
      </section>

      <section className="methodology-concepts">
        <h2>Key concepts</h2>
        <dl>
          <div>
            <dt>Task as independent unit</dt>
            <dd>Each task is treated as an independent observation. The denominator is the count of eligible tasks, never inflated by repetitions.</dd>
          </div>
          <div>
            <dt>Bootstrap confidence interval</dt>
            <dd>Pass-rate intervals are bootstrap confidence bounds reflecting sampling uncertainty, not a superiority claim over other models.</dd>
          </div>
          <div>
            <dt>Repetitions</dt>
            <dd>Some tasks are repeated to measure consistency. Repeatability classes summarize whether outcomes are consistently correct, consistently wrong, inconsistent, or insufficient.</dd>
          </div>
          <div>
            <dt>Agreement</dt>
            <dd>Pairwise agreement is how often two repetitions of the same task agree. All-five agreement is how often all repetitions agree.</dd>
          </div>
          <div>
            <dt>Missingness</dt>
            <dd>When a metric was not observed or is not applicable, the site renders Unavailable with a reason. It never renders zero for missing data.</dd>
          </div>
          <div>
            <dt>Live values are provisional</dt>
            <dd>Metrics from an in-progress run may change as more assignments complete. Live runs are never ranked against terminal datasets by default.</dd>
          </div>
        </dl>
      </section>

      {methodology.limitations.length > 0 ? (
        <section className="methodology-limitations">
          <h2>Limitations</h2>
          <ul>
            {methodology.limitations.map((lim, i) => (
              <li key={i}>{lim}</li>
            ))}
          </ul>
        </section>
      ) : null}
    </div>
  );
}
