// Search-param schema for the commit diff route. Defined here (and not in
// router.tsx) so both the route definition and the page component can import
// from it without forming a circular dependency through router.tsx.
//
// All diff page toggles serialize through the URL so the view is shareable
// and survives reload. Defaults are implicit (omitted from the URL).
//
// `w` rides with the loader because whitespace handling lives in `git diff`,
// not in the parsed-patch client code. Keep `WhitespaceMode` in sync with
// `whitespaceFlag` in `internal/route/repo/commit.go`.

// URL-side whitespace value: the page treats `undefined` as "show", so it
// never appears in the URL. The toolbar uses a richer enum (see DiffToolbar)
// that includes "show" as an explicit option for the radio UI.
export type WhitespaceUrlValue = "ignore-all" | "ignore-change";
// `unified` is the default and stays implicit. Only `split` ever appears.
export type DiffStyleUrlValue = "split";
export type DiffFileStatus = "added" | "modified" | "deleted" | "renamed";

export const ALL_STATUSES: readonly DiffFileStatus[] = ["added", "modified", "deleted", "renamed"];

export interface CommitDiffSearch {
  whitespace?: WhitespaceUrlValue;
  style?: DiffStyleUrlValue;
  wrap?: true;
  // Comma-separated list of enabled statuses, e.g. "added,modified". The
  // string form keeps the URL human-readable (`?status=added,modified`) and
  // sidesteps TanStack's default array stringification.
  status?: string;
}

export function parseStatusFilter(raw: string | undefined): Record<DiffFileStatus, boolean> {
  if (!raw) {
    return { added: true, modified: true, deleted: true, renamed: true };
  }
  const enabled = new Set(
    raw.split(",").filter((s): s is DiffFileStatus => (ALL_STATUSES as readonly string[]).includes(s)),
  );
  return {
    added: enabled.has("added"),
    modified: enabled.has("modified"),
    deleted: enabled.has("deleted"),
    renamed: enabled.has("renamed"),
  };
}

// Serialize a filter map back to the URL string form. Returns undefined when
// every status is enabled, so the URL stays clean by omitting the default.
export function serializeStatusFilter(filter: Record<DiffFileStatus, boolean>): string | undefined {
  const enabled = ALL_STATUSES.filter((k) => filter[k]);
  if (enabled.length === ALL_STATUSES.length) return undefined;
  return enabled.join(",");
}

export function normalizeStatusParam(raw: unknown): string | undefined {
  if (typeof raw !== "string" || raw === "") return undefined;
  const parts = raw.split(",").filter((s): s is DiffFileStatus => (ALL_STATUSES as readonly string[]).includes(s));
  if (parts.length === 0 || parts.length === ALL_STATUSES.length) return undefined;
  // Re-emit in canonical order so the URL is stable regardless of input
  // order and users can't smuggle in arbitrary strings via the URL.
  return ALL_STATUSES.filter((s) => parts.includes(s)).join(",");
}

export function validateCommitDiffSearch(search: Record<string, unknown>): CommitDiffSearch {
  const out: CommitDiffSearch = {};
  if (search.whitespace === "ignore-all" || search.whitespace === "ignore-change") {
    out.whitespace = search.whitespace;
  }
  if (search.style === "split") out.style = "split";
  if (search.wrap === true || search.wrap === "true") out.wrap = true;
  const status = normalizeStatusParam(search.status);
  if (status) out.status = status;
  return out;
}
