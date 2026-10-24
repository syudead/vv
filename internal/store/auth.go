package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Match the singleton table constraint CHECK (id = 1).
const accountID = 1

// Persist only SHA-256 hashes so reading the database does not expose usable session tokens.
func sessionTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Account returns domain.ErrAccountNotConfigured when no account exists.
func (s *AuthStore) Account(ctx context.Context) (domain.Account, error) {
	var account domain.Account
	err := s.sql.QueryRowContext(ctx,
		`select username, password_hash, version from account where id = ?`, accountID,
	).Scan(&account.Username, &account.PasswordHash, &account.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Account{}, domain.ErrAccountNotConfigured
	}
	if err != nil {
		return domain.Account{}, fmt.Errorf("cannot read the account: %w", err)
	}
	return account, nil
}

// Setup creates the account and session atomically; the primary key permits only one concurrent setup to succeed.
func (s *AuthStore) Setup(
	ctx context.Context, username, passwordHash, sessionToken string, now time.Time,
) error {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot start the initial setup: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		insert into account (id, username, password_hash, version, updated_at)
		values (?, ?, ?, 1, ?)
		on conflict (id) do nothing`,
		accountID, username, passwordHash, now.Unix())
	if err != nil {
		return fmt.Errorf("cannot save the account: %w", err)
	}
	inserted, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot save the account: %w", err)
	}
	if inserted == 0 {
		return domain.ErrAccountAlreadyConfigured
	}

	if err := insertSession(ctx, tx, sessionToken, 1, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot commit the initial setup: %w", err)
	}
	return nil
}

// ChangeUsername increments the account version and revokes all sessions and API tokens.
func (s *AuthStore) ChangeUsername(ctx context.Context, username string, now time.Time) error {
	return s.changeCredentials(ctx, "username", username, now)
}

// ChangePassword increments the account version and revokes all sessions and API tokens.
func (s *AuthStore) ChangePassword(ctx context.Context, passwordHash string, now time.Time) error {
	return s.changeCredentials(ctx, "password_hash", passwordHash, now)
}

// Only fixed column names are accepted; version checks invalidate sessions before cleanup removes their rows.
func (s *AuthStore) changeCredentials(ctx context.Context, column, value string, now time.Time) error {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot update the account: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`update account set `+column+` = ?, version = version + 1, updated_at = ? where id = ?`,
		value, now.Unix(), accountID)
	if err != nil {
		return fmt.Errorf("cannot update the account: %w", err)
	}
	updated, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot update the account: %w", err)
	}
	if updated == 0 {
		return domain.ErrAccountNotConfigured
	}

	if err := revokeAllAccess(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot commit the account update: %w", err)
	}
	return nil
}

// ReplaceCredentials writes both credentials only while sessionToken is still a current session,
// revokes every session and API token, and issues newSessionToken at the new account version.
func (s *AuthStore) ReplaceCredentials(
	ctx context.Context, sessionToken, username, passwordHash, newSessionToken string, now time.Time,
) error {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot update the account: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var version int64
	err = tx.QueryRowContext(ctx, `
		update account
		   set username = ?, password_hash = ?, version = version + 1, updated_at = ?
		 where id = ? and exists (
		       select 1 from sessions s
		        where s.token_hash = ? and s.account_version = account.version and s.expires_at > ?)
		returning version`,
		username, passwordHash, now.Unix(), accountID, sessionTokenHash(sessionToken), now.Unix(),
	).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrSessionNotValid
	}
	if err != nil {
		return fmt.Errorf("cannot update the account: %w", err)
	}

	if err := revokeAllAccess(ctx, tx); err != nil {
		return err
	}
	if err := insertSession(ctx, tx, newSessionToken, version, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot commit the account update: %w", err)
	}
	return nil
}

// revokeAllAccess removes every session and API token after a credential change.
func revokeAllAccess(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `delete from sessions`); err != nil {
		return fmt.Errorf("cannot delete sessions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `delete from api_tokens`); err != nil {
		return fmt.Errorf("cannot delete API tokens: %w", err)
	}
	return nil
}

// AddSession uses the account version read during authentication so concurrent credential changes invalidate the new session.
func (s *AuthStore) AddSession(
	ctx context.Context, sessionToken string, accountVersion int64, replaceToken string, now time.Time,
) error {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot save the session: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `delete from sessions where expires_at <= ?`, now.Unix()); err != nil {
		return fmt.Errorf("cannot delete expired sessions: %w", err)
	}
	if replaceToken != "" {
		if _, err := tx.ExecContext(ctx,
			`delete from sessions where token_hash = ?`, sessionTokenHash(replaceToken)); err != nil {
			return fmt.Errorf("cannot delete the old session: %w", err)
		}
	}
	if err := insertSession(ctx, tx, sessionToken, accountVersion, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot commit the session: %w", err)
	}
	return nil
}

// Session expiry is fixed at creation and never extended.
func insertSession(ctx context.Context, tx *sql.Tx, sessionToken string, accountVersion int64, now time.Time) error {
	created := now.Unix()
	expires := now.Add(domain.SessionLifetime).Unix()
	if _, err := tx.ExecContext(ctx, `
		insert into sessions (token_hash, account_version, created_at, expires_at)
		values (?, ?, ?, ?)`,
		sessionTokenHash(sessionToken), accountVersion, created, expires); err != nil {
		return fmt.Errorf("cannot save the session: %w", err)
	}
	return nil
}

// Session checks hash, account version and expiry in one query; cleanup failures cannot change validity.
func (s *AuthStore) Session(ctx context.Context, sessionToken string, now time.Time) (time.Time, bool, error) {
	hash := sessionTokenHash(sessionToken)
	var (
		expiresAt int64
		current   bool
	)
	err := s.sql.QueryRowContext(ctx, `
		select s.expires_at, a.version is not null and a.version = s.account_version
		  from sessions s
		  left join account a on a.id = ?
		 where s.token_hash = ?`,
		accountID, hash,
	).Scan(&expiresAt, &current)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("cannot check the session: %w", err)
	}

	if expiresAt <= now.Unix() {
		// Best-effort cleanup does not affect the expired verdict; later login or startup can remove the row.
		_, _ = s.sql.ExecContext(ctx,
			`delete from sessions where token_hash = ? and expires_at <= ?`, hash, now.Unix())
		return time.Time{}, false, nil
	}
	if !current {
		return time.Time{}, false, nil
	}
	return time.Unix(expiresAt, 0), true, nil
}

func (s *AuthStore) DeleteSession(ctx context.Context, sessionToken string) error {
	if _, err := s.sql.ExecContext(ctx,
		`delete from sessions where token_hash = ?`, sessionTokenHash(sessionToken)); err != nil {
		return fmt.Errorf("cannot delete sessions: %w", err)
	}
	return nil
}

// DeleteExpiredSessions removes expired sessions at startup and returns their count.
func (s *AuthStore) DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.sql.ExecContext(ctx, `delete from sessions where expires_at <= ?`, now.Unix())
	if err != nil {
		return 0, fmt.Errorf("cannot delete expired sessions: %w", err)
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("cannot delete expired sessions: %w", err)
	}
	return deleted, nil
}
