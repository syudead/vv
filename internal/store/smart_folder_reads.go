package store

// Compute contents on demand from stacked ancestor rules; predicates use owner facts while visibility uses the actual audience.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// SmartFolderContents reads one snapshot, prepending child folders on the first page; owners can read empty folders, guests cannot.
func (s *SmartFolderStore) SmartFolderContents(ctx context.Context, audience domain.Audience, id int64, cursor string, limit int) (domain.SmartFolderPage, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.SmartFolderPage{}, fmt.Errorf("cannot start reading the smart folder: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	folder, err := readSmartFolder(ctx, tx, id)
	if err != nil {
		return domain.SmartFolderPage{}, err
	}
	rules, err := smartFolderPathRules(ctx, tx, id)
	if err != nil {
		return domain.SmartFolderPage{}, err
	}
	now := listNow().Unix()
	view, err := viewSmartFolder(ctx, tx, audience, folder, rules, now, &smartFolderPageRequest{cursor: cursor, limit: limit})
	if err != nil {
		return domain.SmartFolderPage{}, err
	}
	if !view.shown(audience) {
		return domain.SmartFolderPage{}, smartFolderMissing()
	}
	page := domain.SmartFolderPage{
		SmartFolder: view.card, Folder: folder, Missing: view.missing,
		Items: []domain.LibraryItem{}, NextCursor: view.nextCursor,
	}
	if page.Trail, err = smartFolderTrail(ctx, tx, id); err != nil {
		return domain.SmartFolderPage{}, err
	}
	if cursor == "" {
		if page.Items, err = smartFolderChildren(ctx, tx, audience, id, rules, now); err != nil {
			return domain.SmartFolderPage{}, err
		}
	}
	page.Items = append(page.Items, view.items...)
	if page.Roots, err = listMediaFolders(ctx, tx); err != nil {
		return domain.SmartFolderPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.SmartFolderPage{}, fmt.Errorf("cannot finish reading the smart folder: %w", err)
	}
	return page, nil
}

type smartFolderPageRequest struct {
	cursor string
	limit  int
}

type smartFolderView struct {
	card       domain.LibrarySmartFolder
	missing    []domain.SmartRuleRef
	items      []domain.LibraryItem
	nextCursor string
}

func (v smartFolderView) shown(audience domain.Audience) bool {
	return audience.IsOwner() || len(v.card.Members) > 0
}

// Use one captured time for every folder and one query for playback order plus the requested page.
func viewSmartFolder(ctx context.Context, tx *sql.Tx, audience domain.Audience, folder domain.SmartFolder, rules []domain.SmartRule, now int64, request *smartFolderPageRequest) (smartFolderView, error) {
	var view smartFolderView
	var err error
	if view.missing, err = missingSmartRuleRefs(ctx, tx, rules); err != nil {
		return smartFolderView{}, err
	}
	spec := smartFolderSpec(audience, rules, folder.Sort, now)
	if request != nil {
		spec.cursor, spec.limit = request.cursor, request.limit
	}
	var contents smartFolderContents
	if len(view.missing) > 0 && request != nil && spec.cursor != "" {
		// Reject malformed cursors even when no contents need loading.
		if _, _, err := itemOrder(spec.sort).cursorCondition(spec, spec.cursor); err != nil {
			return smartFolderView{}, err
		}
	}
	if len(view.missing) == 0 {
		if contents, err = readSmartFolderContents(ctx, tx, spec, request != nil); err != nil {
			return smartFolderView{}, err
		}
		if request != nil {
			view.items, view.nextCursor, err = libraryPageItems(ctx, tx, spec, contents.order, contents.keys,
				contents.values, contents.limit)
			if err != nil {
				return smartFolderView{}, err
			}
		}
	}
	members, progress, err := playOrderMembers(ctx, tx, audience, contents.playOrder)
	if err != nil {
		return smartFolderView{}, err
	}
	view.card = domain.LibrarySmartFolder{
		LibraryGroup: domain.NewLibraryGroup(folder.ID, "", folder.Name, members, progress),
		ParentID:     folder.ParentID,
	}
	if err := tx.QueryRowContext(ctx, `select exists (select 1 from smart_folder_favorites where smart_folder_id = ?)`,
		folder.ID).Scan(&view.card.Favorite); err != nil {
		return smartFolderView{}, fmt.Errorf("cannot read the smart folder favorite: %w", err)
	}
	return view, nil
}

// Stack ancestor filters while retaining viewer scope and owner search semantics; resolve every referenced tag first.
func smartFolderSpec(audience domain.Audience, rules []domain.SmartRule, sort domain.VideoSort, now int64) listSpec {
	levels := make([]listSpec, 0, len(rules))
	for _, rule := range rules {
		q := rule.Query()
		level := libraryListSpec(audience, q, q.TagIDs)
		level.now = now
		levels = append(levels, level)
	}
	spec := levels[len(levels)-1]
	spec.and = levels[:len(levels)-1]
	spec.ownerSearch = true
	spec.sort = sort
	return spec
}

type smartFolderContents struct {
	playOrder []int64
	// Retain up to limit+1 keys and sort values after the cursor to detect continuation.
	keys   []itemRow
	values []any
	order  listOrder
	limit  int
}

// Expand group items into playback order and deduplicate videos at first occurrence, collecting page keys in the same read.
func readSmartFolderContents(ctx context.Context, tx *sql.Tx, spec listSpec, withPage bool) (smartFolderContents, error) {
	contents := smartFolderContents{order: itemOrder(spec.sort), limit: normalizeLimit(spec.limit)}
	cte, args := libraryItemsCTE(spec)
	if contents.order.seeded {
		args = append(args, spec.seed)
	}
	where, watchArgs := itemsWhere(spec)
	args = append(args, watchArgs...)
	after := `1`
	if withPage && spec.cursor != "" {
		clause, cursorArgs, err := contents.order.cursorCondition(spec, spec.cursor)
		if err != nil {
			return smartFolderContents{}, err
		}
		after = clause
		args = append(args, cursorArgs...)
	}
	rows, err := tx.QueryContext(ctx, cte+`, filtered as materialized (select items.*, `+contents.order.value+
		` as sort_value from items`+where+`)
		select f.group_id, f.id, f.video_id, f.path, f.sort_value, `+after+`, coalesce(gm.video_id, f.video_id)
		from filtered f left join gm on gm.group_id = f.group_id
		order by `+contents.order.orderBy()+`, gm.position`, args...)
	if err != nil {
		return smartFolderContents{}, fmt.Errorf("cannot read the smart folder contents: %w", err)
	}
	defer func() { _ = rows.Close() }()
	seen := map[int64]bool{}
	lastID := int64(-1)
	for rows.Next() {
		var key itemRow
		var value any
		var afterCursor bool
		var videoID int64
		if err := rows.Scan(&key.groupID, &key.id, &key.videoID, &key.path, &value, &afterCursor, &videoID); err != nil {
			return smartFolderContents{}, fmt.Errorf("cannot read the smart folder contents: %w", err)
		}
		if !seen[videoID] {
			seen[videoID] = true
			contents.playOrder = append(contents.playOrder, videoID)
		}
		// A group spans multiple member rows but counts as one item key.
		if withPage && afterCursor && key.id != lastID && len(contents.keys) <= contents.limit {
			contents.keys = append(contents.keys, key)
			contents.values = append(contents.values, value)
		}
		lastID = key.id
	}
	if err := rows.Err(); err != nil {
		return smartFolderContents{}, fmt.Errorf("cannot read the smart folder contents: %w", err)
	}
	return contents, nil
}

// Read members in playback order with representative locations; guests receive no playback records.
func playOrderMembers(ctx context.Context, tx *sql.Tx, audience domain.Audience, ids []int64) ([]domain.Video, []*domain.Progress, error) {
	if len(ids) == 0 {
		return nil, nil, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot build video ids: %w", err)
	}
	progressColumns, progressJoin := memberProgressSelect(audience)
	rows, err := tx.QueryContext(ctx, `select `+videoColumns(audience)+`, `+progressColumns+` from videos`+
		representativeJoin(audience)+progressJoin+` where videos.id in (select value from json_each(?))`, string(encoded))
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read the smart folder videos: %w", err)
	}
	defer func() { _ = rows.Close() }()
	type member struct {
		video    domain.Video
		progress *domain.Progress
	}
	byID := make(map[int64]member, len(ids))
	for rows.Next() {
		var scanned memberProgress
		video, err := scanVideo(extraScanner{rows: rows, extra: scanned.dest()})
		if err != nil {
			return nil, nil, fmt.Errorf("cannot read the smart folder videos: %w", err)
		}
		byID[video.ID] = member{video: video, progress: scanned.progress()}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("cannot read the smart folder videos: %w", err)
	}
	videos := make([]domain.Video, 0, len(ids))
	progress := make([]*domain.Progress, 0, len(ids))
	for _, id := range ids {
		found, ok := byID[id]
		if !ok {
			return nil, nil, fmt.Errorf("cannot read the smart folder video (id=%d)", id)
		}
		videos = append(videos, found.video)
		progress = append(progress, found.progress)
	}
	return videos, progress, nil
}

