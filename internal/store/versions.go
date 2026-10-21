package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Bundles share user data while preserving members' original rows; membership and representative changes rebuild the folder index atomically.

// Treat all work-info tables as one unit when copying or replacing keys so values from different videos cannot mix.
var userDataTables = []struct{ table, columns string }{
	{"playback_progress", "position_ms, duration_ms, completed, updated_at"},
	{"video_tags", "tag_id, created_at"},
	{"public_videos", "published_at"},
	{"video_favorites", "favorited_at"},
	{"video_work_info", "code, code_key, description, search_version"},
	{"video_work_dates", "kind, date"},
	{"video_work_urls", "position, url"},
	{"video_work_external_ids", "source, value"},
	{"video_work_people", "role, position, person_id"},
	{"video_work_organizations", "role, position, organization_id"},
}

type bundleRef struct {
	id                int64
	userKey           string
	representativeKey string
	byOwner           bool
}

// Bundle expands existing memberships and merges every side's user data into a new key, moving references while preserving absorbed keys' data.
func (s *VersionStore) Bundle(ctx context.Context, videoIDs []int64, representativeID int64) (domain.VideoVersions, error) {
	ids := uniqueIDs(videoIDs)
	if len(ids) < 2 {
		return domain.VideoVersions{}, domain.ErrTooFewVersions
	}
	if !slices.Contains(ids, representativeID) {
		return domain.VideoVersions{}, domain.ErrRepresentativeNotSelected
	}

	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot bundle the videos: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	plan, err := planBundle(ctx, tx, ids, representativeID)
	if err != nil {
		return domain.VideoVersions{}, err
	}
	plan.representativeByOwner = true
	outcome, err := applyBundle(ctx, tx, plan)
	if err != nil {
		return domain.VideoVersions{}, err
	}
	c := &changes{}
	if err := recordOwnerBundle(ctx, tx, c, plan.members, plan.joined); err != nil {
		return domain.VideoVersions{}, err
	}
	versions, err := videoVersions(ctx, tx, domain.AudienceOwner, representativeID)
	if err != nil {
		return domain.VideoVersions{}, err
	}
	if err := s.db.commitWith(ctx, tx, c, outcome.events()...); err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot bundle the videos: %w", err)
	}
	return versions, nil
}

// joined excludes the representative's existing bundle members because their ownership has not changed.
type bundlePlan struct {
	members      []string
	joined       []string
	absorbedKeys []string
	// userKeys are the sides' user-data keys, the representative's first.
	userKeys              []string
	representativeByOwner bool
	absorbedBundles       []int64
	representativeKey     string
	representativeUserKey string
}

// Read and validate the entire bundle plan without writing; every selected video must be in the library.
func planBundle(ctx context.Context, tx *sql.Tx, ids []int64, representativeID int64) (bundlePlan, error) {
	var plan bundlePlan
	// Use a map to avoid quadratic membership checks for selections of up to 20,000 videos.
	absorbed := map[int64][]string{}
	var representativeBundle int64
	for _, id := range ids {
		key, err := bundlableContentKey(ctx, tx, id)
		if err != nil {
			return bundlePlan{}, err
		}
		bundle, bundled, err := bundleOf(ctx, tx, key)
		if err != nil {
			return bundlePlan{}, err
		}
		userKey := key
		if bundled {
			userKey = bundle.userKey
		}
		if id == representativeID {
			plan.representativeKey, plan.representativeUserKey = key, userKey
			if bundled {
				representativeBundle = bundle.id
			}
		}
		if !slices.Contains(plan.userKeys, userKey) {
			plan.userKeys = append(plan.userKeys, userKey)
		}
		if !bundled {
			plan.members = append(plan.members, key)
			plan.joined = append(plan.joined, key)
			plan.absorbedKeys = append(plan.absorbedKeys, key)
			continue
		}
		if _, seen := absorbed[bundle.id]; seen {
			continue
		}
		keys, err := bundleMemberKeys(ctx, tx, bundle.id)
		if err != nil {
			return bundlePlan{}, err
		}
		absorbed[bundle.id] = keys
		plan.absorbedBundles = append(plan.absorbedBundles, bundle.id)
		plan.members = append(plan.members, keys...)
		plan.absorbedKeys = append(plan.absorbedKeys, bundle.userKey)
		plan.absorbedKeys = append(plan.absorbedKeys, keys...)
	}
	for _, id := range plan.absorbedBundles {
		if id != representativeBundle {
			plan.joined = append(plan.joined, absorbed[id]...)
		}
	}
	if i := slices.Index(plan.userKeys, plan.representativeUserKey); i > 0 {
		plan.userKeys = append(append([]string{plan.representativeUserKey}, plan.userKeys[:i]...), plan.userKeys[i+1:]...)
	}
	slices.Sort(plan.members)
	plan.members = slices.Compact(plan.members)
	slices.Sort(plan.joined)
	plan.joined = slices.Compact(plan.joined)
	return plan, nil
}

