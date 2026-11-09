import { Link, getRouteApi } from "@tanstack/react-router";
import { BookMarked, Building2, User } from "lucide-react";
import { useTranslation } from "react-i18next";

import { type PageTab, PageTabs } from "@/components/PageTabs";
import { Pagination } from "@/components/Pagination";
import { RepoListItem, type RepoListItemData } from "@/components/RepoListItem";
import { SearchForm } from "@/components/SearchForm";
import { UserListItem, type UserListItemData } from "@/components/UserListItem";
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
  usePageTitle(t("explore"));

  const tabs: PageTab<ExploreKind>[] = [
    { key: "repos", label: t("explore.repos"), icon: BookMarked },
    { key: "users", label: t("explore.users"), icon: User },
    { key: "organizations", label: t("explore.organizations"), icon: Building2 },
  ];

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

      <SearchForm key={data.kind} q={search.q} />

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
