import { describe, expect, test } from "bun:test";
import { amountBound, parseReportState, reportQuery } from "./report-state";

describe("report state", () => {
  test("shared report links round trip all explicit filters and encode note text", () => {
    const params = new URLSearchParams("from=2026-10-01&to=2026-10-31&currency=UYU&bucket=week");
    const state = {
      ...parseReportState(params, "USD"),
      walletIds: ["w1", "w2"],
      categoryIds: ["c1"],
      note: "coffee & lunch + tips",
      min: "0",
      max: "250.50",
    };
    const query = reportQuery(state, params, false);
    expect(parseReportState(new URLSearchParams(query.toString()), "USD")).toEqual(state);
    expect(params.has("wallets")).toBe(false);
  });

  test("cleared filters are removed from shared links", () => {
    const params = new URLSearchParams(
      "from=2026-10-01&to=2026-10-31&wallets=w1&categories=c1&note=test&min=1&max=2",
    );
    const state = {
      ...parseReportState(params, "USD"),
      walletIds: [],
      categoryIds: [],
      note: "  ",
      min: "",
      max: "",
    };
    expect(reportQuery(state, params, false).toString()).toBe("from=2026-10-01&to=2026-10-31");
    expect(amountBound("0", 2)).toBe(0n);
  });
  test("reads filters from the URL, with the default currency", () => {
    const s = parseReportState(
      new URLSearchParams(
        "from=2026-10-01&to=2026-10-31&wallets=a,b&categories=c&note=grocer&min=10&max=",
      ),
      "USD",
    );
    expect(s).toMatchObject({
      period: { from: "2026-10-01", to: "2026-10-31" },
      walletIds: ["a", "b"],
      categoryIds: ["c"],
      currency: "USD",
      note: "grocer",
      min: "10",
      max: "",
    });
    expect(parseReportState(new URLSearchParams("currency=UYU"), "USD").currency).toBe("UYU");
  });

  test("writes only what is set, keeping an explicit currency", () => {
    const url = new URLSearchParams("from=2026-10-01&to=2026-10-31&currency=UYU");
    const s = { ...parseReportState(url, "USD"), note: "  coffee ", max: "50" };
    expect(reportQuery(s, url, false).toString()).toBe(
      "from=2026-10-01&to=2026-10-31&currency=UYU&note=coffee&max=50",
    );
    // A default currency and bucket stay out of the link until picked.
    const plain = new URLSearchParams("from=2026-10-01&to=2026-10-31");
    const d = parseReportState(plain, "USD");
    expect(reportQuery(d, plain, false).toString()).toBe("from=2026-10-01&to=2026-10-31");
    expect(reportQuery(d, plain, true).get("bucket")).toBe(d.bucket);
  });

  test("amount bounds", () => {
    expect(amountBound("12.5", 2)).toBe(1250n);
    expect(amountBound("1,200", 0)).toBe(1200n);
    expect(amountBound("", 2)).toBeUndefined();
    expect(amountBound("12.345", 2)).toBeUndefined();
  });
});
