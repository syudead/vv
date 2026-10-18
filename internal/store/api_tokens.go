package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Persist only SHA-256 token hashes; plaintext appears only in the issuance response.

const apiTokenTouchInterval = 60 * time.Second

// AddAPIToken atomically revalidates the issuing session and uses its account version to prevent issuance after credential changes.
func (s *AuthStore) AddAPIToken(
	ctx context.Context, sessionToken, name, token string, now time.Time,
) (domain.APIToken, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.APIToken{}, fmt.Errorf("cannot save the API token: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		insert into api_tokens (name, token_hash, account_version, created_at)
		select ?, ?, s.account_version, ?
		  from sessions s
		  join account a on a.id = ? and a.version = s.account_version
		 where s.token_hash = ? and s.expires_at > ?`,
		name, sessionTokenHash(token), now.Unix(), accountID, sessionTokenHash(sessionToken), now.Unix())
	if err != nil {
		return domain.APIToken{}, fmt.Errorf("cannot save the API token: %w", err)
	}
	inserted, err := res.RowsAffected()
	if err != nil {
		return domain.APIToken{}, fmt.Errorf("cannot save the API token: %w", err)
	}
	if inserted == 0 {
		return domain.APIToken{}, domain.ErrSessionNotValid
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.APIToken{}, fmt.Errorf("cannot save the API token: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.APIToken{}, fmt.Errorf("cannot commit the API token: %w", err)
	}
	return domain.APIToken{ID: id, Name: name, CreatedAt: time.Unix(now.Unix(), 0)}, nil
}

// ListAPITokens returns valid tokens newest first, excluding stale account versions.
func (s *AuthStore) ListAPITokens(ctx context.Context) ([]domain.APIToken, error) {
	rows, err := s.sql.QueryContext(ctx, `
		select t.id, t.name, t.created_at, t.last_used_at
		  from api_tokens t
		  join account a on a.id = ? and a.version = t.account_version
		 order by t.created_at desc, t.id desc`, accountID)
	if err != nil {
		return nil, fmt.Errorf("cannot list API tokens: %w", err)
	}
	defer func() { _ = rows.Close() }()

	tokens := []domain.APIToken{}
	for rows.Next() {
		var (
			token    domain.APIToken
			created  int64
			lastUsed sql.NullInt64
		)
		if err := rows.Scan(&token.ID, &token.Name, &created, &lastUsed); err != nil {
			return nil, fmt.Errorf("cannot list API tokens: %w", err)
		}
		token.CreatedAt = time.Unix(created, 0)
		if lastUsed.Valid {
			token.LastUsedAt = time.Unix(lastUsed.Int64, 0)
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot list API tokens: %w", err)
	}
	return tokens, nil
}

func (s *AuthStore) DeleteAPIToken(ctx context.Context, id int64) error {
	if _, err := s.sql.ExecContext(ctx, `delete from api_tokens where id = ?`, id); err != nil {
		return fmt.Errorf("cannot delete the API token: %w", err)
	}
	return nil
}

// APIToken requires a matching hash and current account version; query errors remain errors.
func (s *AuthStore) APIToken(ctx context.Context, token string) (domain.APIToken, bool, error) {
	var (
		found    domain.APIToken
		created  int64
		lastUsed sql.NullInt64
	)
	err := s.sql.QueryRowContext(ctx, `
		select t.id, t.name, t.created_at, t.last_used_at
		  from api_tokens t
		  join account a on a.id = ? and a.version = t.account_version
		 where t.token_hash = ?`,
		accountID, sessionTokenHash(token),
	).Scan(&found.ID, &found.Name, &created, &lastUsed)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.APIToken{}, false, nil
	}
	if err != nil {
		return domain.APIToken{}, false, fmt.Errorf("cannot check the API token: %w", err)
	}
	found.CreatedAt = time.Unix(created, 0)
	if lastUsed.Valid {
		found.LastUsedAt = time.Unix(lastUsed.Int64, 0)
	}
	return found, true, nil
}

// TouchAPIToken writes at most once per minute; missing rows are ignored.
func (s *AuthStore) TouchAPIToken(ctx context.Context, id int64, now time.Time) error {
	at := now.Unix()
	if _, err := s.sql.ExecContext(ctx, `
		update api_tokens set last_used_at = ?
		 where id = ? and (last_used_at is null or last_used_at <= ?)`,
		at, id, at-int64(apiTokenTouchInterval/time.Second)); err != nil {
		return fmt.Errorf("cannot record the API token use: %w", err)
	}
	return nil
}
