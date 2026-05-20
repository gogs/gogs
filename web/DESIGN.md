# Gogs web — design notes

A running record of design decisions made for the SPA. Add an entry when a pattern is used in two places, or when a question caused a redo. Don't write aspirationally — only capture what's already true in the code.

## Typography

Self-hosted via `@fontsource-variable`:

- **Sans** — Geist Variable, with PingFang SC / Microsoft YaHei as CJK fallbacks. Used for body text, headings, and UI chrome.
- **Mono** — Geist Mono Variable, with the same CJK fallbacks. Used for code-shaped content (SHA, branch name, file path, shell command, terminal-style surfaces).

The browser does per-glyph fallback via the font-family stack: Latin characters render in Geist / Geist Mono (the designed personality), CJK characters render in the next available system font (PingFang SC, Microsoft YaHei). The result: English looks distinctively Gogs, other scripts look clean and native.

Use mono only for content that **is** code, not for UI chrome (navbars, buttons, labels). Mono CJK fallbacks aren't truly monospace (CJK glyphs are wider than Latin), which is fine when the content is genuinely code, but reads as broken alignment if used decoratively for chrome.

Don't mix sans and mono within the same UI surface for arbitrary reasons. If a component is showing code, all of it goes mono.

## Color hierarchy

Two foreground tokens carry meaning:

- `--color-foreground` — primary content. Use for headings, active states, and the main label of any item.
- `--color-muted-foreground` — secondary content. Use for metadata, helper text, and items that are not the focal point.

Don't use foreground vs muted-foreground to imply "primary action" vs "secondary action" between two peer items (e.g. Sign in vs Register). Peer items get the same color; differentiation comes from positioning, weight, or affordance — not arbitrary contrast.

## Surface chrome

The 404 page wraps its content in a faux-terminal frame (rounded border, traffic-light dots, monospace body). Reuse the same frame for any page that represents a Git/CLI state — error pages, command-output stubs, raw diff fallbacks. Don't reuse it for normal content pages.

Strings rendered inside a terminal frame stay in English across all locales, regardless of the active UI language. Real CLI output (`git`, `ls`, `cat`, etc.) doesn't localize; faux-CLI that does loses authenticity and reads as a translated error page in a costume. Translate the surrounding prose (headings, descriptions, CTAs) but leave command names, prompts, error tokens like `fatal:`, and command output strings untouched.

## Accessibility

WCAG 2.2 AA is the floor. Apply these patterns in components:

- **Icon-only buttons need an accessible name.** Set `aria-label` on every button or link whose visible content is purely a glyph (settings cog, hamburger, social icons in the footer, language switcher trigger). The label is the action, not the icon name — `aria-label="Settings"` not `"Cog icon"`.
- **Decorative icons inside a labeled control** get `aria-hidden`. If the button already has visible text or a sibling label, mark the SVG `aria-hidden` so screen readers don't double-announce.
- **Interactive states must be reachable by keyboard.** Anything that handles `onClick` must also be focusable (use a `<button>` or `<a>`, not a `<div>`). Tab order should follow visual order. Esc closes overlays. Click-outside also closes overlays — but Esc is mandatory, click-outside is convenience.
- **Don't disable focus rings.** If the default ring is visually wrong, restyle via `--color-ring` or `focus-visible:` utilities — never remove it. Sighted keyboard users need to see where focus is.
- **Touch targets are 40×40 CSS px or larger** for primary actions, 24×24 absolute minimum. The settings cog and hamburger use `size-9` (36px) which is acceptable on dense chrome; full-width tap rows on mobile menus use `py-3` to clear 40px.
- **Color is never the sole signal.** The current-item indicator in the language list is a ✓ icon, not just a color shift. The destructive token is paired with the word "fatal:" in the 404 terminal, not just red text. The theme toggle has Sun/Moon/Monitor icons alongside the label.
- **Respect `prefers-reduced-motion`.** Popover animations from `tw-animate-css` honor this by default; if hand-rolling animations, gate them behind `motion-safe:`.
- **Test before merging:** tab through the new UI with the keyboard only; resize to 375px; toggle dark mode; check focus rings are visible against both themes.
