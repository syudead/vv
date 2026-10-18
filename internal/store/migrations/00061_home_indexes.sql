-- +goose Up

drop trigger videos_shelved_ai;
drop trigger videos_shelved_au;
drop trigger videos_shelved_ad;
drop trigger video_locations_shelved_ai;
drop trigger video_locations_shelved_au;
drop trigger video_locations_shelved_ad;
drop trigger video_tags_shelved_ai;
drop trigger video_tags_shelved_ad;
drop trigger playback_progress_shelved_ai;
drop trigger playback_progress_shelved_au;
drop trigger playback_progress_shelved_ad;
drop trigger video_favorites_shelved_ai;
drop trigger video_favorites_shelved_ad;
drop trigger video_work_people_shelved_ai;
drop trigger video_work_people_shelved_ad;
drop trigger video_work_organizations_shelved_ai;
drop trigger video_work_organizations_shelved_ad;
drop trigger smart_folders_shelved_ai;
drop trigger smart_folders_shelved_au;
drop trigger smart_folders_shelved_ad;
drop trigger tags_shelved_ad;
drop trigger tags_shelved_au;
drop trigger tag_names_shelved_ai;
drop trigger tag_names_shelved_ad;
drop trigger video_folder_names_shelved_ai;
drop trigger video_folder_names_shelved_ad;
drop trigger video_bundle_members_shelved_ai;
drop trigger video_bundle_members_shelved_ad;
drop trigger media_folders_shelved_ai;
drop trigger media_folders_shelved_ad;

drop table home_shelved_videos;
drop index home_shelved_dirty_key_idx;
alter table home_shelved_dirty rename to video_watch_dirty;
create index video_watch_dirty_key_idx on video_watch_dirty(content_key);
alter table home_shelved_state rename to home_index_state;
alter table home_index_state rename column all_dirty to watch_dirty;
update home_index_state set watch_dirty = 1, search_dirty = 1;

-- +goose StatementBegin
create trigger videos_watch_ai after insert on videos begin
    insert into video_watch_dirty(content_key) select new.content_key where new.content_key <> '';
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger videos_watch_au after update of content_key on videos begin
    insert into video_watch_dirty(content_key) select new.content_key where new.content_key <> '';
    insert into video_watch_dirty(content_key) select old.content_key where old.content_key <> '' and old.content_key <> new.content_key;
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger videos_watch_ad after delete on videos begin
    insert into video_watch_dirty(content_key) select old.content_key where old.content_key <> '';
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger playback_progress_watch_ai after insert on playback_progress begin
    insert into video_watch_dirty(content_key) values (new.content_key);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger playback_progress_watch_au after update on playback_progress begin
    insert into video_watch_dirty(content_key) values (new.content_key);
    insert into video_watch_dirty(content_key) select old.content_key where old.content_key <> new.content_key;
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger playback_progress_watch_ad after delete on playback_progress begin
    insert into video_watch_dirty(content_key) values (old.content_key);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_bundle_members_watch_ai after insert on video_bundle_members begin
    update home_index_state set watch_dirty = 1;
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_bundle_members_watch_au after update on video_bundle_members begin
    update home_index_state set watch_dirty = 1;
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_bundle_members_watch_ad after delete on video_bundle_members begin
    update home_index_state set watch_dirty = 1;
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_bundles_watch_au after update of user_key on video_bundles begin
    update home_index_state set watch_dirty = 1;
end;
-- +goose StatementEnd

-- +goose Down

drop trigger videos_watch_ai;
drop trigger videos_watch_au;
drop trigger videos_watch_ad;
drop trigger playback_progress_watch_ai;
drop trigger playback_progress_watch_au;
drop trigger playback_progress_watch_ad;
drop trigger video_bundle_members_watch_ai;
drop trigger video_bundle_members_watch_au;
drop trigger video_bundle_members_watch_ad;
drop trigger video_bundles_watch_au;

drop index video_watch_dirty_key_idx;
alter table video_watch_dirty rename to home_shelved_dirty;
create index home_shelved_dirty_key_idx on home_shelved_dirty(content_key);
alter table home_index_state rename column watch_dirty to all_dirty;
alter table home_index_state rename to home_shelved_state;
create table home_shelved_videos (
    content_key text primary key
) without rowid;
update home_shelved_state set all_dirty = 1, search_dirty = 1;