type bundleOutcome struct {
	bundleID      int64
	userKey       string
	changedGroups []int64
	changedVideos []int64
}

func (o bundleOutcome) events() []domain.Event {
	return append([]domain.Event{domain.VideoBundleChanged{VideoIDs: o.changedVideos}},
		groupsChangedEvents(o.changedGroups)...)
}

// Apply the plan in the caller's transaction, merging the sides' user data, moving references, preserving absorbed user data and refreshing folders and changed playlist shapes.
func applyBundle(ctx context.Context, tx *sql.Tx, plan bundlePlan) (bundleOutcome, error) {
	shapes, err := snapshotPlaylistShapes(ctx, tx, plan.absorbedKeys)
	if err != nil {
		return bundleOutcome{}, err
	}
	now := time.Now().Unix()
	bundleID, userKey, err := createBundle(ctx, tx, plan.representativeKey, plan.representativeByOwner, now)
	if err != nil {
		return bundleOutcome{}, err
	}
	if err := mergeUserData(ctx, tx, plan.userKeys, userKey); err != nil {
		return bundleOutcome{}, err
	}
	var rekeyedGroups []int64
	for _, key := range plan.absorbedKeys {
		ids, err := rekeyUserItems(ctx, tx, key, userKey)
		if err != nil {
			return bundleOutcome{}, err
		}
		rekeyedGroups = append(rekeyedGroups, ids...)
	}
	if err := moveBundleMembers(ctx, tx, plan, bundleID, now); err != nil {
		return bundleOutcome{}, err
	}
	if err := pruneVersionCandidates(ctx, tx); err != nil {
		return bundleOutcome{}, err
	}
	changedGroups, err := rebuildFolderIndex(ctx, tx)
	if err != nil {
		return bundleOutcome{}, err
	}
	if err := touchReshapedPlaylists(ctx, tx, shapes); err != nil {
		return bundleOutcome{}, err
	}
	changed, err := videoIDsForContentKeys(ctx, tx, plan.members)
	if err != nil {
		return bundleOutcome{}, err
	}
	return bundleOutcome{bundleID: bundleID, userKey: userKey,
		changedGroups: normalizeIDs(append(rekeyedGroups, changedGroups...)), changedVideos: changed}, nil
}

func createBundle(ctx context.Context, tx *sql.Tx, representativeKey string, byOwner bool, now int64) (int64, string, error) {
	result, err := tx.ExecContext(ctx, `insert into video_bundles (user_key, representative_key, representative_by_owner,
		created_at, updated_at) values ('bundle:new', ?, ?, ?, ?)`, representativeKey, byOwner, now, now)
	if err != nil {
		return 0, "", fmt.Errorf("cannot create the bundle: %w", err)
	}
	bundleID, err := result.LastInsertId()
	if err != nil {
		return 0, "", fmt.Errorf("cannot create the bundle: %w", err)
	}
	userKey := bundleUserKey(bundleID)
	if _, err := tx.ExecContext(ctx, `update video_bundles set user_key = ? where id = ?`, userKey, bundleID); err != nil {
		return 0, "", fmt.Errorf("cannot create the bundle: %w", err)
	}
	return bundleID, userKey, nil
}

