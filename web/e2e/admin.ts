import { AxeBuilder } from "@axe-core/playwright";
import { type Page, expect } from "@playwright/test";

// ADMIN is the account `server.sh` creates.
export const ADMIN = { username: "e2e-admin", password: "e2e-password-1234" };

// PAGES are the pages of the admin console that the sidebar leads to, by the
// name of their link in English.
export const PAGES = [
  { path: "/admin", name: "Overview" },
  { path: "/admin/users", name: "Users" },
  { path: "/admin/orgs", name: "Organizations" },
  { path: "/admin/repos", name: "Repositories" },
  { path: "/admin/auths", name: "Authentication" },
  { path: "/admin/config", name: "Configuration" },
  { path: "/admin/health", name: "Server health" },
  { path: "/admin/traces", name: "Traces" },
  { path: "/admin/notices", name: "System notices" },
  { path: "/admin/operations", name: "Operations" },
  { path: "/admin/jobs", name: "Background jobs" },
];

// signIn authenticates the browser without going through the form, which
// `admin.spec.ts` covers once. The request asks in the language of the
// browser, as the form would, since the server keeps to the language of the
// first request.
export async function signIn(page: Page, locale = "en-US") {
  const res = await page.request.post("/api/web/user/sign-in", {
    data: ADMIN,
    headers: { "Accept-Language": locale },
  });
  expect(res.status()).toBe(200);
}

// watchErrors collects what a browser reports as broken while a test uses the
// page: uncaught exceptions and errors logged to the console.
export function watchErrors(page: Page) {
  const errors: string[] = [];
  page.on("pageerror", (err) => errors.push(err.message));
  page.on("console", (msg) => {
    if (msg.type() === "error") errors.push(msg.text());
  });
  return errors;
}

// expectNoSidewaysScroll fails when something is wider than the screen.
export async function expectNoSidewaysScroll(page: Page) {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  expect(overflow, "the page scrolls sideways").toBeLessThanOrEqual(0);
}

// expectAccessible runs the automated checks of WCAG 2.2 AA, e.g., names of
// controls and contrast. They stand in for what a screen reader user would
// run into, and do not replace trying a page with a screen reader.
//
// They do not run in Firefox, whose page process dies of a segmentation fault
// in about one scan out of ten (Playwright 1.65, Firefox 1572). What they
// check is in the markup and the styles, which the other engines get as well.
export async function expectAccessible(page: Page) {
  if (page.context().browser()?.browserType().name() === "firefox") return;
  const { violations } = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
    .analyze();
  expect(violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
}

// openFromSidebar follows a link of the sidebar, which is a sheet behind a
// button on narrow screens.
export async function openFromSidebar(page: Page, name: string) {
  const toggle = page.getByRole("banner").getByRole("button", { name: "Toggle sidebar" });
  const nav = page.getByRole("navigation", { name: "Admin panel" });
  if (!(await nav.isVisible())) await toggle.click();
  await nav.getByRole("link", { name, exact: true }).click();
}
