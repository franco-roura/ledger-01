import { expect, type Page, test } from "@playwright/test";

const email = process.env.E2E_EMAIL ?? "e2e@example.com";

async function signIn(page: Page) {
  const res = await page.request.post("/auth/dev/login", { data: { email, name: "E2E Tester" } });
  expect(res.status()).toBe(204);
}

async function rpc<T>(page: Page, method: string, data: unknown): Promise<T> {
  const res = await page.request.post(`/api/ledger.v1.${method}`, { data });
  expect(res.ok(), await res.text()).toBe(true);
  return res.json();
}

const localDate = (offsetDays = 0) => {
  const d = new Date();
  d.setDate(d.getDate() + offsetDays);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
};

test("wallet types, a valuation and the net worth page", async ({ page }, info) => {
  const tag = `${info.project.name} ${Date.now() % 100000}`;
  const cardName = `E2E Card ${tag}`;
  const brokerName = `E2E Brokerage ${tag}`;
  await signIn(page);

  // A credit card from the dialog: its starting figure is what it owes.
  await page.goto("/wallets");
  await page.getByRole("button", { name: "Add wallet" }).first().click();
  await page.getByLabel("Name").fill(cardName);
  await page.getByLabel("Type").click();
  await page.getByRole("option", { name: "Credit card" }).click();
  await page.getByLabel("Amount owed (optional)").fill("300");
  await page.getByRole("button", { name: "Create wallet" }).click();
  await expect(page).toHaveURL(/\/wallets\/[0-9a-f-]{36}/);
  await expect(page.getByRole("heading", { name: cardName })).toBeVisible();
  const cardId = page.url().match(/\/wallets\/([0-9a-f-]{36})/)?.[1];

  // A charge still to come on the card: upcoming debt, not spending.
  const { categories } = await rpc<{ categories: { id: string; name: string }[] }>(
    page,
    "CategoryService/ListCategories",
    {},
  );
  const groceries = categories.find((c) => c.name === "Groceries");
  await rpc(page, "TransactionService/CreateTransaction", {
    walletId: cardId,
    categoryId: groceries?.id,
    amountMinor: "12000",
    occurredOn: localDate(10),
    note: "Installment",
    today: localDate(),
  });

  // An investment valued from its settings.
  const { wallet: broker } = await rpc<{ wallet: { id: string } }>(
    page,
    "WalletService/CreateWallet",
    { name: brokerName, currency: "USD", type: "WALLET_TYPE_INVESTMENT" },
  );
  await page.goto(`/wallets/${broker.id}/settings`);
  await page.getByRole("button", { name: "Update value" }).click();
  await page.getByRole("textbox", { name: "Value" }).fill("1500");
  await page.getByRole("button", { name: "Save value" }).click();
  await expect(page.getByText(/Valued on/)).toBeVisible();

  await page.goto("/net-worth");
  await expect(page.getByRole("heading", { name: "Net worth" })).toBeVisible();
  const card = page.getByRole("link", { name: new RegExp(cardName) });
  await expect(card).toContainText(/300\.00 USD/);
  await expect(card).toContainText(/120\.00\s*upcoming/);
  await expect(page.getByRole("link", { name: new RegExp(brokerName) })).toContainText(
    /1,500\.00 USD/,
  );
  const upcoming = page
    .locator("section, div")
    .filter({ has: page.getByRole("heading", { name: "Upcoming installments by category" }) })
    .last();
  await expect(upcoming).toContainText("Groceries");

  // The dashboard links to it.
  await page.goto("/dashboard");
  await page
    .getByRole("link", { name: /^Net worth/ })
    .first()
    .click();
  await expect(page).toHaveURL(/\/net-worth/);
});
