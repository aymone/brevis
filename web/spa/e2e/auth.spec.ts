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
  // By id and by type, not by label: the labels are in the page's language.
  await page.locator("#username").fill(USER);
  await page.locator("#password").fill(password);
  await page.locator("form button[type=submit]").click();
}

test("a signed-out page goes to the login with where it was going, and the API answers 401", async ({ page, request }) => {
  await page.goto("/runs?state=failed");
  await expect(page).toHaveURL(/\/login\?next=%2Fruns%3Fstate%3Dfailed$/);
  await expect(page.locator("form").getByRole("button", { name: "Sign in" })).toBeVisible();
  const res = await request.get("/api/session");
  expect(res.status()).toBe(401);
  expect(await res.json()).toEqual({ error: "sign_in_required" });
});

test("a wrong password says so and stays on the login", async ({ page }) => {
  await page.goto("/login");
  await signIn(page, "not the password at all");
  // The server answers with a code (invalid_credentials) and the screen says it from the catalog.
  await expect(page.getByText("Invalid username or password.")).toBeVisible();
  await expect(page).toHaveURL(/\/login$/);
});

test("the refusal is said in the reader's language: the server sends a code, not a sentence", async ({ page, context }) => {
  await context.addCookies([{ name: "brevis_lang", value: "pt-BR", url: URL! }]);
  await page.goto("/login");
  await signIn(page, "not the password at all");
  await expect(page.getByText("Usuário ou senha inválidos.")).toBeVisible();
  await expect(page.getByText("Invalid username or password.")).toHaveCount(0);
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
