---
"@ledger-01/web": minor
"@ledger-01/backend": minor
"@ledger-01/api": minor
---

Wallets have a type: cash, bank account, investment or property (assets), or credit card or loan (liabilities). Existing wallets become bank accounts. Investment and property wallets take dated valuations, which set their value without counting as income or spending. A new Net worth page shows assets, what cards and loans owe, net worth over time and the installments still to come on cards and loans, by category; the dashboard links to it. Upcoming installments include the occurrences of ending recurring rules not created yet, and still never count as spending. MCP: `list_wallets` reports each wallet's type and value, and a new `net_worth` tool. Backups now carry wallet types and valuations (version 3; older backups still restore).
