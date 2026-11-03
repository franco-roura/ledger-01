import { describe, expect, test } from "bun:test";
import { currentMonth, parsePeriod, shiftPeriod } from "./period";

describe("shiftPeriod", () => {
  test("preserves calendar months across leap years", () => {
    expect(shiftPeriod({ from: "2024-01-01", to: "2024-01-31" }, 1)).toEqual({
      from: "2024-02-01",
      to: "2024-02-29",
    });
    expect(shiftPeriod({ from: "2025-03-01", to: "2025-03-31" }, -1)).toEqual({
      from: "2025-02-01",
      to: "2025-02-28",
    });
  });

  test("round trips an arbitrary span crossing daylight saving and a year boundary", () => {
    for (const period of [
      { from: "2026-03-06", to: "2026-03-10" },
      { from: "2026-12-29", to: "2027-01-03" },
      { from: "2024-02-29", to: "2024-02-29" },
    ]) {
      expect(shiftPeriod(shiftPeriod(period, 1), -1)).toEqual(period);
    }
  });
  test("steps whole months", () => {
    expect(shiftPeriod({ from: "2026-10-01", to: "2026-10-31" }, -1)).toEqual({
      from: "2026-09-01",
      to: "2026-09-30",
    });
    expect(shiftPeriod({ from: "2026-12-01", to: "2026-12-31" }, 1)).toEqual({
      from: "2027-01-01",
      to: "2027-01-31",
    });
    expect(shiftPeriod({ from: "2026-01-01", to: "2026-03-31" }, 1)).toEqual({
      from: "2026-04-01",
      to: "2026-06-30",
    });
  });

  test("steps arbitrary ranges by their length", () => {
    expect(shiftPeriod({ from: "2026-10-05", to: "2026-10-11" }, 1)).toEqual({
      from: "2026-10-12",
      to: "2026-10-18",
    });
  });
});

describe("parsePeriod", () => {
  test("falls back to the current month", () => {
    expect(parsePeriod(new URLSearchParams("from=bad"))).toEqual(currentMonth());
    expect(parsePeriod(new URLSearchParams("from=2026-10-31&to=2026-10-01"))).toEqual(
      currentMonth(),
    );
    expect(parsePeriod(new URLSearchParams("from=2026-10-01&to=2026-10-31"))).toEqual({
      from: "2026-10-01",
      to: "2026-10-31",
    });
  });
});
