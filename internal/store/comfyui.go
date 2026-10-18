package store

import (
	"context"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// Empty connection values are treated as unsaved.
const (
	comfyUIProviderKey = "comfyui.provider"
	comfyUIURLKey      = "comfyui.url"
	comfyUIAPIKeyKey   = "comfyui.api_key"
	comfyUIModelKey    = "comfyui.model"
)

// ComfyUISettings reads all keys in one query to avoid mixing concurrent saves; no settings yields the zero value.
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

// SaveComfyUISettings expects values prepared by domain.ApplyComfyUISettingsUpdate.
func (s *SettingsStore) SaveComfyUISettings(ctx context.Context, settings domain.ComfyUISettings) error {
	return writeSettings(ctx, s.db.sql, []settingValue{
		{comfyUIProviderKey, string(settings.Provider)},
		{comfyUIURLKey, settings.URL},
		{comfyUIAPIKeyKey, settings.APIKey},
		{comfyUIModelKey, string(settings.Model)},
	})
}
