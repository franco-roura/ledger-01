"use client";

import { useQuery } from "@connectrpc/connect-query";
import { HudLabel, Panel, SectionHeader, Skeleton } from "@franco_roura/module-01";
import { BalanceChart } from "@franco_roura/module-01/charts";
import { Amount } from "@franco_roura/module-01/finance";
import { DashboardService, type NetWorthWallet, type Wallet } from "@ledger-01/api";
import Link from "next/link";
import { WalletTypeIcon } from "@/components/wallet-type";
import { useBaseCurrency, useCurrencies, useWallets } from "@/lib/api/hooks";
import { bucketLabels } from "@/lib/buckets";
import { formatDay, today } from "@/lib/period";
import { byClass, walletTypeLabel } from "@/lib/wallet-types";
import { CategoryBreakdown } from "../dashboard/category-breakdown";
import { ReportFilters, useReportState } from "../dashboard/report-filters";

/**
 * Assets minus what credit cards and loans owe, as of today, with a history chart. Liabilities
 * also show their upcoming debt (charges still to come, e.g. the rest of an installment plan),
 * which is owed later and never counts as spending.
 */
export function NetWorthView() {
  const base = useBaseCurrency();
  const { state, set, bucket, currency } = useReportState(base);
  const { data, isPending } = useQuery(DashboardService.method.getNetWorth, {
    period: state.period,
    bucket,
    today: today(),
    currency: currency === base ? "" : currency,
  });
  const shown = data?.currency ?? currency;
  const { minorUnits } = useCurrencies();
  const { wallets } = useWallets();
  const money = { currency: shown, minorUnits: minorUnits(shown) };
  const byId = new Map(wallets.map((w) => [w.id, w]));
  const { assets, liabilities } = byClass(data?.wallets ?? []);

  const points = (data?.series ?? []).map((p) => ({
    ...bucketLabels(p.date, state.bucket),
    balance: p.netWorthMinor,
  }));
  const tiles = [
    { label: "Net worth", value: data?.netWorthMinor },
    { label: "Assets", value: data?.assetsMinor },
    { label: "Owed now", value: data ? -data.liabilitiesMinor : undefined },
    {
      label: "Owed incl. upcoming installments",
      value: data ? -(data.liabilitiesMinor + data.upcomingDebtMinor) : undefined,
    },
  ];

  return (
    <div className="flex flex-col gap-8">
      <SectionHeader code="S-04" label="Overview" title="Net worth" />

      <ReportFilters
        state={state}
        set={set}
        currency={shown}
        showWallets={false}
        showCategories={false}
        showFlowFilters={false}
      />

      {data && data.unconvertedCurrencies.length > 0 && (
        <p className="flex items-center gap-2 rounded-md bg-shell-recess px-4 py-2 text-body-sm text-ink">
          <span className="hud-sm text-ink-soft">Note</span>
          Not counted: {data.unconvertedCurrencies.join(", ")}. Totals use your own transfer rates,
          and there's no transfer between{" "}
          {data.unconvertedCurrencies.length > 1 ? "these currencies" : "this currency"} and {shown}{" "}
          yet.
        </p>
      )}

      <dl className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        {tiles.map((t) => (
          <Panel key={t.label} notch="sm" bodyClassName="p-4 sm:p-5">
            <dt className="hud-sm text-ink-soft">{t.label}</dt>
            <dd className="mt-2">
              {t.value === undefined ? (
                <Skeleton className="h-7 w-32" />
              ) : (
                <Amount
                  minor={t.value}
                  currency={shown}
                  minorUnits={money.minorUnits}
                  size="lg"
                  className="max-sm:text-amount-md"
                />
              )}
            </dd>
          </Panel>
        ))}
      </dl>

      <Panel serial="NW-01 · HISTORY" cross bodyClassName="p-5 pt-11 sm:p-6 sm:pt-11">
        <div className="flex flex-col gap-4">
          <HudLabel rule={false}>Net worth · {shown}</HudLabel>
          {isPending ? (
            <Skeleton className="h-56" />
          ) : (
            <BalanceChart points={points} money={money} />
          )}
        </div>
      </Panel>

      <div className="grid gap-6 lg:grid-cols-2">
        <WalletList
          serial="NW-02 · ASSETS"
          title="Assets"
          empty="No asset wallets."
          rows={assets}
          wallets={byId}
        />
        <WalletList
          serial="NW-03 · LIABILITIES"
          title="Credit cards and loans"
          empty="No credit card or loan wallets yet. Set a wallet's type in its settings."
          rows={liabilities}
          wallets={byId}
          owed
        />
      </div>

      <CategoryBreakdown
        serial="NW-04 · UPCOMING"
        title="Upcoming installments by category"
        centerLabel="Upcoming"
        empty="Nothing scheduled on credit cards or loans."
        totals={data?.upcomingDebtCategories ?? []}
        currency={shown}
        negative
      />
    </div>
  );
}

/** Wallets in their own currency: an asset's value, or what a liability owes plus what's coming. */
function WalletList({
  serial,
  title,
  empty,
  rows,
  wallets,
  owed = false,
}: {
  serial: string;
  title: string;
  empty: string;
  rows: NetWorthWallet[];
  wallets: Map<string, Wallet>;
  owed?: boolean;
}) {
  const { minorUnits } = useCurrencies();
  return (
    <Panel serial={serial} bodyClassName="p-5 pt-11 sm:p-6 sm:pt-11">
      <div className="flex flex-col gap-4">
        <h2 className="font-display text-display-sm text-ink">{title}</h2>
        {rows.length === 0 ? (
          <p className="text-body-sm text-ink-soft">{empty}</p>
        ) : (
          <ul className="flex flex-col">
            {rows.map((r) => {
              const w = wallets.get(r.walletId);
              const cur = w?.currency ?? "USD";
              return (
                <li key={r.walletId} className="border-b border-seam/60 last:border-b-0">
                  <Link
                    href={`/wallets/${r.walletId}`}
                    className="flex items-start gap-3 py-2.5 hover:bg-shell-recess/60"
                  >
                    <WalletTypeIcon
                      type={r.type}
                      className="mt-0.5 size-4 shrink-0 text-ink-soft"
                    />
                    <span className="flex min-w-0 flex-1 flex-col">
                      <span className="truncate text-label text-ink">{w?.name ?? "Wallet"}</span>
                      <span className="hud-sm text-ink-soft">
                        {walletTypeLabel(r.type)}
                        {w?.valuedOn ? ` · valued ${formatDay(w.valuedOn)}` : ""}
                      </span>
                    </span>
                    <span className="flex flex-col items-end gap-0.5">
                      <Amount
                        minor={owed ? -r.amountMinor : r.amountMinor}
                        currency={cur}
                        minorUnits={minorUnits(cur)}
                      />
                      {owed && r.upcomingDebtMinor !== 0n && (
                        <span className="flex items-center gap-1 text-body-sm text-ink-soft">
                          <Amount
                            minor={-r.upcomingDebtMinor}
                            currency={cur}
                            minorUnits={minorUnits(cur)}
                            size="sm"
                            code={false}
                          />
                          upcoming
                        </span>
                      )}
                    </span>
                  </Link>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </Panel>
  );
}
