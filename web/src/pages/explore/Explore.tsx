import { Link, getRouteApi, useNavigate } from "@tanstack/react-router";
import { BookMarked, Building2, Search, User } from "lucide-react";
import type { FormEvent } from "react";
import { useTranslation } from "react-i18next";

import { type PageTab, PageTabs } from "@/components/PageTabs";
import { Pagination } from "@/components/Pagination";
import { RepoListItem, type RepoListItemData } from "@/components/RepoListItem";
import { UserListItem, type UserListItemData } from "@/components/UserListItem";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { usePageTitle } from "@/lib/page-title";

import type { ExploreKind, ExploreSearch } from "./Explore.search";

interface ExploreList<T> {
  items: T[];
  page: number;
  totalPages: number;
}

export type ExplorePage =
  | ({ kind: "repos" } & ExploreList<RepoListItemData>)
  | ({ kind: "users" | "organizations" } & ExploreList<UserListItemData>);

const route = getRouteApi("/explore/$kind");

export function Explore() {
  const { t } = useTranslation();
  const data = route.useLoaderData();
  const search: ExploreSearch = route.useSearch();
  const navigate = useNavigate({ from: "/explore/$kind" });
  usePageTitle(t("explore"));

  const tabs: PageTab<ExploreKind>[] = [
    { key: "repos", label: t("explore.repos"), icon: BookMarked },
    { key: "users", label: t("explore.users"), icon: User },
    { key: "organizations", label: t("explore.organizations"), icon: Building2 },
  ];

  function onSearch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const q = (new FormData(event.currentTarget).get("q") as string | null)?.trim() ?? "";
    void navigate({ search: { q: q || undefined } });
  }

  return (
    <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-6 sm:px-6">
      <h1 className="mb-3 text-xl font-semibold">{t("explore")}</h1>
      <PageTabs
        label={t("explore")}
        tabs={tabs}
        active={data.kind}
        // Only the active tab fits next to the menu at 320px.
        inline={1}
        renderLink={(tab, props) => <Link to="/explore/$kind" params={{ kind: tab.key }} {...props} />}
        className="mb-4 border-b border-(--color-border)"
      />

      <form role="search" onSubmit={onSearch} className="flex gap-2">
        <div className="relative min-w-0 flex-1">
          <Search
            className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-(--color-muted-foreground)"
            aria-hidden
          />
          {/* Remount on navigation so the field always shows the keyword of the current URL. */}
          <Input
            key={`${data.kind}:${search.q ?? ""}`}
            name="q"
            type="search"
            defaultValue={search.q}
            placeholder={`${t("explore.search")}...`}
            aria-label={t("explore.search")}
            className="h-10 pl-9"
          />
        </div>
        <Button type="submit">{t("explore.search")}</Button>
      </form>

      <ul className="mt-4 border-t border-(--color-border)">
        {data.kind === "repos"
          ? data.items.map((repo) => <RepoListItem key={`${repo.owner}/${repo.name}`} repo={repo} showOwner />)
          : data.items.map((user) => (
              <UserListItem key={user.name} user={user} organization={data.kind === "organizations"} />
            ))}
      </ul>

      {data.totalPages > 1 ? (
        <div className="mt-4">
          <Pagination page={data.page} totalPages={data.totalPages} />
        </div>
      ) : null}
    </main>
  );
}
