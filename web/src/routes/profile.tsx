import type { QueryClient } from "@tanstack/react-query";
import { type AnyRoute, createRoute, notFound, redirect } from "@tanstack/react-router";

import { LoaderResponseError } from "@/lib/loader-error";
import { fetchUserRepos, userActivityQuery, userProfileQuery } from "@/lib/queries/user";
import { Profile, type ProfilePage } from "@/pages/user/Profile";
import { type ProfileSearch, validateProfileSearch } from "@/pages/user/Profile.search";

interface RouterContext {
  queryClient: QueryClient;
}

export function createProfileRoutes(rootRoute: AnyRoute) {
  // The server routes organizations to their legacy home page, so only
  // individual users reach this route on a full page load.
  const profileRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$username",
    validateSearch: validateProfileSearch,
    loaderDeps: ({ search }: { search: ProfileSearch }) => ({ tab: search.tab, page: search.page }),
    loader: async ({ params, deps, context }): Promise<ProfilePage> => {
      const routerContext = context as RouterContext;
      try {
        if (deps.tab === "activity") {
          await Promise.all([
            routerContext.queryClient.ensureQueryData(userProfileQuery(params.username)),
            routerContext.queryClient.ensureInfiniteQueryData(userActivityQuery(params.username)),
          ]);
          return { tab: "activity" };
        }
        const [, repos] = await Promise.all([
          routerContext.queryClient.ensureQueryData(userProfileQuery(params.username)),
          fetchUserRepos(params.username, deps.page),
        ]);
        return { tab: "repos", repos };
      } catch (err) {
        if (err instanceof LoaderResponseError && err.status === 404) {
          // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
          throw notFound();
        }
        if (err instanceof LoaderResponseError && (err.status === 401 || err.status === 403)) {
          // The viewer is not allowed to see site content (signed out, email not
          // confirmed, or login prohibited). Reload the page so the server sends
          // them to sign in, account activation, or the login prohibited page.
          // eslint-disable-next-line @typescript-eslint/only-throw-error -- TanStack's redirect() returns a sentinel that must be thrown.
          throw redirect({
            to: "/$username",
            params: { username: params.username },
            search: { tab: deps.tab, page: deps.page },
            reloadDocument: true,
          });
        }
        throw err;
      }
    },
    component: Profile,
  });

  return [profileRoute];
}
