-- +goose Up
create table video_keyframe_spacing (
    video_id integer primary key references videos (id) on delete cascade,
    version integer not null,
    size_bytes integer not null,
    mtime_ns integer not null,
    spacing text not null check (spacing in ('unknown', 'short', 'sparse'))
);

-- +goose Down
drop table video_keyframe_spacing;
