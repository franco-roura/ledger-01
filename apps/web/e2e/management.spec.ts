import { expect, type Page, test } from "@playwright/test";

async function rpc(page: Page, method: string, data: Record<string, unknown> = {}) {
  const response = await page.request.post(`/api/ledger.v1.${method}`, { data });
  expect(response.ok(), `${method}: ${await response.text()}`).toBe(true);
  return response.json();
}

test.beforeEach(async ({ page }, info) => {
  const response = await page.request.post("/auth/dev/login", {
    data: { email: `e2e-management-${info.project.name}@example.com`, name: "E2E Management" },
  });
  expect(response.status()).toBe(204);
});

test("labels: create, cancel editing, rename, reload and delete", async ({ page }) => {
  const name = `Trip ${Date.now()}`;
  const renamed = `Holiday ${Date.now()}`;
  await page.goto("/settings/labels");
  await expect(page.getByRole("button", { name: "Add label" })).toBeDisabled();
  await page.getByLabel("New label", { exact: true }).fill(name);
  await page.getByRole("button", { name: "Add label" }).click();
  await page.getByRole("button", { name: `Rename ${name}`, exact: true }).click();
  await page.getByRole("textbox", { name: "Label name" }).fill("Discard this edit");
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page.getByRole("button", { name: `Rename ${name}`, exact: true })).toBeVisible();
  await page.getByRole("button", { name: `Rename ${name}`, exact: true }).click();
  await page.getByRole("textbox", { name: "Label name" }).fill(renamed);
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.getByRole("button", { name: `Rename ${renamed}`, exact: true })).toBeVisible();
  await page.reload();
  await page.getByRole("button", { name: `Delete ${renamed}`, exact: true }).click();
  await expect(page.getByRole("button", { name: `Rename ${renamed}`, exact: true })).toHaveCount(0);
  const { labels = [] } = await rpc(page, "LabelService/ListLabels");
  expect(
    labels.some((label: { name: string }) => label.name === renamed || label.name === name),
  ).toBe(false);
});

test("wallet settings: rename, hide, restore visibility and cancel deletion", async ({ page }) => {
  const name = `Settings ${Date.now()}`;
  const renamed = `Renamed ${Date.now()}`;
  const { wallet } = await rpc(page, "WalletService/CreateWallet", {
    name,
    currency: "USD",
    initialBalanceMinor: "12345",
  });
  await page.goto(`/wallets/${wallet.id}/settings`);
  await expect(page.getByLabel("Wallet name")).toHaveValue(name);
  await page.getByLabel("Wallet name").fill(renamed);
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.getByRole("button", { name: "Save", exact: true })).toBeDisabled();
  const hidden = page.getByRole("checkbox", {
    name: "Hide this wallet from the wallet list and totals",
  });
  await hidden.click();
  await expect(hidden).toBeChecked();
  await page.reload();
  await expect(hidden).toBeChecked();
  expect(
    (await rpc(page, "WalletService/ListWallets")).wallets?.some(
      (w: { id: string }) => w.id === wallet.id,
    ) ?? false,
  ).toBe(false);
  await hidden.click();
  await expect(hidden).not.toBeChecked();
  await page.getByRole("button", { name: "Delete wallet", exact: true }).click();
  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toContainText(`Delete ${renamed}?`);
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  expect((await rpc(page, "WalletService/GetWallet", { id: wallet.id })).wallet.balanceMinor).toBe(
    "12345",
  );
  await page.getByRole("button", { name: "Delete wallet", exact: true }).click();
  await dialog.getByRole("button", { name: "Delete", exact: true }).click();
  await expect(page).toHaveURL(/\/wallets$/);
  const deleted = await page.request.post("/api/ledger.v1.WalletService/GetWallet", {
    data: { id: wallet.id },
  });
  expect(deleted.status()).toBe(404);
});

test("logout invalidates the session and protects private pages", async ({ page }) => {
  await page.goto("/wallets");
  await page.getByRole("button", { name: "Account menu" }).click();
  await page.getByRole("menuitem", { name: "Sign out" }).click();
  await expect(page).toHaveURL(/\/login/);
  await expect(page.getByRole("link", { name: "Continue with Google" })).toBeVisible();
  const me = await page.request.post("/api/ledger.v1.UserService/GetMe", { data: {} });
  expect(me.status()).toBe(401);
});

test("invalid backups cannot enable restoration or change existing balances", async ({ page }) => {
  const { wallet } = await rpc(page, "WalletService/CreateWallet", {
    name: `Backup guard ${Date.now()}`,
    currency: "USD",
    initialBalanceMinor: "54321",
  });
  await page.goto("/settings/data");
  await page.getByLabel("Backup file").setInputFiles({
    name: "invalid.json",
    mimeType: "application/json",
    buffer: Buffer.from('{"format":"other"}'),
  });
  await expect(page.getByText("This isn't a Ledger/01 backup.", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Restore backup", exact: true })).toBeDisabled();
  expect((await rpc(page, "WalletService/GetWallet", { id: wallet.id })).wallet.balanceMinor).toBe(
    "54321",
  );
});
