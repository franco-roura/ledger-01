---
"@ledger-01/web": minor
"@ledger-01/backend": minor
"@ledger-01/api": minor
---

Repeating budgets can roll over. "Carry leftover" adds unspent money to the next period's limit, and "Carry overspend" takes going over out of it; the carry chains from the period rollover was turned on. Budget cards, the budget page and the MCP `budget_status` tool show the base amount, what carried over and the period's effective limit.
