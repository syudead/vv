package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// タグの管理API（specs/014-video-tags/contracts/tags-api.md §1〜§3）。
//
// internal/app は通さない。どの操作も
// internal/store.TagStore の1つのトランザクションで済み、httpapi は要求の
// 解釈と契約の形への変換だけを持つ。

func (s *server) ListTags(w http.ResponseWriter, r *http.Request) {
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	tags, err := s.tags.ListTags(r.Context())
	if err != nil {
		s.internalError(w, "Could not load tags.", err)
		return
	}
	out := make([]gen.Tag, 0, len(tags))
	for _, tag := range tags {
		out = append(out, toAPITag(tag))
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.TagList{Items: out}, s.logger)
}

func (s *server) CreateTag(w http.ResponseWriter, r *http.Request) {
	var body gen.CreateTagRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	tag, err := s.tags.CreateTag(r.Context(), body.Name)
	if err != nil {
		s.writeTagError(w, err, normalizedTagNameOrRaw(body.Name))
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusCreated, toAPITag(tag), s.logger)
}

func (s *server) RenameTag(w http.ResponseWriter, r *http.Request, id gen.TagId) {
	var body gen.RenameTagRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	tag, err := s.tags.RenameTag(r.Context(), id, body.Name)
	if err != nil {
		s.writeTagError(w, err, normalizedTagNameOrRaw(body.Name))
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPITag(tag), s.logger)
}

