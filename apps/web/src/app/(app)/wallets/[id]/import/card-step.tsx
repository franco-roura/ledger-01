"use client";

import { useMutation } from "@connectrpc/connect-query";
import {
  Button,
  Field,
  fault,
  MultiCombobox,
  Panel,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@franco_roura/module-01";
import { formatMoney } from "@franco_roura/module-01/finance";
import {
  type ImportMapping,
  type ImportStatement,
  type Wallet,
  WalletService,
} from "@ledger-01/api";
import { useId, useState } from "react";
import { toast } from "sonner";
import { useCurrencies, useWallets } from "@/lib/api/hooks";
import { formatDay } from "@/lib/period";

/** Select values that aren't wallets. */
const NEW = "new";
const SKIP = "skip";

/**
 * A card statement has no columns to match. A card can spend in several currencies, each kept in
 * a wallet of its own, so this step links each currency of the statement to the card's wallet in
 * it (or makes one), and says which accounts the bill is debited from. Both are remembered on
 * every card wallet, so next month's statement goes straight to the review, whichever wallet
 * it's dropped on.
 */
export function CardStep({
  statement,
  mapping,
  onChange,
  wallet,
  onBack,
  onNext,
}: {
  statement: ImportStatement;
  mapping: ImportMapping;
  onChange: (m: ImportMapping) => void;
  /** The wallet the statement was dropped on. */
  wallet: Wallet;
  onBack: () => void;
  onNext: () => void;
}) {
  const id = useId();
  const { wallets, refetch } = useWallets();
  const { minorUnits } = useCurrencies();
  const create = useMutation(WalletService.method.createWallet);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const links = mapping.cardWalletIds;
  const cardIds = new Set(Object.values(links).filter(Boolean));
  const payers = wallets.filter((w) => !cardIds.has(w.id) && w.id !== wallet.id);
  const fieldId = (currency: string) => `${id}-card-${currency}`;

  function link(currency: string, walletId: string) {
    setErrors((e) => ({ ...e, [currency]: "" }));
    onChange({
      ...mapping,
      cardWalletIds: { ...links, [currency]: walletId },
      // A wallet that becomes the card's no longer pays it.
      payingWalletIds: mapping.payingWalletIds.filter((p) => p !== walletId),
    });
  }

  async function choose(currency: string, value: string) {
    if (value === SKIP) return link(currency, "");
    if (value !== NEW) return link(currency, value);
    try {
      const res = await create.mutateAsync({ name: `${wallet.name} ${currency}`, currency });
      await refetch();
      if (res.wallet) link(currency, res.wallet.id);
    } catch {
      // The mutation toasted the reason.
      fault(fieldId(currency));
    }
  }

  function next() {
    const codes = statement.currencies.map((c) => c.currency);
    const missing = codes.filter((c) => links[c] === undefined);
    if (missing.length > 0) {
      setErrors(
        Object.fromEntries(
          missing.map((c) => [c, `Choose the card's ${c} wallet, or leave these lines out`]),
        ),
      );
      fault(fieldId(missing[0] ?? ""));
      return;
    }
    if (codes.every((c) => !links[c])) {
      toast.error("Choose a wallet for at least one currency");
      fault(fieldId(codes[0] ?? ""));
      return;
    }
    onNext();
  }

  return (
    <Panel className="max-w-2xl">
      <form
        className="flex flex-col gap-5"
        onSubmit={(e) => {
          e.preventDefault();
          next();
        }}
      >
        <div className="flex flex-col gap-2 text-body-md text-ink-soft">
          <p>
            <span className="text-ink">{statement.bankName}</span> statement of{" "}
            {formatDay(statement.issuedOn)}, due {formatDay(statement.dueOn)}.
          </p>
          <p>
            Each currency's purchases and fees become expenses in the card's wallet in that
            currency. The bill's payment becomes a transfer from the account it was debited from, so
            the debit isn't counted as spending twice.
          </p>
        </div>

        {statement.currencies.map((c) => {
          const options = wallets.filter((w) => w.currency === c.currency);
          const value = links[c.currency];
          return (
            <Field
              key={c.currency}
              label={`${c.currency} wallet`}
              htmlFor={fieldId(c.currency)}
              error={errors[c.currency] || undefined}
              help={`${c.lines} ${c.lines === 1 ? "line" : "lines"}, ${formatMoney(
                c.spentMinor,
                c.currency,
                minorUnits(c.currency),
              )} spent`}
            >
              <Select
                value={value === undefined ? undefined : value || SKIP}
                onValueChange={(v) => choose(c.currency, v)}
                disabled={create.isPending}
              >
                <SelectTrigger id={fieldId(c.currency)} className="w-full min-w-0">
                  <SelectValue placeholder="Choose a wallet" />
                </SelectTrigger>
                <SelectContent>
                  {options.map((w) => (
                    <SelectItem key={w.id} value={w.id}>
                      {w.name}
                    </SelectItem>
                  ))}
                  <SelectItem value={NEW}>
                    New wallet “{wallet.name} {c.currency}”
                  </SelectItem>
                  <SelectItem value={SKIP}>Leave these lines out</SelectItem>
                </SelectContent>
              </Select>
            </Field>
          );
        })}

        <Field
          label="Accounts that pay this card"
          htmlFor={`${id}-payers`}
          help="The bank's debit for the bill is looked for in these, in any currency."
        >
          <MultiCombobox
            id={`${id}-payers`}
            value={mapping.payingWalletIds}
            onChange={(v) => onChange({ ...mapping, payingWalletIds: v })}
            options={payers}
            getKey={(w) => w.id}
            getSearchText={(w) => `${w.name} ${w.currency}`}
            renderOption={(w) => (
              <>
                <span className="truncate">{w.name}</span>
                <span className="hud-sm text-ink-soft">{w.currency}</span>
              </>
            )}
            allLabel="All other accounts"
            noun="accounts"
            searchPlaceholder="Search accounts…"
          />
        </Field>

        <div className="flex flex-wrap justify-between gap-2 pt-2">
          <Button type="button" variant="ghost" onClick={onBack}>
            Choose another file
          </Button>
          <Button type="submit" disabled={create.isPending}>
            Review rows
          </Button>
        </div>
      </form>
    </Panel>
  );
}
