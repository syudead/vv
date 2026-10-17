package httpapi

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 手元の画像を動画のサムネイルにするアップロードの経路。所有者だけで、
// accessRoutes に載せない。画像はメモリに持ち、確かめと適用は internal/app の *Ingest が行う。

// thumbnailUploadMaxBody はアップロードの要求の本文全体の上限である。image 部分の上限に、部分の
// 区切りと見出しのぶんの 1 MiB を足す。
const thumbnailUploadMaxBody = domain.ThumbnailUploadMaxBytes + 1<<20

// thumbnailUploadBufferBudget は、読んでいる途中か読み終えて確かめと適用を待つアップロードの本文が
// プロセス全体で同時に占めてよいバイト数である。本文の上限 4 件ぶんで、要求の数が増えても受け取った
// 画像のメモリはこれを越えない。読む画像を 1 件ずつにする門（internal/app の uploadGate）とは別で、
// 錠を待つアップロードが他の動画のアップロードを止めないよう、門より広くしてある。
const thumbnailUploadBufferBudget = 4 * thumbnailUploadMaxBody

// newThumbnailUploadBuffers はアップロードの本文のメモリの枠を作る。
func newThumbnailUploadBuffers() *semaphore.Weighted {
	return semaphore.NewWeighted(thumbnailUploadBufferBudget)
}

// thumbnailUploadTimeout は、本文が届き終わったあとの確かめと適用の時間の上限である。門と生成の錠を
// 待つ時間と、最大 60 秒の合わせ 2 回が収まる（R-1）。
const thumbnailUploadTimeout = 5 * time.Minute

// ThumbnailUploads は手元の画像のサムネイルへのアップロードである。internal/app の *Ingest が満たす。
type ThumbnailUploads interface {
	// UploadThumbnailImage は画像 image を動画 videoID の代表サムネイルにし、反映後の動画を返す。
	// contentKey は要求の始めに読んだ動画の内容である。上限を越えれば
	// *domain.ThumbnailUploadTooLargeError、読めなければ domain.ErrThumbnailUploadInvalid、動画が
	// 消えたか内容が変われば domain.ErrContentChanged、ffmpeg が合わせられなければ
	// domain.ErrThumbnailImageInvalid を返し、どれも何も書かない。
	UploadThumbnailImage(ctx context.Context, videoID int64, contentKey string, image []byte) (domain.Video, error)
}

// UploadThumbnailImage は multipart の image を動画のサムネイルにする
// （POST /api/videos/{id}/thumbnail-image）。応答は公開が終わってから返す。
func (s *server) UploadThumbnailImage(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	if s.thumbnailUploads == nil {
		s.internalError(w, "Thumbnail uploads are not configured.", nil)
		return
	}
	// 動画の内容は本文より先に読む。読んだあとに付け替わった動画には、適用が content_changed を返す。
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}
	// 本文を読む前に、本文が占めうるぶんをメモリの枠から取り、応答を書き終えるまで持つ。枠が空くのを
	// 待つ間にページが去れば何もしない。
	weight := thumbnailUploadWeight(r)
	if err := s.thumbnailUploadBuffers.Acquire(r.Context(), weight); err != nil {
		return
	}
	defer s.thumbnailUploadBuffers.Release(weight)
	image, contentVersion, ok := s.readThumbnailUpload(w, r)
	if !ok {
		return
	}
	// プレビューを開いたときの内容と、要求の始めに読んだ内容が違えば、門も錠も取らずに断る。
	if video.ContentKey == "" || contentVersion != thumbnailVersion(video.ContentKey) {
		s.thumbnailUploadError(w, domain.ErrContentChanged)
		return
	}
	// 本文が届き終わったら、ページが去っても確かめと適用を最後まで行う。
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), thumbnailUploadTimeout)
	defer cancel()
	saved, err := s.thumbnailUploads.UploadThumbnailImage(ctx, video.ID, video.ContentKey, image)
	if err != nil {
		s.thumbnailUploadError(w, err)
		return
	}
	s.writeVideoDetail(w, r, saved)
}

