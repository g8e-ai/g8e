// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { expect, test, type Page, type Route } from '@playwright/test';
import { fixtureEvaluationSummaries, fixtureModelSummaries } from '../src/fixtures/fixtures';
import type { EvaluationSummary, ModelSummary } from '../src/contract/types';

const mirrorOrigin = 'http://127.0.0.1:8082';
const verifiedDataset = 'ds-browser-verified';
const partialDataset = 'ds-browser-partial';
const runId = 'run-browser-verified';
const sourceId = 'browser-model-quality-source';

function model(datasetId: string, qualityState: ModelSummary['quality_state'], role: ModelSummary['role'] = 'primary'): ModelSummary {
  return {
    ...fixtureModelSummaries[0]!,
    dataset_id: datasetId,
    variant_id: 'quality-model',
    display_name: role === 'primary' ? 'Quality Model' : 'Quality Model Assistant',
    role,
    quality_state: qualityState,
    schema_version: '1.4.0',
    pass_rate: { estimate: 0.75, lower: 0.6, upper: 0.85, denominator: 4 },
  };
}

function evaluation(qualityState: EvaluationSummary['quality_state']): EvaluationSummary {
  return {
    ...fixtureEvaluationSummaries[0]!,
    dataset_id: verifiedDataset,
    run_id: runId,
    schema_version: '1.4.0',
    quality_state: qualityState,
    verifier_state: qualityState === 'exploratory_verified' ? 'passed' : 'not_applicable',
  };
}

function recordsForFeed(records: unknown[]) {
  return records.map((record, index) => ({
    sequence: index + 1,
    record_type: 'projection',
    ...(record as Record<string, unknown>),
  }));
}

async function installMirror(page: Page, records: unknown[]): Promise<string[]> {
  const requests: string[] = [];
  await page.route(`${mirrorOrigin}/**`, async (route: Route) => {
    const url = new URL(route.request().url());
    requests.push(url.origin + url.pathname + url.search);
    if (url.pathname === '/bootstrap') {
      const snapshot = {
        protocol_version: '1.0.0',
        source_id: sourceId,
        high_water_sequence: records.length,
        feed_chain_hash: 'b'.repeat(64),
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
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ protocol_version: '1.0.0', items: recordsForFeed(records), has_more: false, limit: 100 }),
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
  return requests;
}

function expectPublicOnly(requests: string[]): void {
  expect(requests.every((request) => {
    const url = new URL(request);
    return url.origin === mirrorOrigin && ['/bootstrap', '/history', '/snapshot', '/stream'].some((path) => url.pathname === path);
  })).toBe(true);
  expect(requests.some((request) => /api|gateway|operator|ensemble|audit|receipt|tool/i.test(request))).toBe(false);
}

test('replaces partial model revisions with verified revisions and keeps scope isolated', async ({ page }) => {
  const records = [
    model(verifiedDataset, 'exploratory_partial'),
    evaluation('exploratory_partial'),
    evaluation('exploratory_verified'),
    model(verifiedDataset, 'exploratory_verified'),
    model(partialDataset, 'exploratory_partial'),
    model(verifiedDataset, 'exploratory_partial', 'assistant'),
  ];
  const requests = await installMirror(page, records);

  await page.goto('/evaluations');
  await expect(page.getByRole('heading', { name: 'Evaluation runs' })).toBeVisible();
  await expect(page.getByTestId('quality-exploratory_verified')).toBeVisible();
  await expect(page.getByTestId('quality-exploratory_verified')).toHaveText('Run-scoped verification passed');

  await page.goto('/models');
  await expect(page.locator(`a[href="/models/${verifiedDataset}/quality-model?role=primary"]`)).toBeVisible();
  await expect(page.getByTestId('quality-exploratory_verified')).toBeVisible();
  await expect(page.getByTestId('quality-exploratory_verified')).toHaveText('Run-scoped verification passed');
  await expect(page.getByText('Quality Model Assistant')).toBeVisible();
  await expect(page.locator('tr').filter({ has: page.getByRole('link', { name: 'Quality Model Assistant' }) })).toContainText('Not fully verified');
  await expect(
    page.locator('tr').filter({ has: page.locator(`a[href="/models/${partialDataset}/quality-model?role=primary"]`) }),
  ).toContainText('Not fully verified');

  await page.getByLabel('Filter by quality state').selectOption('exploratory_verified');
  await expect(page.locator(`a[href="/models/${verifiedDataset}/quality-model?role=primary"]`)).toBeVisible();
  await expect(page.getByRole('link', { name: 'Quality Model Assistant' })).not.toBeVisible();
  await expect(page.getByText('Quality Model Assistant')).not.toBeVisible();

  await page.reload();
  await expect(page.locator(`a[href="/models/${verifiedDataset}/quality-model?role=primary"]`)).toBeVisible();
  await expect(page.getByTestId('quality-exploratory_verified')).toBeVisible();
  expectPublicOnly(requests);
});

test('downgrades stale verified-public model records before rendering', async ({ page }) => {
  const stale = {
    ...model(verifiedDataset, 'verified_public'),
    schema_version: '1.2.0',
    evaluation_coverage: 0.2,
  };
  await installMirror(page, [stale]);

  await page.goto('/models');
  const row = page.locator('tr').filter({ has: page.getByRole('link', { name: 'Quality Model' }) });
  await expect(row).toContainText('20.0%');
  await expect(row).toContainText('Legacy · not current-standard verified');
  await expect(row).not.toContainText('Current-standard verified');
});

test('does not promote model rows from an evaluation-only update', async ({ page }) => {
  const records = [model(verifiedDataset, 'exploratory_partial'), evaluation('exploratory_partial'), evaluation('exploratory_verified')];
  await installMirror(page, records);

  await page.goto('/models');
  await expect(page.getByRole('link', { name: 'Quality Model' })).toBeVisible();
  await expect(page.getByTestId('quality-exploratory_partial')).toBeVisible();
  await expect(page.getByTestId('quality-exploratory_verified')).not.toBeVisible();
});
