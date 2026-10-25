package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const nodeIDKey = "node.id"

// EnsureNodeID returns this node's id, saving candidate first when none is stored yet.
func (s *PeerStore) EnsureNodeID(ctx context.Context, candidate string) (string, error) {
	if _, err := s.db.sql.ExecContext(ctx, `
		insert into settings (key, value, updated_at) values (?, ?, ?)
		on conflict (key) do nothing`, nodeIDKey, candidate, time.Now().Unix()); err != nil {
		return "", fmt.Errorf("cannot save the node id: %w", err)
	}
	id, found, err := readSetting(ctx, s.db.sql, nodeIDKey)
	if err != nil {
		return "", err
	}
	if !found {
		return "", errors.New("the node id was not saved")
	}
	return id, nil
}

const peerColumns = `node_id, name, address, outgoing_secret, incoming_secret_hash is null, coalesce(pinned_spki, ''), paired_at`

func scanPeer(row interface{ Scan(...any) error }) (domain.PeerRecord, error) {
	var (
		record   domain.PeerRecord
		pairedAt int64
	)
	err := row.Scan(&record.NodeID, &record.Name, &record.Address, &record.OutgoingSecret,
		&record.IncomingRevoked, &record.PinnedSPKI, &pairedAt)
	record.PairedAt = time.Unix(pairedAt, 0)
	return record, err
}

// Peer returns the peer record, or false when this node is not paired.
func (s *PeerStore) Peer(ctx context.Context) (domain.PeerRecord, bool, error) {
	record, err := scanPeer(s.db.sql.QueryRowContext(ctx, `select `+peerColumns+` from peer`))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PeerRecord{}, false, nil
	}
	if err != nil {
		return domain.PeerRecord{}, false, fmt.Errorf("cannot read the peer: %w", err)
	}
	return record, true, nil
}

// AddPeer stores the only peer record, hashing incomingSecret; an existing record answers
// domain.ErrAlreadyPaired. One statement checks and inserts, so two pairings cannot both succeed.
func (s *PeerStore) AddPeer(ctx context.Context, record domain.PeerRecord, incomingSecret string) error {
	var pinned any
	if record.PinnedSPKI != "" {
		pinned = record.PinnedSPKI
	}
	res, err := s.db.sql.ExecContext(ctx, `
		insert into peer (node_id, name, address, outgoing_secret, incoming_secret_hash, pinned_spki, paired_at)
		select ?, ?, ?, ?, ?, ?, ?
		 where not exists (select 1 from peer)`,
		record.NodeID, record.Name, record.Address, record.OutgoingSecret, sessionTokenHash(incomingSecret),
		pinned, record.PairedAt.Unix())
	if err != nil {
		return fmt.Errorf("cannot save the peer: %w", err)
	}
	inserted, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot save the peer: %w", err)
	}
	if inserted == 0 {
		return domain.ErrAlreadyPaired
	}
	return nil
}

// PeerBySecret hashes the presented secret; a record whose incoming secret was revoked never matches.
func (s *PeerStore) PeerBySecret(ctx context.Context, secret string) (domain.PeerRecord, bool, error) {
	record, err := scanPeer(s.db.sql.QueryRowContext(ctx,
		`select `+peerColumns+` from peer where incoming_secret_hash = ?`, sessionTokenHash(secret)))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PeerRecord{}, false, nil
	}
	if err != nil {
		return domain.PeerRecord{}, false, fmt.Errorf("cannot check the peer credential: %w", err)
	}
	return record, true, nil
}

// RevokeIncomingSecret clears the incoming secret hash, so the peer credential is refused from then on.
func (s *PeerStore) RevokeIncomingSecret(ctx context.Context, nodeID string) error {
	if _, err := s.db.sql.ExecContext(ctx,
		`update peer set incoming_secret_hash = null where node_id = ?`, nodeID); err != nil {
		return fmt.Errorf("cannot revoke the peer credential: %w", err)
	}
	return nil
}

// UpdatePeerAddress answers domain.ErrNotFound when this node is not paired.
func (s *PeerStore) UpdatePeerAddress(ctx context.Context, address string) (domain.PeerRecord, error) {
	record, err := scanPeer(s.db.sql.QueryRowContext(ctx,
		`update peer set address = ? returning `+peerColumns, address))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PeerRecord{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.PeerRecord{}, fmt.Errorf("cannot change the peer address: %w", err)
	}
	return record, nil
}

// DeletePeer deletes the record of nodeID and nothing else, reporting whether one was deleted.
func (s *PeerStore) DeletePeer(ctx context.Context, nodeID string) (bool, error) {
	res, err := s.db.sql.ExecContext(ctx, `delete from peer where node_id = ?`, nodeID)
	if err != nil {
		return false, fmt.Errorf("cannot delete the peer: %w", err)
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cannot delete the peer: %w", err)
	}
	return deleted > 0, nil
}
