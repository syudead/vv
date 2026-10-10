// Package clef は Ollama の判定 API（POST /v1/systemone）で、Cloudflare の判定モデル
// Clef（clef・clef-flash）に動画の手がかりを渡し、はい／いいえの質問ごとの確率を受け取る
// （docs/design-docs/auto-tagging.md）。
//
// 依存してよいのは internal/domain だけで、他の internal/* は参照しない。
// 依存の向きは ARCHITECTURE.md を参照。
package clef
