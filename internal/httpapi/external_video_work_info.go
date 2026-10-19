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

// Batch work-info updates share indexed atomic validation with display-name updates and conversions/errors with the UI route.

const maxVideoWorkInfo = 20000

// Keep raw JSON until required nested field presence is checked; generated types would lose it.
type workInfoItem struct {
	Video    extgen.VideoRef `json:"video"`
	WorkInfo json.RawMessage `json:"workInfo"`
}

// WorkInfoStore.SetMany replaces all requested values atomically.
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

// External responses always include workInfo, using null unset scalars and [] empty lists even without a stored row.
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
		out.People[i].Provisional = link.Person.Provisional
	}
	for i, link := range value.Organizations {
		out.Organizations[i].Role = extgen.WorkOrganizationRole(link.Role)
		out.Organizations[i].Organization = toExternalCreditRef(link.Organization)
	}
	return out
}

func toExternalCreditRef(ref domain.CreditRef) extgen.CreditRef {
	return extgen.CreditRef{Id: ref.ID, Name: ref.Name, Provisional: ref.Provisional}
}
