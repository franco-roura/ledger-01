import type { Metadata } from "next";
import { Suspense } from "react";
import { TransactionsView } from "./transactions-view";

export const metadata: Metadata = { title: "Transactions" };

export default function TransactionsPage() {
  return (
    <Suspense fallback={<p className="text-body-sm text-ink-soft">Loading transactions…</p>}>
      <TransactionsView />
    </Suspense>
  );
}
