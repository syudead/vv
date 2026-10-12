package store

import (
	"context"

	"github.com/syudead/vv/internal/domain"
)

// ComfyUI の接続先の設定のキーである（specs/045-comfyui-thumbnails/data-model.md の Settings）。
// 空の値は保存されていないものとして読む。
const (
	comfyUIProviderKey = "comfyui.provider"
	comfyUIURLKey      = "comfyui.url"
	comfyUIAPIKeyKey   = "comfyui.api_key"
)

// ComfyUISettings は保存した ComfyUI の接続先を返す。何も保存されていなければ
// Provider が空のゼロ値になる。
func (s *SettingsStore) ComfyUISettings(ctx context.Context) (domain.ComfyUISettings, error) {
	var provider string
	var settings domain.ComfyUISettings
	for _, item := range []struct {
		key    string
		target *string
	}{
		{comfyUIProviderKey, &provider},
		{comfyUIURLKey, &settings.URL},
		{comfyUIAPIKeyKey, &settings.APIKey},
	} {
		value, _, err := readSetting(ctx, s.db.sql, item.key)
		if err != nil {
			return domain.ComfyUISettings{}, err
		}
		*item.target = value
	}
	settings.Provider = domain.ComfyUIProvider(provider)
	return settings, nil
}

// SaveComfyUISettings は ComfyUI の接続先を 1 つの取引で保存する。値は呼び出し側が
// domain.ApplyComfyUISettingsUpdate で整えたものを渡す。
func (s *SettingsStore) SaveComfyUISettings(ctx context.Context, settings domain.ComfyUISettings) error {
	return writeSettings(ctx, s.db.sql, [][2]string{
		{comfyUIProviderKey, string(settings.Provider)},
		{comfyUIURLKey, settings.URL},
		{comfyUIAPIKeyKey, settings.APIKey},
	})
}
