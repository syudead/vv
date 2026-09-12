// Package httpapi は HTTP の経路分配、ハンドラ、および SPA の配信を担う。
//
// JSON API の形は api/openapi.yaml が唯一の真実であり、そこから生成した型を
// internal/httpapi/gen が持つ。生成物は手編集しない。
// 埋め込んだ web/dist を配信し、/api/ 配下以外の未知のパスは index.html へ
// フォールバックする（クライアント側ルーティングのため）。
//
// 依存してよいのは internal/domain だけで、他の internal/* は参照しない。
// 依存の向きは ARCHITECTURE.md を参照。
package httpapi
