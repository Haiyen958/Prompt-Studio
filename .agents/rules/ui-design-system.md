# UI Design System Rules (Mandatory for All Frontend Changes)

Every HTML, CSS, or JavaScript UI modification in this workspace MUST follow `CSS_DESIGN_SYSTEM.md` and pass these guardrails:

1. **Use Existing Design Tokens (`web/css/01-tokens.css`):** Never hardcode colors (`#...`, `rgb(...)`), arbitrary pixel spacings, font sizes, radii, shadows, or z-index values. Use `--color-*`, `--space-1`..`--space-16`, `--text-*`, `--radius-*`, `--shadow-*`, and `--z-*`.
2. **Use Layout Primitives First (`web/css/04-layout.css`):** Compose layouts with `.container`, `.page-layout`, `.sidebar-layout`, `.grid` (`.col-span-*`, `.grid-cols-*`, `.grid-auto-fit`), `.stack`, and `.cluster` instead of ad-hoc margins or fixed pixel widths.
3. **Reuse BEM Components (`web/css/05-components.css`):** Check existing components (`.btn`, `.field`/`.input`/`.select`/`.textarea`, `.card`/`.panel`, `.modal`/`.dropdown`, `.tabs`/`.segmented`/`.nav-list`, `.badge`/`.chip`/`.alert`, `.table`, `.toolbar`, `.popover`/`[data-tooltip]`, `.spinner`/`.skeleton`/`.empty-state`) and `/showcase` before adding any new CSS selector.
4. **Zero Inline Styles & Zero Frameworks:** Never write `style="..."` in HTML/JS templates or mutate `el.style.*` in JS when a semantic state class (`.is-active`, `.is-open`, `.is-hidden`, `.status-text--*`) applies. Never introduce Tailwind CSS, Bootstrap, or CSS-in-JS.
5. **Cascade Hygiene:** Place every rule in its proper `@layer` (`tokens`, `reset`, `base`, `layout`, `components`, `pages`, `themes`, `utilities`). Never style `#id` selectors, never nest deeper than 2 levels, avoid `!important`, and never mutate global component styles from page-specific CSS.
6. **Mobile-First & Accessibility:** Use only documented `min-width` breakpoints (`640px`, `768px`, `1024px`, `1280px`), prevent horizontal overflow (`minmax(0, 1fr)`, `.table-wrap`), verify both Dark and Light themes (`data-theme`), and preserve `:focus-visible` rings and `prefers-reduced-motion`.
