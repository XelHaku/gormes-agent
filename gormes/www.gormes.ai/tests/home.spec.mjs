import { test, expect } from '@playwright/test';

test('homepage renders the operational-moat story', async ({ page }) => {
  await page.goto('/');

  await expect(page).toHaveTitle('Gormes.ai | The Agent That GOes With You.');
  await expect(page.getByRole('heading', { name: 'The Agent That GOes With You.' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Quick Start' })).toBeVisible();
  await expect(page.getByText('7.9 MB Static Binary')).toBeVisible();
  await expect(page.getByText('Phase 2 is live on trunk')).toBeVisible();
  await expect(page.locator('.hero')).toHaveCount(1);
  await expect(page.locator('.hero-cta-row')).toHaveCount(1);
  await expect(page.locator('link[href="/static/site.css"]')).toHaveCount(1);
  await expect(page.locator('script')).toHaveCount(0);
  await expect(page.locator('body')).not.toContainText('gormes.io');
});
