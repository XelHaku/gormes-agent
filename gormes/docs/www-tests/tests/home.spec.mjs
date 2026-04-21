import { test, expect } from '@playwright/test';

test('docs home renders the three-audience split', async ({ page }) => {
  await page.goto('/');

  await expect(page).toHaveTitle(/Gormes Docs/);
  await expect(page.getByRole('heading', { name: 'Gormes Docs', level: 1 })).toBeVisible();

  // Three cards, one per audience
  await expect(page.getByRole('link', { name: /USING GORMES/i })).toBeVisible();
  await expect(page.getByRole('link', { name: /BUILDING GORMES/i })).toBeVisible();
  await expect(page.getByRole('link', { name: /UPSTREAM HERMES/i })).toBeVisible();

  // Sidebar has colored group labels
  await expect(page.locator('.docs-nav-group-label-shipped')).toBeVisible();
  await expect(page.locator('.docs-nav-group-label-progress')).toBeVisible();
  await expect(page.locator('.docs-nav-group-label-next')).toBeVisible();

  // External script budget: pagefind-ui.js + site.js (always) + livereload.js
  // (Hugo dev server only). Filter livereload so the assertion holds in both
  // dev and prod modes.
  const scripts = await page
    .locator('script[src]')
    .evaluateAll(els => els.filter(el => !el.src.includes('livereload')).length);
  expect(scripts).toBeLessThanOrEqual(2); // pagefind-ui.js + site.js
});
