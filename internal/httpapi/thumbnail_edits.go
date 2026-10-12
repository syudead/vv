package httpapi

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"unicode/utf8"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// サムネイルの編集のセッションの経路（specs/045-comfyui-thumbnails/contracts/screen-api.md）。
// どれも所有者だけで、accessRoutes に載せない。セッションは internal/app の *ThumbnailEdits が
// メモリに持つ。

// thumbnailEditMaxMaskBytes はマスクの PNG の大きさの上限である。2048 px 四方の RGBA を
// 圧縮しなくても収まる。
const thumbnailEditMaxMaskBytes = 20 << 20

// thumbnailEditMaxRunBody は実行の要求の本文全体の上限である。各部分の上限の和に余裕を足す。
const thumbnailEditMaxRunBody = thumbnailEditMaxMaskBytes + domain.ThumbnailEditMaxReferenceBytes + 1<<20

// ThumbnailEdits はサムネイルの編集のセッションである。internal/app の *ThumbnailEdits が満たす。
// 終わったセッションは domain.ErrThumbnailEditNotFound を返す。
type ThumbnailEdits interface {
	Start(ctx context.Context, video domain.Video, path string, start domain.ThumbnailEditStart) (domain.ThumbnailEdit, error)
	Get(ctx context.Context, id string) (domain.ThumbnailEdit, error)
	Frame(ctx context.Context, id string) ([]byte, error)
	Result(ctx context.Context, id string) ([]byte, error)
	StartRun(ctx context.Context, id string, input domain.ThumbnailEditRunInput) (domain.ThumbnailEdit, error)
	Cancel(ctx context.Context, id string) (domain.ThumbnailEdit, error)
	Apply(ctx context.Context, id string) (domain.Video, error)
	Discard(ctx context.Context, id string)
}

// StartThumbnailEdit は動画のコマか今のサムネイルから編集を始める（POST /api/videos/{id}/thumbnail-edits。
// specs/046-thumbnail-image-upload/contracts/screen-api.md）。
func (s *server) StartThumbnailEdit(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	var body gen.StartThumbnailEditRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if !body.Source.Valid() {
		s.invalidRequest(w, "source must be frame or thumbnail.")
		return
	}
	if s.thumbnailEdits == nil {
		s.internalError(w, "Thumbnail editing is not configured.", nil)
		return
	}
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}
	start := domain.ThumbnailEditStart{Source: domain.ThumbnailEditSource(body.Source)}
	if start.Source == domain.ThumbnailEditFromFrame {
		start.PositionMs = body.PositionMs
		// 所在を開く前に位置を確かめる（SetVideoThumbnailPosition と同じ）。
		if err := checkThumbnailEditPosition(video, body.PositionMs); err != nil {
			s.thumbnailPositionError(w, video, err)
			return
		}
	}
	if s.files == nil {
		s.internalError(w, "Media file access is not configured.", nil)
		return
	}
	// 読む元は配信と同じ規則で決め、symlink を辿った先のパスを渡す（SetVideoThumbnailPosition）。
	_, path, ok := s.resolveMediaFile(r, video)
	if !ok {
		s.notFoundReason(w, reasonFileUnavailable, "Cannot open this video's file.")
		return
	}
	edit, err := s.thumbnailEdits.Start(r.Context(), video, path, start)
	if err != nil {
		if !s.thumbnailEditError(w, err) {
			s.thumbnailPositionError(w, video, err)
		}
		return
	}
	s.writeThumbnailEdit(w, http.StatusCreated, edit)
}

// checkThumbnailEditPosition は frame から始める編集の位置 positionMs を確かめる。無い位置は
// 尺の外と同じ誤りである。
func checkThumbnailEditPosition(video domain.Video, positionMs *int64) error {
	if positionMs == nil {
		return domain.ErrThumbnailPositionOutOfRange
	}
	return domain.CheckThumbnailPosition(video, *positionMs)
}

