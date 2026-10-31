package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

const similarPeoplePattern = "GET /api/people/{id}/similar-people"

// rewriteRemoteFaceAppearances joins the people by name; the crops are the peer's, read through the forwarded crop.
func rewriteRemoteFaceAppearances(list *gen.VideoFaceAppearanceList, join domain.VocabularyJoin) {
	for i := range list.Items {
		item := &list.Items[i]
		item.AppearanceId = domain.ToRemoteID(item.AppearanceId)
		item.Person.Id = join.Person(item.Person.Id, item.Person.Name)
	}
}

func rewriteRemotePersonAppearances(list *gen.FaceAppearanceList, _ domain.VocabularyJoin) {
	for i := range list.Items {
		item := &list.Items[i]
		item.AppearanceId = domain.ToRemoteID(item.AppearanceId)
		if item.VideoId != nil {
			*item.VideoId = domain.ToRemoteID(*item.VideoId)
		}
	}
}

func rewriteRemoteFaceMove(move *gen.FaceAppearanceMove, join domain.VocabularyJoin) {
	rewriteRemotePerson(&move.Person, join)
}

func rewriteRemoteFaceCandidates(list *gen.FaceCandidateList, join domain.VocabularyJoin) {
	for i := range list.Items {
		person := &list.Items[i].Person
		person.Id = join.Person(person.Id, person.Name)
	}
}

// rewriteRemoteFaceConfirmation leaves out the undo entry, which is in the peer's activity log, not this node's.
func rewriteRemoteFaceConfirmation(confirmation *gen.FaceCandidateConfirmation, join domain.VocabularyJoin) {
	rewriteRemotePerson(&confirmation.Person, join)
	confirmation.UndoActivityId = nil
}

func rewriteRemoteSimilarPeople(list *gen.SimilarPersonList, join domain.VocabularyJoin) {
	for i := range list.Items {
		item := &list.Items[i]
		rewriteRemotePerson(&item.Person, join)
		for j := range item.SampleAppearanceIds {
			item.SampleAppearanceIds[j] = domain.ToRemoteID(item.SampleAppearanceIds[j])
		}
	}
}

// peerPersonQuery turns the appearances a similar-people read compares into the peer's. This node's appearances
// are not the peer's person's, so they are ignored as the peer ignores appearances the person lacks.
func peerPersonQuery(pattern string, query url.Values, op *remoteOperation) {
	if pattern != similarPeoplePattern || !query.Has("appearanceId") {
		return
	}
	var peer []string
	for _, value := range query["appearanceId"] {
		id, err := strconv.ParseInt(value, 10, 64)
		switch {
		case err != nil:
			peer = append(peer, value)
		case domain.IsRemoteID(id):
			peer = append(peer, strconv.FormatInt(domain.FromRemoteID(id), 10))
		}
	}
	if len(peer) == 0 {
		// Without appearanceId the peer compares every appearance; with none left the list is empty instead.
		op.rewrite = func([]byte, domain.VocabularyJoin) (any, error) {
			return gen.SimilarPersonList{Items: []gen.SimilarPerson{}}, nil
		}
	}
	query["appearanceId"] = peer
}

// peerFaceSplit splits only the peer's appearances of the peer's person.
func peerFaceSplit(body *gen.FaceAppearanceSplitRequest) bool {
	return onlyRemote(body.AppearanceIds) && toPeer(&body.AppearanceIds)
}

// peerFaceMoveBody turns a move of the peer's appearances into the peer's ids; the target goes to the peer's person
// of its name, created there when missing, since the screens show one person per name.
func (s *server) peerFaceMoveBody(w http.ResponseWriter, r *http.Request) bool {
	var body gen.FaceAppearanceMoveRequest
	if _, ok := s.readJSONBodyRaw(w, r, &body); !ok {
		return false
	}
	if !onlyRemote(body.AppearanceIds) {
		s.mixedFaces(w)
		return false
	}
	toPeer(&body.AppearanceIds)
	target, err := translateID(s.peerEditVocabulary(r), domain.VocabularyPerson, body.TargetId, "targetId", true)
	if err != nil {
		s.writeSplitEditError(w, err)
		return false
	}
	body.TargetId = target
	raw, err := json.Marshal(body)
	if err != nil {
		s.internalError(w, "Could not forward the request to the other VVMDM.", err)
		return false
	}
	restoreBody(r, raw)
	return true
}

// peerFaceCandidateSegment turns the other person of /face-candidates/{otherId}/{action} into the peer's person of
// its name; a name the peer lacks has no candidate there.
func (s *server) peerFaceCandidateSegment(w http.ResponseWriter, r *http.Request, segment string) (string, bool) {
	id, err := strconv.ParseInt(segment, 10, 64)
	if err != nil {
		s.invalidRequest(w, "The person id is not a number.")
		return "", false
	}
	peerID, err := translateID(s.peerEditVocabulary(r), domain.VocabularyPerson, id, "otherId", false)
	var missing *domain.CreditNotFoundError
	switch {
	case errors.As(err, &missing) || (err == nil && peerID == 0):
		s.notFoundReason(w, gen.ErrorReasonFaceCandidateNotFound, "The other VVMDM has no such face candidate.")
		return "", false
	case err != nil:
		s.writeSplitEditError(w, err)
		return "", false
	}
	return strconv.FormatInt(peerID, 10), true
}

func (s *server) mixedFaces(w http.ResponseWriter) {
	s.invalidRequestReason(w, gen.ErrorReasonMixedNodes,
		"Faces of both VVMDMs cannot be edited together. Select those of one VVMDM.")
}
