// Package comfyui は画像編集の提供元 ComfyUI のアダプタである。所有者が自分で動かす
// ComfyUI と Comfy Cloud の両方を 1 つのパッケージで扱い、提供元ごとの違い（基底 URL、
// 認証、経路）はこの中に閉じる（docs/design-docs/ai-layer.md、
// docs/design-docs/thumbnail-editing.md）。
//
// 接続先は保存された設定（SettingsSource）から問い合わせのたびに読む。どの機能が
// 使っているか（サムネイルの編集など）は知らない。
//
// 依存してよいのは internal/domain だけで、他の internal/* は参照しない。
// 依存の向きは ARCHITECTURE.md を参照。
package comfyui
