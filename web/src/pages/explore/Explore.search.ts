// Route param and search-param schema for the explore pages. Defined here (and
// not in the page component) so both the route definition and the page can
// import from it.

export const EXPLORE_KINDS = ["repos", "users", "organizations"] as const;
export type ExploreKind = (typeof EXPLORE_KINDS)[number];

export function isExploreKind(value: string): value is ExploreKind {
  return (EXPLORE_KINDS as readonly string[]).includes(value);
}

// The first page and an empty keyword stay implicit, so they never appear in
// the URL.
export interface ExploreSearch {
  q?: string;
  page?: number;
}

// Both keys are always present, even when undefined, so that malformed values
// from the URL do not leak through from the unvalidated root search.
export function validateExploreSearch(search: Record<string, unknown>): ExploreSearch {
  // A repeated parameter arrives as an array. Use the first value, like the server.
  const first = (value: unknown) => (Array.isArray(value) ? (value as unknown[])[0] : value);
  const rawQ = first(search.q);
  // The router turns URL values like `?q=1` or `?q=true` into numbers and booleans,
  // and only when that keeps their exact text, so serializing them restores it.
  const q = rawQ === undefined ? "" : typeof rawQ === "string" ? rawQ : JSON.stringify(rawQ);
  const page = Number(first(search.page));
  return { q: q || undefined, page: Number.isInteger(page) && page > 1 ? page : undefined };
}
