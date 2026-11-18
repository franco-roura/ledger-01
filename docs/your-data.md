[← README](../README.md) · [Features](features.md) · [Getting started](getting-started.md) · **Your data** · [AI assistants](ai-assistants.md) · [Self-hosting](self-hosting.md) · [Architecture](architecture.md) · [Development](development.md)

# Your data

How data gets into Ledger/01 and back out of it. It lives in your database, and one file moves it
anywhere.

**On this page:** [Import bank statements](#import-bank-statements) ·
[Discounts and refunds](#discounts-and-refunds) · [Supported banks](#supported-banks) ·
[Backups](#backups) · [Moving from local to AWS](#moving-from-local-to-aws) ·
[CSV export](#csv-export) · [Where your data lives](#where-your-data-lives)

## Import bank statements

Bank statements in **CSV, XLSX or XLS** import into any wallet. Open the wallet and choose **Import**,
or drop the file anywhere on the page. Statements with bank details above the table work too: the
import starts at the header row.

```mermaid
flowchart LR
  A["Choose a file<br/>CSV · XLSX · XLS"] --> B["Match the columns<br/>guessed, then remembered"]
  B --> C["Review the rows<br/>categories, rules, duplicates"]
  C --> D["Import"]
```

![Matching a statement's columns to date, amount and description](screenshots/import-columns.png)

At the review step:

- **Duplicates** start unticked. A row that matches one you imported before, even from a renamed
  file, is flagged as a possible duplicate.
- **Categories** come from your rules. Turn any row into a rule ("when the note contains …, use …")
  and it applies to every matching row, now and in later imports. Rules are listed under
  **Settings → Import rules**.
- **Filters** show all rows, ticked rows, rows that need a category, duplicates or rows left out.

![Reviewing six rows: five ticked, one possible duplicate](screenshots/import-review.png)

## Discounts and refunds

Ledger/01 recognises discounts and refunds and counts them against the purchase they belong to:

- a VAT refund is merged into its purchase, so the purchase shows what you actually paid;
- a double charge and its refund both start unticked, so neither inflates your spending.

To do this it needs to know how your bank words these lines.

## Card statements

Some banks only give you the card's spending as a PDF, and the account just shows one debit for
the whole bill. A card that spends in two currencies (pesos and dollars, say) keeps each in a
wallet of its own. Drop the statement PDF on the **Import** page of any of the card's wallets:

1. **Match the card's wallets:** each currency on the statement goes to the card's wallet in that
   currency. Pick it, create it on the spot, or leave that currency's lines out. Then pick the
   accounts the bill is debited from (all other accounts by default). Both are remembered on every
   card wallet, so next month the statement goes straight to the review, whichever wallet you
   drop it on. A month with only one currency simply imports that one.
2. **Review the rows:** purchases and fees become expenses, grouped by the wallet they go to. An
   instalment (`2/3`) is dated on the statement's issue date, with its number and purchase date in
   the note.
3. **The bill's payments** each find the debit already imported in one of the paying accounts and
   turn it into a transfer to the card, at the rate the two amounts imply. The debit no longer
   counts as spending, and the card's balance goes back to zero.

The bank doesn't always debit each total from the account in its currency. A debit for the same
amount in the same currency is taken first. Otherwise a debit in the other currency is matched
when its rate is within 15% of your own recent exchanges. A total paid partly from each account
becomes two transfers. If no debit is found yet, the payment is left out; next month's statement
lists it again.

## Supported banks

| Country | Bank                                       |
| ------- | ------------------------------------------ |
| Uruguay | Itaú (account statements, Visa statements) |

**Your bank isn't listed? Please add it.** A bank is one small Go file with a few patterns and a test,
and pull requests are welcome. [CONTRIBUTING.md](../CONTRIBUTING.md#adding-your-bank) walks you
through it.

> [!NOTE]
> Every bank imports fine without a definition. The definition only adds discount and refund
> matching.

## Backups

**Settings → Data** downloads the household's whole ledger as one gzipped JSON file.

| In a backup                                                                                   | Not in a backup                |
| --------------------------------------------------------------------------------------------- | ------------------------------ |
| Wallets, categories, labels                                                                   | Accounts and sign-in sessions  |
| Transactions, transfers, recurring transactions                                               | Connected apps (AI assistants) |
| Budgets                                                                                       |                                |
| Import settings: column mappings, rules, and the fingerprints that spot already-imported rows |                                |

**Restoring** a backup **replaces** everything in the household with the file's contents. It happens
in one database transaction, after you confirm, and the file is previewed before anything changes.
A backup restores on the same version of Ledger/01 or a newer one. Files can be up to 5 MB, which is
tens of thousands of transactions.

![Settings → Data with a backup file previewed before restoring](screenshots/settings-data.png)

## Moving from local to AWS

Backups work across instances, so this is also how you move:

1. On your local setup, go to **Settings → Data → Download backup**.
2. Sign in on your deployment.
3. Go to **Settings → Data** there and restore the file.

## CSV export

The same page exports transactions as CSV: UTF-8, and it opens in Excel. You can limit it to a date
range and to some wallets. Amounts are signed decimals in each wallet's currency.

## Where your data lives

- **In your Postgres**: local Docker in development, or your own [Neon](https://neon.com) project
  when deployed. Nobody else hosts a copy.
- **Money is stored exactly**, as integer minor units plus an ISO 4217 currency code, never as
  floating point. Transfer rates are exact decimals.
- **Every row belongs to a household**, and every query is scoped to the signed-in person's
  household.

---

[← Getting started](getting-started.md) · Next: [AI assistants →](ai-assistants.md)
