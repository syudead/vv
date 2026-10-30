-- +goose Up
-- A proposal holds ordered steps: plan holds {"steps": [domain.AssistantPlan]} and steps each step's
-- domain.AssistantStepState, in card order. Rows of the one-shot build hold a single plan, so they are deleted.
delete from assistant_proposals;
alter table assistant_proposals add column steps text not null default '[]';
-- The turn that built the proposal, without a foreign key; null for an adoption from a review list.
alter table assistant_proposals add column turn_id integer;

-- +goose Down
delete from assistant_proposals;
alter table assistant_proposals drop column turn_id;
alter table assistant_proposals drop column steps;
