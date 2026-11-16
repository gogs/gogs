import type { Completion, CompletionContext, CompletionResult, CompletionSource } from "@codemirror/autocomplete";
import { t } from "i18next";

import { isMarkdownText } from "@/lib/emoji-completion";
import { subUrl } from "@/lib/url";

export interface RepoRef {
  owner: string;
  name: string;
}

interface MentionSuggestion {
  name: string;
  fullName: string;
  avatarURL: string;
}

interface IssueSuggestion {
  index: number;
  title: string;
  isPull: boolean;
  isClosed: boolean;
}

// ReferenceCompletion is a completion that shows the avatar of a user or the
// state of an issue in front of its label.
export interface ReferenceCompletion extends Completion {
  avatarURL?: string;
  issue?: IssueSuggestion;
}

async function fetchItems<T>(url: string, signal?: AbortSignal) {
  const res = await fetch(subUrl(url), { credentials: "same-origin", signal });
  if (!res.ok) throw new Error(`load suggestions: ${res.status}`);
  return ((await res.json()) as { items: T[] }).items;
}

// matchReference matches the reference being typed before the cursor, when it
// starts a line or follows one of the characters allowed before it, in
// Markdown text.
function matchReference(context: CompletionContext, typing: RegExp, before: RegExp) {
  const typed = context.matchBefore(typing);
  if (!typed) return null;
  if (typed.from > context.state.doc.lineAt(typed.from).from) {
    if (!before.test(context.state.sliceDoc(typed.from - 1, typed.from))) return null;
  }
  if (!isMarkdownText(context.state, typed.from)) return null;
  return typed;
}

const mentionables = new Map<string, Promise<ReferenceCompletion[]>>();

// mentionOptions loads the users of the repository on the first mention. A
// failed load is tried again on the next one.
function mentionOptions(repo: RepoRef) {
  const key = `${repo.owner}/${repo.name}`;
  let options = mentionables.get(key);
  if (!options) {
    options = fetchItems<MentionSuggestion>(`/api/web/${key}/suggestions/mentions`)
      .then((users) =>
        users
          .map((u) => ({ label: `@${u.name}`, detail: u.fullName || undefined, avatarURL: u.avatarURL }))
          .sort((a, b) => a.label.localeCompare(b.label)),
      )
      .catch((err: unknown) => {
        mentionables.delete(key);
        throw err;
      });
    mentionables.set(key, options);
  }
  return options;
}

// mentionCompletion completes mentions of the users who can read the
// repository, e.g., "@al" to "@alice". Usernames that start with what was
// typed come first, then usernames and full names that contain it.
function mentionCompletion(repo: RepoRef): CompletionSource {
  return async (context) => {
    const typed = matchReference(context, /@[\w.-]*$/, /\W/);
    if (!typed) return null;

    const query = typed.text.slice(1).toLowerCase();
    // A source that rejects is not queried again until the cursor moves, so a
    // failed load shows no completions instead.
    const all = await mentionOptions(repo).catch(() => null);
    if (!all || context.aborted) return null;
    const prefix: ReferenceCompletion[] = [];
    const inside: ReferenceCompletion[] = [];
    for (const option of all) {
      const at = option.label.toLowerCase().indexOf(query, 1);
      if (at === 1) prefix.push(option);
      else if (at > 1 || option.detail?.toLowerCase().includes(query)) inside.push(option);
    }
    if (prefix.length + inside.length === 0) return null;
    return { from: typed.from, options: [...prefix, ...inside], filter: false };
  };
}

// issueCompletion completes references to the issues and pull requests of the
// repository, e.g., "#12" or "#login" to "#123". The server matches them by
// number and title.
function issueCompletion(repo: RepoRef): CompletionSource {
  return async (context): Promise<CompletionResult | null> => {
    const typed = matchReference(context, /#[\p{L}\p{N}_-]*$/u, /[ ([]/);
    if (!typed) return null;

    const abort = new AbortController();
    context.addEventListener("abort", () => abort.abort(), { onDocChange: true });
    const query = encodeURIComponent(typed.text.slice(1));
    const issues = await fetchItems<IssueSuggestion>(
      `/api/web/${repo.owner}/${repo.name}/suggestions/issues?q=${query}`,
      abort.signal,
    ).catch(() => null);
    if (!issues?.length || context.aborted) return null;
    return {
      from: typed.from,
      options: issues.map((issue) => ({ label: `#${issue.index}`, detail: issue.title, issue })),
      filter: false,
    };
  };
}

// referenceCompletions completes mentions and issue references in the Markdown
// of the repository.
export function referenceCompletions(repo: RepoRef): CompletionSource[] {
  return [mentionCompletion(repo), issueCompletion(repo)];
}

// The icons of IssueListItem, from Lucide.
const ISSUE_ICONS = {
  open: '<circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="1"/>',
  closed: '<circle cx="12" cy="12" r="10"/><path d="m9 12 2 2 4-4"/>',
  openPull:
    '<circle cx="18" cy="18" r="3"/><circle cx="6" cy="6" r="3"/><path d="M13 6h3a2 2 0 0 1 2 2v7"/><line x1="6" x2="6" y1="9" y2="21"/>',
  closedPull:
    '<circle cx="6" cy="6" r="3"/><path d="M6 9v12"/><path d="m21 3-6 6"/><path d="m21 9-6-6"/><path d="M18 11.5V15"/><circle cx="18" cy="18" r="3"/>',
};

// renderReferenceIcon renders the avatar of a user or the state of an issue in
// front of the label of a completion.
export function renderReferenceIcon(completion: ReferenceCompletion) {
  if (completion.avatarURL) {
    const img = document.createElement("img");
    img.src = completion.avatarURL;
    img.alt = "";
    img.className = "cm-referenceIcon cm-referenceAvatar";
    return img;
  }
  if (completion.issue) {
    const { isPull, isClosed } = completion.issue;
    const icon = isPull ? (isClosed ? "closedPull" : "openPull") : isClosed ? "closed" : "open";
    const span = document.createElement("span");
    span.className = `cm-referenceIcon ${isClosed ? "cm-referenceClosed" : "cm-referenceOpen"}`;
    span.role = "img";
    span.ariaLabel = t(isClosed ? "repo.issues.closed_title" : "repo.issues.open_title");
    span.innerHTML = `<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${ISSUE_ICONS[icon]}</svg>`;
    return span;
  }
  return null;
}