// Append each child's rule to the parent rules; hide empty child playback orders from guests.
func smartFolderChildren(ctx context.Context, tx *sql.Tx, audience domain.Audience, parent int64, parentRules []domain.SmartRule, now int64) ([]domain.LibraryItem, error) {
	ids, err := queryInt64s(ctx, tx, `select id from smart_folders where parent_id = ? order by title_key, id`, parent)
	if err != nil {
		return nil, fmt.Errorf("cannot read the child smart folders: %w", err)
	}
	items := make([]domain.LibraryItem, 0, len(ids))
	for _, id := range ids {
		child, err := readSmartFolder(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		target, shown, err := smartFolderTargetOf(ctx, tx, audience, child, append(slices.Clone(parentRules), child.Rule), now)
		if err != nil {
			return nil, err
		}
		if shown {
			items = append(items, domain.LibraryItem{SmartFolder: &target.summary})
		}
	}
	return items, nil
}

func smartFolderTrail(ctx context.Context, tx *sql.Tx, id int64) ([]domain.GroupRef, error) {
	rows, err := tx.QueryContext(ctx, `with recursive up (id, depth) as (
			select parent_id, 1 from smart_folders where id = ? and parent_id is not null
			union
			select f.parent_id, up.depth + 1 from up join smart_folders f on f.id = up.id where f.parent_id is not null)
		select f.id, f.name from up join smart_folders f on f.id = up.id order by up.depth desc`, id)
	if err != nil {
		return nil, fmt.Errorf("cannot read the smart folder trail: %w", err)
	}
	defer func() { _ = rows.Close() }()
	trail := []domain.GroupRef{}
	for rows.Next() {
		var ref domain.GroupRef
		if err := rows.Scan(&ref.ID, &ref.Name); err != nil {
			return nil, fmt.Errorf("cannot read the smart folder trail: %w", err)
		}
		trail = append(trail, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the smart folder trail: %w", err)
	}
	return trail, nil
}

func ownerSmartFolderCard(ctx context.Context, tx *sql.Tx, id int64, now int64) (domain.LibrarySmartFolder, error) {
	folder, err := readSmartFolder(ctx, tx, id)
	if err != nil {
		return domain.LibrarySmartFolder{}, err
	}
	rules, err := smartFolderPathRules(ctx, tx, id)
	if err != nil {
		return domain.LibrarySmartFolder{}, err
	}
	view, err := viewSmartFolder(ctx, tx, domain.AudienceOwner, folder, rules, now, nil)
	if err != nil {
		return domain.LibrarySmartFolder{}, err
	}
	return view.card, nil
}

// Missing referenced entities yield an empty playback order; a missing folder returns false.
func smartFolderVideoGroup(ctx context.Context, tx *sql.Tx, audience domain.Audience, id int64) (domain.VideoGroup, bool, error) {
	folder, err := readSmartFolder(ctx, tx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.VideoGroup{}, false, nil
	}
	if err != nil {
		return domain.VideoGroup{}, false, err
	}
	rules, err := smartFolderPathRules(ctx, tx, id)
	if err != nil {
		return domain.VideoGroup{}, false, err
	}
	group := domain.VideoGroup{ID: id, SmartFolder: true, Name: folder.Name}
	missing, err := missingSmartRuleRefs(ctx, tx, rules)
	if err != nil || len(missing) > 0 {
		return group, err == nil, err
	}
	contents, err := readSmartFolderContents(ctx, tx, smartFolderSpec(audience, rules, folder.Sort, listNow().Unix()), false)
	if err != nil {
		return domain.VideoGroup{}, false, err
	}
	group.MemberIDs = contents.playOrder
	return group, true, nil
}
