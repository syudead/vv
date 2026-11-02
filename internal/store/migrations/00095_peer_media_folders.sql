-- +goose Up
-- The reading node's snapshot of each link's media folders with the owner's switches, which apply on this node
-- only: the switched-off ids go with every request to the link. It is the one local table naming a link's
-- entities, and it is deleted with its link.
create table peer_media_folders (
    node_id      text    not null references peer_link (node_id) on delete cascade,
    -- The folder's id on the link.
    folder_id    integer not null,
    -- The folder's path and version on the link, as last listed; a change of either turns the switch back on.
    path         text    not null,
    version      integer not null,
    enabled      integer not null default 1 check (enabled in (0, 1)),
    -- Unix seconds of the last reconciliation.
    refreshed_at integer not null,
    primary key (node_id, folder_id)
);

-- +goose Down
drop table peer_media_folders;
