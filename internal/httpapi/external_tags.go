package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
)

// 外部連携 API のタグの統合・改名・シノニム・まとめての確定・却下・削除・却下した名前
// （specs/039-external-tag-admin/contracts/external-api.md §2〜§7）。どの操作も画面の経路と同じ TagStore の操作を Tags 越しに呼び、新しい書き込みの経路は
// 持たない（research.md R-6）。タグは経路ではなく本文の id で指す（R-2）。ここが持つのは本文の
// 検査と、外部連携 API の誤りの形（tagId・tagName を添えた 409 など、R-4）への写し方だけである。

// MergeTags は統合元のタグを統合先へ 1 つのトランザクションで統合する（POST /api/v1/tags/merge）。
// 無い統合元は飛ばして notFoundIds に返す。
func (e *externalServer) MergeTags(w http.ResponseWriter, r *http.Request) {
	var body extgen.TagMergeRequest
	if !e.readJSONBody(w, r, &body) {
		return
	}
	if len(body.SourceIds) == 0 || len(body.SourceIds) > domain.MaxTagBatch {
		e.invalidRequestLimit(w, extgen.TooManyTags, domain.MaxTagBatch,
			fmt.Sprintf("sourceIds must contain between 1 and %d items.", domain.MaxTagBatch))
		return
	}
	// 保存先は統合元に混じった統合先を黙って飛ばすが、画面の経路と同じく要求の誤りとして
	// 先に断る（specs/036-tag-admin-scale/contracts/screen-api.md §2）。
	if slices.Contains(body.SourceIds, body.TargetId) {
		reason := extgen.MergeSameTag
		e.invalidRequest(w, &reason, "sourceIds must not contain targetId. Nothing was changed.")
		return
	}
	if e.s.tags == nil {
		e.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	outcome, err := e.s.tags.MergeTags(r.Context(), body.TargetId, body.SourceIds)
	if err != nil {
		e.writeTagError(w, err, "Could not merge the tags.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, extgen.TagMergeResponse{
		Tag:         toExternalTag(outcome.Tag),
		NotFoundIds: nonNilIDs(outcome.NotFoundIDs),
	}, e.s.logger)
}

// RenameTag はタグの元の名前を変える（POST /api/v1/tags/rename）。名前が変われば仮のタグは
// 確定する。
func (e *externalServer) RenameTag(w http.ResponseWriter, r *http.Request) {
	var body extgen.TagRenameRequest
	if !e.readJSONBody(w, r, &body) {
		return
	}
	if e.s.tags == nil {
		e.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	tag, err := e.s.tags.RenameTag(r.Context(), body.Id, body.Name)
	if err != nil {
		e.writeTagError(w, err, "Could not rename the tag.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toExternalTag(tag), e.s.logger)
}

// UpdateTagSynonyms はタグのシノニムを足す・外す（POST /api/v1/tags/synonyms）。どちらも変更後の
// タグを返す（research.md R-5）。
func (e *externalServer) UpdateTagSynonyms(w http.ResponseWriter, r *http.Request) {
	var body extgen.TagSynonymsRequest
	if !e.readJSONBody(w, r, &body) {
		return
	}
	if !body.Action.Valid() {
		e.invalidRequest(w, nil, "action must be add or remove.")
		return
	}
	if e.s.tags == nil {
		e.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	var tag domain.Tag
	var err error
	if body.Action == extgen.TagSynonymsRequestActionAdd {
		tag, err = e.s.tags.AddSynonym(r.Context(), body.Id, body.Name, body.MergeTagId)
	} else {
		// 画面の DELETE と同じく、登録時と同じ整え方にそろえて照合する。整えられない名前は
		// どのタグのシノニムにもなりえないので、そのまま渡して何にも一致させない。
		name, normalizeErr := domain.NormalizeTagName(body.Name)
		if normalizeErr != nil {
			name = body.Name
		}
		tag, err = e.s.tags.RemoveSynonym(r.Context(), body.Id, name)
	}
	if err != nil {
		e.writeTagError(w, err, "Could not update the synonyms.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toExternalTag(tag), e.s.logger)
}

// BatchTags は複数のタグをまとめて確定・却下・削除する（POST /api/v1/tags/batch、
// contracts/external-api.md §5）。1 つのタグ用の経路は持たない（research.md R-3）。種類の合わない
// タグと無いタグは何も変えずに返し、残りを 1 つのトランザクションで処理する。
func (e *externalServer) BatchTags(w http.ResponseWriter, r *http.Request) {
	var body extgen.TagBatchRequest
	if !e.readJSONBody(w, r, &body) {
		return
	}
	if !body.Action.Valid() {
		e.invalidRequest(w, nil, "action must be one of confirm, reject or delete.")
		return
	}
	if len(body.Ids) == 0 || len(body.Ids) > domain.MaxTagBatch {
		e.invalidRequestLimit(w, extgen.TooManyTags, domain.MaxTagBatch,
			fmt.Sprintf("ids must contain between 1 and %d items.", domain.MaxTagBatch))
		return
	}
	if e.s.tags == nil {
		e.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	outcome, err := e.s.tags.BatchTags(r.Context(), domain.TagBatchAction(body.Action), body.Ids)
	if err != nil {
		e.internalError(w, "Could not update the tags. Nothing was changed.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, extgen.TagBatchResponse{
		AppliedIds:       nonNilIDs(outcome.AppliedIDs),
		NotFoundIds:      nonNilIDs(outcome.NotFoundIDs),
		NotApplicableIds: nonNilIDs(outcome.NotApplicableIDs),
	}, e.s.logger)
}

// ListRejectedTagNames は却下した名前を名前の自然順でページに分けて返す
// （GET /api/v1/tags/rejected-names、contracts/external-api.md §6）。limit の既定と上限は画面と同じ。
func (e *externalServer) ListRejectedTagNames(w http.ResponseWriter, r *http.Request, params extgen.ListRejectedTagNamesParams) {
	limit := rejectedTagNamePageDefaultLimit
	if params.Limit != nil {
		limit = *params.Limit
		if limit < 1 || limit > domain.MaxTagPageLimit {
			e.invalidRequest(w, nil, fmt.Sprintf("limit must be between 1 and %d.", domain.MaxTagPageLimit))
			return
		}
	}
	cursor := ""
	if params.Cursor != nil {
		cursor = *params.Cursor
	}
	if e.s.tags == nil {
		e.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	page, err := e.s.tags.ListRejectedTagNames(r.Context(), cursor, limit)
	if errors.Is(err, domain.ErrInvalidCursor) {
		reason := extgen.InvalidCursor
		e.invalidRequest(w, &reason, "Cannot read the cursor. Read the rejected names again from the start.")
		return
	}
	if err != nil {
		e.internalError(w, "Could not load rejected tag names.", err)
		return
	}
	names := page.Items
	if names == nil {
		names = []string{}
	}
	body := extgen.RejectedTagNameList{Items: names, Total: page.Total}
	if page.NextCursor != "" {
		body.NextCursor = &page.NextCursor
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, body, e.s.logger)
}

// ForgetRejectedTagName は名前を却下した名前から外す（DELETE /api/v1/tags/rejected-names?name=…、
// contracts/external-api.md §7）。応答の name は照合した綴りで、保存先と同じく
// domain.NormalizeTagName で整えた名前、整えられなければ送られた名前である。一覧に無い名前は
// 何も変えずに removed: false を返す（research.md R-5）。
func (e *externalServer) ForgetRejectedTagName(w http.ResponseWriter, r *http.Request, params extgen.ForgetRejectedTagNameParams) {
	if e.s.tags == nil {
		e.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	name := params.Name
	if normalized, err := domain.NormalizeTagName(name); err == nil {
		name = normalized
	}
	removed, err := e.s.tags.ForgetRejectedTagName(r.Context(), name)
	if err != nil {
		e.internalError(w, "Could not update rejected tag names.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, extgen.RejectedTagNameForgetResult{Name: name, Removed: removed}, e.s.logger)
}

// writeTagError はタグの操作の保存先の誤りを外部連携 API の誤りへ写す（contracts/external-api.md
// §2〜§4）。名前の衝突は、ぶつかったタグの id と元の名前を tagId・tagName に添えて返し、呼び手が
// 一覧を読み直さずに mergeTagId を送れるようにする（research.md R-4）。
func (e *externalServer) writeTagError(w http.ResponseWriter, err error, failure string) {
	var invalid *domain.InvalidTagNameError
	var nameConflict *domain.TagNameConflict
	var mergeRequired *domain.TagMergeRequired
	switch {
	case errors.As(err, &invalid):
		e.writeInvalidTagName(w, invalid)
	case errors.Is(err, domain.ErrInvalidTagName):
		e.invalidRequest(w, nil, "The tag name cannot be used.")
	case errors.Is(err, domain.ErrTagNotFound):
		reason := extgen.TagNotFound
		writeExternalError(w, e.s, http.StatusNotFound, extgen.Error{
			Code: extgen.ErrorCodeNotFound, Reason: &reason, Message: "Tag not found.",
		})
	case errors.As(err, &nameConflict):
		e.writeTagConflict(w, extgen.TagNameTaken, nameConflict.Tag,
			fmt.Sprintf("The name is already used by the tag %q. Nothing was changed.", nameConflict.Tag.Name))
	case errors.As(err, &mergeRequired):
		e.writeTagConflict(w, extgen.TagMergeRequired, mergeRequired.Tag,
			fmt.Sprintf("%q is the name of another tag. Send mergeTagId %d to merge that tag into this one. Nothing was changed.",
				mergeRequired.Tag.Name, mergeRequired.Tag.ID))
	default:
		e.internalError(w, failure, err)
	}
}

// writeTagConflict は 409 conflict を、ぶつかったタグ owner の id と元の名前（翻訳しない）を
// 添えて返す。
func (e *externalServer) writeTagConflict(w http.ResponseWriter, reason extgen.ErrorReason, owner domain.TagRef, message string) {
	tagID, tagName := owner.ID, owner.Name
	writeExternalError(w, e.s, http.StatusConflict, extgen.Error{
		Code: extgen.ErrorCodeConflict, Reason: &reason, TagId: &tagID, TagName: &tagName, Message: message,
	})
}

// writeInvalidTagName は名前の規則違反を理由ごとの reason の 400 invalid_request へ写す。
func (e *externalServer) writeInvalidTagName(w http.ResponseWriter, invalid *domain.InvalidTagNameError) {
	switch invalid.Problem {
	case domain.TagNameEmpty:
		reason := extgen.TagNameEmpty
		e.invalidRequest(w, &reason, "Enter a tag name.")
	case domain.TagNameControlCharacters:
		reason := extgen.TagNameControlCharacters
		e.invalidRequest(w, &reason, "Tag names cannot contain control characters.")
	case domain.TagNameTooLong:
		e.invalidRequestLimit(w, extgen.TagNameTooLong, domain.TagNameMaxLength,
			"Tag names must be at most "+strconv.Itoa(domain.TagNameMaxLength)+" characters.")
	default:
		e.invalidRequest(w, nil, "The tag name cannot be used.")
	}
}