// thumbnailUploadWeight は要求 r の本文がメモリの枠から取るバイト数である。Content-Length が本文の
// 上限以下ならその長さ（本文はそれより長く読めない）、無いか上限を越えれば本文の上限とする。
func thumbnailUploadWeight(r *http.Request) int64 {
	if r.ContentLength > 0 && r.ContentLength <= thumbnailUploadMaxBody {
		return r.ContentLength
	}
	return thumbnailUploadMaxBody
}

// thumbnailUploadMaxContentVersion は contentVersion 部分の上限（バイト）である。
const thumbnailUploadMaxContentVersion = 64

// readThumbnailUpload はアップロードの要求の multipart/form-data から image と contentVersion を読む。
// どちらかが無いか、ほかの部分を送るか、contentVersion が 64 バイトを越えれば 400、image か本文全体が
// 上限を越えれば 400 thumbnail_upload_too_large である。
func (s *server) readThumbnailUpload(w http.ResponseWriter, r *http.Request) ([]byte, string, bool) {
	var image, contentVersion []byte
	tooLarge := func() {
		s.thumbnailUploadError(w, &domain.ThumbnailUploadTooLargeError{Limit: domain.ThumbnailUploadMaxBytes})
	}
	ok := s.readMultipart(w, r, thumbnailUploadMaxBody, tooLarge, func(name string, part *multipart.Part) bool {
		switch name {
		case "image":
			data, ok := s.readUploadImagePart(w, part)
			image = data
			return ok
		case "contentVersion":
			data, ok := readPart(part, thumbnailUploadMaxContentVersion)
			if !ok {
				s.invalidRequest(w, "contentVersion must be at most 64 bytes.")
				return false
			}
			contentVersion = data
			return true
		default:
			s.invalidRequest(w, "Only image and contentVersion may be sent.")
			return false
		}
	})
	if !ok {
		return nil, "", false
	}
	if image == nil {
		s.invalidRequest(w, "image is required.")
		return nil, "", false
	}
	if contentVersion == nil {
		s.invalidRequest(w, "contentVersion is required.")
		return nil, "", false
	}
	return image, string(contentVersion), true
}

// readUploadImagePart は image 部分を読む。読めなければ応答を書いて false を返す。
func (s *server) readUploadImagePart(w http.ResponseWriter, part io.Reader) ([]byte, bool) {
	data, err := readUploadPart(part, domain.ThumbnailUploadMaxBytes)
	switch {
	case errors.Is(err, domain.ErrThumbnailUploadTooLarge):
		s.thumbnailUploadError(w, err)
		return nil, false
	case err != nil:
		s.invalidRequest(w, "The request body could not be read.")
		return nil, false
	}
	return data, true
}

// readUploadPart は image 部分を上限まで読む。部分が上限を越えるか、本文全体が上限を越えれば
// *domain.ThumbnailUploadTooLargeError を返す。空の部分も nil でない空の値として返す。
func readUploadPart(part io.Reader, limit int64) ([]byte, error) {
	tooLarge := &domain.ThumbnailUploadTooLargeError{Limit: int(limit)}
	data, err := io.ReadAll(io.LimitReader(part, limit+1))
	var bodyTooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &bodyTooLarge):
		return nil, tooLarge
	case err != nil:
		return nil, err
	case int64(len(data)) > limit:
		return nil, tooLarge
	case data == nil:
		return []byte{}, nil
	}
	return data, nil
}

// thumbnailUploadError はアップロードの誤りを契約の応答にする。
func (s *server) thumbnailUploadError(w http.ResponseWriter, err error) {
	var tooLarge *domain.ThumbnailUploadTooLargeError
	switch {
	case errors.As(err, &tooLarge):
		s.invalidRequestLimit(w, reasonThumbnailUploadTooLarge, tooLarge.Limit,
			"The image must be at most 64 MiB and 120 megapixels.")
	case errors.Is(err, domain.ErrThumbnailUploadInvalid):
		s.invalidRequestReason(w, reasonThumbnailUploadInvalid, "The image must be a readable PNG, JPEG or still WebP.")
	case errors.Is(err, domain.ErrContentChanged):
		s.conflictReason(w, reasonContentChanged, "The video is gone or now holds other content.")
	case errors.Is(err, domain.ErrThumbnailImageInvalid):
		s.conflictReason(w, reasonThumbnailImageInvalid, "The image cannot be fitted as the thumbnail.")
	default:
		s.internalError(w, "Could not set the thumbnail image.", err)
	}
}
