# @ledger-01/api

## 1.3.0

### Minor Changes

- [#46](https://github.com/franco-roura/ledger-01/pull/46) [`74fc821`](https://github.com/franco-roura/ledger-01/commit/74fc821e4f7b8a54f3a5135b4191f58eaed87ee9) Thanks [@franco-roura](https://github.com/franco-roura)! - Import rules can rename the rows they match (say, a card's merchant code to "Uber"), and offer to rename the transactions imported before. Rules can be edited, and a single row can get its own name while reviewing an import.

## 1.2.0

### Minor Changes

- [#40](https://github.com/franco-roura/ledger-01/pull/40) [`951a236`](https://github.com/franco-roura/ledger-01/commit/951a236726ea7ee677c3c83bbadf661de4294b52) Thanks [@franco-roura](https://github.com/franco-roura)! - Import card statement PDFs (Itaú Visa, Uruguay) into the card's wallets, one per currency it spends in: purchases and fees become expenses, and the bank's debits for the bill, from whichever account paid it, become transfers to the card.

### Patch Changes

- [#11](https://github.com/franco-roura/ledger-01/pull/11) [`c8f1372`](https://github.com/franco-roura/ledger-01/commit/c8f1372021ca0372f6bfe47f622df49bf07b0127) Thanks [@gbarrera97](https://github.com/gbarrera97)! - Keep a budget from counting unrelated expenses when its last selected wallet or category is deleted. Choose all wallets or all expense categories explicitly, and show a warning when a budget has an empty scope.

## 1.1.0

No changes in this release.

## 1.0.1

No changes in this release.
