package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// An automatic merge records what it moved so undo can move it back; see docs/design-docs/face-recognition.md.

// faceMergeSnapshot is what a face_merge moved from its source other than credits, and what it gave the target.
type faceMergeSnapshot struct {
	CreatedAt     int64                   `json:"createdAt"`
	ImageRevision int64                   `json:"imageRevision"`
	Profile       domain.PersonProfile    `json:"profile"`
	FavoritedAt   *int64                  `json:"favoritedAt,omitempty"`
	Image         *faceMergeImage         `json:"image,omitempty"`
	Vector        *faceMergeVector        `json:"vector,omitempty"`
	Appearances   []int64                 `json:"appearances"`
	Candidates    []faceMergeCandidate    `json:"candidates"`
	SmartFolders  []int64                 `json:"smartFolders"`
	Target        faceMergeTargetSnapshot `json:"target"`
}

type faceMergeImage struct {
	JPEG          []byte `json:"jpeg"`
	Width, Height int
	UpdatedAt     int64 `json:"updatedAt"`
}

type faceMergeVector struct {
	ImageRevision int64  `json:"imageRevision"`
	Recognizer    string `json:"recognizer"`
	Vector        []byte `json:"vector"`
}

// faceMergeCandidate is one pair of the source; Moved is true when the merge turned it into the target's pair.
type faceMergeCandidate struct {
	ProvisionalID int64   `json:"provisionalId"`
	PersonID      int64   `json:"personId"`
	Similarity    float64 `json:"similarity"`
	Dismissed     bool    `json:"dismissed"`
	CreatedAt     int64   `json:"createdAt"`
	Moved         bool    `json:"moved"`
}

// faceMergeTargetSnapshot holds the target's values before and after the merge, so undo puts back only what the
// merge gave and the owner left as it was.
type faceMergeTargetSnapshot struct {
	ProfileBefore domain.PersonProfile `json:"profileBefore"`
	ProfileAfter  domain.PersonProfile `json:"profileAfter"`
	// FavoriteGiven is the favorite time the merge gave a target that was not a favorite.
	FavoriteGiven *int64 `json:"favoriteGiven,omitempty"`
	// ImageRevision is the target's revision after the merge gave it the source's face, or 0.
	ImageRevision int64 `json:"imageRevision,omitempty"`
}

type faceMergeCredit struct {
	userKey, role string
	targetHad     bool
}

// MergeProvisional merges a provisional person into another person as an automatic face_merge, recording what it
// moved for undo, in one transaction. A source named since the decision returns domain.ErrPersonNotProvisional, and a
// missing person domain.ErrNotFound.
func (s *FaceStore) MergeProvisional(ctx context.Context, sourceID, targetID int64) (domain.Credit, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Credit{}, err
	}
	defer func() { _ = tx.Rollback() }()
	c := &changes{}
	after, events, err := mergeProvisional(ctx, tx, c, sourceID, targetID)
	if err != nil {
		return domain.Credit{}, err
	}
	if err := s.db.commitWith(ctx, tx, c, events...); err != nil {
		return domain.Credit{}, fmt.Errorf("cannot merge person %d into person %d: %w", sourceID, targetID, err)
	}
	return after, nil
}

