// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import type { ScenarioTaskDefinition } from '../content/scenario-catalog';

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
  const hasAttachments = Boolean(task.attachments && task.attachments.length > 0);
  const hasTools = Boolean(task.allowedTools && task.allowedTools.length > 0);
  const hasSystemContext = Boolean(task.systemContext);

  return (
    <section
      className={`task-detail-section assignment-section-provided${className ? ` ${className}` : ''}`}
      aria-label="What the model was provided"
    >
      <h2>What the model was provided</h2>
      <p className="task-section-note">
        Context, synthetic attachments, and tool boundaries supplied with the task.
      </p>

      {hasAttachments ? (
        <div className="task-attachments-list">
          {task.attachments!.map((att, idx) => (
            <div key={`${att.label}-${idx}`} className="task-attachment-item">
              <div className="task-attachment-header">
                <span className="task-attachment-label">{att.label}</span>
                <span className="task-attachment-kind">{att.kind}</span>
              </div>
              <pre className="task-attachment-content"><code>{att.content}</code></pre>
            </div>
          ))}
        </div>
      ) : (
        <p className="task-provided-none">No synthetic attachments provided (raw prompt only).</p>
      )}

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
