// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { expect, test, type Page, type Route } from '@playwright/test';

const mirrorOrigin = process.env.G8E_PUBLIC_MIRROR_ORIGIN ?? 'http://127.0.0.1:8082';
const datasetId = 'native-core-execution-boundary';
const runIds = [
  'f5980a8c-de6f-4e03-9ecf-34b536060697',
  '19dcd7b1-3d0f-4d05-a2f7-a46965c382bd',
  '355dcf53-eada-4faf-9dc2-d9e3800f7de7',
];
const sourceId = 'browser-native-evaluation-source';

function nativeEvaluation(runId: string) {
  const verdicts = (prefix: string) => Array.from({ length: 5 }, (_, index) => ({
    assertion_id: `${prefix}-${index + 1}`,
    assertion_version: '1.0.0',
    status: 'pass',
  }));
  return {
    schema_version: '1.3.0',
    kind: 'evaluation_summary',
    dataset_id: datasetId,
    quality_state: 'verified_public',
    observed_at: '2026-09-15T23:38:22Z',
    run_id: runId,
    suite_id: 'core-execution-boundary@1.0.0',
    arm: 'platform',
    evaluation_unit: 'system',
    model_role_mapping: {},
    lifecycle_state: 'completed',
    assignment_total: 2,
    assignment_completed: 2,
    assignment_failed: 0,
    terminal_outcomes: { completed: 1, model_failed: 0, grader_failed: 0, invalid_evidence: 0, stopped: 0 },
    started_at: '2026-09-15T23:38:21Z',
    ended_at: '2026-09-15T23:38:22Z',
    elapsed_seconds: 1,
    verifier_state: 'passed',
    headline_metrics: { pass_rate: { value: 1 } },
    native_result: {
      active_posture: 'doctrine',
      lane: 'platform',
      summary_status: 'pass',
      summary: '10/10 required invariants passed',
      required_verdict_count: 10,
      passed_verdict_count: 10,
      verification_valid: true,
      verification_failure_count: 0,
      scenarios: [
        {
          scenario_id: 'allowed-execution-occurs-once',
          scenario_version: '1.0.0',
          status: 'completed',
          verdicts: verdicts('allowed-effect'),
        },
        {
          scenario_id: 'prohibited-equivalent-causes-no-additional-effect',
          scenario_version: '1.0.0',
          status: 'rejected',
          verdicts: verdicts('prohibited-effect'),
        },
      ],
      metrics: [{ metric_id: 'required-verdict-pass-rate', metric_version: '1.0.0', numerator: 10, denominator: 10, value: 1, unit: 'ratio' }],
    },
  };
}

async function installMirror(page: Page): Promise<void> {
  const records = runIds.map(nativeEvaluation);
  await page.route(`${mirrorOrigin}/**`, async (route: Route) => {
    const url = new URL(route.request().url());
    if (url.pathname === '/bootstrap') {
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          protocol_version: '1.0.0',
          snapshot: {
            protocol_version: '1.0.0',
            source_id: sourceId,
            high_water_sequence: records.length,
            feed_chain_hash: 'b'.repeat(64),
            batch_count: 1,
            generated_at: '2026-09-20T00:00:00Z',
            freshness: 'active',
          },
          source_freshness: 'active',
          recent_projections: [],
          proof_catalog_summary: { artifact_count: 0, total_byte_size: 0 },
          generated_at: '2026-09-20T00:00:00Z',
        }),
      });
      return;
    }
    if (url.pathname === '/history') {
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          protocol_version: '1.0.0',
          items: records.map((record, index) => ({ sequence: index + 1, record_type: 'projection', ...record })),
          has_more: false,
          limit: 100,
        }),
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
          feed_chain_hash: 'b'.repeat(64),
          batch_count: 1,
          generated_at: '2026-09-20T00:00:00Z',
          freshness: 'active',
        }),
      });
      return;
    }
    if (url.pathname === '/stream') {
      await route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': connected\n\n' });
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
}

async function expectNativeRun(page: Page, runId: string) {
  await page.goto(`/evaluations/${datasetId}/${runId}`);
  await expect(page.getByRole('heading', { name: runId })).toBeVisible();
  await expect(page.getByText('core-execution-boundary@1.0.0', { exact: true })).toBeVisible();
  await expect(page.getByText('10/10 required invariants passed', { exact: true })).toBeVisible();
  await expect(page.getByText('Valid (0 failures)', { exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'allowed-execution-occurs-once@1.0.0' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'prohibited-equivalent-causes-no-additional-effect@1.0.0' })).toBeVisible();
  await expect(page.locator('.native-scenario').filter({ hasText: 'allowed-execution-occurs-once' })).toContainText('Status: completed');
  await expect(page.locator('.native-scenario').filter({ hasText: 'prohibited-equivalent-causes-no-additional-effect' })).toContainText('Status: rejected');
  await expect(page.locator('.native-scenario li')).toHaveCount(10);
}

test('reconstructs native evaluations from public mirror history', async ({ page }) => {
  if (!process.env.G8E_PUBLIC_MIRROR_ORIGIN) await installMirror(page);
  const consoleErrors: string[] = [];
  const failedRequests: string[] = [];
  const mirrorResponses: string[] = [];

  page.on('console', (message) => {
    if (message.type() === 'error') consoleErrors.push(message.text());
  });
  page.on('requestfailed', (request) => failedRequests.push(request.url()));
  page.on('response', (response) => {
    if (response.url().startsWith(`${mirrorOrigin}/`)) mirrorResponses.push(response.url());
  });

  await page.goto('/evaluations');
  await expect(page.getByRole('heading', { name: 'Evaluation runs' })).toBeVisible();
  for (const runId of runIds) await expect(page.getByRole('link', { name: runId })).toBeVisible();
  await expect.poll(() => mirrorResponses.some((url) => url.endsWith('/bootstrap'))).toBe(true);
  await expect.poll(() => mirrorResponses.some((url) => url.includes('/history?'))).toBe(true);

  for (const runId of runIds) await expectNativeRun(page, runId);

  await page.reload();
  await expect(page.getByRole('heading', { name: runIds[2] })).toBeVisible();
  await expect(page.getByText('10/10 required invariants passed', { exact: true })).toBeVisible();
  await expect(page.locator('.native-scenario li')).toHaveCount(10);

  await page.goto('/evaluations');
  for (const runId of runIds) await expect(page.getByRole('link', { name: runId })).toBeVisible();

  expect(consoleErrors).toEqual([]);
  expect(failedRequests.filter((url) => !url.includes('/stream?'))).toEqual([]);
});
