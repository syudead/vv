-- +goose Up
-- グループの構造の表（specs/057-home/data-model.md「Group closure」、research.md R-12）。ホームの棚の
-- 読み出しは、なぞった動画がどの根（いちばん上のグループ）に入るか、その根の子孫はどれかを、読み出しの
-- たびに再帰で求めずにここから引く。
--
-- group_closure はグループごとの子孫の動画の項目（内容の鍵。集まりの動画は集まりの利用者データの鍵）と、
-- 再生順の鍵（深さ優先で最初に現れる道の位置を 10 桁ずつつないだもの。groupDescendantsCTE と同じ）である。
-- グループの構造を変える書き込みが同じ取引で作り直す（rebuildGroupClosure）。構造の変化を取りこぼさない
-- よう、groups と group_items への書き込みは group_closure_dirty に印を付け、次の棚の読み出しが印を見れば
-- 作り直してから読む。
create table group_closure (
    root_id     integer not null references groups (id) on delete cascade,
    content_key text    not null,
    sort_key    text    not null,
    primary key (root_id, content_key)
) without rowid;
create index group_closure_key_idx on group_closure (content_key, root_id);

create table group_closure_dirty (
    id    integer primary key check (id = 1),
    dirty integer not null
);
insert into group_closure_dirty (id, dirty) values (1, 0);

-- group_root_values はグループごと・見る人ごと（audience: 1 が所有者、0 がゲスト）のグループの項目の
-- 値を、その見る人に見せる子孫から集めたものである。members は見せる子孫の本数、*_member は各並べ替えの
-- 値が最大（同じなら id が最大）の子孫の動画の id、watched と started は見終えた本数と始めた本数（視聴
-- 状態の元）、playable は再生できる子孫の有無である。値を変える書き込み（動画・所在・再生の記録・公開・
-- 集まり・登録フォルダ）は引き金で根の行を消し、書き込みの確定のあとの維持（maintainHomeIndexes）が
-- 欠けた根の行を埋め直す。読み出しは書かず、まだ欠けている根だけその場で集める。
create table group_root_values (
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
    played_member  integer,
    primary key (root_id, audience)
) without rowid;

-- home_shelved_videos は、ホームのどのフォルダの棚の実効の規則にも動画として一致する動画の利用者データの
-- 鍵である（research.md R-11）。棚にない動画の棚は、この表に無い見せる動画を読む。規則に関わる書き込み
-- （タグ・動画・所在・再生の記録・お気に入り・作品情報）は引き金で動画の鍵を home_shelved_dirty に入れ、
-- 棚の規則や並びの書き込み（smart_folders、tags）は home_shelved_state の all_dirty を立てる。書き込みの
-- 確定のあとの維持（maintainHomeIndexes）が印の付いた動画（all_dirty ならすべて）を求め直す。
create table home_shelved_videos (
    content_key text primary key
) without rowid;
-- 印の表は重複を許す（引き金の中の insert or ignore は、引き金を起こした文の衝突の扱いに上書きされる）。
create table home_shelved_dirty (
    content_key text not null
);
create index home_shelved_dirty_key_idx on home_shelved_dirty (content_key);
create table home_shelved_state (
    id        integer primary key check (id = 1),
    all_dirty integer not null
);
insert into home_shelved_state (id, all_dirty) values (1, 1);

-- 索引でなぞる並び順（specs/057-home/research.md R-12）: 所在の更新日時と作成日時、再生の記録の日時。
create index video_locations_mtime_idx on video_locations (mtime, video_id);
create index video_locations_created_idx on video_locations (coalesce(file_created_at, mtime), video_id);
create index playback_progress_updated_idx on playback_progress (updated_at, content_key);

-- +goose StatementBegin
create trigger groups_closure_ai after insert on groups begin
    update group_closure_dirty set dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger groups_closure_au after update on groups begin
    update group_closure_dirty set dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger groups_closure_ad after delete on groups begin
    update group_closure_dirty set dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger group_items_closure_ai after insert on group_items begin
    update group_closure_dirty set dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger group_items_closure_au after update on group_items begin
    update group_closure_dirty set dirty = 1;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger group_items_closure_ad after delete on group_items begin
    update group_closure_dirty set dirty = 1;
end;
-- +goose StatementEnd

