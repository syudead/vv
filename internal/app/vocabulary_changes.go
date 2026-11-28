package app

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/syudead/vv/internal/domain"
)

// VocabularyChanges writes a rename, promote, unnaming or deletion of a tag, person or organization on every
// link by name in slot order and then here: once every link is connected and, for a rename or promote, no node
// holds the new name, so a link that is down or a name taken leaves every node unchanged; a link that refuses
// after earlier links applied the change stops it there, since a deletion has no inverse
// (docs/design-docs/peer-library.md#vocabulary-by-name).
type VocabularyChanges struct {
	tags    VocabularyTags
	credits VocabularyCredits
	joins   VocabularyJoins
	// tagNames holds a local tag's rename or promote from the links' write through the local write, and every
	// other local write that can give, change or take away a tag's name waits for it, so the local write cannot
	// fail after the links changed (docs/design-docs/tag-organizing.md#promote-reaches-the-paired-nodes).
	tagNames sync.Mutex
}

// VocabularyTags is this node's tag store as a change by name reads and writes it.
type VocabularyTags interface {
	// TagNames are the canonical names of the existing tags among ids.
	TagNames(ctx context.Context, ids []int64) (map[int64]string, error)
	// LookupTagName finds the tag holding the normalized name as its canonical name or a synonym.
	LookupTagName(ctx context.Context, normalized string) (domain.TagRef, bool, error)
	RenameTag(ctx context.Context, id int64, name string) (domain.Tag, error)
	PromoteSynonym(ctx context.Context, id int64, name string) (domain.Tag, error)
}

// VocabularyCredits is this node's person and organization store as a change by name reads and writes it.
type VocabularyCredits interface {
	Get(ctx context.Context, kind domain.CreditKind, id int64) (domain.Credit, error)
	// Names are the canonical names of the existing entries among ids; a provisional person has none.
	Names(ctx context.Context, kind domain.CreditKind, ids []int64) (map[int64]string, error)
	Update(ctx context.Context, kind domain.CreditKind, id int64, patch domain.CreditPatch) (domain.Credit, error)
}

// VocabularyJoins holds this node's people and organizations by name.
type VocabularyJoins interface {
	VocabularyJoin(ctx context.Context) (domain.VocabularyJoin, error)
}

type VocabularyChangesOptions struct {
	Tags    VocabularyTags
	Credits VocabularyCredits
	// Nil Joins holds no names, so no person or organization name here refuses a rename.
	Joins VocabularyJoins
}

func NewVocabularyChanges(opts VocabularyChangesOptions) *VocabularyChanges {
	return &VocabularyChanges{tags: opts.Tags, credits: opts.Credits, joins: opts.Joins}
}

// HoldTagNames takes the tag-name lock for a local write that can give, change or take away a tag's name, and
// returns its unlock for a deferred call.
func (v *VocabularyChanges) HoldTagNames() (unlock func()) {
	v.tagNames.Lock()
	return v.tagNames.Unlock
}

// RenameTag renames every link's tag of id's canonical name, then id, under the tag-name lock. A name the rule
// refuses, a tag that is not here, or a rename to the current name is left to the local rename's answer.
func (v *VocabularyChanges) RenameTag(ctx context.Context, links domain.VocabularyLinks, id int64, name string) (domain.Tag, error) {
	defer v.HoldTagNames()()
	err := v.tagNameOnLinks(ctx, links, id, name, domain.VocabularyRename, func(current, normalized string) (bool, error) {
		if current == normalized {
			return false, nil
		}
		return true, v.tagNameFree(ctx, normalized)
	})
	if err != nil {
		return domain.Tag{}, err
	}
	return v.tags.RenameTag(ctx, id, name)
}

// PromoteTagSynonym makes name the canonical name of every link's tag of id's canonical name, then promotes it
// on id, under the tag-name lock, so tags stay joined by canonical name. A name that is not id's synonym is left
// to the local promote's answer.
func (v *VocabularyChanges) PromoteTagSynonym(ctx context.Context, links domain.VocabularyLinks, id int64, name string) (domain.Tag, error) {
	defer v.HoldTagNames()()
	err := v.tagNameOnLinks(ctx, links, id, name, domain.VocabularyPromote, func(_, normalized string) (bool, error) {
		holder, held, err := v.tags.LookupTagName(ctx, normalized)
		if err != nil {
			return false, fmt.Errorf("look up the tag name: %w", err)
		}
		holds := &domain.TagNameHolder{TagID: holder.ID, Canonical: holder.Name == normalized}
		return held && domain.CheckSynonymOf(id, holds) == nil, nil
	})
	if err != nil {
		return domain.Tag{}, err
	}
	return v.tags.PromoteSynonym(ctx, id, name)
}

