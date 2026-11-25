-- +goose Up
-- Rollover: a repeating budget can carry its leftover and/or its overspend into the next period.
-- The carry is derived from spending, never stored; rollover_from is the first period of the chain
-- (the period that contained "today" when rollover was turned on).
ALTER TABLE budgets
    ADD COLUMN carry_leftover boolean NOT NULL DEFAULT false,
    ADD COLUMN carry_overspend boolean NOT NULL DEFAULT false,
    ADD COLUMN rollover_from date,
    ADD CONSTRAINT budgets_rollover_from_check
        CHECK ((carry_leftover OR carry_overspend) = (rollover_from IS NOT NULL)),
    ADD CONSTRAINT budgets_rollover_repeat_check
        CHECK (repeat <> 'once' OR NOT (carry_leftover OR carry_overspend));

-- +goose Down
ALTER TABLE budgets
    DROP CONSTRAINT budgets_rollover_repeat_check,
    DROP CONSTRAINT budgets_rollover_from_check,
    DROP COLUMN rollover_from,
    DROP COLUMN carry_overspend,
    DROP COLUMN carry_leftover;