// GetThumbnailEdit はセッションの今の状態を返す（GET /api/thumbnail-edits/{editId}）。
func (s *server) GetThumbnailEdit(w http.ResponseWriter, r *http.Request, editID gen.ThumbnailEditId) {
	if s.thumbnailEdits == nil {
		s.internalError(w, "Thumbnail editing is not configured.", nil)
		return
	}
	edit, err := s.thumbnailEdits.Get(r.Context(), editID)
	if err != nil {
		s.thumbnailEditFailure(w, err)
		return
	}
	s.writeThumbnailEdit(w, http.StatusOK, edit)
}

// GetThumbnailEditFrame はコマの PNG を返す（GET /api/thumbnail-edits/{editId}/frame）。
func (s *server) GetThumbnailEditFrame(w http.ResponseWriter, r *http.Request, editID gen.ThumbnailEditId) {
	if s.thumbnailEdits == nil {
		s.internalError(w, "Thumbnail editing is not configured.", nil)
		return
	}
	frame, err := s.thumbnailEdits.Frame(r.Context(), editID)
	if err != nil {
		s.thumbnailEditFailure(w, err)
		return
	}
	s.writeImage(w, "image/png", frame)
}

// GetThumbnailEditResult は成功した実行の出力の画像を返す（GET /api/thumbnail-edits/{editId}/result）。
func (s *server) GetThumbnailEditResult(w http.ResponseWriter, r *http.Request, editID gen.ThumbnailEditId) {
	if s.thumbnailEdits == nil {
		s.internalError(w, "Thumbnail editing is not configured.", nil)
		return
	}
	result, err := s.thumbnailEdits.Result(r.Context(), editID)
	if err != nil {
		s.thumbnailEditFailure(w, err)
		return
	}
	s.writeImage(w, http.DetectContentType(result), result)
}

// RunThumbnailEdit は multipart の入力で実行を始める（POST /api/thumbnail-edits/{editId}/runs）。
func (s *server) RunThumbnailEdit(w http.ResponseWriter, r *http.Request, editID gen.ThumbnailEditId) {
	if s.thumbnailEdits == nil {
		s.internalError(w, "Thumbnail editing is not configured.", nil)
		return
	}
	input, ok := s.readThumbnailEditRun(w, r)
	if !ok {
		return
	}
	edit, err := s.thumbnailEdits.StartRun(r.Context(), editID, input)
	if err != nil {
		s.thumbnailEditFailure(w, err)
		return
	}
	s.writeThumbnailEdit(w, http.StatusAccepted, edit)
}

// CancelThumbnailEdit は進んでいる実行を中止する（POST /api/thumbnail-edits/{editId}/cancel）。
func (s *server) CancelThumbnailEdit(w http.ResponseWriter, r *http.Request, editID gen.ThumbnailEditId) {
	if s.thumbnailEdits == nil {
		s.internalError(w, "Thumbnail editing is not configured.", nil)
		return
	}
	edit, err := s.thumbnailEdits.Cancel(r.Context(), editID)
	if err != nil {
		s.thumbnailEditFailure(w, err)
		return
	}
	s.writeThumbnailEdit(w, http.StatusOK, edit)
}

// ApplyThumbnailEdit は成功した実行の画像をサムネイルにする（POST /api/thumbnail-edits/{editId}/apply）。
func (s *server) ApplyThumbnailEdit(w http.ResponseWriter, r *http.Request, editID gen.ThumbnailEditId) {
	if s.thumbnailEdits == nil {
		s.internalError(w, "Thumbnail editing is not configured.", nil)
		return
	}
	video, err := s.thumbnailEdits.Apply(r.Context(), editID)
	if err != nil {
		s.thumbnailEditFailure(w, err)
		return
	}
	s.writeVideoDetail(w, r, video)
}

