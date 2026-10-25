package store

// Filter individual visible videos through every ancestor rule; materialize shared location, progress and favorite data once per read.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// Use listing-compatible column names and representative locations so existing video predicates apply directly to smart_base.
func smartBaseCTE(audience domain.Audience) (string, []any) {
	cte, args := chosenLocationsCTEFor(libraryScope(audience), audience, nil)
	// Compute each user-data key once for progress, favorites and predicates.
	return cte + `,
	smart_keys as materialized (
		select videos.id as id, videos.content_key as content_key, ` + userKeyExpr("videos") + ` as user_key,
			videos.added_at as added_at, videos.duration_ms as duration_ms,
			videos.thumbnail_state as thumbnail_state, chosen.path as path
		from chosen join videos on videos.id = chosen.video_id),
	smart_base as materialized (
		select k.id, k.content_key, k.user_key, k.added_at, k.duration_ms, k.thumbnail_state,
			` + locationMTimeExpr("loc") + ` as mtime, ` + locationCreatedAtExpr("loc") + ` as created_at,
			` + locationTitleKeyExpr("loc") + ` as title_key,
			loc.size_bytes as size_bytes, p.updated_at as played_at, fav.favorited_at as favorited_at,
			` + videoWatchStateExpr + ` as watch_state
		from smart_keys k` + chosenLocationJoin("k.path", "k") + `
		left join playback_progress p on p.content_key = k.user_key and k.content_key <> ''
		left join video_favorites fav on fav.content_key = k.user_key and k.content_key <> '')`, args
}

type smartMatch struct {
	search     string
	searchArgs []any
	condition  string
	args       []any
}

// Apply every rule to each video using owner search semantics; all search expressions must match one location and are materialized once.
func smartMatchFor(spec listSpec, name string) smartMatch {
	spec.userKeyColumn = `videos.user_key`
	conditions, args := matchConditions(spec)
	if spec.anyLevel(func(level listSpec) bool { return level.favoriteOnly }) {
		conditions = append(conditions, `videos.favorited_at is not null`)
	}
	var match smartMatch
	if slices.ContainsFunc(spec.searchExprs(), func(expr domain.SearchExpr) bool { return !expr.Empty() }) {
		search, searchArgs := chosenLocationsSelect(spec.scope, domain.AudienceOwner, spec.searchExprs())
		match.search, match.searchArgs = name+` as materialized (`+search+`)`, searchArgs
		conditions = append(conditions, `videos.id in (select video_id from `+name+`)`)
	}
	match.condition, match.args = `1`, args
	if len(conditions) > 0 {
		match.condition = strings.Join(conditions, " and ")
	}
	return match
}

type smartFolderTarget struct {
	summary domain.SmartFolderSummary
	sort    domain.VideoSort
	spec    listSpec
	// conditions come from the folder's own rule, without its ancestors'.
	conditions domain.VideoConditions
	// A missing referenced entity makes the folder match nothing.
	missing bool
}

// Owners see empty folders; guests require at least one playback item.
func smartFolderTargetOf(ctx context.Context, tx *sql.Tx, audience domain.Audience, folder domain.SmartFolder, rules []domain.SmartRule) (smartFolderTarget, bool, error) {
	target := smartFolderTarget{
		summary:    domain.SmartFolderSummary{ID: folder.ID, ParentID: folder.ParentID, Name: folder.Name},
		sort:       folder.Sort,
		spec:       smartFolderSpec(audience, rules, folder.Sort),
		conditions: folder.Rule.VideoConditions,
	}
	missing, err := missingSmartRuleRefs(ctx, tx, rules)
	if err != nil {
		return smartFolderTarget{}, false, err
	}
	target.missing = len(missing) > 0
	if err := tx.QueryRowContext(ctx, `select exists (select 1 from smart_folder_favorites where smart_folder_id = ?)`,
		folder.ID).Scan(&target.summary.Favorite); err != nil {
		return smartFolderTarget{}, false, fmt.Errorf("cannot read the smart folder favorite: %w", err)
	}
	if audience.IsOwner() {
		return target, true, nil
	}
	if target.missing {
		return target, false, nil
	}
	shown, err := smartFolderHasItems(ctx, tx, target.spec)
	return target, shown, err
}

func smartFolderTargetByID(ctx context.Context, tx *sql.Tx, audience domain.Audience, id int64) (smartFolderTarget, bool, error) {
	folder, err := readSmartFolder(ctx, tx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return smartFolderTarget{}, false, nil
	}
	if err != nil {
		return smartFolderTarget{}, false, err
	}
	rules, err := smartFolderPathRules(ctx, tx, id)
	if err != nil {
		return smartFolderTarget{}, false, err
	}
	return smartFolderTargetOf(ctx, tx, audience, folder, rules)
}

// Deduplicate and preserve input order while selecting existing audience-visible folders.
func shownSmartFolderTargets(ctx context.Context, tx *sql.Tx, audience domain.Audience, ids []int64) ([]smartFolderTarget, error) {
	targets := make([]smartFolderTarget, 0, len(ids))
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		target, shown, err := smartFolderTargetByID(ctx, tx, audience, id)
		if err != nil {
			return nil, err
		}
		if shown {
			targets = append(targets, target)
		}
	}
	return targets, nil
}

func smartFolderHasItems(ctx context.Context, tx *sql.Tx, spec listSpec) (bool, error) {
	cte, args := libraryItemsCTE(spec)
	var found bool
	if err := tx.QueryRowContext(ctx, cte+` select exists (select 1 from items)`, args...).Scan(&found); err != nil {
		return false, fmt.Errorf("cannot read whether the smart folder has items: %w", err)
	}
	return found, nil
}

type smartFolderQuery struct {
	ctes     []string
	cteArgs  []any
	parts    []string
	partArgs []any
	// Reuse earlier results for identical rules and order instead of adding another SQL branch.
	same map[int]int
}

// Skip impossible targets and reuse identical rule/order branches; callers copy those results through query.same.
func smartFolderBranches(targets []smartFolderTarget, branch func(query *smartFolderQuery, index int, target smartFolderTarget, match smartMatch)) smartFolderQuery {
	query := smartFolderQuery{same: map[int]int{}}
	first := map[string]int{}
	for i, target := range targets {
		if target.missing {
			continue
		}
		key := smartMatchFor(target.spec, `s`)
		identity := fmt.Sprintf("%s\x00%s\x00%v\x00%s\x00%v", target.sort, key.search, key.searchArgs, key.condition, key.args)
		if earlier, ok := first[identity]; ok {
			query.same[i] = earlier
			continue
		}
		first[identity] = i
		match := smartMatchFor(target.spec, `s`+strconv.Itoa(i))
		if match.search != "" {
			query.ctes = append(query.ctes, match.search)
			query.cteArgs = append(query.cteArgs, match.searchArgs...)
		}
		branch(&query, i, target, match)
	}
	return query
}

func (q smartFolderQuery) run(ctx context.Context, tx *sql.Tx, audience domain.Audience) (*sql.Rows, error) {
	if len(q.parts) == 0 {
		return nil, nil
	}
	cte, args := smartBaseCTE(audience)
	for _, item := range q.ctes {
		cte += `, ` + item
	}
	args = append(append(args, q.cteArgs...), q.partArgs...)
	return tx.QueryContext(ctx, cte+` `+strings.Join(q.parts, ` union all `), args...)
}
