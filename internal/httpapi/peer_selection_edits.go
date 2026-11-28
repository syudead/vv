package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// videoEditParts is an edit in each node's ids; a nil local skips this node's part.
type videoEditParts struct {
	local *domain.VideoEdit
	links []linkEdit
}

// linkEdit is the edit of one link's part in that link's ids; a nil edit changes nothing there, and err is the
// link's failure to take its part, which leaves the other parts to apply.
type linkEdit struct {
	selectionLink
	edit *domain.VideoEdit
	err  error
}

// splitVideoEdit translates edit for each node with targets. It answers w and reports false when the edit
// names a missing entity, or a provisional person for another node, or when the local part needs a link's
// names and that link does not answer.
func (s *server) splitVideoEdit(w http.ResponseWriter, r *http.Request, edit domain.VideoEdit, sel selection) (videoEditParts, bool) {
	var parts videoEditParts
	entries := s.linkEntryCache(r)
	if len(sel.local) > 0 {
		local, err := translateEdit(edit, s.localEditVocabulary(r, entries))
		if err != nil {
			s.writeSplitEditError(w, err)
			return parts, false
		}
		if editChanges(local) {
			parts.local = &local
		}
	}
	if len(edit.Merges) > 0 {
		if parts.local == nil {
			parts.local = &domain.VideoEdit{}
		}
		parts.local.Merges = edit.Merges
	}
	for _, link := range sel.links {
		part := linkEdit{selectionLink: link}
		peer, err := translateEdit(edit, s.linkEditVocabulary(r, link.slot, entries))
		switch {
		case errors.Is(err, errPeerPart):
			part.err = err
		case err != nil:
			s.writeSplitEditError(w, err)
			return parts, false
		case editChanges(peer):
			part.edit = &peer
		}
		parts.links = append(parts.links, part)
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

// editOnPeer applies each link's part of a bulk edit and adds its result to out; the tags the first answering link
// resolved stand in for the local ones when there was no local part.
func (s *server) editOnPeer(r *http.Request, parts videoEditParts, out *gen.VideoEditResponse) {
	tagsSet := parts.local != nil
	for _, link := range parts.links {
		if link.err != nil || link.edit == nil {
			addRemoteResult(&out.Remote, s.remoteSelectionResult(link.slot, 0, link.peer, link.err))
			continue
		}
		var answer gen.VideoEditResponse
		err := s.peerCall(r, link.slot, http.MethodPost, "/api/video-edits", nil, videoEditRequestOf(*link.edit, link.selectionLink), &answer)
		addRemoteResult(&out.Remote, s.remoteSelectionResult(link.slot, answer.Applied, link.peer, err))
		if err != nil {
			continue
		}
		out.Applied += answer.Applied
		if tagsSet {
			continue
		}
		tagsSet = true
		join, err := s.vocabularyJoin(r.Context(), link.slot)
		if err != nil {
			s.logger.Warn("could not join a read VVMDM's tags", slog.Any("error", err))
			continue
		}
		rewriteRemoteVideoEditResponse(&answer, join)
		out.Tags = append(out.Tags, answer.Tags...)
	}
}

// rewriteRemoteVideoEditResponse leaves out what editOnPeer does not report from a link: its merges, skipped people
// and remote part, which a request of this node never gives the link.
func rewriteRemoteVideoEditResponse(answer *gen.VideoEditResponse, join domain.VocabularyJoin) {
	for i := range answer.Tags {
		rewriteRemoteTagRef(&answer.Tags[i], join)
	}
	answer.Merged, answer.SkippedPersonIds, answer.Remote = nil, nil, nil
}

// videoEditRequestOf is edit as the request the link takes for its part.
func videoEditRequestOf(edit domain.VideoEdit, link selectionLink) gen.VideoEditRequest {
	ids, folders := link.peerTargets()
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

// rewriteRemoteVideoTagsResponse leaves out the link's remote part, which a request of this node never gives it.
func rewriteRemoteVideoTagsResponse(answer *gen.VideoTagsResponse, join domain.VocabularyJoin) {
	rewriteRemoteTagRef(&answer.Tag, join)
	answer.Remote = nil
}

// rewriteRemoteTagRef gives the peer's tag the same-named local tag's id, else the offset; the kind is copied first,
// since the answer may share it.
func rewriteRemoteTagRef(ref *gen.TagRef, join domain.VocabularyJoin) {
	ref.Id = join.Tag(ref.Id, ref.Name)
	if ref.Kind != nil {
		kind := *ref.Kind
		rewriteRemoteKindRef(&kind, join)
		ref.Kind = &kind
	}
}

func rewriteRemoteTagsSummary(summary *gen.VideoTagsSummary, join domain.VocabularyJoin) {
	for i := range summary.Items {
		rewriteRemoteTagRef(&summary.Items[i].Tag, join)
	}
}

// rewriteRemoteEditSummary leaves out the image revision of a person joined to a local one, since it names the peer's
// image.
func rewriteRemoteEditSummary(summary *gen.VideoEditSummary, join domain.VocabularyJoin) {
	for i := range summary.Tags {
		rewriteRemoteTagRef(&summary.Tags[i].Tag, join)
	}
	for i := range summary.People {
		person := &summary.People[i]
		person.Person.Id = join.Person(person.Person.Id, person.Person.Name)
		if !domain.IsRemoteID(person.Person.Id) {
			person.ImageRevision = nil
		}
	}
	for i := range summary.Organizations {
		rewriteRemoteOrganization(&summary.Organizations[i].Organization, join)
	}
}

// tagSummaryItems are the rewritten summary items of a link.
func tagSummaryItems(items []gen.VideoTagsSummaryItem) []domain.TagSummaryItem {
	out := make([]domain.TagSummaryItem, len(items))
	for i, item := range items {
		ref := item.Tag
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

// editSummary is a link's rewritten summary.
func editSummary(summary gen.VideoEditSummary) domain.VideoEditSummary {
	out := domain.VideoEditSummary{
		Total: summary.Total, Public: summary.Public, Tags: tagSummaryItems(summary.Tags),
	}
	for _, p := range summary.People {
		item := domain.VideoEditPersonCount{
			Role: domain.PersonRole(p.Role), Count: p.Count,
			Person: domain.CreditRef{ID: p.Person.Id, Name: p.Person.Name, Provisional: p.Person.Provisional},
		}
		if p.ImageRevision != nil {
			item.ImageRevision = *p.ImageRevision
		}
		out.People = append(out.People, item)
	}
	for _, o := range summary.Organizations {
		out.Organizations = append(out.Organizations, domain.VideoEditOrganizationCount{
			Role: domain.OrganizationRole(o.Role), Count: o.Count,
			Organization: domain.CreditRef{ID: o.Organization.Id, Name: o.Organization.Name},
		})
	}
	for _, d := range summary.Dates {
		out.Dates = append(out.Dates, domain.VideoEditDateCount{Kind: domain.DateKind(d.Kind), Count: d.Count, Date: valueOrEmpty(d.Date)})
	}
	return out
}
