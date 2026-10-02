// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Condensed, interactive primer for the Overview page: the Docs execution
// pipeline as a stepper, and the Tasks catalog as category jump-links. All copy
// comes from the same content modules the Docs and Tasks pages render.

import { useState } from 'react';
import { Link } from 'react-router-dom';
import { EXECUTION_STAGES } from '../content/execution-stages';
import { SCENARIO_CATEGORY_META, SCENARIO_TASKS } from '../content/scenario-task';
import { SCENARIO_CATEGORIES } from '../contract/types';

export function WhatAmILookingAt() {
  const [active, setActive] = useState(0);
  const stage = EXECUTION_STAGES[active];

  return (
    <section className="panel wala-panel" aria-label="What am I looking at?">
      <div className="panel-head">
        <div>
          <h2>What am I looking at?</h2>
          <p className="panel-note">Each row above is one task, run by one model, through the governed path below.</p>
        </div>
      </div>

      <h3 className="wala-kicker">How a result is made</h3>
      <ol className="wala-rail" role="tablist" aria-label="Execution stages">
        {EXECUTION_STAGES.map((item, index) => (
          <li key={item.label} className="wala-rail-item" data-active={index === active || undefined}>
            <button
              type="button"
              role="tab"
              id={`wala-tab-${index}`}
              aria-selected={index === active}
              aria-controls="wala-detail"
              className="wala-rail-button"
              onClick={() => setActive(index)}
            >
              <span className="wala-rail-number" aria-hidden="true">{index + 1}</span>
              <span className="wala-rail-label">{item.label}</span>
            </button>
          </li>
        ))}
      </ol>
      <div className="wala-detail" id="wala-detail" role="tabpanel" aria-labelledby={`wala-tab-${active}`}>
        {stage ? (
          <>
            <p className="wala-detail-system">{stage.system}</p>
            <p className="wala-detail-text">{stage.detail}</p>
            <code className="wala-detail-output">→ {stage.output}</code>
          </>
        ) : null}
      </div>

      <h3 className="wala-kicker">What gets tested</h3>
      <ul className="wala-chips" aria-label="Task categories">
        {SCENARIO_CATEGORIES.map((category) => {
          const meta = SCENARIO_CATEGORY_META[category];
          return (
            <li key={category}>
              <Link to={`/tasks#category-${category}`} className="wala-chip" title={meta.blurb}>
                {meta.label}
                <span className="wala-chip-count">{meta.count}</span>
              </Link>
            </li>
          );
        })}
      </ul>

      <div className="wala-foot">
        <Link to="/methodology" className="panel-link">Read the Docs →</Link>
        <Link to="/tasks" className="panel-link">Browse all {SCENARIO_TASKS.length} tasks →</Link>
      </div>
    </section>
  );
}
