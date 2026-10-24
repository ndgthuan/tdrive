# TDrive visual design research

Scope: how well-regarded products design dark and light UI, with exact values where a source publishes them. Goal: a refined, low-chroma, non-neon look for a Windows dark-first file manager (Telegram-backed storage, Vietnamese users).

Legend used below:
- **[OFF]** official product, design-system, or developer documentation fetched and read.
- **[3P]** third-party extraction or community reverse-engineering. Treat as approximate.
- **[NS]** not sourced. Looked for it, could not verify. Do not copy.

Method note: many official pages are JavaScript-rendered and returned only titles. Where a `.md` variant or a stylesheet was reachable it was used. Values are reported as published; nothing was guessed.

---

## 1. Linear

- **Refresh write-up [OFF]**: https://linear.app/now/how-we-redesigned-the-linear-ui
- Theme generation moved to LCH (their words: "perpetually uniform"). Light and dark themes are now generated from three inputs (base, accent, contrast) instead of 98 variables. Surfaces (background, panels, dialogs, modals) are derived systematically from those inputs.
- Contrast is an explicit theme variable, which they use to offer high-contrast themes.
- Stated rule: limit blue "chrome" in the calculations for a "more neutral and timeless appearance". This is the closest official statement to "few accents".
- Font: Inter for UI text, Inter Display for headings [OFF]. Sizes and weights not published.
- Electron app; they used Apple HIG conventions to approximate native feel [OFF].
- Community extraction [3P], unverified, from the sites that claim to reverse-engineer Linear: base near `#08090a`, panels `#0f1011`, elevated `#191a1b`, accent `#5e6ad2` (hover `#7170ff`), text `#f7f8f8`, borders rgba(255,255,255,0.05 to 0.08). Sources disagree on the canvas (`#08090a` vs `#010102`).
- Radii, spacing, motion durations: [NS].
- **Takeaway**: one neutral ladder built from a single hue at low chroma, one accent used for brand and focus only, and contrast as an explicit setting.

## 2. Vercel Geist

- Geist docs: https://vercel.com/geist/colors and https://vercel.com/geist/materials.md [OFF]
- Geist gray scale is named `gray-100` to `gray-1000`, plus `background-100` and `background-200`. Borders default to gray-400, hover gray-500, active gray-600. Text secondary gray-900, primary gray-1000 [OFF].
- Stated principles [OFF]: use gray for text, icons, fills, borders and dividers by default. "Draw each border once", avoid overlapping alpha borders. Keep material shadows translucent to avoid a gray halo.
- Materials radii [OFF]: small 6px, medium 12px, menu 12px, modal 12px, fullscreen 16px, tooltip 6px. Shadows are described only relatively (tooltip lightest, modal and fullscreen strongest).
- Accent scales exist (blue, red, amber, green, teal, purple, pink, 10 steps each) [OFF] but hex values are not published in the docs.
- Vercel brand stylesheet [OFF, `https://vercel.com/geist/vercel-brand.css`]. This is the Vercel brand/report token set, so it may not equal the Geist product palette. Values are OKLCH:
  - Dark background `oklch(0 0 0)`, background-200 `oklch(0.027 0 0)`. Chroma is 0 on every gray, so the neutrals are pure, not tinted.
  - Dark grays (gray-100 to gray-1000): `oklch(0.218)`, `0.239`, `0.281`, `0.301`, `0.39`, `0.623`, `0.65`, `0.59`, `0.706`, `0.946` (all chroma 0).
  - Dark borders are white alpha: `oklch(1 0 0 / 0.24)` on contrast bands; gray-alpha steps 0.07 to 0.91.
  - Accent blue-700 dark `oklch(57.61% 0.2321 258.23)`. This is saturated, which is the look to avoid for TDrive.
  - Radius: small 6px, default 8px. Control height 36px (touch 44px).
  - Type: Geist Sans and Geist Mono. Sizes: display 48px, page-title 40px, title 32px, section 24px, subsection 20px, lede 18px, body 16px, compact 14px, label 13px, metadata 13px. Weights 400, 450 (heading), 500, 600. Body line-height 24px, compact 20px.
  - Spacing: 4px base with steps 4, 8, 12, 16, 20, 24, 32, 40, 48, 64.
  - Transitions: hard-coded 150ms [OFF].
