import { expect, type Page, test } from "@playwright/test";

const localDate = (offset = 0) => {
  const date = new Date();
  date.setDate(date.getDate() + offset);
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
};
async function signIn(page: Page, project: string, other = false) {
  const response = await page.request.post("/auth/dev/login", {
    data: {
      email: `e2e-search-${other ? "other-" : ""}${project}@example.com`,
      name: "Search tester",
    },
  });
  expect(response.status()).toBe(204);
}
async function rpc<T>(page: Page, method: string, data: unknown): Promise<T> {
  const response = await page.request.post(`/api/ledger.v1.${method}`, { data });
  expect(response.ok(), await response.text()).toBe(true);
  return response.json();
}
async function fixture(page: Page, prefix: string) {
  const { wallet: usd } = await rpc<{ wallet: { id: string; name: string } }>(
    page,
    "WalletService/CreateWallet",
    { name: `Search USD ${prefix}`, currency: "USD" },
  );
  const { wallet: uyu } = await rpc<{ wallet: { id: string; name: string } }>(
    page,
    "WalletService/CreateWallet",
    { name: `Search UYU ${prefix}`, currency: "UYU" },
  );
  const { categories } = await rpc<{ categories: { id: string; name: string }[] }>(
    page,
    "CategoryService/ListCategories",
    {},
  );
  const categoryId = categories.find((category) => category.name === "Groceries")?.id;
  expect(categoryId).toBeTruthy();
  const { label } = await rpc<{ label: { id: string; name: string } }>(
    page,
    "LabelService/CreateLabel",
    { name: `Shared ${prefix}`, color: "cat-teal" },
  );
  const create = (
    walletId: string,
    note: string,
    amountMinor = "1234",
    labelIds: string[] = [],
    occurredOn = localDate(),
  ) =>
    rpc(page, "TransactionService/CreateTransaction", {
      walletId,
      categoryId,
      amountMinor,
      note,
      labelIds,
      occurredOn,
      today: localDate(),
    });
  return { usd, uyu, label, create };
}

