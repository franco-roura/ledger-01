import { describe, expect, test } from "bun:test";
import { TransactionKind } from "@ledger-01/api";
import { currentMonth } from "./period";
import {
  parseTransactionSearch,
  transactionSearchInput,
  transactionSearchQuery,
  validSearchPeriod,
} from "./transaction-search";

const wallet = "00000000-0000-4000-8000-000000000001";
const category = "00000000-0000-4000-8000-000000000002";
const label = "00000000-0000-4000-8000-000000000003";

describe("transaction search state", () => {
  test("saved links restore every filter and retain safe request values", () => {
    const state = parseTransactionSearch(
      new URLSearchParams({
        from: "2026-10-01",
        to: "2026-10-31",
        note: "Market & café",
        wallets: wallet,
        categories: category,
        labels: label,
        kinds: "expense,transfer_out",
      }),
    );
    expect(parseTransactionSearch(transactionSearchQuery(state))).toEqual(state);
    expect(transactionSearchInput(state, "2026-10-15")).toEqual({
      period: state.period,
      noteQuery: "Market & café",
      walletIds: [wallet],
      categoryIds: [category],
      labelIds: [label],
      kinds: [TransactionKind.EXPENSE, TransactionKind.TRANSFER_OUT],
      today: "2026-10-15",
    });
  });

  test("invalid identifiers and kinds are discarded without losing valid selections", () => {
    const state = parseTransactionSearch(
      new URLSearchParams({
        wallets: `${wallet},invalid,${wallet}`,
        categories: "invalid",
        labels: label,
        kinds: "expense,unknown,expense,__proto__",
      }),
    );
    expect(state.walletIds).toEqual([wallet]);
    expect(state.categoryIds).toEqual([]);
    expect(state.kinds).toEqual(["expense"]);
  });

  test("empty selections mean all and pagination tokens never survive a filter change", () => {
    const state = parseTransactionSearch(
      new URLSearchParams("note=++Market++&page_token=old-cursor"),
    );
    const query = transactionSearchQuery(state);
    expect(query.get("note")).toBe("Market");
    expect(query.has("page_token")).toBe(false);
    expect(query.has("wallets")).toBe(false);
    expect(transactionSearchInput(state, "2026-10-15").kinds).toEqual([]);
  });

  test.each([
    { from: "2026-02-30", to: "2026-03-01" },
    { from: "2026-10-31", to: "2026-10-01" },
    { from: "2000-01-01", to: "2026-01-01" },
    { from: "junk", to: "2026-10-01" },
  ])("rejects invalid or excessive date ranges %j", (period) => {
    expect(validSearchPeriod(period)).toBe(false);
    expect(parseTransactionSearch(new URLSearchParams(period)).period).toEqual(currentMonth());
  });

  test("accepts a real leap day and a one-day period", () => {
    expect(validSearchPeriod({ from: "2024-02-29", to: "2024-02-29" })).toBe(true);
  });
});
