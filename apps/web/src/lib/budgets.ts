import { formatMoney } from "@franco_roura/module-01/finance";
import { BudgetRepeat, BudgetState } from "@ledger-01/api";

/** Repeat choices in the order the form offers them. */
export const repeatOptions: { value: BudgetRepeat; label: string }[] = [
  { value: BudgetRepeat.MONTHLY, label: "Every month" },
  { value: BudgetRepeat.WEEKLY, label: "Every week" },
  { value: BudgetRepeat.BIWEEKLY, label: "Every 2 weeks" },
  { value: BudgetRepeat.YEARLY, label: "Every year" },
  { value: BudgetRepeat.DAILY, label: "Every day" },
  { value: BudgetRepeat.ONCE, label: "Once" },
];

const short: Record<BudgetRepeat, string> = {
  [BudgetRepeat.UNSPECIFIED]: "",
  [BudgetRepeat.ONCE]: "Once",
  [BudgetRepeat.DAILY]: "Daily",
  [BudgetRepeat.WEEKLY]: "Weekly",
  [BudgetRepeat.BIWEEKLY]: "Biweekly",
  [BudgetRepeat.MONTHLY]: "Monthly",
  [BudgetRepeat.YEARLY]: "Yearly",
};

/** One-word repeat for HUD serials ("Monthly"). */
export const repeatShort = (r: BudgetRepeat) => short[r];

type ProgressLike = { state: BudgetState; daysTotal: number; daysLeft: number };

/**
 * Where today sits in the period, 0–1, at the middle of today's day: undefined when today is
 * outside the period (no marker).
 */
export function todayShare(p: ProgressLike): number | undefined {
  if (p.state !== BudgetState.ACTIVE || p.daysTotal <= 0) return undefined;
  return (p.daysTotal - p.daysLeft + 0.5) / p.daysTotal;
}

/** Plain-words status for a period, e.g. "10 days left", "Starts in 3 days", "Ended". */
export function periodStatus(p: ProgressLike): string {
  switch (p.state) {
    case BudgetState.ACTIVE:
      return p.daysLeft === 1 ? "Last day" : `${p.daysLeft} days left`;
    case BudgetState.UPCOMING:
      return "Not started";
    default:
      return "Ended";
  }
}

/**
 * Meter values for a period's spending against its limit. With rollover a limit can be zero or
 * negative, which the meter can't scale to: show it full, and over when anything is spent past it.
 */
export function meterValues(spent: bigint, limit: bigint): { spent: bigint; budget: bigint } {
  if (limit > 0n) return { spent, budget: limit };
  return spent > limit ? { spent: 2n, budget: 1n } : { spent: 1n, budget: 1n };
}

/**
 * The base amount and what carried in, e.g. "100.00 USD + 20.00 USD carried"; undefined when
 * nothing carried.
 */
export function carryText(
  base: bigint,
  carried: bigint,
  currency: string,
  minorUnits: number,
): string | undefined {
  if (carried === 0n) return undefined;
  const sign = carried > 0n ? "+" : "−";
  const abs = carried > 0n ? carried : -carried;
  return `${formatMoney(base, currency, minorUnits)} ${sign} ${formatMoney(abs, currency, minorUnits)} carried`;
}

/** What a budget carries between periods, e.g. "Carries leftover"; undefined without rollover. */
export function rolloverText(b: {
  carryLeftover: boolean;
  carryOverspend: boolean;
}): string | undefined {
  if (b.carryLeftover && b.carryOverspend) return "Carries leftover and overspend";
  if (b.carryLeftover) return "Carries leftover";
  if (b.carryOverspend) return "Carries overspend";
  return undefined;
}
