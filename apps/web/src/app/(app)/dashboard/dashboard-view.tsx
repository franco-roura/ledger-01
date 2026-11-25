"use client";

import { useQuery } from "@connectrpc/connect-query";
import { SectionHeader } from "@franco_roura/module-01";
import { Amount } from "@franco_roura/module-01/finance";
import { Bucket, DashboardService } from "@ledger-01/api";
import { Scale } from "lucide-react";
import Link from "next/link";
import { QuickAdd } from "@/components/quick-add";
import { WalletTypeIcon } from "@/components/wallet-type";
import { useBaseCurrency, useCurrencies, useWallets } from "@/lib/api/hooks";
import { today } from "@/lib/period";
import { takesValuations } from "@/lib/wallet-types";
import { DashboardBody } from "./dashboard-body";
import { ReportFilters, useReportState } from "./report-filters";

export function DashboardView() {
  const base = useBaseCurrency();
  const { state, set, filter, bucket, currency, query } = useReportState(base);
  // Only an explicit currency is sent; the server reports in the base currency by default.
  const { data, isPending } = useQuery(DashboardService.method.getDashboard, {
    filter,
    bucket,
    today: today(),
    currency: currency === base ? "" : currency,
  });
  const shown = data?.currency ?? currency;
  const { minorUnits } = useCurrencies();
  const { wallets } = useWallets();
  // Net worth is always as of today over every visible wallet; the report filters don't apply.
  const day = today();
  const { data: worth } = useQuery(DashboardService.method.getNetWorth, {
    period: { from: day, to: day },
    bucket: Bucket.MONTH,
    today: day,
    currency: currency === base ? "" : currency,
  });

  return (
    <div className="flex flex-col gap-8">
      <SectionHeader code="S-00" label="Overview" title="Dashboard" aside={<QuickAdd />} />

      <ul className="flex gap-3 overflow-x-auto pb-1">
        {worth && (
          <li className="shrink-0">
            <Link
              href="/net-worth"
              className="flex items-center gap-3 rounded-full bg-shell py-2 pr-4 pl-3 shadow-mold hover:bg-shell-recess"
            >
              <Scale className="size-4 text-ink-soft" strokeWidth={1.25} />
              <span className="text-label text-ink">Net worth</span>
              <Amount
                minor={worth.netWorthMinor}
                currency={worth.currency}
                minorUnits={minorUnits(worth.currency)}
                size="sm"
              />
            </Link>
          </li>
        )}
        {wallets
          .filter((w) => !w.hidden && !w.archived)
          .map((w) => (
            <li key={w.id} className="shrink-0">
              <Link
                href={`/wallets/${w.id}`}
                className="flex items-center gap-3 rounded-full bg-shell py-2 pr-4 pl-3 shadow-mold hover:bg-shell-recess"
              >
                <WalletTypeIcon type={w.type} className="size-4 text-ink-soft" />
                <span className="text-label text-ink">{w.name}</span>
                <Amount
                  minor={takesValuations(w.type) ? w.valueMinor : w.balanceMinor}
                  currency={w.currency}
                  minorUnits={minorUnits(w.currency)}
                  size="sm"
                />
              </Link>
            </li>
          ))}
      </ul>

      <ReportFilters state={state} set={set} currency={shown} />

      <DashboardBody
        data={data}
        isPending={isPending}
        bucket={state.bucket}
        currency={shown}
        drillQuery={query}
      />
    </div>
  );
}
