import { WorkerPoolContextProvider } from "@pierre/diffs/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Outlet, RouterProvider, createRootRouteWithContext, createRoute, createRouter } from "@tanstack/react-router";
import { Toaster } from "sonner";

import { Footer } from "@/components/Footer";
import { Navbar } from "@/components/Navbar";
import { TooltipProvider } from "@/components/ui/tooltip";
import { webContext } from "@/lib/context";
import type { UserInfo } from "@/lib/user-info";
import { Landing } from "@/pages/Landing";
import { NotFound } from "@/pages/NotFound";
import { ServerError } from "@/pages/ServerError";
import { createRepoRoutes } from "@/routes/repo";
import { createUserRoutes } from "@/routes/user";

interface RouterContext {
  user: UserInfo | null;
  queryClient: QueryClient;
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

const routeTree = rootRoute.addChildren([landingRoute, ...createUserRoutes(rootRoute), ...createRepoRoutes(rootRoute)]);

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

const queryClient = new QueryClient();

// Pierre's worker pool is a process-wide singleton (refcounted by mount). Hold
// it at the app root so it stays warm across navigations: the diff renderer
// uses workers to tokenize files via Shiki/Oniguruma off the main thread, and
// without a live pool every file falls back to a sync main-thread path that
// returns `undefined` until highlighting resolves, painting blanks on fast
// scroll over large diffs.
const diffWorkerPoolOptions = {
  workerFactory: () => new Worker(new URL("@pierre/diffs/worker/worker.js", import.meta.url), { type: "module" }),
};

export function AppRouter({ user }: { user: UserInfo | null }) {
  const router = makeRouter({ user, queryClient });
  return (
    <QueryClientProvider client={queryClient}>
      <WorkerPoolContextProvider poolOptions={diffWorkerPoolOptions} highlighterOptions={{}}>
        <TooltipProvider delayDuration={300}>
          <RouterProvider router={router} />
          <Toaster position="bottom-right" closeButton richColors />
        </TooltipProvider>
      </WorkerPoolContextProvider>
    </QueryClientProvider>
  );
}
