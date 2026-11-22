import type { QueryClient } from "@tanstack/react-query";
import { type AnyRoute, createRoute, notFound, redirect } from "@tanstack/react-router";

import { LoaderResponseError } from "@/lib/loader-error";
import { type PageSearch, validatePageSearch } from "@/lib/page-search";
import { fetchUserPeople, fetchUserRepos, userActivityQuery, userProfileQuery } from "@/lib/queries/user";
import { Profile, type ProfilePage } from "@/pages/user/Profile";
import { type ProfileSearch, validateProfileSearch } from "@/pages/user/Profile.search";

interface RouterContext {
  queryClient: QueryClient;
}

export function createProfileRoutes(rootRoute: AnyRoute) {
  // Serves both individual users and organizations, which the profile tells
  // apart.
  const profileRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$username",
    validateSearch: validateProfileSearch,
    loaderDeps: ({ search }: { search: ProfileSearch }) => ({ tab: search.tab, page: search.page }),
    loader: async ({ params, deps, context }): Promise<ProfilePage> => {
      const routerContext = context as RouterContext;
      try {
        if (deps.tab === undefined) {
          const [, repos] = await Promise.all([
            routerContext.queryClient.ensureQueryData(userProfileQuery(params.username)),
            fetchUserRepos(params.username, deps.page),
          ]);
          return { tab: "repos", repos };
        }
        const profile = await routerContext.queryClient.ensureQueryData(userProfileQuery(params.username));
        if (profile.kind === "organization") {
          // Organizations have no activity, followers or following tabs.
          // eslint-disable-next-line @typescript-eslint/only-throw-error -- TanStack's redirect() returns a sentinel that must be thrown.
          throw redirect({ to: "/$username", params: { username: params.username }, search: {}, replace: true });
        }
        if (deps.tab === "activity") {
          await routerContext.queryClient.ensureInfiniteQueryData(userActivityQuery(params.username));
          return { tab: "activity" };
        }
        return { tab: deps.tab, people: await fetchUserPeople(params.username, deps.tab, deps.page) };
      } catch (err) {
        if (err instanceof LoaderResponseError && err.status === 404) {
          // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
          throw notFound();
        }
        throw err;
      }
    },
    component: Profile,
  });

  // The followers and following lists used to have their own pages, which
  // links from before they became tabs of the profile still lead to.
  const legacyListRoutes = (["followers", "following"] as const).map((tab) =>
    createRoute({
      getParentRoute: () => rootRoute,
      path: `/$username/${tab}`,
      validateSearch: validatePageSearch,
      beforeLoad: ({ params, search }: { params: { username: string }; search: PageSearch }) => {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- TanStack's redirect() returns a sentinel that must be thrown.
        throw redirect({
          to: "/$username",
          params: { username: params.username },
          search: { tab, page: search.page },
          replace: true,
        });
      },
    }),
  );

  return [profileRoute, ...legacyListRoutes];
}
