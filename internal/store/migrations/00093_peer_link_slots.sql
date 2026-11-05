-- +goose Up
-- A link's slot fixes the offset of its ids in the screen API (slot × 2^40). The existing link takes slot 1, so its
-- ids keep the 2^40 offset they had and open screens and bookmarks stay valid.
alter table peer_link add column slot integer not null default 1 check (slot >= 1);
create unique index peer_link_slot on peer_link (slot);

-- The next slot to give a link; PeerStore advances it with each link and never gives a slot twice, so a bookmarked
-- remote id cannot name another node's entity after an unpairing.
insert into settings (key, value, updated_at)
select 'peer.next_slot', case when exists (select 1 from peer_link) then '2' else '1' end,
       cast(strftime('%s', 'now') as integer);

-- +goose Down
delete from settings where key = 'peer.next_slot';
drop index peer_link_slot;
alter table peer_link drop column slot;
