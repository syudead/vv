// Package web は Vite のビルド成果物（web/dist）を Go バイナリへ同梱する。
//
// 埋め込みの指示は自分のディレクトリより上を参照できないため、宣言だけを web/ に置き、
// 配信の実装は internal/httpapi/spa.go が持つ。
// web/dist にはプレースホルダを版管理しているので、SPA をビルドしていない状態でも
// go build ./... は通る（research.md R-008）。
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist は web/dist を根としたファイルシステムを返す。
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		// dist は埋め込み済みなので、ここへは来ない。
		panic(err)
	}
	return sub
}
