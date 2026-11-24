# @ledger-01/web

## 1.3.0

### Minor Changes

- [#46](https://github.com/franco-roura/ledger-01/pull/46) [`74fc821`](https://github.com/franco-roura/ledger-01/commit/74fc821e4f7b8a54f3a5135b4191f58eaed87ee9) Thanks [@franco-roura](https://github.com/franco-roura)! - Import rules can rename the rows they match (say, a card's merchant code to "Uber"), and offer to rename the transactions imported before. Rules can be edited, and a single row can get its own name while reviewing an import.

### Patch Changes

- [#50](https://github.com/franco-roura/ledger-01/pull/50) [`bed02ea`](https://github.com/franco-roura/ledger-01/commit/bed02ea0e034feacfd15aba86c39bd33d8200b0b) Thanks [@franco-roura](https://github.com/franco-roura)! - Amounts with a misplaced thousands separator, such as `12,50`, are now rejected instead of being read as 1,250.
- Updated dependencies [[`74fc821`](https://github.com/franco-roura/ledger-01/commit/74fc821e4f7b8a54f3a5135b4191f58eaed87ee9)]:
  - @ledger-01/api@1.3.0

## 1.2.0

### Minor Changes

- [#40](https://github.com/franco-roura/ledger-01/pull/40) [`951a236`](https://github.com/franco-roura/ledger-01/commit/951a236726ea7ee677c3c83bbadf661de4294b52) Thanks [@franco-roura](https://github.com/franco-roura)! - Import card statement PDFs (Itaú Visa, Uruguay) into the card's wallets, one per currency it spends in: purchases and fees become expenses, and the bank's debits for the bill, from whichever account paid it, become transfers to the card.

### Patch Changes

- [#11](https://github.com/franco-roura/ledger-01/pull/11) [`c8f1372`](https://github.com/franco-roura/ledger-01/commit/c8f1372021ca0372f6bfe47f622df49bf07b0127) Thanks [@gbarrera97](https://github.com/gbarrera97)! - Keep a budget from counting unrelated expenses when its last selected wallet or category is deleted. Choose all wallets or all expense categories explicitly, and show a warning when a budget has an empty scope.
- Updated dependencies [[`951a236`](https://github.com/franco-roura/ledger-01/commit/951a236726ea7ee677c3c83bbadf661de4294b52), [`c8f1372`](https://github.com/franco-roura/ledger-01/commit/c8f1372021ca0372f6bfe47f622df49bf07b0127)]:
  - @ledger-01/api@1.2.0

## 1.1.0

### Minor Changes

- [#22](https://github.com/franco-roura/ledger-01/pull/22) [`9726c73`](https://github.com/franco-roura/ledger-01/commit/9726c73f518573e075296aa1b87b8713e2ef6abd) Thanks [@aaugoustis](https://github.com/aaugoustis)! - Add transactions from the dashboard: a **New transaction** button (the pinned + on phones) that starts in the wallet you last added to, with a Wallet field to pick another. The installed app also gets a **New transaction** shortcut: press and hold its icon.

### Patch Changes

- [#30](https://github.com/franco-roura/ledger-01/pull/30) [`b5b02b1`](https://github.com/franco-roura/ledger-01/commit/b5b02b140b579c3938c2a74ea7e5ba3aad7f21d2) Thanks [@franco-roura](https://github.com/franco-roura)! - Use the published Module/01 kit (`@franco_roura/module-01`) instead of a copy kept in this repository.
- Updated dependencies []:
  - @ledger-01/api@1.1.0

## 1.0.1

### Patch Changes

- Updated dependencies []:
  - @ledger-01/api@1.0.1
  - @ledger-01/ui@1.0.1

## 1.0.0

Initial public release of Ledger/01, a self-hostable personal and household finance app.

- Wallets in any currency, with categories, labels and notes on every transaction.
- Transfers between currencies: enter any two of the amount sent, the amount received and the rate,
  and the third is worked out.
- Dashboards: balance over time, cash flow and spending by category, filtered by wallet, category,
  note or amount.
- Budgets for any set of categories and wallets, one-off or repeating.
- Recurring transactions, with upcoming ones in a Future view that stay out of balances until their
  date.
- Bank statement import (CSV, XLSX, XLS) with column mapping and duplicate detection. Discounts and
  refunds are matched to the purchases they belong to.
- Backups: download the whole household as one file, restore it here or on another instance, and
  export transactions as CSV.
- Shared households: invite people by email, see and edit the same wallets, and filter
  transactions by who entered them.
- Ask Claude about your money: read-only MCP tools on `/mcp`, connected over OAuth with your
  Google sign-in, and a Connected apps page to disconnect them.
- Installs on phones and desktops as a PWA; the sign-in page offers to install it.
- Google sign-in for the people you allow.
- Deploys to your own AWS account with SST: the Go API on Lambda behind API Gateway, the web app
  through OpenNext, Postgres on Neon.