// tagNameOnLinks sends action, which gives every link's tag of id's canonical name the normalized name, once
// check, given the current and the normalized name, says to send it.
func (v *VocabularyChanges) tagNameOnLinks(
	ctx context.Context, links domain.VocabularyLinks, id int64, name string, action domain.VocabularyAction,
	check func(current, normalized string) (bool, error),
) error {
	normalized, err := domain.NormalizeTagName(name)
	if err != nil {
		return nil
	}
	states, err := connectedLinks(ctx, links)
	if err != nil || len(states) == 0 {
		return err
	}
	names, err := v.tags.TagNames(ctx, []int64{id})
	if err != nil {
		return fmt.Errorf("load the tag: %w", err)
	}
	current, found := names[id]
	if !found {
		return nil
	}
	if send, err := check(current, normalized); err != nil || !send {
		return err
	}
	holderOf := func(entries []domain.LinkVocabularyEntry) (string, bool) {
		return renameHolder(entries, domain.VocabularyTag, current, normalized)
	}
	if action == domain.VocabularyPromote {
		holderOf = func(entries []domain.LinkVocabularyEntry) (string, bool) {
			return promoteHolder(entries, current, normalized)
		}
	}
	if err := nameFreeOnLinks(ctx, links, states, domain.VocabularyTag, normalized, 0, holderOf); err != nil {
		return err
	}
	_, err = sendToLinks(ctx, links, states, domain.VocabularyChange{
		Kind: domain.VocabularyTag, Action: action, Name: current, NewName: normalized,
	}, 0)
	return err
}

// tagNameFree refuses the normalized name when a tag here holds it.
func (v *VocabularyChanges) tagNameFree(ctx context.Context, normalized string) error {
	owner, taken, err := v.tags.LookupTagName(ctx, normalized)
	switch {
	case err != nil:
		return fmt.Errorf("look up the tag name: %w", err)
	case taken:
		return &domain.VocabularyNameTakenError{Kind: domain.VocabularyTag, NewName: normalized, Holder: owner.Name}
	}
	return nil
}

// RenameRemoteTag runs before the rename of the link's tag of the remote id is forwarded to that link: it
// refuses a name a tag here holds and renames the tag of the same name on the other links first. A name the rule
// refuses is left to the link's answer.
func (v *VocabularyChanges) RenameRemoteTag(ctx context.Context, links domain.VocabularyLinks, id int64, name string) error {
	normalized, err := domain.NormalizeTagName(name)
	if err != nil {
		return nil
	}
	if err := v.tagNameFree(ctx, normalized); err != nil {
		return err
	}
	return v.renameOnOtherLinks(ctx, links, domain.VocabularyTag, id, normalized)
}

// UpdateCredit applies patch to id's entry here after, when it changes the display name, every link's entry of
// the current name is renamed, or unnamed for a person the patch leaves without names. The patch is checked
// first, so a list the rule refuses changes no node; a provisional person has no name a link could hold, so only
// the new name is checked on every node.
func (v *VocabularyChanges) UpdateCredit(
	ctx context.Context, links domain.VocabularyLinks, kind domain.CreditKind, id int64, patch domain.CreditPatch,
) (domain.Credit, error) {
	if err := v.creditNameOnLinks(ctx, links, kind, id, patch); err != nil {
		return domain.Credit{}, err
	}
	return v.credits.Update(ctx, kind, id, patch)
}

