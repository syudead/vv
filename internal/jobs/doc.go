// Package jobs はプロセス内のジョブキューとワーカーを担う。
//
// Phase 0 では境界の宣言のみで、実装は Phase 1 で入る。サムネイル生成や
// メタデータ取得など時間のかかる処理を非同期に実行するための境界であり、
// 常駐する外部ミドルウェアは増やさない。
//
// 依存してよいのは internal/domain だけで、他の internal/* は参照しない。
// 依存の向きは ARCHITECTURE.md を参照。
package jobs
