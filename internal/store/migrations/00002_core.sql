-- +goose Up
-- コア機能（取り込み・一覧・再生・検索）のスキーマ。
-- 表の設計は specs/002-core-video-library/data-model.md にある。
--
-- 区分は 2 つある。videos・videos_fts・jobs・scans とサムネイル画像は
-- 「再スキャンで作り直せる索引」で、playback_progress だけが
-- 「作り直せない利用者データ」である。この区分を表の単位で成立させるため、
-- playback_progress の鍵は videos.id ではなく content_key にする。

-- videos に、取り込みで判明する事実と解析の状態を足す。
-- 既存行には content_key が無いため、いったん既定値付きで足してから
-- 空文字を許さない一意索引で「未設定のまま 2 行」を防ぐ。
-- 001 の時点で本番の行は存在しない（走査の実装が無かった）ので、
-- 既定値 '' の行が残っていても次のスキャンで実際の鍵に置き換わる。
alter table videos add column content_key       text    not null default '';
-- 尺（ミリ秒）。ffprobe の取得前は null で、0 では代用しない。
alter table videos add column duration_ms       integer;
alter table videos add column width             integer;
alter table videos add column height            integer;
-- 拡張子から決めたコンテナ（mp4 / webm など）。
alter table videos add column container         text;
alter table videos add column video_codec       text;
-- 音声が無い場合は null。
alter table videos add column audio_codec       text;
-- 解析前は「再生できない」側に倒す。playable = 1 は probe_state = done の
-- ときだけ取り得る（R-103）。
alter table videos add column playable          integer not null default 0;
-- container / video_codec / audio_codec のいずれか。
alter table videos add column unplayable_reason text;
-- pending / done / failed。
alter table videos add column probe_state       text    not null default 'pending';
alter table videos add column probe_error       text;
-- pending / done / failed。
alter table videos add column thumbnail_state   text    not null default 'pending';
alter table videos add column updated_at        integer not null default 0;

-- 一覧の 2 つの並び順をそのまま索引にする（R-109）。カーソル方式の
-- ページングは (並び順の値, id) を境界に使うので、id を索引に含める。
create index videos_added_at_desc_idx on videos (added_at desc, id desc);
create index videos_title_asc_idx     on videos (title asc, id asc);
-- 内容が同じ動画を 2 行持たないことを、移動検出の前提として強制する。
-- 既定値 '' の行は移行直後にだけ生じ得るので、索引の対象から外す。
create unique index videos_content_key_idx on videos (content_key) where content_key <> '';

-- 再生位置。利用者データなので videos への外部キーを張らない。
-- 動画が一覧から消えても、同じ内容を置き直せば再生位置が戻る（FR-025／FR-026）。
create table playback_progress (
    content_key text    primary key,
    position_ms integer not null check (position_ms >= 0),
    -- 記録した時点で判明していた尺。完了判定の再計算に使う。
    duration_ms integer,
    completed   integer not null default 0 check (completed in (0, 1)),
    updated_at  integer not null
);

-- 重い処理（ffprobe の解析とサムネイル生成）の待ち行列。
-- 常駐ミドルウェアを増やさないため、表とプロセス内のワーカーで持つ（R-106）。
create table jobs (
    id         integer primary key,
    kind       text    not null check (kind in ('probe', 'thumbnail')),
    -- 索引側のデータなので、動画が消えたら連鎖して消す。
    video_id   integer not null references videos (id) on delete cascade,
    state      text    not null check (state in ('queued', 'running', 'done', 'failed')),
    attempts   integer not null default 0,
    last_error text,
    created_at integer not null,
    updated_at integer not null
);

-- 取り出しは state と id の順に見る。
create index jobs_state_id_idx on jobs (state, id);
-- 同じ対象に未完了のジョブを 2 件積まない。再スキャンを繰り返しても
-- 待ち行列が膨らまないようにするための制約である。
create unique index jobs_pending_kind_video_idx
    on jobs (kind, video_id)
    where state in ('queued', 'running');

-- 走査 1 回の記録。進捗は Scan として API に出す。
create table scans (
    id          integer primary key,
    state       text    not null check (state in ('running', 'done', 'failed')),
    started_at  integer,
    finished_at integer,
    total       integer not null default 0,
    completed   integer not null default 0,
    failed      integer not null default 0,
    error       text
);

-- 走査は同時に 1 本だけ。POST /api/scans が実行中のものを返す（409 にしない）
-- 判断は、この制約が成り立っていることを前提にしている（R-108）。
create unique index scans_single_running_idx on scans (state) where state = 'running';

-- +goose Down
drop index if exists scans_single_running_idx;
drop table if exists scans;
drop index if exists jobs_pending_kind_video_idx;
drop index if exists jobs_state_id_idx;
drop table if exists jobs;
drop table if exists playback_progress;
drop index if exists videos_content_key_idx;
drop index if exists videos_title_asc_idx;
drop index if exists videos_added_at_desc_idx;

-- SQLite の drop column は、索引・トリガから参照されている列を落とせない。
-- videos_fts の同期トリガは title と path だけを参照しており、ここで落とす
-- 列には触れないため、そのまま実行できる。
alter table videos drop column updated_at;
alter table videos drop column thumbnail_state;
alter table videos drop column probe_error;
alter table videos drop column probe_state;
alter table videos drop column unplayable_reason;
alter table videos drop column playable;
alter table videos drop column audio_codec;
alter table videos drop column video_codec;
alter table videos drop column container;
alter table videos drop column height;
alter table videos drop column width;
alter table videos drop column duration_ms;
alter table videos drop column content_key;
