package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Every transaction that changes a review run or its candidates publishes AssistantChanged after commit.

const assistantReviewRunColumns = `id, kind, request, status, total, judged, listed, coalesce(error, ''), started_at,
	finished_at`

func scanAssistantReviewRun(row rowScanner) (domain.AssistantReviewRun, error) {
	var (
		run          domain.AssistantReviewRun
		kind, status string
		startedAt    int64
		finishedAt   sql.NullInt64
	)
	if err := row.Scan(&run.ID, &kind, &run.Request, &status, &run.Total, &run.Judged, &run.Listed, &run.Error,
		&startedAt, &finishedAt); err != nil {
		return domain.AssistantReviewRun{}, err
	}
	run.Kind, run.Status = domain.AssistantReviewKind(kind), domain.AssistantReviewStatus(status)
	run.StartedAt, run.FinishedAt = time.UnixMilli(startedAt), millisTime(finishedAt)
	return run, nil
}

func assistantReviewRunWhere(ctx context.Context, q rowQueryer, where string, args ...any) (domain.AssistantReviewRun, error) {
	run, err := scanAssistantReviewRun(q.QueryRowContext(ctx,
		`select `+assistantReviewRunColumns+` from assistant_review_runs where `+where, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AssistantReviewRun{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.AssistantReviewRun{}, fmt.Errorf("cannot read the review run: %w", err)
	}
	return run, nil
}

// ReviewRun reads one run; a missing one is domain.ErrNotFound.
func (s *AssistantStore) ReviewRun(ctx context.Context, id int64) (domain.AssistantReviewRun, error) {
	return assistantReviewRunWhere(ctx, s.db.read, `id = ?`, id)
}

// StartReviewRun starts a running run of the kind, or returns the one already running with started false.
func (s *AssistantStore) StartReviewRun(
	ctx context.Context, kind domain.AssistantReviewKind, request string, now time.Time,
) (domain.AssistantReviewRun, bool, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.AssistantReviewRun{}, false, fmt.Errorf("cannot start the review run: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	running, err := assistantReviewRunWhere(ctx, tx, `kind = ? and status = ?`,
		string(kind), string(domain.AssistantReviewRunning))
	if err == nil {
		return running, false, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.AssistantReviewRun{}, false, err
	}
	var id int64
	if err := tx.QueryRowContext(ctx, `insert into assistant_review_runs (kind, request, status, started_at)
		values (?, ?, ?, ?) returning id`, string(kind), request, string(domain.AssistantReviewRunning),
		now.UnixMilli()).Scan(&id); err != nil {
		return domain.AssistantReviewRun{}, false, fmt.Errorf("cannot start the review run: %w", err)
	}
	run, err := assistantReviewRunWhere(ctx, tx, `id = ?`, id)
	if err != nil {
		return domain.AssistantReviewRun{}, false, err
	}
	if err := s.db.commitWith(ctx, tx, &changes{}, domain.AssistantChanged{}); err != nil {
		return domain.AssistantReviewRun{}, false, fmt.Errorf("cannot start the review run: %w", err)
	}
	return run, true, nil
}

// NextReviewRun returns the oldest running run; domain.ErrNoJob when none runs.
func (s *AssistantStore) NextReviewRun(ctx context.Context) (domain.AssistantReviewRun, error) {
	run, err := assistantReviewRunWhere(ctx, s.db.read, `status = ? order by id limit 1`,
		string(domain.AssistantReviewRunning))
	if errors.Is(err, domain.ErrNotFound) {
		return domain.AssistantReviewRun{}, domain.ErrNoJob
	}
	return run, err
}

// execReviewRun runs one statement on a running run and publishes the change.
func (s *AssistantStore) execReviewRun(ctx context.Context, statement string, args ...any) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot update the review run: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	return s.updateRunningRun(ctx, tx, statement, args...)
}

// updateRunningRun runs one guarded update of a running run in tx and commits it.
func (s *AssistantStore) updateRunningRun(ctx context.Context, tx *sql.Tx, statement string, args ...any) error {
	result, err := tx.ExecContext(ctx, statement, args...)
	if err != nil {
		return fmt.Errorf("cannot update the review run: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot update the review run: %w", err)
	}
	if affected != 1 {
		return errors.New("the review run is not running")
	}
	if err := s.db.commitWith(ctx, tx, &changes{}, domain.AssistantChanged{}); err != nil {
		return fmt.Errorf("cannot update the review run: %w", err)
	}
	return nil
}

// SetReviewTotal records how many sets the finder found.
func (s *AssistantStore) SetReviewTotal(ctx context.Context, runID int64, total int) error {
	return s.execReviewRun(ctx, `update assistant_review_runs set total = ? where id = ? and status = ?`,
		total, runID, string(domain.AssistantReviewRunning))
}

// RecordReviewJudgement counts one judged set and stores it as a listed candidate when listed is true.
func (s *AssistantStore) RecordReviewJudgement(
	ctx context.Context, runID int64, set domain.AssistantReviewSet, probability float64, listed bool,
) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot record the review judgement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	listedCount := 0
	if listed {
		listedCount = 1
		members, err := json.Marshal(set.Members)
		if err != nil {
			return fmt.Errorf("cannot encode the review members: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `insert into assistant_review_candidates
			(run_id, members, set_key, probability, state) values (?, ?, ?, ?, ?)`,
			runID, string(members), set.SetKey(), probability, string(domain.AssistantCandidateListed)); err != nil {
			return fmt.Errorf("cannot store the review candidate: %w", err)
		}
	}
	return s.updateRunningRun(ctx, tx, `update assistant_review_runs set judged = judged + 1, listed = listed + ?
		where id = ? and status = ?`, listedCount, runID, string(domain.AssistantReviewRunning))
}

// FinishReviewRun ends a running run as done or failed; failure is the classifier's message.
func (s *AssistantStore) FinishReviewRun(
	ctx context.Context, runID int64, status domain.AssistantReviewStatus, failure string, now time.Time,
) error {
	var message any
	if failure != "" {
		message = failure
	}
	return s.execReviewRun(ctx, `update assistant_review_runs set status = ?, error = ?, finished_at = ?
		where id = ? and status = ?`, string(status), message, now.UnixMilli(), runID,
		string(domain.AssistantReviewRunning))
}

// RecoverReviewRuns runs at startup and marks every running run interrupted.
func (s *AssistantStore) RecoverReviewRuns(ctx context.Context, now time.Time) (int, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("cannot recover the review runs: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `update assistant_review_runs set status = ?, finished_at = ? where status = ?`,
		string(domain.AssistantReviewInterrupted), now.UnixMilli(), string(domain.AssistantReviewRunning))
	if err != nil {
		return 0, fmt.Errorf("cannot recover the review runs: %w", err)
	}
	recovered, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("cannot recover the review runs: %w", err)
	}
	var events []domain.Event
	if recovered > 0 {
		events = append(events, domain.AssistantChanged{})
	}
	if err := s.db.commitWith(ctx, tx, &changes{}, events...); err != nil {
		return 0, fmt.Errorf("cannot recover the review runs: %w", err)
	}
	return int(recovered), nil
}

const assistantReviewCandidateColumns = `id, run_id, members, set_key, probability, state, coalesce(proposal_id, 0)`

func scanAssistantReviewCandidate(row rowScanner) (domain.AssistantReviewCandidate, error) {
	var (
		c              domain.AssistantReviewCandidate
		members, state string
	)
	if err := row.Scan(&c.ID, &c.RunID, &members, &c.SetKey, &c.Probability, &state, &c.ProposalID); err != nil {
		return domain.AssistantReviewCandidate{}, err
	}
	c.State = domain.AssistantReviewCandidateState(state)
	if err := json.Unmarshal([]byte(members), &c.Members); err != nil {
		return domain.AssistantReviewCandidate{}, fmt.Errorf("cannot read the members of candidate %d: %w", c.ID, err)
	}
	return c, nil
}

// assistantReviewCursorSort tells a review page cursor apart from the other lists' cursors.
const assistantReviewCursorSort = "review"

// ReviewCandidates pages a run's candidates, most probable first, with their members' current names and counts. A
// candidate whose member is gone is left out; the page may then hold fewer than limit items.
func (s *AssistantStore) ReviewCandidates(
	ctx context.Context, runID int64, cursor string, limit int,
) ([]domain.AssistantReviewCandidate, string, error) {
	where, args := `run_id = ?`, []any{runID}
	if cursor != "" {
		parts, err := unpackCursor(cursor, 3)
		if err != nil {
			return nil, "", err
		}
		probability, perr := strconv.ParseFloat(parts[1], 64)
		id, ierr := strconv.ParseInt(parts[2], 10, 64)
		if parts[0] != assistantReviewCursorSort || perr != nil || ierr != nil {
			return nil, "", fmt.Errorf("%w: not a review cursor", domain.ErrInvalidCursor)
		}
		where += ` and (probability < ? or (probability = ? and id > ?))`
		args = append(args, probability, probability, id)
	}
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, "", fmt.Errorf("cannot list the review candidates: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	candidates, err := readAssistantReviewCandidates(ctx, tx, where+` order by probability desc, id limit ?`,
		append(args, limit+1)...)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(candidates) > limit {
		candidates = candidates[:limit]
		last := candidates[limit-1]
		next = packCursor(assistantReviewCursorSort, strconv.FormatFloat(last.Probability, 'g', -1, 64),
			strconv.FormatInt(last.ID, 10))
	}
	present := make([]domain.AssistantReviewCandidate, 0, len(candidates))
	for _, c := range candidates {
		if err := currentReviewMembers(ctx, tx, &c); err != nil {
			return nil, "", err
		}
		if !c.Gone {
			present = append(present, c)
		}
	}
	return present, next, nil
}

// ReviewCandidatesByID reads the run's candidates among ids with their members' current names and counts; ids of
// other runs are absent.
func (s *AssistantStore) ReviewCandidatesByID(
	ctx context.Context, runID int64, ids []int64,
) (map[int64]domain.AssistantReviewCandidate, error) {
	encoded, _ := json.Marshal(ids) // A slice of integers always encodes.
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot read the review candidates: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	candidates, err := readAssistantReviewCandidates(ctx, tx,
		`run_id = ? and id in (select value from json_each(?))`, runID, string(encoded))
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]domain.AssistantReviewCandidate, len(candidates))
	for _, c := range candidates {
		if err := currentReviewMembers(ctx, tx, &c); err != nil {
			return nil, err
		}
		byID[c.ID] = c
	}
	return byID, nil
}

func readAssistantReviewCandidates(
	ctx context.Context, tx *sql.Tx, where string, args ...any,
) ([]domain.AssistantReviewCandidate, error) {
	rows, err := tx.QueryContext(ctx, `select `+assistantReviewCandidateColumns+` from assistant_review_candidates
		where `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read the review candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var candidates []domain.AssistantReviewCandidate
	for rows.Next() {
		c, err := scanAssistantReviewCandidate(rows)
		if err != nil {
			return nil, fmt.Errorf("cannot read the review candidates: %w", err)
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the review candidates: %w", err)
	}
	return candidates, nil
}

// currentReviewMembers replaces the stored names with the current ones and reads the counts; a member that no
// longer exists, such as a merged tag's source, marks the candidate gone. A video member's ID becomes its current
// library video.
func currentReviewMembers(ctx context.Context, tx *sql.Tx, c *domain.AssistantReviewCandidate) error {
	for i := range c.Members {
		m := &c.Members[i]
		if m.Kind == domain.AssistantLabelVideo {
			ids, err := assistantVideoIDs(ctx, tx, []string{m.Key})
			if err != nil {
				return err
			}
			id, ok := ids[m.Key]
			c.Gone, m.ID = c.Gone || !ok, id
			continue
		}
		names := map[domain.AssistantLabelRef]string{}
		if err := readAssistantNames(ctx, tx, m.Kind, assistantLabelNames[m.Kind], []int64{m.ID}, names); err != nil {
			return err
		}
		name, ok := names[domain.AssistantLabelRef{Kind: m.Kind, ID: m.ID}]
		if !ok {
			c.Gone = true
			continue
		}
		count, err := assistantVideoCount(ctx, tx, m.Kind, m.ID)
		if err != nil {
			return err
		}
		m.Name, m.Count = name, count
	}
	return nil
}

// AdoptReviewCandidate marks a listed candidate adopted and stores its plan as one approved proposal with its
// assistant_run entry, in one transaction; a candidate that is not listed is domain.ErrAssistantCandidateNotListed.
func (s *AssistantStore) AdoptReviewCandidate(
	ctx context.Context, candidateID int64, request string, plan domain.AssistantPlan, now time.Time,
) (domain.AssistantProposal, error) {
	return s.storeProposal(ctx, request, plan, now, func(tx *sql.Tx, c *changes, p domain.AssistantProposal) (domain.AssistantProposal, error) {
		result, err := tx.ExecContext(ctx, `update assistant_review_candidates set state = ?, proposal_id = ?
			where id = ? and state = ?`, string(domain.AssistantCandidateAdopted), p.ID, candidateID,
			string(domain.AssistantCandidateListed))
		if err != nil {
			return domain.AssistantProposal{}, fmt.Errorf("cannot adopt the review candidate: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return domain.AssistantProposal{}, fmt.Errorf("cannot adopt the review candidate: %w", err)
		}
		if affected != 1 {
			return domain.AssistantProposal{}, domain.ErrAssistantCandidateNotListed
		}
		return approveAssistantProposal(ctx, tx, c, p.ID, now)
	})
}

// RejectReviewCandidates marks the run's listed candidates among ids rejected and remembers each set for the kind;
// it returns the rejected ids in the order of ids.
func (s *AssistantStore) RejectReviewCandidates(
	ctx context.Context, runID int64, ids []int64, now time.Time,
) ([]int64, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot reject the review candidates: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rejected := []int64{}
	for _, id := range ids {
		var setKey, kind string
		err := tx.QueryRowContext(ctx, `update assistant_review_candidates set state = ?
			where id = ? and run_id = ? and state = ?
			returning set_key, (select kind from assistant_review_runs r where r.id = run_id)`,
			string(domain.AssistantCandidateRejected), id, runID, string(domain.AssistantCandidateListed),
		).Scan(&setKey, &kind)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("cannot reject the review candidate %d: %w", id, err)
		}
		if _, err := tx.ExecContext(ctx, `insert into assistant_review_dismissals (kind, set_key, rejected_at)
			values (?, ?, ?) on conflict (kind, set_key) do nothing`, kind, setKey, now.UnixMilli()); err != nil {
			return nil, fmt.Errorf("cannot remember the rejected set: %w", err)
		}
		rejected = append(rejected, id)
	}
	var events []domain.Event
	if len(rejected) > 0 {
		events = append(events, domain.AssistantChanged{})
	}
	if err := s.db.commitWith(ctx, tx, &changes{}, events...); err != nil {
		return nil, fmt.Errorf("cannot reject the review candidates: %w", err)
	}
	return rejected, nil
}

// FindReviewSets lists the candidate sets of the kind with the facts the classifier reads, leaving out the sets
// rejected before.
func (s *AssistantStore) FindReviewSets(
	ctx context.Context, kind domain.AssistantReviewKind,
) ([]domain.AssistantReviewSet, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot find the %s review sets: %w", kind, err)
	}
	defer func() { _ = tx.Rollback() }()
	var sets []domain.AssistantReviewSet
	switch kind {
	case domain.AssistantReviewTags:
		sets, err = findTagReviewSets(ctx, tx)
	case domain.AssistantReviewPeople:
		sets, err = findPeopleReviewSets(ctx, tx)
	case domain.AssistantReviewWorks:
		sets, err = findWorkReviewSets(ctx, tx)
	default:
		err = fmt.Errorf("unknown review kind %q", kind)
	}
	if err != nil {
		return nil, err
	}
	dismissed, err := queryStrings(ctx, tx, `select set_key from assistant_review_dismissals where kind = ?`, string(kind))
	if err != nil {
		return nil, fmt.Errorf("cannot read the rejected sets: %w", err)
	}
	skip := make(map[string]bool, len(dismissed))
	for _, key := range dismissed {
		skip[key] = true
	}
	kept := make([]domain.AssistantReviewSet, 0, len(sets))
	for _, set := range sets {
		if !skip[set.SetKey()] {
			kept = append(kept, set)
		}
	}
	return kept, nil
}

// reviewEntity is a tag or person read for a finder: its display name first, then its synonyms or aliases.
type reviewEntity struct {
	id    int64
	names []string
}

func readReviewEntities(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]reviewEntity, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read the names to review: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var entities []reviewEntity
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("cannot read the names to review: %w", err)
		}
		if n := len(entities); n > 0 && entities[n-1].id == id {
			entities[n-1].names = append(entities[n-1].names, name)
			continue
		}
		entities = append(entities, reviewEntity{id: id, names: []string{name}})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the names to review: %w", err)
	}
	return entities, nil
}

// findTagReviewSets pairs the most used tags whose names are equal or near once folded.
func findTagReviewSets(ctx context.Context, tx *sql.Tx) ([]domain.AssistantReviewSet, error) {
	entities, err := readReviewEntities(ctx, tx, `with top as (
			select t.id, (select count(*) from video_tags vt where vt.tag_id = t.id) as uses from tags t
			order by uses desc, t.id limit ?)
		select top.id, n.name from top join tag_names n on n.tag_id = top.id
		order by top.uses desc, top.id, n.canonical desc, n.name`, domain.AssistantReviewTagLimit)
	if err != nil {
		return nil, err
	}
	named := make([]domain.AssistantNamedEntity, len(entities))
	for i, e := range entities {
		named[i] = domain.AssistantNamedEntity{ID: e.id, Names: e.names}
	}
	var pairs [][2]reviewEntity
	for _, pair := range domain.AssistantNearPairs(named) {
		pairs = append(pairs, [2]reviewEntity{entities[pair[0]], entities[pair[1]]})
	}
	return reviewPairSets(ctx, tx, domain.AssistantLabelTag, pairs,
		`select content_key from video_tags where tag_id = ?`)
}

// findPeopleReviewSets pairs people sharing a folded name or alias, or one link to the same page.
func findPeopleReviewSets(ctx context.Context, tx *sql.Tx) ([]domain.AssistantReviewSet, error) {
	idPairs, err := samePersonPairs(ctx, tx)
	if err != nil {
		return nil, err
	}
	byID := map[int64]reviewEntity{}
	var pairs [][2]reviewEntity
	for _, pair := range idPairs {
		var both [2]reviewEntity
		for i, id := range pair {
			e, ok := byID[id]
			if !ok {
				read, err := readReviewEntities(ctx, tx, `select person_id, name from people_names where person_id = ?
					order by canonical desc, name`, id)
				if err != nil {
					return nil, err
				}
				e = reviewEntity{id: id}
				if len(read) == 1 {
					e = read[0]
				}
				byID[id] = e
			}
			both[i] = e
		}
		pairs = append(pairs, both)
	}
	return reviewPairSets(ctx, tx, domain.AssistantLabelPerson, pairs,
		`select content_key from video_work_people where person_id = ?`)
}

func samePersonPairs(ctx context.Context, tx *sql.Tx) ([][2]int64, error) {
	rows, err := tx.QueryContext(ctx, `select a.person_id, b.person_id from people_names a
			join people_names b on b.search_key = a.search_key and b.person_id > a.person_id
			where a.search_key <> ''
		union
		select a.person_id, b.person_id from people_links a
			join people_links b on b.url = a.url and b.person_id > a.person_id
		order by 1, 2`)
	if err != nil {
		return nil, fmt.Errorf("cannot find the people to review: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var pairs [][2]int64
	for rows.Next() {
		var pair [2]int64
		if err := rows.Scan(&pair[0], &pair[1]); err != nil {
			return nil, fmt.Errorf("cannot find the people to review: %w", err)
		}
		pairs = append(pairs, pair)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot find the people to review: %w", err)
	}
	return pairs, nil
}

// reviewPairSets builds the sets of entity pairs with the titles of each member's videos. keysQuery selects the user
// keys labelled with the entity bound to its one argument; a bundle's user key reaches every version's titles.
func reviewPairSets(
	ctx context.Context, tx *sql.Tx, kind domain.AssistantLabelKind, pairs [][2]reviewEntity, keysQuery string,
) ([]domain.AssistantReviewSet, error) {
	titlesQuery := `select distinct l.title from video_locations l where l.video_id in (` + userKeyVideosSQL(keysQuery) +
		`) order by l.title limit ?`
	titles := map[int64][]string{}
	sets := make([]domain.AssistantReviewSet, 0, len(pairs))
	for _, pair := range pairs {
		set := domain.AssistantReviewSet{}
		for _, e := range pair {
			if _, ok := titles[e.id]; !ok {
				read, err := queryStrings(ctx, tx, titlesQuery, e.id, e.id, domain.AssistantReviewFactTitles)
				if err != nil {
					return nil, fmt.Errorf("cannot read the titles to review: %w", err)
				}
				titles[e.id] = read
			}
			m := domain.AssistantReviewMember{Kind: kind, ID: e.id, Titles: titles[e.id]}
			if len(e.names) > 0 {
				m.Name, m.Aliases = e.names[0], e.names[1:]
			}
			set.Members = append(set.Members, m)
		}
		sets = append(sets, set)
	}
	return sets, nil
}

// findWorkReviewSets groups the ungrouped library videos, one per user key, by work code and work title.
func findWorkReviewSets(ctx context.Context, tx *sql.Tx) ([]domain.AssistantReviewSet, error) {
	userKey := userKeyExpr("v")
	rows, err := tx.QueryContext(ctx, `select `+userKey+`, `+contentLabelExpr("v.content_key")+`,
			coalesce((select l.path from video_locations l where l.video_id = v.id order by l.path limit 1), ''),
			coalesce((select w.code_key from video_work_info w where w.content_key = `+userKey+`), '')
		from videos v
		where v.content_key <> '' and `+libraryVideoCondition("v")+` and `+shownVideoCondition("v", domain.AudienceOwner)+`
		  and not exists (select 1 from group_items gi where gi.content_key = `+userKey+`)
		order by v.id`)
	if err != nil {
		return nil, fmt.Errorf("cannot read the videos to review: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var videos []domain.AssistantWorkVideo
	for rows.Next() {
		var v domain.AssistantWorkVideo
		var path string
		if err := rows.Scan(&v.Key, &v.Title, &path, &v.CodeKey); err != nil {
			return nil, fmt.Errorf("cannot read the videos to review: %w", err)
		}
		if path != "" {
			v.Folder = filepath.Base(filepath.Dir(path))
		}
		videos = append(videos, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the videos to review: %w", err)
	}
	return domain.AssistantWorkSets(videos), nil
}
