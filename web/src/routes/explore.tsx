import { type AnyRoute, createRoute, notFound } from "@tanstack/react-router";

import { loaderResponseError } from "@/lib/loader-error";
import { validateListSearch } from "@/lib/page-search";
import type { CodeSearchData } from "@/lib/queries/repo";
import { subUrl } from "@/lib/url";
import { Explore, type ExplorePage } from "@/pages/explore/Explore";
import {
  type ExploreKind,
  type ExploreSearch,
  isExploreKind,
  validateExploreSearch,
} from "@/pages/explore/Explore.search";
import { CodeSearch } from "@/pages/search/CodeSearch";

export function createExploreRoutes(rootRoute: AnyRoute) {
  const exploreRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/explore/$kind",
    validateSearch: validateExploreSearch,
    params: {
      parse: (raw: { kind: string }) => {
        if (!isExploreKind(raw.kind)) {
          // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
          throw notFound();
        }
        return { kind: raw.kind };
      },
      stringify: (params: { kind: ExploreKind }) => params,
    },
    loaderDeps: ({ search }: { search: ExploreSearch }) => ({ q: search.q, page: search.page }),
    loader: async ({ params, deps }): Promise<ExplorePage> => {
      const query = new URLSearchParams();
      if (deps.q) query.set("q", deps.q);
      if (deps.page) query.set("page", String(deps.page));
      const qs = query.toString();
      const res = await fetch(subUrl(`/api/web/explore/${params.kind}`) + (qs ? `?${qs}` : ""), {
        credentials: "same-origin",
      });
      if (!res.ok) {
        throw await loaderResponseError(res);
      }
      return { kind: params.kind, ...(await res.json()) } as ExplorePage;
    },
    component: Explore,
  });

  const codeSearchRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/search",
    validateSearch: (search: Record<string, unknown>): { q?: string } => ({ q: validateListSearch(search).q }),
    loaderDeps: ({ search }: { search: { q?: string } }) => ({ q: search.q }),
    loader: async ({ deps }): Promise<CodeSearchData> => {
      const qs = deps.q ? `?${new URLSearchParams({ q: deps.q }).toString()}` : "";
      const res = await fetch(subUrl(`/api/web/search/code${qs}`), { credentials: "same-origin" });
      if (!res.ok) {
        throw await loaderResponseError(res);
      }
      return (await res.json()) as CodeSearchData;
    },
    component: CodeSearch,
  });

  return [exploreRoute, codeSearchRoute];
}
