package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Treat all six user-keyed work-info tables as one value; store scalar rows only when code or description is nonempty.

var workInfoTables = []string{
	"video_work_info", "video_work_dates", "video_work_urls", "video_work_external_ids",
	"video_work_people", "video_work_organizations",
}

// Set atomically replaces normalized work information, rejects missing credit references, and leaves timestamps/events untouched for equal values.
func (s *WorkInfoStore) Set(ctx context.Context, videoID int64, info domain.WorkInfo) (domain.Video, error) {
	normalized, _, err := domain.NormalizeWorkInfo(info)
	if err != nil {
		return domain.Video{}, err
	}

	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Video{}, fmt.Errorf("cannot update the work information: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	keys, err := userKeysForVideoIDs(ctx, tx, []int64{videoID})
	if err != nil {
		return domain.Video{}, err
	}
	if len(keys) == 0 {
		return domain.Video{}, domain.ErrNotFound
	}
	if _, err := checkCreditRefs(ctx, tx, []domain.WorkInfo{info}); err != nil {
		return domain.Video{}, err
	}
	now := time.Now()
	edited, err := applyWorkInfo(ctx, tx, keys[:1], map[string]domain.WorkInfo{keys[0]: normalized})
	if err != nil {
		return domain.Video{}, err
	}
	if err := touchEditedAtForUserKeys(ctx, tx, edited, now); err != nil {
		return domain.Video{}, err
	}
	video, events, err := savedWorkInfoVideo(ctx, tx, videoID, edited)
	if err != nil {
		return domain.Video{}, err
	}
	if err := s.db.commitEvents(ctx, tx, events...); err != nil {
		return domain.Video{}, fmt.Errorf("cannot update the work information: %w", err)
	}
	return video, nil
}

// SetMany is atomic and preserves request order; last writes win per shared key, while edit times compare initial and final values.
func (s *WorkInfoStore) SetMany(ctx context.Context, changes []domain.WorkInfoChange) ([]domain.Video, error) {
	raw := make([]domain.WorkInfo, 0, len(changes))
	infos := make([]domain.WorkInfo, 0, len(changes))
	refs := make([]domain.VideoRef, 0, len(changes))
	for i, change := range changes {
		normalized, _, err := domain.NormalizeWorkInfo(change.WorkInfo)
		if err != nil {
			return nil, &domain.WorkInfoAtError{Index: i, Err: err}
		}
		raw = append(raw, change.WorkInfo)
		infos = append(infos, normalized)
		refs = append(refs, change.Video)
	}

	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot update the work information: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	targets, err := resolveTaggableVideos(ctx, tx, refs)
	if err != nil {
		return nil, err
	}
	if index, err := checkCreditRefs(ctx, tx, raw); err != nil {
		if index < 0 {
			return nil, err
		}
		return nil, &domain.WorkInfoAtError{Index: index, Err: err}
	}
	final := make(map[string]domain.WorkInfo, len(targets))
	for i, target := range targets {
		final[target.userKey] = infos[i]
	}
	now := time.Now()
	edited, err := applyWorkInfo(ctx, tx, uniqueUserKeys(targets), final)
	if err != nil {
		return nil, err
	}
	if err := touchEditedAtForUserKeys(ctx, tx, edited, now); err != nil {
		return nil, err
	}
	out, err := readTargetVideos(ctx, tx, targets)
	if err != nil {
		return nil, err
	}
	if err := attachWorkInfo(ctx, tx, out); err != nil {
		return nil, err
	}
	events, err := workInfoChangedEvents(ctx, tx, edited)
	if err != nil {
		return nil, err
	}
	if err := s.db.commitEvents(ctx, tx, events...); err != nil {
		return nil, fmt.Errorf("cannot update the work information: %w", err)
	}
	return out, nil
}

// SetDetails validates display name and work information before writing either; content-specific names and shared work info notify only actual changes.
func (s *WorkInfoStore) SetDetails(ctx context.Context, videoID int64, displayName string, info domain.WorkInfo) (domain.Video, error) {
	name, _, err := domain.NormalizeDisplayName(displayName)
	if err != nil {
		return domain.Video{}, err
	}
	normalized, _, err := domain.NormalizeWorkInfo(info)
	if err != nil {
		return domain.Video{}, err
	}

	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Video{}, fmt.Errorf("cannot update the video details: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	contentKeys, err := registeredContentKeysForVideoIDs(ctx, tx, []int64{videoID})
	if err != nil {
		return domain.Video{}, err
	}
	userKeys, err := userKeysForVideoIDs(ctx, tx, []int64{videoID})
	if err != nil {
		return domain.Video{}, err
	}
	if len(contentKeys) == 0 || len(userKeys) == 0 {
		return domain.Video{}, domain.ErrNotFound
	}
	if _, err := checkCreditRefs(ctx, tx, []domain.WorkInfo{info}); err != nil {
		return domain.Video{}, err
	}
	now := time.Now()
	touched, nameChanged, err := writeChangedDisplayName(ctx, tx, contentKeys[0], name, now)
	if err != nil {
		return domain.Video{}, err
	}
	edited, err := applyWorkInfo(ctx, tx, userKeys[:1], map[string]domain.WorkInfo{userKeys[0]: normalized})
	if err != nil {
		return domain.Video{}, err
	}
	members, err := contentKeysForUserKeys(ctx, tx, edited)
	if err != nil {
		return domain.Video{}, err
	}
	touched = append(touched, members...)
	slices.Sort(touched)
	if err := touchEditedAt(ctx, tx, slices.Compact(touched), now); err != nil {
		return domain.Video{}, err
	}
	video, events, err := savedWorkInfoVideo(ctx, tx, videoID, edited)
	if err != nil {
		return domain.Video{}, err
	}
	if nameChanged {
		events = append([]domain.Event{domain.VideoOverrideChanged{VideoID: videoID}}, events...)
	}
	if err := s.db.commitMatchChangeIf(ctx, tx, nameChanged, events...); err != nil {
		return domain.Video{}, fmt.Errorf("cannot update the video details: %w", err)
	}
	return video, nil
}

// ByUserKeys reads all six tables in one snapshot so concurrent replacement cannot mix old and new work information.
func (s *WorkInfoStore) ByUserKeys(ctx context.Context, userKeys []string) (map[string]domain.WorkInfo, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot read the work information: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	return workInfoByUserKeys(ctx, tx, userKeys)
}

func writeChangedDisplayName(ctx context.Context, tx *sql.Tx, key, name string, now time.Time) ([]string, bool, error) {
	before, err := displayNamesOf(ctx, tx, []string{key})
	if err != nil {
		return nil, false, err
	}
	if before[key] == name {
		return nil, false, nil
	}
	if err := writeDisplayName(ctx, tx, key, name, now); err != nil {
		return nil, false, err
	}
	if err := refreshSearchKeysForContentKeys(ctx, tx, []string{key}); err != nil {
		return nil, false, err
	}
	return []string{key}, true, nil
}

// Report the first missing credit in original input order and field position; query failures use index -1.
func checkCreditRefs(ctx context.Context, tx *sql.Tx, infos []domain.WorkInfo) (int, error) {
	var personIDs, organizationIDs []int64
	for _, info := range infos {
		for _, p := range info.People {
			personIDs = append(personIDs, p.Person.ID)
		}
		for _, o := range info.Organizations {
			organizationIDs = append(organizationIDs, o.Organization.ID)
		}
	}
	people, err := existingCreditIDs(ctx, tx, creditTablesByKind[domain.CreditPerson], personIDs)
	if err != nil {
		return -1, err
	}
	organizations, err := existingCreditIDs(ctx, tx, creditTablesByKind[domain.CreditOrganization], organizationIDs)
	if err != nil {
		return -1, err
	}
	for index, info := range infos {
		for i, p := range info.People {
			if !people[p.Person.ID] {
				return index, &domain.CreditNotFoundError{
					Kind: domain.CreditPerson, ID: p.Person.ID, Field: fmt.Sprintf("people[%d].person.id", i),
				}
			}
		}
		for i, o := range info.Organizations {
			if !organizations[o.Organization.ID] {
				return index, &domain.CreditNotFoundError{
					Kind: domain.CreditOrganization, ID: o.Organization.ID,
					Field: fmt.Sprintf("organizations[%d].organization.id", i),
				}
			}
		}
	}
	return -1, nil
}

// Write only changed keys in input order, dropping the face appearances of removed performers and marking the
// removed credits of automatic merges; the caller advances edit times.
func applyWorkInfo(ctx context.Context, tx *sql.Tx, keys []string, final map[string]domain.WorkInfo) ([]string, error) {
	before, err := workInfoByUserKeys(ctx, tx, keys)
	if err != nil {
		return nil, err
	}
	var edited []string
	for _, key := range keys {
		if before[key].Equal(final[key]) {
			continue
		}
		if err := dropRemovedPerformerFaces(ctx, tx, key, before[key], final[key]); err != nil {
			return nil, err
		}
		if err := markOwnerChangedMergeCredits(ctx, tx, key, before[key], final[key]); err != nil {
			return nil, err
		}
		if err := replaceWorkInfo(ctx, tx, key, final[key]); err != nil {
			return nil, err
		}
		edited = append(edited, key)
	}
	return edited, nil
}

// Replace all six tables together; skip empty scalar rows and retain input position within each credit role.
func replaceWorkInfo(ctx context.Context, tx *sql.Tx, key string, info domain.WorkInfo) error {
	if err := deleteWorkInfo(ctx, tx, key); err != nil {
		return err
	}
	if info.Code != "" || info.Description != "" {
		if _, err := tx.ExecContext(ctx,
			`insert into video_work_info (content_key, code, code_key, description, search_version) values (?, ?, ?, ?, ?)`,
			key, nullableString(info.Code), domain.FoldForMatch(info.Code), nullableString(info.Description),
			domain.SearchKeyVersion,
		); err != nil {
			return fmt.Errorf("cannot save the work information: %w", err)
		}
	}
	var statements []workInfoInsert
	for _, d := range info.Dates {
		statements = append(statements, workInfoInsert{`video_work_dates (content_key, kind, date) values (?, ?, ?)`,
			[]any{key, string(d.Kind), d.Date}})
	}
	for position, u := range info.URLs {
		statements = append(statements, workInfoInsert{`video_work_urls (content_key, position, url) values (?, ?, ?)`,
			[]any{key, position, u}})
	}
	for _, id := range info.ExternalIDs {
		statements = append(statements, workInfoInsert{
			`video_work_external_ids (content_key, source, value) values (?, ?, ?)`, []any{key, id.Source, id.Value}})
	}
	people := map[domain.PersonRole]int{}
	for _, p := range info.People {
		statements = append(statements, workInfoInsert{
			`video_work_people (content_key, role, position, person_id) values (?, ?, ?, ?)`,
			[]any{key, string(p.Role), people[p.Role], p.Person.ID}})
		people[p.Role]++
	}
	organizations := map[domain.OrganizationRole]int{}
	for _, o := range info.Organizations {
		statements = append(statements, workInfoInsert{
			`video_work_organizations (content_key, role, position, organization_id) values (?, ?, ?, ?)`,
			[]any{key, string(o.Role), organizations[o.Role], o.Organization.ID}})
		organizations[o.Role]++
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, `insert into `+statement.into, statement.args...); err != nil {
			return fmt.Errorf("cannot save the work information: %w", err)
		}
	}
	return nil
}

type workInfoInsert struct {
	into string
	args []any
}

func deleteWorkInfo(ctx context.Context, tx *sql.Tx, key string) error {
	for _, table := range workInfoTables {
		if _, err := tx.ExecContext(ctx, `delete from `+table+` where content_key = ?`, key); err != nil {
			return fmt.Errorf("cannot clear the %s rows: %w", table, err)
		}
	}
	return nil
}

// If any source work-info row exists, replace the whole destination value; otherwise preserve the destination.
func moveWorkInfo(ctx context.Context, tx *sql.Tx, from, to string) error {
	args := make([]any, len(workInfoTables))
	for i := range args {
		args[i] = from
	}
	var present bool
	if err := tx.QueryRowContext(ctx, workInfoExistsQuery, args...).Scan(&present); err != nil {
		return fmt.Errorf("cannot read the work information: %w", err)
	}
	if !present {
		return nil
	}
	if err := deleteWorkInfo(ctx, tx, to); err != nil {
		return err
	}
	for _, table := range workInfoTables {
		if _, err := tx.ExecContext(ctx, `update `+table+` set content_key = ? where content_key = ?`, to, from); err != nil {
			return fmt.Errorf("cannot move the %s values: %w", table, err)
		}
	}
	return nil
}

var workInfoExistsQuery = func() string {
	parts := make([]string, 0, len(workInfoTables))
	for _, table := range workInfoTables {
		parts = append(parts, `exists (select 1 from `+table+` where content_key = ?)`)
	}
	return `select ` + strings.Join(parts, ` or `)
}()

// Read the saved video with its work information and the change events of the edited user keys.
func savedWorkInfoVideo(ctx context.Context, tx *sql.Tx, videoID int64, edited []string) (domain.Video, []domain.Event, error) {
	video, err := ownerVideoWithWorkInfo(ctx, tx, videoID)
	if err != nil {
		return domain.Video{}, nil, err
	}
	events, err := workInfoChangedEvents(ctx, tx, edited)
	if err != nil {
		return domain.Video{}, nil, err
	}
	return video, events, nil
}

func ownerVideoWithWorkInfo(ctx context.Context, tx *sql.Tx, videoID int64) (domain.Video, error) {
	video, err := getVideo(ctx, tx, domain.AudienceOwner, videoID)
	if err != nil {
		return domain.Video{}, err
	}
	videos := []domain.Video{video}
	if err := attachWorkInfo(ctx, tx, videos); err != nil {
		return domain.Video{}, err
	}
	return videos[0], nil
}

func attachWorkInfo(ctx context.Context, q queryExecer, videos []domain.Video) error {
	keys := make([]string, 0, len(videos))
	for _, video := range videos {
		if video.UserKey != "" {
			keys = append(keys, video.UserKey)
		}
	}
	infos, err := workInfoByUserKeys(ctx, q, keys)
	if err != nil {
		return err
	}
	for i := range videos {
		if info, ok := infos[videos[i].UserKey]; ok {
			videos[i].WorkInfo = &info
		}
	}
	return nil
}

// Notify all videos sharing each changed key, including every bundle member.
func workInfoChangedEvents(ctx context.Context, tx *sql.Tx, edited []string) ([]domain.Event, error) {
	if len(edited) == 0 {
		return nil, nil
	}
	ids, err := videoIDsForUserKeys(ctx, tx, edited)
	if err != nil {
		return nil, err
	}
	return []domain.Event{domain.VideoWorkInfoChanged{VideoIDs: ids}}, nil
}

func videoIDsForUserKeys(ctx context.Context, tx *sql.Tx, userKeys []string) ([]int64, error) {
	contentKeys, err := contentKeysForUserKeys(ctx, tx, userKeys)
	if err != nil {
		return nil, err
	}
	return videoIDsForContentKeys(ctx, tx, contentKeys)
}

// Omit absent work information, but return non-nil empty lists within present values.
func workInfoByUserKeys(ctx context.Context, q queryExecer, userKeys []string) (map[string]domain.WorkInfo, error) {
	infos := map[string]*domain.WorkInfo{}
	if len(userKeys) == 0 {
		return map[string]domain.WorkInfo{}, nil
	}
	encoded, err := json.Marshal(userKeys)
	if err != nil {
		return nil, fmt.Errorf("cannot build user keys: %w", err)
	}
	entry := func(key string) *domain.WorkInfo {
		info, ok := infos[key]
		if !ok {
			info = &domain.WorkInfo{
				Dates: []domain.WorkDate{}, People: []domain.WorkPerson{}, Organizations: []domain.WorkOrganization{},
				URLs: []string{}, ExternalIDs: []domain.ExternalID{},
			}
			infos[key] = info
		}
		return info
	}
	for _, read := range workInfoReads(entry) {
		if err := eachWorkInfoRow(ctx, q, read.query, string(encoded), read.scan); err != nil {
			return nil, err
		}
	}
	out := make(map[string]domain.WorkInfo, len(infos))
	for key, info := range infos {
		slices.SortStableFunc(info.Dates, func(a, b domain.WorkDate) int {
			return slices.Index(domain.DateKinds, a.Kind) - slices.Index(domain.DateKinds, b.Kind)
		})
		slices.SortStableFunc(info.People, func(a, b domain.WorkPerson) int {
			return slices.Index(domain.PersonRoles, a.Role) - slices.Index(domain.PersonRoles, b.Role)
		})
		slices.SortStableFunc(info.Organizations, func(a, b domain.WorkOrganization) int {
			return slices.Index(domain.OrganizationRoles, a.Role) - slices.Index(domain.OrganizationRoles, b.Role)
		})
		out[key] = *info
	}
	return out, nil
}

type workInfoRead struct {
	query string
	scan  func(*sql.Rows) error
}

// Read by key and position; the caller restores date-kind and credit-role ordering.
func workInfoReads(entry func(string) *domain.WorkInfo) []workInfoRead {
	const keys = `content_key in (select value from json_each(?))`
	return []workInfoRead{
		{`select content_key, coalesce(code, ''), coalesce(description, '') from video_work_info where ` + keys,
			func(rows *sql.Rows) error {
				var key, code, description string
				if err := rows.Scan(&key, &code, &description); err != nil {
					return err
				}
				info := entry(key)
				info.Code, info.Description = code, description
				return nil
			}},
		{`select content_key, kind, date from video_work_dates where ` + keys,
			func(rows *sql.Rows) error {
				var key string
				var d domain.WorkDate
				if err := rows.Scan(&key, &d.Kind, &d.Date); err != nil {
					return err
				}
				info := entry(key)
				info.Dates = append(info.Dates, d)
				return nil
			}},
		{`select content_key, url from video_work_urls where ` + keys + ` order by content_key, position`,
			func(rows *sql.Rows) error {
				var key, u string
				if err := rows.Scan(&key, &u); err != nil {
					return err
				}
				info := entry(key)
				info.URLs = append(info.URLs, u)
				return nil
			}},
		{`select content_key, source, value from video_work_external_ids where ` + keys + ` order by content_key, source, value`,
			func(rows *sql.Rows) error {
				var key string
				var id domain.ExternalID
				if err := rows.Scan(&key, &id.Source, &id.Value); err != nil {
					return err
				}
				info := entry(key)
				info.ExternalIDs = append(info.ExternalIDs, id)
				return nil
			}},
		creditLinkRead(creditTablesByKind[domain.CreditPerson], func(key, role string, ref domain.CreditRef, revision int64) {
			info := entry(key)
			info.People = append(info.People,
				domain.WorkPerson{Role: domain.PersonRole(role), Person: ref, ImageRevision: revision})
		}),
		creditLinkRead(creditTablesByKind[domain.CreditOrganization], func(key, role string, ref domain.CreditRef, _ int64) {
			info := entry(key)
			info.Organizations = append(info.Organizations,
				domain.WorkOrganization{Role: domain.OrganizationRole(role), Organization: ref})
		}),
	}
}

// creditLinkRead also reads each person's image revision; organizations read 0. A person without a canonical name reads as provisional.
func creditLinkRead(t creditTables, add func(key, role string, ref domain.CreditRef, revision int64)) workInfoRead {
	revision := `0`
	if t.kind == domain.CreditPerson {
		revision = personImageRevision("l.person_id")
	}
	return workInfoRead{
		query: t.q(`select l.content_key, l.role, l.{id}, coalesce(n.name, ''), n.{id} is null, ` + revision + ` from {links} l
			left join {names} n on n.{id} = l.{id} and n.canonical = 1
			where l.content_key in (select value from json_each(?)) order by l.content_key, l.position`),
		scan: func(rows *sql.Rows) error {
			var key, role string
			var ref domain.CreditRef
			var revision int64
			if err := rows.Scan(&key, &role, &ref.ID, &ref.Name, &ref.Provisional, &revision); err != nil {
				return err
			}
			add(key, role, ref, revision)
			return nil
		},
	}
}

func eachWorkInfoRow(ctx context.Context, q queryExecer, query, arg string, scan func(*sql.Rows) error) error {
	rows, err := q.QueryContext(ctx, query, arg)
	if err != nil {
		return fmt.Errorf("cannot read the work information: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return fmt.Errorf("cannot read the work information: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot read the work information: %w", err)
	}
	return nil
}

// Preserve target order and repeated references in owner-visible bulk responses.
func readTargetVideos(ctx context.Context, tx *sql.Tx, targets []taggableVideo) ([]domain.Video, error) {
	ids := make([]int64, 0, len(targets))
	for _, target := range targets {
		ids = append(ids, target.id)
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build video ids: %w", err)
	}
	byID, err := readVideosByIDs(ctx, tx, string(encoded))
	if err != nil {
		return nil, err
	}
	out := make([]domain.Video, 0, len(targets))
	for _, target := range targets {
		video, ok := byID[target.id]
		if !ok {
			return nil, fmt.Errorf("cannot read the video (id=%d): %w", target.id, domain.ErrNotFound)
		}
		out = append(out, video)
	}
	return out, nil
}
