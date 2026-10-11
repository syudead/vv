package app

import (
	"context"

	"github.com/syudead/vv/internal/domain"
)

// Classifier は判定モデル（AI）の口である。機能（自動タグ付けなど）はこれだけを通して判定
// モデルを使い、どの提供元が答えるか、接続先、1 回に送れる質問の数を知らない。提供元の
// アダプタ（internal/ollama の *Classifier など）が満たす（docs/design-docs/ai-layer.md）。
type Classifier interface {
	// Classify は質問ごとに「はい」の確率（0 から 1）を返す。答えの無い質問は結果に含めない。
	Classify(ctx context.Context, req domain.ClassifierRequest) (map[string]float64, error)
}
