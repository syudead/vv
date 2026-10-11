-- +goose Up
-- 自動タグ付けの待ち行列と、判定を終えた記録（docs/design-docs/auto-tagging.md）。鍵は動画の
-- 内容の識別子（content_key）で、videos への外部キーを張らない。判定を終えた行（done）を
-- 残し、取り込みのあとの自動の判定が同じ内容を何度も判定して、所有者が外したタグを
-- 付け直さないようにする。作り直しで消えても、判定し直すだけで利用者データは失われない。
create table auto_tag_queue (
    content_key text    primary key,
    state       text    not null check (state in ('queued', 'running', 'done', 'failed')),
    -- 失敗の理由（英語）。failed のときだけ空でない。
    error       text    not null default '',
    -- 積んだ順に取り出すための時刻（Unix ミリ秒）。
    queued_at   integer not null,
    updated_at  integer not null
) without rowid;

create index auto_tag_queue_state_idx on auto_tag_queue (state, queued_at);

-- 自動タグ付けが確率の中間の帯で仮に付けたタグ。1 は仮の付与で、所有者が確定すると 0 になる。
-- 手で付けた分と確定した分は 0 である。
alter table video_tags add column tentative integer not null default 0 check (tentative in (0, 1));

-- 所有者が却下した仮の付与。鍵は video_tags と同じ利用者データの鍵で、この動画にこのタグを
-- 自動タグ付けで付け直さない。作り直せない利用者データで、videos への外部キーを張らない。
create table auto_tag_rejections (
    content_key text    not null,
    tag_id      integer not null references tags (id) on delete cascade,
    rejected_at integer not null,
    primary key (content_key, tag_id)
) without rowid;

-- +goose Down
drop table if exists auto_tag_rejections;
alter table video_tags drop column tentative;
drop table if exists auto_tag_queue;
