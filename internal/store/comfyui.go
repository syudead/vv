package store

import (
	"context"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// ComfyUI の接続先の設定のキーである。
// 空の値は保存されていないものとして読む。
const (
	comfyUIProviderKey = "comfyui.provider"
	comfyUIURLKey      = "comfyui.url"
	comfyUIAPIKeyKey   = "comfyui.api_key"
	comfyUIModelKey    = "comfyui.model"
)

// ComfyUISettings は保存した ComfyUI の接続先を返す。何も保存されていなければ
// Provider が空のゼロ値になる。キーは 1 つの問い合わせで読み、同時の保存の前後の値が
// 混ざらないようにする（SaveComfyUISettings はすべてを 1 つの取引で書く）。
func (s *SettingsStore) ComfyUISettings(ctx context.Context) (domain.ComfyUISettings, error) {
	rows, err := s.db.sql.QueryContext(ctx, `select key, value from settings where key in (?, ?, ?, ?)`,
		comfyUIProviderKey, comfyUIURLKey, comfyUIAPIKeyKey, comfyUIModelKey)
	if err != nil {
		return domain.ComfyUISettings{}, fmt.Errorf("cannot read the ComfyUI settings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var settings domain.ComfyUISettings
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return domain.ComfyUISettings{}, fmt.Errorf("cannot read the ComfyUI settings: %w", err)
		}
		switch key {
		case comfyUIProviderKey:
			settings.Provider = domain.ComfyUIProvider(value)
		case comfyUIURLKey:
			settings.URL = value
		case comfyUIAPIKeyKey:
			settings.APIKey = value
		case comfyUIModelKey:
			settings.Model = domain.ComfyUIModel(value)
		}
	}
	if err := rows.Err(); err != nil {
		return domain.ComfyUISettings{}, fmt.Errorf("cannot read the ComfyUI settings: %w", err)
	}
	return settings, nil
}

// SaveComfyUISettings は ComfyUI の接続先を 1 つの取引で保存する。値は呼び出し側が
// domain.ApplyComfyUISettingsUpdate で整えたものを渡す。
func (s *SettingsStore) SaveComfyUISettings(ctx context.Context, settings domain.ComfyUISettings) error {
	return writeSettings(ctx, s.db.sql, []settingValue{
		{comfyUIProviderKey, string(settings.Provider)},
		{comfyUIURLKey, settings.URL},
		{comfyUIAPIKeyKey, settings.APIKey},
		{comfyUIModelKey, string(settings.Model)},
	})
}
