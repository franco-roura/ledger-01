import { describe, expect, test } from "bun:test";
import { WalletType } from "@ledger-01/api";
import { byClass, isLiability, takesValuations, walletTypeLabel } from "./wallet-types";

describe("wallet types", () => {
  test("labels, with unspecified read as a bank account", () => {
    expect(walletTypeLabel(WalletType.CREDIT_CARD)).toBe("Credit card");
    expect(walletTypeLabel(WalletType.UNSPECIFIED)).toBe("Bank account");
  });

  test("classes", () => {
    expect(isLiability(WalletType.LOAN)).toBe(true);
    expect(isLiability(WalletType.CASH)).toBe(false);
    expect(takesValuations(WalletType.PROPERTY)).toBe(true);
    expect(takesValuations(WalletType.CREDIT_CARD)).toBe(false);
  });

  test("byClass keeps order", () => {
    const ws = [
      { id: "a", type: WalletType.CREDIT_CARD },
      { id: "b", type: WalletType.BANK },
      { id: "c", type: WalletType.LOAN },
      { id: "d", type: WalletType.INVESTMENT },
    ];
    const { assets, liabilities } = byClass(ws);
    expect(assets.map((w) => w.id)).toEqual(["b", "d"]);
    expect(liabilities.map((w) => w.id)).toEqual(["a", "c"]);
  });
});
