package httpapi

import (
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// タグの管理API（specs/014-video-tags/contracts/tags-api.md §1〜§3）。
//
// internal/app は通さない（Plan の Structural Decisions 14）。どの操作も
// internal/store.TagStore の1つのトランザクションで済み、httpapi は要求の
// 解釈と契約の形への変換だけを持つ。

func (s *server) ListTags(w http.ResponseWriter, r *http.Request) {
	if s.tags == nil {
		s.internalError(w, "タグの経路が設定されていません", nil)
		return
	}
	tags, err := s.tags.ListTags(r.Context())
	if err != nil {
		s.internalError(w, "タグを取得できませんでした", err)
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
		s.internalError(w, "タグの経路が設定されていません", nil)
		return
	}
	tag, err := s.tags.CreateTag(r.Context(), body.Name)
	if err != nil {
		s.writeTagError(w, err)
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
		s.internalError(w, "タグの経路が設定されていません", nil)
		return
	}
	tag, err := s.tags.RenameTag(r.Context(), id, body.Name)
	if err != nil {
		s.writeTagError(w, err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPITag(tag), s.logger)
}

func (s *server) DeleteTag(w http.ResponseWriter, r *http.Request, id gen.TagId) {
	if s.tags == nil {
		s.internalError(w, "タグの経路が設定されていません", nil)
		return
	}
	if err := s.tags.DeleteTag(r.Context(), id); err != nil {
		s.writeTagError(w, err)
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
		s.invalidRequest(w, "統合元と統合先に同じタグは指定できません")
		return
	}
	if s.tags == nil {
		s.internalError(w, "タグの経路が設定されていません", nil)
		return
	}
	tag, err := s.tags.MergeTag(r.Context(), id, body.SourceId)
	if err != nil {
		s.writeTagError(w, err)
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
		s.internalError(w, "タグの経路が設定されていません", nil)
		return
	}
	tag, err := s.tags.AddSynonym(r.Context(), id, body.Name, body.MergeTagId)
	if err != nil {
		s.writeTagError(w, err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPITag(tag), s.logger)
}

func (s *server) RemoveTagSynonym(w http.ResponseWriter, r *http.Request, id gen.TagId, params gen.RemoveTagSynonymParams) {
	if s.tags == nil {
		s.internalError(w, "タグの経路が設定されていません", nil)
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
		s.writeTagError(w, err)
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
		Id: tag.ID, Name: tag.Name, Synonyms: synonyms, VideoCount: tag.VideoCount,
	}
}

func (s *server) writeTagError(w http.ResponseWriter, err error) {
	var nameConflict *domain.TagNameConflict
	var mergeRequired *domain.TagMergeRequired
	switch {
	case errors.Is(err, domain.ErrInvalidTagName):
		s.invalidRequest(w, err.Error())
	case errors.Is(err, domain.ErrTagNotFound):
		s.writeError(w, http.StatusNotFound, codeTagNotFound, "タグが見つかりません")
	case errors.As(err, &nameConflict):
		s.writeError(w, http.StatusConflict, codeTagNameTaken,
			"「"+nameConflict.Tag.Name+"」という名前のタグ（またはそのシノニム）が既にあります")
	case errors.As(err, &mergeRequired):
		s.writeError(w, http.StatusConflict, codeTagMergeRequired,
			"「"+mergeRequired.Tag.Name+"」は既に別のタグの名前です。統合するタグを確かめてください")
	default:
		s.internalError(w, "タグを変更できませんでした", err)
	}
}
