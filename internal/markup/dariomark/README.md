# Dariomark

A fast, safe Markdown engine for Go that implements CommonMark and GitHub Flavored Markdown.

Dariomark was written from scratch, from the specifications, for the Markdown that people write on code hosting: READMEs, issues, comments, wikis and long documents written by AI agents. It renders that Markdown the way GitHub does, and it renders 1.2 to 5.1 times faster than the same pipeline built on blackfriday or goldmark.

## Features

- **CommonMark 0.31.2**, passing the 652 examples of the specification, except 11 where Dariomark renders what GitHub renders.
- **GitHub's rendering where it departs from the specifications**, e.g., in trailing punctuation after autolinks, nested strong emphasis and task list markers.
- **GitHub Flavored Markdown**: tables, strikethrough, task lists and extended autolinks, e.g., bare `www.` links and email addresses.
- **GitHub extensions**: footnotes and alerts (`> [!NOTE]`, `> [!TIP]`, `> [!IMPORTANT]`, `> [!WARNING]` and `> [!CAUTION]`).
- **Emoji shortcodes**: with a table of names, e.g., GitHub's, shortcodes like `:tada:` render as 🎉 wherever GitHub replaces them, e.g., not in code or in `10:30:45 :tada:`.
- **Heading anchors**: headings get stable ids made the way GitHub makes them, e.g., `user-content-getting-started`, with `-1`, `-2` and so on for repeated headings.
- **Table of contents data**: `Result.Headings` lists every heading with its level, text and id.
- **YAML front matter**: with `Options.FrontMatter`, front matter at the start of a file renders as a table above the document, with nested tables for nested mappings and lists. Invalid YAML renders as a code block, and YAML that is not a mapping renders as Markdown.
- **Editable task lists**: task list items carry their source lines, e.g., `<li class="task-list-item" data-source-lines="5-7">`, so a client can toggle or reorder items, e.g., by drag and drop, by editing those lines of the source.
- **SmartyPants punctuation**: curly quotes, dashes, ellipses and fractions.
- **HTML sanitizer**: `Policy` sanitizes HTML of user content with an allowlist, the same output as bluemonday's UGC policy plus rules for the HTML that Dariomark produces, 4 to 5 times faster than bluemonday.
- **Built-in sanitizing**: documents without raw HTML are sanitized while they are written, with the same result as `Policy`.
- **Safe for concurrent use**: `Render` can be called from any number of goroutines at once.

## Performance

`markup.Markdown`, the full pipeline (rendering, special links and sanitizing), in microseconds per document. Lower is better.

| Document | Size | Dariomark | blackfriday v1.6.0 | goldmark v1.8.7 |
|---|---|---|---|---|
| Project README | 1.4 KB | **55** | 146 | 258 |
| Code review comment | 5.6 KB | **120** | 370 | 569 |
| Answer with code | 6.1 KB | **94** | 338 | 477 |
| Specification | 40 KB | **882** | 2,156 | 2,904 |
| GFM specification | 216 KB | **10,004** | 11,786 | 14,504 |

The engine alone, without special links and sanitizing, in microseconds and allocations per document:

| Document | Dariomark | blackfriday v1.6.0 | goldmark v1.8.7 |
|---|---|---|---|
| Project README | **25.2** (18) | 36.0 (165) | 119 (327) |
| Code review comment | **43.8** (12) | 92.7 (262) | 267 (686) |
| Answer with code | **33.2** (9) | 96.2 (254) | 220 (539) |
| Specification | **321** (115) | 611 (2,092) | 1,473 (4,386) |
| GFM specification | **1,602** (469) | 3,077 (8,950) | 6,800 (16,955) |

The numbers were measured with Go 1.27.4 on a 16-core Intel Xeon at 2.60 GHz, as the median of 5 runs of Go benchmarks. The documents are a short project README, a code review comment and an answer with code examples written by an AI agent, the WebAssembly SIMD specification, and the GFM specification. The blackfriday and goldmark columns of the first table use the same pipeline built for each library: the HTML post-processing of `internal/markup` for special links, then `Policy`. The GFM specification has a lot of raw HTML, which Dariomark cannot sanitize while rendering, so it goes through the same post-processing and sanitizer, and gains the least.

