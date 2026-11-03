"use client";

import type { Category, Label, Wallet } from "@ledger-01/api";
import { Button } from "@ledger-01/ui/components/button";
import { Field } from "@ledger-01/ui/components/field";
import { Input } from "@ledger-01/ui/components/input";
import { useState } from "react";
import { MultiPicker } from "@/components/multi-picker";
import {
  type SearchKind,
  searchKinds,
  type TransactionSearch,
  validSearchPeriod,
} from "@/lib/transaction-search";

export function SearchFilters({
  value,
  wallets,
  categories,
  labels,
  onApply,
  onClear,
}: {
  value: TransactionSearch;
  wallets: Wallet[];
  categories: Category[];
  labels: Label[];
  onApply: (state: TransactionSearch) => void;
  onClear: () => void;
}) {
  const [draft, setDraft] = useState(value);
  const [error, setError] = useState("");
  return (
    <form
      aria-label="Transaction search"
      className="flex flex-col gap-5"
      onSubmit={(event) => {
        event.preventDefault();
        if (!validSearchPeriod(draft.period)) {
          setError("Choose a valid date range of at most 20 years.");
          return;
        }
        setError("");
        onApply(draft);
      }}
    >
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-[2fr_1fr_1fr]">
        <Field label="Search notes" htmlFor="transaction-note">
          <Input
            id="transaction-note"
            placeholder="A merchant, bill or note…"
            maxLength={200}
            value={draft.note}
            onChange={(event) => setDraft({ ...draft, note: event.target.value })}
          />
        </Field>
        <Field label="From" htmlFor="transaction-from">
          <Input
            id="transaction-from"
            type="date"
            required
            value={draft.period.from}
            max={draft.period.to}
            onChange={(event) =>
              setDraft({ ...draft, period: { ...draft.period, from: event.target.value } })
            }
          />
        </Field>
        <Field label="To" htmlFor="transaction-to">
          <Input
            id="transaction-to"
            type="date"
            required
            value={draft.period.to}
            min={draft.period.from}
            onChange={(event) =>
              setDraft({ ...draft, period: { ...draft.period, to: event.target.value } })
            }
          />
        </Field>
      </div>
      <div className="flex flex-wrap items-end gap-4">
        <Field label="Wallets" htmlFor="search-wallets">
          <MultiPicker
            id="search-wallets"
            options={wallets.map((w) => ({
              key: w.id,
              label: `${w.name}${w.archived ? " (archived)" : ""}`,
            }))}
            value={draft.walletIds}
            onChange={(walletIds) => setDraft({ ...draft, walletIds })}
            allLabel="All wallets"
            noun="wallets"
            searchPlaceholder="Search wallets…"
          />
        </Field>
        <Field label="Categories" htmlFor="search-categories">
          <MultiPicker
            id="search-categories"
            options={categories.map((c) => ({ key: c.id, label: c.name }))}
            value={draft.categoryIds}
            onChange={(categoryIds) => setDraft({ ...draft, categoryIds })}
            allLabel="All categories"
            noun="categories"
            searchPlaceholder="Search categories…"
          />
        </Field>
        <Field label="Labels" htmlFor="search-labels">
          <MultiPicker
            id="search-labels"
            options={labels.map((l) => ({ key: l.id, label: l.name }))}
            value={draft.labelIds}
            onChange={(labelIds) => setDraft({ ...draft, labelIds })}
            allLabel="All labels"
            noun="labels"
            searchPlaceholder="Search labels…"
          />
        </Field>
        <Field label="Type" htmlFor="search-kinds">
          <MultiPicker
            id="search-kinds"
            options={Object.entries(searchKinds).map(([key, kind]) => ({ key, label: kind.label }))}
            value={draft.kinds}
            onChange={(kinds) => setDraft({ ...draft, kinds: kinds as SearchKind[] })}
            allLabel="All types"
            noun="types"
          />
        </Field>
        <div className="flex flex-wrap gap-2">
          <Button type="submit">Search transactions</Button>
          <Button type="button" variant="ghost" onClick={onClear}>
            Clear filters
          </Button>
        </div>
      </div>
      {error && (
        <p role="alert" className="text-body-sm text-danger">
          {error}
        </p>
      )}
    </form>
  );
}
