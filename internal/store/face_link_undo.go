package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Undo of automatic face links is described in docs/design-docs/activity.md.

// faceLinkPersonJoin finds the person a link now points to: its appearance's, which follows merges and is null once
// the person is deleted, or the link's own when a re-analysis removed the appearance.
const faceLinkPersonJoin = `
	left join face_appearances a on a.id = l.appearance_id
	left join people p on p.id = case when a.id is null then l.person_id else a.person_id end
	left join people_names n on n.person_id = p.id and n.canonical = 1`

func attachActivityFaceLinks(ctx context.Context, q rowsQueryer, entries []domain.ActivityEntry) error {
	index, encoded := activityEntryIDs(entries, func(entry domain.ActivityEntry) bool {
		return entry.Kind == domain.ActivityFaceAnalysis
	})
	if len(index) == 0 {
		return nil
	}
	rows, err := q.QueryContext(ctx, `select l.entry_id, l.appearance_id, coalesce(p.id, 0), coalesce(n.name, ''),
		n.person_id is null, l.created_person, coalesce(l.undone_by, 0)
		from activity_entry_face_links l`+faceLinkPersonJoin+`
		where l.entry_id in (select value from json_each(?)) order by l.entry_id, l.appearance_id`, encoded)
	if err != nil {
		return fmt.Errorf("cannot read the activity face links: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var entryID int64
		var link domain.ActivityFaceLink
		var person domain.CreditRef
		if err := rows.Scan(&entryID, &link.AppearanceID, &person.ID, &person.Name, &person.Provisional,
			&link.CreatedPerson, &link.UndoneBy); err != nil {
			return fmt.Errorf("cannot read the activity face links: %w", err)
		}
		if person.ID != 0 {
			link.Person = &person
		}
		entry := &entries[index[entryID]]
		entry.FaceLinks = append(entry.FaceLinks, link)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot read the activity face links: %w", err)
	}
	return nil
}

// faceLinkTarget is one link to undo with the current state of its appearance and person.
type faceLinkTarget struct {
	appearanceID, linkedPerson int64
	userKey                    string
	created                    bool
	undoneBy                   int64
	appearanceKept, dropped    bool
	// person is the person the link now points to, zero once deleted.
	person int64
	name   string
}

func readFaceLinkTargets(ctx context.Context, tx *sql.Tx, entryID, appearanceID int64) ([]faceLinkTarget, error) {
	rows, err := tx.QueryContext(ctx, `select l.appearance_id, l.person_id, l.user_key, l.created_person,
		coalesce(l.undone_by, 0), a.id is not null, coalesce(a.dropped, 0), coalesce(p.id, 0), coalesce(n.name, '')
		from activity_entry_face_links l`+faceLinkPersonJoin+`
		where l.entry_id = ?1 and (?2 = 0 or l.appearance_id = ?2) order by l.appearance_id`, entryID, appearanceID)
	if err != nil {
		return nil, fmt.Errorf("cannot read the face links of activity %d: %w", entryID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []faceLinkTarget
	for rows.Next() {
		var t faceLinkTarget
		if err := rows.Scan(&t.appearanceID, &t.linkedPerson, &t.userKey, &t.created, &t.undoneBy, &t.appearanceKept,
			&t.dropped, &t.person, &t.name); err != nil {
			return nil, fmt.Errorf("cannot read the face links of activity %d: %w", entryID, err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the face links of activity %d: %w", entryID, err)
	}
	return out, nil
}

// UndoFaceLinks undoes one link of a face_analysis entry, or every link not yet undone when appearanceID is zero,
// and records the owner's undo entry: already_reverted when the owner removed every one of those performers since.
// An unknown link is domain.ErrNotFound; an entry with no link left to undo is domain.ErrActivityNotUndoable.
func (s *FaceStore) UndoFaceLinks(ctx context.Context, entryID, appearanceID int64) (domain.ActivityEntry, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	defer func() { _ = tx.Rollback() }()
	entry, pending, err := faceLinksToUndo(ctx, tx, entryID, appearanceID)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	c := &changes{}
	var edited []string
	reverted := false
	for _, link := range pending {
		undone, key, err := revertFaceLink(ctx, tx, c, entry.Subject.Key, link)
		if err != nil {
			return domain.ActivityEntry{}, err
		}
		reverted = reverted || undone
		if key != "" {
			edited = append(edited, key)
		}
	}
	now := time.Now()
	undoID, err := recordFaceLinkUndo(ctx, tx, c, entry, pending, appearanceID, reverted, now)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	var events []domain.Event
	if len(edited) > 0 {
		if err := touchEditedAtForUserKeys(ctx, tx, edited, now); err != nil {
			return domain.ActivityEntry{}, err
		}
		if events, err = workInfoChangedEvents(ctx, tx, edited); err != nil {
			return domain.ActivityEntry{}, err
		}
	}
	if err := s.db.commitWith(ctx, tx, c, events...); err != nil {
		return domain.ActivityEntry{}, fmt.Errorf("cannot undo the face links of activity %d: %w", entryID, err)
	}
	return readActivityEntry(ctx, s.db.sql, undoID)
}

// faceLinksToUndo rereads the entry and its links in the transaction and keeps the links still offered for undo.
func faceLinksToUndo(
	ctx context.Context, tx *sql.Tx, entryID, appearanceID int64,
) (domain.ActivityEntry, []faceLinkTarget, error) {
	entry, err := scanOneActivityEntry(ctx, tx, entryID)
	if err != nil {
		return domain.ActivityEntry{}, nil, err
	}
	if entry.Kind != domain.ActivityFaceAnalysis {
		return domain.ActivityEntry{}, nil, domain.ErrActivityNotUndoable
	}
	links, err := readFaceLinkTargets(ctx, tx, entryID, appearanceID)
	if err != nil {
		return domain.ActivityEntry{}, nil, err
	}
	if appearanceID != 0 && len(links) == 0 {
		return domain.ActivityEntry{}, nil, fmt.Errorf("face link %d of activity entry %d: %w", appearanceID, entryID,
			domain.ErrNotFound)
	}
	var pending []faceLinkTarget
	for _, link := range links {
		if entry.FaceLinkUndoOffered(domain.ActivityFaceLink{UndoneBy: link.undoneBy}) {
			pending = append(pending, link)
		}
	}
	if len(pending) == 0 {
		return domain.ActivityEntry{}, nil, domain.ErrActivityNotUndoable
	}
	return entry, pending, nil
}

// revertFaceLink drops the appearance and removes the performer row at the video's current user key, unless another
// appearance of the video still links the person, then deletes the provisional person the run created when nothing
// else credits it. A dropped appearance means the owner removed the performer, undid the link or deleted the person
// since, so the link is left as it is. It returns whether it
// changed anything and the user key whose credits it changed.
func revertFaceLink(
	ctx context.Context, tx *sql.Tx, c *changes, contentKey string, link faceLinkTarget,
) (bool, string, error) {
	if link.dropped {
		return false, "", nil
	}
	if link.appearanceKept {
		if _, err := tx.ExecContext(ctx, `update face_appearances set dropped = 1 where id = ?`, link.appearanceID); err != nil {
			return false, "", fmt.Errorf("cannot drop face appearance %d: %w", link.appearanceID, err)
		}
	}
	if link.person == 0 {
		return link.appearanceKept, "", nil
	}
	keep, err := keepsFacePerformer(ctx, tx, contentKey, link)
	if err != nil || keep {
		return link.appearanceKept, "", err
	}
	key, err := currentUserKey(ctx, tx, contentKey)
	if err != nil {
		return false, "", err
	}
	if key == "" {
		key = link.userKey
	}
	removed, err := removeFacePerformer(ctx, tx, key, link.person)
	if err != nil {
		return false, "", err
	}
	if link.created && link.person == link.linkedPerson {
		if err := deleteUnusedProvisionalPerson(ctx, tx, c, link.person); err != nil {
			return false, "", err
		}
	}
	if !removed {
		return link.appearanceKept, "", nil
	}
	return true, key, nil
}

// keepsFacePerformer reports whether the undo must keep the performer row of the person the link now points to:
// another appearance still links the person, or an automatic merge moved the linked person into one that already
// had the credit, which the link therefore did not add.
func keepsFacePerformer(ctx context.Context, tx *sql.Tx, contentKey string, link faceLinkTarget) (bool, error) {
	linked, err := personStillLinked(ctx, tx, contentKey, link.person)
	if err != nil || linked || link.person == link.linkedPerson {
		return linked, err
	}
	var had bool
	if err := tx.QueryRowContext(ctx, `select exists (select 1 from activity_entry_person_merges m
		join activity_entries e on e.id = m.entry_id
		join activity_entry_merge_credits c on c.entry_id = m.entry_id
		where m.source_id = ? and m.target_id = ? and e.undone_by is null
		  and c.user_key = ? and c.role = ? and c.target_had = 1)`,
		link.linkedPerson, link.person, link.userKey, string(domain.PersonPerformer)).Scan(&had); err != nil {
		return false, fmt.Errorf("cannot read the merges of person %d: %w", link.linkedPerson, err)
	}
	return had, nil
}

// personStillLinked reports whether an appearance of the video not dropped still links the person, which keeps its
// performer row: a credit's origin is the appearances linked to it.
func personStillLinked(ctx context.Context, tx *sql.Tx, contentKey string, personID int64) (bool, error) {
	var linked bool
	if err := tx.QueryRowContext(ctx, `select exists (select 1 from face_appearances
		where content_key = ? and person_id = ? and dropped = 0)`, contentKey, personID).Scan(&linked); err != nil {
		return false, fmt.Errorf("cannot read the face appearances of person %d: %w", personID, err)
	}
	return linked, nil
}

func removeFacePerformer(ctx context.Context, tx *sql.Tx, userKey string, personID int64) (bool, error) {
	res, err := tx.ExecContext(ctx, `delete from video_work_people
		where content_key = ? and role = 'performer' and person_id = ?`, userKey, personID)
	if err != nil {
		return false, fmt.Errorf("cannot remove person %d as a performer: %w", personID, err)
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return false, err
	}
	list := creditLinkList{contentKey: userKey, role: string(domain.PersonPerformer)}
	return true, renumberCreditLinks(ctx, tx, creditTablesByKind[domain.CreditPerson], []creditLinkList{list})
}

// deleteUnusedProvisionalPerson deletes the person, with its face data, while it is still provisional and credited
// nowhere.
func deleteUnusedProvisionalPerson(ctx context.Context, tx *sql.Tx, c *changes, personID int64) error {
	var unused bool
	if err := tx.QueryRowContext(ctx, `select
		not exists (select 1 from people_names where person_id = ?1 and canonical = 1)
		and not exists (select 1 from video_work_people where person_id = ?1)`, personID).Scan(&unused); err != nil {
		return fmt.Errorf("cannot read the credits of person %d: %w", personID, err)
	}
	if !unused {
		return nil
	}
	if err := deletePersonFaceData(ctx, tx, personID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `delete from people where id = ?`, personID); err != nil {
		return fmt.Errorf("cannot delete person %d: %w", personID, err)
	}
	c.matchesChanged()
	return nil
}

// recordFaceLinkUndo writes the owner's undo entry, naming the person when one link was asked for, and marks the
// links undone by it.
func recordFaceLinkUndo(
	ctx context.Context, tx *sql.Tx, c *changes, entry domain.ActivityEntry, links []faceLinkTarget,
	appearanceID int64, reverted bool, now time.Time,
) (int64, error) {
	undo := domain.ActivityEntry{
		Kind: domain.ActivityUndo, Subject: entry.Subject, Status: domain.ActivityDone,
		Outcome: domain.ActivityAlreadyReverted, UndoesID: entry.ID, Decider: domain.ActivityByOwner,
	}
	if reverted {
		undo.Outcome = domain.ActivityCompleted
	}
	if appearanceID != 0 {
		undo.Detail = links[0].name
	}
	undoID, err := recordActivity(ctx, tx, c, undo, now)
	if err != nil {
		return 0, err
	}
	for _, link := range links {
		if _, err := tx.ExecContext(ctx, `update activity_entry_face_links set undone_by = ?
			where entry_id = ? and appearance_id = ?`, undoID, entry.ID, link.appearanceID); err != nil {
			return 0, fmt.Errorf("cannot mark face link %d undone: %w", link.appearanceID, err)
		}
	}
	return undoID, nil
}
