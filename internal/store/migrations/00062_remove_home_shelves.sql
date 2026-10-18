-- +goose Up
-- Remove the retired Home shelf indexes and bookkeeping. User collections remain.
drop trigger if exists groups_closure_ai;
drop trigger if exists groups_closure_au;
drop trigger if exists groups_closure_ad;
drop trigger if exists group_items_closure_ai;
drop trigger if exists group_items_closure_au;
drop trigger if exists group_items_closure_ad;
drop trigger if exists videos_root_values_ai;
drop trigger if exists videos_root_values_au;
drop trigger if exists videos_root_values_ad;
drop trigger if exists video_locations_root_values_ai;
drop trigger if exists video_locations_root_values_au;
drop trigger if exists video_locations_root_values_ad;
drop trigger if exists playback_progress_root_values_ai;
drop trigger if exists playback_progress_root_values_au;
drop trigger if exists playback_progress_root_values_ad;
drop trigger if exists media_folders_root_values_ai;
drop trigger if exists media_folders_root_values_au;
drop trigger if exists media_folders_root_values_ad;
drop trigger if exists video_bundle_members_root_values_ai;
drop trigger if exists video_bundle_members_root_values_au;
drop trigger if exists video_bundle_members_root_values_ad;
drop trigger if exists video_bundles_root_values_au;
drop trigger if exists public_videos_root_values_ai;
drop trigger if exists public_videos_root_values_ad;
drop trigger if exists videos_shelved_ai;
drop trigger if exists videos_shelved_au;
drop trigger if exists videos_shelved_ad;
drop trigger if exists video_locations_shelved_ai;
drop trigger if exists video_locations_shelved_au;
drop trigger if exists video_locations_shelved_ad;
drop trigger if exists video_tags_shelved_ai;
drop trigger if exists video_tags_shelved_ad;
drop trigger if exists playback_progress_shelved_ai;
drop trigger if exists playback_progress_shelved_au;
drop trigger if exists playback_progress_shelved_ad;
drop trigger if exists video_favorites_shelved_ai;
drop trigger if exists video_favorites_shelved_ad;
drop trigger if exists video_work_people_shelved_ai;
drop trigger if exists video_work_people_shelved_ad;
drop trigger if exists video_work_organizations_shelved_ai;
drop trigger if exists video_work_organizations_shelved_ad;
drop trigger if exists smart_folders_shelved_ai;
drop trigger if exists smart_folders_shelved_au;
drop trigger if exists smart_folders_shelved_ad;
drop trigger if exists tags_shelved_ad;
drop trigger if exists tags_shelved_au;
drop trigger if exists tag_names_shelved_ai;
drop trigger if exists tag_names_shelved_ad;
drop trigger if exists video_folder_names_shelved_ai;
drop trigger if exists video_folder_names_shelved_ad;
drop trigger if exists video_bundle_members_shelved_ai;
drop trigger if exists video_bundle_members_shelved_ad;
drop trigger if exists media_folders_shelved_ai;
drop trigger if exists media_folders_shelved_ad;
drop trigger if exists video_locations_search_ai;
drop trigger if exists video_locations_search_au;
drop trigger if exists video_locations_search_ad;
drop trigger if exists video_tags_search_ai;
drop trigger if exists video_tags_search_ad;
drop trigger if exists tag_names_search_ai;
drop trigger if exists tag_names_search_au;
drop trigger if exists tag_names_search_ad;
drop trigger if exists video_work_info_search_ai;
drop trigger if exists video_work_info_search_au;
drop trigger if exists video_work_info_search_ad;
drop trigger if exists people_names_search_ai;
drop trigger if exists people_names_search_au;
drop trigger if exists people_names_search_ad;
drop trigger if exists video_work_people_search_ai;
drop trigger if exists video_work_people_search_ad;
drop trigger if exists organization_names_search_ai;
drop trigger if exists organization_names_search_au;
drop trigger if exists organization_names_search_ad;
drop trigger if exists video_work_organizations_search_ai;
drop trigger if exists video_work_organizations_search_ad;
drop trigger if exists video_folder_names_search_ai;
drop trigger if exists video_folder_names_search_ad;
drop trigger if exists video_bundle_members_search_ai;
drop trigger if exists video_bundle_members_search_ad;
drop trigger if exists video_bundles_search_au;
drop trigger if exists media_folders_search_ai;
drop trigger if exists media_folders_search_ad;
drop trigger if exists smart_folders_search_ai;
drop trigger if exists smart_folders_search_au;
drop trigger if exists smart_folders_search_ad;
drop trigger if exists videos_watch_ai;
drop trigger if exists videos_watch_au;
drop trigger if exists videos_watch_ad;
drop trigger if exists playback_progress_watch_ai;
drop trigger if exists playback_progress_watch_au;
drop trigger if exists playback_progress_watch_ad;
drop trigger if exists video_bundle_members_watch_ai;
drop trigger if exists video_bundle_members_watch_au;
drop trigger if exists video_bundle_members_watch_ad;
drop trigger if exists video_bundles_watch_au;
drop index if exists smart_folders_home_idx;
drop index if exists group_closure_key_idx;
drop index if exists home_shelved_dirty_key_idx;
drop index if exists video_locations_mtime_idx;
drop index if exists video_locations_created_idx;
drop index if exists playback_progress_updated_idx;
drop index if exists group_root_values_watch_idx;
drop index if exists video_watch_states_state_idx;
drop index if exists video_watch_dirty_key_idx;
drop table if exists group_closure;
drop table if exists group_closure_dirty;
drop table if exists group_root_values;
drop table if exists home_shelved_videos;
drop table if exists home_shelved_dirty;
drop table if exists home_shelved_state;
drop table if exists video_watch_states;
drop table if exists home_search_hits;
drop table if exists video_watch_dirty;
drop table if exists home_index_state;
alter table smart_folders drop column home_position;
alter table smart_folders drop column owner_kept;

