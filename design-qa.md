# Design QA: 发布单详情状态气泡同步

## Visual truth

- Source visual truth path: `/var/folders/q9/0ld1by9n6x5cp7p8szmgnpg00000gn/T/codex-clipboard-8fa5909f-a544-466e-b0c5-d2482d4326cf.png`
- Source pixels: 1663 × 925.
- Implementation URL: `http://127.0.0.1:5175/releases/qa-order-133`
- Implementation screenshot path: captured inline from the Codex in-app Browser; this browser surface does not expose a filesystem screenshot path.
- Implementation pixels: 1663 × 925 at a 1663 × 925 CSS viewport, device density 1.
- Density normalization: none; source and implementation were compared at equal pixel and CSS dimensions.
- State: authenticated isolated QA release order in `building`; CI stages include success and running, and execution units include running and pending.

## Full-view comparison evidence

- The source screenshot and rendered implementation were placed in one browser-rendered comparison board at equal 1663 × 925 frame sizes before judgment.
- The red-boxed source areas identify the legacy detail-page status surfaces that needed synchronization. The rendered page keeps the existing detail information architecture while applying the release-list semantic badge system to the overall progress meta, pipeline stages, and execution-unit states.
- The source capture excludes the left application sidebar while the live route includes it; this is an existing crop difference and does not affect the status-component comparison.
- The final implementation capture contains no fixture warning notice and no browser console errors or warnings.

## Focused status evidence

- Overall status meta: 145 × 26 px, 8 px radius, 3 × 9 px padding, 6 px icon gap, running background `rgb(239, 246, 255)`, border `rgb(147, 197, 253)`, foreground `rgb(29, 78, 216)`.
- Stage success badges: 70 × 26 px, 8 px radius, success background `rgb(236, 253, 243)`, border `rgb(134, 239, 172)`, foreground `rgb(21, 128, 61)`.
- Stage and execution running badges: 82 × 26 px with the same running token and a semantic loading icon.
- Pending execution badge: 82 × 26 px, pending background `rgb(255, 247, 237)`, border `rgb(253, 186, 116)`, foreground `rgb(180, 83, 9)`.
- Overall progress surface: 533 × 181 px, 16 px radius, flat running background and border. No decorative gradient remains.

## Required fidelity surfaces

- Fonts and typography: existing application font stack and hierarchy are preserved; status copy uses the shared 12 px/700 badge treatment with stable single-line truncation.
- Spacing and layout rhythm: all target status badges use 26 px minimum height, 8 px radius, 3 × 9 px padding, and 6 px icon gap. Existing stage and execution layouts remain aligned.
- Colors and visual tokens: success, running, failed, pending, and neutral states now use the same flat semantic token family as the release list.
- Image quality and asset fidelity: no raster or generated assets were introduced. Status marks use the project's existing Ant Design Vue icon set.
- Copy and content: existing Chinese status labels are preserved; icons reinforce rather than replace text.
- Responsiveness: the full detail frame remained readable at the 1663 × 925 target viewport; badge text did not wrap or collide.
- Accessibility and behavior: labels remain textual, icons are decorative, and running icons retain animation. The changed status elements are intentionally non-interactive.

## Findings

- No actionable P0, P1, or P2 mismatch remains for the requested status surfaces.
- P3: isolated fixture timestamps display as 1970 because the fixture uses millisecond-like integers against a seconds-based formatter. This is fixture-only and unrelated to the production status styling.

## Comparison history

- Pass 1: source and implementation were compared in the same equal-size browser board. No actionable P0/P1/P2 visual finding was identified, so no design-fix iteration was required.
- Evidence cleanup: a missing-template warning from the isolated fixture was removed by adding the corresponding fixture record; the final capture has no notice. This was test-data normalization, not a UI change.

## Functional QA

- Local isolated login and detail loading succeeded.
- Verified visible states: overall building, stage success, stage running, execution running, and execution pending.
- Browser console warnings/errors: none.
- Targeted frontend tests: 38/38 passed.
- Go tests: `go test ./...` passed.
- Production frontend bundle: `npm run build` passed.
- Existing unrelated frontend static-test drift: 12 of 238 full-suite assertions remain failing; none is introduced by the status-badge or v1.3.3 changes.

## Implementation checklist

- [x] Synchronize detail status badge geometry and semantic colors with the release list.
- [x] Add semantic icons to all detail-page status badges.
- [x] Flatten the overall progress status surface while preserving its layout.
- [x] Verify the v1.3.3 sidebar version display.
- [x] Build and test the release bundle.

final result: passed