// mergeProvisional performs MergeProvisional inside tx and returns the target with the events of a face it took.
func mergeProvisional(ctx context.Context, tx *sql.Tx, c *changes, sourceID, targetID int64) (
	domain.Credit, []domain.Event, error,
) {
	t := creditTablesByKind[domain.CreditPerson]
	merged, _, err := mergeSources(ctx, tx, t, targetID, []int64{sourceID})
	if err != nil {
		return domain.Credit{}, nil, err
	}
	if len(merged) == 0 {
		return domain.Credit{}, nil, domain.ErrNotFound
	}
	source, err := creditByID(ctx, tx, t, sourceID)
	if err != nil {
		return domain.Credit{}, nil, err
	}
	if !source.Provisional {
		return domain.Credit{}, nil, domain.ErrPersonNotProvisional
	}
	before, err := creditByID(ctx, tx, t, targetID)
	if err != nil {
		return domain.Credit{}, nil, err
	}
	snapshot, err := readFaceMergeSource(ctx, tx, source, targetID)
	if err != nil {
		return domain.Credit{}, nil, err
	}
	credits, err := readFaceMergeCredits(ctx, tx, sourceID, targetID)
	if err != nil {
		return domain.Credit{}, nil, err
	}
	if err := mergeCreditsInto(ctx, tx, t, targetID, merged); err != nil {
		return domain.Credit{}, nil, err
	}
	if err := checkMergedCreditLimits(ctx, tx, t, targetID); err != nil {
		return domain.Credit{}, nil, err
	}
	after, err := creditByID(ctx, tx, t, targetID)
	if err != nil {
		return domain.Credit{}, nil, err
	}
	if snapshot.Target, err = faceMergeTarget(ctx, tx, before, after); err != nil {
		return domain.Credit{}, nil, err
	}
	if err := recordFaceMerge(ctx, tx, c, sourceID, after, snapshot, credits, time.Now()); err != nil {
		return domain.Credit{}, nil, err
	}
	c.matchesChanged()
	return after, personFaceEvents(before, after), nil
}

// readFaceMergeSource reads what the merge is about to move from the source, other than credits.
func readFaceMergeSource(ctx context.Context, tx *sql.Tx, source domain.Credit, targetID int64) (faceMergeSnapshot, error) {
	snapshot := faceMergeSnapshot{Profile: source.Profile}
	var favoritedAt sql.NullInt64
	// The raw revision, not the domain one that reads 0 without a face, so the restored person never repeats one.
	if err := tx.QueryRowContext(ctx, `select p.created_at, p.image_revision, f.favorited_at from people p
		left join person_favorites f on f.person_id = p.id where p.id = ?`, source.ID).
		Scan(&snapshot.CreatedAt, &snapshot.ImageRevision, &favoritedAt); err != nil {
		return faceMergeSnapshot{}, fmt.Errorf("cannot read person %d: %w", source.ID, err)
	}
	if favoritedAt.Valid {
		snapshot.FavoritedAt = &favoritedAt.Int64
	}
	var err error
	if snapshot.Image, snapshot.Vector, err = readFaceMergeFace(ctx, tx, source.ID); err != nil {
		return faceMergeSnapshot{}, err
	}
	if snapshot.Appearances, err = queryInt64s(ctx, tx, `select id from face_appearances where person_id = ?
		order by id`, source.ID); err != nil {
		return faceMergeSnapshot{}, fmt.Errorf("cannot read the face appearances of person %d: %w", source.ID, err)
	}
	if snapshot.Candidates, err = readFaceMergeCandidates(ctx, tx, source.ID, targetID); err != nil {
		return faceMergeSnapshot{}, err
	}
	if snapshot.SmartFolders, err = smartFoldersOfPerson(ctx, tx, source.ID); err != nil {
		return faceMergeSnapshot{}, err
	}
	return snapshot, nil
}

func readFaceMergeFace(ctx context.Context, tx *sql.Tx, personID int64) (*faceMergeImage, *faceMergeVector, error) {
	var image faceMergeImage
	err := tx.QueryRowContext(ctx, `select image, width, height, updated_at from person_images where person_id = ?`,
		personID).Scan(&image.JPEG, &image.Width, &image.Height, &image.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read the image of person %d: %w", personID, err)
	}
	var vector faceMergeVector
	err = tx.QueryRowContext(ctx, `select image_revision, recognizer, vector from person_face_vectors
		where person_id = ?`, personID).Scan(&vector.ImageRevision, &vector.Recognizer, &vector.Vector)
	if errors.Is(err, sql.ErrNoRows) {
		return &image, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read the face vector of person %d: %w", personID, err)
	}
	return &image, &vector, nil
}