func moveBundleMembers(ctx context.Context, tx *sql.Tx, plan bundlePlan, bundleID, now int64) error {
	for _, key := range plan.members {
		if _, err := tx.ExecContext(ctx,
			`insert into video_bundle_members (content_key, bundle_id, added_at) values (?, ?, ?)
			 on conflict (content_key) do update set bundle_id = excluded.bundle_id`,
			key, bundleID, now); err != nil {
			return fmt.Errorf("cannot add a version to the bundle: %w", err)
		}
	}
	for _, id := range plan.absorbedBundles {
		if _, err := tx.ExecContext(ctx, `delete from video_bundles where id = ?`, id); err != nil {
			return fmt.Errorf("cannot remove the absorbed bundle: %w", err)
		}
	}
	return nil
}

// MakeRepresentative changes no shared user data; missing or unbundled videos fail.
func (s *VersionStore) MakeRepresentative(ctx context.Context, videoID int64) (domain.VideoVersions, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot change the representative: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	key, err := bundlableContentKey(ctx, tx, videoID)
	if err != nil {
		return domain.VideoVersions{}, err
	}
	bundle, bundled, err := bundleOf(ctx, tx, key)
	if err != nil {
		return domain.VideoVersions{}, err
	}
	if !bundled {
		return domain.VideoVersions{}, domain.ErrNotBundled
	}
	if _, err := tx.ExecContext(ctx,
		`update video_bundles set representative_key = ?, representative_by_owner = 1, updated_at = ? where id = ?`,
		key, time.Now().Unix(), bundle.id); err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot change the representative: %w", err)
	}

	changedGroups, err := rebuildFolderIndex(ctx, tx)
	if err != nil {
		return domain.VideoVersions{}, err
	}
	versions, err := videoVersions(ctx, tx, domain.AudienceOwner, videoID)
	if err != nil {
		return domain.VideoVersions{}, err
	}
	members, err := bundleMemberKeys(ctx, tx, bundle.id)
	if err != nil {
		return domain.VideoVersions{}, err
	}
	changed, err := videoIDsForContentKeys(ctx, tx, members)
	if err != nil {
		return domain.VideoVersions{}, err
	}
	c := &changes{}
	if err := recordOwnerVideoActivity(ctx, tx, c, domain.ActivityMakeRepresentative,
		ownerVideoChanges([]string{key}, ""), time.Now()); err != nil {
		return domain.VideoVersions{}, err
	}
	events := append([]domain.Event{domain.VideoBundleChanged{VideoIDs: changed}}, groupsChangedEvents(changedGroups)...)
	if err := s.db.commitWith(ctx, tx, c, events...); err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot change the representative: %w", err)
	}
	return versions, nil
}

