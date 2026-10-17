package domain

import "errors"

// 所有者が適用したサムネイルの画像。画像は内容の識別子に結ぶ thumbnail_images の1行で持ち、代表サムネイルの
// ファイルはその行から作り直す。

// ErrContentChanged は、適用しようとした画像の動画が消えたか、別の内容を持つようになった
// ことを表す。何も書かない。API では conflict（409）/ content_changed になる（R-10）。
var ErrContentChanged = errors.New("the video no longer holds the content")

// ErrThumbnailImageInvalid は、適用しようとした画像を読めないことを表す。何も書かない。
// API では conflict（409）/ thumbnail_image_invalid になる。
var ErrThumbnailImageInvalid = errors.New("the thumbnail image cannot be decoded")

// ThumbnailImageAspect は、適用する画像を合わせる動画の表示上の縦横比（横÷縦）を返す
// （R-7）。表示上の比率（回転と画素の縦横比を反映済み）があればそれ、無ければ幅と高さから
// 求める。どちらも分からなければ 0 を返し、画像は自身の縦横比を保つ。
func ThumbnailImageAspect(video Video) float64 {
	if video.DisplayAspectRatio != nil && *video.DisplayAspectRatio > 0 {
		return *video.DisplayAspectRatio
	}
	if video.Width != nil && video.Height != nil && *video.Width > 0 && *video.Height > 0 {
		return float64(*video.Width) / float64(*video.Height)
	}
	return 0
}

// ThumbnailImageOrigin は適用した画像の出どころである。thumbnail_images.origin に書き、所有者の応答の thumbnailImage になる。
type ThumbnailImageOrigin string

const (
	// ThumbnailImageGenerated は ComfyUI の編集結果を適用した画像である。
	ThumbnailImageGenerated ThumbnailImageOrigin = "generated"
	// ThumbnailImageUploaded は所有者がアップロードした画像である。
	ThumbnailImageUploaded ThumbnailImageOrigin = "uploaded"
)

// Valid は出どころが既知の値かを返す。
func (o ThumbnailImageOrigin) Valid() bool {
	return o == ThumbnailImageGenerated || o == ThumbnailImageUploaded
}

// AppliedThumbnailImage は適用する画像の行（thumbnail_images）に書く内容である
type AppliedThumbnailImage struct {
	// Image は動画の縦横比に合わせた長辺 640 px の JPEG で、公開する代表サムネイルと同じバイト列。
	Image []byte
	// Source は Image と同じ切り抜きで長辺を最大 2048 px にした JPEG で、今のサムネイルからの
	// 編集の元になる。
	Source []byte
	// Origin は画像の出どころである。
	Origin ThumbnailImageOrigin
}

// CurrentThumbnailSelection は内容の今の代表サムネイルの選び方である。今のサムネイルからの編集が
// 始める画像を決める。適用した画像と指定した
// 位置は同じ時点のもので、並行するアップロード・位置の変更・元に戻す操作の途中の組み合わせには
// ならない。
type CurrentThumbnailSelection struct {
	// Image は適用した画像の元の画像で、元の画像を持たない古い行では代表サムネイルである。
	// 画像を適用していなければ nil である。
	Image []byte
	// PositionMs は指定した代表サムネイルの位置で、指定していなければ nil である。
	PositionMs *int64
}