test("search crosses wallets, combines filters and survives reload and browser navigation", async ({
  page,
}, info) => {
  if (info.project.name === "mobile") await page.setViewportSize({ width: 360, height: 800 });
  await signIn(page, info.project.name);
  const tag = Math.random().toString(36).slice(2, 9);
  const { usd, uyu, label, create } = await fixture(page, tag);
  await create(usd.id, `Market ${tag} one`, "1234", [label.id]);
  await create(uyu.id, `Market ${tag} two`, "25000");
  await create(usd.id, `Market ${tag} future`, "900", [], localDate(1));
  await page.goto("/wallets");
  await page.getByRole("link", { name: /Search$/ }).click();
  await expect(page.getByRole("heading", { name: "Transactions", exact: true })).toBeVisible();
  await page.getByLabel("To", { exact: true }).fill(localDate(1));
  await page.getByLabel("Search notes", { exact: true }).fill(`Market ${tag}`);
  await page.getByRole("button", { name: "Search transactions", exact: true }).click();
  const results = page.getByRole("region", { name: "Search results" });
  await expect(results.getByRole("listitem")).toHaveCount(2);
  await expect(results).toContainText("12.34 USD");
  await expect(results).toContainText("250.00 UYU");
  await expect(results).not.toContainText(`Market ${tag} future`);
  await expect(page.getByRole("link", { name: /Search$/ })).toHaveAttribute("aria-current", "page");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  if (info.project.name === "desktop") {
    const viewport = page.viewportSize();
    for (const width of [640, 768]) {
      await page.setViewportSize({ width, height: 900 });
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
    }
    if (viewport) await page.setViewportSize(viewport);
  }
  const walletRight = await page
    .getByRole("link", { name: /Wallets$/ })
    .evaluate((link) => link.getBoundingClientRect().right);
  const accountLeft = await page
    .getByRole("button", { name: "Account menu" })
    .evaluate((button) => button.getBoundingClientRect().left);
  expect(walletRight).toBeLessThanOrEqual(accountLeft);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({ path: info.outputPath("transactions.png"), fullPage: true });

  await page.getByRole("combobox", { name: "Wallets", exact: true }).click();
  await page.getByRole("option", { name: new RegExp(usd.name) }).click();
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: "Search transactions", exact: true }).click();
  await expect(results.getByRole("listitem")).toHaveCount(1);
  await expect(page).toHaveURL(new RegExp(`wallets=${usd.id}`));
  await page.reload();
  await expect(page.getByLabel("Search notes", { exact: true })).toHaveValue(`Market ${tag}`);
  await expect(page.getByRole("combobox", { name: "Wallets", exact: true })).toContainText(
    usd.name,
  );
  await expect(results.getByRole("listitem")).toHaveCount(1);
  await page.goBack();
  await expect(results.getByRole("listitem")).toHaveCount(2);
  await expect(page.getByRole("combobox", { name: "Wallets", exact: true })).toContainText(
    "All wallets",
  );

  for (const [name, option] of [
    ["Labels", label.name],
    ["Categories", "Groceries"],
    ["Type", "Expenses"],
  ]) {
    await page.getByRole("combobox", { name, exact: true }).click();
    await page.getByRole("option", { name: new RegExp(`^${option}`) }).click();
    await page.keyboard.press("Escape");
  }
  await page.getByRole("button", { name: "Search transactions", exact: true }).click();
  await expect(results.getByRole("listitem")).toHaveCount(1);
  await expect(results).toContainText(`Market ${tag} one`);
  await results.getByRole("link", { name: usd.name, exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/wallets/${usd.id}`));
  await expect(page.getByRole("heading", { name: usd.name, exact: true })).toBeVisible();
  await page.goBack();
  await expect(results.getByRole("listitem")).toHaveCount(1);
  await page.getByLabel("Search notes", { exact: true }).fill(`missing-${tag}`);
  await page.getByRole("button", { name: "Search transactions", exact: true }).click();
  await expect(results).toContainText("No transactions match your search.");
  await page.getByRole("button", { name: "Clear filters", exact: true }).click();
  await expect(page.getByLabel("Search notes", { exact: true })).toHaveValue("");
});

test("pagination has no duplicate rows and a new search starts at the first page", async ({
  page,
}, info) => {
  await signIn(page, info.project.name);
  const tag = Math.random().toString(36).slice(2, 9);
  const { usd, create } = await fixture(page, tag);
  for (let index = 0; index < 51; index++) await create(usd.id, `Page ${tag} row ${index}`);
  await page.goto(`/transactions?from=${localDate()}&to=${localDate()}&note=Page+${tag}`);
  const results = page.getByRole("region", { name: "Search results" });
  await expect(results.getByRole("listitem")).toHaveCount(50);
  await page.getByRole("button", { name: "Load more transactions", exact: true }).click();
  await expect(results.getByRole("listitem")).toHaveCount(51);
  expect(new Set(await results.getByRole("listitem").allTextContents()).size).toBe(51);
  await expect(
    page.getByRole("button", { name: "Load more transactions", exact: true }),
  ).toHaveCount(0);
  await page.getByLabel("Search notes", { exact: true }).fill(`Page ${tag} row 50`);
  await page.getByRole("button", { name: "Search transactions", exact: true }).click();
  await expect(results.getByRole("listitem")).toHaveCount(1);
  await expect(results).toContainText(`Page ${tag} row 50`);
});

test("another household cannot find the first household's transactions", async ({ page }, info) => {
  await signIn(page, info.project.name);
  const tag = Math.random().toString(36).slice(2, 9);
  const { usd, create } = await fixture(page, tag);
  await create(usd.id, `Private ${tag}`);
  const url = `/transactions?from=${localDate()}&to=${localDate()}&wallets=${usd.id}&note=Private+${tag}`;
  await page.goto(url);
  await expect(
    page.getByRole("region", { name: "Search results" }).getByRole("listitem"),
  ).toHaveCount(1);
  await signIn(page, info.project.name, true);
  await page.reload();
  await expect(page.getByRole("region", { name: "Search results" })).toContainText(
    "No transactions match your search.",
  );
  await expect(page.getByText(`Private ${tag}`, { exact: true })).toHaveCount(0);
});

test("signed-out search visitors are sent to sign in", async ({ page }) => {
  await page.goto("/transactions?note=market");
  await expect(page).toHaveURL(/\/login\?next=/);
});
