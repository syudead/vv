-- +goose Up
-- Order in which the plugins that declare domains are tried for a link, lowest first; null for a plugin without domains.
alter table work_info_plugins add column link_rank integer;
-- 1 for the plugin VVMDM bundles: it has no module, its manifest comes from the code, and it is never replaced or deleted.
alter table work_info_plugins add column builtin integer not null default 0 check (builtin in (0, 1));
-- The bundled link plugin starts approved, after every uploaded plugin that declares domains (none could before now).
insert or ignore into work_info_plugins
    (name, file_name, version, description, contract, operations, settings, module, digest, problem, approved_digest,
     added_at, updated_at, kind, domains, builtin, link_rank)
values ('vv.link', 'Built in', '1', '', 1, 'fetch', '[]', null, 'builtin:vv.link', null, 'builtin:vv.link',
        unixepoch(), unixepoch(), 'work', '["*"]', 1, 1);

-- 1 while a virtual video created from a link still has the title VVMDM gave it, which a fetch may replace.
alter table virtual_videos add column title_from_link integer not null default 0 check (title_from_link in (0, 1));
-- The fetch started when the video was created from a link: null for none, then running, failed or done.
alter table virtual_videos add column link_fetch text check (link_fetch in ('running', 'failed', 'done'));
-- Why a failed fetch failed, as the owner reads it.
alter table virtual_videos add column link_fetch_failure text;
-- The name of the plugin the fetch ran, null when no plugin handled the link.
alter table virtual_videos add column link_fetch_plugin text;

-- +goose Down
alter table virtual_videos drop column link_fetch_plugin;
alter table virtual_videos drop column link_fetch_failure;
alter table virtual_videos drop column link_fetch;
alter table virtual_videos drop column title_from_link;
delete from work_info_plugins where builtin = 1;
alter table work_info_plugins drop column builtin;
alter table work_info_plugins drop column link_rank;
