import { useSuspenseQuery } from "@tanstack/react-query";
import { useLoaderData, useParams, useSearch } from "@tanstack/react-router";
import { Check, Copy, FileCode2 } from "lucide-react";
import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { DiffWorkspace } from "@/components/DiffWorkspace";
import { RepoHeader } from "@/components/RepoHeader";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { repoHeaderQuery } from "@/lib/queries/repo";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";
import { subUrl } from "@/lib/url";

import type { RepoCommitSearch } from "./Commit.search";

export interface RepoCommitSignature {
  name: string;
  email: string;
  avatarURL: string;
  profileURL?: string;
  when: string;
}

export interface RepoCommitPage {
  sha: string;
  subject: string;
  body: string;
  author: RepoCommitSignature;
  parents: string[];
  patch: string;
}

const BODY_CLAMP_LINES = 3;

function CommitBody({ body }: { body: string }) {
  const { t } = useTranslation();
  const lines = body.split("\n");
  const needsClamp = lines.length > BODY_CLAMP_LINES;
  const [expanded, setExpanded] = useState(!needsClamp);
  const visible = expanded ? body : lines.slice(0, BODY_CLAMP_LINES).join("\n");

  return (
    <div className="mt-2 max-w-3xl">
      <pre className="text-sm whitespace-pre-wrap text-(--color-muted-foreground)">{visible}</pre>
      {needsClamp ? (
        <button
          type="button"
          onClick={() => setExpanded((e) => !e)}
          className="mt-1.5 cursor-pointer rounded border border-(--color-border) bg-(--color-surface) px-2 py-0.5 text-xs text-(--color-muted-foreground) hover:bg-(--color-surface)/80 hover:text-(--color-foreground)"
        >
          {expanded ? t("show_less") : t("show_more")}
        </button>
      ) : null}
    </div>
  );
}

export function RepoCommit() {
  const data = useLoaderData({ from: "/$owner/$repo/commit/$sha" });
  const { sha, subject, body, author, parents, patch } = data;
  const { owner, repo } = useParams({ from: "/$owner/$repo/commit/$sha" });
  const search: RepoCommitSearch = useSearch({ from: "/$owner/$repo/commit/$sha" });
  const { data: repoHeader } = useSuspenseQuery(repoHeaderQuery(owner, repo));
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);

  const authorLabel = author.profileURL ? (
    <a href={author.profileURL} className="font-semibold text-(--color-foreground) hover:underline">
      {author.name}
    </a>
  ) : (
    <span className="font-semibold text-(--color-foreground)">{author.name}</span>
  );

  const repoLink = subUrl(`/${owner}/${repo}`);
  const browseFilesHref = `${repoLink}/src/${sha}`;

  const copySha = useCallback(() => {
    void (async () => {
      try {
        await navigator.clipboard.writeText(sha);
        setCopied(true);
        window.setTimeout(() => setCopied(false), 1500);
      } catch {
        toast.error(t("copy_failed"));
      }
    })();
  }, [sha, t]);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repoHeader} activeTab="code" />

      <section className="mx-auto w-full max-w-7xl px-4 pt-6 pb-4 sm:px-6">
        <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
          <h2 className="min-w-0 flex-1 text-xl font-semibold break-words text-(--color-foreground)">{subject}</h2>
          <div className="basis-full sm:basis-auto">
            <a
              href={browseFilesHref}
              className="inline-flex h-7 shrink-0 items-center gap-1 rounded-md border border-(--color-border) px-2 text-sm hover:bg-(--color-surface)"
            >
              <FileCode2 className="size-3.5" aria-hidden />
              <span>{t("repo.browse_files")}</span>
            </a>
          </div>
        </div>
        {(body ?? "").trim() ? <CommitBody body={(body ?? "").trim()} /> : null}

        <div className="mt-4 flex flex-wrap items-center gap-x-3 gap-y-2 text-sm text-(--color-muted-foreground)">
          <span className="inline-flex items-center gap-1.5">
            <img src={author.avatarURL} alt="" className="size-6 rounded-full" />
            {authorLabel}
            <span>{t("repo.authored")}</span>
            <Tooltip>
              <TooltipTrigger asChild>
                <time dateTime={author.when}>{formatRelativeTime(t, author.when)}</time>
              </TooltipTrigger>
              <TooltipContent>{formatAbsoluteTime(author.when)}</TooltipContent>
            </Tooltip>
          </span>
          {/* TODO: Verified pill once the backend exposes signature verification. */}

          <span aria-hidden className="hidden h-4 w-px bg-(--color-border) sm:inline-block" />

          {/* Wraps between groups on narrow screens, keeping each label with its value. */}
          <span className="inline-flex flex-wrap items-center gap-x-1 gap-y-1.5 font-mono text-xs">
            <a href={subUrl(`/${owner}/${repo}/commit/${sha}.patch`)} className="hover:underline">
              {t("repo.patch_label")}
            </a>
            <span aria-hidden>·</span>
            <a href={subUrl(`/${owner}/${repo}/commit/${sha}.diff`)} className="hover:underline">
              {t("repo.diff_label")}
            </a>
            {parents.length > 0 ? (
              <>
                <span aria-hidden>·</span>
                <span className="inline-flex items-center gap-1">
                  <span>{parents.length > 1 ? `${parents.length} ${t("repo.parents")}` : t("repo.commit_parent")}</span>
                  {parents.map((p) => (
                    <a
                      key={p}
                      href={`${repoLink}/commit/${p}`}
                      className="rounded bg-(--color-surface) px-1.5 py-0.5 text-(--color-foreground) hover:underline"
                    >
                      {p.slice(0, 7)}
                    </a>
                  ))}
                </span>
              </>
            ) : null}
            <span aria-hidden>·</span>
            <span className="inline-flex items-center gap-1">
              <span>{t("repo.commit_label")}</span>
              <code className="rounded bg-(--color-surface) px-1.5 py-0.5 text-(--color-foreground)">
                {sha.slice(0, 10)}
              </code>
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type="button"
                    onClick={copySha}
                    aria-label={t("repo.copy_full_sha")}
                    className="grid size-6 cursor-pointer place-items-center rounded hover:bg-(--color-surface)"
                  >
                    {copied ? (
                      <Check className="size-3.5 text-(--color-success)" aria-hidden />
                    ) : (
                      <Copy className="size-3.5" aria-hidden />
                    )}
                  </button>
                </TooltipTrigger>
                <TooltipContent>{t("repo.copy_full_sha")}</TooltipContent>
              </Tooltip>
            </span>
          </span>
        </div>
      </section>

      <DiffWorkspace owner={owner} repo={repo} patch={patch} oldRef={parents[0]} newRef={sha} search={search} />
    </div>
  );
}
