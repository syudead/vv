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

// accountID は唯一のアカウントの行の主キーである。表の check (id = 1) と合わせる。
const accountID = 1

// sessionTokenHash はセッション ID を保存する形（SHA-256 の16進）にする。
// ID そのものは Cookie にだけあり、DB には書かない。DB を読まれても、そのまま
// 使えるセッション ID は漏れない。
func sessionTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Account は唯一のアカウントを返す。行が無ければ domain.ErrAccountNotConfigured を返す。
func (s *AuthStore) Account(ctx context.Context) (domain.Account, error) {
	var account domain.Account
	err := s.sql.QueryRowContext(ctx,
		`select username, password_hash, version from account where id = ?`, accountID,
	).Scan(&account.Username, &account.PasswordHash, &account.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Account{}, domain.ErrAccountNotConfigured
	}
	if err != nil {
		return domain.Account{}, fmt.Errorf("アカウントを読み出せません: %w", err)
	}
	return account, nil
}

// Setup は初回設定である。アカウントの行と最初のセッションを1つの取引で足す。
// 行が既にあれば主キーの衝突で何も書かず、domain.ErrAccountAlreadyConfigured を返す。
// 同時に走った初回設定は、書き込みの錠で順に並ぶので一方だけが成立する。
//
// username と passwordHash の規則（domain.ValidateUsername など）は呼び出し側が確かめる。
func (s *AuthStore) Setup(
	ctx context.Context, username, passwordHash, sessionToken string, now time.Time,
) error {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("初回設定を始められません: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		insert into account (id, username, password_hash, version, updated_at)
		values (?, ?, ?, 1, ?)
		on conflict (id) do nothing`,
		accountID, username, passwordHash, now.Unix())
	if err != nil {
		return fmt.Errorf("アカウントを保存できません: %w", err)
	}
	inserted, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("アカウントを保存できません: %w", err)
	}
	if inserted == 0 {
		return domain.ErrAccountAlreadyConfigured
	}

	if err := insertSession(ctx, tx, sessionToken, 1, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("初回設定を確定できません: %w", err)
	}
	return nil
}

// ChangeUsername はユーザー名を書き換え、版を 1 増やし、セッションを全件消す。
// 行が無ければ domain.ErrAccountNotConfigured を返す。
func (s *AuthStore) ChangeUsername(ctx context.Context, username string, now time.Time) error {
	return s.changeCredentials(ctx, "username", username, now)
}

// ChangePassword はパスワードのハッシュを書き換え、版を 1 増やし、セッションを全件消す。
// 行が無ければ domain.ErrAccountNotConfigured を返す。
func (s *AuthStore) ChangePassword(ctx context.Context, passwordHash string, now time.Time) error {
	return s.changeCredentials(ctx, "password_hash", passwordHash, now)
}

// changeCredentials は資格情報の列 column を value に書き換える。column は
// ChangeUsername と ChangePassword が渡す固定の列名だけである。
//
// 既存のセッションを無効にするのは版の一致（data-model.md §4）で、sessions の
// 全件削除は後片付けである。
func (s *AuthStore) changeCredentials(ctx context.Context, column, value string, now time.Time) error {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("アカウントを書き換えられません: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	//nolint:gosec // column は固定の列名だけで、値は引数で渡す。
	res, err := tx.ExecContext(ctx,
		`update account set `+column+` = ?, version = version + 1, updated_at = ? where id = ?`,
		value, now.Unix(), accountID)
	if err != nil {
		return fmt.Errorf("アカウントを書き換えられません: %w", err)
	}
	updated, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("アカウントを書き換えられません: %w", err)
	}
	if updated == 0 {
		return domain.ErrAccountNotConfigured
	}

	if _, err := tx.ExecContext(ctx, `delete from sessions`); err != nil {
		return fmt.Errorf("セッションを消せません: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("アカウントの書き換えを確定できません: %w", err)
	}
	return nil
}

// AddSession はログインの成功で新しいセッションを足す。同じ取引で期限切れの行を
// 消し、replaceToken が空でなければその行も消す（有効なセッションの Cookie を持って
// ログインし直したとき、古い行を残さないため）。
//
// accountVersion には、照合に使った domain.Account.Version を読んだ値のまま渡す。
// 照合と追加の間に資格情報が書き換えられていれば、足した行は版が合わずに無効になる。
func (s *AuthStore) AddSession(
	ctx context.Context, sessionToken string, accountVersion int64, replaceToken string, now time.Time,
) error {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("セッションを保存できません: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `delete from sessions where expires_at <= ?`, now.Unix()); err != nil {
		return fmt.Errorf("期限切れのセッションを消せません: %w", err)
	}
	if replaceToken != "" {
		if _, err := tx.ExecContext(ctx,
			`delete from sessions where token_hash = ?`, sessionTokenHash(replaceToken)); err != nil {
			return fmt.Errorf("古いセッションを消せません: %w", err)
		}
	}
	if err := insertSession(ctx, tx, sessionToken, accountVersion, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("セッションの保存を確定できません: %w", err)
	}
	return nil
}

// insertSession はセッションを1行足す。期限は now から domain.SessionLifetime 後で、
// 延長しない。
func insertSession(ctx context.Context, tx *sql.Tx, sessionToken string, accountVersion int64, now time.Time) error {
	created := now.Unix()
	expires := now.Add(domain.SessionLifetime).Unix()
	if _, err := tx.ExecContext(ctx, `
		insert into sessions (token_hash, account_version, created_at, expires_at)
		values (?, ?, ?, ?)`,
		sessionTokenHash(sessionToken), accountVersion, created, expires); err != nil {
		return fmt.Errorf("セッションを保存できません: %w", err)
	}
	return nil
}

// Session は sessionToken のセッションが有効かを確かめ、有効なら期限を返す。
//
// 有効なのは、ハッシュが sessions にあり、account の行があり、発行時の版が
// account.version と一致し、期限が now より後のときだけである（data-model.md §4）。
// これを1回の問い合わせで確かめ、判定は読むだけで決める。期限切れの行に当たったら
// その行の削除を試みるが、削除の失敗は判定を変えない。
//
// 問い合わせが失敗したときは無効とせず、誤りを返す。
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
		return time.Time{}, false, fmt.Errorf("セッションを確かめられません: %w", err)
	}

	if expiresAt <= now.Unix() {
		// 掃除は判定の副作用にとどめる。失敗しても期限切れの判定は変わらず、
		// 残った行は次のログインか起動時の掃除で消える。
		_, _ = s.sql.ExecContext(ctx,
			`delete from sessions where token_hash = ? and expires_at <= ?`, hash, now.Unix())
		return time.Time{}, false, nil
	}
	if !current {
		return time.Time{}, false, nil
	}
	return time.Unix(expiresAt, 0), true, nil
}

// DeleteSession はログアウトで sessionToken のセッションを消す。無ければ何もしない。
func (s *AuthStore) DeleteSession(ctx context.Context, sessionToken string) error {
	if _, err := s.sql.ExecContext(ctx,
		`delete from sessions where token_hash = ?`, sessionTokenHash(sessionToken)); err != nil {
		return fmt.Errorf("セッションを消せません: %w", err)
	}
	return nil
}

// DeleteExpiredSessions は期限が now 以前のセッションを消し、消した件数を返す。
// 起動時に呼ぶ。
func (s *AuthStore) DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.sql.ExecContext(ctx, `delete from sessions where expires_at <= ?`, now.Unix())
	if err != nil {
		return 0, fmt.Errorf("期限切れのセッションを消せません: %w", err)
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("期限切れのセッションを消せません: %w", err)
	}
	return deleted, nil
}
