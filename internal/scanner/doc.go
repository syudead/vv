// Package scanner はメディアディレクトリの走査と索引の更新を担う。
//
// Phase 0 では境界の宣言のみで、実装は Phase 1 で入る。走査結果は
// internal/store を通して永続化し、ファイルシステムへの依存はこの
// パッケージに閉じ込める。
//
// 依存してよいのは internal/domain だけで、他の internal/* は参照しない。
// 依存の向きは ARCHITECTURE.md を参照。
package scanner
