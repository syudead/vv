package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 動画の作品情報の読み書き（specs/047-video-work-info/contracts/screen-api.md「PUT /api/videos/{id}/work-info」
// 「PUT /api/videos/{id}/details」「Video changes」、research.md R-5・R-10・R-11・R-14）。どちらの経路も
// 所有者だけ（accessRoutes の既定）で、internal/app は通さず internal/store の WorkInfoStore の 1 つの
// 取引で済ませる。誤りは人物・団体の経路と同じ workInfoFailureOf で写す。

// WorkInfo は動画の作品情報の保存先である。internal/store の *WorkInfoStore がこれを満たす。
type WorkInfo interface {
	// Set は動画 videoID の作品情報をまるごと置き換え、作品情報を含む動画を返す。
	Set(ctx context.Context, videoID int64, info domain.WorkInfo) (domain.Video, error)
	// SetMany は外部連携の一括操作で、各動画の作品情報を 1 つの取引でまとめて置き換え、反映後の動画を
	// changes の順に返す。誤りは *domain.WorkInfoAtError か *domain.VideoRefNotFoundError で位置を運ぶ。
	SetMany(ctx context.Context, changes []domain.WorkInfoChange) ([]domain.Video, error)
	// SetDetails は表示名と作品情報を 1 つの取引で書き、両方を含む動画を返す。
	SetDetails(ctx context.Context, videoID int64, displayName string, info domain.WorkInfo) (domain.Video, error)
	// ByUserKeys は利用者データの鍵ごとの作品情報を返す。行の無い鍵は含めない。
	ByUserKeys(ctx context.Context, userKeys []string) (map[string]domain.WorkInfo, error)
}

// SetVideoWorkInfo は動画の作品情報をまるごと置き換える（PUT /api/videos/{id}/work-info）。
func (s *server) SetVideoWorkInfo(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	var body *gen.WorkInfoInput
	raw, ok := s.readJSONBodyRaw(w, r, &body)
	if !ok {
		return
	}
	if body == nil {
		s.invalidRequest(w, "The body must be a JSON object.")
		return
	}
	info, ok := s.readWorkInfoInput(w, *body, raw)
	if !ok || !s.workInfoConfigured(w) {
		return
	}
	video, err := s.workInfo.Set(r.Context(), id, info)
	if err != nil {
		s.writeVideoWorkInfoError(w, err)
		return
	}
	s.writeVideoDetail(w, r, video)
}

