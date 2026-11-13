# Dariomark

A fast, safe Markdown engine for Go that implements CommonMark and GitHub Flavored Markdown.

Dariomark was written from scratch, from the specifications, for the Markdown that people write on code hosting: READMEs, issues, comments, wikis and long documents written by AI agents. It renders that Markdown the way GitHub does, and in Gogs it renders 2 to 7 times faster than the same pipeline built on blackfriday or goldmark.

## Features

- **CommonMark 0.31.2**, passing all 652 examples of the specification.
- **GitHub Flavored Markdown**: tables, strikethrough, task lists and extended autolinks, e.g., bare `www.` links and email addresses.
- **GitHub extensions**: footnotes and alerts (`> [!NOTE]`, `> [!TIP]`, `> [!IMPORTANT]`, `> [!WARNING]` and `> [!CAUTION]`).
- **Heading anchors**: headings get stable ids made the way GitHub makes them, e.g., `user-content-getting-started`, with `-1`, `-2` and so on for repeated headings.
- **Table of contents data**: `Result.Headings` lists every heading with its level, text and id.
- **Editable task lists**: task list items carry their source lines, e.g., `<li class="task-list-item" data-source-lines="5-7">`, so a client can toggle or reorder items, e.g., by drag and drop, by editing those lines of the source.
- **SmartyPants punctuation**: curly quotes, dashes, ellipses and fractions.
- **Built-in sanitizing**: documents without raw HTML are sanitized while they are written, with the same result as [bluemonday](https://github.com/microcosm-cc/bluemonday)'s user content policy.
- **Safe for concurrent use**, with no dependencies beyond the Go standard library and `golang.org/x/net/html`.

## Performance

`markup.Markdown`, the full Gogs pipeline (rendering, special links and sanitizing), in microseconds per document. Lower is better.

| Document | Size | Dariomark | blackfriday v1.6.0 | goldmark v1.8.7 |
|---|---|---|---|---|
| Comment | 59 B | **18** | 35 | 62 |
| README | 4.5 KB | **305** | 883 | 1,023 |
| Cheat sheet | 12 KB | **874** | 2,133 | 2,189 |
| Changelog | 41 KB | **1,110** | 8,029 | 8,214 |
| Documentation | 92 KB | **4,257** | 14,974 | 15,349 |

The engine alone, without the Gogs passes, in microseconds and allocations per document:

| Document | Dariomark | blackfriday v1.6.0 | goldmark v1.8.7 |
|---|---|---|---|
| Comment | **1.4** (8) | 3.7 (15) | 33.1 (280) |
| README | **48.6** (28) | 66.2 (273) | 180.7 (654) |
| Cheat sheet | **133** (81) | 161 (641) | 429 (1,307) |
| Changelog | **522** (271) | 602 (2,432) | 1,596 (3,766) |
| Documentation | **859** (76) | 1,121 (3,652) | 2,546 (6,649) |

The numbers were measured with Go 1.27.4 on a 4-core Intel Xeon at 2.60 GHz, as the median of 5 runs of Go benchmarks. The comment is a one-line issue comment, and the other documents are a project README, a Markdown cheat sheet, a project changelog and a documentation page. The blackfriday and goldmark columns of the first table use the Gogs pipeline built for each library, which tokenizes the HTML again for special links and sanitizes all of it with bluemonday.

## How it is fast

A general-purpose Markdown library only produces HTML. Rendering a comment on a code hosting site takes more passes over that HTML: linking mentions, issue references and commit IDs, wrapping images in links, and sanitizing. With goldmark v1.8.7, those passes take 80% to 83% of the time of `markup.Markdown` on the README, cheat sheet, changelog and documentation above. Dariomark does all of it in one pass over the document tree:

- **Special links** are rendered while text is written, so the HTML is never tokenized again. Text that cannot contain a reference skips the reference patterns entirely.
- **Sanitizing** happens while writing. Dariomark knows every element and attribute it emits, so documents without raw HTML are checked against the allowlist without parsing HTML. Documents with raw HTML go through bluemonday, which is the final pass for anything an author typed as HTML.
- **Memory** is reused: parsers are pooled, nodes point into the source instead of copying it, and the common paths allocate a few times per document instead of a few times per node.

## Correctness

- `TestSpec` runs all 652 examples of the CommonMark 0.31.2 specification.
- Code blocks in list items match GitHub's rendering on 25 cases covering fenced and indented code, nesting, ordered lists, tabs, task items and blockquotes (`TestMarkdown_CodeInListItems` keeps 7 of them).
- Differential testing compared the output with an independent CommonMark and GFM implementation, byte for byte, on real-world READMEs, changelogs and documentation, and on millions of random documents in every Gogs configuration.
- `FuzzRender` checks that rendering never panics and is deterministic.
- The built-in sanitizer was compared with bluemonday on about 2 million random and fuzzed documents, including hostile URLs, entities, control characters and issue tracker formats, with no difference.

## Security and robustness

Markdown on a code hosting site comes from anyone who can open an issue, so one document must never be able to take down the server.

- **Fail-closed sanitizing.** Every element and attribute Dariomark writes goes through one allowlist. Anything it does not recognize sends the document to bluemonday instead of through.
- **Safe URLs.** Link and image URLs are checked by scheme. `javascript:`, `vbscript:` and `data:` URLs other than common image types are dropped, however they are encoded.
- **Bounded nesting.** Containers nested more than 100 levels deep, e.g., `>` repeated, show the deeper markers as text, and the rest of the document renders normally.
- **Document limits in Gogs.** `markup.Render` shows documents larger than `[ui] MAX_DISPLAY_FILE_SIZE` (8 MB by default) as plain text, and documents of 64 KB or more share a budget of concurrent render memory, so a burst of large documents waits instead of exhausting memory.
