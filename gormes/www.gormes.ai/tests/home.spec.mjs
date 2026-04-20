import { test, expect } from '@playwright/test';

test('homepage renders the redesigned landing', async ({ page }) => {
  await page.goto('/');

  await expect(page).toHaveTitle('Gormes — One Go Binary. Same Hermes Brain.');
  await expect(page.getByRole('heading', { name: 'One Go Binary. Same Hermes Brain.' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Why a Go layer matters.' })).toBeVisible();
  await expect(page.getByRole('heading', { name: "What ships now, what doesn't." })).toBeVisible();
  await expect(page.getByText('curl -fsSL https://gormes.ai/install.sh | sh')).toBeVisible();
  await expect(page.getByText('Requires Hermes backend at localhost:8642.')).toBeVisible();
  await expect(page.getByText('Run Hermes Through a Go Operator Console.')).toHaveCount(0);
  await expect(page.locator('link[href="/static/site.css"]')).toHaveCount(1);
  // Copy buttons require a tiny inline clipboard script — bounded to install steps.
  await expect(page.locator('button.copy-btn')).toHaveCount(2);
});

// Long-term bulletproof: the page must stay readable as content
// grows (longer phase names, more ledger rows, more feature cards).
// Parametrize over multiple mobile widths so narrow viewports catch
// regressions from future copy/inventory expansion.
const MOBILE_VIEWPORTS = [
  { label: 'iPhone SE', width: 320, height: 568 },
  { label: 'small Android', width: 360, height: 760 },
  { label: 'iPhone 15', width: 390, height: 844 },
  { label: 'iPhone Plus', width: 430, height: 932 },
];

for (const vp of MOBILE_VIEWPORTS) {
  test(`mobile (${vp.label} ${vp.width}×${vp.height}) has no horizontal overflow`, async ({ page }) => {
    await page.setViewportSize({ width: vp.width, height: vp.height });
    await page.goto('/');

    await expect(page.getByRole('heading', { name: 'One Go Binary. Same Hermes Brain.' })).toBeVisible();
    await expect(page.getByText('curl -fsSL https://gormes.ai/install.sh | sh')).toBeVisible();

    // The page itself must never generate a horizontal scrollbar. Long code
    // blocks get their own scroll inside .cmd via overflow-x: auto.
    const pageOverflow = await page.evaluate(() =>
      document.documentElement.scrollWidth > window.innerWidth,
    );
    expect(pageOverflow, `page body overflows at ${vp.width}px`).toBeFalsy();

    // Copy buttons stay visible + tappable on every supported viewport.
    const copyButtons = page.locator('button.copy-btn');
    await expect(copyButtons).toHaveCount(2);
    for (let i = 0; i < 2; i++) {
      const btn = copyButtons.nth(i);
      await expect(btn).toBeVisible();
      const box = await btn.boundingBox();
      expect(box, `copy button ${i} has no bounding box`).not.toBeNull();
      // iOS HIG minimum touch target is 44×44; we pass with 32 min-height +
      // padding, but enforce at least 28×28 so future CSS tweaks can't
      // silently shrink the button below usability.
      expect(box.height, `copy button ${i} too short at ${vp.width}px`).toBeGreaterThanOrEqual(28);
      expect(box.width, `copy button ${i} too narrow at ${vp.width}px`).toBeGreaterThanOrEqual(28);
    }

    // The roadmap has 5 phase groups with expanded sub-items. No phase
    // card or roadmap item should overflow its container on any mobile
    // viewport — long sub-item labels (4.A Provider adapters has ~100
    // chars, Phase 5 collapsed row has ~200 chars) must wrap cleanly.
    const overflowingNodes = await page.evaluate(() => {
      const nodes = Array.from(
        document.querySelectorAll('.roadmap-phase, .roadmap-item, .roadmap-label'),
      );
      return nodes
        .filter((n) => n.scrollWidth > n.clientWidth + 1)
        .map((n) => `${n.className}: ${n.textContent.trim().slice(0, 60)}`);
    });
    expect(overflowingNodes, 'roadmap nodes overflow their container').toHaveLength(0);

    // All five phase groups must be visible.
    await expect(page.locator('.roadmap-phase')).toHaveCount(5);
  });
}
