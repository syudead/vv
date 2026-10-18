-- +goose Up
-- Keep legacy free-form errors intact and leave new codes NULL rather than guessing structured reasons from text.
alter table videos add column probe_error_code text;
alter table scans add column error_code text;
-- Present only when the failure refers to a specific location.
alter table scans add column error_path text;

-- +goose Down
alter table scans drop column error_path;
alter table scans drop column error_code;
alter table videos drop column probe_error_code;
