export interface RepoReleasesSearch {
  // after is the tag the page starts after.
  after?: string;
}

export function validateRepoReleasesSearch(search: Record<string, unknown>): RepoReleasesSearch {
  return { after: typeof search.after === "string" && search.after ? search.after : undefined };
}
