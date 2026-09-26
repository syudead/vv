-- +goose Up
-- ライブ変換に要る解析情報（specs/018-live-transcode-seek/data-model.md §1）。
-- 索引であり、消えても次の変換か再解析で埋まる（親 Issue #371 要件 10）。
create table video_transcode_probes (
    video_id   integer primary key references videos (id) on delete cascade,
    -- domain.TranscodeProbeVersion。違えば「無い」と読む（data-model.md §3）。
    version    integer not null,
    -- 解析したファイルの os.Stat の大きさと更新時刻（Unix ナノ秒）。要求時に開いたファイルと比べる。
    size_bytes integer not null,
    mtime_ns   integer not null,
    -- domain.TranscodeProbe の JSON（data-model.md §2）。
    probe      text    not null,
    updated_at integer not null
) without rowid;

-- +goose Down
drop table if exists video_transcode_probes;
