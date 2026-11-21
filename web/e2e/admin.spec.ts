import { expect, test } from "@playwright/test";

import { ADMIN, PAGES, expectAccessible, expectNoSidewaysScroll, openFromSidebar, signIn, watchErrors } from "./admin";

test("the sign-in form leads an admin to the admin panel they asked for", async ({ page }) => {
  await page.goto("/admin/users");
  await expect(page).toHaveURL(/\/user\/sign-in/);
  // A browser that knows passkeys is offered one first.
  const username = page.locator('input[name="username"]');
  const withPassword = page.getByRole("button", { name: "Sign in with password" });
  await expect(username.or(withPassword).first()).toBeVisible();
  if (await withPassword.isVisible()) await withPassword.click();
  await username.fill(ADMIN.username);
  await page.locator('input[name="password"]').fill(ADMIN.password);
  await page.locator('input[name="password"]').press("Enter");
  await expect(page).toHaveURL(/\/admin\/users$/);
  await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toHaveText("Users");
});

test.describe("as an admin", () => {
  test.beforeEach(async ({ page }) => {
    await signIn(page);
  });

  for (const { path, name } of PAGES) {
    test(`${name} renders, fits the screen and passes the accessibility checks`, async ({ page }) => {
      const errors = watchErrors(page);
      await page.goto(path);
      await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
      // The cards of a page answer one by one, each behind a skeleton.
      await expect(page.getByRole("main").locator('[aria-busy=true], [class*="animate-pulse"]')).toHaveCount(0);
      await expectNoSidewaysScroll(page);
      await expectAccessible(page);
      expect(errors).toEqual([]);
    });
  }

  test("the sidebar leads to every page and marks the one shown", async ({ page }) => {
    await page.goto("/admin");
    for (const { path, name } of PAGES) {
      await openFromSidebar(page, name);
      await expect(page).toHaveURL(new RegExp(`${path}$`));
      // On a narrow screen the sidebar is a sheet, which closes once its link is followed.
      await expect(page.getByRole("dialog")).toHaveCount(0);
    }
    await page.goBack();
    await expect(page).toHaveURL(/\/admin\/operations$/);
  });

  test("the command palette takes the keyboard and opens the page picked", async ({ page, isMobile }) => {
    await page.goto("/admin/config");
    const button = page.getByRole("banner").getByRole("button", { name: /search/i });
    if (isMobile) {
      await button.click();
    } else {
      // The button names the shortcut of the platform, e.g., Meta+K on a Mac.
      await page.keyboard.press((await button.getAttribute("aria-keyshortcuts")) ?? "");
    }
    const input = page.getByRole("dialog").getByRole("combobox");
    await expect(input).toBeFocused();
    await input.fill("Admin panel");
    await input.press("Enter");
    await expect(page).toHaveURL(/\/admin$/);
    await expect(page.getByRole("dialog")).toHaveCount(0);
  });

  test("Server health shows placeholders, not old numbers, while it loads again", async ({ page }) => {
    await page.goto("/admin/health");
    const main = page.getByRole("main");
    await expect(main.getByRole("heading", { level: 1 })).toHaveText("Server health");
    await openFromSidebar(page, "Background jobs");
    await expect(page).toHaveURL(/\/admin\/jobs$/);

    let release = () => {};
    const held = new Promise<void>((resolve) => (release = resolve));
    await page.route("**/api/web/admin/health*", async (route) => {
      await held;
      await route.continue();
    });
    await page.goBack();
    await expect(main.locator("[aria-busy=true]")).toBeVisible();
    await expect(main.getByRole("heading", { level: 1 })).toHaveText("Server health");
    release();
    await expect(main.locator("[aria-busy=true]")).toHaveCount(0);
  });

  test("a button pressed after the session expired leads to sign in and back", async ({ page, context }) => {
    await page.goto("/admin/operations");
    const run = page.getByRole("main").getByRole("button", { name: /^Run/ }).first();
    await expect(run).toBeVisible();
    await context.clearCookies();
    await run.click();
    const confirm = page.getByRole("dialog").getByRole("button", { name: /^Run/ });
    if (await confirm.isVisible()) await confirm.click();
    await expect(page).toHaveURL(/\/user\/sign-in\?.*admin%2Foperations/);
  });
});
