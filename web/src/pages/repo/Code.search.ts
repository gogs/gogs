// Search-param schema for the code browser. Defined here so both the route
// definitions and the page can import it without a circular dependency.
//
// `tab` picks the file shown below the file list, e.g., "license-ov-file", with
// the same values as GitHub. The first tab stays implicit.
export interface RepoCodeSearch {
  tab?: string;
}

export function validateRepoCodeSearch(search: Record<string, unknown>): RepoCodeSearch {
  return { tab: typeof search.tab === "string" && search.tab ? search.tab : undefined };
}
