package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// API トークン（specs/026-external-api/data-model.md §1）。平文は発行の応答にだけ現れ、
// 保存するのは SHA-256 だけである。セッション ID と同じ作りなので、ハッシュには
// sessionTokenHash を使う。

// apiTokenTouchInterval は最終使用日時を書き換える最小の間隔である（research.md R-9）。
const apiTokenTouchInterval = 60 * time.Second

// AddAPIToken は名前 name・平文 token の API トークンを 1 件足し、足した行を返す。
// 同じ取引で読んだ account.version を行に書く。account の行が無ければ
// domain.ErrAccountNotConfigured を返す。
//
// name は domain.NormalizeAPITokenName を通した値を渡す（呼び出し側が確かめる）。
func (s *AuthStore) AddAPIToken(ctx context.Context, name, token string, now time.Time) (domain.APIToken, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.APIToken{}, fmt.Errorf("cannot save the API token: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		insert into api_tokens (name, token_hash, account_version, created_at)
		select ?, ?, version, ? from account where id = ?`,
		name, sessionTokenHash(token), now.Unix(), accountID)
	if err != nil {
		return domain.APIToken{}, fmt.Errorf("cannot save the API token: %w", err)
	}
	inserted, err := res.RowsAffected()
	if err != nil {
		return domain.APIToken{}, fmt.Errorf("cannot save the API token: %w", err)
	}
	if inserted == 0 {
		return domain.APIToken{}, domain.ErrAccountNotConfigured
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

// ListAPITokens は有効な API トークンを作成日時の降順（同じ時刻は id の降順）で返す。
// 版の合わない行はアカウントの変更で消えるが、残っていても返さない。
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

// DeleteAPIToken は id の API トークンを消す。無ければ何もしない。
func (s *AuthStore) DeleteAPIToken(ctx context.Context, id int64) error {
	if _, err := s.sql.ExecContext(ctx, `delete from api_tokens where id = ?`, id); err != nil {
		return fmt.Errorf("cannot delete the API token: %w", err)
	}
	return nil
}

// APIToken は平文 token の API トークンが有効かを確かめ、有効ならその行を返す。
//
// 有効なのは、ハッシュが api_tokens にあり、account の行があり、発行時の版が
// account.version と一致するときだけである（research.md R-2）。問い合わせが失敗したときは
// 無効とせず、誤りを返す。
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

// TouchAPIToken は id の API トークンの最終使用日時を now にする。ただし、保存した値が
// 空か 60 秒以上前のときだけ書く（research.md R-9）。行が無ければ何もしない。
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
