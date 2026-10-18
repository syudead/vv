-- +goose Up
-- Rebuildable probe data is refreshed by the next transcode or ingest probe.
create table video_transcode_probes (
    video_id   integer primary key references videos (id) on delete cascade,
    -- A mismatched domain.TranscodeProbeVersion is read as absent.
    version    integer not null,
    -- Probed file size and modification time in Unix nanoseconds, checked against the opened file.
    size_bytes integer not null,
    mtime_ns   integer not null,
    -- Serialized domain.TranscodeProbe.
    probe      text    not null,
    updated_at integer not null
) without rowid;

-- +goose Down
drop table if exists video_transcode_probes;
