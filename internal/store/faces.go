package store

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Face analysis is described in docs/design-docs/face-recognition.md.

const (
	// Stored as true or false; absent means disabled.
	faceEnabledKey = "face.enabled"
	// An integer increased by a recognizer switch; absent means 0.
	faceGenerationKey = "face.generation"
)

// faceQueueStates lists the queue states a scope queues again; an empty state represents a missing row.
func faceQueueStates(scope domain.FaceQueueScope) (string, error) {
	switch scope {
	case domain.FaceQueueMissing:
		return `'', 'failed'`, nil
	case domain.FaceQueueAll:
		return queueAgainStates, nil
	default:
		return "", fmt.Errorf("unknown face analysis scope %q", scope)
	}
}

// QueueLibrary queues registered content for face analysis with a waiting entry each; running rows never move.
func (s *FaceStore) QueueLibrary(ctx context.Context, scope domain.FaceQueueScope, decider domain.ActivityDecider) (int, error) {
	states, err := faceQueueStates(scope)
	if err != nil {
		return 0, err
	}
	return s.db.queueContent(ctx, faceQueue, `select v.content_key from videos v
		where v.content_key <> '' and `+registeredVideoCondition("v"), nil, states, decider, nil)
}

// RetryVideo queues the content again for the owner's retry of entryID, unless it is running; the new entry joins
// every pipeline of the retried entry.
func (s *FaceStore) RetryVideo(ctx context.Context, contentKey string, entryID int64) (int, error) {
	return s.db.queueContent(ctx, faceQueue, `select v.content_key from videos v
		where v.content_key = ? and v.content_key <> '' and `+registeredVideoCondition("v"), []any{contentKey},
		queueAgainStates, domain.ActivityByOwner, []int64{entryID})
}

// Claim takes the next queued video or returns domain.ErrNoJob, creating a processing activity if necessary. The
// job carries the recognizer and face.generation read in the claim's transaction.
func (s *FaceStore) Claim(ctx context.Context) (domain.FaceJob, error) {
	var job domain.FaceJob
	key, err := s.db.claimContent(ctx, faceQueue, func(tx *sql.Tx) error {
		var err error
		if job.Recognizer, err = faceRecognizer(ctx, tx); err != nil {
			return err
		}
		job.Generation, err = faceGeneration(ctx, tx)
		return err
	})
	if err != nil {
		return domain.FaceJob{}, err
	}
	job.ContentKey = key
	return job, nil
}

// VideoPaths returns the registered locations of the video with the content key, or domain.ErrNotFound when the
// library no longer has it.
func (s *FaceStore) VideoPaths(ctx context.Context, contentKey string) ([]string, error) {
	rows, err := s.db.sql.QueryContext(ctx, `select l.path from videos v join video_locations l on l.video_id = v.id
		where v.content_key = ? and v.content_key <> '' and `+registeredLocationCondition("l")+` order by l.path`,
		contentKey)
	if err != nil {
		return nil, fmt.Errorf("cannot read the locations of the video: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, fmt.Errorf("cannot read the locations of the video: %w", err)
		}
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the locations of the video: %w", err)
	}
	if len(paths) == 0 {
		return nil, domain.ErrNotFound
	}
	return paths, nil
}

// Fail ignores videos no longer running and records the failure with their activity atomically.
func (s *FaceStore) Fail(ctx context.Context, contentKey, reason string) error {
	return s.db.failContent(ctx, faceQueue, contentKey, reason)
}

// RequeueRunning puts running analyses back in the queue, at startup and when the feature is turned off.
func (s *FaceStore) RequeueRunning(ctx context.Context) (int, error) {
	return s.db.requeueRunning(ctx, faceQueue)
}

// Counts counts the face queue's rows by state, with the reason of the latest failure.
func (s *FaceStore) Counts(ctx context.Context) (domain.QueueCounts, error) {
	return s.db.contentCounts(ctx, faceQueue)
}

