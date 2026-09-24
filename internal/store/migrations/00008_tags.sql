-- +goose Up
-- タグ（specs/014-video-tags/data-model.md §1）。再スキャンやメディアフォルダの
-- 変更で消えてはならない利用者データなので、videos への外部キーは張らない
-- （ARCHITECTURE.md「Two kinds of data」）。

-- タグそのもの。名前は tag_names が持つ。
-- autoincrement にするのは、削除したタグの id を別のタグに再利用させないため。
-- 古いタブや URL に残った id が、無関係の新しいタグを指さないようにする。
create table tags (
    id         integer primary key autoincrement,
    created_at integer not null
);

-- タグの名前とシノニムを1つの名前空間で持つ。
-- name の照合は既定の BINARY で、綴りが完全に一致するときだけ同じ名前になる。
create table tag_names (
    name      text    primary key,
    tag_id    integer not null references tags (id) on delete cascade,
    -- 1 は表示する元の名前、0 はシノニム。
    canonical integer not null check (canonical in (0, 1)),
    -- 検索欄の照合用の鍵。domain.FoldForMatch(name) を Go が書く（§7）。
    search_key     text    not null default '',
    -- search_key を作った規則の版。video_locations.search_version と同じ domain.SearchKeyVersion。
    search_version integer not null default 0
) without rowid;

-- 1つのタグに元の名前は1つだけ。
create unique index tag_names_canonical_idx on tag_names (tag_id) where canonical = 1;
create index tag_names_tag_idx on tag_names (tag_id);

-- 動画の中身とタグの対応。利用者データなので videos への外部キーを張らない。
create table video_tags (
    content_key text    not null,
    tag_id      integer not null references tags (id) on delete cascade,
    created_at  integer not null,
    primary key (content_key, tag_id)
) without rowid;

-- タグごとの本数と、統合・削除での付け替えに使う。
create index video_tags_tag_idx on video_tags (tag_id, content_key);

-- +goose Down
drop table if exists video_tags;
drop table if exists tag_names;
drop table if exists tags;
