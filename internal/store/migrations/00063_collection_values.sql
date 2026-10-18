-- +goose Up
-- Collection values belong to an audience because private videos never affect guest cards.
create table group_members (
    group_id integer not null references groups(id) on delete cascade,
    audience integer not null check (audience in (0, 1)),
    video_id integer not null references videos(id) on delete cascade,
    position text not null,
    primary key (group_id, audience, video_id)
) without rowid;
create index group_members_video_idx on group_members(video_id, group_id);
create index group_members_order_idx on group_members(group_id, audience, position);
create table group_values (
    group_id integer not null references groups(id) on delete cascade,
    audience integer not null check (audience in (0, 1)),
    video_count integer not null,
    added_at integer, mtime integer, created_at integer, played_at integer,
    duration_ms integer, size_bytes integer,
    watched integer not null, started integer not null, playable integer not null,
    open_video_id integer,
    primary key (group_id, audience)
) without rowid;
create table smart_folder_values (
    folder_id integer not null references smart_folders(id) on delete cascade,
    audience integer not null check (audience in (0, 1)),
    visible integer not null,
    added_at integer, mtime integer, created_at integer,
    duration_ms integer, size_bytes integer, played_at integer,
    expires_at integer,
    primary key (folder_id, audience)
) without rowid;
create index smart_folder_values_expiry_idx on smart_folder_values(expires_at) where expires_at is not null;
-- Duplicate marks are intentional: an outer INSERT conflict policy also applies inside triggers.
create table collection_changes (kind text not null, id integer not null);
create index collection_changes_kind_idx on collection_changes(kind, id);
insert into collection_changes values ('group', 0), ('smart', 0);

