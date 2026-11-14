import { type AnyRoute, createRoute, notFound, redirect } from "@tanstack/react-router";

import { loaderResponseError } from "@/lib/loader-error";
import { subUrl } from "@/lib/url";
import { Explore, type ExplorePage } from "@/pages/explore/Explore";
import {
  type ExploreKind,
  type ExploreSearch,
  isExploreKind,
  validateExploreSearch,
} from "@/pages/explore/Explore.search";

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
      if (res.status === 401 || res.status === 403) {
        // The viewer is not allowed to see site content (unauthenticated, email
        // not confirmed, or login prohibited). Reload the page so the server
        // sends them to sign in, account activation, or the login prohibited
        // page.
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- TanStack's redirect() returns a sentinel that must be thrown.
        throw redirect({
          to: "/explore/$kind",
          params: { kind: params.kind },
          search: { q: deps.q, page: deps.page },
          reloadDocument: true,
        });
      }
      if (!res.ok) {
        throw await loaderResponseError(res);
      }
      return { kind: params.kind, ...(await res.json()) } as ExplorePage;
    },
    component: Explore,
  });

  return [exploreRoute];
}