-- +goose StatementBegin
create trigger videos_shelved_ai after insert on videos begin
    insert into home_shelved_dirty (content_key)
        select coalesce((select b.user_key from video_bundles b join video_bundle_members m on m.bundle_id = b.id where m.content_key = new.content_key), new.content_key)
        where new.content_key <> '';
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger videos_shelved_au after update on videos begin
    insert into home_shelved_dirty (content_key)
        select coalesce((select b.user_key from video_bundles b join video_bundle_members m on m.bundle_id = b.id where m.content_key = new.content_key), new.content_key)
        where new.content_key <> '';
    insert into home_shelved_dirty (content_key) select old.content_key where old.content_key <> '' and old.content_key <> new.content_key;
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger videos_shelved_ad after delete on videos begin
    delete from home_shelved_videos where content_key = old.content_key;
    delete from home_shelved_dirty where content_key = old.content_key;
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_locations_shelved_ai after insert on video_locations begin
    insert into home_shelved_dirty (content_key)
        select coalesce((select b.user_key from video_bundles b join video_bundle_members m on m.bundle_id = b.id where m.content_key = v.content_key), v.content_key)
        from videos v where v.id = new.video_id and v.content_key <> '';
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_locations_shelved_au after update on video_locations begin
    insert into home_shelved_dirty (content_key)
        select coalesce((select b.user_key from video_bundles b join video_bundle_members m on m.bundle_id = b.id where m.content_key = v.content_key), v.content_key)
        from videos v where v.id in (old.video_id, new.video_id) and v.content_key <> '';
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_locations_shelved_ad after delete on video_locations begin
    insert into home_shelved_dirty (content_key)
        select coalesce((select b.user_key from video_bundles b join video_bundle_members m on m.bundle_id = b.id where m.content_key = v.content_key), v.content_key)
        from videos v where v.id = old.video_id and v.content_key <> '';
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_tags_shelved_ai after insert on video_tags begin
    insert into home_shelved_dirty (content_key) values (new.content_key);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_tags_shelved_ad after delete on video_tags begin
    insert into home_shelved_dirty (content_key) values (old.content_key);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger playback_progress_shelved_ai after insert on playback_progress begin
    insert into home_shelved_dirty (content_key) values (new.content_key);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger playback_progress_shelved_au after update on playback_progress begin
    insert into home_shelved_dirty (content_key) values (new.content_key);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger playback_progress_shelved_ad after delete on playback_progress begin
    insert into home_shelved_dirty (content_key) values (old.content_key);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_favorites_shelved_ai after insert on video_favorites begin
    insert into home_shelved_dirty (content_key) values (new.content_key);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_favorites_shelved_ad after delete on video_favorites begin
    insert into home_shelved_dirty (content_key) values (old.content_key);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_work_people_shelved_ai after insert on video_work_people begin
    insert into home_shelved_dirty (content_key) values (new.content_key);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_work_people_shelved_ad after delete on video_work_people begin
    insert into home_shelved_dirty (content_key) values (old.content_key);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_work_organizations_shelved_ai after insert on video_work_organizations begin
    insert into home_shelved_dirty (content_key) values (new.content_key);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_work_organizations_shelved_ad after delete on video_work_organizations begin
    insert into home_shelved_dirty (content_key) values (old.content_key);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger smart_folders_shelved_ai after insert on smart_folders begin
    update home_shelved_state set all_dirty = 1;
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger smart_folders_shelved_au after update on smart_folders begin
    update home_shelved_state set all_dirty = 1;
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger smart_folders_shelved_ad after delete on smart_folders begin
    update home_shelved_state set all_dirty = 1;
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger tags_shelved_ad after delete on tags begin
    update home_shelved_state set all_dirty = 1;
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger tags_shelved_au after update on tags begin
    update home_shelved_state set all_dirty = 1;
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger tag_names_shelved_ai after insert on tag_names begin
    update home_shelved_state set all_dirty = 1;
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger tag_names_shelved_ad after delete on tag_names begin
    update home_shelved_state set all_dirty = 1;
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_folder_names_shelved_ai after insert on video_folder_names begin
    update home_shelved_state set all_dirty = 1;
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_folder_names_shelved_ad after delete on video_folder_names begin
    update home_shelved_state set all_dirty = 1;
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_bundle_members_shelved_ai after insert on video_bundle_members begin
    update home_shelved_state set all_dirty = 1;
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger video_bundle_members_shelved_ad after delete on video_bundle_members begin
    update home_shelved_state set all_dirty = 1;
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger media_folders_shelved_ai after insert on media_folders begin
    update home_shelved_state set all_dirty = 1;
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger media_folders_shelved_ad after delete on media_folders begin
    update home_shelved_state set all_dirty = 1;
end;
-- +goose StatementEnd
