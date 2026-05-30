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

export interface RepoCommitSearch {
  whitespace?: WhitespaceUrlValue;
  style?: DiffStyleUrlValue;
  wrap?: true;
}

export function validateRepoCommitSearch(search: Record<string, unknown>): RepoCommitSearch {
  const out: RepoCommitSearch = {};
  if (search.whitespace === "ignore-all" || search.whitespace === "ignore-change") {
    out.whitespace = search.whitespace;
  }
  if (search.style === "split") out.style = "split";
  if (search.wrap === true || search.wrap === "true") out.wrap = true;
  return out;
}
