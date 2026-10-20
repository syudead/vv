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

// Work-info mutations are owner-only, use one transaction, and share workInfoFailureOf with other APIs.

type WorkInfo interface {
	Set(ctx context.Context, videoID int64, info domain.WorkInfo) (domain.Video, error)
	// SetMany is atomic; indexed errors preserve input order.
	SetMany(ctx context.Context, changes []domain.WorkInfoChange) ([]domain.Video, error)
	// SetDetails saves display name and work information atomically.
	SetDetails(ctx context.Context, videoID int64, displayName string, info domain.WorkInfo) (domain.Video, error)
	// EditCredits applies role edits to the listed videos atomically and counts the videos that changed.
	EditCredits(ctx context.Context, videoIDs []int64, edits []domain.CreditEdit) (int, error)
	// ByUserKeys omits keys that do not exist.
	ByUserKeys(ctx context.Context, userKeys []string) (map[string]domain.WorkInfo, error)
}

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

func (s *server) SetVideoDetails(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	// Check raw field presence so generated zero values cannot silently clear data.
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
	// Validate displayName before work-info enums and required fields, even when both are invalid.
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

func (s *server) writeDetailsDisplayNameError(w http.ResponseWriter, err error) bool {
	failure, ok := displayNameFailureOf(err)
	if !ok {
		return false
	}
	failure.field = "displayName"
	s.writeWorkInfoFailure(w, failure)
	return true
}

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

// Check required nested fields before decoding: missing dates must not silently clear values. Omitted or null lists clear them; enum values are validated.
func (s *server) readWorkInfoInput(w http.ResponseWriter, input gen.WorkInfoInput, raw json.RawMessage) (domain.WorkInfo, bool) {
	info, message, ok := workInfoFromInput(input, raw)
	if !ok {
		s.invalidRequest(w, message)
	}
	return info, ok
}

// Share body parsing with the REST API.
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

// Omitted or null lists are empty; null entries still fail required-field checks.
type workInfoInputEntries struct {
	Dates         []map[string]json.RawMessage `json:"dates"`
	People        []map[string]json.RawMessage `json:"people"`
	Organizations []map[string]json.RawMessage `json:"organizations"`
	ExternalIds   []map[string]json.RawMessage `json:"externalIds"`
}

// Check required fields in schema order, including reference IDs.
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

func jsonPropertyNotNull(fields map[string]json.RawMessage, name string) bool {
	value, ok := jsonProperty(fields, name)
	return ok && !bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

func valueOrZero[T any](value *T) T {
	if value == nil {
		var zero T
		return zero
	}
	return *value
}

// Work details are owner-only; leave guest responses and responses without a store unchanged.
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

// Omit nil work information.
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
			Role: gen.WorkPersonRole(person.Role),
			Person: gen.CreditRef{
				Id: person.Person.ID, Name: person.Person.Name, Provisional: person.Person.Provisional,
			},
			Provisional: person.Person.Provisional, ImageRevision: optionalPositive(person.ImageRevision),
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
