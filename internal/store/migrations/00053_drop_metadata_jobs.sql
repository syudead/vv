-- +goose Up
-- Remove obsolete metadata jobs and issues because no worker can finish them; startup recomputes completion without rewriting released migration 00052.
update scans set issues_revision = issues_revision + 1
 where id = (select max(id) from scans)
   and exists (select 1 from scan_issues where kind = 'metadata_failed');
delete from scan_issues where kind = 'metadata_failed';
delete from jobs where kind = 'metadata';

-- +goose Down
select 1;