-- +goose StatementBegin
create trigger groups_collection_i after insert on groups begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger groups_collection_u after update on groups begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger groups_collection_d after delete on groups begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger group_items_collection_i after insert on group_items begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger group_items_collection_u after update on group_items begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger group_items_collection_d after delete on group_items begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger videos_collection_i after insert on videos begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger videos_collection_u after update of content_key, added_at, duration_ms, playable, probe_state on videos begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger videos_collection_d after delete on videos begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_locations_collection_i after insert on video_locations begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_locations_collection_u after update of video_id, path, size_bytes, mtime, file_created_at, title_key, search_key on video_locations begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_locations_collection_d after delete on video_locations begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger media_folders_collection_i after insert on media_folders begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger media_folders_collection_u after update on media_folders begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger media_folders_collection_d after delete on media_folders begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger public_videos_collection_i after insert on public_videos begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger public_videos_collection_u after update on public_videos begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger public_videos_collection_d after delete on public_videos begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_bundles_collection_i after insert on video_bundles begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_bundles_collection_u after update on video_bundles begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_bundles_collection_d after delete on video_bundles begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_bundle_members_collection_i after insert on video_bundle_members begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_bundle_members_collection_u after update on video_bundle_members begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_bundle_members_collection_d after delete on video_bundle_members begin
    insert into collection_changes values ('group', 0), ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger smart_folders_collection_i after insert on smart_folders begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger smart_folders_collection_u after update on smart_folders begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger smart_folders_collection_d after delete on smart_folders begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger smart_folder_favorites_collection_i after insert on smart_folder_favorites begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger smart_folder_favorites_collection_u after update on smart_folder_favorites begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger smart_folder_favorites_collection_d after delete on smart_folder_favorites begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger group_favorites_collection_i after insert on group_favorites begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger group_favorites_collection_u after update on group_favorites begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger group_favorites_collection_d after delete on group_favorites begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_favorites_collection_i after insert on video_favorites begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_favorites_collection_u after update on video_favorites begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_favorites_collection_d after delete on video_favorites begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger tags_collection_i after insert on tags begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger tags_collection_u after update on tags begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger tags_collection_d after delete on tags begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger tag_names_collection_i after insert on tag_names begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger tag_names_collection_u after update on tag_names begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger tag_names_collection_d after delete on tag_names begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_tags_collection_i after insert on video_tags begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_tags_collection_u after update on video_tags begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_tags_collection_d after delete on video_tags begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_folder_names_collection_i after insert on video_folder_names begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_folder_names_collection_u after update on video_folder_names begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_folder_names_collection_d after delete on video_folder_names begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_work_info_collection_i after insert on video_work_info begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_work_info_collection_u after update on video_work_info begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_work_info_collection_d after delete on video_work_info begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger people_collection_i after insert on people begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger people_collection_u after update on people begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger people_collection_d after delete on people begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger people_names_collection_i after insert on people_names begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger people_names_collection_u after update on people_names begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger people_names_collection_d after delete on people_names begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger organizations_collection_i after insert on organizations begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger organizations_collection_u after update on organizations begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger organizations_collection_d after delete on organizations begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger organization_names_collection_i after insert on organization_names begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger organization_names_collection_u after update on organization_names begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger organization_names_collection_d after delete on organization_names begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_work_people_collection_i after insert on video_work_people begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_work_people_collection_u after update on video_work_people begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_work_people_collection_d after delete on video_work_people begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_work_organizations_collection_i after insert on video_work_organizations begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_work_organizations_collection_u after update on video_work_organizations begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_work_organizations_collection_d after delete on video_work_organizations begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger tag_kinds_collection_i after insert on tag_kinds begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger tag_kinds_collection_u after update on tag_kinds begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger tag_kinds_collection_d after delete on tag_kinds begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger playback_progress_collection_i after insert on playback_progress begin
    insert into collection_changes (kind, id)
    select 'group', m.group_id from group_members m join videos v on v.id = m.video_id
    where v.content_key in (new.content_key) or exists (
        select 1 from video_bundle_members bm join video_bundles b on b.id = bm.bundle_id
        where bm.content_key = v.content_key and b.user_key in (new.content_key));
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger playback_progress_collection_u after update on playback_progress begin
    insert into collection_changes (kind, id)
    select 'group', m.group_id from group_members m join videos v on v.id = m.video_id
    where v.content_key in (old.content_key, new.content_key) or exists (
        select 1 from video_bundle_members bm join video_bundles b on b.id = bm.bundle_id
        where bm.content_key = v.content_key and b.user_key in (old.content_key, new.content_key));
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger playback_progress_collection_d after delete on playback_progress begin
    insert into collection_changes (kind, id)
    select 'group', m.group_id from group_members m join videos v on v.id = m.video_id
    where v.content_key in (old.content_key) or exists (
        select 1 from video_bundle_members bm join video_bundles b on b.id = bm.bundle_id
        where bm.content_key = v.content_key and b.user_key in (old.content_key));
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose Down
drop trigger if exists playback_progress_collection_d;
drop trigger if exists playback_progress_collection_u;
drop trigger if exists playback_progress_collection_i;
drop trigger if exists tag_kinds_collection_d;
drop trigger if exists tag_kinds_collection_u;
drop trigger if exists tag_kinds_collection_i;
drop trigger if exists video_work_organizations_collection_d;
drop trigger if exists video_work_organizations_collection_u;
drop trigger if exists video_work_organizations_collection_i;
drop trigger if exists video_work_people_collection_d;
drop trigger if exists video_work_people_collection_u;
drop trigger if exists video_work_people_collection_i;
drop trigger if exists organization_names_collection_d;
drop trigger if exists organization_names_collection_u;
drop trigger if exists organization_names_collection_i;
drop trigger if exists organizations_collection_d;
drop trigger if exists organizations_collection_u;
drop trigger if exists organizations_collection_i;
drop trigger if exists people_names_collection_d;
drop trigger if exists people_names_collection_u;
drop trigger if exists people_names_collection_i;
drop trigger if exists people_collection_d;
drop trigger if exists people_collection_u;
drop trigger if exists people_collection_i;
drop trigger if exists video_work_info_collection_d;
drop trigger if exists video_work_info_collection_u;
drop trigger if exists video_work_info_collection_i;
drop trigger if exists video_folder_names_collection_d;
drop trigger if exists video_folder_names_collection_u;
drop trigger if exists video_folder_names_collection_i;
drop trigger if exists video_tags_collection_d;
drop trigger if exists video_tags_collection_u;
drop trigger if exists video_tags_collection_i;
drop trigger if exists tag_names_collection_d;
drop trigger if exists tag_names_collection_u;
drop trigger if exists tag_names_collection_i;
drop trigger if exists tags_collection_d;
drop trigger if exists tags_collection_u;
drop trigger if exists tags_collection_i;
drop trigger if exists video_favorites_collection_d;
drop trigger if exists video_favorites_collection_u;
drop trigger if exists video_favorites_collection_i;
drop trigger if exists group_favorites_collection_d;
drop trigger if exists group_favorites_collection_u;
drop trigger if exists group_favorites_collection_i;
drop trigger if exists smart_folder_favorites_collection_d;
drop trigger if exists smart_folder_favorites_collection_u;
drop trigger if exists smart_folder_favorites_collection_i;
drop trigger if exists smart_folders_collection_d;
drop trigger if exists smart_folders_collection_u;
drop trigger if exists smart_folders_collection_i;
drop trigger if exists video_bundle_members_collection_d;
drop trigger if exists video_bundle_members_collection_u;
drop trigger if exists video_bundle_members_collection_i;
drop trigger if exists video_bundles_collection_d;
drop trigger if exists video_bundles_collection_u;
drop trigger if exists video_bundles_collection_i;
drop trigger if exists public_videos_collection_d;
drop trigger if exists public_videos_collection_u;
drop trigger if exists public_videos_collection_i;
drop trigger if exists media_folders_collection_d;
drop trigger if exists media_folders_collection_u;
drop trigger if exists media_folders_collection_i;
drop trigger if exists video_locations_collection_d;
drop trigger if exists video_locations_collection_u;
drop trigger if exists video_locations_collection_i;
drop trigger if exists videos_collection_d;
drop trigger if exists videos_collection_u;
drop trigger if exists videos_collection_i;
drop trigger if exists group_items_collection_d;
drop trigger if exists group_items_collection_u;
drop trigger if exists group_items_collection_i;
drop trigger if exists groups_collection_d;
drop trigger if exists groups_collection_u;
drop trigger if exists groups_collection_i;
drop table collection_changes;
drop table smart_folder_values;
drop table group_values;
drop table group_members;
