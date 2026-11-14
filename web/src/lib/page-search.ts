// Search-param schema for pages whose only parameter is the page number. The
// first page stays implicit, so it never appears in the URL.
export interface PageSearch {
  page?: number;
}

export function validatePageSearch(search: Record<string, unknown>): PageSearch {
  // A repeated parameter arrives as an array. Use the first value, like the server.
  const value: unknown = Array.isArray(search.page) ? (search.page as unknown[])[0] : search.page;
  const page = Number(value);
  return { page: Number.isInteger(page) && page > 1 ? page : undefined };
}

// Search-param schema for paginated lists that can be searched by keyword. An
// empty keyword stays implicit too.
export interface ListSearch extends PageSearch {
  q?: string;
}

// Both keys are always present, even when undefined, so that malformed values
// from the URL do not leak through from the unvalidated root search.
export function validateListSearch(search: Record<string, unknown>): ListSearch {
  // A repeated parameter arrives as an array. Use the first value, like the server.
  const first = (value: unknown) => (Array.isArray(value) ? (value as unknown[])[0] : value);
  const rawQ = first(search.q);
  // The router turns URL values like `?q=1` or `?q=true` into numbers and booleans,
  // and only when that keeps their exact text, so serializing them restores it.
  const q = rawQ === undefined ? "" : typeof rawQ === "string" ? rawQ : JSON.stringify(rawQ);
  return { q: q || undefined, ...validatePageSearch(search) };
}
