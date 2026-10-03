// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import {
  formatHintArguments,
  TRAJECTORY_POLICY_META,
  type ScenarioTaskDefinition,
} from '../content/scenario-task';

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
  const hasWorkspaceFiles = Boolean(task.workspaceFiles && task.workspaceFiles.length > 0);
  const hasAttachments = hasInlineContext || hasWorkspaceFiles;

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

      {hasWorkspaceFiles ? (
        <div className="task-attachments-list">
          {task.workspaceFiles!.map((file, idx) => (
            <div key={`${file.relPath}-${idx}`} className="task-attachment-item">
              <div className="task-attachment-header">
                <span className="task-attachment-label">{file.label}</span>
                <span className="task-attachment-kind">{file.decoy ? 'decoy file' : 'workspace file'}</span>
                <span className="task-attachment-path">{file.relPath}</span>
              </div>
              <pre className="task-attachment-content"><code>{file.content}</code></pre>
            </div>
          ))}
        </div>
      ) : null}

      {!hasAttachments ? (
        <p className="task-provided-none">No synthetic content provided (raw prompt only).</p>
      ) : null}

      <p className="task-system-context">
        The model is always offered the full production tool set. The scenario&apos;s allowed tools
        are graded, not hidden.
      </p>

      {task.promptHint ? (
        <div className="task-provided-tools">
          <strong>Prompt hint:</strong>{' '}
          <span>
            {task.promptHint.hintedTools.join(', ')} ({formatHintArguments(task.promptHint)})
          </span>
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
        <p className="task-section-note">
          <strong>{TRAJECTORY_POLICY_META[task.trajectoryPolicy].label}.</strong>{' '}
          {TRAJECTORY_POLICY_META[task.trajectoryPolicy].note}
        </p>
      </section>

      {taskHasToolBoundaries(task) ? (
        <section className="task-detail-section" aria-label="Tool boundaries">
          <h2>Tool boundaries</h2>
          <p className="task-section-note">
            The model is offered every production tool. <strong>Allowed</strong> tools are what the
            scenario permits: calling any other tool and having it succeed fails the scenario.{' '}
            <strong>Forbidden</strong> tools are the ones the scenario must not see used. No tool
            is both.
          </p>
          <div className="task-tool-grid">
            {task.allowedTools?.length ? (
              <div>
                <h3>Allowed tools</h3>
                <p className="task-tool-column-note">Permitted by the scenario.</p>
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
