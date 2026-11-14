import type { QueryClient } from "@tanstack/react-query";
import { type AnyRoute, createRoute, notFound, redirect } from "@tanstack/react-router";

import { LoaderResponseError, loadSignedInPage, loaderResponseError } from "@/lib/loader-error";
import { type PageSearch, validatePageSearch } from "@/lib/page-search";
import {
  fetchRepoBranchSettings,
  fetchRepoBranches,
  fetchRepoCollaborators,
  fetchRepoDeployKeys,
  fetchRepoForkOptions,
  fetchRepoForks,
  fetchRepoIssue,
  fetchRepoIssues,
  fetchRepoLabels,
  fetchRepoMilestone,
  fetchRepoMilestones,
  fetchRepoNewIssue,
  fetchRepoNewOptions,
  fetchRepoNewRelease,
  fetchRepoPeople,
  fetchRepoProtectedBranch,
  fetchRepoRelease,
  fetchRepoReleases,
  repoHeaderQuery,
  repoSettingsQuery,
} from "@/lib/queries/repo";
import { type WebhookType, fetchWebhook, fetchWebhooks } from "@/lib/queries/webhooks";
import { subUrl } from "@/lib/url";
import { WebhookEdit, WebhookNew, Webhooks } from "@/pages/Webhooks";
import { RepoBranches } from "@/pages/repo/Branches";
import { RepoCommit, type RepoCommitPage } from "@/pages/repo/Commit";
import { type RepoCommitSearch, validateRepoCommitSearch } from "@/pages/repo/Commit.search";
import { RepoForks } from "@/pages/repo/Forks";
import { RepoIssue } from "@/pages/repo/Issue";
import { RepoIssues } from "@/pages/repo/Issues";
import { type RepoIssuesSearch, validateRepoIssuesSearch } from "@/pages/repo/Issues.search";
import { RepoLabels } from "@/pages/repo/Labels";
import { RepoMilestoneEditor, RepoMilestones, type RepoMilestonesSearch } from "@/pages/repo/Milestones";
import { RepoCreate, RepoFork, RepoMigrate } from "@/pages/repo/New";
import { RepoNewIssue } from "@/pages/repo/NewIssue";
import { type RepoNewIssueSearch, validateRepoNewIssueSearch } from "@/pages/repo/NewIssue.search";
import { RepoPeople } from "@/pages/repo/People";
import { RepoReleaseEditor, RepoReleases } from "@/pages/repo/Releases";
import { type RepoReleasesSearch, validateRepoReleasesSearch } from "@/pages/repo/Releases.search";
import {
  RepoBranchProtection,
  RepoBranchSettings,
  RepoCollaboration,
  RepoDeployKeys,
  RepoOptions,
} from "@/pages/repo/Settings";

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

  // loadRepoPage loads the header of a repository along with the data of the
  // page.
  async function loadRepoPage<T>(queryClient: QueryClient, owner: string, repo: string, load: () => Promise<T>) {
    try {
      const [, data] = await Promise.all([queryClient.ensureQueryData(repoHeaderQuery(owner, repo)), load()]);
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
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoPeople(params.owner, params.repo, "stargazers", deps.page),
      ),
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
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoPeople(params.owner, params.repo, "watchers", deps.page),
      ),
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

  const repoBranchSettingsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/settings/branches",
    loader: ({ params, context }) =>
      loadSettingsPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoBranchSettings(params.owner, params.repo),
      ),
    component: function BranchSettings() {
      const { owner, repo } = repoBranchSettingsRoute.useParams();
      const settings = repoBranchSettingsRoute.useLoaderData();
      // Start over once the default branch is saved, so the choice shows it.
      return <RepoBranchSettings key={settings.defaultBranch} owner={owner} name={repo} settings={settings} />;
    },
  });

  // Branch names can contain slashes, so the branch is the rest of the path.
  const repoBranchProtectionRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/settings/branches/$",
    // Without a branch the path is the branch settings, which a splat would
    // otherwise take over.
    params: { parse: (params) => (params._splat ? params : false) },
    loader: ({ params, context }) =>
      loadSettingsPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoProtectedBranch(params.owner, params.repo, params._splat ?? ""),
      ),
    component: function BranchProtection() {
      const { owner, repo, _splat: branch } = repoBranchProtectionRoute.useParams();
      return (
        <RepoBranchProtection
          key={`${owner}/${repo}@${branch}`}
          owner={owner}
          name={repo}
          branch={repoBranchProtectionRoute.useLoaderData()}
        />
      );
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

  const repoWebhooksRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/settings/hooks",
    loader: ({ params, context }) =>
      loadSettingsPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchWebhooks({ owner: params.owner, repo: params.repo }),
      ),
    component: function RepoWebhooks() {
      const { owner, repo } = repoWebhooksRoute.useParams();
      return <Webhooks scope={{ owner, repo }} list={repoWebhooksRoute.useLoaderData()} />;
    },
  });

  const repoWebhookNewRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/settings/hooks/$type/new",
    loader: ({ params, context }) =>
      loadSettingsPage((context as RouterContext).queryClient, params.owner, params.repo, async () => {
        const { types } = await fetchWebhooks({ owner: params.owner, repo: params.repo });
        if (!types.includes(params.type as WebhookType)) {
          // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
          throw notFound();
        }
      }),
    component: function RepoWebhookNew() {
      const { owner, repo, type } = repoWebhookNewRoute.useParams();
      return <WebhookNew key={type} scope={{ owner, repo }} type={type as WebhookType} />;
    },
  });

  const repoWebhookRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/settings/hooks/$id",
    loader: ({ params, context }) => {
      const id = Number(params.id);
      if (!Number.isInteger(id) || id <= 0) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
        throw notFound();
      }
      return loadSettingsPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchWebhook({ owner: params.owner, repo: params.repo }, id),
      );
    },
    component: function RepoWebhook() {
      const { owner, repo } = repoWebhookRoute.useParams();
      const hook = repoWebhookRoute.useLoaderData();
      return <WebhookEdit key={hook.id} scope={{ owner, repo }} hook={hook} />;
    },
  });

  const repoLabelsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/labels",
    loader: ({ params, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoLabels(params.owner, params.repo),
      ),
    component: function Labels() {
      const { owner, repo } = repoLabelsRoute.useParams();
      return <RepoLabels key={`${owner}/${repo}`} owner={owner} name={repo} data={repoLabelsRoute.useLoaderData()} />;
    },
  });

  const repoMilestonesRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/milestones",
    validateSearch: (search: Record<string, unknown>): RepoMilestonesSearch => ({
      state: search.state === "closed" ? "closed" : undefined,
      ...validatePageSearch(search),
    }),
    loaderDeps: ({ search }: { search: RepoMilestonesSearch }) => ({ state: search.state, page: search.page }),
    loader: ({ params, deps, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoMilestones(params.owner, params.repo, deps.state === "closed", deps.page),
      ),
    component: function Milestones() {
      const { owner, repo } = repoMilestonesRoute.useParams();
      return (
        <RepoMilestones
          owner={owner}
          name={repo}
          data={repoMilestonesRoute.useLoaderData()}
          search={repoMilestonesRoute.useSearch()}
        />
      );
    },
  });

  const repoMilestoneNewRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/milestones/new",
    loader: ({ params, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, async () => {
        // Like the server-rendered page, only writers find it.
        const { viewerCanWrite } = await fetchRepoMilestones(params.owner, params.repo, false);
        if (!viewerCanWrite) {
          // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
          throw notFound();
        }
      }),
    component: function MilestoneNew() {
      const { owner, repo } = repoMilestoneNewRoute.useParams();
      return <RepoMilestoneEditor key={`${owner}/${repo}`} owner={owner} name={repo} />;
    },
  });

  const repoMilestoneEditRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/milestones/$id/edit",
    loader: async ({ params, context }) => {
      const id = Number(params.id);
      if (!Number.isInteger(id) || id <= 0) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
        throw notFound();
      }
      const milestone = await loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoMilestone(params.owner, params.repo, id),
      );
      return { id, milestone };
    },
    component: function MilestoneEdit() {
      const { owner, repo } = repoMilestoneEditRoute.useParams();
      const { id, milestone } = repoMilestoneEditRoute.useLoaderData();
      return <RepoMilestoneEditor key={id} owner={owner} name={repo} id={id} initial={milestone} />;
    },
  });

  function createIssueListRoute(kind: "issues" | "pulls") {
    const route = createRoute({
      getParentRoute: () => rootRoute,
      path: `/$owner/$repo/${kind}`,
      validateSearch: validateRepoIssuesSearch,
      loaderDeps: ({ search }: { search: RepoIssuesSearch }) => search,
      loader: ({ params, deps, context }) =>
        loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
          fetchRepoIssues(params.owner, params.repo, kind, { ...deps }),
        ),
      component: function IssueList() {
        const { owner, repo } = route.useParams();
        return (
          <RepoIssues owner={owner} name={repo} kind={kind} data={route.useLoaderData()} search={route.useSearch()} />
        );
      },
    });
    return route;
  }
  const repoIssuesRoute = createIssueListRoute("issues");
  const repoPullsRoute = createIssueListRoute("pulls");

  const repoNewIssueRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/issues/new",
    validateSearch: validateRepoNewIssueSearch,
    loader: ({ params, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoNewIssue(params.owner, params.repo),
      ),
    component: function NewIssue() {
      const { owner, repo } = repoNewIssueRoute.useParams();
      const search: RepoNewIssueSearch = repoNewIssueRoute.useSearch();
      return (
        <RepoNewIssue
          key={`${owner}/${repo}`}
          owner={owner}
          name={repo}
          options={repoNewIssueRoute.useLoaderData()}
          search={search}
        />
      );
    },
  });

  const repoIssueRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/issues/$index",
    loader: ({ params, context }) => {
      const index = Number(params.index);
      if (!Number.isInteger(index) || index <= 0) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
        throw notFound();
      }
      return loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoIssue(params.owner, params.repo, index),
      );
    },
    component: function Issue() {
      const { owner, repo } = repoIssueRoute.useParams();
      const data = repoIssueRoute.useLoaderData();
      // Start over on another issue, e.g., after following a reference.
      return <RepoIssue key={`${owner}/${repo}#${data.index}`} owner={owner} name={repo} data={data} />;
    },
  });

  const repoForksRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/forks",
    loader: ({ params, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoForks(params.owner, params.repo),
      ),
    component: function Forks() {
      const { owner, repo } = repoForksRoute.useParams();
      return <RepoForks owner={owner} name={repo} forks={repoForksRoute.useLoaderData()} />;
    },
  });

  const repoBranchesRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/branches",
    loader: ({ params, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoBranches(params.owner, params.repo),
      ),
    component: function Branches() {
      const { owner, repo } = repoBranchesRoute.useParams();
      return <RepoBranches owner={owner} name={repo} view="overview" data={repoBranchesRoute.useLoaderData()} />;
    },
  });

  const repoAllBranchesRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/branches/all",
    loader: ({ params, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoBranches(params.owner, params.repo),
      ),
    component: function AllBranches() {
      const { owner, repo } = repoAllBranchesRoute.useParams();
      return <RepoBranches owner={owner} name={repo} view="all" data={repoAllBranchesRoute.useLoaderData()} />;
    },
  });

  const repoReleasesRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/releases",
    validateSearch: validateRepoReleasesSearch,
    loaderDeps: ({ search }: { search: RepoReleasesSearch }) => ({ after: search.after }),
    loader: ({ params, deps, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoReleases(params.owner, params.repo, deps.after),
      ),
    component: function Releases() {
      const { owner, repo } = repoReleasesRoute.useParams();
      return <RepoReleases owner={owner} name={repo} data={repoReleasesRoute.useLoaderData()} />;
    },
  });

  const repoReleaseNewRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/releases/new",
    loader: ({ params, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoNewRelease(params.owner, params.repo),
      ),
    component: function ReleaseNew() {
      const { owner, repo } = repoReleaseNewRoute.useParams();
      return (
        <RepoReleaseEditor
          key={`${owner}/${repo}`}
          owner={owner}
          name={repo}
          options={repoReleaseNewRoute.useLoaderData()}
        />
      );
    },
  });

  // Tag names can contain slashes, so the tag is the rest of the path.
  const repoReleaseEditRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/releases/edit/$",
    loader: ({ params, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoRelease(params.owner, params.repo, params._splat ?? ""),
      ),
    component: function ReleaseEdit() {
      const { owner, repo, _splat: tag } = repoReleaseEditRoute.useParams();
      return (
        <RepoReleaseEditor
          key={`${owner}/${repo}@${tag}`}
          owner={owner}
          name={repo}
          release={repoReleaseEditRoute.useLoaderData()}
        />
      );
    },
  });

  return [
    repoCommitRoute,
    repoStargazersRoute,
    repoWatchersRoute,
    repoIssuesRoute,
    repoPullsRoute,
    repoNewIssueRoute,
    repoIssueRoute,
    repoForksRoute,
    repoBranchesRoute,
    repoAllBranchesRoute,
    repoReleasesRoute,
    repoReleaseNewRoute,
    repoReleaseEditRoute,
    repoLabelsRoute,
    repoMilestonesRoute,
    repoMilestoneNewRoute,
    repoMilestoneEditRoute,
    repoCreateRoute,
    repoMigrateRoute,
    repoForkRoute,
    repoSettingsRoute,
    repoCollaborationRoute,
    repoBranchSettingsRoute,
    repoBranchProtectionRoute,
    repoDeployKeysRoute,
    repoWebhooksRoute,
    repoWebhookNewRoute,
    repoWebhookRoute,
  ];
}