- Principles from the Vercel design guide [OFF, https://vercel.com/design.dark.md, partially read]: design in monochrome, use color only when it adds meaning and pair it with a non-color cue. Build hierarchy with typography before surfaces or color. Avoid gradients, glows and textures. Keep motion minimal and respect reduced motion.
- **Takeaway**: neutrals with zero chroma are a legitimate choice; hierarchy by type first; hairline borders drawn once.

## 3. Apple (macOS / iOS HIG, Finder)

- Dark Mode [OFF]: https://developer.apple.com/design/human-interface-guidelines/dark-mode.md
  - Dark palette uses "dimmer background colors and brighter foreground colors". Not simple inversions.
  - Base vs elevated background: base is dimmer and recedes, elevated is brighter and advances. Depth is shown by brightness, not shadow.
  - Contrast floor: at least 4.5:1, and 7:1 for custom foreground/background pairs, especially small text.
  - Advice to avoid app-specific appearance settings; follow the system.
- Color spec [OFF]: https://developer.apple.com/design/human-interface-guidelines/color.md. Published system values, default dark:
  - systemBlue `#0091FF`. Gray scale: systemGray `#8E8E93`, systemGray2 `#636366`, systemGray3 `#48484A`, systemGray4 `#3A3A3C`, systemGray5 `#2C2C2E`, systemGray6 `#1C1C1E`.
  - System background colors (from a third-party cheat sheet that matches the iOS system palette, [3P] sarunw.com/posts/dark-color-cheat-sheet): systemBackground `#000000`, secondary `#1C1C1E`, tertiary `#2C2C2E`, opaque separator `#38383A`. Labels: label `#FFFFFF`, secondary label about 60% white, tertiary about 30% white.
  - Apple's own guidance: use vibrant/semantic colors rather than hard-coded values; "avoid hard-coding system color values".
  - Accent guidance [OFF]: apply color sparingly; do not add color to many controls; avoid similar colors in control labels over colorful backgrounds.
- Materials [OFF]: https://developer.apple.com/design/human-interface-guidelines/materials.md
  - Use standard materials for content-layer elements; "Don't use Liquid Glass in the content layer." Liquid Glass is for controls and navigation that float above content. Use it sparingly.
  - Thicker materials give more contrast; thinner ones keep context. Vibrant label colors on top of materials.
- Typography [OFF]: https://developer.apple.com/design/human-interface-guidelines/typography.md
  - iOS default styles (size/leading, weight): Large Title 34/41 Regular; Title 1 28/34 Regular; Headline 17/22 Semibold; Body 17/22 Regular; Caption 2 11/13 Regular.
  - Default and minimum sizes: iOS 17/11 pt, macOS 13/10 pt.
  - System font is SF Pro on iOS and macOS; SF Compact on watchOS.
- Finder specifics: [NS] (no fetched source for Finder row height, selection color or motion).
- Motion: [NS] in fetched HIG pages.
- **Takeaway**: depth by brightness, system grays as a ladder, accent only where it carries meaning, glass only on floating chrome.

## 4. Google Drive / Material 3

- Color system [OFF]: https://m3.material.io/styles/color/system/overview (page fetched was title-only; not reached).
- Tone-based surface roles [OFF, partial]: https://material.io/blog/tone-based-surface-color-m3. Elevation maps to surface containers: +1 is Surface Container Low, +2 Surface Container, +3 Surface Container High, +4/+5 deprecated in favour of Surface Container Highest.
- In dark theme, higher tonal elevation is lighter [OFF].
- State-layer opacities [3P, Flutter generated defaults, token v0_137]: hover 0.08, focus 0.12, pressed 0.12, dragged 0.16. Disabled content 0.38 is a common convention; treat all as secondary-sourced.
- Corner radius scale [3P, Flutter Material 3 package]: none 0, extra-small 4, small 8, medium 12, large 16, extra-large 28, full. The Material Web and Wear sources use different numbers, so this is not settled.
- Dark surface hex values (surface, surface-container): [NS]. Searches did not return them. Do not use values from memory.
- Google Drive's own UI tokens: [NS]. No Drive design source was reached.
- **Takeaway**: M3's useful idea is the surface-container ladder and state layers as opacity over a base color, not the specific hex values.

## 5. Dropbox (Dash, 2023-25)

- Dropbox's own design blog for Dash was not found. Brand guidelines exist [OFF, https://www.dropbox.com/branding] but were not extracted.
- 2025 brand-site refresh covers color, typography, illustration and voice [OFF via designcompass.org article, https://designcompass.org/en/2025/01/21/dropbox-brand-website-renewal/].
- Dash motion: a third-party agency case study describes motion as a storytelling device [3P]. Not usable as UI motion guidance.
- Hex, type and motion values for Dash UI: [NS].
- Third-party style catalogs describe a vivid royal blue primary and a cream canvas [3P, unreliable].
- **Takeaway**: no reliable Dropbox UI data. Do not cite Dropbox as a model for the dark UI.

## 6. Notion

- Official dark-mode help page returned 404 [NS].
- Third-party extraction [3P, matthiasfrank.de/en/notion-colors/ and notioneerseunew.super.site]: default dark background `#191919`, default text `#D4D4D4`, gray text `#9B9B9B` on `#252525`. Other tints listed (brown, orange, yellow, green, blue, purple, pink, red) at about 60 to 75% saturation.
- Conflicting [3P, gridfiti.com]: main window `#2F3438`, sidebar `#373C3F`. Probably an older build.
- Obsidian "Notion Style" theme [3P] dims text by white alpha 0.81, 0.50, 0.35.
- **Takeaway**: #191919 canvas with warm-neutral text and low-saturation tag colors is the commonly observed pattern. Verify before relying on it.

## 7. Raycast

- Developer docs [OFF, https://developers.raycast.com/api-reference/user-interface/colors]: named colors Blue, Green, Magenta, Orange, Purple, Red, Yellow, PrimaryText, SecondaryText. Dynamic colors are "adjusted to achieve high contrast" with the Raycast UI, and can be turned off.
- Third-party DESIGN.md extractions [3P]: canvas about `#07080a` (almost-black, faintly blue), brand red `#FF6363` used sparingly in hero imagery, blue `#56c2ff`, green `#5fd9a0`. Depth from tonal grey layers plus subtle inset highlights. Borders white alpha 0.06 to 0.10. Typeface Inter, mono for shortcuts.
- Official values for surfaces: [NS].
- **Takeaway**: depth from layered neutrals and thin translucent borders; saturated accents are kept for specific moments.

## 8. Arc / The Browser Company (and Dia)

- Official design details: [NS]. The arc.net page reached was marketing copy for Dia, with no colors, type or motion values.
- Known from the page [OFF]: vertical "sidebar tabs"; sidebar toggled with Cmd/Ctrl+S; "Spaces" renamed "Profiles"; "Folders" renamed "Tab Groups".
- **Takeaway**: Arc's sidebar-first layout is a structural reference only. No sourced colour or type data.

## 9. Things 3 (Cultured Code)

- Official blog on dark mode [OFF, https://culturedcode.com/things/blog/2018/09/night-and-day/ was reached but had no values]. Stated: a "finely tuned" dark mode for macOS Mojave; dark appearance can be selected independently of the system.
- Things 3.8 for iOS added dark appearances and a third option named Black for OLED screens [3P, MacStories and The Sweet Setup reviews].
- Dark theme reviewers describe as a gray background with blue as the accent [3P]. Exact hex: [NS].
- Switching by screen brightness threshold is a reported behavior [3P].
- **Takeaway**: a true-black variant is offered as a separate choice, not forced. Accent blue stays muted and rare.

## 10. Stripe (dashboard and embedded components)

- Official dark-mode appearance tokens [OFF, https://docs.stripe.com/connect/embedded-appearance-support-dark-mode]:
  - Background `#14171D`, form background `#14171D`, offset background `#1B1E25`.
  - Text `#C9CED8`, secondary text `#8C99AD`.
  - Border `#2B3039`, secondary button background `#2B3039`.
  - Primary `#0085FF` (a saturated blue; avoid for TDrive).
  - Danger `#F23154`. Badge neutral background `#1B1E25`, border `#2B3039`, text `#8C99AD`.
  - Overlay backdrop `rgba(0,0,0,0.5)`.
- Stripe's docs note each site should customize its dark theme and that a "demo dark mode" preset is a starting point.
- Stripe's own dashboard typography, radii and motion: [NS] (the Sail design-system search returned nothing official).
- **Takeaway**: the neutral set (`#14171D` bg, `#1B1E25` raised, `#2B3039` border, `#C9CED8` text, `#8C99AD` secondary) is a good low-chroma reference. The primary blue is the part to replace with a muted accent.

---

## Cross-cutting principles (sourced)

1. **Derive neutrals from one hue at very low chroma, or use pure neutrals.** Linear generates themes from base/accent/contrast in LCH [OFF, linear.app/now/how-we-redesigned-the-linear-ui]. Vercel's brand grays are chroma 0 [OFF, vercel.com/geist/vercel-brand.css]. Either works; tinted near-black (Stripe `#14171D`) reads as premium.
2. **Accent is rare and carries meaning.** Apple: "apply color sparingly", and avoid color on many controls at once [OFF, HIG color]. Vercel: "design in monochrome" [OFF, design.dark.md]. Linear limits blue chrome [OFF].
3. **Hierarchy comes from typography, then surface, then color.** Vercel: "build hierarchy with typography before surfaces or color" [OFF, design.dark.md]. Apple defines label, secondary, tertiary labels as alpha steps of one color [OFF, HIG Dark Mode].
4. **Depth comes from brightness steps, not shadows.** Apple base vs elevated backgrounds [OFF, HIG dark-mode.md]. M3 surface-container ladder [OFF, m3 tone-based blog]. Vercel: avoid ornamental shadows [OFF].
5. **Borders are hairlines drawn once, often translucent.** Vercel: "draw each border once", use gray-alpha for edges that blend [OFF, geist/colors]. Vercel brand: white alpha 0.24 borders on contrast bands [OFF, vercel-brand.css].
6. **Dark mode is not an inversion.** Dimmer backgrounds, brighter foregrounds, some colors left unchanged [OFF, HIG dark-mode.md].
7. **Contrast floor is 4.5:1 for text, and 7:1 for custom pairs** [OFF, HIG dark-mode.md]. Use the system's contrast setting where possible (Linear's contrast variable [OFF]).
8. **Restrain motion and effects.** Vercel: keep motion minimal and respect reduced motion; no gradients, glows, or textures [OFF, design.dark.md]. Material and glass only on floating chrome, not in the content layer [OFF, HIG materials.md].
9. **Type scale is small and dense for tools.** Apple iOS body 17/22, Vercel body 16 and compact 14, label 13 [OFF]. Desktop file managers sit at 13 to 14 body.

## Five things that make a UI look neon or AI-generated

1. **Saturated glow accents on dark (electric blue, violet, cyan).** Pro alternative: one muted accent, desaturated, used only for selection and focus. Example: Linear's single accent family; Apple's "sparingly" rule.
2. **Gradient borders, gradient buttons, gradient text.** Pro alternative: flat fills and a 1px translucent hairline. Vercel avoids gradients and glows [OFF].
3. **Glassmorphism and blur everywhere.** Pro alternative: opaque surfaces for content; translucency only on floating menus. Apple says glass should not sit in the content layer [OFF, materials.md].
4. **Pure black canvas with neon text, bloom shadows, or colored shadows.** Pro alternative: tinted near-black canvas (about `#0F1113`), off-white text (about `#E8EAED`), neutral shadows at low alpha. Apple: dark palette is dimmer background, not a black void.
5. **Mixed-weight oversized type, large radii on everything, and heavy drop-shadow cards.** Pro alternative: 6 to 8px radii on controls and cards, flat surfaces separated by a 1 to 2 luminance step, weights 400 to 600 only, one clear type ramp.

---

## Recommended palette and rules for TDrive

Design choices below are our recommendation, derived from the sourced principles above. Only the Apple contrast floor, Stripe neutral reference and Vercel token structure are published; the exact hexes are a proposal. All contrast ratios were computed with the WCAG 2.x relative-luminance formula.

### Dark palette (default)

| Token | Hex | Use |
|---|---|---|
| canvas | #0F1113 | window background |
| sidebar | #13161A | navigation rail |
| surface | #181B1F | file list, cards |
| surface-raised | #1E2227 | menus, dialogs, popovers |
| surface-hover | #242930 | row hover |
| border | #262B31 | hairline dividers (decorative) |
| border-input | #6E7680 | input and checkbox outlines |
| text-primary | #E8EAED | names, headings |
| text-secondary | #A0A7B1 | metadata, sizes |
| text-tertiary | #7D8590 | placeholders, timestamps |
| accent | #7F9CC4 | focus ring, one primary action, link text |
| accent-soft | #26364D | selected row background |

Accent is a desaturated slate blue (clearly lower chroma than the Vercel, Stripe or Apple blues quoted above). Use it for at most one primary button per view and for selection; never for large fills.

### Light palette

| Token | Hex | Use |
|---|---|---|
| canvas | #F6F7F8 | window background |
| sidebar | #EFF1F3 | navigation rail |
| surface | #FFFFFF | file list, cards |
| surface-sunken | #EDEFF2 | input wells, code blocks |
| surface-hover | #E9ECEF | row hover |
| border | #DDE1E5 | hairline dividers (decorative) |
| border-input | #7D8590 | input outlines (3:1 on white is about 3.7:1) |
| text-primary | #15181C | names, headings |
| text-secondary | #5B636D | metadata |
| text-tertiary | #666E78 | placeholders, timestamps |
| accent | #3D5A80 | focus ring, primary action, link text |
| accent-soft | #DCE6F2 | selected row background |

### Contrast ratios (WCAG 2.x, computed)

Dark mode:

| Foreground on background | Ratio | Passes |
|---|---|---|
| text-primary #E8EAED on canvas #0F1113 | 15.70 | AA and AAA |
| text-primary on surface #181B1F | 14.34 | AA and AAA |
| text-primary on surface-raised #1E2227 | 13.27 | AA and AAA |
| text-primary on surface-hover #242930 | 12.14 | AA and AAA |
| text-primary on accent-soft #26364D | 10.15 | AA and AAA |
| text-secondary #A0A7B1 on canvas | 7.80 | AA and AAA |
| text-secondary on surface | 7.12 | AA and AAA |
| text-secondary on surface-raised | 6.59 | AA and AAA |
| text-tertiary #7D8590 on canvas | 5.07 | AA |
| text-tertiary on surface | 4.63 | AA (barely) |
| text-tertiary on surface-raised | 4.29 | fails AA for body text; use only on canvas or surface |
| accent #8FA8CF text on surface | 7.13 | AA and AAA |
| input border #6E7680 on surface | 3.76 | passes 3:1 non-text |
| border hairline #262B31 | not applicable | decorative only |

Light mode:

| Foreground on background | Ratio | Passes |
|---|---|---|
| text-primary #15181C on canvas #F6F7F8 | 16.60 | AA and AAA |
| text-primary on surface #FFFFFF | 17.81 | AA and AAA |
| text-primary on surface-hover #E9ECEF | 15.02 | AA and AAA |
| text-primary on accent-soft #E3EAF3 | 14.69 | AA and AAA |
| text-secondary #5B636D on surface | 6.09 | AA and AAA |
| text-secondary on surface-hover #E9ECEF | 5.13 | AA |
| text-secondary on surface-sunken #EDEFF2 | 5.28 | AA |
| text-tertiary #666E78 on surface | 5.16 | AA |
| text-tertiary on canvas | 4.81 | AA |
| text-tertiary on surface-sunken | 4.48 | just below AA; avoid on sunken wells |
| accent #3D5A80 on surface | 7.06 | AA and AAA |
| accent on accent-soft #DCE6F2 | 5.60 | AA |
| white text on accent fill #3D5A80 | 7.06 | AA and AAA |
| input border #7D8590 on surface | 3.73 | passes 3:1 non-text |
| input border #7D8590 on canvas | 3.48 | passes 3:1 non-text |

Rule: text-primary and text-secondary satisfy AA on every surface. text-tertiary is for non-essential metadata only.

### Radii

- 6px: buttons, inputs, list rows, chips, tooltips (Geist small, 6px [OFF]).
- 8px: cards, menus, popovers (Geist default 8px [OFF]).
- 12px: dialogs and sheets only (Geist modal and menu 12px [OFF]).
- Avoid anything larger on desktop chrome.

### Type

- Family: Segoe UI Variable (system on Windows 11) as default, so no bundled font cost. Confirm Vietnamese diacritics (ẩ, ệ, ữ, etc.) render in the chosen face before shipping. If bundling, Geist or Inter are both sourced [OFF] and both support Latin Extended.
- Mono for paths, sizes and hashes only: Geist Mono or Cascadia Mono.
- Scale (px / line-height): 24/32 section title; 20/28 subsection; 16/24 body; 14/20 compact body (default for file rows); 13/18 labels and metadata.
- Weights: 400 body, 500 labels and row names, 600 headings only. No 700.
- Letter-spacing: 0 for body; -0.01em for headings above 20px.

### Spacing

- 4px base. Steps: 4, 8, 12, 16, 24, 32 (Geist uses 4 to 64 in the same base [OFF]).
- Control height 32px on desktop (Geist 36px, touch 44px [OFF]); row height 36px for dense lists.
- Sidebar padding 12px; content gutter 16px.

### Elevation, borders, states

- Surfaces separated by a luminance step (canvas to surface to surface-raised), not by shadow [OFF, Apple base vs elevated].
- Hairline borders 1px, drawn once; use border only where luminance step is not enough.
- Floating menus: 1px border plus a neutral shadow of about 24px blur at 40% black alpha (our proposal; no published values).
- Hover: change surface token one step. Selection: accent-soft fill, text-primary, no glow.
- Focus: 2px accent ring at 2px offset. Never color-only (pair with a shape or underline).

### Motion

- Hover and state change: 120 to 150ms, ease-out (Vercel brand uses 150ms [OFF]).
- Panel open and close: 180 to 220ms, ease-out. No bounce or overshoot.
- Respect the system reduced-motion setting: disable transforms, keep opacity fades under 100ms.

### Non-negotiable rules

1. One accent, used on at most one primary action per view.
2. No gradients, glows, colored shadows, or glass in the content area.
3. Every text token must meet 4.5:1 on the surface it sits on; check tertiary text first when adding a surface.
4. Support the Windows light and dark setting and a manual override; do not add an app-specific appearance toggle as the only control.

---

## Products not sourced properly

- Google Drive: no design source reached; Material 3 hex values and state-layer tokens are secondary-sourced only.
- Dropbox Dash: no official design-blog post found; brand values unverified.
- Arc / Browser Company: only Dia marketing copy reached; no colour or type data.
- Things 3: exact dark hex values not published in the reached sources.
- Stripe dashboard (Sail): official dark appearance tokens found; dashboard type, radii and motion not found.
- Notion: official dark-mode page returned 404; values are third-party and conflicting.
- Linear: official post gives method but no hex values; hex values are community extractions.
- Apple Finder: macOS row, selection and motion specifics not reached; iOS system palette and HIG principles are sourced.

## Primary source list

- https://linear.app/now/how-we-redesigned-the-linear-ui
- https://vercel.com/geist/colors (and .md), https://vercel.com/geist/materials.md, https://vercel.com/geist/vercel-brand.css, https://vercel.com/design.dark.md
- https://developer.apple.com/design/human-interface-guidelines/dark-mode.md
- https://developer.apple.com/design/human-interface-guidelines/color.md
- https://developer.apple.com/design/human-interface-guidelines/materials.md
- https://developer.apple.com/design/human-interface-guidelines/typography.md
- https://m3.material.io/styles/color/system/overview (not reached; title only)
- https://material.io/blog/tone-based-surface-color-m3
- https://docs.stripe.com/connect/embedded-appearance-support-dark-mode
- https://developers.raycast.com/api-reference/user-interface/colors
- https://culturedcode.com/things/blog/2018/09/night-and-day/
- https://www.dropbox.com/branding
- https://arc.net/