## How it is fast

A general-purpose Markdown library only produces HTML. Rendering a comment on a code hosting site takes more passes over that HTML: linking mentions, issue references and commit IDs, wrapping images in links, and sanitizing. With goldmark v1.8.7, those passes take about half of the time of the pipeline on the documents above. Dariomark does all of it in one pass over the document tree:

- **Special links** are rendered while text is written, so the HTML is never tokenized again. Text that cannot contain a reference skips the reference patterns entirely.
- **Sanitizing** happens while writing. Dariomark knows every element and attribute it emits, so documents without raw HTML are checked against the allowlist without parsing HTML. Documents with raw HTML go through `Policy`, Dariomark's HTML sanitizer, which is the final pass for anything an author typed as HTML. It tokenizes the HTML in place, without copying it, and allocates a few times per document.
- **Common blocks skip the general inline parser.** Plain paragraphs, headings, table cells and task items, simple emphasis, inline links and extended autolinks are written while the text is scanned, without building inline nodes. Text is scanned and escaped several bytes at a time.
- **Memory** is reused: parsers are pooled, nodes point into the source instead of copying it, and the common paths allocate a few times per document instead of a few times per node.

## Correctness

- `TestSpec` runs all 652 examples of the CommonMark 0.31.2 specification, and checks GitHub's rendering for the 11 where GitHub departs from it.
- Code blocks in list items match GitHub's rendering on 25 cases covering fenced and indented code, nesting, ordered lists, tabs, task items and blockquotes (`TestMarkdown_CodeInListItems` keeps 7 of them).
- `TestGFMSpec` runs the examples of the GFM extensions: tables, task lists, strikethrough and autolinks.
- Differential testing compared the output with cmark-gfm and commonmark.js, byte for byte, on real-world READMEs, changelogs and documentation, and on millions of random documents in every configuration that `markup` uses. Where GitHub's rendering and the specifications disagree, Dariomark follows GitHub. The remaining differences are rare edge cases, e.g., leading whitespace on lazy continuation lines after a backslash line break.
- `FuzzRender` checks that rendering never panics and is deterministic.
- The built-in sanitizer was compared with bluemonday on about 2 million random and fuzzed documents, including hostile URLs, entities, control characters and issue tracker formats, with no difference.
- `Policy` replaced bluemonday after producing the same output on 18,705 documents, including the OWASP, PayloadsAllTheThings and SecLists XSS lists, and on about 40 million fuzzed documents. It differs only in always checking `data:` URIs, even when `[markdown] CUSTOM_URL_SCHEMES` lists `data`.
- `FuzzMarkdown` in `internal/markup` keeps checking the built-in sanitizer against `Policy`: run `go test -fuzz FuzzMarkdown -timeout 0 ./internal/markup`, and commit the failing inputs it writes to `testdata/fuzz` with the fix, so that `go test` replays them.

## Security and robustness

Markdown on a code hosting site comes from anyone who can open an issue, so one document must never be able to take down the server.

- **Fail-closed sanitizing.** Every element and attribute Dariomark writes goes through one allowlist. Anything it does not recognize sends the document to `Policy` instead of through.
- **Safe URLs.** Link and image URLs are checked by scheme. `javascript:`, `vbscript:` and `data:` URLs other than common image types are dropped, however they are encoded.
- **Bounded front matter.** Front matter must end within the first 64 KB of a file and nest at most 16 levels deep, and YAML aliases are rejected, so alias bombs like "billion laughs" cannot multiply the output.
- **Bounded nesting.** Containers nested more than 100 levels deep, e.g., `>` repeated, show the deeper markers as text, and the rest of the document renders normally.
- **Document limits.** `markup.Render` shows documents larger than `[ui] MAX_DISPLAY_FILE_SIZE` (8 MB by default) as plain text, and documents of 64 KB or more share a budget of concurrent render memory, so a burst of large documents waits instead of exhausting memory.