// DiscardThumbnailEdit はセッションを捨てる（DELETE /api/thumbnail-edits/{editId}）。
func (s *server) DiscardThumbnailEdit(w http.ResponseWriter, r *http.Request, editID gen.ThumbnailEditId) {
	if s.thumbnailEdits == nil {
		s.internalError(w, "Thumbnail editing is not configured.", nil)
		return
	}
	s.thumbnailEdits.Discard(r.Context(), editID)
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

// readThumbnailEditRun は実行の要求の multipart/form-data を読む。prompt・mask・reference の
// どれも省けるが、同じ部分を 2 度送るか、ほかの部分を送れば 400 である。大きすぎるマスクと
// 参考画像は、それぞれの reason の 400 にする。
func (s *server) readThumbnailEditRun(w http.ResponseWriter, r *http.Request) (domain.ThumbnailEditRunInput, bool) {
	var input domain.ThumbnailEditRunInput
	ok := s.readMultipart(w, r, thumbnailEditMaxRunBody, nil, func(name string, part io.Reader) bool {
		return s.readThumbnailEditPart(w, name, part, &input)
	})
	return input, ok
}

// readMultipart は要求の本文を multipart/form-data として、全体で maxBody バイトまで読み、部分ごとに
// その名前と中身で readPart を呼ぶ。Content-Type が違うか形が壊れているか、同じ部分を 2 度送れば
// 400 を書いて false を返す。部分の区切りや見出しを読む間に本文全体が maxBody を越えたときは、
// tooLarge があればそれが応答を書き、無ければ形の誤りと同じ 400 にする。readPart が false を返せば
// （応答は readPart が書く）そこでやめる。サムネイルの編集の実行とアップロード（thumbnail_upload.go）が
// 共有する。
func (s *server) readMultipart(
	w http.ResponseWriter, r *http.Request, maxBody int64, tooLarge func(),
	readPart func(name string, part io.Reader) bool,
) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		s.invalidRequest(w, "Content-Type must be multipart/form-data.")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	reader, err := r.MultipartReader()
	if err != nil {
		s.invalidRequest(w, "The request body is not valid multipart/form-data.")
		return false
	}
	seen := map[string]bool{}
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			return true
		}
		var bodyTooLarge *http.MaxBytesError
		if errors.As(err, &bodyTooLarge) && tooLarge != nil {
			tooLarge()
			return false
		}
		if err != nil {
			s.invalidRequest(w, "The request body is not valid multipart/form-data.")
			return false
		}
		name := part.FormName()
		if seen[name] {
			s.invalidRequest(w, "Each part may be sent only once.")
			return false
		}
		seen[name] = true
		if !readPart(name, part) {
			return false
		}
	}
}

// readThumbnailEditPart は multipart の部分 part を名前 name に応じて input へ読む。
func (s *server) readThumbnailEditPart(w http.ResponseWriter, name string, part io.Reader, input *domain.ThumbnailEditRunInput) bool {
	switch name {
	case "prompt":
		// 1 文字は UTF-8 で 4 バイトまでなので、上限の文字数の 4 倍まで読めば判定できる。
		text, ok := readPart(part, domain.ThumbnailEditMaxInstruction*4)
		if !ok || !utf8.Valid(text) || utf8.RuneCount(text) > domain.ThumbnailEditMaxInstruction {
			s.invalidRequest(w, "prompt must be valid text of at most 4000 characters.")
			return false
		}
		input.Instruction = string(text)
	case "mask":
		mask, ok := readPart(part, thumbnailEditMaxMaskBytes)
		if !ok {
			s.invalidRequestReason(w, reasonThumbnailEditMaskInvalid, "The mask must be a PNG of the frame's size.")
			return false
		}
		input.Mask = mask
	case "reference":
		reference, ok := readPart(part, domain.ThumbnailEditMaxReferenceBytes)
		if !ok {
			s.invalidRequestReason(w, reasonThumbnailEditReferenceInvalid, "The reference image must be a PNG, JPEG or WebP image of at most 20 MiB.")
			return false
		}
		input.Reference = reference
	default:
		s.invalidRequest(w, "Only prompt, mask and reference may be sent.")
		return false
	}
	return true
}

