package domain

import "errors"

// 所有者が適用したサムネイルの画像（specs/045-comfyui-thumbnails/data-model.md、research.md
// R-6・R-7・R-10）。画像は内容の識別子に結ぶ thumbnail_images の1行で持ち、代表サムネイルの
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
