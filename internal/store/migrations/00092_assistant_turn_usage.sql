-- +goose Up
-- How the turn used the generation model so far, as JSON: model calls, the last prompt and context in tokens, and
-- whether the model ran partly on the CPU; null before the first model reply.
alter table assistant_turns add column usage text;

-- +goose Down
alter table assistant_turns drop column usage;
