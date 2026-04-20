import { test, expect } from '@playwright/test';

test('homepage renders the redesigned landing', async ({ page }) => {
  await page.goto('/');

  await expect(page).toHaveTitle('Gormes — Hermes, In a Single Static Binary');
  await expect(page.getByRole('heading', { name: 'Hermes, In a Single Static Binary.' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Why a Go layer matters.' })).toBeVisible();
  await expect(page.getByRole('heading', { name: "What ships now, what doesn't." })).toBeVisible();
  await expect(page.getByText('curl -fsSL https://gormes.ai/install.sh | sh')).toBeVisible();
  await expect(page.getByText('Requires Hermes backend at localhost:8642.')).toBeVisible();
  await expect(page.getByText('Run Hermes Through a Go Operator Console.')).toHaveCount(0);
  await expect(page.locator('link[href="/static/site.css"]')).toHaveCount(1);
  await expect(page.locator('script')).toHaveCount(0);
});

test('mobile keeps the install command readable', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/');

  await expect(page.getByRole('heading', { name: 'Hermes, In a Single Static Binary.' })).toBeVisible();
  await expect(page.getByText('curl -fsSL https://gormes.ai/install.sh | sh')).toBeVisible();

  const hasOverflow = await page.evaluate(() =>
    document.documentElement.scrollWidth > window.innerWidth
  );
  expect(hasOverflow).toBeFalsy();
});
