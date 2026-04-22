import { test, expect } from '@playwright/test';

test('docs home hero, quickstart, and three enhanced cards render', async ({ page }) => {
  await page.goto('/');

  await expect(page).toHaveTitle(/Gormes Docs/);
  // Hero
  await expect(page.locator('.docs-home-hero h1')).toBeVisible();
  await expect(page.locator('.docs-home-hero .kicker')).toBeVisible();

  // Quickstart strip
  const qs = page.locator('.docs-home-quickstart');
  await expect(qs).toBeVisible();
  await expect(qs.locator('code')).toContainText('curl -fsSL https://gormes.ai/install.sh | sh');
  await expect(qs.locator('code')).not.toContainText('brew install trebuchet/gormes');

  // Three enhanced cards with ordinals and mini-TOCs
  const cards = page.locator('.docs-home-card');
  await expect(cards).toHaveCount(3);
  for (let i = 0; i < 3; i++) {
    const c = cards.nth(i);
    await expect(c.locator('.docs-home-card-ordinal')).toBeVisible();
    await expect(c.locator('.docs-home-card-mini-toc li')).toHaveCount(3);
    await expect(c.locator('.docs-home-card-cta')).toContainText(/Explore/i);
  }

  // Kickers map to the existing colored labels
  await expect(page.getByRole('link', { name: /USING GORMES/i })).toBeVisible();
  await expect(page.getByRole('link', { name: /BUILDING GORMES/i })).toBeVisible();
  await expect(page.getByRole('link', { name: /UPSTREAM HERMES/i })).toBeVisible();

  // Sidebar unchanged
  await expect(page.locator('.docs-nav-group-label-shipped')).toBeVisible();
  await expect(page.locator('.docs-nav-group-label-progress')).toBeVisible();
  await expect(page.locator('.docs-nav-group-label-next')).toBeVisible();

  // External script budget: pagefind-ui.js + site.js (always) + livereload.js
  // (Hugo dev server only). Filter livereload so the assertion holds in both
  // dev and prod modes.
  const scripts = await page
    .locator('script[src]')
    .evaluateAll(els => els.filter(el => !el.src.includes('livereload')).length);
  expect(scripts).toBeLessThanOrEqual(2);
});
