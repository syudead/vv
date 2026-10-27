package httpapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// A fetch for the peer's video runs this node's plugin; the peer gives the video and the credit matches and
// writes the chosen values (docs/design-docs/peer-library.md).

const maxPeerFetchedWorkInfoBody = 1 << 20

// peerWorkInfo is the peer as r's work-info fetch and apply reach it.
type peerWorkInfo struct {
	s *server
	r *http.Request
}

func (s *server) peerWorkInfo(r *http.Request) peerWorkInfo { return peerWorkInfo{s: s, r: r} }

// peerWorkInfoError turns the peer's 404 into ErrNotFound, keeping ErrPeerUnreachable.
func peerWorkInfoError(err error) error {
	var refused *peerRefusal
	if errors.As(err, &refused) && refused.status == http.StatusNotFound {
		return domain.ErrNotFound
	}
	return err
}

func (p peerWorkInfo) GetVideo(_ context.Context, _ domain.Audience, id int64) (domain.Video, error) {
	var video gen.Video
	if err := p.s.peerGet(p.r, peerVideoPath(domain.FromRemoteID(id), "", nil), url.Values{}, &video); err != nil {
		return domain.Video{}, peerWorkInfoError(err)
	}
	if video.ContentVersion == nil {
		return domain.Video{}, domain.ErrNotFound
	}
	return domain.Video{ID: id, ContentKey: *video.ContentVersion}, nil
}

// Match turns the peer's ids into the ids its videos show under, so a match names the person or organization
// the video's credits name.
func (p peerWorkInfo) Match(
	ctx context.Context, kind domain.CreditKind, entries []domain.CreditMatchEntry,
) ([]domain.CreditMatch, error) {
	body := gen.PeerCreditMatchRequest{
		Kind: gen.PeerCreditMatchRequestKind(kind), Entries: make([]gen.PeerCreditMatchEntry, 0, len(entries)),
	}
	for _, entry := range entries {
		body.Entries = append(body.Entries, gen.PeerCreditMatchEntry{Name: entry.Name, ExternalIds: toAPIExternalIDs(entry.ExternalIDs)})
	}
	var answer gen.PeerCreditMatches
	if err := p.s.peerCall(p.r, http.MethodPost, "/api/peer/credit-matches", url.Values{}, body, &answer); err != nil {
		return nil, peerWorkInfoError(err)
	}
	if len(answer.Matches) != len(entries) {
		return nil, fmt.Errorf("%w: %d matches for %d entries", errPeerAnswer, len(answer.Matches), len(entries))
	}
	join, err := p.s.vocabularyJoin(ctx)
	if err != nil {
		return nil, err
	}
	matches := make([]domain.CreditMatch, len(answer.Matches))
	for i, match := range answer.Matches {
		matches[i] = domain.CreditMatch{State: domain.CreditMatchState(match.State)}
		if match.State == gen.WorkInfoMatchStateExisting && match.Id != nil && match.Name != nil {
			id := join.Person(*match.Id, *match.Name)
			if kind == domain.CreditOrganization {
				id = join.Organization(*match.Id, *match.Name)
			}
			matches[i].Credit = &domain.CreditRef{ID: id, Name: *match.Name}
		}
	}
	return matches, nil
}

func (p peerWorkInfo) ApplyFetchedWorkInfo(
	_ context.Context, apply domain.FetchedWorkInfoApply,
) (domain.FetchedWorkInfoApplied, error) {
	body := toPeerFetchedWorkInfo(apply)
	var answer gen.PeerFetchedWorkInfoApplied
	path := "/api/peer/videos/" + strconv.FormatInt(domain.FromRemoteID(apply.VideoID), 10) + "/fetched-work-info"
	if err := p.s.peerCall(p.r, http.MethodPost, path, url.Values{}, body, &answer); err != nil {
		return domain.FetchedWorkInfoApplied{}, peerWorkInfoError(err)
	}
	applied := domain.FetchedWorkInfoApplied{
		Video: domain.Video{ID: apply.VideoID}, RejectedTags: answer.RejectedTags,
		Skipped: make([]domain.SkippedWorkInfoValue, 0, len(answer.Skipped)),
	}
	for _, value := range answer.Skipped {
		applied.Skipped = append(applied.Skipped, domain.SkippedWorkInfoValue{
			Field: domain.WorkInfoField(value.Field), Value: value.Value, Reason: domain.SkippedWorkInfoReason(value.Reason),
		})
	}
	return applied, nil
}

