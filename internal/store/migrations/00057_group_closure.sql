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

-- group_root_values は根ごとのグループの項目の値を、所有者に見せる子孫（登録フォルダの下に所在を持つ
-- 動画）から集めたものである。members は見せる子孫の本数、*_member は各並べ替えの値が最大（同じなら
-- id が最大）の子孫の動画の id、watched と started は見終えた本数と始めた本数（視聴状態の元）、playable
-- は再生できる子孫の有無である。値を変える書き込み（動画・所在・再生の記録・集まり・登録フォルダ）は
-- 引き金で根の行を消し、次の棚の読み出しが欠けた根の行を埋め直す（refillGroupRootValues）。ゲストに
-- 見せる値は公開の子孫から読み出しのたびに集める。
create table group_root_values (
    root_id        integer primary key references groups (id) on delete cascade,
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
    played_member  integer
);

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
drop table if exists group_root_values;
drop table if exists group_closure_dirty;
drop table if exists group_closure;
