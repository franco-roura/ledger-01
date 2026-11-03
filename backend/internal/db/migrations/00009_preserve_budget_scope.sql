-- +goose Up
-- An empty selection means all wallets/categories. Cascading away the last link would
-- therefore broaden a budget. Require its selection to be updated before deleting a target.
ALTER TABLE budget_wallets DROP CONSTRAINT budget_wallets_wallet_id_fkey;
ALTER TABLE budget_wallets ADD CONSTRAINT budget_wallets_wallet_id_fkey
    FOREIGN KEY (wallet_id) REFERENCES wallets (id) ON DELETE NO ACTION;

ALTER TABLE budget_categories DROP CONSTRAINT budget_categories_category_id_fkey;
ALTER TABLE budget_categories ADD CONSTRAINT budget_categories_category_id_fkey
    FOREIGN KEY (category_id) REFERENCES categories (id) ON DELETE NO ACTION;

-- +goose Down
ALTER TABLE budget_categories DROP CONSTRAINT budget_categories_category_id_fkey;
ALTER TABLE budget_categories ADD CONSTRAINT budget_categories_category_id_fkey
    FOREIGN KEY (category_id) REFERENCES categories (id) ON DELETE CASCADE;

ALTER TABLE budget_wallets DROP CONSTRAINT budget_wallets_wallet_id_fkey;
ALTER TABLE budget_wallets ADD CONSTRAINT budget_wallets_wallet_id_fkey
    FOREIGN KEY (wallet_id) REFERENCES wallets (id) ON DELETE CASCADE;
