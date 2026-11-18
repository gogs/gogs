# Gogs web design notes

A running record of design decisions made for the web frontend. Add an entry when a pattern is used in two places, or when a question caused a redo. Don't write aspirationally. Only capture what's already true in the code.

## Typography

Self-hosted via `@fontsource-variable`:

- **Sans**: Geist Variable, with PingFang SC and Microsoft YaHei as CJK fallbacks. Used for body text, headings, and UI chrome.
- **Mono**: Geist Mono Variable, with the same CJK fallbacks. Used for code-shaped content (SHA, branch name, file path, shell command, terminal-style surfaces).

The browser does per-glyph fallback via the font-family stack. Latin characters render in Geist (the designed personality). CJK characters render in the next available system font (PingFang SC, Microsoft YaHei). The result: English looks distinctively Gogs, other scripts look clean and native.

Use mono only for content that **is** code, not for UI chrome (navbars, buttons, labels). Mono CJK fallbacks aren't truly monospace (CJK glyphs are wider than Latin), which is fine when the content is genuinely code, but reads as broken alignment if used decoratively for chrome.

Don't mix sans and mono within the same UI surface for arbitrary reasons. If a component is showing code, all of it goes mono.

Inline code within text is a chip, `rounded border border-(--color-border) bg-(--color-surface) px-1.5 font-mono`, so it reads as code without relying on the font alone. The border keeps the chip visible where the surface color is close to the background. Give it `text-(--color-foreground)` when it sits in muted text, and `box-decoration-clone` when a long value may wrap.

## Color hierarchy

