---
"@ledger-01/ui": patch
---

Reject decimal-comma amounts and exchange rates instead of silently treating them as larger numbers. Correctly grouped values such as 1,234.50 remain supported.
