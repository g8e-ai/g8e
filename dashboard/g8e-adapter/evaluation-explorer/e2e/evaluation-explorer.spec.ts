// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { expect, test } from '@playwright/test';

const mirrorOrigin = process.env.G8E_PUBLIC_MIRROR_ORIGIN ?? 'http://127.0.0.1:8082';
const datasetId = 'native-core-execution-boundary';
const runIds = [
  'f5980a8c-de6f-4e03-9ecf-34b536060697',
  '19dcd7b1-3d0f-4d05-a2f7-a46965c382bd',
  '355dcf53-eada-4faf-9dc2-d9e3800f7de7',
];

async function expectNativeRun(page: import('@playwright/test').Page, runId: string) {
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