func (v *VocabularyChanges) creditNameOnLinks(
	ctx context.Context, links domain.VocabularyLinks, kind domain.CreditKind, id int64, patch domain.CreditPatch,
) error {
	if patch.Name == nil && patch.Names == nil {
		return nil
	}
	states, err := connectedLinks(ctx, links)
	if err != nil || len(states) == 0 {
		return err
	}
	current, err := v.credits.Get(ctx, kind, id)
	if err != nil {
		return nil
	}
	next, err := domain.PatchCredit(current, patch)
	if err != nil {
		return err
	}
	newName := domain.DisplayNameOf(next.Names)
	vocabulary := domain.VocabularyKindOf(kind)
	change := domain.VocabularyChange{Kind: vocabulary, Action: domain.VocabularyRename, Name: current.Name, NewName: newName}
	switch newName {
	case current.Name:
		return nil
	case "":
		change = domain.VocabularyChange{Kind: vocabulary, Action: domain.VocabularyUnname, Name: current.Name}
	default:
		if err := v.creditNameFree(ctx, kind, id, newName); err != nil {
			return err
		}
		if err := nameFreeOnLinks(ctx, links, states, vocabulary, newName, 0, renameHolderOf(vocabulary, current.Name, newName)); err != nil {
			return err
		}
		if current.Name == "" {
			return nil
		}
	}
	_, err = sendToLinks(ctx, links, states, change, 0)
	return err
}

// creditNameFree refuses name when an entry of kind here other than id holds it.
func (v *VocabularyChanges) creditNameFree(ctx context.Context, kind domain.CreditKind, id int64, name string) error {
	if v.joins == nil {
		return nil
	}
	join, err := v.joins.VocabularyJoin(ctx)
	if err != nil {
		return fmt.Errorf("read the names of people and organizations: %w", err)
	}
	holders := join.People
	if kind == domain.CreditOrganization {
		holders = join.Organizations
	}
	if holder, taken := holders[name]; taken && holder != id {
		return &domain.VocabularyNameTakenError{Kind: domain.VocabularyKindOf(kind), NewName: name, Holder: name}
	}
	return nil
}

// RenameRemoteCredit runs before a patch of the link's person or organization of the remote id is forwarded to
// that link, newName being the normalized display name the patch sets, "" for a person's emptied list. A name
// that is already the entry's display name renames nothing; otherwise a name an entry here holds is refused, and
// the entry of the same name on the other links is renamed or unnamed first.
func (v *VocabularyChanges) RenameRemoteCredit(
	ctx context.Context, links domain.VocabularyLinks, kind domain.CreditKind, id int64, newName string,
) error {
	vocabulary := domain.VocabularyKindOf(kind)
	if newName != "" {
		kept, err := v.remoteNameKept(ctx, links, vocabulary, id, newName)
		if err != nil || kept {
			return err
		}
		if err := v.creditNameFree(ctx, kind, 0, newName); err != nil {
			return err
		}
	}
	return v.renameOnOtherLinks(ctx, links, vocabulary, id, newName)
}

// remoteNameKept reports whether name is already the display name of the remote id's entry, as when an edit
// changes only the other names: that renames nothing, so this node's entry of the name is its own join.
func (v *VocabularyChanges) remoteNameKept(
	ctx context.Context, links domain.VocabularyLinks, kind domain.VocabularyKind, id int64, name string,
) (bool, error) {
	if id == 0 {
		return false, nil
	}
	states, err := links.Links(ctx)
	if err != nil {
		return false, fmt.Errorf("read the pairing: %w", err)
	}
	names, err := v.names(ctx, links, states, kind, []int64{id})
	return names[id] == name, err
}

// renameOnOtherLinks renames, by name, the entry of the remote id on every other link before the rename is
// forwarded to the id's own link, so one name stays one entry on every node. With one link it does nothing, and
// the forward answers as before. An entry without a name, a provisional person, has nothing to rename elsewhere,
// so only the new name is checked there; an empty newName unnames a person. An id of 0 names no entry.
func (v *VocabularyChanges) renameOnOtherLinks(
	ctx context.Context, links domain.VocabularyLinks, kind domain.VocabularyKind, id int64, newName string,
) error {
	states, err := links.Links(ctx)
	switch {
	case id == 0 || err != nil || len(states) < 2:
		return nil
	case !allConnected(states):
		return notConnected(states)
	}
	names, err := v.names(ctx, links, states, kind, []int64{id})
	if err != nil {
		return err
	}
	current, named := names[id]
	_, credit := kind.CreditKind()
	skip := domain.RemoteSlot(id)
	change := domain.VocabularyChange{Kind: kind, Action: domain.VocabularyRename, Name: current, NewName: newName}
	switch {
	case !named && credit && newName != "":
		return nameFreeOnLinks(ctx, links, states, kind, newName, skip, renameHolderOf(kind, "", newName))
	case !named || current == newName:
		return nil
	case newName == "":
		change = domain.VocabularyChange{Kind: kind, Action: domain.VocabularyUnname, Name: current}
	default:
		if err := nameFreeOnLinks(ctx, links, states, kind, newName, 0, renameHolderOf(kind, current, newName)); err != nil {
			return err
		}
	}
	_, err = sendToLinks(ctx, links, states, change, skip)
	return err
}

