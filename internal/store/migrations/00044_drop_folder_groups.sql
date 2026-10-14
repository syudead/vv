-- +goose Up
-- フォルダの索引のグループの表を落とす（specs/048-groups/research.md R-1、data-model.md「Migration」）。
-- グループは groups と group_items が持ち、この 2 表はもう誰も読まない。

drop table folder_group_members;
drop table folder_groups;

-- +goose Down
-- 表は空で戻す。前の版は索引の作り直しで埋めるので、索引を古い（stale）と記録して次の起動で
-- 作り直させる。
create table folder_groups (
    id              integer primary key,
    path_key        text    not null unique,
    path            text    not null,
    name            text    not null,
    title_key       text    not null
);

create table folder_group_members (
    video_id  integer primary key references videos (id) on delete cascade,
    group_id  integer not null references folder_groups (id) on delete cascade,
    position  integer not null,
    unique (group_id, position)
);

update folder_index_state set stale = 1;
