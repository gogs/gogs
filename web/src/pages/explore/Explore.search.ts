import { type ListSearch, validateListSearch } from "@/lib/page-search";

// Route param and search-param schema for the explore pages. Defined here (and
// not in the page component) so both the route definition and the page can
// import from it.

export const EXPLORE_KINDS = ["repos", "users", "organizations"] as const;
export type ExploreKind = (typeof EXPLORE_KINDS)[number];

export function isExploreKind(value: string): value is ExploreKind {
  return (EXPLORE_KINDS as readonly string[]).includes(value);
}

export type ExploreSearch = ListSearch;

export const validateExploreSearch = validateListSearch;
