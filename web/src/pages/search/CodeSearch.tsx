import { Link, getRouteApi, useNavigate } from "@tanstack/react-router";
import { TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";

import { CodeSearchForm, CodeSearchHelp, CodeSearchResults } from "@/components/CodeSearchResults";
import { usePageTitle } from "@/lib/page-title";
import { refPath } from "@/lib/queries/repo";
import { subUrl } from "@/lib/url";

const route = getRouteApi("/search");

// CodeSearch searches the default branches of all repositories the viewer can
// read, narrowed down by qualifiers such as org: and repo:.
export function CodeSearch() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const data = route.useLoaderData();
  usePageTitle(`${data.query ? `${data.query} - ` : ""}${t("repo.code_search")}`);

  const matchCount = data.files.reduce((n, f) => n + f.lines.filter((l) => l.isMatch).length, 0);

  return (
    <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
      <h1 className="mb-3 text-xl font-semibold">{t("repo.code_search")}</h1>
      <div className="mb-5 flex">
        <CodeSearchForm
          key={data.query}
          query={data.query}
          autoFocus={!data.query}
          global
          onSearch={(q) => void navigate({ to: "/search", search: { q: q || undefined } })}
        />
      </div>

      {!data.keyword ? (
        <CodeSearchHelp hint={t("repo.code_search.global_hint")} global />
      ) : (
        <>
          {data.timedOut ? (
            <p className="mb-4 flex items-center gap-2 rounded-md border border-(--color-warning) bg-(--color-warning)/10 px-4 py-2 text-sm">
              <TriangleAlert className="size-4 shrink-0 text-(--color-warning)" aria-hidden />
              {t("repo.code_search.timed_out_partial")}
            </p>
          ) : null}
          {data.files.length ? (
            <CodeSearchResults
              files={data.files}
              keyword={data.keyword}
              urlOf={(file, line) =>
                subUrl(`/${file.repo}/src/${refPath(`${file.ref}/${file.path}`)}${line ? `#L${line}` : ""}`)
              }
              headerOf={(file) => {
                const [owner, name] = (file.repo ?? "").split("/");
                return (
                  <>
                    <Link
                      to="/$owner/$repo"
                      params={{ owner, repo: name }}
                      className="font-semibold text-(--color-foreground) hover:underline"
                    >
                      {file.repo}
                    </Link>
                    <span className="mx-1.5 text-(--color-muted-foreground)" aria-hidden>
                      /
                    </span>
                  </>
                );
              }}
            />
          ) : (
            <p className="rounded-md border border-(--color-border) px-4 py-10 text-center text-sm text-(--color-muted-foreground)">
              {t("repo.code_search.no_results")}
            </p>
          )}
          {data.truncated ? (
            <p className="mt-4 text-center text-sm text-(--color-muted-foreground)">
              {t("repo.code_search.truncated", { count: matchCount })}
            </p>
          ) : null}
        </>
      )}
    </main>
  );
}
