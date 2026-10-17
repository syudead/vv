-- +goose Up
-- ホームの棚のカードと視聴状態の表（specs/057-home/data-model.md「Group closure」、research.md R-12）。
--
-- group_root_values に、カードが開くメンバー（open_member。GroupOpenIndex と同じ決め方）と、フォルダの絵柄に
-- 差し込むサムネイル生成済みのメンバー（previews。再生順の先頭 4 件の動画の id の JSON の配列）と、視聴状態
-- （watch_state。groupWatchStateExpr と同じ決め方）を足す。棚のグループの項目はメンバーを読まずにこの行から
-- 作り、視聴状態の棚は索引で視聴状態の根を引く。行は確定後の維持（maintainHomeIndexes）が埋め直すので、
-- 列の無い古い行は消す。
alter table group_root_values add column open_member integer;
alter table group_root_values add column previews text not null default '[]';
alter table group_root_values add column watch_state text generated always as (
    case when started = 0 then 'unwatched' when watched = members then 'watched' else 'inProgress' end) virtual;
create index group_root_values_watch_idx on group_root_values (audience, watch_state);
delete from group_root_values;

-- video_watch_states は動画（利用者データの鍵。集まりの動画は集まりの鍵）ごとの視聴状態（ClassifyWatch と
-- 同じ決め方。再生の記録の無い動画は unwatched）である。視聴状態の棚は、グループに入らない動画をここから
-- 索引で引く。再生の記録と動画の書き込みは home_shelved_dirty の印を付け（00057 の引き金）、確定後の維持が
-- 印の付いた鍵の行を求め直す。all_dirty を立てて、最初の維持で全動画の行を作る。
create table video_watch_states (
    content_key text primary key,
    state       text not null check (state in ('unwatched', 'inProgress', 'watched'))
) without rowid;
create index video_watch_states_state_idx on video_watch_states (state, content_key);
update home_shelved_state set all_dirty = 1;

-- home_search_hits は、ホームのフォルダの棚の検索式の段（識別子 search は条件句と引数）に当たる動画の id
-- である（所有者の範囲。ゲストには読み出しが見せる動画の条件を重ねる）。棚の読み出しと棚への一致の求め直し
-- は全文索引を引かずにここを引く。検索に関わる表（所在、タグとその名前、作品情報・人物・団体とその名前、
-- フォルダ名のタグ、集まり、登録フォルダ、棚の規則）への書き込みが search_dirty を立て、確定後の維持が
-- 全部を求め直す。再生の記録や解析の確定は検索に関わらないので、求め直さない。
alter table home_shelved_state add column search_dirty integer not null default 1;
create table home_search_hits (
    search   text not null,
    video_id integer not null,
    primary key (search, video_id)
) without rowid;
-- +goose StatementBegin
create trigger video_locations_search_ai after insert on video_locations begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_locations_search_au after update on video_locations begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_locations_search_ad after delete on video_locations begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_tags_search_ai after insert on video_tags begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_tags_search_ad after delete on video_tags begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger tag_names_search_ai after insert on tag_names begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger tag_names_search_au after update on tag_names begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger tag_names_search_ad after delete on tag_names begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_work_info_search_ai after insert on video_work_info begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_work_info_search_au after update on video_work_info begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_work_info_search_ad after delete on video_work_info begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger people_names_search_ai after insert on people_names begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger people_names_search_au after update on people_names begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger people_names_search_ad after delete on people_names begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_work_people_search_ai after insert on video_work_people begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_work_people_search_ad after delete on video_work_people begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger organization_names_search_ai after insert on organization_names begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger organization_names_search_au after update on organization_names begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger organization_names_search_ad after delete on organization_names begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_work_organizations_search_ai after insert on video_work_organizations begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_work_organizations_search_ad after delete on video_work_organizations begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_folder_names_search_ai after insert on video_folder_names begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_folder_names_search_ad after delete on video_folder_names begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_bundle_members_search_ai after insert on video_bundle_members begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_bundle_members_search_ad after delete on video_bundle_members begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_bundles_search_au after update on video_bundles begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger media_folders_search_ai after insert on media_folders begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger media_folders_search_ad after delete on media_folders begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger smart_folders_search_ai after insert on smart_folders begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger smart_folders_search_au after update on smart_folders begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger smart_folders_search_ad after delete on smart_folders begin
    update home_shelved_state set search_dirty = 1;
end;
-- +goose StatementEnd

-- +goose Down
drop table if exists home_search_hits;
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
alter table home_shelved_state drop column search_dirty;
drop table if exists video_watch_states;
drop index if exists group_root_values_watch_idx;
alter table group_root_values drop column watch_state;
alter table group_root_values drop column previews;
alter table group_root_values drop column open_member;