// readFaceMergeCandidates reads the source's pairs; a pair moves unless it joins the target or the target has it.
func readFaceMergeCandidates(ctx context.Context, tx *sql.Tx, sourceID, targetID int64) ([]faceMergeCandidate, error) {
	rows, err := tx.QueryContext(ctx, `select c.provisional_id, c.person_id, c.similarity, c.dismissed, c.created_at,
		c.provisional_id <> ?2 and c.person_id <> ?2 and not exists (select 1 from face_candidates t
			where t.provisional_id = iif(c.provisional_id = ?1, ?2, c.provisional_id)
			  and t.person_id = iif(c.person_id = ?1, ?2, c.person_id))
		from face_candidates c where c.provisional_id = ?1 or c.person_id = ?1
		order by c.provisional_id, c.person_id`, sourceID, targetID)
	if err != nil {
		return nil, fmt.Errorf("cannot read the face candidates of person %d: %w", sourceID, err)
	}
	defer func() { _ = rows.Close() }()
	candidates := []faceMergeCandidate{}
	for rows.Next() {
		var c faceMergeCandidate
		if err := rows.Scan(&c.ProvisionalID, &c.PersonID, &c.Similarity, &c.Dismissed, &c.CreatedAt, &c.Moved); err != nil {
			return nil, fmt.Errorf("cannot read the face candidates of person %d: %w", sourceID, err)
		}
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

func smartFoldersOfPerson(ctx context.Context, tx *sql.Tx, personID int64) ([]int64, error) {
	rules, err := smartFolderRules(ctx, tx)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	for _, folder := range rules {
		if folder.rule.Person == personID {
			ids = append(ids, folder.id)
		}
	}
	return ids, nil
}

func readFaceMergeCredits(ctx context.Context, tx *sql.Tx, sourceID, targetID int64) ([]faceMergeCredit, error) {
	credits, err := queryFaceMergeCredits(ctx, tx, `select s.content_key, s.role, exists (select 1 from video_work_people t
			where t.content_key = s.content_key and t.role = s.role and t.person_id = ?2)
		from video_work_people s where s.person_id = ?1 order by s.content_key, s.role`, sourceID, targetID)
	if err != nil {
		return nil, fmt.Errorf("cannot read the credits of person %d: %w", sourceID, err)
	}
	return credits, nil
}

// queryFaceMergeCredits reads rows of user key, role and target_had.
func queryFaceMergeCredits(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]faceMergeCredit, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var credits []faceMergeCredit
	for rows.Next() {
		var c faceMergeCredit
		if err := rows.Scan(&c.userKey, &c.role, &c.targetHad); err != nil {
			return nil, err
		}
		credits = append(credits, c)
	}
	return credits, rows.Err()
}

func faceMergeTarget(ctx context.Context, tx *sql.Tx, before, after domain.Credit) (faceMergeTargetSnapshot, error) {
	target := faceMergeTargetSnapshot{ProfileBefore: before.Profile, ProfileAfter: after.Profile}
	if before.ImageRevision == 0 && after.ImageRevision != 0 {
		target.ImageRevision = after.ImageRevision
	}
	if before.Favorite || !after.Favorite {
		return target, nil
	}
	var at int64
	if err := tx.QueryRowContext(ctx, `select favorited_at from person_favorites where person_id = ?`, after.ID).
		Scan(&at); err != nil {
		return faceMergeTargetSnapshot{}, fmt.Errorf("cannot read the favorite of person %d: %w", after.ID, err)
	}
	target.FavoriteGiven = &at
	return target, nil
}

func recordFaceMerge(ctx context.Context, tx *sql.Tx, c *changes, sourceID int64, target domain.Credit,
	snapshot faceMergeSnapshot, credits []faceMergeCredit, now time.Time,
) error {
	entryID, err := recordActivity(ctx, tx, c, domain.ActivityEntry{
		Kind: domain.ActivityFaceMerge, Status: domain.ActivityDone, Outcome: domain.ActivityCompleted,
		Decider: domain.ActivityByRule,
		Subject: domain.ActivitySubject{
			Kind: domain.ActivitySubjectPerson, Key: strconv.FormatInt(target.ID, 10), Label: target.Name,
		},
	}, now)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("cannot encode the merge of person %d: %w", sourceID, err)
	}
	if _, err := tx.ExecContext(ctx, `insert into activity_entry_person_merges (entry_id, source_id, target_id, snapshot)
		values (?, ?, ?, ?)`, entryID, sourceID, target.ID, string(encoded)); err != nil {
		return fmt.Errorf("cannot record the merge of person %d: %w", sourceID, err)
	}
	for _, credit := range credits {
		if _, err := tx.ExecContext(ctx, `insert into activity_entry_merge_credits (entry_id, user_key, role, target_had)
			values (?, ?, ?, ?)`, entryID, credit.userKey, credit.role, boolToInt(credit.targetHad)); err != nil {
			return fmt.Errorf("cannot record the merged credits of person %d: %w", sourceID, err)
		}
	}
	return nil
}

