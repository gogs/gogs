import { canUseWASM } from "@pierre/diffs";
import { WorkerPoolContextProvider } from "@pierre/diffs/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  Outlet,
  RouterProvider,
  createRootRouteWithContext,
  createRouter,
  parseSearchWith,
  stringifySearchWith,
} from "@tanstack/react-router";
import { Toaster } from "sonner";

import { Footer } from "@/components/Footer";
import { Navbar } from "@/components/Navbar";
import { TooltipProvider } from "@/components/ui/tooltip";
import { webContext } from "@/lib/context";
import type { UserInfo } from "@/lib/user-info";
import { NotFound } from "@/pages/NotFound";
import { ServerError } from "@/pages/ServerError";
import { createDashboardRoutes } from "@/routes/dashboard";
import { createExploreRoutes } from "@/routes/explore";
import { createOrgRoutes } from "@/routes/org";
import { createProfileRoutes } from "@/routes/profile";
import { createRepoRoutes } from "@/routes/repo";
import { createSettingsRoutes } from "@/routes/settings";
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

const routeTree = rootRoute.addChildren([
  ...createDashboardRoutes(rootRoute),
  ...createUserRoutes(rootRoute),
  ...createSettingsRoutes(rootRoute),
  ...createExploreRoutes(rootRoute),
  ...createOrgRoutes(rootRoute),
  ...createRepoRoutes(rootRoute),
  ...createProfileRoutes(rootRoute),
]);

// Search values become numbers, booleans, or null only when that keeps their
// exact text. Everything else, including `?q=1.0`, `?q="foo"` and `?q=[1]`,
// stays the string from the URL. Throwing tells the router to keep the raw
// string. Strings are written to the URL as they are.
function parseSearchValue(value: string): unknown {
  const parsed: unknown = JSON.parse(value);
  const primitive = parsed === null || typeof parsed === "number" || typeof parsed === "boolean";
  if (!primitive || JSON.stringify(parsed) !== value) throw new Error("search value is kept as text");
  return parsed;
}

function makeRouter(context: RouterContext) {
  return createRouter({
    routeTree,
    basepath: webContext.subURL || "/",
    defaultNotFoundComponent: NotFound,
    defaultErrorComponent: ServerError,
    parseSearch: parseSearchWith(parseSearchValue),
    stringifySearch: stringifySearchWith(JSON.stringify),
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
// uses workers to tokenize files off the main thread, and
// without a live pool every file falls back to a sync main-thread path that
// returns `undefined` until highlighting resolves, painting blanks on fast
// scroll over large diffs.
const diffWorkerPoolOptions = {
  workerFactory: () => new Worker(new URL("@pierre/diffs/worker/worker.js", import.meta.url), { type: "module" }),
};

// Highlights tokenizes with WebAssembly SIMD. Browsers that lack it or block
// WebAssembly (e.g., a CSP without `wasm-unsafe-eval`) use Shiki's JavaScript
// regex engine instead.
const diffHighlighterOptions = (() => {
  if (canUseWASM()) {
    return { preferredHighlighter: "highlights" as const };
  }
  console.log("WebAssembly SIMD is unavailable, falling back to Shiki for diff highlighting.");
  return { preferredHighlighter: "shiki-js" as const };
})();

export function AppRouter({ user }: { user: UserInfo | null }) {
  const router = makeRouter({ user, queryClient });
  return (
    <QueryClientProvider client={queryClient}>
      <WorkerPoolContextProvider poolOptions={diffWorkerPoolOptions} highlighterOptions={diffHighlighterOptions}>
        <TooltipProvider delayDuration={300}>
          <RouterProvider router={router} />
          <Toaster position="bottom-right" closeButton richColors />
        </TooltipProvider>
      </WorkerPoolContextProvider>
    </QueryClientProvider>
  );
}