Palettes are adapted from [Pierre Theme](https://github.com/pierrecomputer/theme)'s "Light" and "Dark" (non-soft) variants. Text, link, border, and input tokens are darkened in light mode and lightened in dark mode from Pierre's values, so every text pair meets 4.5:1 and input edges meet 3:1 against the page background. Check a changed token against both floors before landing it. Dark mode is opt-in via the `.dark` class on `:root` (see `lib/theme.ts`), not media-query driven, so the user's stored preference always wins. The `@custom-variant dark` rule in `index.css` lets utilities like `dark:...` target the same class.

Use these tokens. Don't introduce raw hex values in components.

**Surfaces and content**

- `--color-background`: page background. Body uses this by default.
- `--color-foreground`: primary content. Headings, active states, the main label of any item, body text on `--color-background`.
- `--color-muted-foreground`: secondary content. Metadata, helper text, terminal prompt characters, footer chrome, inactive items in a toggle group.
- `--color-surface`: subtle raised surface. Used for hover backgrounds (`hover:bg-(--color-surface)` on links, buttons, menu rows) and for the muted fill of the faux-terminal frame.
- `--color-card` / `--color-card-foreground`: card surface and its body text. Not currently used in components, but available for content cards.
- `--color-popover` / `--color-popover-foreground`: popover surface and body. Used by the Radix popover primitive.

**Accents and state**

- `--color-primary` / `--color-primary-foreground`: brand blue, deepened to `#0b6bcb` in light mode and lightened to `#33b1ff` in dark mode so it reads as link text. `--color-primary-foreground` flips to near-black in dark mode to keep button labels readable on the lighter fill. Reserved for genuine brand emphasis. Don't use it to mean "primary action" between two peer links (see the peer-item rule below).
- `--color-secondary` / `--color-secondary-foreground`: neutral support fill. Available for chips, tags, low-emphasis fills.
- `--color-destructive` / `--color-destructive-foreground`: error and danger. The 404 page uses `text-(--color-destructive)` on the `fatal:` token, always paired with the word itself (color is never the sole signal).
- `--color-success`: affirmative state for signature verification badges and copy-confirm checkmarks. Lighter in dark mode (`#4ade80`) than light (`#15803d`) so it reads on both backgrounds. Always pair with a label or icon, never color alone.
- `--color-merged`: merged pull requests, for the Merged pill and the merge box icon and border. Purple like other forges, darker in light mode (`#6f42c1`) than dark (`#b392f0`) so it keeps 4.5:1 on the pill's 15% tint. Always pair with a label or icon.
- `--color-diff-added` / `--color-diff-removed`: diff change markers (the +/- dots in the diff toolbar stats row, and any future per-line tints). Separate from `--color-success`/`--color-destructive` so the diff palette can drift toward the universal git green/red without dragging the success/error semantics along.
- `--color-ring`: keyboard focus ring color, at least 3:1 against the surfaces of its theme. Don't override per-component. If a default ring looks wrong, fix it at the token level. A control filled with a color, e.g., a primary button, a checked checkbox or a pressed toggle, shows its focus with `FILLED_FOCUS` from `@/lib/utils` in place of the ring: an outline in the same color a gap away, since a ring that touches the fill merges with it.

**Structure**

- `--color-border`: soft container and divider lines. Used for the navbar bottom border, popover edges, card outlines, mobile-menu separators. Quieter than `--color-input` so chrome reads as a boundary, not as a hard rule, but still visible against the page background in both modes.
- `--color-input`: input field borders. Similar weight to `--color-border` but kept as a separate token so form fields can drift independently if needed.

**The terminal frame is the exception.** Faux-CLI surfaces (`Landing.tsx`, `NotFound.tsx`, `ServerError.tsx`) wrap their output in a heavy outline so it actually looks like a terminal window. That frame uses `border-(--color-foreground)/80` (light) and the regular `--color-border` token (dark) directly, instead of the shared chrome token.

**Peer-item rule**

Don't use foreground vs muted-foreground to imply "primary action" vs "secondary action" between two peer items (e.g. Sign in vs Register). Peer items get the same color. Differentiation comes from positioning, weight, or affordance, not arbitrary contrast. Active vs inactive _states_ of the same control (e.g. the selected theme segment in `SettingsMenu`) are a different case and may use the foreground/muted-foreground split to communicate selection.

**Ad-hoc colors**

The traffic-light cluster in the faux-terminal frame uses one ad-hoc value: the amber dot is the literal `oklch(0.795 0.184 86.047)` at 70% opacity. It is decoration, so it keeps that softer amber instead of the warning token.

**Warning**

`--color-warning` (amber, defined in both palettes) marks content that asks for attention without being an error, e.g., the notice banner below the navbar. It is dark enough in the light palette to meet 3:1 as an icon on white, so use it for the icon and border, a 10% tint of it for the background, and keep the text in `--color-foreground`. A warning bar across the page takes `glass-bar` like the navbar, with its tint on an inner layer, since the marker sets the bar's background color. Links inside a warning surface use the foreground color with an underline, because the primary color loses contrast on the tint.

## Glass

The app has a frosted glass look, off by default. Users switch it on with "Glass effect" in the settings menu, next to the theme. The choice is kept in `localStorage` under `gogs-glass` (see `lib/glass.ts`). While it is on, `<html>` carries the `glass-ui` class, which the inline script in `index.html` sets before the first paint. Every glass style is scoped under that class, so the app without it looks exactly as it does with plain tokens: never change the plain look to get a glass effect.

With glass on, `html` paints `--glass-backdrop`, a few soft color blobs over `--glass-page`, fixed to the viewport, with a dot grid that fades toward the bottom. Panels are translucent over it. Components opt in with marker classes, which style nothing while glass is off. Write the element's plain look with utilities as usual and add the marker:

- **`glass`** goes on content containers: bordered boxes, comments, `Card`, the terminal frames, the settings panels and the diff workspace. It sets the translucent `--glass-fill`, a light top edge and a soft drop shadow, blurs what is behind it and rounds it to `--glass-radius` (`0.75rem`, `Card` sets `1rem`).
- **`glass-list`** goes on lists of rows divided by bottom borders, e.g., the activity feed and repository lists, which become a glass panel with the rows inset. An empty list hides.
- **`glass-section`** goes on a section whose heading labels one glass box or list below it, e.g., the dashboard activity feed, the branch groups and the commit days. The section becomes the panel, the heading its header row and the box inside loses its own frame, so the heading does not float alone on the backdrop. Tabs that switch what one panel shows, e.g., Repositories and Public Activity on a profile, become its header row the same way. Page titles stay above their panels.
- **`glass-bar`** goes on full-width chrome that content scrolls under: the navbar and the repository header.
- **`glass-control`** goes on outline buttons and segmented groups, e.g., Watch, Star and Fork, and the view switchers. A glass control sets its own fill, which outranks `hover:` utilities, so it brings its own hover: `--glass-control-hover` and a `--glass-line-hover` edge, and a group lights its edge while a button in it is hovered. Check hover in a browser that reports `(hover: hover)`, since Tailwind wraps `hover:` in that media query and headless Chrome without a mouse matches none of them.
- **`glass-solid`** goes on filled buttons, with `--glass-solid` set to the fill color. It adds a light sheen, a bright top edge, a ring in that color and a soft glow, like the landing page's call to action. Glass shadows that replace `box-shadow` list Tailwind's `--tw-ring-offset-shadow` and `--tw-ring-shadow` first, so keyboard focus stays visible.
- **`glass-tabs`** goes beside `glass-control` on a row of tabs or toggles that are loose pills without glass, e.g., Write and Preview above a Markdown editor, the branches and tags of the branch picker and the protocols of the clone box. The row shrinks to its tabs and becomes one control on a single row, with the tabs as flush segments divided by the glass edge, so a row that may run out of room scrolls sideways or truncates its labels instead of wrapping. The tab shown keeps the `--color-surface` fill and the bold text it has without glass, and a hovered tab takes the same fill. Two rows differ: the theme picker of `SettingsMenu` spans its menu with `glass-ui:!w-full`, and `Segmented` of the admin console, a pill without glass, takes the fill and the bold text through `glass-ui:` utilities (see Admin console). Page tabs with an underline, e.g., on the explore page, stay as they are.

Layout that only applies to glass, e.g., a border or padding the plain look does not have, uses the `glass-ui:` variant, e.g., `glass-ui:border glass-ui:p-3`.

With glass on, only page titles, page tabs and toolbars sit on the backdrop. Everything else sits on a glass panel, including sidebars, e.g., the repository About column and the issue sidebar, which become one panel with `glass glass-ui:self-start glass-ui:border glass-ui:p-4`, and rendered documents such as wiki pages. Forms that are not in a settings panel, e.g., new repository, new milestone and new release, are one panel with `glass glass-ui:border glass-ui:p-4 sm:glass-ui:p-6`. Columns of panels sit 16px apart with `glass-ui:gap-x-4`. Full-height chrome beside the content, e.g., the file tree pane of the code view, takes `glass-bar`. A panel inside another panel, e.g., the Markdown editor in a form, drops its fill and shadow and keeps a faint border, and a `glass-section` inside another keeps its header row without a frame.

Every control that draws its own border takes a marker. A button or link styled by hand rather than through `Button`, e.g., a commit SHA link or "Browse files", adds `glass-control`, and so does a dropdown in a toolbar, e.g., the issue filters. A field you type into or pick a value for in a form keeps the field look of `--color-field` and `--color-input`, whose stronger edge keeps the 3:1 contrast a field boundary needs.

Borders follow the 1.0 landing page: thin translucent lines, never opaque gray. A glass panel draws its edge in `--glass-line` and lowers `--color-border` to `--glass-divider` for everything inside it, so its rows and sections divide faintly. A state that recolors a panel's edge, e.g., a linked comment, needs `!border-...`, since the marker rules outrank utilities. Round a header that fills the top of a panel with `rounded-t-[inherit]`, so it follows the panel's radius in both looks.

Glass also makes `--color-surface`, `--color-secondary`, `--color-border` and `--color-input` translucent, so hover fills and dividers tint the glass under them, and `--color-popover` mostly opaque, so popovers, selects and dialogs keep text readable over what they cover. Their blur is `glass-ui:backdrop-blur-*`, so it only applies with glass on. Form fields use `bg-(--color-field)`: the page color without glass, and with glass lighter than the panels in light mode and darker in dark mode, so fields read as inset. Code in diffs and highlighted blocks takes the same field color, and file trees take the panel they sit in. Don't put a glass marker on elements with `position: fixed` children, since `backdrop-filter` makes it their containing block.

## Surface chrome

The landing and 404/500 pages wrap their content in a faux-terminal frame (rounded border, traffic-light dots, monospace body). Reuse the same frame for any page that represents a Git/CLI state: the landing page, error pages, command-output stubs, raw diff fallbacks. Don't reuse it for normal content pages like settings or repository views.

Strings rendered inside a terminal frame stay in English across all locales, regardless of the active UI language. Real CLI output (`git`, `ls`, `cat`, etc.) doesn't localize. Faux-CLI that does loses authenticity and reads as a translated error page in a costume. Translate the surrounding prose (headings, descriptions, CTAs), but leave command names, prompts, error tokens like `fatal:`, and command output strings untouched.

## Page width

Pages with a file tree, i.e., the code view below the repository root and commit, compare and pull request diffs, use the whole window width. They set `data-fullwidth` on `<html>`, which widens the navbar, the repository header and the footer to match, and keep the 16px or 24px page padding. The file tree sits in its own bordered box beside the content, never flush against the window edge. The admin console uses the whole window width too, in a layout of its own (see Admin console). Other pages stay centered at `max-w-6xl` or `max-w-7xl`.

## File naming

Two conventions coexist in `web/src/`:

- **shadcn primitives** in `components/ui/` use **lowercase** filenames (`popover.tsx`). This matches the `shadcn` CLI output and lets dropped-in components stay byte-identical to upstream.
- **App components** anywhere else use **PascalCase** matching the exported component (`Footer.tsx`, `SettingsMenu.tsx`, `Landing.tsx`). This is the React community default.

Library modules in `lib/` are plain `.ts` files in lowercase (`i18n.ts`, `theme.ts`, `utils.ts`).

## Forms

Disable the entire form while a submit is in flight, not just the submit button. Wrap the body in `<fieldset disabled={submitting} className="contents">` — native `disabled` propagates to every nested input and button.

Anchor links inside the form aren't covered by `disabled`. For each, set `tabIndex={submitting ? -1 : N}`, `aria-disabled={submitting || undefined}`, `className={submitting ? "pointer-events-none opacity-50" : undefined}`, and an `onClick` that calls `e.preventDefault()` when submitting.

Swap the submit label to a present-continuous string ("Signing in…", "Verifying…") while submitting. Keep idle and active strings as separate locale keys.

Take the message for a whole form from `formErrors` (`@/lib/queries/request`) and show it in the form, e.g., in `FormAlert`. It tells a viewer whose session expired to sign in again, with a link that opens on another tab, so the form keeps what they typed. Report the failure of a change made with a button, where nothing typed is lost, with `toastError` (`@/lib/toast-error`): it takes that viewer to sign in and back to the page instead of showing a toast. Toasts follow the theme of the page, and take the pointer above an open dialog (see `index.css` and `DialogContent`), so a click on one never lands on the button under it.

## Settings pages

The settings pages of users, organizations and repositories use `SettingsLayout` from `@/components/SettingsLayout`: the links between the pages sit beside the page. Below the `md` breakpoint they move into a full-width dropdown above the page, named after the current page, since a row of ten links does not fit. Wrap each link in `SettingsNavItem` with a leading Lucide icon, and give every item of a menu one. The current page gets a bar in `--color-primary` beside its surface fill, so it shows in dark mode too. Menus with more than a few items put related links under a heading with `SettingsNavGroup`, as GitHub does, e.g., "Access" and "Code and automation", and leave the first group without one. Each page does one job. The destructive actions of a menu sit on their own page named "Danger zone", below a `SettingsNavDivider`, marked with `destructive`, along with changes that are hard to undo, e.g., the visibility of a repository. Pages still served by the server are plain `<a>` links. Give each `Link` `activeOptions={{ exact: true }}`, since TanStack Router otherwise marks a link as the current page on every page under its path.

Actions that cannot be taken back, such as deleting a team or leaving a repository, ask first in a dialog with `ConfirmButton`. A destructive button on the page is an outline button with `--color-destructive` text, and only the button that confirms inside the dialog is solid `destructive`. When the action only starts something that goes on for a while, e.g., an operation of the admin console, let it return at once so the dialog closes, and set `busy` on the `ConfirmButton` until the work ends. The dialog (`components/ui/dialog.tsx`) is a bottom sheet below `sm` and centered from `sm` up. Its body scrolls while the title and the actions stay in view, so the actions never get clipped on a short screen. After such an action empties the page of a paginated list, move to the previous page instead of showing an empty one. Deleting an account asks for the password instead, with `DeleteForm`, and the danger zone of a repository asks for its name in the same dialog.

## Lists

Lists that can be searched by keyword take `q` and `page` search params, validated with `validateListSearch` from `@/lib/page-search`, and search with `SearchForm` from `@/components/SearchForm`, which starts over from the first page.

The branches page searches as you type instead, like GitHub: the URL follows the field after a short pause, through a `query` search param validated with `validateBranchesSearch`. The field sits to the right of the view switcher from `sm` up and below it on narrow screens. Searching from the overview moves to all branches.

## Reports

Pages that show settings or measurements, e.g., the admin configuration, list them as a `<dl>` with the label beside its value where there is room for both and above it otherwise. The configuration page decides that by the width of the card of a section, with a container query. Divide long reports into groups with a gap rather than a rule. Show each setting value as inline code, as it is written in the configuration, and keep units, notes and Yes or No text outside the chip. Measurements, e.g., memory sizes and durations on the admin dashboard, show the whole value with its unit in the chip. Show a setting that has no value as "(not set)" instead of leaving it blank. When a report has more labels than the SPA should bundle in every locale, as the configuration page does, the server sends them already translated, and the page only renders them.

## Highlighted code

Show code with syntax highlighting, e.g., the payload of a webhook delivery, with `CodeBlock` from `@/components/CodeBlock`. It renders through `@pierre/diffs` with the same themes and worker pool as the files of a commit, and follows the app theme.

Show the changes between two commits, e.g., a commit and its parent or the range of a push, with `DiffWorkspace` from `@/components/DiffWorkspace`. It takes the raw diff and both commits, and brings the full-width layout, the file tree, the toolbar and line expansion, so a page only adds its own header above it.

Edit Markdown and code with `CodeEditor` from `@/components/CodeEditor`, never a plain `<textarea>`. It is the one editor of the app, built on CodeMirror 6. Markdown wraps like prose and Tab moves focus. `code` lays out source code with line numbers and Tab indents, and Escape then Tab leaves it. Token colors are the `--code-*` variables in `index.css`. For a file, pass the `lang` that `codeLanguageOf` picks by its name, and the `indentUnit` and `tabSize` its `.editorconfig` sets, which can all change while the editor is open. Plain text fields, such as descriptions and SSH keys, stay textareas.

## Rendered Markdown

Show Markdown that the server rendered and sanitized, e.g., the content of an issue and its comments, with `Markdown` from `@/components/Markdown`. It styles the HTML with the `markdown` class in `index.css`, and highlights fenced code blocks that name a language with `CodeBlock`. Previews of Markdown being written go through the same component, so they look like the result.

Forms that open or change issues and releases share their parts from `@/components/IssueForm`: `MarkdownEditor` with its Write and Preview tabs, `AttachFilesButton` and `PendingFiles` with `useAttachmentUploads`, and `IssueMeta` for the labels, milestone and assignee that writers set on issues. `useAttachmentUploads` takes `"releases"` to upload under the release attachment limits.

The issue and pull request pages share their parts from `pages/repo/Issue.tsx`: `IssueHeader` for the title, the state pill and a summary, and `IssueConversation` for the comments, the form to comment and the sidebar. The pull request page adds its tabs below the header and its merge box above the form to comment.

## Repository refs and clone URLs

Pages that show a repository at a branch, a tag or a commit, e.g., the code browser and the commit history, switch it with `RefPicker` from `@/components/RefPicker`, which links each ref to the same page through `renderLink`. Without `tags`, it picks only branches, under the `label` it is given, e.g., the base and the compared branch of a new pull request. Show the clone URL of a repository or a wiki with `CloneBox` from `@/components/CloneBox`. It remembers the protocol the viewer picked under the key the server-rendered pages use.

## Admin console

The pages under `/admin` render in `AdminLayout` from `@/components/AdminLayout`, which the admin route mounts in place of the navbar and the footer: a sidebar with the links between the pages, in groups, and a bar above the page with the breadcrumb, the command palette, the settings menu and the user menu, in the order of the navbar. The sidebar collapses to a rail of icons, each named by a tooltip, remembers that choice, and moves into a sheet below the `md` breakpoint. The route lets only site admins in, so the console never shows around a page the viewer may not see. The notice banner shows below the bar, at the width of the page. Add a page by adding its route under the admin route, with the shared `PAGE` options and a loader that goes through `loadAdminPage`, and its link to `NAV`. With those options a page that is not found or fails to load says so in place of the page, and the console names the problem in the title and the breadcrumb. Pages below the path of a link, e.g., a user below the users, mark the link as the current page.

Pages of the console lay their content out in `Card`s on a grid that falls back to one column on narrow screens. A card that shows numbers or a chart fetches what it shows itself, with `useQuery` and a query from `@/lib/queries/admin`, instead of the route loader, since the sources of one page answer at different speeds and one of them failing must not take the page down. Render the answer through `Loaded` from `@/components/Loaded`: until the query answers it shows the `Skeleton` it is given, sized like the content it stands for so the page does not jump, and a failed query shows what could not load with a Retry button, only in the cards that read it. A part that lays out its own answer, e.g., the items of a list, renders `NotLoaded` from the same file until the query answers. A viewer the web API turns away, e.g., after their session expired, leaves the console through a full page load, whether a card or a loader asked (see `leaveAdmin` in `@/lib/admin-exit`). Titles, descriptions and links of a card render right away. The overview does this, and refreshes itself: its queries take the interval of `useRefreshSeconds` from `@/lib/admin-refresh` as their `refetchInterval`, a minute unless the viewer picked another, and the browser remembers the pick. The other pages of the console still load through their route loader. Of those, the pages that show measurements (server health, and the overview and the search of the traces) refresh themselves at the same interval: their loader loads through the query of the page (`loadRefreshed` in `routes/admin.tsx`), and the page reads that query through `useRefreshed` from `@/lib/admin-refresh`, which asks again on the interval and keeps the last answer when a refresh fails. The two ask once between them, and a view the viewer returns to, e.g., with Back, shows at once at the scroll position they left, with skeletons in place of the numbers of their last visit until the answer arrives. The page takes `reloadingProps(useReloading())` from the same file on its root, and under it every part with the class `reloads` and every table cell shows as a skeleton of its own size, so nothing moves. Give that class to each value, chart and count the answer fills. The same skeletons show while another view of the page loads, e.g., another range, and not on a refresh, whose numbers stay until the new ones replace them. When that answer fails, the numbers of the last visit come back with the `RefreshNote`, and only a view that has none shows the error page. A `RefreshNote` then tells from when the page shows its numbers, so old numbers never pass for current ones. It goes on a line of its own below the controls (the `note` of `AdminPageHeader`), where it comes and goes without moving the Refresh button from under the pointer. Queries that refresh themselves take `refreshOptions` from the same file, so that turning the interval off also stops them from asking again when the viewer comes back to the tab. State of the page, e.g., a pinned moment or the filters in the URL, stays across a refresh.

The parts that pages of the console share are in `@/components/AdminPage`:

- `AdminPageHeader` heads a page with its title, a line that says what the page holds, e.g., the total of a list, and the actions of the page at its end. A page below a page of the navigation, e.g., one user, passes `back` for a link to that page and `crumb` for its own name in the breadcrumb.
- `RefreshControl` sits in the header of a page that refreshes itself: a button that refreshes right away, and a dropdown that sets how often the page does so by itself, or turns that off. While a refresh runs, the button is `aria-disabled` and takes no click. The queries a page refreshes give up after 60 seconds (`inTime` in `@/lib/queries/admin`), so the button comes back when the server does not answer.
- `SegmentedLinks` switches a page between views of the same data through a search param, e.g., the last hour and the last 24 hours, so the route loader fetches the view. The view shown is a raised pill with a border and `aria-current="page"`. That is the view the page shows, not the one the URL names, which is still loading right after a click.
- `StatCard` shows one number under what it measures, sized with `STAT_VALUE`. `TrendChip` in its corner says what stands out, e.g., "Spike from 14:32", with an icon and text instead of a color. Cards below a section heading take `as="h3"`.
- `AdminFormCard` holds a form, with its fields in a `FieldGrid` that takes two columns once the card is wide enough, its checkboxes in a `CHECKBOX_GROUP` below a rule, and its actions in a bar at the foot of the card. Forms stay at most `max-w-5xl` wide, since longer fields are harder to read.
- `Rows` and `Row` list labeled values in a card.

Lists of the console are a `Table` from `@/components/ui/table` in a `Card` with `@container` and `overflow-hidden`, under the header of the page and the `SearchForm`, with the pagination in the footer of the card. The first column names the row and links to it, and the last holds the action of the row. Columns hide by the width of the card, e.g., `hidden @2xl:table-cell`, not of the window, since the sidebar changes how much room the table has, and until a column shows, the first column tells what it holds in a muted line below the name. Nothing a table shows may be out of reach at 375px, and a table never scrolls sideways. A checkbox in a cell sits in a `<label>` of at least 24px by 24px. A column that sorts the table is headed by `TableSortHead`, a button in the header cell that carries `aria-sort`: a click puts the largest first, a second the smallest, and a third returns to the first order of the table. Sort in the browser only when it holds every row, not a page of them. While the card hides sortable columns, a `Select` above the table sorts by any of them, as on the traces page.

With [glass](#glass) on, the sidebar and the bar take `glass-bar`, and the frame around the page drops its fill and border, so the cards of a page sit on the backdrop as the panels of every other page do. The link of the current page in the sidebar fills with `--color-surface`, as in `SettingsLayout`. `Segmented` takes `glass-tabs`, so it turns into one control like the view switchers of the site, and its view shown is bold, so it does not rely on its fill alone. A dropdown or a chip in the toolbar above the cards takes `glass-control` too, e.g., the filters of the trace search, and a chip that marks its edge keeps the color with `!border-...`. Navigation beside the cards, e.g., the section list of the configuration, is a panel with `glass glass-ui:border glass-ui:p-2`.

## Charts

Charts use [Recharts](https://recharts.org) through `@/components/ui/chart`, the shadcn/ui chart primitive: wrap the chart in `ChartContainer` with a `ChartConfig` that gives each series its label and its color as a theme token, e.g., `var(--color-primary)`, so the chart follows the theme, and use `ChartTooltipContent` and `ChartLegendContent` for the tooltip and the legend. A series can set `format` in the config to write its values with their unit in the tooltip. Give the container a height, e.g., `aspect-auto h-64 w-full`. A chart with axes keeps the accessibility layer of Recharts, which moves the tooltip with the arrow keys, and takes an `aria-label` that says what it shows, not a `title`, which browsers show as a tooltip over the whole chart. A sparkline turns that layer off and is `role="img"` with an `aria-label` instead. Bars and lines that carry meaning use tokens with 3:1 contrast against the card, e.g., `--color-input` and `--color-primary`. Turn animations off with `isAnimationActive={false}`, since the data replaces itself on every refetch. The focused chart shows a ring, and `ChartContainer` keeps the groups Recharts renders its layers into out of the tab order, so a chart is a single stop.

Charts of the same moments share a cursor through `syncId`, as the cards of the server health page do. When the arrow keys do more than move the tooltip, e.g., pin a moment and jump to the first or last one, wrap the chart in a `role="slider"` that handles the keys and turn the accessibility layer off. Marks that differ in meaning differ in shape as well as in color, e.g., round marks for requests and square ones for failed requests in the plot of traces, and the legend shows each shape. A mark that opens something, e.g., a trace, is also reachable with the arrow keys and Enter. Plots of single traces or measurements start their time axis at the first one, with a window of at least 5 minutes, rather than at the start of the range, so a server that started recently does not squeeze them into the right edge.

Timelines of spans, e.g., the waterfall of a trace, are HTML rows with bars positioned by percentage, not a chart of the library.

## Translated markup

Some translations carry the HTML the server-rendered pages used, e.g., `<a href="%s">`, `<code>`, `<strong>` and colored `<span>` highlights, which show in bold. Render them with `RichText` from `@/components/RichText`, never with `dangerouslySetInnerHTML`. Pass link targets in `links`, in the order the links appear, since the placeholders in their `href` stay unfilled.

## Interactive affordances

Tailwind 4's preflight removes the browser default `cursor: pointer` on `<button>`. Without it, controls visually read as static text. Apply `cursor-pointer` on every interactive element that isn't a plain link: buttons, custom clickable rows, menu triggers, anything whose `onClick` activates an action. `<a href>` keeps the link cursor automatically.

When in doubt, hover-test the new control: if the cursor is still an I-beam or arrow, add `cursor-pointer`.

## Linked items

When the URL links to an item on the page, e.g., `#issuecomment-1`, `#event-2` or a section of the admin configuration, highlight it like GitHub does: a primary-colored 1px edge with a soft `ring-2 ring-(--color-primary)/30` halo. Read the hash from the router state with `useIsLinked` from `@/lib/use-is-linked`, not `:target`, which doesn't match items that render after the page loads or links that update the URL through the router. The page's `scroll-padding-top` keeps the item clear of the sticky navbar.

## Tabs

Page-level tabs (the repository header tabs, the explore tabs, the user profile tabs) are links in a row with a bottom border of `--color-border`. Each tab is `border-b-2 px-3 py-2 text-sm` with a leading `size-4` icon. The active tab gets `border-(--color-primary)`, `font-semibold`, foreground text, and `aria-current="page"`. Inactive tabs are muted and show a `--color-border` underline on hover. Below `sm`, never scroll the row horizontally. Keep only as many tabs inline as fit at 375px, always including the active one, and fold the rest into a trailing "More" popover menu (`Menu` icon, `aria-label` of `more_tabs`). From `sm` up, show the full row. Build these rows with `PageTabs`, which does this given how many tabs to keep inline. The repository header has its own row. A tab that counts something (the organization tabs, the pull request tabs) sets `count`, which ends it with a `rounded-full bg-(--color-secondary) px-2 text-xs/5 font-medium tabular-nums` badge. When one tab's path is a prefix of another's, e.g., a pull request and its commits, give its TanStack `Link` `activeOptions={{ exact: true }}`, or it also gets `aria-current="page"` on the longer path.

Tabs that switch what one editor shows, e.g., Write and Preview of `MarkdownEditor` and the tabs of the file editor, use `ContentTabs` from `@/components/IssueForm`: the same `border-b-2` underline in `--color-primary` on a row with a bottom border, without icons. Never mark the active tab with only a surface fill, which vanishes when the editor sits on a surface, e.g., the reply form of a thread.

Switches between sibling views inside a page (labels and milestones, the branch overview and all branches) are a `<nav>` of links in one `rounded-md border border-(--color-input)` group, divided by `border-l`. Each link is `px-3 py-1.5 text-sm` with a leading `size-4` icon. The current one gets `bg-(--color-surface)`, `font-semibold`, foreground text, and `aria-current="page"`. The others are muted.

A switcher with more links than fit at 375px, e.g., the five views of the branches page, scrolls sideways inside its own row, which runs to the edges of the screen below `sm`. The page itself stays 375px wide, and the current link scrolls into view when the page opens. Give each link `activeOptions={{ exact: true }}`, so that only one link is marked as the current page.

## Narrow screens

Every page works at 375px wide. Only content that is meant to scroll does so sideways: code, diffs, and a view switcher too long for the screen. Tab rows fold into a "More" menu instead (see Tabs).

- **Wrap long text with `wrap-anywhere`, not `break-all`.** Names, URLs and emails break at hyphens and slashes first, and mid-word only when nothing else fits. `break-all` splits ordinary words, e.g., "Joined o / n". Terminal frames keep `break-all`, since real terminals wrap anywhere, but translated prose inside them does not.
- **Keep a label and its value in one box.** Inside a flex row, wrap them in a single `<span>` so they wrap together as text instead of as two squeezed columns.
- **Top-align page grids that fill the height.** A `grid flex-1` page stretches its rows over the leftover height when it stacks on narrow screens, which shows as a gap between the sidebar and the content. Add `content-start`.
- **Truncate chips such as labels** with `max-w-full truncate` rather than letting a rounded pill wrap.

## Tooltips

Use the `Tooltip` component from `@/components/ui/tooltip` (built on `@radix-ui/react-tooltip`) for any hover hint. Never use the native HTML `title` attribute: it renders unstyled, has inconsistent timing across browsers, and is invisible on touch. The tooltip provider lives at the router root, so `Tooltip`/`TooltipTrigger`/`TooltipContent` work anywhere downstream.

The tooltip is supplementary information, not a substitute for an accessible name. Icon-only buttons still need `aria-label`. The tooltip just makes the same label visible to sighted users.

## Accessibility

WCAG 2.2 AA is the floor. Apply these patterns in components:

- **Icon-only buttons need an accessible name.** Set `aria-label` on every button or link whose visible content is purely a glyph (settings cog, hamburger, social icons in the footer, language switcher trigger). The label is the action, not the icon name (`aria-label="Settings"`, not `"Cog icon"`).
- **Decorative icons inside a labeled control** get `aria-hidden`. If the button already has visible text or a sibling label, mark the SVG `aria-hidden` so screen readers don't double-announce.
- **Interactive states must be reachable by keyboard.** Anything that handles `onClick` must also be focusable (use a `<button>` or `<a>`, not a `<div>`). Tab order should follow visual order. Esc closes overlays. Click-outside also closes overlays, but Esc is mandatory and click-outside is convenience.
- **Popover menus take `useMenuFocus`** from `@/lib/use-menu-focus`, as the menus of the navbar do. The menu itself takes the focus when it opens, so Tab goes on to its first control and Shift+Tab to its last, the trigger names the menu, and closing it hands the focus back to the trigger without scrolling the page. Whatever hands the focus to a control in a sticky bar passes `preventScroll`, or Chromium scrolls the page to its top.
- **Don't dim a control that keeps the focus with `opacity`**, e.g., a button that is busy. Its focus ring dims with it. Dim its text color instead.
- **Don't disable focus rings.** If the default ring is visually wrong, restyle via `--color-ring` or `focus-visible:` utilities. Never remove it. Sighted keyboard users need to see where focus is.
- **Touch targets are 24×24 CSS px at minimum.** Compact chrome (settings cog, hamburger) uses `size-9` (36px). Full-width tap rows in popovers and the mobile menu use `px-2 py-1.5`, which yields ~28px in height. The full row width gives the tap area enough horizontal slack to clear the minimum comfortably. Exception: edge-seam affordances like the resize handle in `ResizableSidebar` stay visually thin (4–8px) and rely on a keyboard fallback (`tabIndex={0}` plus arrow-key handlers) for accessibility. The handle is desktop-only (`lg:block`), so the 24px tap floor for touch users does not apply.
- **Color is never the sole signal.** The current-item indicator in the language list is a ✓ icon, not just a color shift. The destructive token is paired with the word `fatal:` in the 404 terminal, not just red text. The theme toggle has Sun/Moon/Monitor icons alongside the label.
- **Respect `prefers-reduced-motion`.** Popover animations from `tw-animate-css` honor this by default. If hand-rolling animations, gate them behind `motion-safe:`.
- **Test before merging:** tab through the new UI with the keyboard only; resize to 375px and check nothing but code and diffs scrolls sideways; toggle dark mode; check focus rings are visible against both themes.
