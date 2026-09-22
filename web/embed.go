// Package web は Vite のビルド成果物（web/dist）を Go バイナリへ同梱する。
//
// 埋め込みの指示は自分のディレクトリより上を参照できないため、宣言だけを web/ に置き、
// 配信の実装は internal/httpapi/spa.go が持つ。
// web/dist には .gitkeep だけを版管理している。埋め込み対象が空にならないので、
// SPA をビルドしていない状態でも go build ./... は通る（その場合 SPA の配信は
// internal/httpapi/spa.go が「task build を実行してください」と案内する）。
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
