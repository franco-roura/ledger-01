import { describe, expect, test } from "bun:test";
import { formatAmountInput, formatMoney, minorToDecimal, parseAmount } from "./money";

describe("minorToDecimal", () => {
  test("round trips non-negative amounts without losing precision at each currency scale", () => {
    for (const scale of [0, 2, 3, 4]) {
      for (const amount of [0n, 1n, 99n, 123456n, 900719925474099312n]) {
        expect(parseAmount(minorToDecimal(amount, scale), scale)).toBe(amount);
      }
    }
  });
  test("handles scale, sign and zero decimals", () => {
    expect(minorToDecimal(123456n, 2)).toBe("1234.56");
    expect(minorToDecimal(-5n, 2)).toBe("-0.05");
    expect(minorToDecimal(1866n, 0)).toBe("1866");
  });
});

describe("parseAmount", () => {
  test("accepts plain and grouped decimals", () => {
    expect(parseAmount("1,234.5", 2)).toBe(123450n);
    expect(parseAmount("0.99", 2)).toBe(99n);
    expect(parseAmount(".5", 2)).toBe(50n);
    expect(parseAmount("42", 0)).toBe(42n);
  });

  test("rejects too many decimals, signs and junk", () => {
    expect(parseAmount("1.234", 2)).toBeNull();
    expect(parseAmount("-1", 2)).toBeNull();
    expect(parseAmount("1e3", 2)).toBeNull();
    expect(parseAmount("", 2)).toBeNull();
  });
});

describe("formatMoney", () => {
  test("suppresses signs and currency codes independently", () => {
    expect(formatMoney(-12345n, "USD", 2, { sign: "never", code: false })).toBe("123.45");
    expect(formatMoney(0n, "JPY", 0, { sign: "always" })).toBe("0 JPY");
    expect(formatMoney(1234n, "KWD", 3)).toBe("1.234 KWD");
  });
  test("formats like the app", () => {
    expect(formatMoney(123456n, "USD", 2, { sign: "always" })).toBe("+1,234.56 USD");
    expect(formatMoney(-4205n, "USD", 2)).toBe("−42.05 USD");
    expect(formatMoney(0n, "USD", 2, { sign: "always" })).toBe("0.00 USD");
    expect(formatMoney(-470000n, "UYU", 2, { code: false })).toBe("−4,700.00");
  });

  test("keeps precision for large amounts", () => {
    expect(formatMoney(900719925474099312n, "USD", 2)).toBe("9,007,199,254,740,993.12 USD");
  });
});

test("input formatting distinguishes an empty value from zero", () => {
  expect(formatAmountInput(null, 2)).toBe("");
  expect(formatAmountInput(0n, 2)).toBe("0.00");
  expect(formatAmountInput(-1n, 3)).toBe("-0.001");
});