// markOwnerChangedMergeCredits marks the credits a work-information write removes from the target of a face_merge
// not yet undone, so undo leaves them as the owner set them.
func markOwnerChangedMergeCredits(ctx context.Context, tx *sql.Tx, userKey string, before, after domain.WorkInfo) error {
	type credit struct {
		role domain.PersonRole
		id   int64
	}
	kept := map[credit]bool{}
	for _, p := range after.People {
		kept[credit{p.Role, p.Person.ID}] = true
	}
	for _, p := range before.People {
		if kept[credit{p.Role, p.Person.ID}] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `update activity_entry_merge_credits set owner_changed = 1
			where user_key = ?1 and role = ?2 and owner_changed = 0 and entry_id in (
				select m.entry_id from activity_entry_person_merges m join activity_entries e on e.id = m.entry_id
				 where m.target_id = ?3 and e.undone_by is null)`, userKey, string(p.Role), p.Person.ID); err != nil {
			return fmt.Errorf("cannot mark the merged credits of person %d: %w", p.Person.ID, err)
		}
	}
	return nil
}

// UndoFaceMerge re-creates the source of an automatic merge under its old id and moves back what the merge moved and
// the owner has not changed since; a target deleted since records already_reverted.
func (s *ActivityStore) UndoFaceMerge(ctx context.Context, id int64) (domain.ActivityEntry, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.ActivityEntry{}, fmt.Errorf("cannot undo the merge: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	entry, err := scanOneActivityEntry(ctx, tx, id)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	if entry.Kind != domain.ActivityFaceMerge || !entry.UndoOffered() {
		return domain.ActivityEntry{}, domain.ErrActivityNotUndoable
	}
	var sourceID, targetID int64
	var raw string
	if err := tx.QueryRowContext(ctx, `select source_id, target_id, snapshot from activity_entry_person_merges
		where entry_id = ?`, id).Scan(&sourceID, &targetID, &raw); err != nil {
		return domain.ActivityEntry{}, fmt.Errorf("cannot read the merge of entry %d: %w", id, err)
	}
	var snapshot faceMergeSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return domain.ActivityEntry{}, fmt.Errorf("cannot read the merge of entry %d: %w", id, err)
	}
	t := creditTablesByKind[domain.CreditPerson]
	existing, err := existingCreditIDs(ctx, tx, t, []int64{sourceID, targetID})
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	restored := existing[targetID] && !existing[sourceID]
	var edited []string
	if restored {
		if edited, err = undoFaceMerge(ctx, tx, id, sourceID, targetID, snapshot); err != nil {
			return domain.ActivityEntry{}, err
		}
	}
	c := &changes{}
	now := time.Now()
	undoID, err := recordUndo(ctx, tx, c, entry, restored, now)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	if err := touchEditedAtForUserKeys(ctx, tx, edited, now); err != nil {
		return domain.ActivityEntry{}, err
	}
	events, err := workInfoChangedEvents(ctx, tx, edited)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	c.matchesChanged()
	if err := s.db.commitWith(ctx, tx, c, events...); err != nil {
		return domain.ActivityEntry{}, fmt.Errorf("cannot undo the merge: %w", err)
	}
	return s.ActivityEntry(ctx, undoID)
}