// Store writes one video's result in one transaction: appearances with carry-over, provisional people with their
// crop as face, performer rows, candidates, the completed face_analysis entry with its links, and the cast rule's
// face_merge. A result computed under an older face.generation returns domain.ErrFaceGenerationChanged; one whose
// row no longer runs writes nothing.
func (s *FaceStore) Store(ctx context.Context, result domain.FaceAnalysis) error {
	spec, ok := domain.LookupFaceRecognizer(result.Recognizer)
	if !ok {
		return fmt.Errorf("unknown face recognizer %q", result.Recognizer)
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	generation, err := faceGeneration(ctx, tx)
	if err != nil {
		return err
	}
	if generation != result.Generation {
		return domain.ErrFaceGenerationChanged
	}
	activityID, running, err := runningContent(ctx, tx, faceQueue, result.ContentKey)
	if err != nil || !running {
		return err
	}
	now := time.Now()
	c := &changes{}
	completed := domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivityCompleted}
	userKey, err := faceUserKey(ctx, tx, result.ContentKey)
	if errors.Is(err, domain.ErrNotFound) {
		// The video left the library during the analysis; its stored appearances stay for a returning file.
		if _, err := tx.ExecContext(ctx, `delete from face_queue where content_key = ?`, result.ContentKey); err != nil {
			return fmt.Errorf("cannot remove the face analysis job: %w", err)
		}
		if err := closeQueueActivity(ctx, tx, c, activityID, completed, "", "", now); err != nil {
			return err
		}
		return s.db.commit(ctx, tx, c)
	}
	if err != nil {
		return err
	}
	links, added, err := writeFaceAppearances(ctx, tx, spec, userKey, result, now)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `update face_queue set state = 'done', recognizer = ?, reason = null,
		updated_at = ? where content_key = ?`, string(result.Recognizer), now.UnixMilli(), result.ContentKey); err != nil {
		return fmt.Errorf("cannot finish the face analysis job: %w", err)
	}
	if err := closeQueueActivity(ctx, tx, c, activityID, completed, domain.ActivityByRule, "", now); err != nil {
		return err
	}
	if err := recordFaceLinks(ctx, tx, activityID, userKey, links); err != nil {
		return err
	}
	// A refused cast merge leaves the result to be stored without it, as a later work-information change would.
	_, events, err := mergeByCast(ctx, tx, c, userKey, now)
	if err != nil && !errors.Is(err, domain.ErrFaceMergeRefused) {
		return err
	}
	if added {
		if err := touchEditedAtForUserKeys(ctx, tx, []string{userKey}, now); err != nil {
			return err
		}
		changed, err := workInfoChangedEvents(ctx, tx, []string{userKey})
		if err != nil {
			return err
		}
		events = append(events, changed...)
	}
	if err := s.db.commitWith(ctx, tx, c, events...); err != nil {
		return fmt.Errorf("cannot store the face analysis: %w", err)
	}
	return nil
}

func faceGeneration(ctx context.Context, q rowQueryer) (int64, error) {
	value, found, err := readSetting(ctx, q, faceGenerationKey)
	if err != nil || !found {
		return 0, err
	}
	generation, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot read setting %s: %w", faceGenerationKey, err)
	}
	return generation, nil
}

func faceUserKey(ctx context.Context, tx *sql.Tx, contentKey string) (string, error) {
	videoID, err := videoIDForContent(ctx, tx, contentKey)
	if err != nil {
		return "", err
	}
	keys, err := userKeysForVideoIDs(ctx, tx, []int64{videoID})
	if err != nil {
		return "", err
	}
	if len(keys) != 1 {
		return "", fmt.Errorf("cannot find the user key of video %d", videoID)
	}
	return keys[0], nil
}

// faceLink is one person a result linked to the video.
type faceLink struct {
	appearanceID, personID int64
	created                bool
}

// writeFaceAppearances replaces the content key's appearances with the result's and reports the links it made and
// whether it added a performer row.
func writeFaceAppearances(
	ctx context.Context, tx *sql.Tx, spec domain.FaceRecognizerSpec, userKey string, result domain.FaceAnalysis,
	now time.Time,
) ([]faceLink, bool, error) {
	stored, err := storedFaceAppearances(ctx, tx, result.ContentKey)
	if err != nil {
		return nil, false, err
	}
	vectors, err := facePersonVectors(ctx, tx, result.Recognizer)
	if err != nil {
		return nil, false, err
	}
	people := make([]domain.MainPerson, len(result.People))
	for i, p := range result.People {
		people[i] = p.MainPerson()
	}
	plans := domain.PlanFaceAppearances(spec.Thresholds, stored, people, vectors)
	kept := map[int64]bool{}
	var links []faceLink
	var created []domain.FacePersonVector
	added := false
	for i, plan := range plans {
		person := result.People[i]
		if plan.CarriedFrom != 0 {
			kept[plan.CarriedFrom] = true
			if err := carryFaceAppearance(ctx, tx, plan.CarriedFrom, result.Recognizer, person); err != nil {
				return nil, false, err
			}
			continue
		}
		link, performerAdded, err := linkFaceAppearance(ctx, tx, result, person, plan.Match, userKey, now)
		if err != nil {
			return nil, false, err
		}
		links = append(links, link)
		if link.created {
			created = append(created, domain.FacePersonVector{PersonID: link.personID, Vector: person.Centroid})
		}
		added = added || performerAdded
	}
	// People made by this one analysis are not in vectors, so the match could not offer them to each other.
	for _, pair := range domain.SiblingFacePairs(spec.Thresholds, created) {
		candidate := []domain.FaceCandidate{{PersonID: pair.PersonID, Similarity: pair.Similarity}}
		if err := writeFaceCandidates(ctx, tx, pair.ProvisionalID, candidate, now); err != nil {
			return nil, false, err
		}
	}
	for _, old := range stored {
		if kept[old.ID] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `delete from face_appearances where id = ?`, old.ID); err != nil {
			return nil, false, fmt.Errorf("cannot delete face appearance %d: %w", old.ID, err)
		}
	}
	return links, added, nil
}