// Unbundle restores the leaving video's own data; a final singleton inherits bundle data, and cross-side pairs are held from automatic bundling.
func (s *VersionStore) Unbundle(ctx context.Context, videoID int64) (domain.Video, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Video{}, fmt.Errorf("cannot unbundle the video: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	key, err := bundlableContentKey(ctx, tx, videoID)
	if err != nil {
		return domain.Video{}, err
	}
	bundle, bundled, err := bundleOf(ctx, tx, key)
	if err != nil {
		return domain.Video{}, err
	}
	if !bundled {
		return domain.Video{}, domain.ErrNotBundled
	}
	c := &changes{}
	split, err := splitBundle(ctx, tx, c, bundle, []string{key}, key, true)
	if err != nil {
		return domain.Video{}, err
	}
	if err := recordOwnerVideoActivity(ctx, tx, c, domain.ActivityUnbundle, ownerVideoChanges([]string{key}, ""),
		time.Now()); err != nil {
		return domain.Video{}, err
	}
	video, err := getVideo(ctx, tx, domain.AudienceOwner, videoID)
	if err != nil {
		return domain.Video{}, err
	}
	if err := s.db.commitWith(ctx, tx, c, split.events()...); err != nil {
		return domain.Video{}, fmt.Errorf("cannot unbundle the video: %w", err)
	}
	return video, nil
}

type bundleSplit struct {
	changedVideos []int64
	changedGroups []int64
}

func (s bundleSplit) events() []domain.Event {
	return append([]domain.Event{domain.VideoBundleChanged{VideoIDs: s.changedVideos}, domain.VersionCandidatesChanged{}},
		groupsChangedEvents(s.changedGroups)...)
}

// Split one side atomically; multiple leaving members retain copied bundle data and a singleton remainder inherits
// the original bundle data. holdApart records the cross-side pairs in video_version_unbundles, which keeps automatic
// judging from bundling them again.
func splitBundle(
	ctx context.Context, tx *sql.Tx, c *changes, bundle bundleRef, leaving []string, representative string,
	holdApart bool,
) (bundleSplit, error) {
	members, err := bundleMemberKeys(ctx, tx, bundle.id)
	if err != nil {
		return bundleSplit{}, err
	}
	changed, err := videoIDsForContentKeys(ctx, tx, members)
	if err != nil {
		return bundleSplit{}, err
	}
	shapes, err := snapshotPlaylistShapes(ctx, tx, append(slices.Clone(members), bundle.userKey))
	if err != nil {
		return bundleSplit{}, err
	}
	remaining := slices.DeleteFunc(slices.Clone(members), func(member string) bool { return slices.Contains(leaving, member) })
	now := time.Now().Unix()
	if err := detachLeavingMembers(ctx, tx, bundle, leaving, remaining, representative, holdApart, now); err != nil {
		return bundleSplit{}, err
	}
	rekeyedGroups, err := settleRemainingMembers(ctx, tx, bundle, leaving, remaining, now)
	if err != nil {
		return bundleSplit{}, err
	}
	for _, key := range leaving {
		if err := rebuildCandidatesOf(ctx, tx, c, key); err != nil {
			return bundleSplit{}, err
		}
	}
	changedGroups, err := rebuildFolderIndex(ctx, tx)
	if err != nil {
		return bundleSplit{}, err
	}
	if err := touchReshapedPlaylists(ctx, tx, shapes); err != nil {
		return bundleSplit{}, err
	}
	return bundleSplit{changedVideos: changed, changedGroups: normalizeIDs(append(rekeyedGroups, changedGroups...))}, nil
}

func detachLeavingMembers(
	ctx context.Context, tx *sql.Tx, bundle bundleRef, leaving, remaining []string, representative string,
	holdApart bool, now int64,
) error {
	for _, key := range leaving {
		if holdApart {
			if err := recordUnbundledPairs(ctx, tx, key, remaining, now); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `delete from video_bundle_members where content_key = ?`, key); err != nil {
			return fmt.Errorf("cannot unbundle the video: %w", err)
		}
	}
	if len(leaving) < 2 {
		return nil
	}
	if !slices.Contains(leaving, representative) {
		representative = leaving[0]
	}
	byOwner := bundle.byOwner && representative == bundle.representativeKey
	bundleID, userKey, err := createBundle(ctx, tx, representative, byOwner, now)
	if err != nil {
		return err
	}
	if err := copyUserData(ctx, tx, bundle.userKey, userKey); err != nil {
		return err
	}
	return moveBundleMembers(ctx, tx, bundlePlan{members: leaving}, bundleID, now)
}

func settleRemainingMembers(
	ctx context.Context, tx *sql.Tx, bundle bundleRef, leaving, remaining []string, now int64,
) ([]int64, error) {
	var rekeyedGroups []int64
	switch {
	case len(remaining) <= 1:
		if len(remaining) == 1 {
			if err := replaceUserData(ctx, tx, bundle.userKey, remaining[0]); err != nil {
				return nil, err
			}
			// References move only to a singleton remainder; otherwise they remain on the bundle rather than following leaving members.
			var err error
			if rekeyedGroups, err = rekeyUserItems(ctx, tx, bundle.userKey, remaining[0]); err != nil {
				return nil, err
			}
		}
		if _, err := tx.ExecContext(ctx, `delete from video_bundles where id = ?`, bundle.id); err != nil {
			return nil, fmt.Errorf("cannot dissolve the bundle: %w", err)
		}
	case slices.Contains(leaving, bundle.representativeKey):
		next, err := nextRepresentativeKey(ctx, tx, bundle.id)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx,
			`update video_bundles set representative_key = ?, representative_by_owner = 0, updated_at = ? where id = ?`,
			next, now, bundle.id); err != nil {
			return nil, fmt.Errorf("cannot change the representative: %w", err)
		}
	}
	return rekeyedGroups, nil
}

