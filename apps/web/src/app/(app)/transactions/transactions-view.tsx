"use client";

import { useInfiniteQuery, useQuery } from "@connectrpc/connect-query";
import { TransactionKind, TransactionService, WalletService } from "@ledger-01/api";
import { Amount } from "@ledger-01/ui/components/amount";
import { Button } from "@ledger-01/ui/components/button";
import { SectionHeader } from "@ledger-01/ui/components/hud";
import { Panel } from "@ledger-01/ui/components/panel";
import { Skeleton } from "@ledger-01/ui/components/skeleton";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useCategories, useCurrencies, useLabels } from "@/lib/api/hooks";
import { formatDay, today } from "@/lib/period";
import {
  parseTransactionSearch,
  transactionSearchInput,
  transactionSearchQuery,
} from "@/lib/transaction-search";
import { SearchFilters } from "./search-filters";

export function TransactionsView() {
  const params = useSearchParams();
  const router = useRouter();
  const state = parseTransactionSearch(new URLSearchParams(params.toString()));
  const localToday = today();
  const walletsQ = useQuery(WalletService.method.listWallets, {
    includeHidden: true,
    includeArchived: true,
    today: localToday,
  });
  const { categories, byId: categoriesById } = useCategories();
  const { labels, byId: labelsById } = useLabels();
  const { minorUnits } = useCurrencies();
  const wallets = walletsQ.data?.wallets ?? [];
  const walletsById = new Map(wallets.map((wallet) => [wallet.id, wallet]));
  const results = useInfiniteQuery(
    TransactionService.method.searchTransactions,
    { ...transactionSearchInput(state, localToday), pageSize: 50, pageToken: "" },
    { pageParamKey: "pageToken", getNextPageParam: (last) => last.nextPageToken || undefined },
  );
  const rows = results.data?.pages.flatMap((page) => page.transactions) ?? [];

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-2">
        <SectionHeader code="S-04" title="Transactions" />
        <p className="text-body-sm text-ink-soft">
          Find expenses, income and transfers across your household. Upcoming transactions stay in
          the Future view.
        </p>
      </div>
      <Panel serial="SEARCH · FILTERS">
        <SearchFilters
          key={params.toString()}
          value={state}
          wallets={wallets}
          categories={categories}
          labels={labels}
          onApply={(next) =>
            router.push(`/transactions?${transactionSearchQuery(next)}`, { scroll: false })
          }
          onClear={() => router.push("/transactions", { scroll: false })}
        />
      </Panel>
      <section aria-label="Search results" className="flex flex-col gap-3">
        <p aria-live="polite" className="hud-sm text-ink-soft">
          {results.isError
            ? "Search unavailable"
            : results.isPending
              ? "Searching…"
              : `${rows.length} transactions ${results.hasNextPage ? "shown" : "found"}`}
        </p>
        <Panel bodyClassName="p-0 sm:p-0">
          {results.isError ? (
            <div role="alert" className="flex flex-col items-start gap-3 p-6">
              <p className="text-body-sm">Could not load transactions. Try again.</p>
              <Button variant="secondary" onClick={() => void results.refetch()}>
                Retry search
              </Button>
            </div>
          ) : results.isPending ? (
            <div className="flex flex-col gap-4 p-6">
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
            </div>
          ) : rows.length === 0 ? (
            <div className="flex flex-col gap-2 p-6">
              <p className="text-label text-ink">No transactions match your search.</p>
              <p className="text-body-sm text-ink-soft">
                Try another note, a wider date range or fewer filters.
              </p>
            </div>
          ) : (
            <ul aria-label="Transactions">
              {rows.map((row) => {
                const outgoing =
                  row.kind === TransactionKind.EXPENSE || row.kind === TransactionKind.TRANSFER_OUT;
                const category = row.counterpart
                  ? `${outgoing ? "To" : "From"} ${row.counterpart.walletName}`
                  : (categoriesById.get(row.categoryId)?.name ?? "Transaction");
                return (
                  <li
                    key={row.id}
                    className="flex items-center gap-4 border-b border-seam/60 px-4 py-4 last:border-b-0 sm:px-6"
                  >
                    <div className="flex min-w-0 flex-1 flex-col gap-1 sm:grid sm:grid-cols-[10rem_10rem_1fr] sm:items-center sm:gap-4">
                      <span className="hud-sm text-ink-soft">{formatDay(row.occurredOn)}</span>
                      <Link
                        href={`/wallets/${row.walletId}?from=${state.period.from}&to=${state.period.to}`}
                        className="truncate text-label text-signal-ink hover:underline"
                      >
                        {walletsById.get(row.walletId)?.name ?? "Open wallet"}
                      </Link>
                      <div className="min-w-0">
                        <p className="truncate text-label text-ink">{row.note || category}</p>
                        <p className="truncate text-body-sm text-ink-soft">
                          {category}
                          {row.labelIds.length > 0 &&
                            ` · ${row.labelIds
                              .map((id) => labelsById.get(id)?.name)
                              .filter(Boolean)
                              .join(", ")}`}
                        </p>
                      </div>
                    </div>
                    <Amount
                      minor={outgoing ? -row.amountMinor : row.amountMinor}
                      currency={row.currency}
                      minorUnits={minorUnits(row.currency)}
                    />
                  </li>
                );
              })}
            </ul>
          )}
        </Panel>
        {results.hasNextPage && (
          <Button
            variant="secondary"
            className="self-center"
            disabled={results.isFetchingNextPage}
            onClick={() => void results.fetchNextPage()}
          >
            {results.isFetchingNextPage ? "Loading…" : "Load more transactions"}
          </Button>
        )}
      </section>
    </div>
  );
}