// SetVideoDetails は表示名と作品情報を 1 回で保存する（PUT /api/videos/{id}/details）。
func (s *server) SetVideoDetails(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	// gen.VideoDetailsInput の displayName は string、workInfo は値なので、欠けても空（解除）に読めて
	// しまう。欠けた本文で表示名や作品情報を消さないよう、有無を区別して読む。
	var body struct {
		DisplayName *string            `json:"displayName"`
		WorkInfo    *gen.WorkInfoInput `json:"workInfo"`
	}
	raw, ok := s.readJSONBodyRaw(w, r, &body)
	if !ok {
		return
	}
	if body.DisplayName == nil || body.WorkInfo == nil {
		s.invalidRequest(w, "displayName and workInfo are required.")
		return
	}
	// 契約は displayName を先に確かめる。作品情報の列挙と必須のプロパティの検査より前に表示名の規則を
	// 見て、両方に誤りがあっても display_name_* と field: displayName を返す。
	if _, _, err := domain.NormalizeDisplayName(*body.DisplayName); s.writeDetailsDisplayNameError(w, err) {
		return
	}
	var fields struct {
		WorkInfo json.RawMessage `json:"workInfo"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		s.invalidBody(w, err, "Cannot parse the JSON body.")
		return
	}
	info, ok := s.readWorkInfoInput(w, *body.WorkInfo, fields.WorkInfo)
	if !ok || !s.workInfoConfigured(w) {
		return
	}
	video, err := s.workInfo.SetDetails(r.Context(), id, *body.DisplayName, info)
	if s.writeDetailsDisplayNameError(w, err) {
		return
	}
	if err != nil {
		s.writeVideoWorkInfoError(w, err)
		return
	}
	s.writeVideoDetail(w, r, video)
}

// writeDetailsDisplayNameError は err が表示名の誤りなら field: displayName を付けて書き、true を返す。
func (s *server) writeDetailsDisplayNameError(w http.ResponseWriter, err error) bool {
	failure, ok := displayNameFailureOf(err)
	if !ok {
		return false
	}
	failure.field = "displayName"
	s.writeWorkInfoFailure(w, failure)
	return true
}

// writeVideoWorkInfoError は作品情報の書き込みの誤りを写す。項目と参照の誤りは workInfoFailureOf が、
// 動画が無いことはここが video_not_found で写す。
func (s *server) writeVideoWorkInfoError(w http.ResponseWriter, err error) {
	if failure, ok := workInfoFailureOf(err); ok {
		s.writeWorkInfoFailure(w, failure)
		return
	}
	switch {
	case errors.Is(err, domain.ErrInvalidWorkInfo):
		s.invalidRequest(w, "The work information cannot be used.")
	case errors.Is(err, domain.ErrNotFound):
		s.notFoundReason(w, reasonVideoNotFound, "Video not found.")
	default:
		s.internalError(w, "Could not save the work information.", err)
	}
}

func (s *server) workInfoConfigured(w http.ResponseWriter) bool {
	if s.workInfo == nil {
		s.internalError(w, "Work information storage is not configured.", nil)
		return false
	}
	return true
}

// readWorkInfoInput は要求の作品情報を domain の形にする。raw は input を読んだ JSON そのもので、
// 一覧の各件の必須のプロパティ（WorkDate の kind・date など）が欠けているか null なら、または種類と
// 役割が契約の列挙に無ければ、400 invalid_request を書いて false を返す（契約の「generated schema
// check」に当たる検査）。生成型は欠けた date を空の文字列に読み、空の日付は落として保存済みの日付を
// 消してしまうので、有無は raw で確かめる。省いた一覧の値と null は空で、空は解除である。
func (s *server) readWorkInfoInput(w http.ResponseWriter, input gen.WorkInfoInput, raw json.RawMessage) (domain.WorkInfo, bool) {
	info, message, ok := workInfoFromInput(input, raw)
	if !ok {
		s.invalidRequest(w, message)
	}
	return info, ok
}

// workInfoFromInput は readWorkInfoInput の本体で、応答を書かずに誤りの文言と false を返す。外部連携 API の
// POST /api/v1/video-work-info も、項目の workInfo を同じ形の gen.WorkInfoInput に読んでこれを呼ぶ。
func workInfoFromInput(input gen.WorkInfoInput, raw json.RawMessage) (domain.WorkInfo, string, bool) {
	message, ok := workInfoInputRequiredPresent(raw)
	if ok {
		message, ok = workInfoInputEnumsValid(input)
	}
	if !ok {
		return domain.WorkInfo{}, message, false
	}
	info := domain.WorkInfo{Code: valueOrZero(input.Code), Description: valueOrZero(input.Description)}
	for _, date := range valueOrZero(input.Dates) {
		info.Dates = append(info.Dates, domain.WorkDate{Kind: domain.DateKind(date.Kind), Date: date.Date})
	}
	for _, person := range valueOrZero(input.People) {
		info.People = append(info.People, domain.WorkPerson{
			Role: domain.PersonRole(person.Role), Person: domain.CreditRef{ID: person.Person.Id},
		})
	}
	for _, organization := range valueOrZero(input.Organizations) {
		info.Organizations = append(info.Organizations, domain.WorkOrganization{
			Role: domain.OrganizationRole(organization.Role), Organization: domain.CreditRef{ID: organization.Organization.Id},
		})
	}
	info.URLs = valueOrZero(input.Urls)
	info.ExternalIDs = fromAPIExternalIDs(input.ExternalIds)
	return info, "", true
}

// workInfoInputEnumsValid は日付の種類と人物・団体の役割が契約の列挙にあるかを確かめ、無ければ
// 誤りの文言と false を返す。
func workInfoInputEnumsValid(input gen.WorkInfoInput) (string, bool) {
	for _, date := range valueOrZero(input.Dates) {
		if !date.Kind.Valid() {
			return "dates[].kind must be release, recorded or published.", false
		}
	}
	for _, person := range valueOrZero(input.People) {
		if !person.Role.Valid() {
			return "people[].role must be performer, director, photographer or streamer.", false
		}
	}
	for _, organization := range valueOrZero(input.Organizations) {
		if !organization.Role.Valid() {
			return "organizations[].role must be maker or label.", false
		}
	}
	return "", true
}

// workInfoInputEntries は WorkInfoInput の一覧のうち、件が必須のプロパティを持つものを JSON のまま
// 読む形である。一覧の null と省略は空で、件の null は空の map（どのプロパティも無い）になる。
type workInfoInputEntries struct {
	Dates         []map[string]json.RawMessage `json:"dates"`
	People        []map[string]json.RawMessage `json:"people"`
	Organizations []map[string]json.RawMessage `json:"organizations"`
	ExternalIds   []map[string]json.RawMessage `json:"externalIds"`
}

// workInfoInputRequired は一覧ごとの件の必須のプロパティを WorkInfo の順に並べたものである
// （api/openapi.yaml の WorkDate・WorkPersonInput・WorkOrganizationInput・ExternalId）。ref は
// CreditIdRef を値に持つプロパティで、その id も必須である。
var workInfoInputRequired = []struct {
	list     string
	entries  func(workInfoInputEntries) []map[string]json.RawMessage
	required []string
	ref      string
}{
	{"dates", func(e workInfoInputEntries) []map[string]json.RawMessage { return e.Dates }, []string{"kind", "date"}, ""},
	{"people", func(e workInfoInputEntries) []map[string]json.RawMessage { return e.People }, []string{"role", "person"}, "person"},
	{"organizations", func(e workInfoInputEntries) []map[string]json.RawMessage { return e.Organizations },
		[]string{"role", "organization"}, "organization"},
	{"externalIds", func(e workInfoInputEntries) []map[string]json.RawMessage { return e.ExternalIds },
		[]string{"source", "value"}, ""},
}

// workInfoInputRequiredPresent は raw（WorkInfoInput の JSON）の一覧の各件が必須のプロパティを null で
// なく持つかを WorkInfo の順に確かめ、最初に欠けたものの誤りの文言と false を返す。
func workInfoInputRequiredPresent(raw json.RawMessage) (string, bool) {
	var entries workInfoInputEntries
	if err := json.Unmarshal(raw, &entries); err != nil {
		return "The work information must be a JSON object.", false
	}
	for _, list := range workInfoInputRequired {
		for i, entry := range list.entries(entries) {
			for _, name := range list.required {
				if !jsonPropertyNotNull(entry, name) {
					return fmt.Sprintf("%s[%d].%s is required.", list.list, i, name), false
				}
			}
			if list.ref == "" {
				continue
			}
			value, _ := jsonProperty(entry, list.ref)
			var ref map[string]json.RawMessage
			if err := json.Unmarshal(value, &ref); err != nil || !jsonPropertyNotNull(ref, "id") {
				return fmt.Sprintf("%s[%d].%s.id is required.", list.list, i, list.ref), false
			}
		}
	}
	return "", true
}

// jsonPropertyNotNull は JSON オブジェクトが name を null でない値で持つことを返す。
func jsonPropertyNotNull(fields map[string]json.RawMessage, name string) bool {
	value, ok := jsonProperty(fields, name)
	return ok && !bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

// valueOrZero は省いた値と null を型の零値として読む。
func valueOrZero[T any](value *T) T {
	if value == nil {
		var zero T
		return zero
	}
	return *value
}

// workInfoForDetail は動画 1 件の応答に載せる作品情報を読む（GET /api/videos/{id}）。所有者にだけ
// 読み、ゲストと保存先の無い構成では video をそのまま返す。
func (s *server) workInfoForDetail(ctx context.Context, video domain.Video) (domain.Video, error) {
	if s.workInfo == nil || !audienceFrom(ctx).IsOwner() || video.UserKey == "" {
		return video, nil
	}
	infos, err := s.workInfo.ByUserKeys(ctx, []string{video.UserKey})
	if err != nil {
		return domain.Video{}, err
	}
	if info, ok := infos[video.UserKey]; ok {
		video.WorkInfo = &info
	}
	return video, nil
}

// toAPIWorkInfo は作品情報を契約の WorkInfo にする。nil（行の無い動画）なら nil を返し、応答から省く。
func toAPIWorkInfo(info *domain.WorkInfo) *gen.WorkInfo {
	if info == nil {
		return nil
	}
	out := gen.WorkInfo{
		Code: optionalString(info.Code), Description: optionalString(info.Description),
		Dates:         make([]gen.WorkDate, 0, len(info.Dates)),
		People:        make([]gen.WorkPerson, 0, len(info.People)),
		Organizations: make([]gen.WorkOrganization, 0, len(info.Organizations)),
		Urls:          append([]string{}, info.URLs...),
		ExternalIds:   toAPIExternalIDs(info.ExternalIDs),
	}
	for _, date := range info.Dates {
		out.Dates = append(out.Dates, gen.WorkDate{Kind: gen.WorkDateKind(date.Kind), Date: date.Date})
	}
	for _, person := range info.People {
		out.People = append(out.People, gen.WorkPerson{
			Role: gen.WorkPersonRole(person.Role), Person: gen.CreditRef{Id: person.Person.ID, Name: person.Person.Name},
		})
	}
	for _, organization := range info.Organizations {
		out.Organizations = append(out.Organizations, gen.WorkOrganization{
			Role:         gen.WorkOrganizationRole(organization.Role),
			Organization: gen.CreditRef{Id: organization.Organization.ID, Name: organization.Organization.Name},
		})
	}
	return &out
}
