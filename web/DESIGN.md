# Gogs web design notes

A running record of design decisions made for the web frontend. Add an entry when a pattern is used in two places, or when a question caused a redo. Don't write aspirationally. Only capture what's already true in the code.

## Typography

Self-hosted via `@fontsource-variable`:

- **Sans**: Geist Variable, with PingFang SC and Microsoft YaHei as CJK fallbacks. Used for body text, headings, and UI chrome.
- **Mono**: Geist Mono Variable, with the same CJK fallbacks. Used for code-shaped content (SHA, branch name, file path, shell command, terminal-style surfaces).

The browser does per-glyph fallback via the font-family stack. Latin characters render in Geist (the designed personality). CJK characters render in the next available system font (PingFang SC, Microsoft YaHei). The result: English looks distinctively Gogs, other scripts look clean and native.

Use mono only for content that **is** code, not for UI chrome (navbars, buttons, labels). Mono CJK fallbacks aren't truly monospace (CJK glyphs are wider than Latin), which is fine when the content is genuinely code, but reads as broken alignment if used decoratively for chrome.

Don't mix sans and mono within the same UI surface for arbitrary reasons. If a component is showing code, all of it goes mono.

## Color hierarchy

Palettes are derived from [Happy Hues](https://www.happyhues.co): palette 6 for light, palette 13 for dark. Dark mode is opt-in via the `.dark` class on `:root` (see `lib/theme.ts`), not media-query driven, so the user's stored preference always wins. The `@custom-variant dark` rule in `index.css` lets utilities like `dark:...` target the same class.

Use these tokens. Don't introduce raw hex values in components.

**Surfaces and content**

- `--color-background`: page background. Body uses this by default.
- `--color-foreground`: primary content. Headings, active states, the main label of any item, body text on `--color-background`.
- `--color-muted-foreground`: secondary content. Metadata, helper text, terminal prompt characters, footer chrome, inactive items in a toggle group.
- `--color-surface`: subtle raised surface. Used for hover backgrounds (`hover:bg-(--color-surface)` on links, buttons, menu rows) and for the muted fill of the faux-terminal frame.
- `--color-card` / `--color-card-foreground`: card surface and its body text. Not currently used in components, but available for content cards.
- `--color-popover` / `--color-popover-foreground`: popover surface and body. Used by the Radix popover primitive.

**Accents and state**

- `--color-primary` / `--color-primary-foreground`: brand purple. Reserved for genuine brand emphasis. Don't use it to mean "primary action" between two peer links (see the peer-item rule below).
- `--color-secondary` / `--color-secondary-foreground`: muted brand support. Available for chips, tags, low-emphasis fills.
- `--color-destructive` / `--color-destructive-foreground`: error and danger. The 404 page uses `text-(--color-destructive)` on the `fatal:` token, always paired with the word itself (color is never the sole signal).
- `--color-ring`: keyboard focus ring color. Don't override per-component. If a default ring looks wrong, fix it at the token level.

**Structure**

- `--color-border`: borders on dividers, popovers, the terminal frame.
- `--color-input`: input field borders. Not currently used; reserved for forms.

**Peer-item rule**

Don't use foreground vs muted-foreground to imply "primary action" vs "secondary action" between two peer items (e.g. Sign in vs Register). Peer items get the same color. Differentiation comes from positioning, weight, or affordance, not arbitrary contrast. Active vs inactive _states_ of the same control (e.g. the selected theme tile in `SettingsMenu`) are a different case and may use the foreground/muted-foreground split to communicate selection.

**Ad-hoc colors**

The traffic-light cluster in the faux-terminal frame uses one ad-hoc value: the amber dot falls back to `oklch(0.795 0.184 86.047)` via `bg-(--color-warning,...)`. There is no `--color-warning` token defined, so the fallback always wins. This is deliberate. Promoting it to a real token would invite reuse, and warning is not a system-wide concern in the current UI. Leave it inline until a second site needs warning semantics, then define the token in both light and dark palettes.

## Surface chrome

The 404 page wraps its content in a faux-terminal frame (rounded border, traffic-light dots, monospace body). Reuse the same frame for any page that represents a Git/CLI state: error pages, command-output stubs, raw diff fallbacks. Don't reuse it for normal content pages.

Strings rendered inside a terminal frame stay in English across all locales, regardless of the active UI language. Real CLI output (`git`, `ls`, `cat`, etc.) doesn't localize. Faux-CLI that does loses authenticity and reads as a translated error page in a costume. Translate the surrounding prose (headings, descriptions, CTAs), but leave command names, prompts, error tokens like `fatal:`, and command output strings untouched.

## File naming

Two conventions coexist in `web/src/`:

- **shadcn primitives** in `components/ui/` use **lowercase** filenames (`popover.tsx`). This matches the `shadcn` CLI output and lets dropped-in components stay byte-identical to upstream.
- **App components** anywhere else use **PascalCase** matching the exported component (`Footer.tsx`, `SettingsMenu.tsx`, `Landing.tsx`). This is the React community default.

Library modules in `lib/` are plain `.ts` files in lowercase (`i18n.ts`, `theme.ts`, `utils.ts`).

## Accessibility

WCAG 2.2 AA is the floor. Apply these patterns in components:

- **Icon-only buttons need an accessible name.** Set `aria-label` on every button or link whose visible content is purely a glyph (settings cog, hamburger, social icons in the footer, language switcher trigger). The label is the action, not the icon name (`aria-label="Settings"`, not `"Cog icon"`).
- **Decorative icons inside a labeled control** get `aria-hidden`. If the button already has visible text or a sibling label, mark the SVG `aria-hidden` so screen readers don't double-announce.
- **Interactive states must be reachable by keyboard.** Anything that handles `onClick` must also be focusable (use a `<button>` or `<a>`, not a `<div>`). Tab order should follow visual order. Esc closes overlays. Click-outside also closes overlays, but Esc is mandatory and click-outside is convenience.
- **Don't disable focus rings.** If the default ring is visually wrong, restyle via `--color-ring` or `focus-visible:` utilities. Never remove it. Sighted keyboard users need to see where focus is.
- **Touch targets are 40×40 CSS px or larger** for primary actions, with 24×24 as the absolute minimum. The settings cog and hamburger use `size-9` (36px), which is acceptable on dense chrome. Full-width tap rows on mobile menus use `py-3` to clear 40px.
- **Color is never the sole signal.** The current-item indicator in the language list is a ✓ icon, not just a color shift. The destructive token is paired with the word `fatal:` in the 404 terminal, not just red text. The theme toggle has Sun/Moon/Monitor icons alongside the label.
- **Respect `prefers-reduced-motion`.** Popover animations from `tw-animate-css` honor this by default. If hand-rolling animations, gate them behind `motion-safe:`.
- **Test before merging:** tab through the new UI with the keyboard only; resize to 375px; toggle dark mode; check focus rings are visible against both themes.
