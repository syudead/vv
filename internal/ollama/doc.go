// Package ollama は判定モデルの提供元 Ollama のアダプタである。判定 API
// （POST /v1/systemone）で Cloudflare の判定モデル Clef（clef・clef-flash）に
// domain.ClassifierRequest を渡し、はい／いいえの質問ごとの確率を受け取る
// （docs/design-docs/ai-layer.md）。
//
// 接続先と模型は保存された設定（SettingsSource）から問い合わせのたびに読み、質問は
// 提供元の制約に合わせて分けて送る。どの機能が使っているか（自動タグ付けなど）は知らない。
//
// 依存してよいのは internal/domain だけで、他の internal/* は参照しない。
// 依存の向きは ARCHITECTURE.md を参照。
package ollama
