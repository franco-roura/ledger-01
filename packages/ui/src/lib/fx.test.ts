import { describe, expect, test } from "bun:test";
import { divRoundHalfEven, editFx, fxResult, initialFxState, parseRate } from "./fx";

const USD_UYU = { fromMinorUnits: 2, toMinorUnits: 2 };
const USD_JPY = { fromMinorUnits: 2, toMinorUnits: 0 };

describe("editFx touch order", () => {
  test("typing from then to derives the rate", () => {
    let s = initialFxState();
    s = editFx(s, "from", "100.00", USD_UYU);
    s = editFx(s, "to", "3905.00", USD_UYU);
    expect(s.rate).toBe("39.050000");
    expect(s.ratePrecise).toBe("39.0500000000");
  });

  test("then editing the rate recomputes from (the oldest of the other two)", () => {
    let s = initialFxState();
    s = editFx(s, "from", "100.00", USD_UYU);
    s = editFx(s, "to", "3905.00", USD_UYU);
    s = editFx(s, "rate", "39.10", USD_UYU);
    expect(s.order).toEqual(["from", "to", "rate"]);
    expect(s.from).toBe("99.87");
    expect(s.to).toBe("3905.00");
  });

  test("then editing from recomputes to (oldest of to, rate)", () => {
    let s = initialFxState();
    s = editFx(s, "from", "100.00", USD_UYU);
    s = editFx(s, "to", "3905.00", USD_UYU);
    s = editFx(s, "rate", "39.10", USD_UYU);
    s = editFx(s, "from", "200", USD_UYU);
    expect(s.to).toBe("7820.00");
    expect(s.rate).toBe("39.10");
  });

  test("typing only the rate and from fills to", () => {
    let s = initialFxState();
    s = editFx(s, "rate", "40", USD_UYU);
    expect(s.to).toBe(""); // from still empty
    s = editFx(s, "from", "10.5", USD_UYU);
    // order was [to, from, rate] after the rate edit; editing from recomputes to
    expect(s.to).toBe("420.00");
  });

  test("respects different decimal places", () => {
    let s = initialFxState();
    s = editFx(s, "rate", "151.234", USD_JPY);
    s = editFx(s, "from", "12.34", USD_JPY);
    expect(s.to).toBe("1866"); // 12.34 × 151.234 = 1866.227…
  });

  test("invalid input clears the computed field", () => {
    let s = initialFxState();
    s = editFx(s, "from", "100", USD_UYU);
    s = editFx(s, "to", "abc", USD_UYU);
    expect(s.rate).toBe("");
    expect(fxResult(s, USD_UYU)).toBeNull();
  });
});

describe("fxResult", () => {
  test.each(["from", "to", "rate"] as const)(
    "refuses a decimal-comma value in the %s field",
    (field) => {
      let s = initialFxState();
      s = editFx(s, "from", "100", USD_UYU);
      s = editFx(s, "to", "4000", USD_UYU);
      s = editFx(s, field, "12,50", USD_UYU);
      expect(fxResult(s, USD_UYU)).toBeNull();
    },
  );

  test("returns minor units and the precise rate", () => {
    let s = initialFxState();
    s = editFx(s, "from", "120.00", USD_UYU);
    s = editFx(s, "to", "4700.00", USD_UYU);
    expect(fxResult(s, USD_UYU)).toEqual({
      fromMinor: 12000n,
      toMinor: 470000n,
      rate: "39.1666666667",
    });
  });
});

describe("helpers", () => {
  test.each(["12,50", "1,2,3", "1234,567", "1,,234", ",123", "123,", "1.234,56", "1,234.5,6"])(
    "parseRate rejects ambiguous or malformed comma input %s",
    (input) => {
      expect(parseRate(input)).toBeNull();
    },
  );

  test("parseRate keeps valid grouping, decimal notation and precision", () => {
    expect(parseRate(" 1,234,567.89 ")).toBe(12345678900000000n);
    expect(parseRate(".5")).toBe(5000000000n);
    expect(parseRate("42.")).toBe(420000000000n);
    expect(parseRate("1,234.12345678909")).toBe(12341234567890n);
  });

  test("divRoundHalfEven", () => {
    expect(divRoundHalfEven(5n, 2n)).toBe(2n);
    expect(divRoundHalfEven(7n, 2n)).toBe(4n);
    expect(divRoundHalfEven(10n, 3n)).toBe(3n);
    expect(divRoundHalfEven(20n, 3n)).toBe(7n);
  });

  test("parseRate rejects zero and garbage", () => {
    expect(parseRate("0")).toBeNull();
    expect(parseRate("-1")).toBeNull();
    expect(parseRate("x")).toBeNull();
    expect(parseRate("39.05")).toBe(390500000000n);
  });
});
