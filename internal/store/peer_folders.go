package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const peerFolderColumns = `folder_id, path, version, enabled, refreshed_at`

func scanPeerFolder(row interface{ Scan(...any) error }) (domain.PeerMediaFolder, error) {
	var (
		folder    domain.PeerMediaFolder
		refreshed int64
	)
	err := row.Scan(&folder.FolderID, &folder.Path, &folder.Version, &folder.Enabled, &refreshed)
	folder.RefreshedAt = time.Unix(refreshed, 0)
	return folder, err
}

// PeerFolders returns the snapshot of the media folders of the link of nodeID, in the link's id order; empty
// without that link.
func (s *PeerStore) PeerFolders(ctx context.Context, nodeID string) ([]domain.PeerMediaFolder, error) {
	return readPeerFolders(ctx, s.db.cached, nodeID)
}

func readPeerFolders(ctx context.Context, q rowsQueryer, nodeID string) ([]domain.PeerMediaFolder, error) {
	rows, err := q.QueryContext(ctx,
		`select `+peerFolderColumns+` from peer_media_folders where node_id = ? order by folder_id`, nodeID)
	if err != nil {
		return nil, fmt.Errorf("cannot read the media folders of the peer link: %w", err)
	}
	defer func() { _ = rows.Close() }()
	folders := []domain.PeerMediaFolder{}
	for rows.Next() {
		folder, err := scanPeerFolder(rows)
		if err != nil {
			return nil, fmt.Errorf("cannot read the media folders of the peer link: %w", err)
		}
		folders = append(folders, folder)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the media folders of the peer link: %w", err)
	}
	return folders, nil
}

// ReplacePeerFolders reconciles the snapshot of the link of nodeID with the folders the link listed, by
// domain.ReconcilePeerFolders, and reports whether a row other than its stamp changed. Without that link it
// answers domain.ErrNotFound, so a listing that arrives after an unpairing stores nothing.
func (s *PeerStore) ReplacePeerFolders(
	ctx context.Context, nodeID string, listed []domain.PeerFolder, now time.Time,
) (bool, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("cannot save the media folders of the peer link: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var exists int
	err = tx.QueryRowContext(ctx, `select 1 from peer_link where node_id = ?`, nodeID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, domain.ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("cannot read the peer link: %w", err)
	}
	current, err := readPeerFolders(ctx, tx, nodeID)
	if err != nil {
		return false, err
	}
	next, changed := domain.ReconcilePeerFolders(current, listed, now)
	if _, err := tx.ExecContext(ctx, `delete from peer_media_folders where node_id = ?`, nodeID); err != nil {
		return false, fmt.Errorf("cannot save the media folders of the peer link: %w", err)
	}
	for _, folder := range next {
		if _, err := tx.ExecContext(ctx, `
			insert into peer_media_folders (node_id, folder_id, path, version, enabled, refreshed_at)
			values (?, ?, ?, ?, ?, ?)`,
			nodeID, folder.FolderID, folder.Path, folder.Version, boolToInt(folder.Enabled), now.Unix()); err != nil {
			return false, fmt.Errorf("cannot save the media folders of the peer link: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("cannot commit the media folders of the peer link: %w", err)
	}
	return changed, nil
}

// SetPeerFolderEnabled writes the owner's switch of one snapshot row and answers it with whether the value
// changed; a folder the snapshot of nodeID lacks answers domain.ErrNotFound.
func (s *PeerStore) SetPeerFolderEnabled(
	ctx context.Context, nodeID string, folderID int64, enabled bool,
) (domain.PeerMediaFolder, bool, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.PeerMediaFolder{}, false, fmt.Errorf("cannot switch the media folder of the peer link: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	folder, err := scanPeerFolder(tx.QueryRowContext(ctx,
		`select `+peerFolderColumns+` from peer_media_folders where node_id = ? and folder_id = ?`, nodeID, folderID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PeerMediaFolder{}, false, domain.ErrNotFound
	}
	if err != nil {
		return domain.PeerMediaFolder{}, false, fmt.Errorf("cannot read the media folder of the peer link: %w", err)
	}
	if folder.Enabled == enabled {
		return folder, false, tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, `update peer_media_folders set enabled = ? where node_id = ? and folder_id = ?`,
		boolToInt(enabled), nodeID, folderID); err != nil {
		return domain.PeerMediaFolder{}, false, fmt.Errorf("cannot switch the media folder of the peer link: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.PeerMediaFolder{}, false, fmt.Errorf("cannot switch the media folder of the peer link: %w", err)
	}
	folder.Enabled = enabled
	return folder, true, nil
}
