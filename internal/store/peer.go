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

const linkColumns = `node_id, name, address, secret, revoked, coalesce(pinned_spki, ''), paired_at`

func scanLink(row interface{ Scan(...any) error }) (domain.PeerLink, error) {
	var (
		link     domain.PeerLink
		pairedAt int64
	)
	err := row.Scan(&link.NodeID, &link.Name, &link.Address, &link.Secret, &link.Revoked, &link.PinnedSPKI, &pairedAt)
	link.PairedAt = time.Unix(pairedAt, 0)
	return link, err
}

// Link returns the VVMDM this node reads, or false when there is none.
func (s *PeerStore) Link(ctx context.Context) (domain.PeerLink, bool, error) {
	link, err := scanLink(s.db.sql.QueryRowContext(ctx, `select `+linkColumns+` from peer_link`))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PeerLink{}, false, nil
	}
	if err != nil {
		return domain.PeerLink{}, false, fmt.Errorf("cannot read the peer link: %w", err)
	}
	return link, true, nil
}

// AddLink stores the only link; an existing one answers domain.ErrAlreadyPaired. One statement checks and
// inserts, so two pairings cannot both succeed.
func (s *PeerStore) AddLink(ctx context.Context, link domain.PeerLink) error {
	var pinned any
	if link.PinnedSPKI != "" {
		pinned = link.PinnedSPKI
	}
	res, err := s.db.sql.ExecContext(ctx, `
		insert into peer_link (node_id, name, address, secret, pinned_spki, paired_at)
		select ?, ?, ?, ?, ?, ?
		 where not exists (select 1 from peer_link)`,
		link.NodeID, link.Name, link.Address, link.Secret, pinned, link.PairedAt.Unix())
	if err != nil {
		return fmt.Errorf("cannot save the peer link: %w", err)
	}
	return insertedOne(res, "cannot save the peer link")
}

// insertedOne answers domain.ErrAlreadyPaired when the guarded insert of res stored nothing.
func insertedOne(res sql.Result, failure string) error {
	inserted, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", failure, err)
	}
	if inserted == 0 {
		return domain.ErrAlreadyPaired
	}
	return nil
}

// MarkLinkRevoked records that the peer refused the link's secret, so a start shows revoked.
func (s *PeerStore) MarkLinkRevoked(ctx context.Context, nodeID string) error {
	if _, err := s.db.sql.ExecContext(ctx, `update peer_link set revoked = 1 where node_id = ?`, nodeID); err != nil {
		return fmt.Errorf("cannot mark the peer link revoked: %w", err)
	}
	return nil
}

// PinLinkCertificate stores the SPKI fingerprint the owner confirmed; without the link of nodeID it answers
// domain.ErrNotFound.
func (s *PeerStore) PinLinkCertificate(ctx context.Context, nodeID, spki string) error {
	res, err := s.db.sql.ExecContext(ctx, `update peer_link set pinned_spki = ? where node_id = ?`, spki, nodeID)
	if err != nil {
		return fmt.Errorf("cannot pin the peer certificate: %w", err)
	}
	updated, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot pin the peer certificate: %w", err)
	}
	if updated == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// UpdateLinkAddress answers domain.ErrNotFound without a link.
func (s *PeerStore) UpdateLinkAddress(ctx context.Context, address string) (domain.PeerLink, error) {
	link, err := scanLink(s.db.sql.QueryRowContext(ctx,
		`update peer_link set address = ? returning `+linkColumns, address))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PeerLink{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.PeerLink{}, fmt.Errorf("cannot change the peer address: %w", err)
	}
	return link, nil
}

// DeleteLink deletes the link of nodeID and nothing else, reporting whether one was deleted.
func (s *PeerStore) DeleteLink(ctx context.Context, nodeID string) (bool, error) {
	return s.deletePeerRow(ctx, `delete from peer_link where node_id = ?`, nodeID, "cannot delete the peer link")
}

const grantColumns = `node_id, name, paired_at`

func scanGrant(row interface{ Scan(...any) error }) (domain.PeerGrant, error) {
	var (
		grant    domain.PeerGrant
		pairedAt int64
	)
	err := row.Scan(&grant.NodeID, &grant.Name, &pairedAt)
	grant.PairedAt = time.Unix(pairedAt, 0)
	return grant, err
}

// Grant returns the VVMDM that reads this node, or false when there is none.
func (s *PeerStore) Grant(ctx context.Context) (domain.PeerGrant, bool, error) {
	grant, err := scanGrant(s.db.sql.QueryRowContext(ctx, `select `+grantColumns+` from peer_grant`))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PeerGrant{}, false, nil
	}
	if err != nil {
		return domain.PeerGrant{}, false, fmt.Errorf("cannot read the peer grant: %w", err)
	}
	return grant, true, nil
}

// AddGrant stores the only grant, hashing the secret the reader will present; an existing one answers
// domain.ErrAlreadyPaired.
func (s *PeerStore) AddGrant(ctx context.Context, grant domain.PeerGrant, secret string) error {
	res, err := s.db.sql.ExecContext(ctx, `
		insert into peer_grant (node_id, name, secret_hash, paired_at)
		select ?, ?, ?, ?
		 where not exists (select 1 from peer_grant)`,
		grant.NodeID, grant.Name, sessionTokenHash(secret), grant.PairedAt.Unix())
	if err != nil {
		return fmt.Errorf("cannot save the peer grant: %w", err)
	}
	return insertedOne(res, "cannot save the peer grant")
}

// GrantBySecret hashes the presented secret and returns the grant it belongs to.
func (s *PeerStore) GrantBySecret(ctx context.Context, secret string) (domain.PeerGrant, bool, error) {
	grant, err := scanGrant(s.db.sql.QueryRowContext(ctx,
		`select `+grantColumns+` from peer_grant where secret_hash = ?`, sessionTokenHash(secret)))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PeerGrant{}, false, nil
	}
	if err != nil {
		return domain.PeerGrant{}, false, fmt.Errorf("cannot check the peer credential: %w", err)
	}
	return grant, true, nil
}

// DeleteGrant deletes the grant of nodeID and nothing else, reporting whether one was deleted.
func (s *PeerStore) DeleteGrant(ctx context.Context, nodeID string) (bool, error) {
	return s.deletePeerRow(ctx, `delete from peer_grant where node_id = ?`, nodeID, "cannot delete the peer grant")
}

func (s *PeerStore) deletePeerRow(ctx context.Context, query, nodeID, failure string) (bool, error) {
	res, err := s.db.sql.ExecContext(ctx, query, nodeID)
	if err != nil {
		return false, fmt.Errorf("%s: %w", failure, err)
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("%s: %w", failure, err)
	}
	return deleted > 0, nil
}
