package domain

import "time"

// CopySeekAllowance は、映像をコピーして途中から始める変換で許す「指定位置と実際の
// 開始位置（直前のキーフレーム）の差」の上限である。x264/x265 の既定のキーフレーム間隔
// （250 フレーム）は 23.976fps で約 10.4 秒になり、それを含むよう 15 秒にしている
// （specs/018-live-transcode-seek/research.md R-2、docs/design-docs/live-transcode-seek.md）。
const CopySeekAllowance = 15 * time.Second

// CopySeekWithinAllowance は、指定位置 requestedMs から始めようとしたコピーが実際には
// actualMs から始まったとき、そのまま使ってよいかを返す。差が CopySeekAllowance を
// 超えたら、呼び出し側は映像をエンコードし直して指定位置から始める。最初のキーフレーム
// より前へのシークのように実際の開始位置が指定位置より後になるときは使ってよい。
func CopySeekWithinAllowance(requestedMs, actualMs int64) bool {
	return requestedMs-actualMs <= CopySeekAllowance.Milliseconds()
}
