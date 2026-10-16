-- +goose Up
-- アクティビティの記録（specs/055-activity/data-model.md）。利用者データで、作り直さず、間引かない
-- （research.md R-1）。videos への外部キーは持たず、対象は内容の鍵と書いた時点の表示名で残す（R-10）。
-- この移行より前に起きたことの記録は書かない。
create table activity_entries (
    id              integer primary key,
    kind            text    not null,
    -- 種類の機能（contracts/screen-api.md「Features」）。undo は取り消した記録のものを写す。
    feature         text    not null,
    subject_kind    text    not null check (subject_kind in ('video', 'pair', 'scan', 'folder')),
    -- 内容の鍵、組の小さい方の鍵、走査の id、外したメディアフォルダのパス。
    subject_key     text    not null,
    -- 組の大きい方の鍵。
    subject_key_b   text,
    subject_label   text    not null,
    subject_label_b text,
    status          text    not null check (status in ('running', 'attention', 'done')),
    -- running と attention の理由。
    reason          text,
    -- done の結果。
    outcome         text,
    detail          text,
    -- 失敗した import の、諦めた段階（jobs.kind の値）。
    stage           text,
    -- undo の記録が取り消した記録。
    undoes_id       integer,
    decider         text    not null check (decider in ('rule', 'ai', 'owner')),
    probability     real,
    -- 自動で束ねた組の、加わった側のメンバーの鍵と元の代表の鍵（JSON）。
    undo_keys       text,
    -- この記録を取り消した所有者の記録。
    undone_by       integer,
    -- 所有者のタグの操作（tag_attach・tag_detach・tag_keep・tag_reject）が変えたタグ。改名に左右されない。
    tag_id          integer,
    -- 自動の結果（auto_tag の done、version_judgement の same/different）を書いた時点の、記録の id の最大値。
    -- 取り消しは、これより大きい id の所有者の記録を「その後の変更」と見る（R-7）。
    decided_mark    integer,
    created_at      integer not null,
    updated_at      integer not null
);

-- 状態の絞り込みと件数、機能の絞り込み、同じ種類と対象の記録を引く（R-4・R-9）。
create index activity_entries_status_idx on activity_entries (status, id);
create index activity_entries_feature_idx on activity_entries (feature, id);
create index activity_entries_subject_idx on activity_entries (kind, subject_key, subject_key_b, id);

-- auto_tag の記録が付けたタグ。取り消しはタグごとに行い、所有者がその後にそのタグを変えたかは、より大きい id の
-- 所有者の記録で見分ける（R-7）。
create table activity_entry_tags (
    entry_id    integer not null,
    tag_id      integer not null,
    tag_name    text    not null,
    suggested   integer not null default 0,
    -- 付けたときの利用者データの鍵（内容の鍵か集まりの鍵）。
    user_key    text    not null,
    probability real    not null,
    undone_by   integer,
    primary key (entry_id, tag_id)
) without rowid;

-- auto_tag の記録が付けたタグの行が今ある利用者データの鍵: 書いた鍵と、束ねる・解くときにその行を写した先の鍵。
-- 行を写した先が別のメンバーの行で埋まっていたとき（代表の行が勝つ）や、解いて残る 1 本の行を集まりの行で置き換えた
-- ときは、その鍵は入らない。取り消しはこの鍵の行だけを消し、対象の動画の今の鍵が無ければ already_reverted にする
-- （R-7）。利用者データ。
create table activity_entry_tag_keys (
    entry_id    integer not null,
    tag_id      integer not null,
    user_key    text    not null,
    primary key (entry_id, tag_id, user_key)
) without rowid;
create index activity_entry_tag_keys_key_idx on activity_entry_tag_keys (user_key);

-- 所有者のタグの操作が触った内容の鍵。集まりの鍵への操作はその時のメンバー全員の鍵。取り消しは、タグの id と
-- 内容の鍵で所有者の後の操作を探す（R-7）。利用者データ。
create table activity_entry_keys (
    entry_id    integer not null,
    content_key text    not null,
    primary key (entry_id, content_key)
) without rowid;
create index activity_entry_keys_key_idx on activity_entry_keys (content_key, entry_id);

-- 所有者が自動の「違う動画」を取り消した組。判定のワーカーは取り出さない（R-7）。利用者データ。
create table video_version_judging_holds (
    key_a      text    not null,
    key_b      text    not null,
    created_at integer not null,
    primary key (key_a, key_b),
    check (key_a < key_b)
) without rowid;

-- 待ち行列の行が属する記録。記録が running でない行を取り出すと、新しい記録を開く。
alter table jobs add column activity_id integer;
alter table auto_tag_queue add column activity_id integer;
alter table video_version_candidates add column activity_id integer;

-- +goose Down
alter table video_version_candidates drop column activity_id;
alter table auto_tag_queue drop column activity_id;
alter table jobs drop column activity_id;
drop table if exists video_version_judging_holds;
drop table if exists activity_entry_keys;
drop table if exists activity_entry_tag_keys;
drop table if exists activity_entry_tags;
drop table if exists activity_entries;