-- 動画の行が変わればその動画の根の値を消す。鍵は内容の鍵と、集まりならその利用者データの鍵である。
-- +goose StatementBegin
create trigger videos_root_values_ai after insert on videos begin
    delete from group_root_values where root_id in (select root_id from group_closure where content_key = new.content_key
        or content_key in (select b.user_key from video_bundles b join video_bundle_members m on m.bundle_id = b.id where m.content_key = new.content_key));
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger videos_root_values_au after update on videos begin
    delete from group_root_values where root_id in (select root_id from group_closure where content_key in (old.content_key, new.content_key)
        or content_key in (select b.user_key from video_bundles b join video_bundle_members m on m.bundle_id = b.id where m.content_key in (old.content_key, new.content_key)));
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger videos_root_values_ad after delete on videos begin
    delete from group_root_values where root_id in (select root_id from group_closure where content_key = old.content_key
        or content_key in (select b.user_key from video_bundles b join video_bundle_members m on m.bundle_id = b.id where m.content_key = old.content_key));
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_locations_root_values_ai after insert on video_locations begin
    delete from group_root_values where root_id in (select c.root_id from group_closure c join videos v on v.id = new.video_id
        where c.content_key = v.content_key
        or c.content_key in (select b.user_key from video_bundles b join video_bundle_members m on m.bundle_id = b.id where m.content_key = v.content_key));
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_locations_root_values_au after update on video_locations begin
    delete from group_root_values where root_id in (select c.root_id from group_closure c join videos v on v.id in (old.video_id, new.video_id)
        where c.content_key = v.content_key
        or c.content_key in (select b.user_key from video_bundles b join video_bundle_members m on m.bundle_id = b.id where m.content_key = v.content_key));
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_locations_root_values_ad after delete on video_locations begin
    delete from group_root_values where root_id in (select c.root_id from group_closure c join videos v on v.id = old.video_id
        where c.content_key = v.content_key
        or c.content_key in (select b.user_key from video_bundles b join video_bundle_members m on m.bundle_id = b.id where m.content_key = v.content_key));
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger playback_progress_root_values_ai after insert on playback_progress begin
    delete from group_root_values where root_id in (select root_id from group_closure where content_key = new.content_key);
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger playback_progress_root_values_au after update on playback_progress begin
    delete from group_root_values where root_id in (select root_id from group_closure where content_key in (old.content_key, new.content_key));
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger playback_progress_root_values_ad after delete on playback_progress begin
    delete from group_root_values where root_id in (select root_id from group_closure where content_key = old.content_key);
end;
-- +goose StatementEnd
-- 登録フォルダと集まりの変化は見せる子孫を広く変えるので、すべての根の値を消す。
-- +goose StatementBegin
create trigger media_folders_root_values_ai after insert on media_folders begin
    delete from group_root_values;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger media_folders_root_values_au after update on media_folders begin
    delete from group_root_values;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger media_folders_root_values_ad after delete on media_folders begin
    delete from group_root_values;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_bundle_members_root_values_ai after insert on video_bundle_members begin
    delete from group_root_values;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_bundle_members_root_values_au after update on video_bundle_members begin
    delete from group_root_values;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_bundle_members_root_values_ad after delete on video_bundle_members begin
    delete from group_root_values;
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_bundles_root_values_au after update on video_bundles begin
    delete from group_root_values;
end;
-- +goose StatementEnd
-- 公開の変化はゲストに見せる子孫を変える。
-- +goose StatementBegin
create trigger public_videos_root_values_ai after insert on public_videos begin
    delete from group_root_values where audience = 0 and root_id in (select root_id from group_closure where content_key = new.content_key);
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger public_videos_root_values_ad after delete on public_videos begin
    delete from group_root_values where audience = 0 and root_id in (select root_id from group_closure where content_key = old.content_key);
end;
-- +goose StatementEnd

-- 棚の規則への一致に関わる書き込みは、動画の鍵（内容の鍵。集まりなら利用者データの鍵）に印を付ける。
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
-- 棚の規則・並び、タグとその種類、フォルダ名のタグ、集まり、登録フォルダの変化は全体を求め直す。
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

-- 既存のグループの構造を埋める（groupDescendantsCTE と同じ求め方で、どのグループも根にする。根と子孫のグループの組ごとに、深さ優先で
-- 最初に現れる道の鍵を 1 本だけ取る）。
insert into group_closure (root_id, content_key, sort_key)
with recursive
groots(id) as (select id from groups),
gnodes(id) as (
    select id from groots
    union
    select gi.child_id from gnodes n join group_items gi on gi.group_id = n.id where gi.child_id is not null),
