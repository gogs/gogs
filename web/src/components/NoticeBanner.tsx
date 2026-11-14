import { TriangleAlert } from "lucide-react";

import { webContext } from "@/lib/context";

// NoticeBanner shows the notice that site admins put in
// "custom/notice/banner.md", e.g., a maintenance window, below the navbar of
// every page. The server renders and sanitizes its Markdown.
export function NoticeBanner() {
  if (!webContext.notice) return null;
  return (
    <div role="note" className="border-b border-(--color-warning)/40 bg-(--color-warning)/10">
      <div className="mx-auto flex max-w-6xl items-start gap-2 px-4 py-2.5 sm:px-6">
        <TriangleAlert className="mt-0.5 size-4 shrink-0 text-(--color-warning)" aria-hidden />
        <div
          className="markdown min-w-0 flex-1 [&_a]:text-(--color-foreground)"
          dangerouslySetInnerHTML={{ __html: webContext.notice }}
        />
      </div>
    </div>
  );
}
