import { parsePatchFiles } from "@pierre/diffs";
import { CodeView, type CodeViewHandle, type CodeViewItem } from "@pierre/diffs/react";
import { useLoaderData } from "@tanstack/react-router";
import { useMemo, useRef } from "react";

import { DiffSearch } from "@/components/DiffSearch";
import { useTheme } from "@/lib/theme-context";

export interface CommitDiffSignature {
  name: string;
  email: string;
  avatarUrl: string;
  userPath?: string;
  when: string;
}

export interface CommitDiffPage {
  commit: {
    sha: string;
    shortSha: string;
    summary: string;
    message: string;
    author: CommitDiffSignature;
    committer: CommitDiffSignature;
    parents: string[];
  };
  patch: string;
  sourcePath: string;
  rawDiffUrl: string;
}

function formatWhen(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString();
}

// Pierre's default file header sits on the same buffer tint as the surrounding
// padding, so 200+ headers blur together. Give each header an opaque, slightly
// stronger tint plus a bottom hairline so they read as distinct cards. The
// background must be opaque or sticky headers bleed through scrolling content.
//
// The "N unmodified lines" metadata separator ships with `inset-inline: 100%
// auto` so it spans only as wide as a downstream column boundary forces it.
// Override to span the full row width, matching the file header above it.
const DIFF_UNSAFE_CSS = `
  [data-diffs-header] {
    background: color-mix(in lab, var(--diffs-bg) 96%, var(--diffs-mixer));
    border-top: 1px solid var(--color-border);
    border-left: 1px solid var(--color-border);
    border-right: 1px solid var(--color-border);
    border-bottom: 1px solid color-mix(in lab, var(--diffs-bg) 85%, var(--diffs-mixer));
    /* Negative top/side margins pull the header up and out by 1px so its
       top/side borders overlay the card's borders pixel-perfectly in docked
       state (no doubled lines). In sticky state the header detaches and
       carries its own visible top edge with side borders intact. */
    margin: -1px -1px 0;
  }
  /* The library's <pre data-diff> wraps the actual diff body. Round its
     bottom corners so the body bg doesn't paint a square edge through the
     host's rounded bottom corners. */
  [data-diff] {
    border-bottom-left-radius: 3px;
    border-bottom-right-radius: 3px;
    overflow: hidden;
  }
  /* The "N unmodified lines" strip (line-info separator) lives inside the
     gutter column. The library paints the small wrapper but leaves the gutter
     cell behind it unpainted, creating a notch. Paint the gutter cell itself
     for the separator row. */
  [data-separator=line-info],
  [data-separator=line-info-basic] {
    background-color: var(--diffs-bg-separator) !important;
  }
`;

export function CommitDiff() {
  const data = useLoaderData({ from: "/$owner/$repo/_diff/$sha" });
  const { theme } = useTheme();
  const viewRef = useRef<CodeViewHandle<undefined> | null>(null);

  const items = useMemo<CodeViewItem[]>(
    () =>
      parsePatchFiles(data.patch).flatMap((parsed, patchIndex) =>
        parsed.files.map<CodeViewItem>((fileDiff, fileIndex) => ({
          id: `${patchIndex}:${fileIndex}:${fileDiff.name}`,
          type: "diff",
          fileDiff,
        })),
      ),
    [data.patch],
  );

  const { commit } = data;
  const authorLabel = commit.author.userPath ? (
    <a href={commit.author.userPath} className="font-medium hover:underline">
      {commit.author.name}
    </a>
  ) : (
    <span className="font-medium">{commit.author.name}</span>
  );

  return (
    <main className="mx-auto flex h-dvh w-full max-w-6xl flex-col px-4 py-8">
      <header className="mb-6 shrink-0 border-b border-(--color-border) pb-4">
        <h1 className="text-lg font-medium break-words">{commit.summary}</h1>
        {commit.message.trim() ? (
          <pre className="mt-2 text-sm whitespace-pre-wrap text-(--color-muted-foreground)">
            {commit.message.trim()}
          </pre>
        ) : null}
        <div className="mt-3 flex flex-wrap items-center gap-2 text-sm text-(--color-muted-foreground)">
          <img src={commit.author.avatarUrl} alt="" className="size-5 rounded-full" />
          {authorLabel}
          <span>committed</span>
          <time dateTime={commit.author.when}>{formatWhen(commit.author.when)}</time>
          <span aria-hidden>·</span>
          <code className="font-mono text-xs">{commit.shortSha}</code>
        </div>
      </header>

      <div className="relative min-h-0 flex-1">
        <DiffSearch items={items} viewRef={viewRef} />
        <CodeView
          ref={viewRef}
          items={items}
          className="h-full overflow-auto"
          options={{
            theme: { light: "pierre-light", dark: "pierre-dark" },
            themeType: theme,
            diffStyle: "unified",
            stickyHeaders: true,
            unsafeCSS: DIFF_UNSAFE_CSS,
          }}
        />
      </div>
    </main>
  );
}
