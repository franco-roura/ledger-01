import { TransactionKind } from "@ledger-01/api";
import { currentMonth, type Period } from "./period";

export const searchKinds = {
  expense: { value: TransactionKind.EXPENSE, label: "Expenses" },
  income: { value: TransactionKind.INCOME, label: "Income" },
  transfer_out: { value: TransactionKind.TRANSFER_OUT, label: "Transfers out" },
  transfer_in: { value: TransactionKind.TRANSFER_IN, label: "Transfers in" },
} as const;
export type SearchKind = keyof typeof searchKinds;
export type TransactionSearch = {
  period: Period;
  note: string;
  walletIds: string[];
  categoryIds: string[];
  labelIds: string[];
  kinds: SearchKind[];
};

const uuid = /^[\da-f]{8}-[\da-f]{4}-[\da-f]{4}-[\da-f]{4}-[\da-f]{12}$/i;
const ids = (value: string | null) => [
  ...new Set((value ?? "").split(",").filter((id) => uuid.test(id))),
];
const isDate = (value: string) => {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const parsed = new Date(`${value}T00:00:00Z`);
  return !Number.isNaN(parsed.getTime()) && parsed.toISOString().slice(0, 10) === value;
};

export function validSearchPeriod(period: Period): boolean {
  return (
    isDate(period.from) &&
    isDate(period.to) &&
    period.from <= period.to &&
    new Date(period.to).getTime() - new Date(period.from).getTime() <= 20 * 366 * 86400000
  );
}

export function parseTransactionSearch(params: URLSearchParams): TransactionSearch {
  const period = { from: params.get("from") ?? "", to: params.get("to") ?? "" };
  return {
    period: validSearchPeriod(period) ? period : currentMonth(),
    note: params.get("note") ?? "",
    walletIds: ids(params.get("wallets")),
    categoryIds: ids(params.get("categories")),
    labelIds: ids(params.get("labels")),
    kinds: [
      ...new Set(
        (params.get("kinds") ?? "")
          .split(",")
          .filter((kind): kind is SearchKind => Object.hasOwn(searchKinds, kind)),
      ),
    ],
  };
}

export function transactionSearchQuery(state: TransactionSearch): URLSearchParams {
  const query = new URLSearchParams({ from: state.period.from, to: state.period.to });
  if (state.note.trim()) query.set("note", state.note.trim());
  for (const [key, values] of [
    ["wallets", state.walletIds],
    ["categories", state.categoryIds],
    ["labels", state.labelIds],
    ["kinds", state.kinds],
  ] as const) {
    if (values.length) query.set(key, [...new Set(values)].join(","));
  }
  return query;
}

export function transactionSearchInput(state: TransactionSearch, today: string) {
  return {
    period: state.period,
    noteQuery: state.note.trim(),
    walletIds: state.walletIds,
    categoryIds: state.categoryIds,
    labelIds: state.labelIds,
    kinds: state.kinds.map((kind) => searchKinds[kind].value),
    today,
  };
}
