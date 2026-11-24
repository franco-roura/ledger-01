-- +goose Up
-- What a rule renames the rows it matches to; empty keeps the bank's text.
ALTER TABLE import_category_rules
    ADD COLUMN rename text NOT NULL DEFAULT '' CHECK (char_length(rename) <= 100);

-- +goose Down
ALTER TABLE import_category_rules DROP COLUMN rename;