// Versions returns audience-visible versions with the effective representative first, or the video alone when unbundled.
func (s *VersionStore) Versions(ctx context.Context, audience domain.Audience, videoID int64) (domain.VideoVersions, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot read the versions: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	versions, err := videoVersions(ctx, tx, audience, videoID)
	if err != nil {
		return domain.VideoVersions{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot read the versions: %w", err)
	}
	return versions, nil
}

func videoVersions(ctx context.Context, tx *sql.Tx, audience domain.Audience, videoID int64) (domain.VideoVersions, error) {
	video, err := getVideo(ctx, tx, audience, videoID)
	if err != nil {
		return domain.VideoVersions{}, err
	}
	if video.Versions == nil {
		return domain.VideoVersions{RepresentativeID: video.ID, Items: []domain.Video{video}}, nil
	}
	rows, err := tx.QueryContext(ctx, `select `+videoColumns(audience)+` from videos`+representativeJoin(audience)+`
		join video_bundle_members vm on vm.content_key = videos.content_key
		where vm.bundle_id = (select bundle_id from video_bundle_members where content_key = ?) and `+
		visibleVideoCondition("videos", audience), video.ContentKey)
	if err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot read the versions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := []domain.Video{}
	for rows.Next() {
		item, err := scanVideo(rows)
		if err != nil {
			return domain.VideoVersions{}, fmt.Errorf("cannot read the versions: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot read the versions: %w", err)
	}
	if err := rows.Close(); err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot read the versions: %w", err)
	}
	for i := range items {
		ref := *video.Versions
		items[i].Versions = &ref
	}
	representativeID := video.Versions.RepresentativeID
	domain.SortVideoVersions(items, representativeID)
	return domain.VideoVersions{RepresentativeID: representativeID, Items: items}, nil
}

type rowsQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Batch audience-visible bundle summaries; omit nonmembers and bundles without visible members.
func bundleVersionsRefs(ctx context.Context, q rowsQueryer, audience domain.Audience, keys []string) (map[string]domain.VideoVersionsRef, error) {
	out := map[string]domain.VideoVersionsRef{}
	if len(keys) == 0 {
		return out, nil
	}
	encoded, err := json.Marshal(keys)
	if err != nil {
		return nil, fmt.Errorf("cannot build content keys: %w", err)
	}
	rows, err := q.QueryContext(ctx, `
		with shown(self_key, id, is_representative) as (
			select self.content_key, v.id, v.content_key = b.representative_key
			  from video_bundle_members self
			  join video_bundle_members m on m.bundle_id = self.bundle_id
			  join video_bundles b on b.id = m.bundle_id
			  join videos v on v.content_key = m.content_key and v.content_key <> ''
			 where self.content_key in (select value from json_each(?)) and `+visibleVideoCondition("v", audience)+`)
		select self_key, count(*),
		       (select r.id from shown r where r.self_key = shown.self_key order by r.is_representative desc, r.id limit 1)
		  from shown group by self_key`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the bundles: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var key string
		var ref domain.VideoVersionsRef
		if err := rows.Scan(&key, &ref.Count, &ref.RepresentativeID); err != nil {
			return nil, fmt.Errorf("cannot read the bundles: %w", err)
		}
		out[key] = ref
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the bundles: %w", err)
	}
	return out, nil
}

// Only bundle members have differing user/content keys, so skip the summary query when none are present.
func attachVersions(ctx context.Context, q rowsQueryer, audience domain.Audience, videos []domain.Video) error {
	var keys []string
	for _, video := range videos {
		if video.ContentKey != "" && video.UserKey != video.ContentKey {
			keys = append(keys, video.ContentKey)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	refs, err := bundleVersionsRefs(ctx, q, audience, keys)
	if err != nil {
		return err
	}
	for i := range videos {
		if ref, ok := refs[videos[i].ContentKey]; ok {
			videos[i].Versions = &ref
		}
	}
	return nil
}

func attachVersionsEach(ctx context.Context, q rowsQueryer, audience domain.Audience, lists [][]domain.Video) error {
	var flat []domain.Video
	for _, list := range lists {
		flat = append(flat, list...)
	}
	if err := attachVersions(ctx, q, audience, flat); err != nil {
		return err
	}
	offset := 0
	for _, list := range lists {
		offset += copy(list, flat[offset:])
	}
	return nil
}

func attachVersionsIn[K comparable](ctx context.Context, q rowsQueryer, audience domain.Audience, videos map[K]domain.Video) error {
	keys := make([]K, 0, len(videos))
	list := make([]domain.Video, 0, len(videos))
	for key, video := range videos {
		keys = append(keys, key)
		list = append(list, video)
	}
	if err := attachVersions(ctx, q, audience, list); err != nil {
		return err
	}
	for i, key := range keys {
		videos[key] = list[i]
	}
	return nil
}

// Bundling requires identified content with a registered location.
func bundlableContentKey(ctx context.Context, tx *sql.Tx, videoID int64) (string, error) {
	var key string
	err := tx.QueryRowContext(ctx, `select v.content_key from videos v
		where v.id = ? and v.content_key <> '' and `+registeredVideoCondition("v"), videoID).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("cannot read the video (id=%d): %w", videoID, err)
	}
	return key, nil
}

func bundleOf(ctx context.Context, tx *sql.Tx, contentKey string) (bundleRef, bool, error) {
	var ref bundleRef
	err := tx.QueryRowContext(ctx, `select b.id, b.user_key, b.representative_key, b.representative_by_owner
		from video_bundle_members m join video_bundles b on b.id = m.bundle_id
		where m.content_key = ?`, contentKey).Scan(&ref.id, &ref.userKey, &ref.representativeKey, &ref.byOwner)
	if errors.Is(err, sql.ErrNoRows) {
		return bundleRef{}, false, nil
	}
	if err != nil {
		return bundleRef{}, false, fmt.Errorf("cannot read the bundle: %w", err)
	}
	return ref, true, nil
}

func bundleMemberKeys(ctx context.Context, tx *sql.Tx, bundleID int64) ([]string, error) {
	return queryStrings(ctx, tx, `select content_key from video_bundle_members where bundle_id = ? order by content_key`, bundleID)
}

// Prefer the lowest registered video ID, falling back to the smallest member key to preserve the representative-membership invariant.
func nextRepresentativeKey(ctx context.Context, tx *sql.Tx, bundleID int64) (string, error) {
	var key string
	err := tx.QueryRowContext(ctx, `select v.content_key from video_bundle_members m
		join videos v on v.content_key = m.content_key and v.content_key <> ''
		where m.bundle_id = ? and `+registeredVideoCondition("v")+` order by v.id limit 1`, bundleID).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `select min(content_key) from video_bundle_members where bundle_id = ?`, bundleID).Scan(&key)
	}
	if err != nil {
		return "", fmt.Errorf("cannot choose the next representative: %w", err)
	}
	return key, nil
}

