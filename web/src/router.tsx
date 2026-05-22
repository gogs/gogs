import {
  Outlet,
  RouterProvider,
  createRootRouteWithContext,
  createRoute,
  createRouter,
  redirect,
} from "@tanstack/react-router";

import { Footer } from "@/components/Footer";
import { Navbar } from "@/components/Navbar";
import { webContext } from "@/lib/context";
import { subUrl } from "@/lib/url";
import type { UserInfo } from "@/lib/user-info";
import { Landing } from "@/pages/Landing";
import { MFA } from "@/pages/MFA";
import { NotFound } from "@/pages/NotFound";
import { ResetPassword, type ResetPasswordPage } from "@/pages/ResetPassword";
import { SignIn, type SignInPage } from "@/pages/SignIn";

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

const signInRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/user/sign-in",
  beforeLoad: ({ context }) => {
    if (context.user) {
      // Full navigation to "/" so the server-rendered dashboard handler runs.
      // A client-side TanStack redirect would render the SPA's "/" route
      // (Landing, the anon page) and make an authed user look signed out.
      window.location.assign(subUrl("/"));
      // Throw to halt loader execution. TanStack treats the thrown redirect
      // as a sentinel; we never reach a SPA navigation because the line
      // above already started a document-level one.
      // eslint-disable-next-line @typescript-eslint/only-throw-error -- TanStack's redirect() returns a sentinel that must be thrown.
      throw redirect({ to: "/", replace: true });
    }
  },
  loader: async (): Promise<SignInPage> => {
    const res = await fetch(subUrl("/api/web/user/sign-in"), { credentials: "same-origin" });
    if (!res.ok) {
      return { loginSources: [] };
    }
    return (await res.json()) as SignInPage;
  },
  component: SignIn,
});

const resetPasswordRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/user/reset_password",
  loader: async (): Promise<ResetPasswordPage> => {
    const code = new URLSearchParams(window.location.search).get("code") ?? "";
    if (!code) {
      return { code, valid: false };
    }

    const res = await fetch(subUrl("/api/web/user/reset-password") + "?code=" + encodeURIComponent(code), {
      credentials: "same-origin",
    });
    if (!res.ok) {
      return { code, valid: false };
    }
    const data = (await res.json()) as { valid: boolean };
    return { code, valid: data.valid };
  },
  component: ResetPassword,
});

const mfaRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/user/mfa",
  loader: async (): Promise<{ pending: boolean }> => {
    const res = await fetch(subUrl("/api/web/user/mfa"), { credentials: "same-origin" });
    if (res.status === 404) {
      // No pending MFA challenge — there is nothing to verify here, so fall
      // through to the server-rendered home, which will redirect to sign-in
      // for anonymous visitors and to the dashboard for signed-in ones.
      window.location.assign(subUrl("/"));
      return { pending: false };
    }
    return { pending: res.ok };
  },
  component: MFA,
});

const routeTree = rootRoute.addChildren([landingRoute, signInRoute, resetPasswordRoute, mfaRoute]);

function makeRouter(context: RouterContext) {
  return createRouter({
    routeTree,
    basepath: webContext.subURL || "/",
    defaultNotFoundComponent: NotFound,
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
  return <RouterProvider router={router} />;
}
