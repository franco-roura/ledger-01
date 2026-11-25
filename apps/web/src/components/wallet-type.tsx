"use client";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@franco_roura/module-01";
import { WalletType } from "@ledger-01/api";
import {
  Banknote,
  CreditCard,
  HandCoins,
  House,
  Landmark,
  type LucideIcon,
  TrendingUp,
} from "lucide-react";
import { walletTypeLabel, walletTypeOptions } from "@/lib/wallet-types";

const icons: Record<WalletType, LucideIcon> = {
  [WalletType.UNSPECIFIED]: Landmark,
  [WalletType.BANK]: Landmark,
  [WalletType.CASH]: Banknote,
  [WalletType.INVESTMENT]: TrendingUp,
  [WalletType.PROPERTY]: House,
  [WalletType.CREDIT_CARD]: CreditCard,
  [WalletType.LOAN]: HandCoins,
};

/** The wallet's type as a Lucide icon, labelled for screen readers. */
export function WalletTypeIcon({ type, className }: { type: WalletType; className?: string }) {
  const Icon = icons[type] ?? Landmark;
  return <Icon className={className} strokeWidth={1.25} aria-label={walletTypeLabel(type)} />;
}

export function WalletTypeSelect({
  id,
  value,
  onChange,
  disabled,
}: {
  id?: string;
  value: WalletType;
  onChange: (t: WalletType) => void;
  disabled?: boolean;
}) {
  return (
    <Select
      value={String(value === WalletType.UNSPECIFIED ? WalletType.BANK : value)}
      onValueChange={(v) => onChange(Number(v) as WalletType)}
      disabled={disabled}
    >
      <SelectTrigger id={id} className="w-full">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {walletTypeOptions.map((o) => (
          <SelectItem key={o.value} value={String(o.value)}>
            <span className="flex items-center gap-2">
              <WalletTypeIcon type={o.value} className="size-4 text-ink-soft" />
              {o.label}
            </span>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
