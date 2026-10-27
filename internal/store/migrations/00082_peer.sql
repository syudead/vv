-- +goose Up
-- Configuration: the one VVMDM this node is paired with, rebuilt by pairing again. At most one row, which PeerStore.AddPeer enforces.
create table peer (
    -- The peer's node id (UUID), compared on every handshake.
    node_id              text    primary key,
    -- The peer's host name at pairing.
    name                 text    not null,
    -- The peer's base URL (http:// or https://), edited by the owner.
    address              text    not null,
    -- The secret this node presents to the peer, in plain text because it must be sent.
    outgoing_secret      text    not null,
    -- Hexadecimal SHA-256 of the secret the peer presents, as api_tokens.token_hash; null once revoked.
    incoming_secret_hash text,
    -- Base64 SHA-256 of the SPKI of the certificate the owner confirmed; null when the system roots verify it.
    pinned_spki          text,
    -- Unix seconds.
    paired_at            integer not null
);

-- +goose Down
drop table peer;
