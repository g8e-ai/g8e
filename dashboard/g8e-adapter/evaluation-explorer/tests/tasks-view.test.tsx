// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, expect, it } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { TasksView } from '../src/views/TasksView';
import { SCENARIO_TASKS, scenarioTaskSourceUrl } from '../src/content/scenario-catalog';

function renderTasks(initialPath = '/tasks') {
  return render(
    <MemoryRouter initialEntries={[initialPath]}>
      <Routes>
        <Route path="/tasks" element={<TasksView />} />
        <Route path="/tasks/:taskId" element={<TasksView />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('TasksView', () => {
  it('lists all 25 tasks grouped by category on the overview', () => {
    renderTasks();

    expect(screen.getByRole('heading', { name: 'Task catalog' })).toBeInTheDocument();
    expect(screen.getByText('north-star-25@1.0.0')).toBeInTheDocument();
    expect(screen.getAllByRole('link', { name: 'View full definition' })).toHaveLength(SCENARIO_TASKS.length);
    expect(screen.getByRole('heading', { name: /Instruction adherence/i })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: /Security & policy/i })).toBeInTheDocument();
    expect(screen.getByText(/Cards show a prompt excerpt/i)).toBeInTheDocument();
  });

  it('shows a detailed definition for a selected task with g8e source links', () => {
    const task = SCENARIO_TASKS.find((entry) => entry.id === 'tool-select-grep');
    expect(task).toBeDefined();

    renderTasks('/tasks/tool-select-grep');

    const panel = screen.getByRole('article');
    expect(within(panel).getByRole('heading', { name: 'tool-select-grep' })).toBeInTheDocument();
    expect(within(panel).getByText(task!.publicDescription)).toBeInTheDocument();
    expect(within(panel).getByText(task!.userPrompt)).toBeInTheDocument();
    expect(within(panel).getByRole('heading', { name: 'Expected tools' })).toBeInTheDocument();
    expect(within(panel).getByText(/not one whitelist/i)).toBeInTheDocument();
    expect(within(panel).getByText(/Fixed rules over trace evidence/i)).toBeInTheDocument();
    expect(within(panel).getByText(/User prompt sent to the agent/i)).toBeInTheDocument();
    expect(within(panel).getByText(/Which tool is called affects the grade/i)).toBeInTheDocument();
    expect(within(panel).getByRole('link', { name: /Scenario definition in g8e/i })).toHaveAttribute(
      'href',
      scenarioTaskSourceUrl(task!),
    );
  });

  it('redirects unknown task ids back to the overview', () => {
    renderTasks('/tasks/not-a-real-task');

    expect(screen.getByRole('heading', { name: 'Task catalog' })).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'not-a-real-task' })).not.toBeInTheDocument();
  });
});
