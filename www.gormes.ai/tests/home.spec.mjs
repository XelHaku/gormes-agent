import { test, expect } from '@playwright/test';

test('homepage renders the phase-1 story', async ({ page }) => {
  await page.goto('/');

  await expect(page).toHaveTitle('Gormes.ai | The Agent That GOes With You.');
  await expect(page.getByRole('heading', { name: 'The Agent That GOes With You.' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Quick Start' })).toBeVisible();
  await expect(page.getByText('Phase 1 uses your existing Hermes backend.')).toBeVisible();
  await expect(page.locator('.hero')).toHaveCount(1);
  await expect(page.locator('.hero-cta-row')).toHaveCount(1);
  await expect(page.locator('link[href="/static/site.css"]')).toHaveCount(1);
  await expect(page.locator('script')).toHaveCount(0);
  await expect(page.locator('body')).not.toContainText('gormes.io');
});