// undoFaceMerge returns the user keys whose credits it changed.
func undoFaceMerge(ctx context.Context, tx *sql.Tx, entryID, sourceID, targetID int64, snapshot faceMergeSnapshot) ([]string, error) {
	if err := restoreMergedPerson(ctx, tx, sourceID, snapshot); err != nil {
		return nil, err
	}
	if err := revertMergeTarget(ctx, tx, targetID, snapshot.Target); err != nil {
		return nil, err
	}
	if err := restoreMergedFaceRows(ctx, tx, sourceID, targetID, snapshot); err != nil {
		return nil, err
	}
	if err := restoreMergedSmartFolders(ctx, tx, sourceID, targetID, snapshot.SmartFolders); err != nil {
		return nil, err
	}
	return restoreMergedCredits(ctx, tx, entryID, sourceID, targetID)
}

// restoreMergedPerson inserts the source under its old id, which AUTOINCREMENT never hands out again.
func restoreMergedPerson(ctx context.Context, tx *sql.Tx, sourceID int64, snapshot faceMergeSnapshot) error {
	if _, err := tx.ExecContext(ctx, `insert into people (id, created_at, image_revision) values (?, ?, ?)`,
		sourceID, snapshot.CreatedAt, snapshot.ImageRevision); err != nil {
		return fmt.Errorf("cannot restore person %d: %w", sourceID, err)
	}
	if !snapshot.Profile.Equal(domain.PersonProfile{}) {
		if err := writePersonProfile(ctx, tx, sourceID, snapshot.Profile); err != nil {
			return err
		}
	}
	if snapshot.FavoritedAt != nil {
		if _, err := tx.ExecContext(ctx, `insert into person_favorites (person_id, favorited_at) values (?, ?)`,
			sourceID, *snapshot.FavoritedAt); err != nil {
			return fmt.Errorf("cannot restore the favorite of person %d: %w", sourceID, err)
		}
	}
	if image := snapshot.Image; image != nil {
		if _, err := tx.ExecContext(ctx, `insert into person_images (person_id, image, width, height, updated_at)
			values (?, ?, ?, ?, ?)`, sourceID, image.JPEG, image.Width, image.Height, image.UpdatedAt); err != nil {
			return fmt.Errorf("cannot restore the image of person %d: %w", sourceID, err)
		}
	}
	if vector := snapshot.Vector; vector != nil {
		if _, err := tx.ExecContext(ctx, `insert into person_face_vectors (person_id, image_revision, recognizer, vector)
			values (?, ?, ?, ?)`, sourceID, vector.ImageRevision, vector.Recognizer, vector.Vector); err != nil {
			return fmt.Errorf("cannot restore the face vector of person %d: %w", sourceID, err)
		}
	}
	return nil
}

