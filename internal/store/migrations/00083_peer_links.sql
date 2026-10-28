-- +goose Up
-- Configuration: pairing became one-directional, so the one peer row splits into the VVMDM this node reads
-- (peer_link) and the VVMDM that reads this one (peer_grant). At most one row each, which PeerStore enforces.
create table peer_link (
    -- The read VVMDM's node id (UUID), compared on every handshake.
    node_id     text    primary key,
    -- Its host name at pairing.
    name        text    not null,
    -- Its base URL (http:// or https://), edited by the owner.
    address     text    not null,
    -- The secret this node presents to it, in plain text because it must be sent.
    secret      text    not null,
    -- 1 once it refused the secret as unknown; the owner removes the link to pair again.
    revoked     integer not null default 0,
    -- Base64 SHA-256 of the SPKI of the certificate the owner confirmed; null when the system roots verify it.
    pinned_spki text,
    -- Unix seconds.
    paired_at   integer not null
);

create table peer_grant (
    -- The reading VVMDM's node id (UUID).
    node_id     text    primary key,
    -- Its host name at pairing.
    name        text    not null,
    -- Hexadecimal SHA-256 of the secret it presents, as api_tokens.token_hash.
    secret_hash text    not null,
    -- Unix seconds.
    paired_at   integer not null
);

-- A mutual pairing keeps both directions: the outgoing secret becomes the link, and the incoming secret the
-- grant. A row whose incoming secret was revoked was shown as revoked by the peer, so its link stays revoked
-- and no grant is made.
insert into peer_link (node_id, name, address, secret, revoked, pinned_spki, paired_at)
select node_id, name, address, outgoing_secret, incoming_secret_hash is null, pinned_spki, paired_at
  from peer;

insert into peer_grant (node_id, name, secret_hash, paired_at)
select node_id, name, incoming_secret_hash, paired_at
  from peer
 where incoming_secret_hash is not null;

drop table peer;

-- +goose Down
create table peer (
    node_id              text    primary key,
    name                 text    not null,
    address              text    not null,
    outgoing_secret      text    not null,
    incoming_secret_hash text,
    pinned_spki          text,
    paired_at            integer not null
);

insert into peer (node_id, name, address, outgoing_secret, incoming_secret_hash, pinned_spki, paired_at)
select l.node_id, l.name, l.address, l.secret, g.secret_hash, l.pinned_spki, l.paired_at
  from peer_link l
  left join peer_grant g on g.node_id = l.node_id;

drop table peer_grant;
drop table peer_link;
