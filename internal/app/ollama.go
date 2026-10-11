package app

import (
	"context"
	"sync"

	"github.com/syudead/vv/internal/domain"
)

// OllamaSettingsStore は Ollama の接続先の保存先である。internal/store の *SettingsStore が満たす。
type OllamaSettingsStore interface {
	OllamaSettings(ctx context.Context) (domain.OllamaSettings, error)
	SaveOllamaSettings(ctx context.Context, settings domain.OllamaSettings) error
}

// OllamaProbe は保存した接続先で判定モデルが答えるかを確かめる。internal/ollama の
// *Classifier が満たす。
type OllamaProbe interface {
	Check(ctx context.Context) error
}

// OllamaConnection は判定モデルの提供元 Ollama への接続先の設定と確かめである。判定
// モデルを使う機能（自動タグ付け）の設定とは別に持ち、機能が切でも確かめられる
// （docs/design-docs/ai-layer.md）。
type OllamaConnection struct {
	store OllamaSettingsStore
	probe OllamaProbe
	mu    sync.Mutex
}

// NewOllamaConnection は接続先の設定と確かめを組み立てる。
func NewOllamaConnection(store OllamaSettingsStore, probe OllamaProbe) *OllamaConnection {
	return &OllamaConnection{store: store, probe: probe}
}

// Settings は保存した接続先を返す。
func (o *OllamaConnection) Settings(ctx context.Context) (domain.OllamaSettings, error) {
	return o.store.OllamaSettings(ctx)
}

// SaveSettings は接続先を整えて保存し、保存後の値を返す。規則に合わなければ
// domain.ErrInvalidOllamaSettings を包んだ誤りを返す。
func (o *OllamaConnection) SaveSettings(ctx context.Context, settings domain.OllamaSettings) (domain.OllamaSettings, error) {
	normalized, err := domain.NormalizeOllamaSettings(settings)
	if err != nil {
		return domain.OllamaSettings{}, err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.store.SaveOllamaSettings(ctx, normalized); err != nil {
		return domain.OllamaSettings{}, err
	}
	return normalized, nil
}

// Check は保存した接続先で判定モデルが答えるかを確かめる。要求の側が渡した URL へは送らない。
func (o *OllamaConnection) Check(ctx context.Context) error {
	return o.probe.Check(ctx)
}