// revertMergeTarget takes back the face, favorite and profile fields the merge gave the target while they still
// hold what it gave.
func revertMergeTarget(ctx context.Context, tx *sql.Tx, targetID int64, target faceMergeTargetSnapshot) error {
	if target.ImageRevision != 0 {
		res, err := tx.ExecContext(ctx, `delete from person_images where person_id = ?1
			and (select image_revision from people where id = ?1) = ?2`, targetID, target.ImageRevision)
		if err != nil {
			return fmt.Errorf("cannot take back the image of person %d: %w", targetID, err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return fmt.Errorf("cannot take back the image of person %d: %w", targetID, err)
		} else if n > 0 {
			if _, err := tx.ExecContext(ctx, `delete from person_face_vectors where person_id = ?`, targetID); err != nil {
				return fmt.Errorf("cannot take back the face vector of person %d: %w", targetID, err)
			}
			if err := bumpPersonImageRevision(ctx, tx, targetID); err != nil {
				return err
			}
		}
	}
	if target.FavoriteGiven != nil {
		if _, err := tx.ExecContext(ctx, `delete from person_favorites where person_id = ? and favorited_at = ?`,
			targetID, *target.FavoriteGiven); err != nil {
			return fmt.Errorf("cannot take back the favorite of person %d: %w", targetID, err)
		}
	}
	current, err := creditByID(ctx, tx, creditTablesByKind[domain.CreditPerson], targetID)
	if err != nil {
		return err
	}
	reverted := domain.RevertFilledProfile(current.Profile, target.ProfileBefore, target.ProfileAfter)
	if reverted.Equal(current.Profile) {
		return nil
	}
	return writePersonProfile(ctx, tx, targetID, reverted)
}

// restoreMergedFaceRows moves the source's appearances back and restores its candidate pairs, removing the target's
// copy of a moved pair the owner has not decided since.
func restoreMergedFaceRows(ctx context.Context, tx *sql.Tx, sourceID, targetID int64, snapshot faceMergeSnapshot) error {
	encoded, err := json.Marshal(snapshot.Appearances)
	if err != nil {
		return fmt.Errorf("cannot build appearance ids: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `update face_appearances set person_id = ?1
		where person_id = ?2 and id in (select value from json_each(?3))`, sourceID, targetID, string(encoded)); err != nil {
		return fmt.Errorf("cannot move the face appearances back to person %d: %w", sourceID, err)
	}
	swap := func(id int64) int64 {
		if id == sourceID {
			return targetID
		}
		return id
	}
	for _, pair := range snapshot.Candidates {
		if pair.Moved {
			if _, err := tx.ExecContext(ctx, `delete from face_candidates
				where provisional_id = ? and person_id = ? and dismissed = ?`,
				swap(pair.ProvisionalID), swap(pair.PersonID), boolToInt(pair.Dismissed)); err != nil {
				return fmt.Errorf("cannot take back a face candidate of person %d: %w", targetID, err)
			}
		}
		// The other person may have been deleted since; its pair goes with it.
		if _, err := tx.ExecContext(ctx, `insert or ignore into face_candidates
			(provisional_id, person_id, similarity, dismissed, created_at)
			select ?1, ?2, ?3, ?4, ?5 where exists (select 1 from people where id = ?1)
			  and exists (select 1 from people where id = ?2)`,
			pair.ProvisionalID, pair.PersonID, pair.Similarity, boolToInt(pair.Dismissed), pair.CreatedAt); err != nil {
			return fmt.Errorf("cannot restore a face candidate of person %d: %w", sourceID, err)
		}
	}
	return nil
}

// restoreMergedSmartFolders points back to the source the rules the merge pointed at the target, unless the owner
// changed their person since.
func restoreMergedSmartFolders(ctx context.Context, tx *sql.Tx, sourceID, targetID int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	rules, err := smartFolderRules(ctx, tx)
	if err != nil {
		return err
	}
	for _, folder := range rules {
		if folder.rule.Person != targetID || !slices.Contains(ids, folder.id) {
			continue
		}
		folder.rule.Person = sourceID
		encoded, err := folder.rule.Encode()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `update smart_folders set rule = ? where id = ?`, encoded, folder.id); err != nil {
			return fmt.Errorf("cannot restore the smart folder rule %d: %w", folder.id, err)
		}
	}
	return nil
}

// restoreMergedCredits gives the source back each credit the owner has not removed from the target since: a credit
// the merge added to the target moves back in place, and one the target already had is added after the role's others.
func restoreMergedCredits(ctx context.Context, tx *sql.Tx, entryID, sourceID, targetID int64) ([]string, error) {
	credits, err := queryFaceMergeCredits(ctx, tx, `select user_key, role, target_had from activity_entry_merge_credits
		where entry_id = ? and owner_changed = 0 order by user_key, role`, entryID)
	if err != nil {
		return nil, fmt.Errorf("cannot read the merged credits of entry %d: %w", entryID, err)
	}
	var edited []string
	for _, c := range credits {
		statement := `update video_work_people set person_id = ?1 where content_key = ?3 and role = ?4 and person_id = ?2`
		if c.targetHad {
			statement = `insert into video_work_people (content_key, role, position, person_id)
				select ?3, ?4, coalesce(max(position) + 1, 0), ?1 from video_work_people where content_key = ?3 and role = ?4`
		}
		res, err := tx.ExecContext(ctx, statement, sourceID, targetID, c.userKey, c.role)
		if err != nil {
			return nil, fmt.Errorf("cannot restore a credit of person %d: %w", sourceID, err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return nil, fmt.Errorf("cannot restore a credit of person %d: %w", sourceID, err)
		} else if n > 0 && (len(edited) == 0 || edited[len(edited)-1] != c.userKey) {
			edited = append(edited, c.userKey)
		}
	}
	return edited, nil
}
