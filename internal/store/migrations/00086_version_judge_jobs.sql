-- +goose Up
-- Duplicate-judging work moves into jobs; a candidate keeps only its judgement. A waiting or interrupted pair is
-- queued in its listing order, and a failed pair keeps its attempts and retry time on its job.
insert into jobs (kind, subject_key, subject_key_b, state, attempts, created_at, updated_at, activity_id, priority,
                  run_after)
select 'version_judge', key_a, key_b, 'queued', 0, created_at, created_at, activity_id, 1, created_at
from video_version_candidates where judgement is null or judgement = 'running'
order by created_at, key_a, key_b;
insert into jobs (kind, subject_key, subject_key_b, state, attempts, created_at, updated_at, activity_id, priority,
                  run_after)
select 'version_judge', key_a, key_b, 'queued', attempts, created_at, created_at, activity_id, 1, retry_at
from video_version_candidates where judgement = 'failed' and retry_at is not null
order by retry_at, key_a, key_b;
update video_version_candidates set judgement = null where judgement = 'running';
alter table video_version_candidates drop column retry_at;
alter table video_version_candidates drop column attempts;
-- A pair's job goes with its candidate row, whichever statement removes it; a rebuilt row is queued afresh.
-- +goose StatementBegin
create trigger video_version_candidates_job_delete after delete on video_version_candidates
begin
    delete from jobs where kind = 'version_judge' and subject_key = old.key_a and subject_key_b = old.key_b;
end;
-- +goose StatementEnd

-- +goose Down
drop trigger video_version_candidates_job_delete;
alter table video_version_candidates add column attempts integer not null default 0;
alter table video_version_candidates add column retry_at integer;
update video_version_candidates as c
set attempts = coalesce((select j.attempts from jobs j where j.kind = 'version_judge' and j.subject_key = c.key_a
                          and j.subject_key_b = c.key_b and j.state in ('queued', 'running')), 0),
    retry_at = case when c.judgement = 'failed' then (
        select j.run_after from jobs j where j.kind = 'version_judge' and j.subject_key = c.key_a
           and j.subject_key_b = c.key_b and j.state = 'queued') end;
-- The candidate claim takes a waiting row, so an interrupted pair waits again, as startup recovery did.
update video_version_candidates set judgement = null where judgement = 'running';
delete from jobs where kind = 'version_judge';
