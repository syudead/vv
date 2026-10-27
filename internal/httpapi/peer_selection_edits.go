package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// videoEditParts is an edit in each node's ids; nil skips a node's part, and peerErr is the peer's failure to
// take its part, which leaves the local part to apply.
type videoEditParts struct {
	local, peer *domain.VideoEdit
	peerErr     error
}

// splitVideoEdit translates edit for each node with targets. It answers w and reports false when the edit
// names a missing entity, or a provisional person for the other node, or when the local part needs the peer's
// names and the peer does not answer.
func (s *server) splitVideoEdit(w http.ResponseWriter, r *http.Request, edit domain.VideoEdit, sel selection) (videoEditParts, bool) {
	var parts videoEditParts
	if len(sel.local) > 0 {
		local, err := translateEdit(edit, s.localEditVocabulary(r))
		if err != nil {
			s.writeSplitEditError(w, err)
			return parts, false
		}
		if editChanges(local) {
			parts.local = &local
		}
	}
	if sel.hasPeer() {
		peer, err := translateEdit(edit, s.peerEditVocabulary(r))
		switch {
		case errors.Is(err, errPeerPart):
			parts.peerErr = err
		case err != nil:
			s.writeSplitEditError(w, err)
			return parts, false
		case editChanges(peer):
			parts.peer = &peer
		}
	}
	return parts, true
}

func (s *server) writeSplitEditError(w http.ResponseWriter, err error) {
	var provisional *provisionalPersonError
	switch {
	case errors.As(err, &provisional):
		reason := gen.ErrorReasonProvisionalPersonRemote
		s.writeErrorBody(w, http.StatusBadRequest, gen.Error{
			Code: codeInvalidRequest, Reason: &reason, Field: &provisional.field,
			Message: "A person without a name cannot be added to or removed from the other VVMDM's videos.",
		})
	case errors.Is(err, errPeerPart):
		s.logger.Info("could not read the other VVMDM's names", slog.Any("error", err))
		s.peerUnreachable(w)
	default:
		s.writeVideoEditError(w, err)
	}
}

// editChanges reports whether edit still changes anything after the entities a node lacks were dropped.
func editChanges(edit domain.VideoEdit) bool {
	return len(edit.TagsAdd)+len(edit.TagsRemove)+len(edit.Credits)+len(edit.Dates) > 0 || edit.Public != nil
}

// editOnPeer applies the peer's part of a bulk edit and adds its result to out; the tags the peer resolved
// stand in for the local ones when there was no local part.
func (s *server) editOnPeer(r *http.Request, sel selection, parts videoEditParts, out *gen.VideoEditResponse) {
	if parts.peerErr != nil || parts.peer == nil {
		out.Remote = s.remoteSelectionResult(0, sel.peer, parts.peerErr)
		return
	}
	var answer gen.VideoEditResponse
	err := s.peerCall(r, http.MethodPost, "/api/video-edits", nil, videoEditRequestOf(*parts.peer, sel), &answer)
	out.Remote = s.remoteSelectionResult(answer.Applied, sel.peer, err)
	if err != nil {
		return
	}
	out.Applied += answer.Applied
	if parts.local != nil {
		return
	}
	join, err := s.vocabularyJoin(r.Context())
	if err != nil {
		s.logger.Warn("could not join the other VVMDM's tags", slog.Any("error", err))
		return
	}
	for _, ref := range answer.Tags {
		out.Tags = append(out.Tags, peerTagRef(ref, join))
	}
}

