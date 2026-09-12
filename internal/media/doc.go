// Package media は ffmpeg／ffprobe など外部コマンドへのアダプタを提供する。
//
// Phase 0 の時点では起動前の存在確認（preflight）だけを行い、解析や変換は
// Phase 1 以降で追加する。外部コマンドの実行はこのパッケージに閉じ込め、
// internal/domain には持ち込まない。
//
// 依存してよいのは internal/domain だけで、他の internal/* は参照しない。
// 依存の向きは ARCHITECTURE.md を参照。
package media
