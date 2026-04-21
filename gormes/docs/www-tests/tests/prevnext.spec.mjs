import { test, expect } from '@playwright/test';

test('single page has prev/next links at bottom', async ({ page }) => {
  await page.goto('/using-gormes/quickstart/');
  const nav = page.locator('nav.docs-prevnext');
  await expect(nav).toBeVisible();

  // At least one of prev or next must exist on a non-boundary page.
  const anchors = nav.locator('a');
  const count = await anchors.count();
  expect(count).toBeGreaterThanOrEqual(1);

  // Each anchor has a direction label + a page title.
  for (let i = 0; i < count; i++) {
    const a = anchors.nth(i);
    await expect(a.locator('.docs-prevnext-dir')).toBeVisible();
    await expect(a.locator('.docs-prevnext-title')).toBeVisible();
  }
});

test('section index page does not show prev/next', async ({ page }) => {
  await page.goto('/using-gormes/');
  const nav = page.locator('nav.docs-prevnext');
  await expect(nav).toHaveCount(0);
});
