-- +goose Up
-- 動画の作品情報（specs/047-video-work-info/data-model.md「Migration」、research.md R-2）。
-- 作り直せない利用者データで、video_favorites と同じく利用者データの鍵（content_key の列に
-- 集まりなら 'bundle:<id>'、そうでなければ内容の識別子）に結び、videos への外部キーを張らない。
-- 行は少なくとも 1 つの項目か出演者があるときだけある（書く側が消す）。
create table video_work_info (
    content_key    text    primary key,
    code           text,
    release_date   text,
    maker          text,
    -- domain.FoldForMatch(maker)。メーカーの一覧の照合に使う。
    maker_key      text,
    description    text,
    source_url     text,
    -- 品番・メーカー・出演者名の照合形を改行でつないだもの（検索）。search_version は
    -- domain.SearchKeyVersion で、起動時の WorkInfoStore.RefreshSearchKeys が古い行を作り直す。
    search_key     text    not null default '',
    search_version integer not null default 0,
    updated_at     integer not null
) without rowid;
create index video_work_info_maker_idx on video_work_info (maker_key);

-- 出演者。position は入力の順の 0 … n-1、name_key は domain.FoldForMatch(name)。
-- 鍵の付け替え（同じパスの引き継ぎ）は video_work_info の更新から on update cascade で移る。
create table video_work_performers (
    content_key text    not null references video_work_info (content_key)
                            on delete cascade on update cascade,
    position    integer not null,
    name        text    not null,
    name_key    text    not null,
    primary key (content_key, position)
) without rowid;
create index video_work_performers_name_idx on video_work_performers (name_key, content_key);

-- +goose Down
drop table if exists video_work_performers;
drop table if exists video_work_info;
