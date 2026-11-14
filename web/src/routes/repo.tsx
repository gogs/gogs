import type { QueryClient } from "@tanstack/react-query";
import { type AnyRoute, createRoute, notFound, redirect } from "@tanstack/react-router";

import { LoaderResponseError, loadSignedInPage, loaderResponseError } from "@/lib/loader-error";
import { type PageSearch, validatePageSearch } from "@/lib/page-search";
import {
  fetchRepoCollaborators,
  fetchRepoDeployKeys,
  fetchRepoForkOptions,
  fetchRepoNewOptions,
  fetchRepoPeople,
  repoHeaderQuery,
  repoSettingsQuery,
} from "@/lib/queries/repo";
import { subUrl } from "@/lib/url";
import { RepoCommit, type RepoCommitPage } from "@/pages/repo/Commit";
import { type RepoCommitSearch, validateRepoCommitSearch } from "@/pages/repo/Commit.search";
import { RepoCreate, RepoFork, RepoMigrate } from "@/pages/repo/New";
import { RepoPeople } from "@/pages/repo/People";
import { RepoCollaboration, RepoDeployKeys, RepoOptions } from "@/pages/repo/Settings";

// Match the legacy server-side route constraint (see `web.go` near the
// `/commit/:sha([a-f0-9]{7,64})$` declaration). The server enforces the same
// shape for SEO and to skip the SPA shell for malformed paths; this client
// check short-circuits the loader so we render 404 without a wasted fetch.
const SHA_RE = /^[a-f0-9]{7,64}$/;

interface RouterContext {
  queryClient: QueryClient;
}

