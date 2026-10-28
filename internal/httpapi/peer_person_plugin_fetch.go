package httpapi

import (
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// A person page fetch for the peer's person runs this node's plugin; the peer gives the person and writes the chosen
// values and the face (docs/design-docs/peer-library.md).

// peerPersonFetch is the peer as r's person page fetch and apply reach it. latest is the peer's person after the
// last write, in the peer's ids.
type peerPersonFetch struct {
	s      *server
	r      *http.Request
	latest *gen.Person
}

func peerPersonPath(id int64, rest string) string {
	return "/api/people/" + strconv.FormatInt(domain.FromRemoteID(id), 10) + rest
}

// Get gives the person's external IDs and links, the values a fetch picks its key from.
func (p *peerPersonFetch) Get(_ context.Context, _ domain.CreditKind, id int64) (domain.Credit, error) {
	var person gen.Person
	if err := p.s.peerGet(p.r, peerPersonPath(id, ""), url.Values{}, &person); err != nil {
		return domain.Credit{}, peerWorkInfoError(err)
	}
	return domain.Credit{
		ID: id, Name: person.Name, ExternalIDs: fromAPIExternalIDs(&person.ExternalIds),
		Profile: fromAPIProfile(person.Profile),
	}, nil
}

func (p *peerPersonFetch) ApplyPersonPluginFetch(
	_ context.Context, apply domain.PersonPluginApply,
) (domain.PersonPluginApplied, error) {
	fetched := apply.Fetched
	body := gen.PeerFetchedPluginPerson{
		Name: fetched.Name, Aliases: toAPIAliases(fetched.Aliases), ExternalIds: toAPIExternalIDs(fetched.ExternalIDs),
		Items: make([]gen.PersonPluginItem, 0, len(apply.Items)),
	}
	if !fetched.Profile.Equal(domain.PersonProfile{}) {
		body.Profile = new(toAPIProfile(fetched.Profile))
	}
	for _, item := range apply.Items {
		body.Items = append(body.Items, gen.PersonPluginItem(item))
	}
	var answer gen.PersonPluginFetchApplied
	path := "/api/peer/people/" + strconv.FormatInt(domain.FromRemoteID(apply.PersonID), 10) + "/fetched-person"
	if err := p.s.peerCall(p.r, http.MethodPost, path, url.Values{}, body, &answer); err != nil {
		return domain.PersonPluginApplied{}, peerWorkInfoError(err)
	}
	p.latest = &answer.Person
	applied := domain.PersonPluginApplied{
		Person: domain.Credit{ID: apply.PersonID}, Skipped: make([]domain.SkippedPersonValue, 0, len(answer.Skipped)),
	}
	for _, value := range answer.Skipped {
		applied.Skipped = append(applied.Skipped, domain.SkippedPersonValue{
			Item: domain.PersonPluginItem(value.Item), Value: value.Value, Reason: domain.SkippedWorkInfoReason(value.Reason),
			HolderName: valueOrEmpty(value.HolderName),
		})
	}
	return applied, nil
}

// ReplacePersonImage uploads the face as the screens upload one, as a multipart form.
func (p *peerPersonFetch) ReplacePersonImage(ctx context.Context, id int64, data []byte) (int64, error) {
	var person gen.Person
	err := p.s.peerUpload(ctx, http.MethodPut, peerPersonPath(id, "/image"), func(form *multipart.Writer) error {
		part, err := form.CreateFormFile("image", "face")
		if err != nil {
			return err
		}
		if _, err := part.Write(data); err != nil {
			return err
		}
		return form.Close()
	}, &person)
	if err != nil {
		return 0, err
	}
	p.latest = &person
	return 0, nil
}

// appliedPerson is the peer's person after the apply in this node's ids; it reads the person again when no write
// answered with it.
func (p *peerPersonFetch) appliedPerson(ctx context.Context, id int64) (gen.Person, error) {
	var person gen.Person
	if p.latest != nil {
		person = *p.latest
	} else if err := p.s.peerGet(p.r, peerPersonPath(id, ""), url.Values{}, &person); err != nil {
		return gen.Person{Id: id}, err
	}
	join, err := p.s.vocabularyJoin(ctx)
	if err != nil {
		return gen.Person{Id: id}, err
	}
	rewriteRemotePerson(&person, join)
	return person, nil
}

// ApplyPeerFetchedPerson writes what the peer fetched for this node's person id.
func (s *server) ApplyPeerFetchedPerson(w http.ResponseWriter, r *http.Request, id gen.PersonId) {
	if !s.requirePeerCaller(w, r) || !s.workInfoPluginsConfigured(w) {
		return
	}
	var body gen.PeerFetchedPluginPerson
	if !s.readLargeJSONBody(w, r, maxPeerFetchedWorkInfoBody, &body) {
		return
	}
	fetched := domain.FetchedPluginPerson{
		Name: body.Name, Aliases: fromAPIAliases(&body.Aliases), ExternalIDs: fromAPIExternalIDs(&body.ExternalIds),
	}
	if body.Profile != nil {
		fetched.Profile = fromAPIProfile(*body.Profile)
	}
	items := make([]domain.PersonPluginItem, 0, len(body.Items))
	for _, item := range body.Items {
		items = append(items, domain.PersonPluginItem(item))
	}
	applied, err := s.workInfoPlugins.ApplyPeerFetchedPerson(r.Context(), id, fetched, items)
	switch {
	case errors.Is(err, domain.ErrWorkInfoApplyInvalid):
		s.invalidRequest(w, "Each item must be a profile item, aliases or externalIds with a value in the request.")
		return
	case errors.Is(err, domain.ErrPersonNotFound):
		s.notFoundReason(w, gen.ErrorReasonPersonNotFound, "The person was deleted or merged into another.")
		return
	case err != nil:
		s.internalError(w, "Could not apply the fetched person.", err)
		return
	}
	writeJSON(w, http.StatusOK, gen.PersonPluginFetchApplied{
		Person: toAPIPerson(applied.Person), Skipped: toAPISkippedPersonValues(applied.Skipped),
	}, s.logger)
}
