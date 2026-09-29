// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import type { ScenarioTaskDefinition } from '../content/scenario-catalog';

function taskHasToolBoundaries(task: ScenarioTaskDefinition): boolean {
  return Boolean(task.allowedTools?.length || task.expectedTools?.length || task.forbiddenTools?.length);
}

function gradingMethodNote(task: ScenarioTaskDefinition): string {
  const base =
    task.gradingMethod === 'semantic_judge'
      ? 'An LLM judge scores the interaction against this rubric.'
      : 'Fixed rules over trace evidence decide pass or fail.';
  return taskHasToolBoundaries(task)
    ? `${base} Tool boundaries below refine what counts as a correct tool choice.`
    : base;
}

export function TaskPromptSection({
  task,
  className,
}: {
  task: ScenarioTaskDefinition;
  className?: string;
}) {
  return (
    <section
      className={`task-detail-section assignment-section-asked${className ? ` ${className}` : ''}`}
      aria-label="What the model is asked"
    >
      <h2>What the model is asked</h2>
      <p className="task-section-note">
        User prompt sent to the agent.
      </p>
      <blockquote className="task-prompt">{task.userPrompt}</blockquote>
    </section>
  );
}

export function TaskProvidedSection({
  task,
  className,
}: {
  task: ScenarioTaskDefinition;
  className?: string;
}) {
  const hasInlineContext = Boolean(task.inlineContext && task.inlineContext.length > 0);
  const hasSimulatedFiles = Boolean(task.simulatedFiles && task.simulatedFiles.length > 0);
  const hasAttachments = hasInlineContext || hasSimulatedFiles;
  const hasTools = Boolean(task.allowedTools && task.allowedTools.length > 0);
  const hasSystemContext = Boolean(task.systemContext);

  return (
    <section
      className={`task-detail-section assignment-section-provided${className ? ` ${className}` : ''}`}
      aria-label="What the model was provided"
    >
      <h2>What the model was provided</h2>
      <p className="task-section-note">
        Context, synthetic content, and tool boundaries supplied with the task.
      </p>

      {hasInlineContext ? (
        <div className="task-attachments-list">
          {task.inlineContext!.map((att, idx) => (
            <div key={`${att.label}-${idx}`} className="task-attachment-item">
              <div className="task-attachment-header">
                <span className="task-attachment-label">{att.label}</span>
                <span className="task-attachment-kind">{att.kind}</span>
              </div>
              <pre className="task-attachment-content"><code>{att.content}</code></pre>
            </div>
          ))}
        </div>
      ) : null}

      {hasSimulatedFiles ? (
        <div className="task-attachments-list">
          {task.simulatedFiles!.map((file, idx) => (
            <div key={`${file.label}-${idx}`} className="task-attachment-item">
              <div className="task-attachment-header">
                <span className="task-attachment-label">{file.label}</span>
                <span className="task-attachment-kind">{file.kind}</span>
                <span className="task-attachment-path">{file.path}</span>
              </div>
              <pre className="task-attachment-content"><code>{file.content}</code></pre>
            </div>
          ))}
        </div>
      ) : null}

      {!hasAttachments ? (
        <p className="task-provided-none">No synthetic content provided (raw prompt only).</p>
      ) : null}

      {hasSystemContext ? (
        <p className="task-system-context">
          <strong>System context:</strong> {task.systemContext}
        </p>
      ) : null}

      {hasTools ? (
        <div className="task-provided-tools">
          <strong>Allowed tools:</strong>
          <div className="task-tool-chips">
            {task.allowedTools!.map((tool) => (
              <code key={tool} className="task-tool-chip">{tool}</code>
            ))}
          </div>
        </div>
      ) : null}
    </section>
  );
}

export function TaskExpectationSection({
  task,
  className,
}: {
  task: ScenarioTaskDefinition;
  className?: string;
}) {
  return (
    <>
      <section
        className={`task-detail-section assignment-section-expected${className ? ` ${className}` : ''}`}
        aria-label="What the model is expected to do"
      >
        <h2>Pass criteria</h2>
        <p className="task-section-note">{gradingMethodNote(task)}</p>
        <p>{task.expectedBehavior}</p>
      </section>

      {taskHasToolBoundaries(task) ? (
        <section className="task-detail-section" aria-label="Tool boundaries">
          <h2>Tool boundaries</h2>
          <p className="task-section-note">
            Three separate policy dimensions, not one whitelist. A tool may appear in both{' '}
            <strong>Allowed</strong> and <strong>Forbidden</strong>: available as a choice, but
            calling it fails the scenario. Common in tool-selection and security-policy tasks.
          </p>
          <div className="task-tool-grid">
            {task.allowedTools?.length ? (
              <div>
                <h3>Allowed tools</h3>
                <p className="task-tool-column-note">Exposed to the model during the scenario.</p>
                <ul>
                  {task.allowedTools.map((tool) => (
                    <li key={tool}><code>{tool}</code></li>
                  ))}
                </ul>
              </div>
            ) : null}
            {task.expectedTools?.length ? (
              <div>
                <h3>Expected tools</h3>
                <p className="task-tool-column-note">Tools the grader expects the model to call.</p>
                <ul>
                  {task.expectedTools.map((tool) => (
                    <li key={tool}><code>{tool}</code></li>
                  ))}
                </ul>
              </div>
            ) : null}
            {task.forbiddenTools?.length ? (
              <div>
                <h3>Forbidden tools</h3>
                <p className="task-tool-column-note">Tools that fail the scenario if the model calls them.</p>
                <ul>
                  {task.forbiddenTools.map((tool) => (
                    <li key={tool}><code>{tool}</code></li>
                  ))}
                </ul>
              </div>
            ) : null}
          </div>
        </section>
      ) : null}
    </>
  );
}
