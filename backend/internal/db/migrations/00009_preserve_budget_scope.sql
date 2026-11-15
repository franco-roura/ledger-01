-- +goose Up
ALTER TABLE budgets
    ADD COLUMN all_wallets boolean NOT NULL DEFAULT false,
    ADD COLUMN all_categories boolean NOT NULL DEFAULT false;

-- Before this migration, each empty selection independently meant "all".
UPDATE budgets b
SET all_wallets = NOT EXISTS (SELECT FROM budget_wallets bw WHERE bw.budget_id = b.id),
    all_categories = NOT EXISTS (SELECT FROM budget_categories bc WHERE bc.budget_id = b.id);

-- +goose Down
ALTER TABLE budgets DROP COLUMN all_wallets, DROP COLUMN all_categories;
