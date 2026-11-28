package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ComfyUI connection settings belong to the provider adapter; see docs/design-docs/ai-layer.md and docs/design-docs/thumbnail-editing.md.

type ComfyUIProvider string

const (
	ComfyUISelfHosted ComfyUIProvider = "self_hosted"
	ComfyUICloud      ComfyUIProvider = "cloud"
)

// ComfyUIModel names the image-edit model the owner chose; internal/comfyui defines the names and the default
// (docs/design-docs/ai-layer.md).
type ComfyUIModel string

const comfyUIMaxAPIKeyLength = 1000

var (
	// The URL and missing-key errors wrap ErrInvalidComfyUISettings.
	ErrInvalidComfyUISettings = errors.New("invalid ComfyUI settings")
	ErrComfyUIURLInvalid      = fmt.Errorf("%w: a self-hosted ComfyUI needs an http or https URL", ErrInvalidComfyUISettings)
	ErrComfyUIAPIKeyMissing   = fmt.Errorf("%w: Comfy Cloud needs an API key", ErrInvalidComfyUISettings)
)

// ComfyUISettings retains inactive provider values for switching back and never returns APIKey through the API; empty Provider means unsaved and empty Model selects the adapter's default.
type ComfyUISettings struct {
	Provider ComfyUIProvider
	URL      string
	APIKey   string
	Model    ComfyUIModel
}

func (s ComfyUISettings) Saved() bool {
	return s.Provider != ""
}

// ComfyUISettingsUpdate preserves nil fields; an empty APIKey deletes the saved key.
type ComfyUISettingsUpdate struct {
	Provider ComfyUIProvider
	URL      string
	APIKey   *string
	Model    *ComfyUIModel
}

// ApplyComfyUISettingsUpdate wraps ErrInvalidComfyUISettings for invalid URLs, missing cloud keys or a model not in
// models, and preserves the saved URL when selecting cloud.
func ApplyComfyUISettingsUpdate(current ComfyUISettings, update ComfyUISettingsUpdate, models []ComfyUIModel) (ComfyUISettings, error) {
	next := current
	next.Provider = update.Provider
	if update.APIKey != nil {
		key := strings.TrimSpace(*update.APIKey)
		if len(key) > comfyUIMaxAPIKeyLength || strings.ContainsFunc(key, isControlOrSpace) {
			return ComfyUISettings{}, fmt.Errorf("%w: the API key must be a single token", ErrInvalidComfyUISettings)
		}
		next.APIKey = key
	}
	if update.Model != nil {
		if !slices.Contains(models, *update.Model) {
			return ComfyUISettings{}, fmt.Errorf("%w: the model must be one of %v", ErrInvalidComfyUISettings, models)
		}
		next.Model = *update.Model
	}
	switch update.Provider {
	case ComfyUISelfHosted:
		normalized, ok := normalizeBaseURL(update.URL)
		if !ok {
			return ComfyUISettings{}, ErrComfyUIURLInvalid
		}
		next.URL = normalized
	case ComfyUICloud:
		if next.APIKey == "" {
			return ComfyUISettings{}, ErrComfyUIAPIKeyMissing
		}
	default:
		return ComfyUISettings{}, fmt.Errorf("%w: the provider must be self_hosted or cloud", ErrInvalidComfyUISettings)
	}
	return next, nil
}

type ComfyUIMissingKind string

const (
	ComfyUIMissingNode  ComfyUIMissingKind = "node"
	ComfyUIMissingModel ComfyUIMissingKind = "model"
)

// Folder and Source apply only to models and identify the models subdirectory and download page.
type ComfyUIMissingItem struct {
	Kind   ComfyUIMissingKind
	Name   string
	Folder string
	Source string
}

// ComfyUIMissingError means the server responded but required nodes or models are missing; Items is nonempty.
type ComfyUIMissingError struct {
	Items []ComfyUIMissingItem
}

// Error omits names because Items carries them.
func (e *ComfyUIMissingError) Error() string {
	nodes, models := 0, 0
	for _, item := range e.Items {
		if item.Kind == ComfyUIMissingNode {
			nodes++
		} else {
			models++
		}
	}
	parts := make([]string, 0, 2)
	if models > 0 {
		parts = append(parts, fmt.Sprintf("%d model file(s) are missing; download each into its folder", models))
	}
	if nodes > 0 {
		parts = append(parts, fmt.Sprintf("%d node type(s) are missing; update ComfyUI", nodes))
	}
	return "ComfyUI is missing files the thumbnail workflows need: " + strings.Join(parts, ", and ")
}
