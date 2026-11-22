// Search-param schema for the user profile route. Defined here (and not in the
// page component) so both the route definition and the page can import from it.

export type ProfileListTab = "followers" | "following";

// The repositories tab and the first page stay implicit, so they never appear
// in the URL. Any other `tab` value shows the repositories.
export interface ProfileSearch {
  tab?: "activity" | ProfileListTab;
  page?: number;
}

// Both keys are always present, even when undefined, so that malformed values
// from the URL do not leak through from the unvalidated root search.
export function validateProfileSearch(search: Record<string, unknown>): ProfileSearch {
  // A repeated parameter arrives as an array. Use the first value, like the server.
  const first = (value: unknown) => (Array.isArray(value) ? (value as unknown[])[0] : value);
  const tab = first(search.tab);
  if (tab === "activity") return { tab: "activity", page: undefined };
  const page = Number(first(search.page));
  return {
    tab: tab === "followers" || tab === "following" ? tab : undefined,
    page: Number.isInteger(page) && page > 1 ? page : undefined,
  };
}
