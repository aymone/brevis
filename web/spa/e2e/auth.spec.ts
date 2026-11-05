import { expect, test } from "@playwright/test";

// The sign-in flow against an engine that has a credential, which the open console the other specs use does not.
// E2E_AUTH_URL is a second engine (same database, no scheduler) started with BREVIS_AUTH_USER, a password hash
// and a secret; E2E_AUTH_USER and E2E_AUTH_PASSWORD are the credential (see the e2e job in test.yml).
const URL = process.env.E2E_AUTH_URL;
const USER = process.env.E2E_AUTH_USER ?? "operator";
const PASSWORD = process.env.E2E_AUTH_PASSWORD ?? "";

test.skip(!URL && !process.env.E2E_REQUIRE, "no engine with a credential (E2E_AUTH_URL)");
test.beforeAll(() => {
  if (!URL) throw new Error("E2E_REQUIRE is set and E2E_AUTH_URL is not: the sign-in flow would go untested");
});
test.use({ baseURL: URL });

async function signIn(page: import("@playwright/test").Page, password = PASSWORD) {
  await page.getByLabel("Username").fill(USER);
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.locator("form").getByRole("button", { name: "Sign in" }).click();
}

test("a signed-out page goes to the login with where it was going, and the API answers 401", async ({ page, request }) => {
  await page.goto("/runs?state=failed");
  await expect(page).toHaveURL(/\/login\?next=%2Fruns%3Fstate%3Dfailed$/);
  await expect(page.locator("form").getByRole("button", { name: "Sign in" })).toBeVisible();
  const res = await request.get("/api/session");
  expect(res.status()).toBe(401);
  expect(await res.json()).toEqual({ error: "sign in" });
});

test("a wrong password says so and stays on the login", async ({ page }) => {
  await page.goto("/login");
  await signIn(page, "not the password at all");
  // The sentence is the server's (login_api.go), in English whatever the page's language; the catalog's
  // login.failed is only for an answer with no text.
  await expect(page.getByText("Invalid username or password.")).toBeVisible();
  await expect(page).toHaveURL(/\/login$/);
});

test("the right password goes where the page was going, then Sign out comes back to the login", async ({ page, request }) => {
  await page.goto("/runs?state=failed");
  await signIn(page);
  await expect(page).toHaveURL(/\/runs\?state=failed$/);
  await expect(page.locator("nav a[href='/workflows']").first()).toBeVisible();
  expect((await page.request.get("/api/session")).status()).toBe(200);

  await page.locator("summary").filter({ hasText: USER }).click();
  await page.getByRole("button", { name: "Sign out" }).click();
  await expect(page).toHaveURL(/\/login$/);
  expect((await request.get("/api/session")).status()).toBe(401);
  await page.goto("/workflows");
  await expect(page).toHaveURL(/\/login\?next=%2Fworkflows$/);
});

test("a next that leaves this site is ignored", async ({ page }) => {
  await page.goto("/login?next=https%3A%2F%2Fevil.example%2F");
  await signIn(page);
  await expect(page).toHaveURL(new RegExp(`^${URL!.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}/?$`));
});
