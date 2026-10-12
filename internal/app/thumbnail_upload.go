package app

import (
	"bytes"
	"context"
	"errors"

	"github.com/syudead/vv/internal/domain"
)

// 所有者が手元の画像を動画のサムネイルにするアップロード（specs/046-thumbnail-image-upload/
// research.md R-1・R-2、contracts/screen-api.md）。画像はメモリに持ち、確かめてから生成した
// 画像と同じ適用（Ingest.ApplyThumbnailImage）で代表サムネイルにする。

// uploadGate は、アップロードの元の画像を読む手順を一度に 1 件だけ通す門である。120 メガ
// ピクセルの PNG は読むと 480 MB になるので、確かめと、生成の錠の中での適用の 2 回の合わせの
// どれも、同時には 1 件しか動かさない（R-2）。
type uploadGate chan struct{}

func newUploadGate() uploadGate {
	return make(uploadGate, 1)
}

// acquire は門を取り、放す関数を返す。取れる前に ctx が終われば ctx の誤りを返す。
func (g uploadGate) acquire(ctx context.Context) (release func(), err error) {
	select {
	case g <- struct{}{}:
		return func() { <-g }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// UploadThumbnailImage は、所有者がアップロードした画像 image を動画 videoID の代表サムネイルに
// し、反映後の動画を返す。contentKey は要求の始めに読んだ動画の内容である。
//
// 画像はアップロードの門を持って確かめ、門を放してから出どころ uploaded で ApplyThumbnailImage に
// 渡す。適用は生成の錠を取ってから、2 回の合わせの間だけ改めて門を持つので、自分の動画の錠を
// 待つ間は門を持たない。大きさか画素数の上限を越えれば *domain.ThumbnailUploadTooLargeError、
// PNG・JPEG・静止画の WebP として最後まで読めなければ domain.ErrThumbnailUploadInvalid で、
// どちらも何も書かない。適用の誤りは ApplyThumbnailImage と同じである。
func (i *Ingest) UploadThumbnailImage(
	ctx context.Context, videoID int64, contentKey string, image []byte,
) (domain.Video, error) {
	release, err := i.uploads.acquire(ctx)
	if err != nil {
		return domain.Video{}, err
	}
	err = i.checkUpload(image)
	release()
	if err != nil {
		return domain.Video{}, err
	}
	return i.ApplyThumbnailImage(ctx, videoID, contentKey, image, domain.ThumbnailImageUploaded)
}

// checkThumbnailUpload は、image が上限以下の、最後まで読める PNG・JPEG・静止画の WebP かを
// 確かめる。読み方はサムネイルの編集の画像と同じ checkImage で、画素数の上限だけが大きい。
func checkThumbnailUpload(image []byte) error {
	if len(image) > domain.ThumbnailUploadMaxBytes {
		return &domain.ThumbnailUploadTooLargeError{Limit: domain.ThumbnailUploadMaxBytes}
	}
	if animatedWebP(image) {
		return domain.ErrThumbnailUploadInvalid
	}
	err := checkImage(image, domain.ThumbnailUploadMaxPixels)
	switch {
	case errors.Is(err, errImageTooManyPixels):
		return &domain.ThumbnailUploadTooLargeError{Limit: domain.ThumbnailUploadMaxPixels}
	case err != nil:
		return domain.ErrThumbnailUploadInvalid
	}
	return nil
}

// animatedWebP は image が、拡張形式の見出し（VP8X）でアニメーションの印を立てた WebP かを返す。
// 読み手が読めるかに頼らず、アニメーションの WebP は見出しで断る。
func animatedWebP(image []byte) bool {
	const animationBit = 1 << 1
	if len(image) < 21 || !bytes.Equal(image[0:4], []byte("RIFF")) || !bytes.Equal(image[8:12], []byte("WEBP")) {
		return false
	}
	return bytes.Equal(image[12:16], []byte("VP8X")) && image[20]&animationBit != 0
}