gclos(anc, descendant) as (
    select id, id from gnodes
    union
    select c.anc, gi.child_id from gclos c join group_items gi on gi.group_id = c.descendant where gi.child_id is not null),
gpath(root_id, group_id, target, sort_path, seen) as (
    select r.id, r.id, c.descendant, '', ',' || r.id || ',' from groots r join gclos c on c.anc = r.id
    union all
    select p.root_id, gi.child_id, p.target, p.sort_path || printf('%010d', gi.position), p.seen || gi.child_id || ','
    from gpath p join group_items gi on gi.group_id = p.group_id
    where p.group_id <> p.target and gi.child_id is not null
        and instr(p.seen, ',' || gi.child_id || ',') = 0
        and gi.position = (select min(n.position) from group_items n
            join gclos nc on nc.anc = n.child_id and nc.descendant = p.target where n.group_id = p.group_id))
select t.root_id, gi.content_key, min(t.sort_path || printf('%010d', gi.position))
from gpath t join group_items gi on gi.group_id = t.target
where t.group_id = t.target and gi.content_key is not null
group by t.root_id, gi.content_key;
update group_closure_dirty set dirty = 0;

-- +goose Down
drop trigger if exists media_folders_shelved_ad;
drop trigger if exists media_folders_shelved_ai;
drop trigger if exists video_bundle_members_shelved_ad;
drop trigger if exists video_bundle_members_shelved_ai;
drop trigger if exists video_folder_names_shelved_ad;
drop trigger if exists video_folder_names_shelved_ai;
drop trigger if exists tag_names_shelved_ad;
drop trigger if exists tag_names_shelved_ai;
drop trigger if exists tags_shelved_au;
drop trigger if exists tags_shelved_ad;
drop trigger if exists smart_folders_shelved_ad;
drop trigger if exists smart_folders_shelved_au;
drop trigger if exists smart_folders_shelved_ai;
drop trigger if exists video_work_organizations_shelved_ad;
drop trigger if exists video_work_organizations_shelved_ai;
drop trigger if exists video_work_people_shelved_ad;
drop trigger if exists video_work_people_shelved_ai;
drop trigger if exists video_favorites_shelved_ad;
drop trigger if exists video_favorites_shelved_ai;
drop trigger if exists playback_progress_shelved_ad;
drop trigger if exists playback_progress_shelved_au;
drop trigger if exists playback_progress_shelved_ai;
drop trigger if exists video_tags_shelved_ad;
drop trigger if exists video_tags_shelved_ai;
drop trigger if exists video_locations_shelved_ad;
drop trigger if exists video_locations_shelved_au;
drop trigger if exists video_locations_shelved_ai;
drop trigger if exists videos_shelved_ad;
drop trigger if exists videos_shelved_au;
drop trigger if exists videos_shelved_ai;
drop trigger if exists public_videos_root_values_ad;
drop trigger if exists public_videos_root_values_ai;
drop trigger if exists video_bundles_root_values_au;
drop trigger if exists video_bundle_members_root_values_ad;
drop trigger if exists video_bundle_members_root_values_au;
drop trigger if exists video_bundle_members_root_values_ai;
drop trigger if exists media_folders_root_values_ad;
drop trigger if exists media_folders_root_values_au;
drop trigger if exists media_folders_root_values_ai;
drop trigger if exists playback_progress_root_values_ad;
drop trigger if exists playback_progress_root_values_au;
drop trigger if exists playback_progress_root_values_ai;
drop trigger if exists video_locations_root_values_ad;
drop trigger if exists video_locations_root_values_au;
drop trigger if exists video_locations_root_values_ai;
drop trigger if exists videos_root_values_ad;
drop trigger if exists videos_root_values_au;
drop trigger if exists videos_root_values_ai;
drop trigger if exists group_items_closure_ad;
drop trigger if exists group_items_closure_au;
drop trigger if exists group_items_closure_ai;
drop trigger if exists groups_closure_ad;
drop trigger if exists groups_closure_au;
drop trigger if exists groups_closure_ai;
drop index if exists playback_progress_updated_idx;
drop index if exists video_locations_created_idx;
drop index if exists video_locations_mtime_idx;
drop table if exists home_shelved_state;
drop table if exists home_shelved_dirty;
drop table if exists home_shelved_videos;
drop table if exists group_root_values;
drop table if exists group_closure_dirty;
drop table if exists group_closure;
