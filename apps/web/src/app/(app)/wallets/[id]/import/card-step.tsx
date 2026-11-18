"use client";

import {
  Button,
  Field,
  fault,
  Panel,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@franco_roura/module-01";
import type { ImportMapping, ImportStatement } from "@ledger-01/api";
import { useId, useState } from "react";
import { useWallets } from "@/lib/api/hooks";
import { formatDay } from "@/lib/period";

/**
 * A card statement has no columns to match: the one choice is the account the bill is debited
 * from, whose debit becomes a transfer to this card.
 */
export function CardStep({
  statement,
  mapping,
  onChange,
  walletId,
  onBack,
  onNext,
}: {
  statement: ImportStatement;
  mapping: ImportMapping;
  onChange: (m: ImportMapping) => void;
  walletId: string;
  onBack: () => void;
  onNext: () => void;
}) {
  const id = useId();
  const { wallets } = useWallets();
  const [error, setError] = useState<string>();
  const payers = wallets.filter((w) => w.id !== walletId);

  return (
    <Panel className="max-w-2xl">
      <form
        className="flex flex-col gap-5"
        onSubmit={(e) => {
          e.preventDefault();
          if (!mapping.paidFromWalletId) {
            setError("Choose the account that pays this card");
            fault(`${id}-payer`);
            return;
          }
          onNext();
        }}
      >
        <div className="flex flex-col gap-2 text-body-md text-ink-soft">
          <p>
            <span className="text-ink">{statement.bankName}</span> statement of{" "}
            {formatDay(statement.issuedOn)}, due {formatDay(statement.dueOn)}.
          </p>
          <p>
            Its purchases and fees become expenses in this wallet. Its payment becomes a transfer
            from the account below: the card's debit already imported there turns into that
            transfer, so it isn't counted as spending twice.
          </p>
          {statement.otherCurrencyLines > 0 && (
            <p>
              {statement.otherCurrencyLines} {statement.otherCurrencyLines === 1 ? "line" : "lines"}{" "}
              in another currency are left out. Import the statement into a card wallet in that
              currency too.
            </p>
          )}
        </div>
        <Field label="Paid from" htmlFor={`${id}-payer`} error={error}>
          <Select
            value={mapping.paidFromWalletId || undefined}
            onValueChange={(v) => {
              setError(undefined);
              onChange({ ...mapping, paidFromWalletId: v });
            }}
          >
            <SelectTrigger id={`${id}-payer`} className="w-full min-w-0">
              <SelectValue placeholder="Choose an account" />
            </SelectTrigger>
            <SelectContent>
              {payers.map((w) => (
                <SelectItem key={w.id} value={w.id}>
                  {w.name} · {w.currency}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <div className="flex flex-wrap justify-between gap-2 pt-2">
          <Button type="button" variant="ghost" onClick={onBack}>
            Choose another file
          </Button>
          <Button type="submit">Review rows</Button>
        </div>
      </form>
    </Panel>
  );
}