// readPart は part を最大 limit バイトまで読む。越えるか読めなければ ok が false である。
// 空の部分も nil でない空の値として返す。
func readPart(part io.Reader, limit int64) ([]byte, bool) {
	data, err := io.ReadAll(io.LimitReader(part, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, false
	}
	if data == nil {
		data = []byte{}
	}
	return data, true
}

// thumbnailEditFailure は編集のセッションの誤りを応答にする。編集に固有でない誤りは 500 である。
func (s *server) thumbnailEditFailure(w http.ResponseWriter, err error) {
	if !s.thumbnailEditError(w, err) {
		s.internalError(w, "Could not handle the thumbnail edit.", err)
	}
}

// thumbnailEditError は編集に固有の誤りを契約の応答にし、書いたかを返す
// （specs/045-comfyui-thumbnails/contracts/screen-api.md）。提供元の文は message に入れない。
func (s *server) thumbnailEditError(w http.ResponseWriter, err error) bool {
	var failure *domain.ImageEditError
	switch {
	case errors.Is(err, domain.ErrThumbnailEditNotFound):
		s.notFoundReason(w, reasonThumbnailEditNotFound, "The thumbnail edit has ended.")
	case errors.Is(err, domain.ErrThumbnailEditNotReady):
		s.conflictReason(w, reasonThumbnailEditNotReady, "No ComfyUI connection is saved.")
	case errors.Is(err, domain.ErrThumbnailEditMaskInvalid):
		s.invalidRequestReason(w, reasonThumbnailEditMaskInvalid, "The mask must be a PNG of the frame's size.")
	case errors.Is(err, domain.ErrThumbnailEditReferenceInvalid):
		s.invalidRequestReason(w, reasonThumbnailEditReferenceInvalid, "The reference image must be a PNG, JPEG or WebP image of at most 20 MiB.")
	case errors.Is(err, domain.ErrThumbnailEditNoResult):
		s.conflictReason(w, reasonThumbnailEditNoResult, "The thumbnail edit has no generated image.")
	case errors.Is(err, domain.ErrContentChanged):
		s.conflictReason(w, reasonContentChanged, "The video is gone or now holds other content.")
	case errors.Is(err, domain.ErrThumbnailImageInvalid):
		s.conflictReason(w, reasonThumbnailImageInvalid, "The generated image cannot be decoded.")
	case errors.As(err, &failure):
		s.writeReasonError(w, http.StatusBadGateway, codeProviderFailed, gen.ErrorReason(failure.Reason),
			"ComfyUI did not accept the run.")
	default:
		return false
	}
	return true
}

// writeThumbnailEdit はセッションの今の状態を status で書く。
func (s *server) writeThumbnailEdit(w http.ResponseWriter, status int, edit domain.ThumbnailEdit) {
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, status, thumbnailEditBody(edit), s.logger)
}

// writeImage は画像 data を content type contentType で書く。中身はセッションとともに変わるので
// キャッシュさせない。
func (s *server) writeImage(w http.ResponseWriter, contentType string, data []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", cacheNoStore)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		s.logger.Debug("could not write the image", "error", err)
	}
}

// thumbnailEditBody はセッションを応答の形にする。
func thumbnailEditBody(edit domain.ThumbnailEdit) gen.ThumbnailEdit {
	base := "/api/thumbnail-edits/" + edit.ID
	body := gen.ThumbnailEdit{
		Id: edit.ID, VideoId: edit.VideoID, State: gen.ThumbnailEditState(edit.State),
		Source: gen.ThumbnailEditSource(edit.Source), PositionMs: edit.PositionMs,
		FrameUrl: base + "/frame", FrameWidth: edit.FrameWidth, FrameHeight: edit.FrameHeight,
	}
	if edit.Elapsed != nil {
		elapsed := edit.Elapsed.Milliseconds()
		body.ElapsedMs = &elapsed
	}
	if edit.Progress != nil {
		body.Progress = &gen.ThumbnailEditProgress{Value: edit.Progress.Value, Max: edit.Progress.Max}
	}
	if edit.Error != nil {
		failure := gen.ThumbnailEditError{Reason: gen.ThumbnailEditFailure(edit.Error.Reason)}
		if edit.Error.Message != "" {
			message := edit.Error.Message
			failure.Message = &message
		}
		body.Error = &failure
	}
	if edit.HasResult {
		result := base + "/result"
		body.ResultUrl = &result
	}
	return body
}
