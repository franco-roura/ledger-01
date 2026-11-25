"use client";

import { Panel } from "@franco_roura/module-01";
import { DonutChart } from "@franco_roura/module-01/charts";
import { Amount, CategoryIcon } from "@franco_roura/module-01/finance";
import type { CategoryTotal } from "@ledger-01/api";
import Link from "next/link";
import type { ReactNode } from "react";
import { useCategories, useCurrencies } from "@/lib/api/hooks";

/**
 * A donut of category totals with a ranked list below it. Rows link to `href(categoryId)` when
 * given. Amounts are shown negative when `negative` (money going out).
 */
export function CategoryBreakdown({
  serial,
  title,
  centerLabel,
  empty,
  totals,
  currency,
  negative,
  href,
}: {
  serial: string;
  title: string;
  centerLabel: string;
  empty: string;
  totals: CategoryTotal[];
  currency: string;
  negative: boolean;
  href?: (categoryId: string) => string;
}) {
  const { minorUnits } = useCurrencies();
  const { byId: categories } = useCategories();
  const money = { currency, minorUnits: minorUnits(currency) };
  const total = totals.reduce((s, t) => s + t.amountMinor, 0n);

  return (
    <Panel serial={serial} bodyClassName="p-5 pt-11 sm:p-6 sm:pt-11">
      <div className="flex flex-col gap-5">
        <h2 className="font-display text-display-sm text-ink">{title}</h2>
        {totals.length === 0 ? (
          <p className="text-body-sm text-ink-soft">{empty}</p>
        ) : (
          <>
            <DonutChart
              money={money}
              centerLabel={centerLabel}
              slices={totals.map((t) => {
                const c = categories.get(t.categoryId);
                return {
                  key: t.categoryId,
                  label: c?.name ?? "Category",
                  color: c?.color ?? "cat-slate",
                  icon: c?.icon ?? "circle-help",
                  amount: t.amountMinor,
                };
              })}
            />
            <ul className="flex flex-col">
              {totals.map((t) => {
                const c = categories.get(t.categoryId);
                const share = total > 0n ? Number((t.amountMinor * 1000n) / total) / 10 : 0;
                const row: ReactNode = (
                  <>
                    <CategoryIcon
                      icon={c?.icon ?? "circle-help"}
                      color={c?.color ?? "cat-slate"}
                      size="sm"
                    />
                    <span className="min-w-0 flex-1 truncate text-label text-ink">
                      {c?.name ?? "Category"}
                    </span>
                    <span className="hud-sm hidden text-ink-soft sm:inline">
                      {t.transactionCount.toString()} txn · {share.toFixed(1)}%
                    </span>
                    <Amount
                      minor={negative ? -t.amountMinor : t.amountMinor}
                      currency={currency}
                      minorUnits={money.minorUnits}
                      code={false}
                    />
                  </>
                );
                return (
                  <li key={t.categoryId} className="border-b border-seam/60 last:border-b-0">
                    {href ? (
                      <Link
                        href={href(t.categoryId)}
                        className="flex items-center gap-3 py-2.5 hover:bg-shell-recess/60"
                      >
                        {row}
                      </Link>
                    ) : (
                      <div className="flex items-center gap-3 py-2.5">{row}</div>
                    )}
                  </li>
                );
              })}
            </ul>
          </>
        )}
      </div>
    </Panel>
  );
}
