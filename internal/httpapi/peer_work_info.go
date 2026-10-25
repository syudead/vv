package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// remoteDetailsInput is VideoDetailsInput with each property kept absent when the screen left it out, so the
// peer answers a missing one as it answers its own screens.
type remoteDetailsInput struct {
	DisplayName    *string            `json:"displayName,omitempty"`
	WorkInfo       *gen.WorkInfoInput `json:"workInfo,omitempty"`
	ContentVersion *string            `json:"contentVersion,omitempty"`
}

// peerWorkInfoBody gives the peer a work-info save in its own person and organization ids; workInfo picks the
// work information out of the body. A person or organization the peer lacks is created there by name, as a bulk
// edit does.
func peerWorkInfoBody[T any](workInfo func(*T) *gen.WorkInfoInput) func(*server, http.ResponseWriter, *http.Request) bool {
	return func(s *server, w http.ResponseWriter, r *http.Request) bool {
		var body T
		if _, ok := s.readJSONBodyRaw(w, r, &body); !ok {
			return false
		}
		if info := workInfo(&body); info != nil {
			if err := translateWorkInfoCredits(info, s.peerEditVocabulary(r)); err != nil {
				s.writeSplitEditError(w, err)
				return false
			}
		}
		raw, err := json.Marshal(body)
		if err != nil {
			s.internalError(w, "Could not forward the request to the other VVMDM.", err)
			return false
		}
		r.Body, r.ContentLength = io.NopCloser(bytes.NewReader(raw)), int64(len(raw))
		return true
	}
}

// translateWorkInfoCredits turns the people and organizations of info into v's node's ids.
func translateWorkInfoCredits(info *gen.WorkInfoInput, v editVocabulary) error {
	v = rememberCreated(v)
	if info.People != nil {
		err := translateCreditRefs(v, domain.VocabularyPerson, *info.People, "people[%d].person.id",
			func(p *gen.WorkPersonInput) *gen.CreditIdRef { return &p.Person })
		if err != nil {
			return err
		}
	}
	if info.Organizations != nil {
		return translateCreditRefs(v, domain.VocabularyOrganization, *info.Organizations, "organizations[%d].organization.id",
			func(o *gen.WorkOrganizationInput) *gen.CreditIdRef { return &o.Organization })
	}
	return nil
}

func translateCreditRefs[T any](
	v editVocabulary, kind domain.VocabularyKind, items []T, field string, ref func(*T) *gen.CreditIdRef,
) error {
	for i := range items {
		target := ref(&items[i])
		id, err := translateID(v, kind, target.Id, fmt.Sprintf(field, i), true)
		if err != nil {
			return err
		}
		target.Id = id
	}
	return nil
}
