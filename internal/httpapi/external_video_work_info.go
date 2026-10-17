package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 外部連携 API の作品情報の一括操作。動画の指定・index・「全件を確かめてから反映する」
// 形は POST /api/v1/video-display-names と同じにし、項目の workInfo の読み方と誤りの写し方は画面の
// PUT /api/videos/{id}/work-info と共有する（workInfoFromInput・workInfoFailureOf）。

// maxVideoWorkInfo は POST /api/v1/video-work-info の items の上限である。
const maxVideoWorkInfo = 20000

// workInfoItem は本文の要素の JSON の形である。workInfo は画面の API と同じ形の gen.WorkInfoInput として
// 後で読む。生成型では失われる一覧の件の必須のプロパティの有無を、JSON のまま確かめるためである。
type workInfoItem struct {
	Video    extgen.VideoRef `json:"video"`
	WorkInfo json.RawMessage `json:"workInfo"`
}

// UpdateVideoWorkInfo は複数の動画の作品情報をまるごと置き換える（POST /api/v1/video-work-info）。
// 全件を 1 つの取引で行う保存先（WorkInfoStore.SetMany）に任せ、internal/app は通さない。
func (e *externalServer) UpdateVideoWorkInfo(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []workInfoItem `json:"items"`
	}
	if !e.readJSONBody(w, r, &body) {
		return
	}
	if len(body.Items) < 1 || len(body.Items) > maxVideoWorkInfo {
		e.invalidRequestLimit(w, extgen.TooManyVideos, maxVideoWorkInfo,
			fmt.Sprintf("items must contain between 1 and %d items.", maxVideoWorkInfo))
		return
	}
	changes := make([]domain.WorkInfoChange, 0, len(body.Items))
	for i, item := range body.Items {
		change, message, ok := workInfoChangeOf(item)
		if !ok {
			e.invalidItem(w, i, "Item "+strconv.Itoa(i)+": "+message)
			return
		}
		changes = append(changes, change)
	}
	if e.s.workInfo == nil {
		e.internalError(w, "Work information storage is not configured.", nil)
		return
	}

	videos, err := e.s.workInfo.SetMany(r.Context(), changes)
	if err != nil {
		e.writeWorkInfoItemsError(w, err)
		return
	}
	out := extgen.VideoWorkInfoResponse{Items: make([]extgen.VideoWorkInfoItem, 0, len(videos))}
	for _, video := range videos {
		out.Items = append(out.Items, extgen.VideoWorkInfoItem{
			Video:    extgen.VideoTagsVideo{Id: video.ID, ContentKey: video.ContentKey},
			WorkInfo: toExternalWorkInfo(video.WorkInfo),
		})
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, e.s.logger)
}

// workInfoChangeOf は本文の要素 1 件を domain の形にする。形の誤りなら文言と false を返す。
func workInfoChangeOf(item workInfoItem) (domain.WorkInfoChange, string, bool) {
	ref, ok := externalVideoRef(item.Video)
	if !ok {
		return domain.WorkInfoChange{}, "Each video must have exactly one of id, contentKey and path.", false
	}
	raw := bytes.TrimSpace(item.WorkInfo)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return domain.WorkInfoChange{}, "workInfo is required.", false
	}
	var input gen.WorkInfoInput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return domain.WorkInfoChange{}, "workInfo must be a work information object: " + err.Error(), false
	}
	info, message, ok := workInfoFromInput(input, raw)
	if !ok {
		return domain.WorkInfoChange{}, message, false
	}
	return domain.WorkInfoChange{Video: ref, WorkInfo: info}, "", true
}

// writeWorkInfoItemsError は一括操作の保存先の誤りを応答へ写す。項目の規則違反と無い人物・団体は画面と
// 同じ workInfoFailureOf で reason・field・limit にし、index に items の位置を添える。
func (e *externalServer) writeWorkInfoItemsError(w http.ResponseWriter, err error) {
	var notFound *domain.VideoRefNotFoundError
	if errors.As(err, &notFound) {
		e.videoNotFoundAt(w, notFound.Index, "Nothing was changed.")
		return
	}
	var at *domain.WorkInfoAtError
	if errors.As(err, &at) {
		if failure, ok := workInfoFailureOf(at.Err); ok {
			failure.message = "Item " + strconv.Itoa(at.Index) + ": " + failure.message + " Nothing was changed."
			e.writeWorkInfoFailure(w, failure, &at.Index)
			return
		}
	}
	e.internalError(w, "Could not update the work information. Nothing was changed.", err)
}

// writeWorkInfoFailure は外部連携 API の Error として failure を書き出す。index は一括操作の items の
// 位置で、無ければ nil。
func (e *externalServer) writeWorkInfoFailure(w http.ResponseWriter, failure workInfoFailure, index *int) {
	reason := extgen.ErrorReason(failure.reason)
	body := extgen.Error{Code: extgen.ErrorCode(failure.code), Reason: &reason, Index: index, Message: failure.message}
	if failure.field != "" {
		field := failure.field
		body.Field = &field
	}
	if failure.limit > 0 {
		limit := failure.limit
		body.Limit = &limit
	}
	writeExternalError(w, e.s, failure.status, body)
}

// toExternalWorkInfo は作品情報を外部連携 API の WorkInfo にする。画面の API と違って行の無い動画も
// 省かず、未設定の code・description は null、空の一覧は [] で書く。
func toExternalWorkInfo(info *domain.WorkInfo) extgen.WorkInfo {
	var value domain.WorkInfo
	if info != nil {
		value = *info
	}
	out := extgen.WorkInfo{
		Code: optionalString(value.Code), Description: optionalString(value.Description),
		Dates: make([]extgen.WorkDate, len(value.Dates)), People: make([]extgen.WorkPerson, len(value.People)),
		Organizations: make([]extgen.WorkOrganization, len(value.Organizations)),
		Urls:          append([]string{}, value.URLs...), ExternalIds: toExternalExternalIDs(value.ExternalIDs),
	}
	for i, date := range value.Dates {
		out.Dates[i] = extgen.WorkDate{Kind: extgen.WorkDateKind(date.Kind), Date: date.Date}
	}
	for i, link := range value.People {
		out.People[i].Role = extgen.WorkPersonRole(link.Role)
		out.People[i].Person = toExternalCreditRef(link.Person)
	}
	for i, link := range value.Organizations {
		out.Organizations[i].Role = extgen.WorkOrganizationRole(link.Role)
		out.Organizations[i].Organization = toExternalCreditRef(link.Organization)
	}
	return out
}

func toExternalCreditRef(ref domain.CreditRef) extgen.CreditRef {
	return extgen.CreditRef{Id: ref.ID, Name: ref.Name}
}
