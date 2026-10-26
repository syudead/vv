-- +goose Up
-- Smart folders keep no stored values: the library lists them by name ahead of its items, and guests do not see them.
-- Group values are refreshed only for the groups a change can affect; refreshes add their ancestors.
drop trigger groups_collection_i;
drop trigger groups_collection_u;
drop trigger groups_collection_d;
drop trigger group_items_collection_i;
drop trigger group_items_collection_u;
drop trigger group_items_collection_d;
drop trigger videos_collection_i;
drop trigger videos_collection_u;
drop trigger videos_collection_d;
drop trigger video_locations_collection_i;
drop trigger video_locations_collection_u;
drop trigger video_locations_collection_d;
drop trigger media_folders_collection_i;
drop trigger media_folders_collection_u;
drop trigger media_folders_collection_d;
drop trigger public_videos_collection_i;
drop trigger public_videos_collection_u;
drop trigger public_videos_collection_d;
drop trigger video_bundles_collection_i;
drop trigger video_bundles_collection_u;
drop trigger video_bundles_collection_d;
drop trigger video_bundle_members_collection_i;
drop trigger video_bundle_members_collection_u;
drop trigger video_bundle_members_collection_d;
drop trigger smart_folders_collection_i;
drop trigger smart_folders_collection_u;
drop trigger smart_folders_collection_d;
drop trigger smart_folder_favorites_collection_i;
drop trigger smart_folder_favorites_collection_u;
drop trigger smart_folder_favorites_collection_d;
drop trigger group_favorites_collection_i;
drop trigger group_favorites_collection_u;
drop trigger group_favorites_collection_d;
drop trigger video_favorites_collection_i;
drop trigger video_favorites_collection_u;
drop trigger video_favorites_collection_d;
drop trigger tags_collection_i;
drop trigger tags_collection_u;
drop trigger tags_collection_d;
drop trigger tag_names_collection_i;
drop trigger tag_names_collection_u;
drop trigger tag_names_collection_d;
drop trigger video_tags_collection_i;
drop trigger video_tags_collection_u;
drop trigger video_tags_collection_d;
drop trigger video_folder_names_collection_i;
drop trigger video_folder_names_collection_u;
drop trigger video_folder_names_collection_d;
drop trigger video_work_info_collection_i;
drop trigger video_work_info_collection_u;
drop trigger video_work_info_collection_d;
drop trigger people_collection_i;
drop trigger people_collection_u;
drop trigger people_collection_d;
drop trigger people_names_collection_i;
drop trigger people_names_collection_u;
drop trigger people_names_collection_d;
drop trigger organizations_collection_i;
drop trigger organizations_collection_u;
drop trigger organizations_collection_d;
drop trigger organization_names_collection_i;
drop trigger organization_names_collection_u;
drop trigger organization_names_collection_d;
drop trigger video_work_people_collection_i;
drop trigger video_work_people_collection_u;
drop trigger video_work_people_collection_d;
drop trigger video_work_organizations_collection_i;
drop trigger video_work_organizations_collection_u;
drop trigger video_work_organizations_collection_d;
drop trigger tag_kinds_collection_i;
drop trigger tag_kinds_collection_u;
drop trigger tag_kinds_collection_d;
drop trigger playback_progress_collection_i;
drop trigger playback_progress_collection_u;
drop trigger playback_progress_collection_d;
drop table smart_folder_values;
delete from collection_changes where kind = 'smart';

