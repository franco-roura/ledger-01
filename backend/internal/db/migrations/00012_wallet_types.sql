-- +goose Up
-- What a wallet is: cash, bank, investment and property are assets; credit_card and loan are
-- liabilities. Net worth adds assets and subtracts what liabilities owe.
CREATE TYPE wallet_type AS ENUM ('cash', 'bank', 'investment', 'property', 'credit_card', 'loan');

ALTER TABLE wallets ADD COLUMN type wallet_type NOT NULL DEFAULT 'bank';

-- What an investment or property wallet is worth on a date. Kept apart from transactions, so a
-- change in value never counts as income or spending. The wallet's value on a later date is the
-- latest valuation plus the wallet's movements after it.
CREATE TABLE wallet_valuations (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id uuid        NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    wallet_id    uuid        NOT NULL REFERENCES wallets (id) ON DELETE CASCADE,
    valued_on    date        NOT NULL,
    value_minor  bigint      NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX wallet_valuations_wallet_date_key ON wallet_valuations (wallet_id, valued_on);
CREATE INDEX wallet_valuations_household_idx ON wallet_valuations (household_id);

-- +goose Down
DROP TABLE wallet_valuations;
ALTER TABLE wallets DROP COLUMN type;
DROP TYPE wallet_type;
