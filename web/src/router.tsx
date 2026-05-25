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
import { loaderResponseError } from "@/lib/loader-error";
import { subUrl } from "@/lib/url";
import type { UserInfo } from "@/lib/user-info";
import { Activate, type ActivatePage } from "@/pages/Activate";
import { Landing } from "@/pages/Landing";
import { MFA } from "@/pages/MFA";
import { NotFound } from "@/pages/NotFound";
import { ResetPassword, type ResetPasswordPage } from "@/pages/ResetPassword";
import { ServerError } from "@/pages/ServerError";
import { SignIn, type SignInPage } from "@/pages/SignIn";
import { SignUp, type SignUpPage } from "@/pages/SignUp";

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

function requireUnauthenticated({ context }: { context: RouterContext }) {
  if (!context.user) return;
  // Bounce authenticated visits to "/" via full navigation so the server-rendered
  // dashboard handler runs.
  window.location.assign(subUrl("/"));
  // The thrown redirect is a sentinel to halt loader execution;
  // the document-level navigation above is what actually moves the user.
  // eslint-disable-next-line @typescript-eslint/only-throw-error -- TanStack's redirect() returns a sentinel that must be thrown.
  throw redirect({ to: "/", replace: true });
}

const signInRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/user/sign-in",
  beforeLoad: requireUnauthenticated,
  loader: async (): Promise<SignInPage> => {
    const res = await fetch(subUrl("/api/web/user/sign-in"), { credentials: "same-origin" });
    if (!res.ok) {
      return { loginSources: [] };
    }
    return (await res.json()) as SignInPage;
  },
  component: SignIn,
});

const signUpRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/user/sign-up",
  beforeLoad: requireUnauthenticated,
  loader: async (): Promise<SignUpPage> => {
    const res = await fetch(subUrl("/api/web/user/sign-up"), { credentials: "same-origin" });
    if (!res.ok) {
      throw await loaderResponseError(res);
    }
    return (await res.json()) as SignUpPage;
  },
  component: SignUp,
});

const resetPasswordRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/user/reset-password",
  beforeLoad: requireUnauthenticated,
  loader: async (): Promise<ResetPasswordPage> => {
    const code = new URLSearchParams(window.location.search).get("code") ?? "";
    const url = code
      ? subUrl("/api/web/user/reset-password") + "?code=" + encodeURIComponent(code)
      : subUrl("/api/web/user/reset-password");
    const res = await fetch(url, { credentials: "same-origin" });
    if (!res.ok) {
      return { code, emailEnabled: false, valid: false };
    }
    const data = (await res.json()) as { emailEnabled: boolean; valid: boolean };
    return { code, emailEnabled: data.emailEnabled, valid: data.valid };
  },
  component: ResetPassword,
});

const activateRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/user/activate",
  loader: async ({ context }): Promise<ActivatePage> => {
    const code = new URLSearchParams(window.location.search).get("code") ?? "";
    if (!context.user) {
      return { code, email: "", codeLifetimeHours: 0 };
    }
    const res = await fetch(subUrl("/api/web/user/activate"), { credentials: "same-origin" });
    if (res.status === 404) {
      // Already-active user hit a stale activation link. Send them home via
      // a full navigation so the server-rendered dashboard handler decides
      // where to land.
      window.location.assign(subUrl("/"));
      return { code, email: "", codeLifetimeHours: 0 };
    }
    if (!res.ok) {
      throw await loaderResponseError(res);
    }
    const data = (await res.json()) as Omit<ActivatePage, "code">;
    return { code, ...data };
  },
  component: Activate,
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

const routeTree = rootRoute.addChildren([
  landingRoute,
  signInRoute,
  signUpRoute,
  resetPasswordRoute,
  activateRoute,
  mfaRoute,
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
  return <RouterProvider router={router} />;
}