export function createRepoRoutes(rootRoute: AnyRoute) {
  const repoCommitRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/commit/$sha",
    validateSearch: validateRepoCommitSearch,
    // Reject malformed SHA at parse time so the route doesn't match for paths
    // like `/owner/repo/commit/garbage`. The thrown `notFound()` bubbles to the
    // root route's NotFound component.
    params: {
      parse: (raw: { owner: string; repo: string; sha: string }) => {
        if (!SHA_RE.test(raw.sha)) {
          // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
          throw notFound();
        }
        return raw;
      },
      stringify: (params: { owner: string; repo: string; sha: string }) => params,
    },
    loaderDeps: ({ search }: { search: RepoCommitSearch }) => ({ whitespace: search.whitespace }),
    loader: async ({ params, deps, context }): Promise<RepoCommitPage> => {
      const metaURL = subUrl(`/api/web/${params.owner}/${params.repo}/commit/${params.sha}`);
      const rawBase = subUrl(`/${params.owner}/${params.repo}/commit/${params.sha}.diff`);
      const rawURL = deps.whitespace ? `${rawBase}?whitespace=${encodeURIComponent(deps.whitespace)}` : rawBase;
      const routerContext = context as RouterContext;
      // Three requests in parallel: repo header (via Query cache for cross-page
      // reuse), commit metadata JSON, raw patch text. Splitting the patch out
      // skips JSON-string escaping and lets the browser cache the (often large)
      // patch separately from the metadata.
      try {
        const [, meta, patch] = await Promise.all([
          routerContext.queryClient.ensureQueryData(repoHeaderQuery(params.owner, params.repo)),
          fetch(metaURL, { credentials: "same-origin" }).then(async (res) => {
            if (!res.ok) throw await loaderResponseError(res);
            return (await res.json()) as Omit<RepoCommitPage, "patch">;
          }),
          fetch(rawURL, { credentials: "same-origin" }).then(async (res) => {
            if (!res.ok) throw await loaderResponseError(res);
            return res.text();
          }),
        ]);
        return { ...meta, patch };
      } catch (err) {
        if (err instanceof LoaderResponseError && err.status === 404) {
          // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
          throw notFound();
        }
        throw err;
      }
    },
    component: RepoCommit,
  });

  async function loadPeople(
    queryClient: QueryClient,
    owner: string,
    repo: string,
    list: "stargazers" | "watchers",
    page: number | undefined,
  ) {
    try {
      const [, data] = await Promise.all([
        queryClient.ensureQueryData(repoHeaderQuery(owner, repo)),
        fetchRepoPeople(owner, repo, list, page),
      ]);
      return data;
    } catch (err) {
      if (err instanceof LoaderResponseError && err.status === 404) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
        throw notFound();
      }
      if (err instanceof LoaderResponseError && (err.status === 401 || err.status === 403)) {
        // The viewer is not allowed to see site content. Reload the page so the
        // server sends them to sign in, account activation, or the login
        // prohibited page.
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- TanStack's redirect() returns a sentinel that must be thrown.
        throw redirect({ href: window.location.href, reloadDocument: true });
      }
      throw err;
    }
  }

  const repoStargazersRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/stars",
    validateSearch: validatePageSearch,
    loaderDeps: ({ search }: { search: PageSearch }) => ({ page: search.page }),
    loader: ({ params, deps, context }) =>
      loadPeople((context as RouterContext).queryClient, params.owner, params.repo, "stargazers", deps.page),
    component: function RepoStargazers() {
      const { owner, repo } = repoStargazersRoute.useParams();
      return <RepoPeople owner={owner} name={repo} list="stargazers" data={repoStargazersRoute.useLoaderData()} />;
    },
  });

  const repoWatchersRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/watchers",
    validateSearch: validatePageSearch,
    loaderDeps: ({ search }: { search: PageSearch }) => ({ page: search.page }),
    loader: ({ params, deps, context }) =>
      loadPeople((context as RouterContext).queryClient, params.owner, params.repo, "watchers", deps.page),
    component: function RepoWatchers() {
      const { owner, repo } = repoWatchersRoute.useParams();
      return <RepoPeople owner={owner} name={repo} list="watchers" data={repoWatchersRoute.useLoaderData()} />;
    },
  });

  interface RepoNewSearch {
    // The organization to create the repository for.
    org?: number;
    // Whether a migration starts out as a mirror.
    mirror?: boolean;
  }

  function validateRepoNewSearch(search: Record<string, unknown>): RepoNewSearch {
    const org = Number(search.org);
    return {
      org: Number.isInteger(org) && org > 0 ? org : undefined,
      mirror: String(search.mirror) === "1" || search.mirror === true ? true : undefined,
    };
  }

  async function loadRepoNewOptions(org: number | undefined) {
    try {
      return await fetchRepoNewOptions(org);
    } catch (err) {
      // An organization the viewer cannot create repositories for is not
      // found.
      if (err instanceof LoaderResponseError && err.status === 404) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
        throw notFound();
      }
      if (err instanceof LoaderResponseError && (err.status === 401 || err.status === 403)) {
        // The viewer is signed out, has not confirmed their email, or is
        // prohibited from signing in. Reload the page so the server sends them to
        // sign in, account activation, or the login prohibited page.
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- TanStack's redirect() returns a sentinel that must be thrown.
        throw redirect({ href: window.location.href, reloadDocument: true });
      }
      throw err;
    }
  }

  const repoCreateRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/repo/create",
    validateSearch: validateRepoNewSearch,
    loaderDeps: ({ search }: { search: RepoNewSearch }) => ({ org: search.org }),
    loader: ({ deps }) => loadRepoNewOptions(deps.org),
    component: function Create() {
      const options = repoCreateRoute.useLoaderData();
      // Start over when the owner asked for changes.
      return <RepoCreate key={options.ownerID} options={options} />;
    },
  });

  const repoMigrateRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/repo/migrate",
    validateSearch: validateRepoNewSearch,
    loaderDeps: ({ search }: { search: RepoNewSearch }) => ({ org: search.org, mirror: search.mirror ?? false }),
    loader: async ({ deps }) => ({ options: await loadRepoNewOptions(deps.org), mirror: deps.mirror }),
    component: function Migrate() {
      const { options, mirror } = repoMigrateRoute.useLoaderData();
      return <RepoMigrate key={`${options.ownerID}-${mirror}`} options={options} mirror={mirror} />;
    },
  });

  const repoForkRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/repo/fork/$id",
    loader: async ({ params }) => {
      const id = Number(params.id);
      if (!Number.isInteger(id) || id <= 0) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
        throw notFound();
      }
      try {
        return { id, options: await fetchRepoForkOptions(id) };
      } catch (err) {
        if (err instanceof LoaderResponseError && err.status === 404) {
          // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
          throw notFound();
        }
        if (err instanceof LoaderResponseError && (err.status === 401 || err.status === 403)) {
          // The viewer is signed out, has not confirmed their email, or is
          // prohibited from signing in. Reload the page so the server sends them to
          // sign in, account activation, or the login prohibited page.
          // eslint-disable-next-line @typescript-eslint/only-throw-error -- TanStack's redirect() returns a sentinel that must be thrown.
          throw redirect({ href: window.location.href, reloadDocument: true });
        }
        throw err;
      }
    },
    component: function Fork() {
      const { id, options } = repoForkRoute.useLoaderData();
      return <RepoFork key={id} id={id} options={options} />;
    },
  });

  // loadSettingsPage loads the header and the settings of a repository, which
  // every settings page shows, along with the data of the page. Only admins of
  // the repository get past it.
  function loadSettingsPage<T>(queryClient: QueryClient, owner: string, repo: string, load: () => Promise<T>) {
    return loadSignedInPage(async () => {
      const [, , data] = await Promise.all([
        queryClient.ensureQueryData(repoHeaderQuery(owner, repo)),
        queryClient.ensureQueryData(repoSettingsQuery(owner, repo)),
        load(),
      ]);
      return data;
    });
  }

  const repoSettingsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/settings",
    loader: ({ params, context }) =>
      loadSettingsPage((context as RouterContext).queryClient, params.owner, params.repo, () => Promise.resolve()),
    component: function Settings() {
      const { owner, repo } = repoSettingsRoute.useParams();
      // Start over when the repository changes, e.g., after renaming it.
      return <RepoOptions key={`${owner}/${repo}`} owner={owner} name={repo} />;
    },
  });

  const repoCollaborationRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/settings/collaboration",
    loader: ({ params, context }) =>
      loadSettingsPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoCollaborators(params.owner, params.repo),
      ),
    component: function Collaboration() {
      const { owner, repo } = repoCollaborationRoute.useParams();
      return <RepoCollaboration owner={owner} name={repo} collaborators={repoCollaborationRoute.useLoaderData()} />;
    },
  });

  const repoDeployKeysRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/settings/keys",
    loader: ({ params, context }) =>
      loadSettingsPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoDeployKeys(params.owner, params.repo),
      ),
    component: function DeployKeys() {
      const { owner, repo } = repoDeployKeysRoute.useParams();
      return <RepoDeployKeys owner={owner} name={repo} keys={repoDeployKeysRoute.useLoaderData()} />;
    },
  });

  return [
    repoCommitRoute,
    repoStargazersRoute,
    repoWatchersRoute,
    repoCreateRoute,
    repoMigrateRoute,
    repoForkRoute,
    repoSettingsRoute,
    repoCollaborationRoute,
    repoDeployKeysRoute,
  ];
}
