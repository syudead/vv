// Package httpapi は HTTP の経路分配、ハンドラ、および SPA の配信を担う。
//
// JSON API の形は api/openapi.yaml が唯一の真実であり、そこから生成した型を
// internal/httpapi/gen が持つ。生成物は手編集しない。
// 埋め込んだ web/dist を配信し、/api/ 配下以外の未知のパスは index.html へ
// フォールバックする（クライアント側ルーティングのため）。
//
// 要求の解釈、アプリケーション層（internal/app）や保存層の呼び出し、gen 型への
// 変換だけを持つ。ユースケースの判断はここに置かない。
//
// 依存してよいのは internal/domain だけで、他の internal/* は参照しない
// （使う操作は interface として宣言する）。依存の向きは ARCHITECTURE.md を参照。
package httpapi
