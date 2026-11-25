import { WalletType } from "@ledger-01/api";

/** Wallet types in the order the form offers them: assets first, then liabilities. */
export const walletTypeOptions: { value: WalletType; label: string }[] = [
  { value: WalletType.BANK, label: "Bank account" },
  { value: WalletType.CASH, label: "Cash" },
  { value: WalletType.INVESTMENT, label: "Investment" },
  { value: WalletType.PROPERTY, label: "Property" },
  { value: WalletType.CREDIT_CARD, label: "Credit card" },
  { value: WalletType.LOAN, label: "Loan" },
];

/** The API reports unspecified as bank; treat it the same here. */
const normal = (t: WalletType) => (t === WalletType.UNSPECIFIED ? WalletType.BANK : t);

export const walletTypeLabel = (t: WalletType) =>
  walletTypeOptions.find((o) => o.value === normal(t))?.label ?? "Bank account";

/** Credit cards and loans hold debt: their (usually negative) balance is what is owed. */
export const isLiability = (t: WalletType) => t === WalletType.CREDIT_CARD || t === WalletType.LOAN;

/** Investments and property are valued with dated valuations instead of their ledger balance. */
export const takesValuations = (t: WalletType) =>
  t === WalletType.INVESTMENT || t === WalletType.PROPERTY;

/**
 * Splits wallets into assets and liabilities, keeping their order. Hidden and archived wallets
 * are left to the caller.
 */
export function byClass<W extends { type: WalletType }>(wallets: W[]) {
  return {
    assets: wallets.filter((w) => !isLiability(w.type)),
    liabilities: wallets.filter((w) => isLiability(w.type)),
  };
}
