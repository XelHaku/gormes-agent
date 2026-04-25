# Gormes Landing Serious Hierarchy Design

**Status:** Approved inline 2026-04-24
**Owner:** xel

## Goal

Tighten `gormes.ai` from a good-but-generic landing page into a serious infrastructure-runtime page with decisive hierarchy on mobile and desktop.

## Locked Direction

- Do the visual reset in one cohesive pass.
- Trim primary nav to `Install`, `Roadmap`, and `GitHub`; move `Docs` and `Company` to secondary/footer locations.
- Use Fraunces for the hero headline only.
- Use DM Sans for body, nav, cards, roadmap, and footer.
- Use JetBrains Mono only for code blocks and command copy controls.
- Remove the gopher/bear illustration from the hero.
- Rebalance the hero as a single-column editorial block with tighter line length.
- Make `Install` the dominant CTA and `View Source` a smaller outline secondary action.
- Add expectation-setting copy below the hero: `Early-stage. Built for developers who care about reliability over polish.`
- Make feature cards more technical: tighter padding, stronger title contrast, thinner borders, sharper corners, no soft marketing glow.
- Add an explicit pain block before feature cards: `Hermes breaks in production because:` followed by short operational failure bullets.
- Rework install spacing into clear numbered steps with aligned labels, code blocks, and copy buttons.
- Promote the source-backed installer note so it reads as product truth, not a footnote.
- Add a roadmap summary before the full phase list:
  - `Current focus: Gateway stability; Memory system`
  - `Next milestone: Full Go-native runtime, no Hermes`
- Keep the full generated roadmap available, but collapse phase details on mobile to reduce overload.

## Out of Scope

- No new frontend framework or client-side navigation system.
- No hamburger menu.
- No changes to installer scripts.
- No change to roadmap data generation.
- No generated imagery or social-card redesign.

## Acceptance Criteria

- Rendered page uses the serious hero copy and no hero image.
- Primary nav contains only `Install`, `Roadmap`, and `GitHub`.
- CSS encodes the typography rule: Fraunces is only used by `.hero-title`; JetBrains Mono is limited to code/copy command surfaces.
- Feature section includes the operational pain block before cards.
- Install section contains three clear steps and source-backed copy.
- Roadmap includes the current-focus and next-milestone summary before phase groups.
- Mobile Playwright checks show no horizontal overflow and collapsed roadmap details below tablet width.
- `go test ./...` passes in `www.gormes.ai`.
- `npm run test:e2e` passes in `www.gormes.ai`.
