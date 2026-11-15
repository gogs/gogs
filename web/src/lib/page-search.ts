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

// Search-param schema for the branches page, which GitHub searches through
// `query` rather than `q`.
export interface BranchesSearch extends PageSearch {
  query?: string;
}

export function validateBranchesSearch(search: Record<string, unknown>): BranchesSearch {
  const { q, page } = validateListSearch({ q: search.query, page: search.page });
  return { query: q, page };
}

// validateFlag reads an on/off search param, which only appears in the URL
// when it is on, as `true`.
export function validateFlag(value: unknown): true | undefined {
  return value === true || value === "true" ? true : undefined;
}

// Search params of the admin dashboard. The last hour of server health stays
// implicit.
export interface DashboardSearch {
  health?: "24h";
}

export function validateDashboardSearch(search: Record<string, unknown>): DashboardSearch {
  return { health: search.health === "24h" ? "24h" : undefined };
}

// Search params of the traces of the admin panel. The overview of the last
// hour stays implicit. The other params filter the search.
export interface TraceSearch {
  view?: "search";
  range?: "15m";
  q?: string;
  route?: string;
  minMs?: number;
  status?: "5xx" | "4xx";
  sort?: "slowest";
  // A window of start times, as Unix milliseconds.
  from?: number;
  to?: number;
}

export function validateTraceSearch(search: Record<string, unknown>): TraceSearch {
  const first = (value: unknown) => (Array.isArray(value) ? (value as unknown[])[0] : value);
  const text = (value: unknown) => {
    const v = first(value);
    // The router turns URL values like `?q=1` into numbers, see validateListSearch.
    const s = v === undefined ? "" : typeof v === "string" ? v : JSON.stringify(v);
    return s.trim() || undefined;
  };
  const positive = (value: unknown) => {
    const n = Number(first(value));
    return Number.isFinite(n) && n > 0 ? n : undefined;
  };
  // The server reads times as whole Unix milliseconds.
  const unixMs = (value: unknown) => {
    const n = positive(value);
    return n !== undefined && Number.isInteger(n) ? n : undefined;
  };
  const status = first(search.status);
  return {
    view: first(search.view) === "search" ? "search" : undefined,
    range: first(search.range) === "15m" ? "15m" : undefined,
    q: text(search.q),
    route: text(search.route),
    minMs: positive(search.minMs),
    status: status === "5xx" || status === "4xx" ? status : undefined,
    sort: first(search.sort) === "slowest" ? "slowest" : undefined,
    from: unixMs(search.from),
    to: unixMs(search.to),
  };
}
