package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"
)

const transcodeVideoEncoderKey = "transcode.video_encoder"

// TranscodeEncoderChoice returns the raw value; domain.ParseEncoderChoice owns interpretation.
func (s *SettingsStore) TranscodeEncoderChoice(ctx context.Context) (value string, found bool, err error) {
	return readSetting(ctx, s.db.sql, transcodeVideoEncoderKey)
}

// SaveTranscodeEncoderChoice uses last-write-wins for concurrent saves.
func (s *SettingsStore) SaveTranscodeEncoderChoice(ctx context.Context, value string) error {
	return writeSetting(ctx, s.db.sql, transcodeVideoEncoderKey, value)
}

func readSetting(ctx context.Context, q rowQueryer, key string) (string, bool, error) {
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

func writeSetting(ctx context.Context, q queryExecer, key, value string) error {
	_, err := q.ExecContext(ctx, `
		insert into settings (key, value, updated_at) values (?, ?, ?)
		on conflict (key) do update set value = excluded.value, updated_at = excluded.updated_at`,
		key, value, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("cannot save setting %s: %w", key, err)
	}
	return nil
}

const desktopLANAccessKey = "desktop.lan_access"

// LANAccess defaults to false unless the stored value is exactly true.
func (s *SettingsStore) LANAccess(ctx context.Context) (bool, error) {
	value, found, err := readSetting(ctx, s.db.sql, desktopLANAccessKey)
	if err != nil {
		return false, err
	}
	return found && value == strconv.FormatBool(true), nil
}

func (s *SettingsStore) SaveLANAccess(ctx context.Context, allowed bool) error {
	return writeSetting(ctx, s.db.sql, desktopLANAccessKey, strconv.FormatBool(allowed))
}

const libraryAutoImportKey = "library.auto_import"

// AutoImport defaults to true unless the stored value is exactly false.
func (s *SettingsStore) AutoImport(ctx context.Context) (bool, error) {
	value, found, err := readSetting(ctx, s.db.sql, libraryAutoImportKey)
	if err != nil {
		return false, err
	}
	return !found || value != strconv.FormatBool(false), nil
}

func (s *SettingsStore) SaveAutoImport(ctx context.Context, enabled bool) error {
	return writeSetting(ctx, s.db.sql, libraryAutoImportKey, strconv.FormatBool(enabled))
}

const workInfoPluginProxyURLKey = "work_info_plugins.proxy_url"

// WorkInfoPluginProxyURL returns the raw value or empty; the domain validates it.
func (s *SettingsStore) WorkInfoPluginProxyURL(ctx context.Context) (string, error) {
	value, _, err := readSetting(ctx, s.db.sql, workInfoPluginProxyURLKey)
	return value, err
}

// SaveWorkInfoPluginProxyURL uses an empty string to disable the proxy.
func (s *SettingsStore) SaveWorkInfoPluginProxyURL(ctx context.Context, value string) error {
	return writeSetting(ctx, s.db.sql, workInfoPluginProxyURLKey, value)
}
