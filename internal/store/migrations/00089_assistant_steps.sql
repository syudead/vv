-- +goose Up
-- A proposal holds ordered steps: plan holds {"steps": [domain.AssistantPlan]} and steps each step's
-- domain.AssistantStepState, in card order. Rows of the one-shot build hold a single plan, so they are deleted; the
-- run entry of an approved one is interrupted first, as no recovery reaches it once its proposal is gone.
update activity_entries set status = 'attention', reason = 'interrupted', updated_at = unixepoch() * 1000
where status = 'running' and id in (select activity_id from assistant_proposals where activity_id is not null);
delete from assistant_proposals;
alter table assistant_proposals add column steps text not null default '[]';
-- The turn that built the proposal, without a foreign key; null for an adoption from a review list.
alter table assistant_proposals add column turn_id integer;

-- +goose Down
update activity_entries set status = 'attention', reason = 'interrupted', updated_at = unixepoch() * 1000
where status = 'running' and id in (select activity_id from assistant_proposals where activity_id is not null);
delete from assistant_proposals;
alter table assistant_proposals drop column turn_id;
alter table assistant_proposals drop column steps;
