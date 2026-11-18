# @ledger-01/backend

## 1.2.0

### Minor Changes

- [#40](https://github.com/franco-roura/ledger-01/pull/40) [`951a236`](https://github.com/franco-roura/ledger-01/commit/951a236726ea7ee677c3c83bbadf661de4294b52) Thanks [@franco-roura](https://github.com/franco-roura)! - Import card statement PDFs (Itaú Visa, Uruguay) into the card's wallets, one per currency it spends in: purchases and fees become expenses, and the bank's debits for the bill, from whichever account paid it, become transfers to the card.

### Patch Changes

- [#11](https://github.com/franco-roura/ledger-01/pull/11) [`c8f1372`](https://github.com/franco-roura/ledger-01/commit/c8f1372021ca0372f6bfe47f622df49bf07b0127) Thanks [@gbarrera97](https://github.com/gbarrera97)! - Keep a budget from counting unrelated expenses when its last selected wallet or category is deleted. Choose all wallets or all expense categories explicitly, and show a warning when a budget has an empty scope.

## 1.1.0

### Patch Changes

- [#18](https://github.com/franco-roura/ledger-01/pull/18) [`fa49980`](https://github.com/franco-roura/ledger-01/commit/fa49980a30edbdb71c780f8be65811d8c87c2f9e) Thanks [@gbarrera97](https://github.com/gbarrera97)! - Show matching category transactions even when they fall beyond the newest 200 unfiltered rows. Note and converted amount filters now apply before the response limit.

## 1.0.1

### Patch Changes

- [#5](https://github.com/franco-roura/ledger-01/pull/5) [`c04e2c2`](https://github.com/franco-roura/ledger-01/commit/c04e2c2ed81e1a74836c6012e4287aa55802dc24) Thanks [@franco-roura](https://github.com/franco-roura)! - Security updates: go-jose 4.1.6, used to verify Google sign-in tokens, and excelize, which reads uploaded XLSX statements. Both fix crashes or runaway CPU and memory on malformed input.
