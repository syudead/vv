package domain

import (
	"io"
	"time"
)

// LiveTranscodeRequest は 1 要求分のライブ変換の指示である。httpapi の経路が作り、
// internal/media の LiveTranscoder が受け取る（specs/018-live-transcode-seek/plan.md
// Structural Decision 6）。
type LiveTranscodeRequest struct {
	// Path は変換で開いたファイルのパスである。
	Path string
	// Source は開いたファイルの大きさと更新時刻である。その場で解析したときは、
	// 解析のあともこの値のままのファイルだった場合にだけ結果を返す。
	Source FileStamp
	// Probe は保存済みで、使ってよいと判定した解析情報である。nil ならその場で
	// ffprobe を実行する。
	Probe *TranscodeProbe
	// StartMs は変換を始める位置（ミリ秒）である。
	StartMs int64
	// Normalize は映像と音声を必ずエンコードし直すかである。
	Normalize bool
	// StartupDeadline はその場の解析と最初のデータまでを合わせた期限である。
	StartupDeadline time.Time
}

// LiveTranscode は最初のデータが出たライブ変換 1 本である。
type LiveTranscode struct {
	// Stream は最初のデータから読める出力である。
	Stream io.ReadCloser
	// Wait は処理の終わりを待つ。ちょうど 1 回呼ぶ。
	Wait func() error
	// Stop は処理を止める。何度呼んでもよい。
	Stop func()
	// Probed は変換の中でその場の ffprobe を実行したときの結果である。保存済みの
	// 解析情報だけで始められたとき、または解析のあとにファイルが変わっていたときは
	// nil で、呼び出し側はこれがあるときだけ保存する。
	Probed *TranscodeProbe
}
