// Package store は SQLite への接続とスキーマのマイグレーションを担う。
//
// ドライバは CGO を必要としない modernc.org/sqlite を使い、マイグレーションは
// goose のライブラリ API と embed.FS で起動時に自動適用する。
// データベースのパスは Config.DataDir/mdm.db に固定する。
//
// 依存してよいのは internal/domain だけで、他の internal/* は参照しない。
// 依存の向きは ARCHITECTURE.md を参照。
package store
