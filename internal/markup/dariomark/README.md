# dariomark

dariomark is the Markdown engine of Gogs. It implements CommonMark 0.31.2 and GitHub Flavored Markdown (tables, strikethrough, task lists and extended autolinks), plus footnotes and GitHub alerts (`> [!NOTE]`, `> [!TIP]`, `> [!IMPORTANT]`, `> [!WARNING]` and `> [!CAUTION]`). It also implements SmartyPants punctuation, including fractions.

Headings get stable ids made the way GitHub makes them, e.g., `user-content-getting-started`, with `-1`, `-2` and so on for repeated headings. `Result.Headings` lists every heading with its level, text and id, which is what a table of contents needs.

It was written from scratch for Gogs, from the specifications, with no code taken from other Markdown implementations.

## Why a Gogs-specific engine

A general-purpose Markdown library only produces HTML. Rendering a comment in Gogs takes three more passes over that HTML: turning mentions, issue references and commit SHAs into links, wrapping images in links to themselves, and sanitizing the result with bluemonday. Before dariomark, those passes took about 90% of the render time.

dariomark does all of it in one pass over the document tree:

- **Special links** are rendered while writing text nodes, so the HTML is never tokenized again. Text that cannot contain a reference skips the reference patterns entirely.
- **Sanitizing** happens while writing. dariomark knows every element and attribute it emits, so documents without raw HTML are checked against the same allowlist as bluemonday without parsing HTML. Documents with raw HTML still go through bluemonday, which stays the final pass for anything an author typed as HTML.
- **Memory** comes from one pooled parser per goroutine. Nodes point into the source instead of copying it, and the common paths allocate a few times per document instead of a few times per node.

## Performance

`markup.Markdown`, the full Gogs pipeline (rendering, special links, sanitizing), in microseconds per document. Lower is better.

| Document | Size | dariomark | blackfriday (`main`) | goldmark |
|---|---|---|---|---|
| Comment | 59 B | **18** | 35 | 62 |
| README | 4.5 KB | **305** | 883 | 1,023 |
| Cheat sheet | 12 KB | **874** | 2,133 | 2,189 |
| Changelog | 41 KB | **1,110** | 8,029 | 8,214 |
| Documentation | 92 KB | **4,257** | 14,974 | 15,349 |

The engine alone, without the Gogs passes, in microseconds and allocations per document:

| Document | dariomark | blackfriday v1 | goldmark |
|---|---|---|---|
| Comment | **1.4** (8) | 3.7 (15) | 33.1 (280) |
| README | **48.6** (28) | 66.2 (273) | 180.7 (654) |
| Cheat sheet | **133** (81) | 161 (641) | 429 (1,307) |
| Changelog | **522** (271) | 602 (2,432) | 1,596 (3,766) |
| Documentation | **859** (76) | 1,121 (3,652) | 2,546 (6,649) |

dariomark is faster than blackfriday while implementing the full CommonMark and GFM specifications, which blackfriday v1 does not. It is three times faster than goldmark with the same feature set.

## Correctness

- All 652 examples of the CommonMark 0.31.2 specification pass (`TestSpec`).
- The output was compared byte for byte with goldmark, which follows the same specifications, on the GFM specification examples, real-world READMEs, changelogs and documentation, and millions of random documents, in every Gogs configuration (typographer settings, hard line breaks, URL prefixes, issue tracker formats). The only differences are documented below.
- `FuzzRender` checks that rendering never panics and is deterministic.
- The fast sanitizer was compared with bluemonday on about 2 million random and fuzzed documents, including hostile URLs, entities, control characters and issue tracker formats, with no difference.

## Robustness

User content is untrusted, and one document must never be able to take down the server. Every part of dariomark runs in time and memory linear in the input. Where a construct could make the output much larger than the input, a limit degrades only that element and the rest of the document renders normally:

- Containers nested more than 100 levels deep, e.g., `>` repeated, show the deeper markers as text.

`markup.Render` adds two limits for the whole document:

- Documents larger than `[ui] MAX_DISPLAY_FILE_SIZE` (8 MB by default) are shown as plain text.
- Documents of 256 KB or more share a budget of concurrent render memory, so a burst of large documents waits instead of exhausting memory.

## Known differences from goldmark

These need unusual input and do not affect real documents:

- A link reference definition whose title starts on the next line and is followed by another definition, e.g., `[a]: /u\n(x(\n[b]: /v`, or `[a]: /u\n't'[b]: /v`.
- Tabs after two or more container markers on one line, e.g., `>- > \ty`.
- Nesting deeper than 100 levels, see above.
