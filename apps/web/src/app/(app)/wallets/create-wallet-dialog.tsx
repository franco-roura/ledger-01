"use client";

import { useMutation } from "@connectrpc/connect-query";
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Field,
  fault,
  Input,
} from "@franco_roura/module-01";
import { MoneyInput, parseAmount } from "@franco_roura/module-01/finance";
import { WalletService, WalletType } from "@ledger-01/api";
import { useRouter } from "next/navigation";
import { useId, useState } from "react";
import { CurrencySelect } from "@/components/currency-select";
import { WalletTypeSelect } from "@/components/wallet-type";
import { useCurrencies } from "@/lib/api/hooks";
import { isLiability } from "@/lib/wallet-types";

type FieldKey = "name" | "initial";

export function CreateWalletDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const router = useRouter();
  const id = useId();
  const { minorUnits } = useCurrencies();
  const [name, setName] = useState("");
  const [type, setType] = useState<WalletType>(WalletType.BANK);
  const [currency, setCurrency] = useState("USD");
  const [initial, setInitial] = useState("");
  const [negative, setNegative] = useState(false);
  const [errors, setErrors] = useState<Partial<Record<FieldKey, string>>>({});
  const create = useMutation(WalletService.method.createWallet);

  const owes = isLiability(type);
  const units = minorUnits(currency);
  const amount = initial.trim() === "" ? 0n : parseAmount(initial, units);
  const fieldId = (key: FieldKey) => `${id}-${key}`;
  function reject(key: FieldKey, message: string) {
    setErrors({ [key]: message });
    fault(fieldId(key));
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErrors({});
    if (!name.trim()) return reject("name", "Name the wallet");
    if (amount === null) return reject("initial", `Use up to ${units} decimals`);
    // A liability's starting figure is what it owes, stored as a negative balance.
    const initialBalanceMinor = owes ? -amount : negative ? -amount : amount;
    const res = await create.mutateAsync({
      name: name.trim(),
      currency,
      type,
      initialBalanceMinor,
    });
    onOpenChange(false);
    setName("");
    setInitial("");
    setType(WalletType.BANK);
    setNegative(false);
    if (res.wallet) router.push(`/wallets/${res.wallet.id}`);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={submit} noValidate className="flex flex-col gap-6">
          <DialogHeader>
            <DialogTitle>Add wallet</DialogTitle>
            <DialogDescription>
              One wallet per account, card or cash pile. Its currency can't change later.
            </DialogDescription>
          </DialogHeader>
          <Field label="Name" htmlFor={fieldId("name")} error={errors.name}>
            <Input
              id={fieldId("name")}
              value={name}
              onChange={(e) => {
                setName(e.target.value);
                setErrors((x) => ({ ...x, name: undefined }));
              }}
              maxLength={60}
            />
          </Field>
          <Field label="Type" htmlFor={`${id}-type`}>
            <WalletTypeSelect id={`${id}-type`} value={type} onChange={setType} />
          </Field>
          <Field label="Currency" htmlFor={`${id}-currency`}>
            <CurrencySelect id={`${id}-currency`} value={currency} onChange={setCurrency} />
          </Field>
          <Field
            label={owes ? "Amount owed (optional)" : "Initial balance (optional)"}
            htmlFor={fieldId("initial")}
            error={errors.initial ?? (amount === null ? `Use up to ${units} decimals` : undefined)}
          >
            <div className="flex gap-2">
              {!owes && (
                <Button
                  type="button"
                  variant="secondary"
                  size="icon"
                  aria-label={negative ? "Negative balance" : "Positive balance"}
                  aria-pressed={negative}
                  onClick={() => setNegative((n) => !n)}
                  className="tabular text-amount-md"
                >
                  {negative ? "−" : "+"}
                </Button>
              )}
              <MoneyInput
                id={fieldId("initial")}
                currency={currency}
                value={initial}
                onChange={(e) => {
                  setInitial(e.target.value);
                  setErrors((x) => ({ ...x, initial: undefined }));
                }}
              />
            </div>
          </Field>
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={create.isPending}>
              Create wallet
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
