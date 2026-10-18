-- +goose Up
-- Plugin modules and settings are persistent user data, independent of videos and rescanning.

-- Never reuse deleted plugin IDs, preventing stale approval screens from targeting another plugin.
create table work_info_plugins (
    id              integer primary key autoincrement,
    -- Unreadable manifest names are NULL and do not conflict with one another.
    name            text    unique,
    file_name       text    not null,
    version         text,
    description     text,
    contract        integer,
    -- Comma-separated manifest operations: search and fetch.
    operations      text,
    -- JSON settings declarations from the manifest.
    settings        text,
    -- Module bytes are NULL for unusable uploads.
    module          blob,
    -- Lowercase hexadecimal SHA-256 of uploaded bytes.
    digest          text    not null,
    problem         text,
    -- Approved only when this matches digest and problem is NULL.
    approved_digest text,
    -- First upload and latest upload/approval-change times in Unix seconds.
    added_at        integer not null,
    updated_at      integer not null
);

create table work_info_plugin_settings (
    plugin_id  integer not null references work_info_plugins (id) on delete cascade,
    key        text    not null,
    -- At most 2000 characters, and only declared keys; both application and store validate this.
    value      text    not null,
    updated_at integer not null,
    primary key (plugin_id, key)
) without rowid;

-- +goose Down
drop table if exists work_info_plugin_settings;
drop table if exists work_info_plugins;
