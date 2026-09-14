import { expect, test } from '@playwright/test';

const mirrorOrigin = process.env.G8E_PUBLIC_MIRROR_ORIGIN ?? 'http://127.0.0.1:8082';

test('reconstructs the evaluation explorer from the public mirror', async ({ page }) => {
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

  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'AI evaluations. Working in public.' })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Feed status' })).toContainText(/Snapshot sealed|mirror stream is unreachable/);
  await expect(page.getByRole('region', { name: 'Live evaluation' })).toContainText(/failed|stopped|completed|No evaluation has been observed/);
  await expect(page.getByRole('region', { name: 'Dataset summary' })).toContainText('1,125');
  await expect.poll(() => mirrorResponses.some((url) => url.endsWith('/bootstrap'))).toBe(true);
  await expect.poll(() => mirrorResponses.some((url) => url.includes('/history?'))).toBe(true);

  await page.getByRole('link', { name: 'Models', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'All known models, evaluated or not' })).toBeVisible();
  await expect(page.locator('.result-count')).toHaveText(/3[12] models/);
  await page.getByRole('searchbox', { name: 'Search models' }).fill('qwen');
  await expect(page.locator('.result-count')).not.toHaveText('0 models');
  await page.getByRole('link', { name: 'Qwen3-4B', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Suite results' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Eligible' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Pass rate' })).toBeVisible();
  await page.locator('.model-assignments a').first().click();
  await expect(page.getByRole('heading', { name: 'Benchmark observations' })).toBeVisible();
  const scenarioRow = page.locator('.assignment-benchmark .detail-row').filter({ hasText: 'Scenario category' });
  await expect(scenarioRow).toBeVisible();
  await expect(scenarioRow).not.toContainText('Not observed');
  await expect(page.getByText('Model evaluation', { exact: true })).toBeVisible();
  await expect(page.getByText('Not observed in this dataset', { exact: true }).first()).toBeVisible();

  await page.getByRole('link', { name: 'Overview' }).click();
  await expect(page.getByRole('heading', { name: 'AI evaluations. Working in public.' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Four questions, not one score' })).toBeVisible();
  await page.getByRole('combobox', { name: 'Select dataset' }).selectOption('ds-verified-public-20260914');
  await expect(page.getByRole('region', { name: 'Dataset summary' })).toContainText('Verified public snapshot');

  await page.getByRole('link', { name: 'Methodology', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Four evaluation questions' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Roles are responsibilities' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Initial 25-scenario campaign' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Model and system leaderboards' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Escalation quality' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Tool-calling scorecard' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Event-based security and privacy' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Complete inference telemetry' })).toBeVisible();

  await page.setViewportSize({ width: 320, height: 800 });
  await page.getByRole('link', { name: 'Overview' }).click();
  await expect(page.getByRole('navigation', { name: 'Primary navigation' })).toBeVisible();
  await expect(page.getByRole('img', { name: 'Committed evaluation reports publish outward through the public mirror to this browser' })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Live evaluation' })).toBeVisible();
  await expect(page.getByRole('combobox', { name: 'Select dataset' })).toBeVisible();
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(320);

  await page.getByRole('link', { name: 'Models', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'All known models, evaluated or not' })).toBeVisible();
  const tableWrap = page.getByTestId('data-table');
  await expect(tableWrap).toBeVisible();
  const tableDimensions = await tableWrap.evaluate((element) => ({
    clientWidth: element.clientWidth,
    scrollWidth: element.scrollWidth,
  }));
  expect(tableDimensions.clientWidth).toBeLessThanOrEqual(300);
  expect(tableDimensions.scrollWidth).toBeGreaterThanOrEqual(tableDimensions.clientWidth);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(320);

  expect(consoleErrors).toEqual([]);
  expect(failedRequests.filter((url) => !url.includes('/stream?'))).toEqual([]);
});