func videoIDsForContentKeys(ctx context.Context, tx *sql.Tx, keys []string) ([]int64, error) {
	encoded, err := json.Marshal(keys)
	if err != nil {
		return nil, fmt.Errorf("cannot build content keys: %w", err)
	}
	rows, err := tx.QueryContext(ctx,
		`select id from videos where content_key in (select value from json_each(?)) order by id`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the bundled videos: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cannot read the bundled videos: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the bundled videos: %w", err)
	}
	return ids, nil
}

// Copy without overwriting destination rows and preserve auto-tag provenance for undo.
func copyUserData(ctx context.Context, tx *sql.Tx, from, to string) error {
	for _, t := range userDataTables {
		if err := copyUserTable(ctx, tx, t.table, t.columns, from, to); err != nil {
			return err
		}
	}
	return copyAutoTagKeys(ctx, tx, from, to)
}

func copyUserTable(ctx context.Context, tx *sql.Tx, table, columns, from, to string) error {
	if _, err := tx.ExecContext(ctx, `insert or ignore into `+table+` (content_key, `+columns+`)
		select ?, `+columns+` from `+table+` where content_key = ?`, to, from); err != nil {
		return fmt.Errorf("cannot copy the %s values: %w", table, err)
	}
	return nil
}

// mergeUserData gives a new bundle every side's user data, so choosing the representative never loses any: the
// union of tags, favourites and publication, the latest playback position, and the first side's work information
// that has any, the representative's first. See docs/design-docs/video-versions.md.
func mergeUserData(ctx context.Context, tx *sql.Tx, sources []string, to string) error {
	workCopied := false
	for _, from := range sources {
		if err := copyNewAutoTagKeys(ctx, tx, from, to); err != nil {
			return err
		}
		for _, t := range userDataTables {
			var err error
			switch {
			case strings.HasPrefix(t.table, "video_work_"):
				continue
			case t.table == "playback_progress":
				_, err = tx.ExecContext(ctx, `insert into playback_progress (content_key, `+t.columns+`)
					select ?, `+t.columns+` from playback_progress where content_key = ? and true
					on conflict (content_key) do update set position_ms = excluded.position_ms,
					duration_ms = excluded.duration_ms, completed = excluded.completed, updated_at = excluded.updated_at
					where excluded.updated_at > playback_progress.updated_at`, to, from)
			default:
				err = copyUserTable(ctx, tx, t.table, t.columns, from, to)
			}
			if err != nil {
				return fmt.Errorf("cannot merge the %s values: %w", t.table, err)
			}
		}
		if workCopied {
			continue
		}
		var err error
		if workCopied, err = copyWorkInfo(ctx, tx, from, to); err != nil {
			return err
		}
	}
	return nil
}

