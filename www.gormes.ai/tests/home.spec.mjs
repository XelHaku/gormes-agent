import { test, expect } from '@playwright/test';

test('homepage renders the redesigned landing', async ({ page }) => {
  await page.goto('/');

  await expect(page).toHaveTitle('Gormes — One Go Binary. No Python. No Drift.');
  await expect(page.getByRole('heading', { name: 'One Go Binary. No Python. No Drift.' })).toBeVisible();
  await expect(page.getByText('Gormes is a Go-native runtime for AI agents.')).toBeVisible();
  await expect(page.getByText('Built to solve the operations problem')).toBeVisible();
  await expect(page.getByText('One static binary. No virtualenvs. No dependency hell.')).toBeVisible();
  await expect(page.getByText('Early-stage, reliability-first runtime.')).toBeVisible();
  await expect(page.getByText('Built for developers who care about reliability over polish.')).toBeVisible();
  await expect(page.locator('.topnav a')).toHaveText(['Install', 'Roadmap', 'GitHub']);
  await expect(page.locator('.hero-image')).toHaveCount(0);
  await expect(page.locator('img[src="/static/go-gopher-bear-lowpoly.png"]')).toHaveCount(0);
  await expect(page.locator('.hero-ctas .btn-primary')).toHaveText('Install');
  await expect(page.locator('.hero-ctas .btn-secondary')).toHaveText('View Source');
  await expect(page.getByRole('heading', { name: 'Why Hermes breaks in production — and how Gormes fixes it.' })).toBeVisible();
  await expect(page.getByText('Hermes breaks in production because:')).toBeVisible();
  await expect(page.getByText('environments drift')).toBeVisible();
  await expect(page.getByText('installs fail')).toBeVisible();
  await expect(page.getByText('agents crash mid-run')).toBeVisible();
  await expect(page.getByText('streams drop and lose work')).toBeVisible();
  await expect(page.getByRole('heading', { name: "What works today, and what's still being wired up." })).toBeVisible();
  await expect(page.getByText('Current focus')).toBeVisible();
  await expect(page.getByText('Gateway stability')).toBeVisible();
  await expect(page.getByText('Memory system')).toBeVisible();
  await expect(page.getByText('Next milestone')).toBeVisible();
  await expect(page.getByText('Full Go-native runtime, no Hermes')).toBeVisible();
  await expect(page.getByText('curl -fsSL https://gormes.ai/install.sh | sh')).toBeVisible();
  await expect(page.getByText('irm https://gormes.ai/install.ps1 | iex')).toBeVisible();
  await expect(page.getByText('Source-backed for now')).toBeVisible();
  await expect(page.getByText('Read the installer source →')).toBeVisible();
  await expect(page.getByText('Requires Hermes backend at localhost:8642.')).toHaveCount(0);
  await expect(page.getByText('Run Hermes Through a Go Operator Console.')).toHaveCount(0);
  await expect(page.getByText('Deeper reference material lives at')).toHaveCount(0);
  await expect(page.locator('link[href="/static/site.css"]')).toHaveCount(1);
  // Copy buttons require a tiny inline clipboard script — bounded to install steps.
  // Three steps now: Unix install, Windows install, run.
  await expect(page.locator('button.copy-btn')).toHaveCount(3);
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

    await expect(page.getByRole('heading', { name: 'One Go Binary. No Python. No Drift.' })).toBeVisible();
    await expect(page.getByText('curl -fsSL https://gormes.ai/install.sh | sh')).toBeVisible();

    const heroLayout = await page.evaluate(() => {
      const content = document.querySelector('.hero-content')?.getBoundingClientRect();
      const title = document.querySelector('.hero-title')?.getBoundingClientRect();
      return {
        contentWidth: content?.width ?? 0,
        titleWidth: title?.width ?? 0,
      };
    });
    expect(heroLayout.contentWidth, `hero content collapsed at ${vp.width}px`).toBeGreaterThan(vp.width * 0.6);
    expect(heroLayout.titleWidth, `hero title too wide at ${vp.width}px`).toBeLessThanOrEqual(vp.width);

    // The page itself must never generate a horizontal scrollbar. Long code
    // blocks get their own scroll inside .cmd via overflow-x: auto.
    const pageOverflow = await page.evaluate(() =>
      document.documentElement.scrollWidth > window.innerWidth,
    );
    expect(pageOverflow, `page body overflows at ${vp.width}px`).toBeFalsy();

    // Copy buttons stay visible + tappable on every supported viewport.
    // Three install steps: Unix, Windows, run.
    const copyButtons = page.locator('button.copy-btn');
    await expect(copyButtons).toHaveCount(3);
    for (let i = 0; i < 3; i++) {
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

    // The roadmap has 7 phase groups under a single disclosure. No phase
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

    // All seven phase groups are present in the generated roadmap, but the
    // full checklist starts collapsed so mobile users get a clear entry point.
    await expect(page.locator('.roadmap-phase')).toHaveCount(7);
    await expect(page.locator('.roadmap-details')).not.toHaveAttribute('open', '');
  });
}
