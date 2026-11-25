-- name: ListWalletValuations :many
SELECT v.id, v.wallet_id, v.valued_on, v.value_minor, v.created_at
FROM wallet_valuations v
WHERE v.household_id = @household_id AND v.wallet_id = @wallet_id
ORDER BY v.valued_on DESC;

-- name: SaveWalletValuation :one
-- One valuation per wallet and date: saving the same date again replaces the value.
INSERT INTO wallet_valuations (id, household_id, wallet_id, valued_on, value_minor)
VALUES (@id, @household_id, @wallet_id, @valued_on, @value_minor)
ON CONFLICT (wallet_id, valued_on) DO UPDATE SET value_minor = excluded.value_minor, updated_at = now()
RETURNING id, wallet_id, valued_on, value_minor, created_at;

-- name: DeleteWalletValuation :execrows
DELETE FROM wallet_valuations WHERE id = @id AND household_id = @household_id;

-- name: ValuationOffsets :many
-- Every valuation up to today with the wallet's ledger balance on its date, oldest first. The
-- difference (value - ledger balance) is carried forward: the wallet's value on a later date is
-- its ledger balance then plus that difference.
SELECT v.wallet_id,
       v.valued_on,
       v.value_minor,
       (w.initial_balance_minor + coalesce((
           SELECT sum(CASE WHEN t.kind IN ('income', 'transfer_in') THEN t.amount_minor ELSE -t.amount_minor END)
           FROM transactions t
           WHERE t.wallet_id = v.wallet_id AND t.occurred_on <= v.valued_on), 0))::bigint AS ledger_balance_minor
FROM wallet_valuations v
JOIN wallets w ON w.id = v.wallet_id
WHERE v.household_id = @household_id AND v.valued_on <= @today
ORDER BY v.wallet_id, v.valued_on;

-- name: UpcomingLiabilityRows :many
-- Expenses and income dated after today on the given (liability) wallets, per wallet and
-- category. They are debt still to come, never spending.
SELECT t.wallet_id, t.category_id, t.kind,
       sum(t.amount_minor)::bigint AS amount_minor,
       count(*)::bigint AS row_count
FROM transactions t
WHERE t.household_id = @household_id
  AND t.wallet_id = ANY(@wallet_ids::uuid[])
  AND t.kind IN ('expense', 'income')
  AND t.occurred_on > @today
GROUP BY t.wallet_id, t.category_id, t.kind;

-- name: FiniteLiabilityRules :many
-- Expense and income rules on the given (liability) wallets that end after the occurrences they
-- have created so far: the rest of an installment plan.
SELECT r.id, r.wallet_id, r.kind, r.category_id, r.amount_minor, r.frequency, r.start_date, r.end_date,
       r.generated_through
FROM recurring_rules r
WHERE r.household_id = @household_id
  AND r.wallet_id = ANY(@wallet_ids::uuid[])
  AND r.kind IN ('expense', 'income')
  AND r.end_date IS NOT NULL
  AND r.end_date > r.generated_through;