// Work information is one record across its tables; mixing two sides' would describe neither work.
func copyWorkInfo(ctx context.Context, tx *sql.Tx, from, to string) (bool, error) {
	var conditions []string
	for _, t := range userDataTables {
		if strings.HasPrefix(t.table, "video_work_") {
			conditions = append(conditions, `exists (select 1 from `+t.table+` where content_key = ?1)`)
		}
	}
	var found bool
	if err := tx.QueryRowContext(ctx, `select `+strings.Join(conditions, " or "), from).Scan(&found); err != nil || !found {
		return false, err
	}
	for _, t := range userDataTables {
		if strings.HasPrefix(t.table, "video_work_") {
			if err := copyUserTable(ctx, tx, t.table, t.columns, from, to); err != nil {
				return false, err
			}
		}
	}
	return true, nil
}

// Replace all destination user data and drop obsolete auto-tag provenance before copying.
func replaceUserData(ctx context.Context, tx *sql.Tx, from, to string) error {
	for _, t := range userDataTables {
		if _, err := tx.ExecContext(ctx, `delete from `+t.table+` where content_key = ?`, to); err != nil {
			return fmt.Errorf("cannot replace the %s values: %w", t.table, err)
		}
	}
	if err := dropAutoTagKeys(ctx, tx, to); err != nil {
		return err
	}
	return copyUserData(ctx, tx, from, to)
}

func queryStrings(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]string, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read the bundle: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, fmt.Errorf("cannot read the bundle: %w", err)
		}
		out = append(out, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the bundle: %w", err)
	}
	return out, nil
}

func uniqueIDs(ids []int64) []int64 {
	seen := make(map[int64]bool, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
