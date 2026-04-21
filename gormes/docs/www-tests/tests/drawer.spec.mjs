import { test, expect } from '@playwright/test';

test('mobile hamburger is an accessible button', async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 760 });
  await page.goto('/using-gormes/quickstart/');

  const btn = page.locator('[data-testid="drawer-open"]');
  await expect(btn).toBeVisible();
  // It's a real button, not a label
  const tagName = await btn.evaluate(el => el.tagName);
  expect(tagName).toBe('BUTTON');
  // Accessible name is set
  await expect(btn).toHaveAttribute('aria-label', /nav/i);
});

test('mobile drawer opens via hamburger and closes via backdrop', async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 760 });
  await page.goto('/using-gormes/quickstart/');

  const sidebar = page.locator('.docs-sidebar');
  let leftBefore = await sidebar.evaluate(el => el.getBoundingClientRect().left);
  expect(leftBefore).toBeLessThan(0);

  await page.locator('[data-testid="drawer-open"]').click();
  await page.waitForTimeout(250);
  const leftOpen = await sidebar.evaluate(el => el.getBoundingClientRect().left);
  expect(leftOpen).toBeGreaterThanOrEqual(0);

  await page.locator('.drawer-backdrop').click({ force: true });
  await page.waitForTimeout(250);
  const leftClosed = await sidebar.evaluate(el => el.getBoundingClientRect().left);
  expect(leftClosed).toBeLessThan(0);
});

test('desktop >=768px does not show the hamburger', async ({ page }) => {
  await page.setViewportSize({ width: 1024, height: 768 });
  await page.goto('/');
  const btn = page.locator('[data-testid="drawer-open"]');
  const display = await btn.evaluate(el => getComputedStyle(el).display);
  expect(display).toBe('none');
});
