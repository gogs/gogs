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

Palettes are adapted from [Pierre Theme](https://github.com/pierrecomputer/theme)'s "Light" and "Dark" (non-soft) variants. The dark-mode input background is bumped slightly above Pierre's value (`#262626` instead of `#1d1d1d`) so form fields read as edged elements outside an IDE panel context. Dark mode is opt-in via the `.dark` class on `:root` (see `lib/theme.ts`), not media-query driven, so the user's stored preference always wins. The `@custom-variant dark` rule in `index.css` lets utilities like `dark:...` target the same class.

Use these tokens. Don't introduce raw hex values in components.

**Surfaces and content**

- `--color-background`: page background. Body uses this by default.
- `--color-foreground`: primary content. Headings, active states, the main label of any item, body text on `--color-background`.
- `--color-muted-foreground`: secondary content. Metadata, helper text, terminal prompt characters, footer chrome, inactive items in a toggle group.
- `--color-surface`: subtle raised surface. Used for hover backgrounds (`hover:bg-(--color-surface)` on links, buttons, menu rows) and for the muted fill of the faux-terminal frame.
- `--color-card` / `--color-card-foreground`: card surface and its body text. Not currently used in components, but available for content cards.
- `--color-popover` / `--color-popover-foreground`: popover surface and body. Used by the Radix popover primitive.

**Accents and state**

- `--color-primary` / `--color-primary-foreground`: brand blue (`#009fff` in both modes). Reserved for genuine brand emphasis. Don't use it to mean "primary action" between two peer links (see the peer-item rule below). Note: white-on-primary contrast is 2.84:1, which is below WCAG AA in both modes since the token is identical light and dark. Avoid using primary as a fill for body-sized text. Use it for chrome accents, ring/focus, and large CTAs only.
- `--color-secondary` / `--color-secondary-foreground`: neutral support fill. Available for chips, tags, low-emphasis fills.
- `--color-destructive` / `--color-destructive-foreground`: error and danger. The 404 page uses `text-(--color-destructive)` on the `fatal:` token, always paired with the word itself (color is never the sole signal).
- `--color-success`: affirmative state for signature verification badges and copy-confirm checkmarks. Lighter in dark mode (`#4ade80`) than light (`#15803d`) so it reads on both backgrounds. Always pair with a label or icon, never color alone.
- `--color-diff-added` / `--color-diff-removed`: diff change markers (the +/- dots in the diff toolbar stats row, and any future per-line tints). Separate from `--color-success`/`--color-destructive` so the diff palette can drift toward the universal git green/red without dragging the success/error semantics along.
- `--color-ring`: keyboard focus ring color. Don't override per-component. If a default ring looks wrong, fix it at the token level.

**Structure**

- `--color-border`: soft container and divider lines. Used for the navbar bottom border, popover edges, card outlines, mobile-menu separators. Deliberately low-contrast (close to `--color-secondary`) so chrome reads as quiet boundary, not as a hard rule.
- `--color-input`: input field borders. Similar weight to `--color-border` but kept as a separate token so form fields can drift independently if needed.

**The terminal frame is the exception.** Faux-CLI surfaces (`Landing.tsx`, `NotFound.tsx`, `ServerError.tsx`) wrap their output in a heavy outline so it actually looks like a terminal window. That frame uses `border-(--color-foreground)/80` (light) and the regular `--color-border` token (dark) directly, instead of the shared chrome token.

**Peer-item rule**

Don't use foreground vs muted-foreground to imply "primary action" vs "secondary action" between two peer items (e.g. Sign in vs Register). Peer items get the same color. Differentiation comes from positioning, weight, or affordance, not arbitrary contrast. Active vs inactive _states_ of the same control (e.g. the selected theme tile in `SettingsMenu`) are a different case and may use the foreground/muted-foreground split to communicate selection.

**Ad-hoc colors**

The traffic-light cluster in the faux-terminal frame uses one ad-hoc value: the amber dot falls back to `oklch(0.795 0.184 86.047)` via `bg-(--color-warning,...)`. There is no `--color-warning` token defined, so the fallback always wins. This is deliberate. Promoting it to a real token would invite reuse, and warning is not a system-wide concern in the current UI. Leave it inline until a second site needs warning semantics, then define the token in both light and dark palettes.

## Surface chrome

The landing and 404/500 pages wrap their content in a faux-terminal frame (rounded border, traffic-light dots, monospace body). Reuse the same frame for any page that represents a Git/CLI state: the landing page, error pages, command-output stubs, raw diff fallbacks. Don't reuse it for normal content pages like settings or repository views.

Strings rendered inside a terminal frame stay in English across all locales, regardless of the active UI language. Real CLI output (`git`, `ls`, `cat`, etc.) doesn't localize. Faux-CLI that does loses authenticity and reads as a translated error page in a costume. Translate the surrounding prose (headings, descriptions, CTAs), but leave command names, prompts, error tokens like `fatal:`, and command output strings untouched.

## File naming

Two conventions coexist in `web/src/`:

- **shadcn primitives** in `components/ui/` use **lowercase** filenames (`popover.tsx`). This matches the `shadcn` CLI output and lets dropped-in components stay byte-identical to upstream.
- **App components** anywhere else use **PascalCase** matching the exported component (`Footer.tsx`, `SettingsMenu.tsx`, `Landing.tsx`). This is the React community default.

Library modules in `lib/` are plain `.ts` files in lowercase (`i18n.ts`, `theme.ts`, `utils.ts`).

## Forms

Disable the entire form while a submit is in flight, not just the submit button. Wrap the body in `<fieldset disabled={submitting} className="contents">` — native `disabled` propagates to every nested input and button.

Anchor links inside the form aren't covered by `disabled`. For each, set `tabIndex={submitting ? -1 : N}`, `aria-disabled={submitting || undefined}`, `className={submitting ? "pointer-events-none opacity-50" : undefined}`, and an `onClick` that calls `e.preventDefault()` when submitting.

Swap the submit label to a present-continuous string ("Signing in…", "Verifying…") while submitting. Keep idle and active strings as separate locale keys.

## Interactive affordances

Tailwind 4's preflight removes the browser default `cursor: pointer` on `<button>`. Without it, controls visually read as static text. Apply `cursor-pointer` on every interactive element that isn't a plain link: buttons, custom clickable rows, menu triggers, anything whose `onClick` activates an action. `<a href>` keeps the link cursor automatically.

When in doubt, hover-test the new control: if the cursor is still an I-beam or arrow, add `cursor-pointer`.

## Tooltips

Use the `Tooltip` component from `@/components/ui/tooltip` (built on `@radix-ui/react-tooltip`) for any hover hint. Never use the native HTML `title` attribute: it renders unstyled, has inconsistent timing across browsers, and is invisible on touch. The tooltip provider lives at the router root, so `Tooltip`/`TooltipTrigger`/`TooltipContent` work anywhere downstream.

The tooltip is supplementary information, not a substitute for an accessible name. Icon-only buttons still need `aria-label`. The tooltip just makes the same label visible to sighted users.

## Accessibility

WCAG 2.2 AA is the floor. Apply these patterns in components:

- **Icon-only buttons need an accessible name.** Set `aria-label` on every button or link whose visible content is purely a glyph (settings cog, hamburger, social icons in the footer, language switcher trigger). The label is the action, not the icon name (`aria-label="Settings"`, not `"Cog icon"`).
- **Decorative icons inside a labeled control** get `aria-hidden`. If the button already has visible text or a sibling label, mark the SVG `aria-hidden` so screen readers don't double-announce.
- **Interactive states must be reachable by keyboard.** Anything that handles `onClick` must also be focusable (use a `<button>` or `<a>`, not a `<div>`). Tab order should follow visual order. Esc closes overlays. Click-outside also closes overlays, but Esc is mandatory and click-outside is convenience.
- **Don't disable focus rings.** If the default ring is visually wrong, restyle via `--color-ring` or `focus-visible:` utilities. Never remove it. Sighted keyboard users need to see where focus is.
- **Touch targets are 24×24 CSS px at minimum.** Compact chrome (settings cog, hamburger) uses `size-9` (36px). Full-width tap rows in popovers and the mobile menu use `px-2 py-1.5`, which yields ~28px in height. The full row width gives the tap area enough horizontal slack to clear the minimum comfortably. Exception: edge-seam affordances like the resize handle in `ResizableSidebar` stay visually thin (4–8px) and rely on a keyboard fallback (`tabIndex={0}` plus arrow-key handlers) for accessibility. The handle is desktop-only (`lg:block`), so the 24px tap floor for touch users does not apply.
- **Color is never the sole signal.** The current-item indicator in the language list is a ✓ icon, not just a color shift. The destructive token is paired with the word `fatal:` in the 404 terminal, not just red text. The theme toggle has Sun/Moon/Monitor icons alongside the label.
- **Respect `prefers-reduced-motion`.** Popover animations from `tw-animate-css` honor this by default. If hand-rolling animations, gate them behind `motion-safe:`.
- **Test before merging:** tab through the new UI with the keyboard only; resize to 375px; toggle dark mode; check focus rings are visible against both themes.