func (s *server) DeleteTag(w http.ResponseWriter, r *http.Request, id gen.TagId) {
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	if err := s.tags.DeleteTag(r.Context(), id); err != nil {
		s.writeTagError(w, err, "")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) MergeTag(w http.ResponseWriter, r *http.Request, id gen.TagId) {
	var body gen.MergeTagRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	// store.MergeTag はtarget==sourceを何も変えない黙った成功にする
	// （data-model.mdの統合の規則、#264 レビューの持ち越し）。APIはそれより先に
	// 400 invalid_requestにする — 統合は違う2つのタグを1つにする操作であり、
	// 同じidを送るのは要求の誤りだからである。
	if body.SourceId == id {
		s.invalidRequestReason(w, reasonMergeSameTag, "The source and target of a merge must be different tags.")
		return
	}
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	tag, err := s.tags.MergeTag(r.Context(), id, body.SourceId)
	if err != nil {
		s.writeTagError(w, err, "")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPITag(tag), s.logger)
}

func (s *server) AddTagSynonym(w http.ResponseWriter, r *http.Request, id gen.TagId) {
	var body gen.AddTagSynonymRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	tag, err := s.tags.AddSynonym(r.Context(), id, body.Name, body.MergeTagId)
	if err != nil {
		s.writeTagError(w, err, normalizedTagNameOrRaw(body.Name))
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPITag(tag), s.logger)
}

func (s *server) RemoveTagSynonym(w http.ResponseWriter, r *http.Request, id gen.TagId, params gen.RemoveTagSynonymParams) {
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	// store.RemoveSynonymは受け取ったnameをそのまま照合する。前後の空白などで
	// 登録時と食い違うと黙って何も変えないので、登録時と同じ整え方
	// （domain.NormalizeTagName）にそろえてから渡す（#264 レビューの持ち越し）。
	// 整えられない入力（空・制御文字・上限超）は、そもそもどのタグのシノニムにも
	// なりえないので、整える前の文字列のまま渡す。何にも一致しないだけで、タグ
	// 自体が無ければ RemoveSynonym が domain.ErrTagNotFound を返す。
	name, err := domain.NormalizeTagName(params.Name)
	if err != nil {
		name = params.Name
	}
	if err := s.tags.RemoveSynonym(r.Context(), id, name); err != nil {
		s.writeTagError(w, err, "")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

// ConfirmTag は仮のタグを確定する（POST /api/tags/{id}/confirm、
// specs/031-tentative-tags/contracts/screen-api.md §2）。既に確定したタグでも
// 何も変えずに今の状態を返す。
func (s *server) ConfirmTag(w http.ResponseWriter, r *http.Request, id gen.TagId) {
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	tag, err := s.tags.ConfirmTag(r.Context(), id)
	if err != nil {
		s.writeTagError(w, err, "")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPITag(tag), s.logger)
}

// RejectTag は仮のタグを却下する（POST /api/tags/{id}/reject、
// specs/031-tentative-tags/contracts/screen-api.md §2）。確定したタグは
// 409 tag_not_tentative で何も変えない。却下した名前は応答に載せず、画面は
// 却下した名前の一覧を取り直す。
func (s *server) RejectTag(w http.ResponseWriter, r *http.Request, id gen.TagId) {
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	if _, err := s.tags.RejectTag(r.Context(), id); err != nil {
		s.writeTagError(w, err, "")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

// ListRejectedTagNames は却下した名前を名前の自然順で返す
// （GET /api/tags/rejected-names、specs/031-tentative-tags/contracts/screen-api.md §3）。
func (s *server) ListRejectedTagNames(w http.ResponseWriter, r *http.Request) {
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	names, err := s.tags.ListRejectedTagNames(r.Context())
	if err != nil {
		s.internalError(w, "Could not load rejected tag names.", err)
		return
	}
	if names == nil {
		names = []string{}
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.RejectedTagNameList{Items: names}, s.logger)
}

// ForgetRejectedTagName は名前を却下した名前の一覧から外す
// （DELETE /api/tags/rejected-names?name=…、specs/031-tentative-tags/contracts/screen-api.md §3）。
// 一覧に無い名前や整えられない入力でも、何も変えずに 204 を返す。整え方は
// store.ForgetRejectedTagName が登録時と同じ domain.NormalizeTagName で行う。
func (s *server) ForgetRejectedTagName(w http.ResponseWriter, r *http.Request, params gen.ForgetRejectedTagNameParams) {
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	if err := s.tags.ForgetRejectedTagName(r.Context(), params.Name); err != nil {
		s.internalError(w, "Could not update rejected tag names.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

func toAPITag(tag domain.Tag) gen.Tag {
	synonyms := tag.Synonyms
	if synonyms == nil {
		synonyms = []string{}
	}
	return gen.Tag{
		Id: tag.ID, Name: tag.Name, Synonyms: synonyms, VideoCount: tag.VideoCount, Tentative: tag.Tentative,
		CreatedAt: tag.CreatedAt,
	}
}

// toAPITagRef は動画に付いたタグ1件を応答の形にする。仮かどうかも載せる
// （specs/031-tentative-tags/contracts/screen-api.md §0）。
func toAPITagRef(ref domain.TagRef) gen.TagRef {
	return gen.TagRef{Id: ref.ID, Name: ref.Name, Tentative: ref.Tentative}
}

// toAPIVideoTag は動画に付いたタグ1件を出所つきで応答の形にする。フォルダ由来だけで
// 付いているタグも、そのタグの仮かどうかを出す（contracts/screen-api.md §0）。
func toAPIVideoTag(tag domain.VideoTag) gen.VideoTag {
	return gen.VideoTag{
		Id: tag.ID, Name: tag.Name, Manual: tag.Manual, FromFolder: tag.FromFolder, Tentative: tag.Tentative,
	}
}

// normalizedTagNameOrRaw は writeTagNameTaken の比較に使う、要求で送られた
// 名前の整えた形を返す。domain.TagNameConflict.Tag.Name は常に整えた形
// （domain.NormalizeTagName の結果）で保存されているので、比較する側もそろえる
// 必要がある。整えられない入力（空・制御文字・上限超）は、その時点で
// domain.ErrInvalidTagName になり TagNameConflict は返らないので、ここでの
// フォールバックは使われない。
func normalizedTagNameOrRaw(name string) string {
	normalized, err := domain.NormalizeTagName(name)
	if err != nil {
		return name
	}
	return normalized
}

func (s *server) writeTagError(w http.ResponseWriter, err error, submittedName string) {
	var nameConflict *domain.TagNameConflict
	var mergeRequired *domain.TagMergeRequired
	switch {
	case errors.Is(err, domain.ErrInvalidTagName):
		s.invalidTagName(w, err, "")
	case errors.Is(err, domain.ErrTagNotFound):
		s.writeError(w, http.StatusNotFound, codeTagNotFound, "Tag not found.")
	case errors.Is(err, domain.ErrTagNotTentative):
		s.writeError(w, http.StatusConflict, codeTagNotTentative, "The tag is already confirmed.")
	case errors.As(err, &nameConflict):
		s.writeTagNameTaken(w, submittedName, nameConflict.Tag)
	case errors.As(err, &mergeRequired):
		tagName := mergeRequired.Tag.Name
		s.writeErrorBody(w, http.StatusConflict, gen.Error{
			Code:    codeTagMergeRequired,
			TagName: &tagName,
			Message: fmt.Sprintf("%q is already the name of another tag. Confirm the tag to merge.", tagName),
		})
	default:
		s.internalError(w, "Could not update the tag.", err)
	}
}

// writeTagNameTaken は tag_name_taken を返す。owner はその名前を既に持つタグ
// （元の名前としてでもシノニムとしてでも。domain.TagNameConflict.Tag）で、Name は
// 常にそのタグの元の名前である（contracts/tags-api.md §2、親 Issue の Edge Case
// 「シノニム名の衝突」）。submittedName が owner.Name と一致すれば、それはその
// タグ自身の元の名前（自分のシノニムとして自分の元の名前を送った場合を含む）で
// reason は name_is_tag、一致しなければ owner の既存のシノニムで name_is_synonym
// になる。tagName はどちらも owner の元の名前を翻訳せずに返す
// （specs/023-english-i18n/contracts/error-api.md §1）。
func (s *server) writeTagNameTaken(w http.ResponseWriter, submittedName string, owner domain.TagRef) {
	tagName := owner.Name
	body := gen.Error{Code: codeTagNameTaken, TagName: &tagName}
	var reason gen.ErrorReason
	if submittedName == owner.Name {
		reason = reasonNameIsTag
		body.Message = fmt.Sprintf("A tag named %q already exists.", owner.Name)
	} else {
		reason = reasonNameIsSynonym
		body.Message = fmt.Sprintf("%q is already a synonym of %q.", submittedName, owner.Name)
	}
	body.Reason = &reason
	s.writeErrorBody(w, http.StatusConflict, body)
}

// invalidTagName は domain.NormalizeTagName の失敗を、その理由に応じた reason の
// invalid_request へ写す（specs/023-english-i18n/contracts/error-api.md §1）。
// prefix は message の前に置く英語の説明で、要らなければ空にする。
func (s *server) invalidTagName(w http.ResponseWriter, err error, prefix string) {
	var invalid *domain.InvalidTagNameError
	if !errors.As(err, &invalid) {
		s.invalidRequest(w, prefix+"The tag name cannot be used.")
		return
	}
	switch invalid.Problem {
	case domain.TagNameEmpty:
		s.invalidRequestReason(w, reasonTagNameEmpty, prefix+"Enter a tag name.")
	case domain.TagNameControlCharacters:
		s.invalidRequestReason(w, reasonTagNameControlCharacters, prefix+"Tag names cannot contain control characters.")
	case domain.TagNameTooLong:
		s.invalidRequestLimit(w, reasonTagNameTooLong, domain.TagNameMaxLength,
			prefix+"Tag names must be at most "+strconv.Itoa(domain.TagNameMaxLength)+" characters.")
	default:
		s.invalidRequest(w, prefix+"The tag name cannot be used.")
	}
}