-- +goose Down
-- Restore only the former schema for rollback; derived caches rebuild in the older application.
alter table smart_folders add column owner_kept integer not null default 1 check (owner_kept in (0, 1));
alter table smart_folders add column home_position integer;
CREATE TABLE group_closure (
    root_id     integer not null references groups (id) on delete cascade,
    content_key text    not null,
    sort_key    text    not null,
    primary key (root_id, content_key)
) without rowid;
CREATE TABLE group_closure_dirty (
    id    integer primary key check (id = 1),
    dirty integer not null
);
CREATE TABLE group_root_values (
    root_id        integer not null references groups (id) on delete cascade,
    audience       integer not null check (audience in (0, 1)),
    members        integer not null,
    added_at       integer,
    mtime          integer,
    created_at     integer,
    played_at      integer,
    duration_ms    integer,
    size_bytes     integer,
    watched        integer not null default 0,
    started        integer not null default 0,
    playable       integer not null default 0,
    added_member   integer,
    mtime_member   integer,
    created_member integer,
    played_member  integer, open_member integer, previews text not null default '[]', watch_state text generated always as (
    case when started = 0 then 'unwatched' when watched = members then 'watched' else 'inProgress' end) virtual,
    primary key (root_id, audience)
) without rowid;
CREATE TABLE "video_watch_dirty" (
    content_key text not null
);
CREATE TABLE "home_index_state" (
    id        integer primary key check (id = 1),
    watch_dirty integer not null
, search_dirty integer not null default 1);
CREATE TABLE video_watch_states (
    content_key text primary key,
    state       text not null check (state in ('unwatched', 'inProgress', 'watched'))
) without rowid;
CREATE TABLE home_search_hits (
    search   text not null,
    video_id integer not null,
    primary key (search, video_id)
) without rowid;
CREATE INDEX smart_folders_home_idx on smart_folders (home_position) where home_position is not null;
CREATE INDEX group_closure_key_idx on group_closure (content_key, root_id);
CREATE INDEX video_locations_mtime_idx on video_locations (mtime, video_id);
CREATE INDEX video_locations_created_idx on video_locations (coalesce(file_created_at, mtime), video_id);
CREATE INDEX playback_progress_updated_idx on playback_progress (updated_at, content_key);
CREATE INDEX group_root_values_watch_idx on group_root_values (audience, watch_state);
CREATE INDEX video_watch_states_state_idx on video_watch_states (state, content_key);
CREATE INDEX video_watch_dirty_key_idx on video_watch_dirty(content_key);
-- +goose StatementBegin
CREATE TRIGGER groups_closure_ai after insert on groups begin
    update group_closure_dirty set dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER groups_closure_au after update on groups begin
    update group_closure_dirty set dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER groups_closure_ad after delete on groups begin
    update group_closure_dirty set dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER group_items_closure_ai after insert on group_items begin
    update group_closure_dirty set dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER group_items_closure_au after update on group_items begin
    update group_closure_dirty set dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER group_items_closure_ad after delete on group_items begin
    update group_closure_dirty set dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER videos_root_values_ai after insert on videos begin
    delete from group_root_values where root_id in (select root_id from group_closure where content_key = new.content_key
        or content_key in (select b.user_key from video_bundles b join video_bundle_members m on m.bundle_id = b.id where m.content_key = new.content_key));
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER videos_root_values_au after update on videos begin
    delete from group_root_values where root_id in (select root_id from group_closure where content_key in (old.content_key, new.content_key)
        or content_key in (select b.user_key from video_bundles b join video_bundle_members m on m.bundle_id = b.id where m.content_key in (old.content_key, new.content_key)));
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER videos_root_values_ad after delete on videos begin
    delete from group_root_values where root_id in (select root_id from group_closure where content_key = old.content_key
        or content_key in (select b.user_key from video_bundles b join video_bundle_members m on m.bundle_id = b.id where m.content_key = old.content_key));
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_locations_root_values_ai after insert on video_locations begin
    delete from group_root_values where root_id in (select c.root_id from group_closure c join videos v on v.id = new.video_id
        where c.content_key = v.content_key
        or c.content_key in (select b.user_key from video_bundles b join video_bundle_members m on m.bundle_id = b.id where m.content_key = v.content_key));
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_locations_root_values_au after update on video_locations begin
    delete from group_root_values where root_id in (select c.root_id from group_closure c join videos v on v.id in (old.video_id, new.video_id)
        where c.content_key = v.content_key
        or c.content_key in (select b.user_key from video_bundles b join video_bundle_members m on m.bundle_id = b.id where m.content_key = v.content_key));
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_locations_root_values_ad after delete on video_locations begin
    delete from group_root_values where root_id in (select c.root_id from group_closure c join videos v on v.id = old.video_id
        where c.content_key = v.content_key
        or c.content_key in (select b.user_key from video_bundles b join video_bundle_members m on m.bundle_id = b.id where m.content_key = v.content_key));
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER playback_progress_root_values_ai after insert on playback_progress begin
    delete from group_root_values where root_id in (select root_id from group_closure where content_key = new.content_key);
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER playback_progress_root_values_au after update on playback_progress begin
    delete from group_root_values where root_id in (select root_id from group_closure where content_key in (old.content_key, new.content_key));
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER playback_progress_root_values_ad after delete on playback_progress begin
    delete from group_root_values where root_id in (select root_id from group_closure where content_key = old.content_key);
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER media_folders_root_values_ai after insert on media_folders begin
    delete from group_root_values;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER media_folders_root_values_au after update on media_folders begin
    delete from group_root_values;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER media_folders_root_values_ad after delete on media_folders begin
    delete from group_root_values;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_bundle_members_root_values_ai after insert on video_bundle_members begin
    delete from group_root_values;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_bundle_members_root_values_au after update on video_bundle_members begin
    delete from group_root_values;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_bundle_members_root_values_ad after delete on video_bundle_members begin
    delete from group_root_values;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_bundles_root_values_au after update on video_bundles begin
    delete from group_root_values;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER public_videos_root_values_ai after insert on public_videos begin
    delete from group_root_values where audience = 0 and root_id in (select root_id from group_closure where content_key = new.content_key);
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER public_videos_root_values_ad after delete on public_videos begin
    delete from group_root_values where audience = 0 and root_id in (select root_id from group_closure where content_key = old.content_key);
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_locations_search_ai after insert on video_locations begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_locations_search_au after update on video_locations begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_locations_search_ad after delete on video_locations begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_tags_search_ai after insert on video_tags begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_tags_search_ad after delete on video_tags begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER tag_names_search_ai after insert on tag_names begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER tag_names_search_au after update on tag_names begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER tag_names_search_ad after delete on tag_names begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_work_info_search_ai after insert on video_work_info begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_work_info_search_au after update on video_work_info begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_work_info_search_ad after delete on video_work_info begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER people_names_search_ai after insert on people_names begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER people_names_search_au after update on people_names begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER people_names_search_ad after delete on people_names begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_work_people_search_ai after insert on video_work_people begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_work_people_search_ad after delete on video_work_people begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER organization_names_search_ai after insert on organization_names begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER organization_names_search_au after update on organization_names begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER organization_names_search_ad after delete on organization_names begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_work_organizations_search_ai after insert on video_work_organizations begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_work_organizations_search_ad after delete on video_work_organizations begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_folder_names_search_ai after insert on video_folder_names begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_folder_names_search_ad after delete on video_folder_names begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_bundle_members_search_ai after insert on video_bundle_members begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_bundle_members_search_ad after delete on video_bundle_members begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_bundles_search_au after update on video_bundles begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER media_folders_search_ai after insert on media_folders begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER media_folders_search_ad after delete on media_folders begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER smart_folders_search_ai after insert on smart_folders begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER smart_folders_search_au after update on smart_folders begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER smart_folders_search_ad after delete on smart_folders begin
    update "home_index_state" set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER videos_watch_ai after insert on videos begin
    insert into video_watch_dirty(content_key) select new.content_key where new.content_key <> '';
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER videos_watch_au after update of content_key on videos begin
    insert into video_watch_dirty(content_key) select new.content_key where new.content_key <> '';
    insert into video_watch_dirty(content_key) select old.content_key where old.content_key <> '' and old.content_key <> new.content_key;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER videos_watch_ad after delete on videos begin
    insert into video_watch_dirty(content_key) select old.content_key where old.content_key <> '';
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER playback_progress_watch_ai after insert on playback_progress begin
    insert into video_watch_dirty(content_key) values (new.content_key);
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER playback_progress_watch_au after update on playback_progress begin
    insert into video_watch_dirty(content_key) values (new.content_key);
    insert into video_watch_dirty(content_key) select old.content_key where old.content_key <> new.content_key;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER playback_progress_watch_ad after delete on playback_progress begin
    insert into video_watch_dirty(content_key) values (old.content_key);
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_bundle_members_watch_ai after insert on video_bundle_members begin
    update home_index_state set watch_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_bundle_members_watch_au after update on video_bundle_members begin
    update home_index_state set watch_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_bundle_members_watch_ad after delete on video_bundle_members begin
    update home_index_state set watch_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER video_bundles_watch_au after update of user_key on video_bundles begin
    update home_index_state set watch_dirty = 1;
end;
-- +goose StatementEnd
insert into home_index_state (id, watch_dirty, search_dirty) values (1, 1, 1);
insert into group_closure_dirty (id, dirty) values (1, 1);
