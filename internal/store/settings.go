package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// transcodeVideoEncoderKey は所有者が選んだライブ変換の映像エンコード方式のキーである
// （specs/025-hardware-encoding/data-model.md §2）。
const transcodeVideoEncoderKey = "transcode.video_encoder"

// TranscodeEncoderChoice は保存したライブ変換の映像エンコード方式を、解釈せずに文字列の
// まま返す。行が無ければ found は偽である。解釈は domain.ParseEncoderChoice が行う。
func (s *SettingsStore) TranscodeEncoderChoice(ctx context.Context) (value string, found bool, err error) {
	return readSetting(ctx, s.db.sql, transcodeVideoEncoderKey)
}

// SaveTranscodeEncoderChoice はライブ変換の映像エンコード方式を保存する。同時の保存は
// 後に書いたものが残る。
func (s *SettingsStore) SaveTranscodeEncoderChoice(ctx context.Context, value string) error {
	return writeSetting(ctx, s.db.sql, transcodeVideoEncoderKey, value)
}

func readSetting(ctx context.Context, q *sql.DB, key string) (string, bool, error) {
	var value string
	err := q.QueryRowContext(ctx, `select value from settings where key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("cannot read setting %s: %w", key, err)
	}
	return value, true, nil
}

func writeSetting(ctx context.Context, q *sql.DB, key, value string) error {
	_, err := q.ExecContext(ctx, `
		insert into settings (key, value, updated_at) values (?, ?, ?)
		on conflict (key) do update set value = excluded.value, updated_at = excluded.updated_at`,
		key, value, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("cannot save setting %s: %w", key, err)
	}
	return nil
}
