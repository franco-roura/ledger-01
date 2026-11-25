"use client";

import { useMutation, useQuery } from "@connectrpc/connect-query";
import {
  Button,
  DatePicker,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Field,
  fault,
  Panel,
} from "@franco_roura/module-01";
import { Amount, MoneyInput, parseAmount } from "@franco_roura/module-01/finance";
import { type Wallet, WalletService } from "@ledger-01/api";
import { Plus, Trash2 } from "lucide-react";
import { useId, useState } from "react";
import { toast } from "sonner";
import { useCurrencies } from "@/lib/api/hooks";
import { formatDay, today } from "@/lib/period";

/**
 * What an investment or property wallet is worth over time. A valuation sets the wallet's value
 * on its date; movements after it add to that value. Valuations are never income or spending.
 */
export function ValuationsPanel({ wallet }: { wallet: Wallet }) {
  const { minorUnits } = useCurrencies();
  const units = minorUnits(wallet.currency);
  const { data } = useQuery(WalletService.method.listValuations, { walletId: wallet.id });
  const remove = useMutation(WalletService.method.deleteValuation);
  const [open, setOpen] = useState(false);
  const valuations = data?.valuations ?? [];

  return (
    <Panel serial="CFG-02 · VALUE" className="max-w-2xl">
      <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-1">
          <span className="text-body-sm text-ink-soft">Worth today</span>
          <Amount
            minor={wallet.valueMinor}
            currency={wallet.currency}
            minorUnits={units}
            size="lg"
          />
          <span className="text-body-sm text-ink-soft">
            {wallet.valuedOn
              ? `Valued on ${formatDay(wallet.valuedOn)}, plus what moved in or out since.`
              : "No valuation yet, so this is the wallet's balance."}
          </span>
        </div>
        <Button className="self-start" onClick={() => setOpen(true)}>
          <Plus strokeWidth={1.25} /> Update value
        </Button>
        {valuations.length > 0 && (
          <ul className="flex flex-col divide-y divide-seam">
            {valuations.map((v) => (
              <li key={v.id} className="flex items-center gap-3 py-2">
                <span className="flex-1 text-body-sm">{formatDay(v.valuedOn)}</span>
                <Amount
                  minor={v.valueMinor}
                  currency={wallet.currency}
                  minorUnits={units}
                  size="sm"
                />
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={`Delete the valuation of ${formatDay(v.valuedOn)}`}
                  onClick={() => remove.mutate({ id: v.id })}
                >
                  <Trash2 strokeWidth={1.25} />
                </Button>
              </li>
            ))}
          </ul>
        )}
      </div>
      <ValuationDialog wallet={wallet} open={open} onOpenChange={setOpen} />
    </Panel>
  );
}

function ValuationDialog({
  wallet,
  open,
  onOpenChange,
}: {
  wallet: Wallet;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const id = useId();
  const { minorUnits } = useCurrencies();
  const units = minorUnits(wallet.currency);
  const [value, setValue] = useState("");
  const [on, setOn] = useState(today());
  const [error, setError] = useState<string>();
  const save = useMutation(WalletService.method.saveValuation);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    const amount = value.trim() === "" ? null : parseAmount(value, units);
    if (amount === null) {
      setError(value.trim() === "" ? "Enter what it's worth" : `Use up to ${units} decimals`);
      fault(`${id}-value`);
      return;
    }
    await save.mutateAsync({ walletId: wallet.id, valuedOn: on, valueMinor: amount });
    toast.success("Value updated");
    setValue("");
    onOpenChange(false);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={submit} noValidate className="flex flex-col gap-6">
          <DialogHeader>
            <DialogTitle>Update value</DialogTitle>
            <DialogDescription>
              What {wallet.name} is worth on a date. It changes net worth, never income or spending.
            </DialogDescription>
          </DialogHeader>
          <Field label="Value" htmlFor={`${id}-value`} error={error}>
            <MoneyInput
              id={`${id}-value`}
              currency={wallet.currency}
              value={value}
              onChange={(e) => {
                setValue(e.target.value);
                setError(undefined);
              }}
            />
          </Field>
          <Field label="Date" htmlFor={`${id}-date`}>
            <DatePicker id={`${id}-date`} value={on} onChange={(v) => setOn(v || today())} />
          </Field>
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={save.isPending}>
              Save value
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
