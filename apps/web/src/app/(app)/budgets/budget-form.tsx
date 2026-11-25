"use client";

import { Code, ConnectError } from "@connectrpc/connect";
import { useMutation, useQuery } from "@connectrpc/connect-query";
import {
  Button,
  Checkbox,
  DatePicker,
  Field,
  fault,
  Input,
  Label,
  MultiCombobox,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@franco_roura/module-01";
import {
  CategoryIcon,
  formatAmountInput,
  MoneyInput,
  parseAmount,
} from "@franco_roura/module-01/finance";
import {
  type Budget,
  BudgetRepeat,
  BudgetService,
  type Category,
  CategoryKind,
  UserService,
  type Wallet,
} from "@ledger-01/api";
import { type ReactNode, useId, useState } from "react";
import { toast } from "sonner";
import { CurrencySelect } from "@/components/currency-select";
import { repeatOptions } from "@/lib/budgets";
import { currentMonth, today } from "@/lib/period";

type FieldKey =
  | "name"
  | "amount"
  | "currency"
  | "repeat"
  | "start"
  | "end"
  | "wallets"
  | "cats"
  | "leftover"
  | "overspend";

const serverFields: Record<string, FieldKey> = {
  name: "name",
  amount_minor: "amount",
  currency: "currency",
  repeat: "repeat",
  start_date: "start",
  end_date: "end",
  wallet_ids: "wallets",
  category_ids: "cats",
  carry_leftover: "leftover",
  carry_overspend: "overspend",
};

/** Create or edit a budget. Fields are in fill order, so the form flows top to bottom. */
export function BudgetForm({
  editing,
  wallets,
  categories,
  minorUnits,
  onDone,
}: {
  editing: Budget | null;
  wallets: Wallet[];
  categories: Category[];
  minorUnits: (code: string) => number;
  onDone: (budget: Budget) => void;
}) {
  const id = useId();
  const { data: me } = useQuery(UserService.method.getMe, {});
  const [name, setName] = useState(editing?.name ?? "");
  const [currency, setCurrency] = useState(
    editing?.currency ?? me?.household?.baseCurrency ?? "USD",
  );
  const [amount, setAmount] = useState(
    editing ? formatAmountInput(editing.amountMinor, minorUnits(editing.currency)) : "",
  );
  const [repeat, setRepeat] = useState<BudgetRepeat>(editing?.repeat ?? BudgetRepeat.MONTHLY);
  const [start, setStart] = useState(editing?.startDate ?? currentMonth().from);
  const [end, setEnd] = useState(editing?.endDate || currentMonth().to);
  const [walletScope, setWalletScope] = useState(
    !editing || editing.allWallets ? [ALL] : editing.walletIds,
  );
  const [categoryScope, setCategoryScope] = useState(
    !editing || editing.allCategories ? [ALL] : editing.categoryIds,
  );
  const [carryLeftover, setCarryLeftover] = useState(editing?.carryLeftover ?? false);
  const [carryOverspend, setCarryOverspend] = useState(editing?.carryOverspend ?? false);
  const missingWallets = walletScope.length === 0;
  const missingCategories = categoryScope.length === 0;
  const create = useMutation(BudgetService.method.createBudget);
  const update = useMutation(BudgetService.method.updateBudget);

  const units = minorUnits(currency);
  const parsedAmount = amount.trim() === "" ? null : parseAmount(amount, units);
  const once = repeat === BudgetRepeat.ONCE;

  const [errors, setErrors] = useState<Partial<Record<FieldKey, string>>>({});
  const clear = (key: FieldKey) => setErrors((e) => (e[key] ? { ...e, [key]: undefined } : e));
  const fieldId = (key: FieldKey) => `${id}-${key}`;
  function reject(key: FieldKey, message: string, opts: { toast?: boolean } = {}) {
    setErrors({ [key]: message });
    if (opts.toast !== false) toast.error(message);
    fault(fieldId(key));
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErrors({});
    if (!name.trim()) return reject("name", "Name the budget");
    if (parsedAmount === null || parsedAmount <= 0n) return reject("amount", "Enter an amount");
    if (!start) return reject("start", "Pick a start date");
    if (once && !end) return reject("end", "Pick an end date");
    if (once && end < start) return reject("end", "End the budget on or after its start");
    if (missingWallets) return reject("wallets", "Choose a wallet or All wallets");
    if (missingCategories) return reject("cats", "Choose a category or All expense categories");
    const input = {
      name: name.trim(),
      amountMinor: parsedAmount,
      currency,
      repeat,
      startDate: start,
      endDate: once ? end : "",
      allWallets: walletScope.includes(ALL),
      allCategories: categoryScope.includes(ALL),
      walletIds: walletScope.filter((id) => id !== ALL),
      categoryIds: categoryScope.filter((id) => id !== ALL),
      carryLeftover: !once && carryLeftover,
      carryOverspend: !once && carryOverspend,
    };
    try {
      // Turning rollover on starts carrying from the period containing today.
      const res = editing
        ? await update.mutateAsync({ id: editing.id, budget: input, today: today() })
        : await create.mutateAsync({ budget: input, today: today() });
      toast.success(editing ? "Saved" : "Budget added");
      if (res.budget) onDone(res.budget);
    } catch (err) {
      // The API names the offending field ("end_date: must not be before…"); point at it.
      if (!(err instanceof ConnectError) || err.code !== Code.InvalidArgument) return;
      const m = /^([a-z_.]+): (.+)$/s.exec(err.rawMessage);
      const key = m?.[1] ? serverFields[m[1]] : undefined;
      if (key && m?.[2]) reject(key, m[2], { toast: false });
    }
  }

  const expenseCategories = categories.filter(
    (c) => c.kind === CategoryKind.EXPENSE && (!c.archived || categoryScope.includes(c.id)),
  );
  const activeWallets = wallets.filter((w) => !w.archived || walletScope.includes(w.id));

  return (
    <form onSubmit={submit} className="flex flex-col gap-5" noValidate>
      <Field label="Name" htmlFor={fieldId("name")} error={errors.name}>
        <Input
          id={fieldId("name")}
          value={name}
          onChange={(e) => {
            setName(e.target.value);
            clear("name");
          }}
          maxLength={60}
          placeholder="Groceries, Eating out, Trip…"
        />
      </Field>

      <div className="grid gap-4 sm:grid-cols-[1fr_160px]">
        <Field
          label="Amount"
          htmlFor={fieldId("amount")}
          error={
            errors.amount ??
            (amount !== "" && parsedAmount === null ? `Use up to ${units} decimals` : undefined)
          }
        >
          <MoneyInput
            id={fieldId("amount")}
            currency={currency}
            value={amount}
            onChange={(e) => {
              setAmount(e.target.value);
              clear("amount");
            }}
            aria-invalid={amount !== "" && parsedAmount === null}
          />
        </Field>
        <Field label="Currency" htmlFor={fieldId("currency")} error={errors.currency}>
          <CurrencySelect
            id={fieldId("currency")}
            value={currency}
            onChange={(c) => {
              setCurrency(c);
              clear("currency");
            }}
          />
        </Field>
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Repeats" htmlFor={fieldId("repeat")} error={errors.repeat}>
          <Select
            value={String(repeat)}
            onValueChange={(v) => {
              setRepeat(Number(v) as BudgetRepeat);
              clear("repeat");
            }}
          >
            <SelectTrigger id={fieldId("repeat")} className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {repeatOptions.map((o) => (
                <SelectItem key={o.value} value={String(o.value)}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field
          label={once ? "Starts" : "First period starts"}
          htmlFor={fieldId("start")}
          error={errors.start}
        >
          <DatePicker
            id={fieldId("start")}
            value={start}
            onChange={(v) => {
              setStart(v);
              clear("start");
            }}
          />
        </Field>
      </div>

      {once && (
        <Field label="Ends" htmlFor={fieldId("end")} error={errors.end}>
          <DatePicker
            id={fieldId("end")}
            value={end}
            onChange={(v) => {
              setEnd(v);
              clear("end");
            }}
          />
        </Field>
      )}

      <Field
        label="Wallets"
        htmlFor={fieldId("wallets")}
        error={
          errors.wallets ??
          (missingWallets
            ? "This budget counts no spending until you choose a wallet or All wallets."
            : undefined)
        }
      >
        <ScopePicker
          id={fieldId("wallets")}
          value={walletScope}
          onChange={(v) => {
            setWalletScope(v);
            clear("wallets");
          }}
          options={activeWallets.map((w) => ({
            id: w.id,
            search: `${w.name} ${w.currency}`,
            label: (
              <>
                <span className="truncate">{w.name}</span>
                <span className="hud-sm text-ink-soft">{w.currency}</span>
              </>
            ),
          }))}
          allLabel="All wallets"
          noun="wallets"
        />
      </Field>

      <Field
        label="Categories"
        htmlFor={fieldId("cats")}
        error={
          errors.cats ??
          (missingCategories
            ? "This budget counts no spending until you choose a category or All expense categories."
            : undefined)
        }
      >
        <ScopePicker
          id={fieldId("cats")}
          value={categoryScope}
          onChange={(v) => {
            setCategoryScope(v);
            clear("cats");
          }}
          options={expenseCategories.map((c) => ({
            id: c.id,
            search: c.name,
            label: (
              <>
                <CategoryIcon icon={c.icon} color={c.color} size="sm" />
                <span className="truncate">{c.name}</span>
              </>
            ),
          }))}
          allLabel="All expense categories"
          noun="categories"
        />
      </Field>

      {!once && (
        <fieldset className="flex flex-col gap-3">
          <legend className="mb-2 text-body-sm">Rollover</legend>
          <CarryOption
            id={fieldId("leftover")}
            label="Carry leftover"
            hint="Unspent money is added to the next period's limit."
            checked={carryLeftover}
            error={errors.leftover}
            onChange={(v) => {
              setCarryLeftover(v);
              clear("leftover");
            }}
          />
          <CarryOption
            id={fieldId("overspend")}
            label="Carry overspend"
            hint="Going over lowers the next period's limit."
            checked={carryOverspend}
            error={errors.overspend}
            onChange={(v) => {
              setCarryOverspend(v);
              clear("overspend");
            }}
          />
        </fieldset>
      )}

      <div className="flex justify-end gap-2 pt-2">
        <Button type="submit" disabled={create.isPending || update.isPending}>
          {editing ? "Save budget" : "Add budget"}
        </Button>
      </div>
    </form>
  );
}

const ALL = "all";

function CarryOption({
  id,
  label,
  hint,
  checked,
  error,
  onChange,
}: {
  id: string;
  label: string;
  hint: string;
  checked: boolean;
  error?: string;
  onChange: (checked: boolean) => void;
}) {
  return (
    <div className="flex items-start gap-2">
      <Checkbox
        id={id}
        checked={checked}
        onCheckedChange={(v) => onChange(v === true)}
        aria-describedby={`${id}-hint`}
        aria-invalid={error ? true : undefined}
        className="mt-0.5"
      />
      <div className="flex flex-col gap-0.5">
        <Label htmlFor={id} className="text-body-sm">
          {label}
        </Label>
        <span id={`${id}-hint`} className="text-body-sm text-ink-soft">
          {error ?? hint}
        </span>
      </div>
    </div>
  );
}

function ScopePicker({
  id,
  value,
  onChange,
  options,
  allLabel,
  noun,
}: {
  id: string;
  value: string[];
  onChange: (value: string[]) => void;
  options: { id: string; search: string; label: ReactNode }[];
  allLabel: string;
  noun: string;
}) {
  return (
    <MultiCombobox
      id={id}
      value={value}
      onChange={(next) => {
        if (value.includes(ALL)) onChange(next.filter((key) => key !== ALL));
        else if (next.includes(ALL)) onChange([ALL]);
        else onChange(next);
      }}
      options={[{ id: ALL, search: allLabel, label: allLabel }, ...options]}
      getKey={(option) => option.id}
      getSearchText={(option) => option.search}
      renderOption={(option) => option.label}
      allLabel={`No ${noun}`}
      noun={noun}
      searchPlaceholder={`Search ${noun}…`}
      invalid={value.length === 0}
    />
  );
}
