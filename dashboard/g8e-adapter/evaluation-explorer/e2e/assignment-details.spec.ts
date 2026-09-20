// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { expect, test, type Page, type Route } from '@playwright/test';
import { fixtureEnrichedAssignmentResult } from '../src/fixtures/fixtures';
import type { AssignmentResult, EvaluationSummary } from '../src/contract/types';

const mirrorOrigin = 'http://127.0.0.1:8082';
const datasetId = 'ds-browser-assignment';
const runId = 'run-browser-assignment';
const sourceId = 'browser-assignment-source';

function assignment(overrides: Partial<AssignmentResult> = {}): AssignmentResult {
  return {
    ...fixtureEnrichedAssignmentResult,
    dataset_id: datasetId,
    run_id: runId,
    assignment_id: 'assignment-1',
    variant_id: 'browser-model',
    ...overrides,
  };
}

function evaluation(): EvaluationSummary {
  return {
    schema_version: '1.4.0',
    kind: 'evaluation_summary',
    dataset_id: datasetId,
    quality_state: 'exploratory_partial',
    observed_at: '2026-09-20T00:00:00Z',
    run_id: runId,
    suite_id: 'browser-suite',
    arm: 'ensemble_ungoverned',
    evaluation_unit: 'model',
    lifecycle_state: 'completed',
    assignment_total: 3,
    assignment_completed: 3,
    assignment_failed: 0,
    terminal_outcomes: { completed: 3, model_failed: 0, grader_failed: 0, invalid_evidence: 0, stopped: 0 },
    verifier_state: 'not_applicable',
    headline_metrics: { pass_rate: { value: 0.67 } },
  };
}

function feedRecords(records: unknown[]) {
  return records.map((record, index) => ({
    sequence: index + 1,
    record_type: 'projection',
    ...(record as Record<string, unknown>),
  }));
}

async function installMirror(page: Page, records: unknown[], options: { malformed?: boolean } = {}): Promise<string[]> {
  const requests: string[] = [];
  await page.route(`${mirrorOrigin}/**`, async (route: Route) => {
    const url = new URL(route.request().url());
    requests.push(url.origin + url.pathname + url.search);
    if (url.pathname === '/bootstrap') {
      const snapshot = {
        protocol_version: '1.0.0',
        source_id: sourceId,
        high_water_sequence: records.length,
        feed_chain_hash: 'a'.repeat(64),
        batch_count: 1,
        generated_at: '2026-09-20T00:00:00Z',
        freshness: 'active',
      };
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          protocol_version: '1.0.0',
          snapshot,
          source_freshness: 'active',
          recent_projections: [],
          proof_catalog_summary: { artifact_count: 0, total_byte_size: 0 },
          generated_at: snapshot.generated_at,
        }),
      });
      return;
    }
    if (url.pathname === '/history') {
      const items = feedRecords(records).map(({ sequence, record_type, ...record }) => ({
        sequence,
        record_type,
        ...(options.malformed && sequence === 1 ? { ...record, private_prompt: 'must never render' } : record),
      }));
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ protocol_version: '1.0.0', items, has_more: false, limit: 100 }),
      });
      return;
    }
    if (url.pathname === '/snapshot') {
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          protocol_version: '1.0.0',
          source_id: sourceId,
          high_water_sequence: records.length,
          feed_chain_hash: 'a'.repeat(64),
          batch_count: 1,
          generated_at: '2026-09-20T00:00:00Z',
          freshness: 'active',
        }),
      });
      return;
    }
    if (url.pathname === '/stream') {
      await route.fulfill({
        status: 200,
        contentType: 'text/event-stream',
        body: ': connected\n\n',
      });
      return;
    }
    await route.abort();
  });
  await page.route('**/runtime.json', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ schema_version: '1.0.0', mirror_origin: mirrorOrigin }),
    });
  });
  return requests;
}

function expectPublicOnly(requests: string[]): void {
  expect(requests.every((request) => {
    const url = new URL(request);
    return url.origin === mirrorOrigin && ['/bootstrap', '/history', '/snapshot', '/stream'].some((path) => url.pathname === path);
  })).toBe(true);
  expect(requests.some((request) => /api|gateway|operator|ensemble|audit|receipt|tool/i.test(request))).toBe(false);
}

test('renders rich assignment evidence from mirror history and follows sibling repetition links', async ({ page }) => {
  const records = [
    evaluation(),
    assignment(),
    assignment({ assignment_id: 'assignment-2', repetition: 2 }),
    assignment({ assignment_id: 'assignment-3', repetition: 3 }),
  ];
  const requests = await installMirror(page, records);

  await page.goto(`/evaluations/${datasetId}/${runId}/assignments/assignment-1`);
  await expect(page.getByRole('heading', { name: 'Assignment context' })).toBeVisible();
  await expect(page.getByText('Follows a bounded response-format instruction.')).toBeVisible();
  await expect(page.getByRole('heading', { name: 'What happened' })).toBeVisible();
  await expect(page.getByText('1 observed')).toBeVisible();
  await expect(page.getByText('Criterion passed')).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Evidence and methodology' })).toBeVisible();
  await expect(page.getByText('Evaluation projection')).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Sibling repetitions' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'assignment-2' })).toHaveAttribute('href', `/evaluations/${datasetId}/${runId}/assignments/assignment-2`);
  await expect(page.getByRole('link', { name: 'assignment-3' })).toBeVisible();
  await expect(page.getByText('Stage timeline not published for this assignment.')).not.toBeVisible();

  await page.reload();
  await expect(page.getByRole('heading', { name: 'Assignment context' })).toBeVisible();
  await expect(page.getByText('Evaluation projection')).toBeVisible();
  expectPublicOnly(requests);
});

test('preserves legacy and unavailable evidence without rendering private fields', async ({ page }) => {
  const legacy = assignment({
    schema_version: '1.3.0',
    assignment_id: 'legacy-assignment',
    scenario_id: undefined,
    scenario_summary: undefined,
    activity_summary: undefined,
    evidence_bindings: undefined,
    resource_summary: undefined,
    verification_metadata: undefined,
    missingness_reason: 'Historical evidence was not captured.',
    stage_summary: [],
  });
  const requests = await installMirror(page, [evaluation(), legacy]);

  await page.goto(`/evaluations/${datasetId}/${runId}/assignments/legacy-assignment`);
  await expect(page.getByText('Historical evidence was not captured.')).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Evidence and methodology' })).toBeVisible();
  await expect(page.getByText('No public proof bindings were published for this assignment.')).toBeVisible();
  await expect(page.getByText('Stage timeline not published for this assignment.')).toBeVisible();
  await expect(page.getByTestId('metric-latency')).toContainText('Unavailable');
  await expect(page.getByText('private_prompt')).not.toBeVisible();
  expectPublicOnly(requests);
});

test('rejects a malicious assignment record before rendering it', async ({ page }) => {
  const requests = await installMirror(page, [
    { ...assignment(), private_prompt: 'never render this prompt' },
  ], { malformed: true });

  await page.goto(`/evaluations/${datasetId}/${runId}/assignments/assignment-1`);
  await expect(page.getByText('never render this prompt')).not.toBeVisible();
  await expect(page.getByText('No assignment selected.')).not.toBeVisible();
  await expect(page.getByText('No records available.')).toBeVisible();
  await expect(page.getByRole('alert')).toContainText('Feed validation errors');
  expectPublicOnly(requests);
});