func storedFaceAppearances(ctx context.Context, tx *sql.Tx, contentKey string) ([]domain.StoredFaceAppearance, error) {
	rows, err := tx.QueryContext(ctx, `select id, coalesce(person_id, 0), dropped, tracks from face_appearances
		where content_key = ? order by id`, contentKey)
	if err != nil {
		return nil, fmt.Errorf("cannot read the face appearances: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.StoredFaceAppearance
	for rows.Next() {
		var a domain.StoredFaceAppearance
		var tracks string
		if err := rows.Scan(&a.ID, &a.PersonID, &a.Dropped, &tracks); err != nil {
			return nil, fmt.Errorf("cannot read the face appearances: %w", err)
		}
		if a.Spans, err = decodeFaceTracks(tracks); err != nil {
			return nil, fmt.Errorf("cannot read the tracks of face appearance %d: %w", a.ID, err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the face appearances: %w", err)
	}
	return out, nil
}

// facePersonVectors reads every vector of the recognizer that belongs to a person: the centroids of appearances
// not dropped, and the vectors of current face images.
func facePersonVectors(ctx context.Context, tx *sql.Tx, recognizer domain.FaceRecognizer) ([]domain.FacePersonVector, error) {
	rows, err := tx.QueryContext(ctx, `
		select person_id, centroid from face_appearances
		 where person_id is not null and dropped = 0 and centroid is not null and recognizer = ?1
		union all
		select v.person_id, v.vector from person_face_vectors v join people p on p.id = v.person_id
		 where v.vector is not null and v.recognizer = ?1 and v.image_revision = p.image_revision`,
		string(recognizer))
	if err != nil {
		return nil, fmt.Errorf("cannot read the face vectors: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.FacePersonVector
	for rows.Next() {
		var v domain.FacePersonVector
		var blob []byte
		if err := rows.Scan(&v.PersonID, &blob); err != nil {
			return nil, fmt.Errorf("cannot read the face vectors: %w", err)
		}
		v.Vector = decodeFaceVector(blob)
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the face vectors: %w", err)
	}
	return out, nil
}

// carryFaceAppearance keeps the row's id, person, dropped mark and crop; a row whose person was deleted keeps no centroid.
func carryFaceAppearance(
	ctx context.Context, tx *sql.Tx, id int64, recognizer domain.FaceRecognizer, p domain.FaceAnalysedPerson,
) error {
	tracks, err := encodeFaceTracks(p.Spans)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `update face_appearances set recognizer = ?, tracks = ?, good_seconds = ?,
		track_count = ?, centroid = case when person_id is null then null else ? end where id = ?`,
		string(recognizer), tracks, p.GoodSeconds, len(p.Spans), encodeFaceVector(p.Centroid), id); err != nil {
		return fmt.Errorf("cannot update face appearance %d: %w", id, err)
	}
	return nil
}

// linkFaceAppearance stores a new appearance linked to its matched person, or to a new provisional person whose
// face is the crop, and makes that person a performer of the video.
func linkFaceAppearance(
	ctx context.Context, tx *sql.Tx, result domain.FaceAnalysis, p domain.FaceAnalysedPerson, match domain.FaceMatch,
	userKey string, now time.Time,
) (faceLink, bool, error) {
	link := faceLink{personID: match.PersonID}
	if match.Kind == domain.FaceMatchNew {
		id, err := createFacePerson(ctx, tx, p.Crop, now)
		if err != nil {
			return faceLink{}, false, err
		}
		link.personID, link.created = id, true
		if err := writeFaceCandidates(ctx, tx, id, match.Candidates, now); err != nil {
			return faceLink{}, false, err
		}
	}
	tracks, err := encodeFaceTracks(p.Spans)
	if err != nil {
		return faceLink{}, false, err
	}
	res, err := tx.ExecContext(ctx, `insert into face_appearances (content_key, recognizer, person_id, dropped,
		tracks, good_seconds, track_count, centroid, crop) values (?, ?, ?, 0, ?, ?, ?, ?, ?)`,
		result.ContentKey, string(result.Recognizer), link.personID, tracks, p.GoodSeconds, len(p.Spans),
		encodeFaceVector(p.Centroid), nullBytes(p.Crop.JPEG))
	if err != nil {
		return faceLink{}, false, fmt.Errorf("cannot store a face appearance: %w", err)
	}
	if link.appearanceID, err = res.LastInsertId(); err != nil {
		return faceLink{}, false, fmt.Errorf("cannot store a face appearance: %w", err)
	}
	added, err := addFacePerformer(ctx, tx, userKey, link.personID)
	return link, added, err
}

// createFacePerson creates a provisional person, one without names, with the crop as an ordinary face.
func createFacePerson(ctx context.Context, tx *sql.Tx, crop domain.PersonImage, now time.Time) (int64, error) {
	res, err := tx.ExecContext(ctx, `insert into people (created_at) values (?)`, now.Unix())
	if err != nil {
		return 0, fmt.Errorf("cannot create a provisional person: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("cannot create a provisional person: %w", err)
	}
	if len(crop.JPEG) == 0 {
		return id, nil
	}
	if _, err := tx.ExecContext(ctx, `insert into person_images (person_id, image, width, height, updated_at)
		values (?, ?, ?, ?, ?)`, id, crop.JPEG, crop.Width, crop.Height, now.Unix()); err != nil {
		return 0, fmt.Errorf("cannot store the face of person %d: %w", id, err)
	}
	return id, bumpPersonImageRevision(ctx, tx, id)
}

// writeFaceCandidates offers the provisional person with each candidate; a pair the owner dismissed stays dismissed.
func writeFaceCandidates(ctx context.Context, tx *sql.Tx, provisionalID int64, candidates []domain.FaceCandidate, now time.Time) error {
	for _, candidate := range candidates {
		if _, err := tx.ExecContext(ctx, `insert into face_candidates (provisional_id, person_id, similarity, dismissed,
			created_at) values (?, ?, ?, 0, ?)
			on conflict (provisional_id, person_id) do update set similarity = excluded.similarity`,
			provisionalID, candidate.PersonID, candidate.Similarity, now.UnixMilli()); err != nil {
			return fmt.Errorf("cannot store a face candidate of person %d: %w", provisionalID, err)
		}
	}
	return nil
}

// addFacePerformer appends the person to the video's performers unless already there, reporting whether it did.
func addFacePerformer(ctx context.Context, tx *sql.Tx, userKey string, personID int64) (bool, error) {
	res, err := tx.ExecContext(ctx, `insert into video_work_people (content_key, role, position, person_id)
		select ?1, 'performer', coalesce(max(position) + 1, 0), ?2 from video_work_people
		 where content_key = ?1 and role = 'performer'
		on conflict (content_key, role, person_id) do nothing`, userKey, personID)
	if err != nil {
		return false, fmt.Errorf("cannot add person %d as a performer: %w", personID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cannot add person %d as a performer: %w", personID, err)
	}
	return n > 0, nil
}

func recordFaceLinks(ctx context.Context, tx *sql.Tx, entryID int64, userKey string, links []faceLink) error {
	if entryID == 0 {
		return nil
	}
	for _, link := range links {
		if _, err := tx.ExecContext(ctx, `insert into activity_entry_face_links (entry_id, appearance_id, person_id,
			user_key, created_person) values (?, ?, ?, ?, ?)`,
			entryID, link.appearanceID, link.personID, userKey, link.created); err != nil {
			return fmt.Errorf("cannot record the face links of activity %d: %w", entryID, err)
		}
	}
	return nil
}

// encodeFaceTracks stores spans as JSON [[start, end, x, y, w], ...].
func encodeFaceTracks(spans []domain.FaceSpan) (string, error) {
	rows := make([][5]float64, len(spans))
	for i, s := range spans {
		rows[i] = [5]float64{s.Start, s.End, s.X, s.Y, s.W}
	}
	encoded, err := json.Marshal(rows)
	if err != nil {
		return "", fmt.Errorf("cannot encode the face tracks: %w", err)
	}
	return string(encoded), nil
}

func decodeFaceTracks(encoded string) ([]domain.FaceSpan, error) {
	var rows [][5]float64
	if err := json.Unmarshal([]byte(encoded), &rows); err != nil {
		return nil, err
	}
	spans := make([]domain.FaceSpan, len(rows))
	for i, r := range rows {
		spans[i] = domain.FaceSpan{Start: r[0], End: r[1], X: r[2], Y: r[3], W: r[4]}
	}
	return spans, nil
}

// encodeFaceVector stores a vector as little-endian float32.
func encodeFaceVector(v []float32) []byte {
	out := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(out[4*i:], math.Float32bits(x))
	}
	return out
}

func decodeFaceVector(b []byte) []float32 {
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return out
}

func nullBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}