// UploadThumbnailImage sends the cover as the screens upload a thumbnail, as a multipart form.
func (p peerWorkInfo) UploadThumbnailImage(
	ctx context.Context, id int64, contentVersion string, image []byte,
) (domain.Video, error) {
	if p.s.peerLink == nil {
		return domain.Video{}, domain.ErrPeerUnreachable
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if err := writeThumbnailForm(form, contentVersion, image); err != nil {
		return domain.Video{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, peerVideoPath(domain.FromRemoteID(id), "/thumbnail-image", nil), &body)
	if err != nil {
		return domain.Video{}, err
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	// The peer may wait to decode and fit the image, as a forwarded thumbnail upload does.
	resp, err := p.s.peerLink.Forward(req, false, true)
	if err != nil {
		return domain.Video{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxForwardedJSON))
	if resp.StatusCode != http.StatusOK {
		return domain.Video{}, &peerRefusal{status: resp.StatusCode}
	}
	return domain.Video{ID: id}, nil
}

func writeThumbnailForm(form *multipart.Writer, contentVersion string, image []byte) error {
	if err := form.WriteField("contentVersion", contentVersion); err != nil {
		return err
	}
	part, err := form.CreateFormFile("image", "cover")
	if err != nil {
		return err
	}
	if _, err := part.Write(image); err != nil {
		return err
	}
	return form.Close()
}

// toPeerFetchedWorkInfo sends the chosen values with the peer's ids: a same-person pair names one of the video's
// unnamed performers, who never takes a local id.
func toPeerFetchedWorkInfo(apply domain.FetchedWorkInfoApply) gen.PeerFetchedWorkInfo {
	details := apply.Details
	body := gen.PeerFetchedWorkInfo{
		ContentVersion: apply.ContentKey, Title: apply.Title, Tags: apply.Tags,
		Code: optionalString(details.Code), Description: optionalString(details.Description),
		Dates:  make([]gen.WorkInfoFetchedDate, 0, len(details.Dates)),
		People: make([]gen.PeerFetchedPerson, 0, len(details.People)), Urls: details.URLs,
		Organizations: make([]gen.PeerFetchedOrganization, 0, len(details.Organizations)),
		ExternalIds:   toAPIExternalIDs(details.ExternalIDs),
		DetailFields:  make([]gen.WorkInfoField, 0, len(apply.DetailFields)),
		SamePeople:    make([]gen.WorkInfoSamePerson, 0, len(apply.SamePeople)),
	}
	if body.Tags == nil {
		body.Tags = []string{}
	}
	if body.Urls == nil {
		body.Urls = []string{}
	}
	for _, d := range details.Dates {
		body.Dates = append(body.Dates, gen.WorkInfoFetchedDate{Kind: gen.WorkDateKind(d.Kind), Date: d.Date})
	}
	for _, person := range details.People {
		out := gen.PeerFetchedPerson{Role: gen.WorkPersonRole(person.Role), Name: person.Name, ExternalIds: toAPIExternalIDs(person.ExternalIDs)}
		if !person.Profile.Equal(domain.PersonProfile{}) {
			out.Profile = new(toAPIProfile(person.Profile))
		}
		body.People = append(body.People, out)
	}
	for _, organization := range details.Organizations {
		body.Organizations = append(body.Organizations, gen.PeerFetchedOrganization{
			Role: gen.WorkOrganizationRole(organization.Role), Name: organization.Name,
			ExternalIds: toAPIExternalIDs(organization.ExternalIDs),
		})
	}
	for _, field := range apply.DetailFields {
		body.DetailFields = append(body.DetailFields, gen.WorkInfoField(field))
	}
	for _, same := range apply.SamePeople {
		body.SamePeople = append(body.SamePeople, gen.WorkInfoSamePerson{At: same.At, PersonId: domain.FromRemoteID(same.PersonID)})
	}
	return body
}

// fromPeerFetchedWorkInfo is the apply toPeerFetchedWorkInfo sent, without the video, which the path names.
func fromPeerFetchedWorkInfo(body gen.PeerFetchedWorkInfo) (domain.FetchedWorkInfoApply, bool) {
	apply := domain.FetchedWorkInfoApply{Title: body.Title, Tags: body.Tags}
	details := &apply.Details
	details.Code, details.Description = valueOrEmpty(body.Code), valueOrEmpty(body.Description)
	details.URLs, details.ExternalIDs = body.Urls, fromAPIExternalIDs(&body.ExternalIds)
	for _, d := range body.Dates {
		details.Dates = append(details.Dates, domain.WorkDate{Kind: domain.DateKind(d.Kind), Date: d.Date})
	}
	for _, person := range body.People {
		fetched := domain.FetchedPerson{Role: domain.PersonRole(person.Role), Name: person.Name, ExternalIDs: fromAPIExternalIDs(&person.ExternalIds)}
		if person.Profile != nil {
			fetched.Profile = fromAPIProfile(*person.Profile)
		}
		details.People = append(details.People, fetched)
	}
	for _, organization := range body.Organizations {
		details.Organizations = append(details.Organizations, domain.FetchedOrganization{
			Role: domain.OrganizationRole(organization.Role), Name: organization.Name,
			ExternalIDs: fromAPIExternalIDs(&organization.ExternalIds),
		})
	}
	for _, field := range body.DetailFields {
		if !field.Valid() {
			return domain.FetchedWorkInfoApply{}, false
		}
		apply.DetailFields = append(apply.DetailFields, domain.WorkInfoField(field))
	}
	for _, same := range body.SamePeople {
		apply.SamePeople = append(apply.SamePeople, domain.SamePerson{At: same.At, PersonID: same.PersonId})
	}
	return apply, true
}

// MatchPeerCredits answers the peer's fetch for a video of this node with this node's matches.
func (s *server) MatchPeerCredits(w http.ResponseWriter, r *http.Request) {
	if !s.requirePeerCaller(w, r) || !s.workInfoPluginsConfigured(w) {
		return
	}
	var body gen.PeerCreditMatchRequest
	if !s.readLargeJSONBody(w, r, maxPeerFetchedWorkInfoBody, &body) {
		return
	}
	if !body.Kind.Valid() {
		s.invalidRequest(w, "kind must be person or organization.")
		return
	}
	entries := make([]domain.CreditMatchEntry, 0, len(body.Entries))
	for _, entry := range body.Entries {
		entries = append(entries, domain.CreditMatchEntry{Name: entry.Name, ExternalIDs: fromAPIExternalIDs(&entry.ExternalIds)})
	}
	matches, err := s.workInfoPlugins.MatchWorkInfoCredits(r.Context(), domain.CreditKind(body.Kind), entries)
	if err != nil {
		s.internalError(w, "Could not match the fetched credits.", err)
		return
	}
	out := gen.PeerCreditMatches{Matches: make([]gen.WorkInfoMatch, 0, len(matches))}
	for i := range matches {
		out.Matches = append(out.Matches, toAPIWorkInfoMatch(matches, i))
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}

// ApplyPeerFetchedWorkInfo writes what the peer fetched for this node's video id.
func (s *server) ApplyPeerFetchedWorkInfo(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	if !s.requirePeerCaller(w, r) || !s.workInfoPluginsConfigured(w) {
		return
	}
	var body gen.PeerFetchedWorkInfo
	if !s.readLargeJSONBody(w, r, maxPeerFetchedWorkInfoBody, &body) {
		return
	}
	apply, ok := fromPeerFetchedWorkInfo(body)
	if !ok {
		s.invalidRequest(w, "Each detail field must be a field of a fetched result.")
		return
	}
	applied, err := s.workInfoPlugins.ApplyPeerFetchedWorkInfo(r.Context(), id, body.ContentVersion, apply)
	switch {
	case errors.Is(err, domain.ErrWorkInfoApplyInvalid):
		s.invalidRequest(w, "Each same-person pair must name a distinct fetched performer and a distinct person.")
		return
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, "The video was not found, or its content has changed.")
		return
	case err != nil:
		s.internalError(w, "Could not apply the fetched work information.", err)
		return
	}
	out := gen.PeerFetchedWorkInfoApplied{
		RejectedTags: applied.RejectedTags, Skipped: toAPISkippedWorkInfoValues(applied.Skipped),
	}
	if out.RejectedTags == nil {
		out.RejectedTags = []string{}
	}
	writeJSON(w, http.StatusOK, out, s.logger)
}

// peerVideoDetail is the peer's video id after an apply, in this node's ids, or the bare id when the peer does
// not answer, as a local apply answers when it cannot read part of the video.
func (s *server) peerVideoDetail(r *http.Request, id int64) (gen.Video, error) {
	var video gen.Video
	if err := s.peerGet(r, peerVideoPath(domain.FromRemoteID(id), "", nil), url.Values{}, &video); err != nil {
		return gen.Video{Id: id}, err
	}
	join, err := s.vocabularyJoin(r.Context())
	if err != nil {
		return gen.Video{Id: id}, err
	}
	rewriteRemoteVideo(&video, join)
	return video, nil
}