-- +goose StatementBegin
create trigger groups_collection_i after insert on groups begin
    insert into collection_changes values ('group', new.id);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger group_items_collection_i after insert on group_items begin
    insert into collection_changes values ('group', new.group_id);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger media_folders_collection_i after insert on media_folders begin
    insert into collection_changes values ('group', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger public_videos_collection_i after insert on public_videos begin
    insert into collection_changes (kind, id)
    select distinct 'group', gi.group_id from group_items gi where gi.content_key in (new.content_key);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger playback_progress_collection_i after insert on playback_progress begin
    insert into collection_changes (kind, id)
    select distinct 'group', gi.group_id from group_items gi where gi.content_key in (new.content_key);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger videos_collection_i after insert on videos begin
    insert into collection_changes (kind, id)
    select distinct 'group', gi.group_id from group_items gi
    where gi.content_key in (new.content_key) or gi.content_key in (
        select b.user_key from video_bundle_members bm join video_bundles b on b.id = bm.bundle_id
        where bm.content_key in (new.content_key));
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_locations_collection_i after insert on video_locations begin
    insert into collection_changes (kind, id)
    select distinct 'group', gi.group_id from group_items gi
    where gi.content_key in (select content_key from videos where id = new.video_id) or gi.content_key in (
        select b.user_key from video_bundle_members bm join video_bundles b on b.id = bm.bundle_id
        where bm.content_key in (select content_key from videos where id = new.video_id));
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_bundles_collection_i after insert on video_bundles begin
    insert into collection_changes (kind, id)
    select distinct 'group', gi.group_id from group_items gi
    where gi.content_key in (new.user_key) or gi.content_key in (
        select content_key from video_bundle_members where bundle_id in (new.id));
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_bundle_members_collection_i after insert on video_bundle_members begin
    insert into collection_changes (kind, id)
    select distinct 'group', gi.group_id from group_items gi
    where gi.content_key in (new.content_key) or gi.content_key in (
        select user_key from video_bundles where id in (new.bundle_id));
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger groups_collection_u after update on groups begin
    insert into collection_changes values ('group', new.id);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger group_items_collection_u after update on group_items begin
    insert into collection_changes values ('group', new.group_id);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger media_folders_collection_u after update on media_folders begin
    insert into collection_changes values ('group', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger public_videos_collection_u after update on public_videos begin
    insert into collection_changes (kind, id)
    select distinct 'group', gi.group_id from group_items gi where gi.content_key in (old.content_key, new.content_key);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger playback_progress_collection_u after update on playback_progress begin
    insert into collection_changes (kind, id)
    select distinct 'group', gi.group_id from group_items gi where gi.content_key in (old.content_key, new.content_key);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger videos_collection_u after update of content_key, added_at, duration_ms, playable, probe_state on videos begin
    insert into collection_changes (kind, id)
    select distinct 'group', gi.group_id from group_items gi
    where gi.content_key in (old.content_key, new.content_key) or gi.content_key in (
        select b.user_key from video_bundle_members bm join video_bundles b on b.id = bm.bundle_id
        where bm.content_key in (old.content_key, new.content_key));
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_locations_collection_u after update of video_id, path, size_bytes, mtime, file_created_at, title_key, search_key on video_locations begin
    insert into collection_changes (kind, id)
    select distinct 'group', gi.group_id from group_items gi
    where gi.content_key in (select content_key from videos where id = old.video_id union select content_key from videos where id = new.video_id) or gi.content_key in (
        select b.user_key from video_bundle_members bm join video_bundles b on b.id = bm.bundle_id
        where bm.content_key in (select content_key from videos where id = old.video_id union select content_key from videos where id = new.video_id));
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_bundles_collection_u after update on video_bundles begin
    insert into collection_changes (kind, id)
    select distinct 'group', gi.group_id from group_items gi
    where gi.content_key in (old.user_key, new.user_key) or gi.content_key in (
        select content_key from video_bundle_members where bundle_id in (old.id, new.id));
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_bundle_members_collection_u after update on video_bundle_members begin
    insert into collection_changes (kind, id)
    select distinct 'group', gi.group_id from group_items gi
    where gi.content_key in (old.content_key, new.content_key) or gi.content_key in (
        select user_key from video_bundles where id in (old.bundle_id, new.bundle_id));
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger groups_collection_d after delete on groups begin
    insert into collection_changes values ('group', old.id);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger group_items_collection_d after delete on group_items begin
    insert into collection_changes values ('group', old.group_id);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger media_folders_collection_d after delete on media_folders begin
    insert into collection_changes values ('group', 0);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger public_videos_collection_d after delete on public_videos begin
    insert into collection_changes (kind, id)
    select distinct 'group', gi.group_id from group_items gi where gi.content_key in (old.content_key);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger playback_progress_collection_d after delete on playback_progress begin
    insert into collection_changes (kind, id)
    select distinct 'group', gi.group_id from group_items gi where gi.content_key in (old.content_key);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger videos_collection_d after delete on videos begin
    insert into collection_changes (kind, id)
    select distinct 'group', gi.group_id from group_items gi
    where gi.content_key in (old.content_key) or gi.content_key in (
        select b.user_key from video_bundle_members bm join video_bundles b on b.id = bm.bundle_id
        where bm.content_key in (old.content_key));
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_locations_collection_d after delete on video_locations begin
    insert into collection_changes (kind, id)
    select distinct 'group', gi.group_id from group_items gi
    where gi.content_key in (select content_key from videos where id = old.video_id) or gi.content_key in (
        select b.user_key from video_bundle_members bm join video_bundles b on b.id = bm.bundle_id
        where bm.content_key in (select content_key from videos where id = old.video_id));
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_bundles_collection_d after delete on video_bundles begin
    insert into collection_changes (kind, id)
    select distinct 'group', gi.group_id from group_items gi
    where gi.content_key in (old.user_key) or gi.content_key in (
        select content_key from video_bundle_members where bundle_id in (old.id));
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_bundle_members_collection_d after delete on video_bundle_members begin
    insert into collection_changes (kind, id)
    select distinct 'group', gi.group_id from group_items gi
    where gi.content_key in (old.content_key) or gi.content_key in (
        select user_key from video_bundles where id in (old.bundle_id));
end;
-- +goose StatementEnd

insert into collection_changes values ('group', 0);

-- +goose Down
drop trigger groups_collection_i;
drop trigger group_items_collection_i;
drop trigger media_folders_collection_i;
drop trigger public_videos_collection_i;
drop trigger playback_progress_collection_i;
drop trigger videos_collection_i;
drop trigger video_locations_collection_i;
drop trigger video_bundles_collection_i;
drop trigger video_bundle_members_collection_i;
drop trigger groups_collection_u;
drop trigger group_items_collection_u;
drop trigger media_folders_collection_u;
drop trigger public_videos_collection_u;
drop trigger playback_progress_collection_u;
drop trigger videos_collection_u;
drop trigger video_locations_collection_u;
drop trigger video_bundles_collection_u;
drop trigger video_bundle_members_collection_u;
drop trigger groups_collection_d;
drop trigger group_items_collection_d;
drop trigger media_folders_collection_d;
drop trigger public_videos_collection_d;
drop trigger playback_progress_collection_d;
drop trigger videos_collection_d;
drop trigger video_locations_collection_d;
drop trigger video_bundles_collection_d;
drop trigger video_bundle_members_collection_d;
create table smart_folder_values (
    folder_id integer not null references smart_folders(id) on delete cascade,
    audience integer not null check (audience in (0, 1)),
    visible integer not null,
    added_at integer, mtime integer, created_at integer,
    duration_ms integer, size_bytes integer, played_at integer,
    primary key (folder_id, audience)
) without rowid;
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
create trigger people_collection_u after update of id, created_at on people begin
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

insert into collection_changes values ('group', 0), ('smart', 0);