// DeleteOnLinks deletes every link's entry of id's name and reports whether the local deletion follows: a link's
// own entry has none here. An id that names nothing is left to the local deletion's answer.
func (v *VocabularyChanges) DeleteOnLinks(
	ctx context.Context, links domain.VocabularyLinks, kind domain.VocabularyKind, id int64,
) (deleteHere bool, err error) {
	states, err := connectedLinks(ctx, links)
	if err != nil || len(states) == 0 {
		return err == nil, err
	}
	names, err := v.names(ctx, links, states, kind, []int64{id})
	if err != nil {
		return false, err
	}
	name, found := names[id]
	if !found {
		return true, nil
	}
	if _, err := sendToLinks(ctx, links, states, domain.VocabularyChange{Kind: kind, Action: domain.VocabularyDelete, Name: name}, 0); err != nil {
		return false, err
	}
	return !domain.IsRemoteID(id), nil
}

// Propagates reports whether a change by name also goes to the links; while a link is not connected nothing is
// written and the error names the first such link in slot order.
func (v *VocabularyChanges) Propagates(ctx context.Context, links domain.VocabularyLinks) (bool, error) {
	states, err := connectedLinks(ctx, links)
	return len(states) > 0, err
}

// Names are the canonical names of ids: this node's from its store and each link's from that link's whole list;
// an id of a slot without a link has no name here.
func (v *VocabularyChanges) Names(
	ctx context.Context, links domain.VocabularyLinks, kind domain.VocabularyKind, ids []int64,
) (map[int64]string, error) {
	states, err := links.Links(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the pairing: %w", err)
	}
	return v.names(ctx, links, states, kind, ids)
}

// Send applies change by name on every link but the one in skip, in slot order. A link that refuses or fails
// stops it with a *domain.VocabularyLinkError carrying the change; the links before it stay changed.
func (v *VocabularyChanges) Send(
	ctx context.Context, links domain.VocabularyLinks, change domain.VocabularyChange, skip int64,
) (domain.VocabularyOutcome, error) {
	states, err := links.Links(ctx)
	if err != nil {
		return domain.VocabularyOutcome{}, fmt.Errorf("read the pairing: %w", err)
	}
	return sendToLinks(ctx, links, states, change, skip)
}

func (v *VocabularyChanges) names(
	ctx context.Context, links domain.VocabularyLinks, states []domain.PeerLinkState, kind domain.VocabularyKind, ids []int64,
) (map[int64]string, error) {
	local, remote := domain.SlotIDs(ids)
	names := map[int64]string{}
	if len(local) > 0 {
		var err error
		if creditKind, ok := kind.CreditKind(); ok {
			names, err = v.credits.Names(ctx, creditKind, local)
		} else {
			names, err = v.tags.TagNames(ctx, local)
		}
		if err != nil {
			return nil, fmt.Errorf("load the names: %w", err)
		}
	}
	for _, slot := range slices.Sorted(maps.Keys(remote)) {
		entries, err := links.Entries(ctx, slot, kind)
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, &domain.VocabularyLinkError{Link: linkName(states, slot), Err: err}
		}
		for _, entry := range entries {
			if entry.Name != "" && slices.Contains(remote[slot], entry.ID) {
				names[domain.RemoteID(slot, entry.ID)] = entry.Name
			}
		}
	}
	return names, nil
}

// connectedLinks are the links a change by name writes, in slot order, once every one is connected.
func connectedLinks(ctx context.Context, links domain.VocabularyLinks) ([]domain.PeerLinkState, error) {
	states, err := links.Links(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the pairing: %w", err)
	}
	if !allConnected(states) {
		return nil, notConnected(states)
	}
	return states, nil
}

