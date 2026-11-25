import { describe, expect, test } from "bun:test";
import { BudgetState } from "@ledger-01/api";
import { carryText, meterValues, periodStatus, rolloverText, todayShare } from "./budgets";

describe("budgets", () => {
  test("today marker sits in the middle of today's day", () => {
    expect(todayShare({ state: BudgetState.ACTIVE, daysTotal: 10, daysLeft: 10 })).toBe(0.05);
    expect(todayShare({ state: BudgetState.ACTIVE, daysTotal: 10, daysLeft: 1 })).toBe(0.95);
    expect(
      todayShare({ state: BudgetState.UPCOMING, daysTotal: 10, daysLeft: 10 }),
    ).toBeUndefined();
    expect(todayShare({ state: BudgetState.ENDED, daysTotal: 10, daysLeft: 0 })).toBeUndefined();
  });

  test("status words", () => {
    expect(periodStatus({ state: BudgetState.ACTIVE, daysTotal: 31, daysLeft: 10 })).toBe(
      "10 days left",
    );
    expect(periodStatus({ state: BudgetState.ACTIVE, daysTotal: 31, daysLeft: 1 })).toBe(
      "Last day",
    );
    expect(periodStatus({ state: BudgetState.UPCOMING, daysTotal: 31, daysLeft: 31 })).toBe(
      "Not started",
    );
    expect(periodStatus({ state: BudgetState.ENDED, daysTotal: 31, daysLeft: 0 })).toBe("Ended");
  });

  test("meter values for zero and negative limits", () => {
    expect(meterValues(30n, 100n)).toEqual({ spent: 30n, budget: 100n });
    expect(meterValues(0n, 0n)).toEqual({ spent: 1n, budget: 1n });
    expect(meterValues(0n, -50n)).toEqual({ spent: 2n, budget: 1n });
    expect(meterValues(10n, 0n)).toEqual({ spent: 2n, budget: 1n });
  });

  test("carry text", () => {
    expect(carryText(10000n, 0n, "USD", 2)).toBeUndefined();
    expect(carryText(10000n, 2000n, "USD", 2)).toBe("100.00 USD + 20.00 USD carried");
    expect(carryText(10000n, -1550n, "USD", 2)).toBe("100.00 USD − 15.50 USD carried");
  });

  test("rollover text", () => {
    expect(rolloverText({ carryLeftover: false, carryOverspend: false })).toBeUndefined();
    expect(rolloverText({ carryLeftover: true, carryOverspend: false })).toBe("Carries leftover");
    expect(rolloverText({ carryLeftover: true, carryOverspend: true })).toBe(
      "Carries leftover and overspend",
    );
  });
});
