# @ledger-01/backend

## 1.1.0

### Patch Changes

- [#18](https://github.com/franco-roura/ledger-01/pull/18) [`fa49980`](https://github.com/franco-roura/ledger-01/commit/fa49980a30edbdb71c780f8be65811d8c87c2f9e) Thanks [@gbarrera97](https://github.com/gbarrera97)! - Show matching category transactions even when they fall beyond the newest 200 unfiltered rows. Note and converted amount filters now apply before the response limit.

## 1.0.1

### Patch Changes

- [#5](https://github.com/franco-roura/ledger-01/pull/5) [`c04e2c2`](https://github.com/franco-roura/ledger-01/commit/c04e2c2ed81e1a74836c6012e4287aa55802dc24) Thanks [@franco-roura](https://github.com/franco-roura)! - Security updates: go-jose 4.1.6, used to verify Google sign-in tokens, and excelize, which reads uploaded XLSX statements. Both fix crashes or runaway CPU and memory on malformed input.