func allConnected(states []domain.PeerLinkState) bool {
	return !slices.ContainsFunc(states, func(link domain.PeerLinkState) bool { return link.State != domain.PeerConnected })
}

// notConnected names the first link in slot order that is not connected.
func notConnected(states []domain.PeerLinkState) error {
	for _, link := range states {
		if link.State != domain.PeerConnected {
			return &domain.VocabularyLinkError{Link: link.Link.Name, Err: domain.ErrPeerUnreachable}
		}
	}
	return nil
}

// nameFreeOnLinks refuses newName when a link other than the one in skip would refuse it, holderOf naming the
// entry of a link's list that holds it, as that link's own rule does. It runs before any link is written, so a
// taken name leaves every node unchanged and the owner can repeat the change once the name is free.
func nameFreeOnLinks(
	ctx context.Context, links domain.VocabularyLinks, states []domain.PeerLinkState, kind domain.VocabularyKind,
	newName string, skip int64, holderOf func([]domain.LinkVocabularyEntry) (string, bool),
) error {
	for _, link := range states {
		if link.Link.Slot == skip {
			continue
		}
		entries, err := links.Entries(ctx, link.Link.Slot, kind)
		if err != nil {
			return &domain.VocabularyLinkError{Link: link.Link.Name, Err: err}
		}
		if holder, taken := holderOf(entries); taken {
			return &domain.VocabularyNameTakenError{Link: link.Link.Name, Kind: kind, NewName: newName, Holder: holder}
		}
	}
	return nil
}

func sendToLinks(
	ctx context.Context, links domain.VocabularyLinks, states []domain.PeerLinkState, change domain.VocabularyChange, skip int64,
) (domain.VocabularyOutcome, error) {
	var out domain.VocabularyOutcome
	for _, link := range states {
		if link.Link.Slot == skip {
			continue
		}
		outcome, err := links.Apply(ctx, link.Link, change)
		if err != nil {
			return domain.VocabularyOutcome{}, &domain.VocabularyLinkError{Link: link.Link.Name, Change: &change, Err: err}
		}
		out.Changed = out.Changed || outcome.Changed
		out.Skipped = append(out.Skipped, outcome.Skipped...)
	}
	return out, nil
}

// renameHolderOf is renameHolder over a link's entries.
func renameHolderOf(kind domain.VocabularyKind, name, newName string) func([]domain.LinkVocabularyEntry) (string, bool) {
	return func(entries []domain.LinkVocabularyEntry) (string, bool) {
		return renameHolder(entries, kind, name, newName)
	}
}

// renameHolder is the canonical name of the entry, other than name's, that holds newName as its canonical name or
// a tag's synonym; a name several entries hold names the lowest id, as the store's rule does. An empty name, a
// provisional person's, names no entry, so any holder refuses.
func renameHolder(entries []domain.LinkVocabularyEntry, kind domain.VocabularyKind, name, newName string) (string, bool) {
	var renamed, holder *domain.LinkVocabularyEntry
	for i := range entries {
		entry := &entries[i]
		if name != "" && entry.Name == name && (renamed == nil || entry.ID < renamed.ID) {
			renamed = entry
		}
		holds := entry.Name == newName || (kind == domain.VocabularyTag && slices.Contains(entry.Synonyms, newName))
		if holds && (holder == nil || entry.ID < holder.ID) {
			holder = entry
		}
	}
	if holder == nil || (renamed != nil && renamed.ID == holder.ID) {
		return "", false
	}
	return holder.Name, true
}

// promoteHolder is renameHolder for a tag promote, except that a tag whose canonical name is newName and which
// holds name as a synonym holds nothing: an earlier promote applied on that link, which its promote answers
// unchanged.
func promoteHolder(entries []domain.LinkVocabularyEntry, name, newName string) (string, bool) {
	for _, entry := range entries {
		if entry.Name == newName && slices.Contains(entry.Synonyms, name) {
			return "", false
		}
	}
	return renameHolder(entries, domain.VocabularyTag, name, newName)
}

func linkName(states []domain.PeerLinkState, slot int64) string {
	for _, link := range states {
		if link.Link.Slot == slot {
			return link.Link.Name
		}
	}
	return ""
}
