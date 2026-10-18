-- +goose Up
-- Persist only the latest run's stderr per plugin.
create table work_info_plugin_runs (
    plugin_id  integer primary key references work_info_plugins (id) on delete cascade,
    -- Write only while the plugin still has the generation captured at run start.
    generation integer not null,
    -- Either search or fetch.
    operation  text    not null,
    -- Failure kind, NULL on success.
    failure    text,
    -- Completion time in Unix seconds.
    ran_at     integer not null,
    -- First 64 KiB of stderr with secret settings replaced by [secret].
    stderr     text    not null,
    -- One when trailing stderr was discarded.
    truncated  integer not null
);

-- +goose Down
drop table if exists work_info_plugin_runs;
