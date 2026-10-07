package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
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

// desktopLANAccessKey は Windows デスクトップ版で LAN からの接続を許可するかのキーである。
// 値は "true" か "false"（specs/037-windows-app/research.md R-14）。
const desktopLANAccessKey = "desktop.lan_access"

// LANAccess は LAN からの接続を許可するかを返す。行が無いか "true" でなければ偽である。
func (s *SettingsStore) LANAccess(ctx context.Context) (bool, error) {
	value, found, err := readSetting(ctx, s.db.sql, desktopLANAccessKey)
	if err != nil {
		return false, err
	}
	return found && value == strconv.FormatBool(true), nil
}

// SaveLANAccess は LAN からの接続を許可するかを保存する。
func (s *SettingsStore) SaveLANAccess(ctx context.Context, allowed bool) error {
	return writeSetting(ctx, s.db.sql, desktopLANAccessKey, strconv.FormatBool(allowed))
}

// libraryAutoImportKey はメディアフォルダの変更を検知して自動で取り込むかのキーである。
// 値は "true" か "false" で、行が無ければ入である（specs/042-folder-watch-import/research.md R-7）。
const libraryAutoImportKey = "library.auto_import"

// AutoImport は自動の取り込みが入かを返す。行が無いか "false" でなければ入である。
func (s *SettingsStore) AutoImport(ctx context.Context) (bool, error) {
	value, found, err := readSetting(ctx, s.db.sql, libraryAutoImportKey)
	if err != nil {
		return false, err
	}
	return !found || value != strconv.FormatBool(false), nil
}

// SaveAutoImport は自動の取り込みの入と切を保存する。
func (s *SettingsStore) SaveAutoImport(ctx context.Context, enabled bool) error {
	return writeSetting(ctx, s.db.sql, libraryAutoImportKey, strconv.FormatBool(enabled))
}
