// Search-param schema for the issues and pull requests dashboards. Defined here
// (and not in the page component) so both the route definitions and the page
// can import from it.

export const ISSUE_SORTS = ["latest", "oldest", "recentupdate", "leastupdate", "mostcomment", "leastcomment"] as const;
export type IssueSort = (typeof ISSUE_SORTS)[number];

// Defaults stay implicit, so they never appear in the URL: issues in your
// repositories, open, newest first, the first page.
export interface DashboardIssuesSearch {
  type?: "assigned" | "created_by";
  repo?: number;
  state?: "closed";
  sort?: Exclude<IssueSort, "latest">;
  page?: number;
}

export function validateDashboardIssuesSearch(search: Record<string, unknown>): DashboardIssuesSearch {
  // A repeated parameter arrives as an array. Use the first value, like the server.
  const first = (value: unknown) => (Array.isArray(value) ? (value as unknown[])[0] : value);
  const positive = (value: unknown) => {
    const n = Number(first(value));
    return Number.isInteger(n) && n > 0 ? n : undefined;
  };
  const type = first(search.type);
  const sort = first(search.sort);
  const page = positive(search.page);
  return {
    type: type === "assigned" || type === "created_by" ? type : undefined,
    repo: positive(search.repo),
    state: first(search.state) === "closed" ? "closed" : undefined,
    sort:
      sort !== "latest" && ISSUE_SORTS.includes(sort as IssueSort)
        ? (sort as DashboardIssuesSearch["sort"])
        : undefined,
    page: page && page > 1 ? page : undefined,
  };
}
