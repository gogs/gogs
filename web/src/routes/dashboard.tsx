import type { QueryClient } from "@tanstack/react-query";
import { type AnyRoute, createRoute, notFound, redirect } from "@tanstack/react-router";

import { LoaderResponseError } from "@/lib/loader-error";
import { dashboardActivityQuery, dashboardQuery } from "@/lib/queries/user";
import { useUserInfo } from "@/lib/use-user-info";
import type { UserInfo } from "@/lib/user-info";
import { Landing } from "@/pages/Landing";
import { Dashboard } from "@/pages/user/Dashboard";

interface RouterContext {
  user: UserInfo | null;
  queryClient: QueryClient;
}

async function loadDashboard(queryClient: QueryClient, org: string | undefined, reload: () => unknown) {
  try {
    await Promise.all([
      queryClient.ensureQueryData(dashboardQuery(org)),
      queryClient.ensureInfiniteQueryData(dashboardActivityQuery(org)),
    ]);
  } catch (err) {
    if (err instanceof LoaderResponseError && err.status === 404) {
      // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
      throw notFound();
    }
    if (err instanceof LoaderResponseError && (err.status === 401 || err.status === 403)) {
      // The viewer is signed out, has not confirmed their email, or is
      // prohibited from signing in. Reload the page so the server sends them to
      // sign in, account activation, or the login prohibited page.
      throw reload();
    }
    throw err;
  }
}

export function createDashboardRoutes(rootRoute: AnyRoute) {
  const homeRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/",
    loader: async ({ context }) => {
      const { user, queryClient } = context as RouterContext;
      if (!user) return;
      await loadDashboard(queryClient, undefined, () => redirect({ to: "/", reloadDocument: true }));
    },
    // Visitors get the landing page, signed-in users their dashboard.
    component: function Home() {
      return useUserInfo() ? <Dashboard /> : <Landing />;
    },
  });

  const orgDashboardRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/org/$org/dashboard",
    loader: async ({ params, context }) => {
      const { queryClient } = context as RouterContext;
      await loadDashboard(queryClient, params.org, () =>
        redirect({ to: "/org/$org/dashboard", params: { org: params.org }, reloadDocument: true }),
      );
    },
    component: function OrgDashboard() {
      const { org } = orgDashboardRoute.useParams();
      // Remount when switching organizations so the list tab starts over.
      return <Dashboard key={org} org={org} />;
    },
  });

  return [homeRoute, orgDashboardRoute];
}
