import * as DialogPrimitive from "@radix-ui/react-dialog";
import { keepPreviousData, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useRouterState } from "@tanstack/react-router";
import {
  BookMarked,
  Building2,
  CircleCheck,
  CircleDot,
  Code,
  Copy,
  Download,
  Eye,
  EyeOff,
  FileCode,
  FileText,
  Folder,
  GitBranch,
  GitCommitHorizontal,
  GitFork,
  GitPullRequest,
  GitPullRequestClosed,
  Globe,
  HelpCircle,
  History,
  House,
  Import,
  Info,
  KeyRound,
  LayoutDashboard,
  LoaderCircle,
  Lock,
  LogIn,
  LogOut,
  Milestone,
  Monitor,
  Moon,
  Plus,
  Search,
  Settings,
  ShieldCheck,
  Star,
  Sun,
  Tag,
  Upload,
  User,
  UserPlus,
  Users,
  Webhook,
  X,
} from "lucide-react";
import { type ComponentType, type KeyboardEvent, useEffect, useId, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import type { RepoListItemData } from "@/components/RepoListItem";
import type { UserListItemData } from "@/components/UserListItem";
import { type Segment, filterByText, filterFiles, highlightPath, highlightText, paletteShortcut } from "@/lib/palette";
import {
  type RepoHeaderData,
  fetchIssueSearch,
  fetchRepoPaths,
  fetchRepoSearchPreview,
  refPath,
  repoHeaderQuery,
  starRepo,
  unstarRepo,
  unwatchRepo,
  watchRepo,
} from "@/lib/queries/repo";
import { fetchJSON } from "@/lib/queries/request";
import { signOut } from "@/lib/sign-out";
import { useTheme } from "@/lib/theme-context";
import { subUrl } from "@/lib/url";
import { useDebounced } from "@/lib/use-debounced";
import { useUserInfo } from "@/lib/use-user-info";
import type { UserInfo } from "@/lib/user-info";
import { cn } from "@/lib/utils";

type Icon = ComponentType<{ className?: string }>;

interface Item {
  key: string;
  label: string;
  // keywords are more words the item matches, e.g., "log out" for signing out.
  keywords?: string;
  icon: Icon;
  // glyph shows in place of the icon, e.g., the prefix an item puts in the box.
  glyph?: string;
  meta?: string;
  // An item opens a page of the app at `to`, loads the page at `download`,
  // opens `external` in a new tab, runs `run`, or puts `prefix` in the box.
  to?: string;
  download?: string;
  external?: string;
  run?: () => void;
  prefix?: string;
  // mark is how the label shows what matched the query.
  mark?: "text" | "path";
  // markQuery is what to mark when it differs from the query, e.g., the
  // keyword of a code search without its qualifiers.
  markQuery?: string;
  mono?: boolean;
  // disabled items tell something, e.g., that a search is running.
  disabled?: boolean;
  spin?: boolean;
}

interface Group {
  key: string;
  label?: string;
  items: Item[];
}

const PREFIXES = [">", "#", "@", "/"] as const;
type Prefix = (typeof PREFIXES)[number];

// parseInput splits the prefix that picks one kind of result off what was
// typed.
function parseInput(input: string): { prefix: Prefix | null; text: string } {
  const trimmed = input.trimStart();
  const first = trimmed.charAt(0);
  if ((PREFIXES as readonly string[]).includes(first)) {
    return { prefix: first as Prefix, text: trimmed.slice(1).trim() };
  }
  return { prefix: null, text: input.trim() };
}

// useRepoScope returns the repository of the current page, if any.
function useRepoScope() {
  const params = useRouterState({
    select: (state) => state.matches.at(-1)?.params as Record<string, string | undefined> | undefined,
  });
  const owner = params?.owner;
  const name = params?.repo;
  return owner && name ? { owner, name } : null;
}

const RECENT_MAX = 5;

function recentKey(user: UserInfo) {
  return `gogs-palette-recent:${user.username}`;
}

function readRecent(user: UserInfo): string[] {
  try {
    const value: unknown = JSON.parse(localStorage.getItem(recentKey(user)) ?? "[]");
    return Array.isArray(value) ? value.filter((v): v is string => typeof v === "string") : [];
  } catch {
    return [];
  }
}

// useRecordRecentRepo remembers the repositories the viewer opened, the
// latest first, for the palette to offer before anything is typed.
function useRecordRecentRepo(user: UserInfo | null, repo: RepoHeaderData | undefined) {
  const fullName = repo ? `${repo.owner}/${repo.name}` : null;
  useEffect(() => {
    if (!user || !fullName) return;
    const next = [fullName, ...readRecent(user).filter((name) => name !== fullName)].slice(0, RECENT_MAX);
    try {
      localStorage.setItem(recentKey(user), JSON.stringify(next));
    } catch {
      // Storage may be full or blocked, and the list is only a convenience.
    }
  }, [user, fullName]);
}

// plainText returns the text of HTML the server sanitized, e.g., a
// repository description.
function plainText(html: string) {
  return new DOMParser().parseFromString(html, "text/html").body.textContent?.trim() ?? "";
}

// aroundMatch starts a long line shortly before the keyword, so that the
// match shows in a row that cuts the line off.
function aroundMatch(line: string, keyword: string) {
  const text = line.trim();
  const at = text.toLowerCase().indexOf(keyword.toLowerCase());
  return at > 24 ? `…${text.slice(at - 12)}` : text;
}

function basename(path: string) {
  return path.slice(path.lastIndexOf("/") + 1);
}

// CommandPalette searches and jumps to pages, issues, files, code,
// repositories and people, and runs actions, from anywhere. Ctrl K or ⌘K
// opens and closes it.
export function CommandPalette({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation();
  const user = useUserInfo();
  const scope = useRepoScope();
  const { data: repo } = useQuery({
    ...repoHeaderQuery(scope?.owner ?? "", scope?.name ?? ""),
    enabled: scope !== null,
    staleTime: 30_000,
    retry: false,
  });
  // The header of the page before may still be in the cache.
  const scopedRepo = repo && scope && repo.owner === scope.owner && repo.name === scope.name ? repo : undefined;
  useRecordRecentRepo(user, scopedRepo);

  useEffect(() => {
    const onKeyDown = (e: globalThis.KeyboardEvent) => {
      if (!paletteShortcut.matches(e) || e.altKey || e.shiftKey || e.key.toLowerCase() !== "k") return;
      e.preventDefault();
      onOpenChange(!open);
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [open, onOpenChange]);

  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className="fixed inset-0 z-50 bg-black/40" />
        <DialogPrimitive.Content
          aria-describedby={undefined}
          // The box takes the focus, not the button before it that drops the
          // repository scope.
          onOpenAutoFocus={(e) => e.preventDefault()}
          className={cn(
            "fixed inset-0 z-50 flex flex-col bg-(--color-popover) text-(--color-popover-foreground) shadow-lg outline-none",
            "sm:inset-auto sm:top-[12vh] sm:left-1/2 sm:max-h-[min(32rem,76vh)] sm:w-[min(40rem,calc(100%-2rem))] sm:-translate-x-1/2 sm:rounded-lg sm:border sm:border-(--color-border)",
          )}
        >
          <DialogPrimitive.Title className="sr-only">{t("palette.title")}</DialogPrimitive.Title>
          <PaletteBody user={user} scope={scope} repo={scopedRepo} close={() => onOpenChange(false)} />
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}

// PaletteBody is mounted each time the palette opens, so it starts empty and
// scoped to the repository of the page.
function PaletteBody({
  user,
  scope,
  repo,
  close,
}: {
  user: UserInfo | null;
  scope: { owner: string; name: string } | null;
  repo: RepoHeaderData | undefined;
  close: () => void;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { setTheme } = useTheme();
  const listID = useId();
  const inputRef = useRef<HTMLInputElement>(null);
  const [input, setInput] = useState("");
  const [scoped, setScoped] = useState(scope !== null);
  const [active, setActive] = useState(0);

  useEffect(() => inputRef.current?.focus(), []);

  const { prefix, text } = parseInput(input);
  const inRepo = scoped && scope !== null ? scope : null;
  const repoName = inRepo ? `${inRepo.owner}/${inRepo.name}` : "";
  const repoLink = inRepo ? `/${inRepo.owner}/${inRepo.name}` : "";
  // The default branch is only there for viewers who can read the code.
  const branch = inRepo ? repo?.defaultBranch : undefined;

  const lookup = useDebounced(text, 150);
  const codeLookup = useDebounced(text, 300);
  const settled = lookup === text;

  // Issues and pull requests, by number or title.
  const issueText = prefix === null || prefix === "#" ? lookup : "";
  const issues = useQuery({
    queryKey: ["palette", "issues", repoName, issueText],
    queryFn: ({ signal }) => fetchIssueSearch(issueText, repoName || undefined, signal),
    enabled: issueText !== "" && (!inRepo || Boolean(repo?.issuesEnabled || repo?.pullRequestsEnabled)),
    placeholderData: keepPreviousData,
    staleTime: 30_000,
    retry: false,
  });

  // Files of the default branch, listed once per repository.
  const paths = useQuery({
    queryKey: ["repo", inRepo?.owner, inRepo?.name, "paths"],
    queryFn: ({ signal }) => fetchRepoPaths(inRepo!.owner, inRepo!.name, signal),
    enabled: inRepo !== null && branch !== undefined && prefix === null,
    staleTime: 5 * 60_000,
    retry: false,
  });
  const matchedFiles = useMemo(
    () => (paths.data && prefix === null && text ? filterFiles(paths.data.paths, text, 4) : []),
    [paths.data, prefix, text],
  );

  // Repositories, by "owner/name" or a part of the name.
  const repoText = prefix === null ? lookup : "";
  const repoKeyword = repoText.includes("/") ? repoText.slice(repoText.indexOf("/") + 1) : repoText;
  const repos = useQuery({
    queryKey: ["palette", "repos", repoKeyword],
    queryFn: ({ signal }) =>
      fetchJSON<{ items: RepoListItemData[] }>(
        subUrl(`/api/web/explore/repos?${new URLSearchParams({ q: repoKeyword }).toString()}`),
        signal,
      ),
    enabled: repoText !== "",
    placeholderData: keepPreviousData,
    staleTime: 30_000,
    retry: false,
  });

  // People and organizations.
  const peopleText = prefix === "@" ? lookup : prefix === null && lookup ? lookup : null;
  const people = useQuery({
    queryKey: ["palette", "people", peopleText],
    queryFn: async ({ signal }) => {
      const qs = peopleText ? `?${new URLSearchParams({ q: peopleText }).toString()}` : "";
      const list = (kind: string) =>
        fetchJSON<{ items: UserListItemData[] }>(subUrl(`/api/web/explore/${kind}${qs}`), signal).catch(() => ({
          items: [],
        }));
      const [users, orgs] = await Promise.all([list("users"), list("organizations")]);
      return { users: users.items, orgs: orgs.items };
    },
    enabled: peopleText !== null,
    placeholderData: keepPreviousData,
    staleTime: 30_000,
    retry: false,
  });

  // Lines of code, in the repository only, since searching every repository
  // at each pause in typing costs too much.
  const codeText = prefix === null || prefix === "/" ? codeLookup : "";
  const codeKeyword = codeText.replace(/\bpath:\S*/gi, "").trim();
  const wantsCode =
    inRepo !== null &&
    branch !== undefined &&
    (prefix === null || prefix === "/") &&
    text.replace(/\bpath:\S*/gi, "").trim().length >= 3;
  const code = useQuery({
    queryKey: ["palette", "code", repoName, codeText, prefix === "/"],
    queryFn: ({ signal }) =>
      fetchRepoSearchPreview(inRepo!.owner, inRepo!.name, codeText, prefix === "/" ? 8 : 5, signal),
    enabled: wantsCode && codeKeyword.length >= 3,
    staleTime: 30_000,
    retry: false,
  });
  const codePending = wantsCode && (codeLookup !== text || code.isFetching);

  const groups = buildGroups();
  const items = groups.flatMap((group) => group.items);
  const choosable = items.filter((item) => !item.disabled);
  const activeItem = choosable[Math.min(active, choosable.length - 1)];
  const optionID = (item: Item) => `${listID}-${item.key}`;

  useEffect(() => {
    if (activeItem) document.getElementById(optionID(activeItem))?.scrollIntoView({ block: "nearest" });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- scroll only when another row becomes active
  }, [activeItem?.key]);

  function buildGroups(): Group[] {
    const out: Group[] = [];
    const add = (key: string, label: string | undefined, list: Item[]) => {
      if (list.length) out.push({ key, label, items: list });
    };
    const repoItems = repo && inRepo ? repoCommands(repo) : [];
    const globalItems = globalCommands();

    if (prefix === null && !text) {
      add(
        "prefixes",
        t("palette.group.prefixes"),
        PREFIXES.map((p) => ({
          key: `prefix-${p}`,
          label: t(`palette.prefix.${prefixName(p)}`),
          meta: t(`palette.prefix.${prefixName(p)}_hint`),
          icon: Search,
          glyph: p,
          prefix: `${p} `,
        })),
      );
      if (user) {
        add(
          "recent",
          t("palette.group.recent"),
          readRecent(user)
            .filter((name) => name !== repoName)
            .slice(0, 3)
            .map((name) => ({ key: `recent-${name}`, label: name, icon: History, to: `/${name}` })),
        );
      }
      if (inRepo) add("repo", repoName, repoItems.slice(0, 6));
      else add("pages", t("palette.group.pages"), globalItems.slice(0, 6));
      return out;
    }

    if (prefix === ">") {
      const commands = [...repoItems, ...globalItems];
      add("commands", t("palette.group.commands"), text ? filterByText(commands, text, commands.length) : commands);
      return out;
    }

    if (prefix === "#") {
      if (!text) {
        add("hint", undefined, [hintItem("issue-hint", t("palette.issue_hint"))]);
        return out;
      }
      add("issues", issuesLabel(), settled || issues.data ? issueItems() : []);
      return out;
    }

    if (prefix === "@") {
      add("people", t("palette.group.people"), peopleItems(8));
      return out;
    }

    if (prefix === "/") {
      if (!text) {
        add("hint", undefined, [hintItem("code-hint", t("palette.code_hint"))]);
        return out;
      }
      add("code", t("palette.group.code_in", { repo: repoName }), codeItems());
      add("search-code", undefined, [searchCodeItem()]);
      return out;
    }

    add("issues", issuesLabel(), issueItems().slice(0, inRepo ? 3 : 4));
    if (inRepo) {
      add("files", t("palette.group.files"), fileItems());
      add("repo", repoName, filterByText(repoItems, text, 5));
    }
    add("repos", t("palette.group.repos"), repoResults());
    add("people", t("palette.group.people"), peopleItems(3));
    add("pages", t("palette.group.pages"), filterByText(globalItems, text, 5));
    if (inRepo) add("code", t("palette.group.code_in", { repo: repoName }), codeItems());
    add("search-code", undefined, [searchCodeItem()]);
    return out;
  }

  function prefixName(p: Prefix) {
    return { ">": "commands", "#": "issues", "@": "people", "/": "code" }[p];
  }

  function hintItem(key: string, label: string): Item {
    return { key, label, icon: Info, disabled: true };
  }

  function issuesLabel() {
    return inRepo ? t("palette.group.issues_in", { repo: repoName }) : t("palette.group.issues");
  }

  function issueItems(): Item[] {
    return (issues.data?.items ?? []).map((issue) => {
      const icon = issue.isPull
        ? issue.isClosed
          ? GitPullRequestClosed
          : GitPullRequest
        : issue.isClosed
          ? CircleCheck
          : CircleDot;
      const number = inRepo ? `#${issue.index}` : `${issue.repo}#${issue.index}`;
      return {
        key: `issue-${issue.repo}-${issue.index}`,
        label: issue.title,
        icon,
        meta: issue.isClosed ? `${number} · ${t("repo.issues.closed_title")}` : number,
        to: `/${issue.repo}/${issue.isPull ? "pulls" : "issues"}/${issue.index}`,
        mark: "text",
      };
    });
  }

  function fileItems(): Item[] {
    if (!inRepo || branch === undefined) return [];
    return matchedFiles.map((path) => ({
      key: `file-${path}`,
      label: path,
      icon: FileText,
      to: `${repoLink}/src/${refPath(branch)}/${refPath(path)}`,
      mark: "path",
      mono: true,
    }));
  }

  function repoResults(): Item[] {
    const list = (repos.data?.items ?? []).map((r) => {
      const description = plainText(r.descriptionHTML);
      return {
        key: `repo-${r.owner}/${r.name}`,
        label: `${r.owner}/${r.name}`,
        icon: r.isPrivate ? Lock : BookMarked,
        meta: r.isPrivate ? t("palette.private") : description || undefined,
        to: `/${r.owner}/${r.name}`,
        mark: "text" as const,
      };
    });
    return filterByText(list, text, 4);
  }

  function peopleItems(limit: number): Item[] {
    const users = (people.data?.users ?? []).map((u) => ({
      key: `user-${u.name}`,
      label: u.name,
      keywords: u.fullName,
      icon: User,
      meta: u.fullName || undefined,
      to: `/${u.name}`,
      mark: "text" as const,
    }));
    const orgs = (people.data?.orgs ?? []).map((o) => ({
      key: `org-${o.name}`,
      label: o.name,
      keywords: o.fullName,
      icon: Building2,
      meta: t("palette.organization"),
      to: `/${o.name}`,
      mark: "text" as const,
    }));
    const all = [...users, ...orgs];
    return text ? filterByText(all, text, limit) : all.slice(0, limit);
  }

  function codeItems(): Item[] {
    if (!inRepo || branch === undefined || codeKeyword.length < 3) return [];
    if (codePending && !code.data) {
      return [
        {
          key: "code-searching",
          label: t("palette.searching", { repo: repoName }),
          icon: LoaderCircle,
          disabled: true,
          spin: true,
        },
      ];
    }
    const lines = (code.data?.files ?? []).flatMap((file) =>
      file.lines.filter((line) => line.isMatch).map((line) => ({ file, line })),
    );
    return lines.slice(0, prefix === "/" ? 8 : 5).map(({ file, line }) => ({
      key: `code-${file.path}-${line.line}`,
      label: aroundMatch(line.text, code.data!.keyword),
      icon: FileCode,
      meta: `${basename(file.path)}:${line.line}`,
      to: `${repoLink}/src/${refPath(code.data!.ref)}/${refPath(file.path)}#L${line.line}`,
      mark: "text",
      markQuery: code.data!.keyword,
      mono: true,
    }));
  }

  function searchCodeItem(): Item {
    const q = new URLSearchParams({ q: text }).toString();
    return {
      key: "search-code",
      label: t("palette.search_code", { query: text }),
      icon: Search,
      meta: inRepo && branch !== undefined ? t("palette.in_repo", { repo: repoName }) : t("palette.in_all"),
      to: inRepo && branch !== undefined ? `${repoLink}/search/${refPath(branch)}?${q}` : `/search?${q}`,
    };
  }

  function repoCommands(r: RepoHeaderData): Item[] {
    const base = `/${r.owner}/${r.name}`;
    const full = `${r.owner}/${r.name}`;
    const out: Item[] = [];
    const ref = r.defaultBranch;
    if (ref !== undefined) {
      out.push({ key: "r-files", label: t("repo.files"), keywords: "code source browse", icon: Code, to: base });
    }
    if (r.issuesEnabled) out.push({ key: "r-issues", label: t("issues"), icon: CircleDot, to: `${base}/issues` });
    if (r.pullRequestsEnabled) {
      out.push({ key: "r-pulls", label: t("pull_requests"), icon: GitPullRequest, to: `${base}/pulls` });
    }
    if (r.issuesEnabled && user) {
      out.push({ key: "r-new-issue", label: t("repo.issues.new"), icon: Plus, to: `${base}/issues/new` });
    }
    if (ref !== undefined) {
      out.push(
        {
          key: "r-commits",
          label: t("repo.commits"),
          keywords: "history log",
          icon: GitCommitHorizontal,
          to: `${base}/commits/${refPath(ref)}`,
        },
        { key: "r-branches", label: t("repo.branches"), icon: GitBranch, to: `${base}/branches` },
        { key: "r-releases", label: t("repo.releases"), keywords: "tags", icon: Tag, to: `${base}/releases` },
      );
    }
    if (r.wikiEnabled) out.push({ key: "r-wiki", label: t("repo.wiki"), icon: FileText, to: `${base}/wiki` });
    if (r.issuesEnabled) {
      out.push(
        { key: "r-labels", label: t("repo.labels"), icon: Tag, to: `${base}/labels` },
        { key: "r-milestones", label: t("repo.milestones"), icon: Milestone, to: `${base}/milestones` },
      );
    }
    if (ref !== undefined) {
      out.push({ key: "r-forks", label: t("repo.forks"), icon: GitFork, to: `${base}/forks` });
    }
    if (ref !== undefined && r.viewerCanWrite && !r.mirrorOf) {
      out.push(
        { key: "r-new-file", label: t("repo.editor.new_file"), icon: Plus, to: `${base}/_new/${refPath(ref)}` },
        { key: "r-upload", label: t("repo.editor.upload_file"), icon: Upload, to: `${base}/_upload/${refPath(ref)}` },
      );
    }
    if (ref !== undefined && r.viewerCanWrite) {
      out.push({ key: "r-new-release", label: t("repo.release.new_release"), icon: Plus, to: `${base}/releases/new` });
    }
    if (r.wikiEnabled && r.viewerCanWrite) {
      out.push({ key: "r-new-wiki", label: t("repo.wiki.new_page"), icon: Plus, to: `${base}/wiki/_new` });
    }
    const copy = (url: string) => () =>
      void navigator.clipboard.writeText(url).then(
        () => toast.success(t("palette.copied"), { description: url }),
        () => toast.error(t("copy_failed")),
      );
    if (r.cloneLinks?.https) {
      out.push({
        key: "r-clone-https",
        label: t("palette.copy_https"),
        keywords: "git clone url",
        icon: Copy,
        run: copy(r.cloneLinks.https),
      });
    }
    if (r.cloneLinks?.ssh) {
      out.push({
        key: "r-clone-ssh",
        label: t("palette.copy_ssh"),
        keywords: "git clone url",
        icon: Copy,
        run: copy(r.cloneLinks.ssh),
      });
    }
    if (ref !== undefined) {
      out.push({
        key: "r-zip",
        label: t("palette.download_zip"),
        keywords: "archive",
        icon: Download,
        download: `${base}/archive/${refPath(ref)}.zip`,
      });
    }
    if (user) {
      out.push(
        {
          key: "r-star",
          label: r.viewerIsStarring ? t("repo.unstar") : t("repo.star"),
          keywords: "favorite",
          icon: Star,
          run: () => void toggleStar(r, full),
        },
        {
          key: "r-watch",
          label: r.viewerIsWatching ? t("repo.unwatch") : t("repo.watch"),
          keywords: "notifications",
          icon: r.viewerIsWatching ? EyeOff : Eye,
          run: () => void toggleWatch(r, full),
        },
      );
      if (ref !== undefined) {
        out.push({ key: "r-fork", label: t("repo.fork"), icon: GitFork, to: `/repo/fork/${r.id}` });
      }
    }
    if (r.viewerCanAdminister) {
      const page = (key: string, label: string, icon: Icon, path: string, keywords?: string) =>
        out.push({ key, label: t("palette.repo_settings", { page: label }), keywords, icon, to: `${base}${path}` });
      page("r-settings", t("repo.settings.options"), Settings, "/settings", "options rename");
      page("r-collab", t("repo.settings.collaboration"), Users, "/settings/collaboration", "collaborators");
      if (!r.mirrorOf) page("r-protect", t("repo.settings.branches"), ShieldCheck, "/settings/branches", "protection");
      page("r-hooks", t("repo.settings.hooks"), Webhook, "/settings/hooks");
      page("r-keys", t("repo.settings.deploy_keys"), KeyRound, "/settings/keys");
    }
    return out;
  }

  async function toggleStar(r: RepoHeaderData, full: string) {
    const next = !r.viewerIsStarring;
    try {
      const result = next ? await starRepo(r.owner, r.name) : await unstarRepo(r.owner, r.name);
      queryClient.setQueryData(repoHeaderQuery(r.owner, r.name).queryKey, (prev) =>
        prev ? { ...prev, viewerIsStarring: next, starCount: result.starCount } : prev,
      );
      toast.success(t(next ? "palette.starred" : "palette.unstarred", { repo: full }));
    } catch (err) {
      toast.error(t("status.internal_server_error"), { description: (err as Error).message });
    }
  }

  async function toggleWatch(r: RepoHeaderData, full: string) {
    const next = !r.viewerIsWatching;
    try {
      const result = next ? await watchRepo(r.owner, r.name) : await unwatchRepo(r.owner, r.name);
      queryClient.setQueryData(repoHeaderQuery(r.owner, r.name).queryKey, (prev) =>
        prev ? { ...prev, viewerIsWatching: next, watchCount: result.watchCount } : prev,
      );
      toast.success(t(next ? "palette.watching" : "palette.unwatched", { repo: full }));
    } catch (err) {
      toast.error(t("status.internal_server_error"), { description: (err as Error).message });
    }
  }

  function globalCommands(): Item[] {
    const out: Item[] = [];
    if (user) {
      out.push(
        { key: "g-dashboard", label: t("dashboard"), icon: LayoutDashboard, to: "/" },
        { key: "g-issues", label: t("palette.your_issues"), icon: CircleDot, to: "/issues" },
        { key: "g-pulls", label: t("palette.your_pulls"), icon: GitPullRequest, to: "/pulls" },
      );
    } else {
      out.push({ key: "g-home", label: t("home"), icon: House, to: "/" });
    }
    out.push(
      {
        key: "g-explore-repos",
        label: t("palette.explore", { page: t("explore.repos") }),
        icon: Globe,
        to: "/explore/repos",
      },
      {
        key: "g-explore-users",
        label: t("palette.explore", { page: t("explore.users") }),
        icon: Users,
        to: "/explore/users",
      },
      {
        key: "g-explore-orgs",
        label: t("palette.explore", { page: t("explore.organizations") }),
        icon: Building2,
        to: "/explore/organizations",
      },
      { key: "g-search", label: t("repo.code_search.all_repos"), keywords: "code grep", icon: Search, to: "/search" },
    );
    if (user) {
      out.push(
        { key: "g-new-repo", label: t("new_repo"), keywords: "create", icon: Plus, to: "/repo/create" },
        { key: "g-migrate", label: t("new_migrate"), keywords: "import mirror", icon: Import, to: "/repo/migrate" },
      );
      if (user.canCreateOrganization) {
        out.push({ key: "g-new-org", label: t("new_org"), keywords: "create", icon: Plus, to: "/org/create" });
      }
      out.push({ key: "g-profile", label: t("your_profile"), icon: User, to: `/${user.username}` });
      const page = (key: string, label: string, icon: Icon, path: string, keywords?: string) =>
        out.push({ key, label: t("palette.settings_page", { page: label }), keywords, icon, to: path });
      page("s-profile", t("settings.profile"), Settings, "/user/settings", "language");
      page("s-avatar", t("settings.avatar"), User, "/user/settings/avatar");
      page("s-password", t("settings.password"), Lock, "/user/settings/password");
      page("s-email", t("settings.emails"), Settings, "/user/settings/email");
      page("s-ssh", t("settings.ssh_keys"), KeyRound, "/user/settings/ssh");
      page("s-security", t("settings.security"), ShieldCheck, "/user/settings/security", "two-factor 2fa");
      page("s-apps", t("settings.applications"), KeyRound, "/user/settings/applications", "tokens");
      page("s-repos", t("settings.repos"), BookMarked, "/user/settings/repositories");
      page("s-orgs", t("settings.orgs"), Building2, "/user/settings/organizations");
      page("s-delete", t("settings.delete"), Settings, "/user/settings/delete");
      if (user.isAdmin) {
        out.push({ key: "g-admin", label: t("admin_panel"), icon: LayoutDashboard, to: "/admin" });
      }
    } else {
      out.push(
        { key: "g-sign-in", label: t("sign_in"), keywords: "log in", icon: LogIn, to: "/user/sign-in" },
        { key: "g-register", label: t("register"), keywords: "sign up", icon: UserPlus, to: "/user/sign-up" },
      );
    }
    const theme = (key: string, label: string, icon: Icon, value: "light" | "dark" | "system") =>
      out.push({
        key,
        label: t("palette.theme", { theme: label }),
        keywords: "mode appearance",
        icon,
        run: () => setTheme(value),
      });
    theme("g-theme-light", t("theme_light"), Sun, "light");
    theme("g-theme-dark", t("theme_dark"), Moon, "dark");
    theme("g-theme-system", t("theme_system"), Monitor, "system");
    out.push({ key: "g-help", label: t("help"), keywords: "docs", icon: HelpCircle, external: "https://gogs.io" });
    if (user) {
      out.push({
        key: "g-sign-out",
        label: t("sign_out"),
        keywords: "log out",
        icon: LogOut,
        run: () => void signOut(),
      });
    }
    return out;
  }

  function choose(item: Item | undefined) {
    if (!item || item.disabled) return;
    if (item.prefix !== undefined) {
      setInput(item.prefix);
      setActive(0);
      inputRef.current?.focus();
      return;
    }
    close();
    if (item.to !== undefined) void navigate({ href: subUrl(item.to) });
    else if (item.download !== undefined) window.location.assign(subUrl(item.download));
    else if (item.external !== undefined) window.open(item.external, "_blank", "noopener,noreferrer");
    else item.run?.();
  }

  const move = (delta: number) => {
    if (!choosable.length) return;
    const at = activeItem ? choosable.indexOf(activeItem) : 0;
    setActive((at + delta + choosable.length) % choosable.length);
  };

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      move(e.key === "ArrowDown" ? 1 : -1);
    } else if (e.key === "Enter") {
      e.preventDefault();
      choose(activeItem);
    } else if (e.key === "Backspace" && inRepo && e.currentTarget.selectionEnd === 0) {
      e.preventDefault();
      setScoped(false);
      setActive(0);
    }
  };

  const loading = !settled || issues.isFetching || repos.isFetching || people.isFetching;

  return (
    <>
      <div className="flex h-14 shrink-0 items-center gap-2 border-b border-(--color-border) px-3 sm:h-12">
        <Search className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
        {inRepo ? (
          <span className="inline-flex h-6 max-w-[45%] shrink-0 items-center gap-1 rounded-full bg-(--color-surface) pr-0.5 pl-2 text-xs">
            <Folder className="size-3.5 shrink-0 text-(--color-muted-foreground)" aria-hidden />
            <span className="truncate">{repoName}</span>
            <button
              type="button"
              aria-label={t("palette.remove_scope", { repo: repoName })}
              onClick={() => {
                setScoped(false);
                inputRef.current?.focus();
              }}
              className="grid size-6 shrink-0 cursor-pointer place-items-center rounded-full text-(--color-muted-foreground) hover:bg-(--color-border) hover:text-(--color-foreground) focus-visible:outline-2 focus-visible:outline-(--color-ring)"
            >
              <X className="size-3" aria-hidden />
            </button>
          </span>
        ) : null}
        <input
          ref={inputRef}
          type="text"
          role="combobox"
          aria-label={t("palette.title")}
          aria-expanded
          aria-controls={listID}
          aria-autocomplete="list"
          aria-activedescendant={activeItem ? optionID(activeItem) : undefined}
          placeholder={t("palette.placeholder")}
          autoComplete="off"
          autoCapitalize="off"
          spellCheck={false}
          value={input}
          onChange={(e) => {
            setInput(e.target.value);
            setActive(0);
          }}
          onKeyDown={onKeyDown}
          className="h-full min-w-0 flex-1 bg-transparent text-base outline-none placeholder:text-(--color-muted-foreground) sm:text-sm"
        />
        {loading && text ? (
          <LoaderCircle className="size-4 shrink-0 animate-spin text-(--color-muted-foreground)" aria-hidden />
        ) : null}
        <DialogPrimitive.Close className="-mr-1 shrink-0 cursor-pointer rounded-md px-2 py-2 text-sm text-(--color-primary) focus-visible:outline-2 focus-visible:outline-(--color-ring) sm:hidden">
          {t("cancel")}
        </DialogPrimitive.Close>
      </div>

      <div
        id={listID}
        role="listbox"
        aria-label={t("palette.results")}
        className="min-h-0 flex-1 overflow-y-auto overscroll-contain py-1"
      >
        {groups.map((group) => (
          <div key={group.key} role="group" aria-labelledby={group.label ? `${listID}-g-${group.key}` : undefined}>
            {group.label ? (
              <div
                id={`${listID}-g-${group.key}`}
                className="truncate px-3.5 pt-2 pb-1 text-xs text-(--color-muted-foreground)"
              >
                {group.label}
              </div>
            ) : (
              <div className="mx-3.5 my-1 h-px bg-(--color-border)" aria-hidden />
            )}
            {group.items.map((item) => (
              <PaletteRow
                key={item.key}
                id={optionID(item)}
                item={item}
                query={item.markQuery ?? text}
                active={item === activeItem}
                onHover={() => {
                  const at = choosable.indexOf(item);
                  if (at >= 0 && item !== activeItem) setActive(at);
                }}
                onChoose={() => choose(item)}
              />
            ))}
          </div>
        ))}
        {!groups.length && !loading ? (
          <p className="px-3.5 py-4 text-sm text-(--color-muted-foreground)">
            {t("palette.no_results", { query: text })}
          </p>
        ) : null}
      </div>

      <div className="hidden shrink-0 flex-wrap items-center gap-x-4 gap-y-1 border-t border-(--color-border) px-3.5 py-2 text-xs text-(--color-muted-foreground) sm:flex">
        <span className="inline-flex items-center gap-1">
          <Kbd>↑</Kbd>
          <Kbd>↓</Kbd>
          {t("palette.key_move")}
        </span>
        <span className="inline-flex items-center gap-1">
          <Kbd>↵</Kbd>
          {t("palette.key_open")}
        </span>
        <span className="inline-flex items-center gap-1">
          <Kbd>esc</Kbd>
          {t("palette.key_close")}
        </span>
        <span className="ml-auto inline-flex items-center gap-1">
          {PREFIXES.map((p) => (
            <Kbd key={p}>{p}</Kbd>
          ))}
          {t("palette.key_prefixes")}
        </span>
      </div>
    </>
  );
}

function Kbd({ children }: { children: string }) {
  return (
    <kbd className="inline-grid h-5 min-w-5 place-items-center rounded border border-(--color-border) px-1 font-sans text-[11px] leading-none text-(--color-muted-foreground)">
      {children}
    </kbd>
  );
}

function Marked({ segments }: { segments: Segment[] }) {
  return segments.map((segment, i) =>
    segment.match ? (
      <mark key={i} className="rounded-xs bg-(--color-primary)/15 font-semibold text-inherit">
        {segment.text}
      </mark>
    ) : (
      <span key={i}>{segment.text}</span>
    ),
  );
}

function PaletteRow({
  id,
  item,
  query,
  active,
  onHover,
  onChoose,
}: {
  id: string;
  item: Item;
  query: string;
  active: boolean;
  onHover: () => void;
  onChoose: () => void;
}) {
  const RowIcon = item.icon;
  const label =
    item.mark === "path" ? (
      <Marked segments={highlightPath(item.label, query)} />
    ) : item.mark === "text" ? (
      <Marked segments={highlightText(item.label, query)} />
    ) : (
      item.label
    );
  return (
    <div
      id={id}
      role="option"
      aria-selected={active}
      aria-disabled={item.disabled || undefined}
      // Keep the focus in the box when a row is clicked.
      onMouseDown={(e) => e.preventDefault()}
      onMouseMove={onHover}
      onClick={onChoose}
      className={cn(
        "flex min-h-11 items-center gap-2.5 px-3.5 text-sm sm:min-h-9",
        item.disabled ? "text-(--color-muted-foreground)" : "cursor-pointer",
        active && "bg-(--color-surface)",
      )}
    >
      {item.glyph ? (
        <span className="w-4 shrink-0 text-center font-mono text-sm font-semibold" aria-hidden>
          {item.glyph}
        </span>
      ) : (
        <span className="grid w-4 shrink-0 place-items-center text-(--color-muted-foreground)">
          <RowIcon className={cn("size-4", item.spin && "animate-spin")} aria-hidden />
        </span>
      )}
      <span className={cn("min-w-0 flex-1 truncate", item.mono && "font-mono text-xs")}>{label}</span>
      {item.meta ? (
        <span className="max-w-[45%] shrink-0 truncate text-xs text-(--color-muted-foreground)">{item.meta}</span>
      ) : null}
      {active && !item.disabled ? (
        <span className="hidden sm:inline" aria-hidden>
          <Kbd>↵</Kbd>
        </span>
      ) : null}
    </div>
  );
}
