import { Outlet, RouterProvider, createRootRouteWithContext, createRoute, createRouter } from "@tanstack/react-router";

import { Footer } from "@/components/Footer";
import { Navbar } from "@/components/Navbar";
import { TooltipProvider } from "@/components/ui/tooltip";
import { webContext } from "@/lib/context";
import { loaderResponseError } from "@/lib/loader-error";
import { subUrl } from "@/lib/url";
import type { UserInfo } from "@/lib/user-info";
import { CommitDiff, type CommitDiffPage } from "@/pages/CommitDiff";
import { validateCommitDiffSearch } from "@/pages/CommitDiff.search";
import { DiffSpike } from "@/pages/DiffSpike";
import { Landing } from "@/pages/Landing";
import { NotFound } from "@/pages/NotFound";
import { ServerError } from "@/pages/ServerError";
import { createUserRoutes } from "@/routes/user";

interface RouterContext {
  user: UserInfo | null;
}

function RootLayout() {
  return (
    <div className="flex min-h-dvh flex-col">
      <Navbar />
      <Outlet />
      <Footer />
    </div>
  );
}

const rootRoute = createRootRouteWithContext<RouterContext>()({
  component: RootLayout,
  notFoundComponent: NotFound,
});

const landingRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: Landing,
});

const diffSpikeRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/_diff-spike",
  component: DiffSpike,
});

const commitDiffRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/$owner/$repo/_diff/$sha",
  validateSearch: validateCommitDiffSearch,
  loaderDeps: ({ search }) => ({ whitespace: search.whitespace }),
  loader: async ({ params, deps }): Promise<CommitDiffPage> => {
    const base = subUrl(`/${params.owner}/${params.repo}/_api/diff/${params.sha}`);
    const url = deps.whitespace ? `${base}?whitespace=${encodeURIComponent(deps.whitespace)}` : base;
    const res = await fetch(url, { credentials: "same-origin" });
    if (!res.ok) {
      throw await loaderResponseError(res);
    }
    return (await res.json()) as CommitDiffPage;
  },
  component: CommitDiff,
});

const routeTree = rootRoute.addChildren([
  landingRoute,
  ...createUserRoutes(rootRoute),
  diffSpikeRoute,
  commitDiffRoute,
]);

function makeRouter(context: RouterContext) {
  return createRouter({
    routeTree,
    basepath: webContext.subURL || "/",
    defaultNotFoundComponent: NotFound,
    defaultErrorComponent: ServerError,
    context,
  });
}

type AppRouterInstance = ReturnType<typeof makeRouter>;

declare module "@tanstack/react-router" {
  interface Register {
    router: AppRouterInstance;
  }
}

export function AppRouter({ user }: { user: UserInfo | null }) {
  const router = makeRouter({ user });
  return (
    <TooltipProvider delayDuration={300}>
      <RouterProvider router={router} />
    </TooltipProvider>
  );
}