// videoEditRequestOf is edit as the request the peer takes for its part of sel.
func videoEditRequestOf(edit domain.VideoEdit, sel selection) gen.VideoEditRequest {
	ids, folders := sel.peerTargets()
	out := gen.VideoEditRequest{VideoIds: ids, SmartFolderIds: folders, Public: edit.Public}
	if len(edit.TagsAdd)+len(edit.TagsRemove) > 0 {
		tags := gen.VideoTagEdit{}
		if len(edit.TagsAdd) > 0 {
			add := make([]gen.TagInput, len(edit.TagsAdd))
			for i, input := range edit.TagsAdd {
				add[i] = gen.TagInput{Id: input.ID, Name: input.Name}
			}
			tags.Add = &add
		}
		if len(edit.TagsRemove) > 0 {
			tags.Remove = &edit.TagsRemove
		}
		out.Tags = &tags
	}
	if len(edit.Credits) > 0 {
		credits := make([]gen.VideoCreditEdit, len(edit.Credits))
		for i, credit := range edit.Credits {
			credits[i] = gen.VideoCreditEdit{
				Kind: gen.VideoCreditEditKind(credit.Kind), Role: credit.Role, Add: optionalIDs64(credit.Add), Remove: optionalIDs64(credit.Remove),
			}
		}
		out.Credits = &credits
	}
	if len(edit.Dates) > 0 {
		dates := make([]gen.VideoDateEdit, len(edit.Dates))
		for i, date := range edit.Dates {
			dates[i] = gen.VideoDateEdit{Kind: gen.WorkDateKind(date.Kind), Date: date.Date}
		}
		out.Dates = &dates
	}
	return out
}

func optionalIDs64(ids []int64) *[]int64 {
	if len(ids) == 0 {
		return nil
	}
	return &ids
}

// peerTagRef is the peer's tag in this node's ids: the same-named local tag's, else the offset.
func peerTagRef(ref gen.TagRef, join domain.VocabularyJoin) gen.TagRef {
	ref.Id = join.Tag(ref.Id, ref.Name)
	if ref.Kind != nil {
		kind := *ref.Kind
		rewriteRemoteKindRef(&kind, join)
		ref.Kind = &kind
	}
	return ref
}

func peerTagSummaryItems(items []gen.VideoTagsSummaryItem, join domain.VocabularyJoin) []domain.TagSummaryItem {
	out := make([]domain.TagSummaryItem, len(items))
	for i, item := range items {
		ref := peerTagRef(item.Tag, join)
		out[i] = domain.TagSummaryItem{
			Tag:   domain.TagRef{ID: ref.Id, Name: ref.Name, Tentative: ref.Tentative},
			Count: item.Count, ManualCount: item.ManualCount,
		}
		if ref.Kind != nil {
			out[i].Tag.Kind = &domain.TagKindRef{ID: ref.Kind.Id, Name: ref.Kind.Name, CardFirst: ref.Kind.CardFirst}
		}
	}
	return out
}

// peerEditSummary is the peer's summary in this node's ids. A person joined to a local one leaves out the peer's
// image revision, which names the peer's image.
func peerEditSummary(summary gen.VideoEditSummary, join domain.VocabularyJoin) domain.VideoEditSummary {
	out := domain.VideoEditSummary{
		Total: summary.Total, Public: summary.Public, Tags: peerTagSummaryItems(summary.Tags, join),
	}
	for _, p := range summary.People {
		item := domain.VideoEditPersonCount{
			Role: domain.PersonRole(p.Role), Count: p.Count,
			Person: domain.CreditRef{ID: join.Person(p.Person.Id, p.Person.Name), Name: p.Person.Name, Provisional: p.Person.Provisional},
		}
		if p.ImageRevision != nil && domain.IsRemoteID(item.Person.ID) {
			item.ImageRevision = *p.ImageRevision
		}
		out.People = append(out.People, item)
	}
	for _, o := range summary.Organizations {
		out.Organizations = append(out.Organizations, domain.VideoEditOrganizationCount{
			Role: domain.OrganizationRole(o.Role), Count: o.Count,
			Organization: domain.CreditRef{ID: join.Organization(o.Organization.Id, o.Organization.Name), Name: o.Organization.Name},
		})
	}
	for _, d := range summary.Dates {
		out.Dates = append(out.Dates, domain.VideoEditDateCount{Kind: domain.DateKind(d.Kind), Count: d.Count, Date: valueOrEmpty(d.Date)})
	}
	return out
}
