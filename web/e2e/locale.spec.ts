import { expect, test } from "@playwright/test";

import { PAGES, expectNoSidewaysScroll, signIn, watchErrors } from "./admin";

test.beforeEach(async ({ page, locale }) => {
  await signIn(page, locale);
});

for (const { path, name } of PAGES) {
  test(`${name} renders in German and fits a phone`, async ({ page }) => {
    const errors = watchErrors(page);
    await page.goto(path);
    await expect(page.locator("html")).toHaveAttribute("lang", "de-DE");
    await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
    // A text without a translation shows in English, never as the key that names it.
    await expect(page.getByRole("main")).not.toContainText(/\badmin\.[a-z_.]+\b/);
    await expectNoSidewaysScroll(page);
    expect(errors).toEqual([]);
  });
}

test("the sidebar shows the translated names of the pages", async ({ page }) => {
  await page.goto("/admin");
  await page.getByRole("banner").getByRole("button").first().click();
  await expect(page.getByRole("dialog").getByRole("link", { name: "Benutzer", exact: true })).toBeVisible();
});
