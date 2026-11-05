-- +goose Up
-- The numbers and names of a shown answer that no tool result holds, as a JSON array; null when every item was
-- checked.
alter table assistant_turns add column unchecked text;

-- +goose Down
alter table assistant_turns drop column unchecked;
