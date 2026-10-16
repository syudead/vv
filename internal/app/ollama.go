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
	// OllamaConnectionSaved は接続先の行があるかを返す。OllamaSettings は行が無ければ既定値を
	// 返すので、保存したかはこちらで見る（specs/058-settings-pages/research.md R-4）。
	OllamaConnectionSaved(ctx context.Context) (bool, error)
}

// OllamaProbe は保存した接続先に Ollama が動いていて、保存した模型があるかを確かめる。
// 判定はさせない。internal/ollama の *Classifier が満たす。
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
	// saved は接続先を保存したあとに呼ぶ（重複の判定が接続を待っていた組を再開する、
	// specs/054-auto-dup/research.md R-13）。nil なら呼ばない。
	saved func(context.Context)
}

// NewOllamaConnection は接続先の設定と確かめを組み立てる。
func NewOllamaConnection(store OllamaSettingsStore, probe OllamaProbe) *OllamaConnection {
	return &OllamaConnection{store: store, probe: probe}
}

// OnSaved は接続先を保存したあとに呼ぶ関数を登録する。要求を受け付ける前に 1 度だけ呼ぶ。
func (o *OllamaConnection) OnSaved(saved func(context.Context)) {
	o.saved = saved
}

// Settings は保存した接続先を返す。
func (o *OllamaConnection) Settings(ctx context.Context) (domain.OllamaSettings, error) {
	return o.store.OllamaSettings(ctx)
}

// Saved は所有者が接続先を保存したかを返す。重複の判定が待つかを決めるのと同じ答え
// （specs/054-auto-dup/research.md R-13、specs/058-settings-pages/research.md R-4）。
func (o *OllamaConnection) Saved(ctx context.Context) (bool, error) {
	return o.store.OllamaConnectionSaved(ctx)
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
	if o.saved != nil {
		o.saved(ctx)
	}
	return normalized, nil
}

// Check は保存した接続先に Ollama が動いていて、保存した模型があるかを確かめる。判定に
// 使えるかは確かめない（判定の口を使う機能が AutoTagger.CheckClassifier のように確かめる）。
// 要求の側が渡した URL へは送らない。
func (o *OllamaConnection) Check(ctx context.Context) error {
	return o.probe.Check(ctx)
}
