package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 作品情報プラグインの検索・取得と、取得の結果の反映の経路
// （specs/049-work-info-plugins/contracts/screen-api.md）。どれも所有者だけで、accessRoutes に載せない。
// 実行・規則・保存は internal/app と internal/store が持ち、ここは要求の解釈と契約の形への変換だけを持つ。
// Error.message は vv 自身の英文のままで、プラグインの文は pluginMessage に分けて入れる
// （specs/023-english-i18n/contracts/error-api.md）。

// SearchWorkInfo はプラグインの search で作品を探す（POST /api/work-info-plugins/{id}/search）。
func (s *server) SearchWorkInfo(w http.ResponseWriter, r *http.Request, id gen.WorkInfoPluginId) {
	var body struct {
		Query *string `json:"query"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.Query == nil {
		s.invalidRequest(w, "query is required.")
		return
	}
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	candidates, err := s.workInfoPlugins.SearchWorkInfo(r.Context(), id, *body.Query)
	if err != nil {
		s.workInfoRunError(w, err, "The work-info plugin was not found.")
		return
	}
	out := make([]gen.WorkInfoCandidate, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, gen.WorkInfoCandidate{Url: c.URL, Title: c.Title, Detail: optionalString(c.Detail)})
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.WorkInfoSearchResult{Candidates: out}, s.logger)
}

// FetchWorkInfo はプラグインの fetch で動画の作品情報を取り、結果をメモリに持つ
// （POST /api/videos/{id}/work-info-fetches）。動画は変えない。
func (s *server) FetchWorkInfo(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	var body struct {
		PluginID *int64  `json:"pluginId"`
		URL      *string `json:"url"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.PluginID == nil || body.URL == nil {
		s.invalidRequest(w, "pluginId and url are required.")
		return
	}
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	fetch, err := s.workInfoPlugins.FetchWorkInfo(r.Context(), id, *body.PluginID, *body.URL)
	if err != nil {
		s.workInfoRunError(w, err, "The video or the work-info plugin was not found.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusCreated, toAPIWorkInfoFetch(fetch), s.logger)
}

// toAPIWorkInfoFetch は取得の結果を契約の形にする（specs/052-plugin-work-info-fields/contracts/screen-api.md）。
// 一覧は無くても [] で、品番と説明は無ければ省く。
func toAPIWorkInfoFetch(fetch domain.WorkInfoFetchResult) gen.WorkInfoFetch {
	details := fetch.Details
	dates := make([]gen.WorkInfoFetchedDate, 0, len(details.Dates))
	for _, d := range details.Dates {
		dates = append(dates, gen.WorkInfoFetchedDate{Kind: gen.WorkDateKind(d.Kind), Date: d.Date})
	}
	people := make([]gen.WorkInfoFetchedPerson, 0, len(details.People))
	for i, p := range details.People {
		people = append(people, gen.WorkInfoFetchedPerson{
			Role: gen.WorkPersonRole(p.Role), Name: p.Name, ExternalIds: toAPIExternalIDs(p.ExternalIDs),
			Match: toAPIWorkInfoMatch(fetch.PersonMatches, i),
		})
	}
	organizations := make([]gen.WorkInfoFetchedOrganization, 0, len(details.Organizations))
	for i, o := range details.Organizations {
		organizations = append(organizations, gen.WorkInfoFetchedOrganization{
			Role: gen.WorkOrganizationRole(o.Role), Name: o.Name, ExternalIds: toAPIExternalIDs(o.ExternalIDs),
			Match: toAPIWorkInfoMatch(fetch.OrganizationMatches, i),
		})
	}
	urls := details.URLs
	if urls == nil {
		urls = []string{}
	}
	return gen.WorkInfoFetch{
		FetchId:       fetch.ID,
		ExpiresAt:     fetch.ExpiresAt.Unix(),
		Title:         optionalString(fetch.Title),
		Tags:          fetch.Tags,
		Code:          optionalString(details.Code),
		Description:   optionalString(details.Description),
		Dates:         dates,
		People:        people,
		Organizations: organizations,
		Urls:          urls,
		ExternalIds:   toAPIExternalIDs(details.ExternalIDs),
	}
}

// toAPIWorkInfoMatch は i 件目の照合の答えを契約の形にする。答えが無ければ new とする。
func toAPIWorkInfoMatch(matches []domain.CreditMatch, i int) gen.WorkInfoMatch {
	if i >= len(matches) {
		return gen.WorkInfoMatch{State: gen.WorkInfoMatchStateNew}
	}
	match := gen.WorkInfoMatch{State: gen.WorkInfoMatchState(matches[i].State)}
	if credit := matches[i].Credit; match.State == gen.WorkInfoMatchStateExisting && credit != nil {
		match.Id, match.Name = &credit.ID, &credit.Name
	}
	return match
}

// ApplyWorkInfoFetch は取得の結果のうち選んだ項目を動画に反映する
// （POST /api/videos/{id}/work-info-fetches/{fetchId}/apply）。
func (s *server) ApplyWorkInfoFetch(w http.ResponseWriter, r *http.Request, id gen.VideoId, fetchID gen.WorkInfoFetchId) {
	var body gen.ApplyWorkInfoFetchRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	fields := make([]domain.WorkInfoField, 0, len(body.Fields))
	for _, field := range body.Fields {
		if !field.Valid() {
			s.invalidRequest(w, "Each field must be a field of the fetched result, such as title, tags, code or dates.release.")
			return
		}
		fields = append(fields, domain.WorkInfoField(field))
	}
	if len(fields) == 0 {
		s.invalidRequest(w, "fields must name at least one field.")
		return
	}
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	applied, err := s.workInfoPlugins.ApplyWorkInfoFetch(r.Context(), id, fetchID, fields)
	switch {
	case errors.Is(err, domain.ErrWorkInfoApplyInvalid):
		s.invalidRequest(w, "fields must name at least one field, and each must have a value in the result.")
		return
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, "The video was not found, or the result belongs to another video.")
		return
	case errors.Is(err, domain.ErrWorkInfoFetchStale):
		s.writeError(w, http.StatusConflict, codeWorkInfoFetchStale, "The fetched result is out of date. Fetch it again.")
		return
	case err != nil:
		s.internalError(w, "Could not apply the fetched work information.", err)
		return
	}
	// 反映は確定しているので、詳細の一部を読めなくても成功を返す。結果は使い切りで、500 を返すと画面は
	// 反映できたかを知れず、再送も work_info_fetch_stale になる。読めなかった項目は、確定後に発行した動画の
	// 知らせで画面が読み直す。
	video, err := s.videoDetail(r, applied.Video, 0)
	if err != nil {
		s.logger.Warn("cannot load part of the video after applying the fetched work information",
			slog.Int64("video", id), slog.Any("error", err))
	}
	skippedTags := make([]gen.SkippedTag, 0, len(applied.SkippedTags))
	for _, tag := range applied.SkippedTags {
		skippedTags = append(skippedTags, gen.SkippedTag{Name: tag.Name, Reason: gen.SkippedTagReason(tag.Reason)})
	}
	skipped := make([]gen.SkippedWorkInfoValue, 0, len(applied.Skipped))
	for _, value := range applied.Skipped {
		skipped = append(skipped, gen.SkippedWorkInfoValue{
			Field: gen.WorkInfoField(value.Field), Value: value.Value, Reason: gen.SkippedWorkInfoReason(value.Reason),
		})
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.WorkInfoFetchApplied{Video: video, SkippedTags: skippedTags, Skipped: skipped}, s.logger)
}

// workInfoRunError は検索と取得の誤りを契約の応答にする（contracts/screen-api.md の Run failures）。
// notFound は domain.ErrNotFound のときの文である。
func (s *server) workInfoRunError(w http.ResponseWriter, err error, notFound string) {
	var (
		missing *domain.WorkInfoSettingsMissingError
		failed  *domain.WorkInfoRunError
	)
	switch {
	case errors.Is(err, domain.ErrWorkInfoRunInvalid):
		s.invalidRequest(w, "The query must be 1 to 200 characters, and the URL an http or https URL of at most 2048 characters.")
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, notFound)
	case errors.Is(err, domain.ErrWorkInfoPluginNotApproved):
		s.writeError(w, http.StatusConflict, codePluginNotApproved, "The plugin is not approved.")
	case errors.Is(err, domain.ErrWorkInfoOperationUnsupported):
		s.writeError(w, http.StatusConflict, codeOperationUnsupported, "The plugin does not support this operation.")
	case errors.As(err, &missing):
		keys := missing.Keys
		s.writeErrorBody(w, http.StatusConflict, gen.Error{
			Code: codePluginSettingsMissing, SettingKeys: &keys, Message: "The plugin needs settings first.",
		})
	case errors.As(err, &failed):
		s.writeErrorBody(w, workInfoRunFailureStatus(failed.Failure), workInfoRunFailureBody(failed))
	default:
		s.internalError(w, "Could not run the work-info plugin.", err)
	}
}

// workInfoRunFailureStatus は実行の失敗の状態コードである。
func workInfoRunFailureStatus(failure domain.WorkInfoRunFailure) int {
	switch failure {
	case domain.WorkInfoRunTimedOut:
		return http.StatusGatewayTimeout
	case domain.WorkInfoRunBusy:
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadGateway
	}
}

// workInfoRunFailureBody は実行の失敗を、code・reason・pluginMessage・host の付いた Error にする。
func workInfoRunFailureBody(failed *domain.WorkInfoRunError) gen.Error {
	body := gen.Error{
		Code:          codePluginFailed,
		Message:       "The plugin failed.",
		PluginMessage: optionalString(failed.PluginMessage),
		Host:          optionalString(failed.Host),
	}
	reason := func(r gen.ErrorReason, message string) {
		body.Reason, body.Message = &r, message
	}
	switch failed.Failure {
	case domain.WorkInfoRunTimedOut:
		body.Code, body.Message = codePluginTimedOut, "The plugin did not finish in 30 seconds and was stopped."
	case domain.WorkInfoRunBusy:
		body.Code, body.Message = codePluginBusy, "Another plugin is running. Try again in a moment."
	case domain.WorkInfoRunBlockedAddress:
		reason(gen.ErrorReasonBlockedAddress, "The plugin tried to reach an address that is not public.")
	case domain.WorkInfoRunNetworkError:
		reason(gen.ErrorReasonNetworkError, "The plugin could not reach a site.")
	case domain.WorkInfoRunMemoryExceeded:
		reason(gen.ErrorReasonMemoryExceeded, "The plugin used more than 128 MiB of memory and was stopped.")
	case domain.WorkInfoRunOutputTooLarge:
		reason(gen.ErrorReasonOutputTooLarge, "The plugin's output was over 1 MiB.")
	}
	return body
}
