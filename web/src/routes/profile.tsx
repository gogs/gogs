import type { QueryClient } from "@tanstack/react-query";
import { type AnyRoute, createRoute, notFound, redirect } from "@tanstack/react-router";

import { LoaderResponseError } from "@/lib/loader-error";
import { type PageSearch, validatePageSearch } from "@/lib/page-search";
import { fetchUserPeople, fetchUserRepos, userActivityQuery, userProfileQuery } from "@/lib/queries/user";
import { Follows } from "@/pages/user/Follows";
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
        if (deps.tab === "activity") {
          const profile = await routerContext.queryClient.ensureQueryData(userProfileQuery(params.username));
          if (profile.kind === "organization") {
            // Organizations have no activity tab, like the server-rendered page.
            // eslint-disable-next-line @typescript-eslint/only-throw-error -- TanStack's redirect() returns a sentinel that must be thrown.
            throw redirect({ to: "/$username", params: { username: params.username }, search: {}, replace: true });
          }
          await routerContext.queryClient.ensureInfiniteQueryData(userActivityQuery(params.username));
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
          // The viewer is not allowed to see site content (unauthenticated, email
          // not confirmed, or login prohibited). Reload the page so the server
          // sends them to sign in, account activation, or the login prohibited
          // page.
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

  async function loadFollows(
    queryClient: QueryClient,
    username: string,
    list: "followers" | "following",
    page: number | undefined,
  ) {
    try {
      const [, data] = await Promise.all([
        queryClient.ensureQueryData(userProfileQuery(username)),
        fetchUserPeople(username, list, page),
      ]);
      return data;
    } catch (err) {
      // Organizations have no followers, and the viewer may not be allowed to
      // see site content. Handle both like the profile.
      if (err instanceof LoaderResponseError && err.status === 404) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
        throw notFound();
      }
      if (err instanceof LoaderResponseError && (err.status === 401 || err.status === 403)) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- TanStack's redirect() returns a sentinel that must be thrown.
        throw redirect({ href: window.location.href, reloadDocument: true });
      }
      throw err;
    }
  }

  const followersRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$username/followers",
    validateSearch: validatePageSearch,
    loaderDeps: ({ search }: { search: PageSearch }) => ({ page: search.page }),
    loader: ({ params, deps, context }) =>
      loadFollows((context as RouterContext).queryClient, params.username, "followers", deps.page),
    component: function Followers() {
      const { username } = followersRoute.useParams();
      return <Follows username={username} list="followers" data={followersRoute.useLoaderData()} />;
    },
  });

  const followingRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$username/following",
    validateSearch: validatePageSearch,
    loaderDeps: ({ search }: { search: PageSearch }) => ({ page: search.page }),
    loader: ({ params, deps, context }) =>
      loadFollows((context as RouterContext).queryClient, params.username, "following", deps.page),
    component: function Following() {
      const { username } = followingRoute.useParams();
      return <Follows username={username} list="following" data={followingRoute.useLoaderData()} />;
    },
  });

  return [profileRoute, followersRoute, followingRoute];
}
