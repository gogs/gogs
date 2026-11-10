import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Lock } from "lucide-react";
import { useTranslation } from "react-i18next";

import { RepoHeader } from "@/components/RepoHeader";
import { usePageTitle } from "@/lib/page-title";
import { type RepoFork, repoHeaderQuery } from "@/lib/queries/repo";
import { subUrl } from "@/lib/url";

// RepoForks lists the forks of a repository that the viewer can see.
export function RepoForks({ owner, name, forks }: { owner: string; name: string; forks: RepoFork[] }) {
  const { t } = useTranslation();
  const { data: repo } = useSuspenseQuery(repoHeaderQuery(owner, name));
  usePageTitle(`${t("repo.forks")} - ${repo.owner}/${repo.name}`);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repo} />
      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
        <h1 className="flex items-center gap-2 border-b border-(--color-border) pb-2 text-xl font-semibold">
          {t("repo.forks")}
          <span className="rounded-full bg-(--color-secondary) px-2 text-xs/5 font-medium tabular-nums text-(--color-secondary-foreground)">
            {forks.length}
          </span>
        </h1>
        {forks.length ? (
          <ul>
            {forks.map((fork) => (
              <li
                key={`${fork.owner}/${fork.name}`}
                className="flex items-center gap-3 border-b border-(--color-border) py-3"
              >
                <img
                  src={fork.ownerAvatarURL}
                  alt=""
                  className="size-8 shrink-0 rounded-full border border-(--color-border) object-cover"
                />
                <p className="min-w-0 wrap-anywhere">
                  <Link
                    to="/$username"
                    params={{ username: fork.owner }}
                    className="text-(--color-primary) hover:underline"
                  >
                    {fork.owner}
                  </Link>
                  <span className="text-(--color-muted-foreground)"> / </span>
                  {/* The home page of a repository is still served by the server-rendered pages. */}
                  <a
                    href={subUrl(`/${fork.owner}/${fork.name}`)}
                    className="font-semibold text-(--color-primary) hover:underline"
                  >
                    {fork.name}
                  </a>
                </p>
                {fork.isPrivate ? (
                  <span className="inline-flex shrink-0 items-center gap-1 rounded-full border border-(--color-border) px-2 text-xs/5 text-(--color-muted-foreground)">
                    <Lock className="size-3" aria-hidden />
                    {t("admin.repos.private")}
                  </span>
                ) : null}
              </li>
            ))}
          </ul>
        ) : (
          <p className="py-8 text-center text-sm text-(--color-muted-foreground)">{t("repo.pulls.no_results")}</p>
        )}
      </main>
    </div>
  );
}
