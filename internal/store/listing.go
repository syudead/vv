package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"modernc.org/sqlite"

	"github.com/syudead/vv/internal/domain"
)

// Choose one matching location per video, then apply item filters and keyset pagination.

type locationScopeKind int

const (
	scopeLibrary locationScopeKind = iota
	scopeDirect
	scopeSubtree
)

// Every scope is restricted to audience-visible locations.
type locationScope struct {
	kind locationScopeKind
	// Absolute folder path; unused for the library scope.
	dir      string
	audience domain.Audience
}

func libraryScope(audience domain.Audience) locationScope {
	return locationScope{kind: scopeLibrary, audience: audience}
}

func folderScope(dir string, scope domain.FolderScope, audience domain.Audience) locationScope {
	if scope == domain.FolderScopeSubtree {
		return locationScope{kind: scopeSubtree, dir: dir, audience: audience}
	}
	return locationScope{kind: scopeDirect, dir: dir, audience: audience}
}

func (s locationScope) condition(alias string) (string, []any) {
	visible := visibleLocationCondition(alias, s.audience)
	if s.kind == scopeLibrary {
		return visible, nil
	}
	prefix := folderPrefix(s.dir)
	inFolder, args := folderPrefixCondition(alias, prefix)
	clause := visible + ` and ` + inFolder
	if s.kind == scopeDirect {
		clause += ` and ` + directChildCondition(alias)
		args = append(args, prefix)
	}
	return clause, args
}

type listSpec struct {
	scope        locationScope
	expr         domain.SearchExpr
	watch        domain.WatchFilter
	playableOnly bool
	// Callers resolve tag IDs before building the spec.
	tagIDs       []int64
	favoriteOnly bool
	// Match credits in any role; zero disables the filter.
	personID       int64
	organizationID int64
	// Match manual or folder-derived tags of this kind; zero disables the filter.
	tagKindID int64
	// Zero disables age filtering; nil duration bounds are unbounded.
	addedWithinDays int
	minDurationSec  *int
	maxDurationSec  *int
	// Capture time once per read so counts and all page queries share the same age boundary.
	now int64
	// Stack ancestor filters for library items; ignore each level's scope, ordering, pagination and nested and field.
	and []listSpec
	// Smart-folder predicates use owner search semantics even for guests.
	ownerSearch bool
	// Use the precomputed smart_base user key when supplied, avoiding repeated derivation.
	userKeyColumn       string
	smartFolders        []librarySmartFolder
	storedCollections   bool
	includeSmartFolders bool
	sort                domain.VideoSort
	seed                int64
	cursor              string
	limit               int
}

// ListVideos uses keyset pagination and one snapshot for tag existence, counts and rows; callers validate audience filter permissions.
func (s *LibraryStore) ListVideos(ctx context.Context, audience domain.Audience, q domain.VideoQuery) (domain.VideoPage, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.VideoPage{}, fmt.Errorf("cannot start reading the list: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	tagIDs, missingTagIDs, err := existingTagIDs(ctx, tx, q.TagIDs)
	if err != nil {
		return domain.VideoPage{}, err
	}
	page, err := listVideoPageTx(ctx, tx, libraryListSpec(audience, q, tagIDs))
	if err != nil {
		return domain.VideoPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.VideoPage{}, fmt.Errorf("cannot finish reading the list: %w", err)
	}
	page.MissingTagIDs = missingTagIDs
	return page, nil
}

var listNow = time.Now

func libraryListSpec(audience domain.Audience, q domain.VideoQuery, tagIDs []int64) listSpec {
	return listSpec{
		scope: libraryScope(audience), expr: domain.ParseSearchQuery(q.Query),
		watch: q.Watch, playableOnly: q.PlayableOnly, tagIDs: tagIDs, favoriteOnly: q.FavoriteOnly,
		personID: q.Person, organizationID: q.Organization, tagKindID: q.TagKindID,
		addedWithinDays: q.AddedWithinDays, minDurationSec: q.MinDurationSec, maxDurationSec: q.MaxDurationSec,
		now:  listNow().Unix(),
		sort: q.Sort, seed: q.Seed, cursor: q.Cursor, limit: q.Limit,
	}
}

// ListFolderVideos uses ListVideos semantics within the requested folder scope, taking titles from matching locations.
func (s *LibraryStore) ListFolderVideos(ctx context.Context, audience domain.Audience, q domain.FolderVideoQuery) (domain.VideoPage, error) {
	return s.listVideoPage(ctx, listSpec{
		scope: folderScope(q.Dir, q.Scope, audience), expr: domain.ParseSearchQuery(q.Query),
		watch: q.Watch, playableOnly: q.PlayableOnly, favoriteOnly: q.FavoriteOnly,
		sort: q.Sort, seed: q.Seed, cursor: q.Cursor, limit: q.Limit,
	})
}

func (s *LibraryStore) CountVideos(ctx context.Context, audience domain.Audience, search string) (int, error) {
	return s.countVideos(ctx, listSpec{scope: libraryScope(audience), expr: domain.ParseSearchQuery(search)})
}

// Choose the smallest in-scope representative path; every search term must match one location, including a bundle member's registered location.
func chosenLocationsCTE(scope locationScope, expr domain.SearchExpr) (string, []any) {
	return chosenLocationsCTEFor(scope, scope.audience, []domain.SearchExpr{expr})
}

// Search semantics may use the owner while scope and representative visibility use the actual audience.
func chosenLocationsCTEFor(scope locationScope, searchAudience domain.Audience, exprs []domain.SearchExpr) (string, []any) {
	selectSQL, args := chosenLocationsSelect(scope, searchAudience, exprs)
	return `with chosen as (` + selectSQL + `)`, args
}

func chosenLocationsSelect(scope locationScope, searchAudience domain.Audience, exprs []domain.SearchExpr) (string, []any) {
	scopeClause, args := scope.condition("l")
	clauses := []string{scopeClause, `(sbm.bundle_id is null or sv.id = ` +
		effectiveRepresentativeExpr("sbm.bundle_id", scope.audience) + `)`}
	// All expressions must match the same location row, including inside a bundle.
	var exprClauses, bundleClauses []string
	var exprArgs, bundleArgs []any
	for _, expr := range exprs {
		exprClause, eArgs := searchExprCondition(expr, "l", searchAudience)
		if exprClause == "" {
			continue
		}
		bundleClause, bArgs := searchExprCondition(expr, "bl", searchAudience)
		exprClauses, bundleClauses = append(exprClauses, exprClause), append(bundleClauses, bundleClause)
		exprArgs, bundleArgs = append(exprArgs, eArgs...), append(bundleArgs, bArgs...)
	}
	if len(exprClauses) > 0 {
		clauses = append(clauses, `((sbm.bundle_id is null and `+strings.Join(exprClauses, " and ")+`) or (sbm.bundle_id is not null and exists (
			select 1 from video_bundle_members bm
			join videos bv on bv.content_key = bm.content_key and bv.content_key <> ''
			join video_locations bl on bl.video_id = bv.id
			where bm.bundle_id = sbm.bundle_id and `+registeredLocationCondition("bl")+` and `+strings.Join(bundleClauses, " and ")+`)))`)
		args = append(args, exprArgs...)
		args = append(args, bundleArgs...)
	}
	return `select l.video_id, min(l.path) as path from video_locations l
		join videos sv on sv.id = l.video_id
		left join video_bundle_members sbm on sbm.content_key = sv.content_key and sv.content_key <> ''
		where ` + strings.Join(clauses, " and ") + `
		group by l.video_id`, args
}

// Keep watch-state filtering equivalent to domain.ClassifyWatch.
func watchCondition(filter domain.WatchFilter) string {
	switch filter {
	case domain.WatchUnwatched:
		return `(p.content_key is null or (p.completed = 0 and p.position_ms = 0))`
	case domain.WatchInProgress:
		return `(p.content_key is not null and p.completed = 0 and p.position_ms > 0)`
	case domain.WatchWatched:
		return `p.completed = 1`
	default:
		return ""
	}
}

// Keep equivalent to domain.Video.PlayableInBrowser.
const playableCondition = `videos.playable = 1 and videos.probe_state = 'done'`

// Column order matches scanVideo; location attributes come from chosen, and path supports folder metadata.
var listColumns = `videos.id, chosen.path, loc.title, loc.size_bytes, loc.mtime,
	` + fileCreatedAtExpr("loc") + ` as file_created_at, videos.added_at, videos.indexed_at, videos.content_key, ` + userKeyExpr("videos") + ` as user_key, videos.duration_ms, videos.width,
	videos.height, videos.display_aspect_ratio, videos.container, videos.video_codec, videos.audio_codec, videos.playable,
	videos.unplayable_reason, videos.probe_state, videos.probe_error, videos.probe_error_code, videos.thumbnail_state, videos.seek_thumbnail_state, videos.preview_state,
	` + publicColumn + ` as public, ` + favoriteColumn + ` as favorite, ` + overrideColumns + `, ` + editedAtColumn

// Bind arguments in SQL order: CTE and optional shuffle seed, then this clause, then cursor arguments.
func filteredFrom(spec listSpec, withLocation bool) (string, []any) {
	from := ` from chosen join videos on videos.id = chosen.video_id`
	if withLocation {
		from += ` join video_locations loc on loc.path = chosen.path`
	}
	// Unidentified content has no playback position; use the shared user-data key.
	from += ` left join playback_progress p on p.content_key = ` + userKeyExpr("videos") + ` and videos.content_key <> ''`
	from += ` left join video_favorites fav on fav.content_key = ` + userKeyExpr("videos") + ` and videos.content_key <> ''`
	var conditions []string
	if condition := watchCondition(spec.watch); condition != "" {
		conditions = append(conditions, condition)
	}
	if spec.favoriteOnly {
		conditions = append(conditions, `fav.content_key is not null`)
	}
	member, args := memberConditions(spec)
	conditions = append(conditions, member...)
	if len(conditions) > 0 {
		from += ` where ` + strings.Join(conditions, " and ")
	}
	return from, args
}

// Count all filtered videos independently of the cursor.
type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func countVideosWith(ctx context.Context, db queryRower, spec listSpec) (int, error) {
	cte, args := chosenLocationsCTE(spec.scope, spec.expr)
	from, fromArgs := filteredFrom(spec, false)
	args = append(args, fromArgs...)
	var total int
	if err := db.QueryRowContext(ctx, cte+` select count(*)`+from, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("cannot count items: %w", err)
	}
	return total, nil
}

func (s *LibraryStore) countVideos(ctx context.Context, spec listSpec) (int, error) {
	return countVideosWith(ctx, s.db.sql, spec)
}

// Read counts and page rows from one snapshot so concurrent ingestion cannot make them disagree.
func (s *LibraryStore) listVideoPage(ctx context.Context, spec listSpec) (domain.VideoPage, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.VideoPage{}, fmt.Errorf("cannot start reading the list: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	page, err := listVideoPageTx(ctx, tx, spec)
	if err != nil {
		return domain.VideoPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.VideoPage{}, fmt.Errorf("cannot finish reading the list: %w", err)
	}
	return page, nil
}

// The caller owns the transaction, allowing tag existence and page data to share a snapshot.
func listVideoPageTx(ctx context.Context, tx *sql.Tx, spec listSpec) (domain.VideoPage, error) {
	limit := normalizeLimit(spec.limit)
	if !spec.sort.Valid() {
		// The HTTP boundary validates values; unknown internal enums fall back to defaults.
		spec.sort = domain.SortAddedDesc
	}
	order := listOrders[spec.sort]

	var cursorClause string
	var cursorArgs []any
	if spec.cursor != "" {
		var err error
		cursorClause, cursorArgs, err = order.cursorCondition(spec, spec.cursor)
		if err != nil {
			return domain.VideoPage{}, err
		}
	}

	total, err := countVideosWith(ctx, tx, spec)
	if err != nil {
		return domain.VideoPage{}, err
	}

	cte, args := chosenLocationsCTE(spec.scope, spec.expr)
	if order.seeded {
		args = append(args, spec.seed)
	}
	from, fromArgs := filteredFrom(spec, true)
	args = append(args, fromArgs...)
	query := cte + ` select * from (select ` + listColumns + `, ` + order.value + ` as sort_value` +
		from + `) as videos`
	if cursorClause != "" {
		query += ` where ` + cursorClause
		args = append(args, cursorArgs...)
	}
	// Keep ordering independent of search relevance; fetch one extra row to detect continuation without another count.
	query += ` order by ` + order.orderBy() + ` limit ?`

	rows, err := tx.QueryContext(ctx, query, append(args, limit+1)...)
	if err != nil {
		return domain.VideoPage{}, fmt.Errorf("cannot read the list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	page := domain.VideoPage{Total: total, Limit: limit, Items: []domain.Video{}}
	var values []any
	for rows.Next() {
		var value any
		video, err := scanVideo(sortValueScanner{rows: rows, value: &value})
		if err != nil {
			return domain.VideoPage{}, fmt.Errorf("cannot read the list: %w", err)
		}
		page.Items = append(page.Items, video)
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return domain.VideoPage{}, fmt.Errorf("cannot read the list: %w", err)
	}
	if err := rows.Close(); err != nil {
		return domain.VideoPage{}, fmt.Errorf("cannot close the list: %w", err)
	}

	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		last := page.Items[limit-1]
		page.NextCursor, err = order.encodeCursor(spec, values[limit-1], last.ID)
		if err != nil {
			return domain.VideoPage{}, err
		}
	}
	if err := attachVersions(ctx, tx, spec.scope.audience, page.Items); err != nil {
		return domain.VideoPage{}, err
	}
	return page, nil
}

type sortValueScanner struct {
	rows  *sql.Rows
	value *any
}

func (s sortValueScanner) Scan(dest ...any) error {
	return s.rows.Scan(append(dest, s.value)...)
}

const shuffleFunction = "vv_shuffle_key"

// Register once at package initialization: registration affects future connections and rejects duplicate names.
func init() {
	err := sqlite.RegisterDeterministicScalarFunction(shuffleFunction, 2,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			seed, ok := args[0].(int64)
			if !ok {
				return nil, fmt.Errorf("%s: seed is not an integer: %T", shuffleFunction, args[0])
			}
			id, ok := args[1].(int64)
			if !ok {
				return nil, fmt.Errorf("%s: id is not an integer: %T", shuffleFunction, args[1])
			}
			return domain.ShuffleKey(seed, id), nil
		})
	if err != nil {
		panic(fmt.Sprintf("cannot register %s: %v", shuffleFunction, err))
	}
}

type sortValueKind int

const (
	sortInteger sortValueKind = iota
	sortText
)

type listOrder struct {
	// Evaluate against inner video/location/progress rows and expose as sort_value outside.
	value string
	kind  sortValueKind
	desc  bool
	// NULL values sort last in either direction.
	nullable bool
	seeded   bool
}

var listOrders = map[domain.VideoSort]listOrder{
	domain.SortAddedAsc:     {value: `videos.added_at`},
	domain.SortAddedDesc:    {value: `videos.added_at`, desc: true},
	domain.SortModifiedAsc:  {value: `loc.mtime`},
	domain.SortModifiedDesc: {value: `loc.mtime`, desc: true},
	// Use location creation time, falling back to modification time.
	domain.SortCreatedAsc:    {value: fileCreatedAtExpr("loc")},
	domain.SortCreatedDesc:   {value: fileCreatedAtExpr("loc"), desc: true},
	domain.SortTitleAsc:      {value: `loc.title_key`, kind: sortText},
	domain.SortTitleDesc:     {value: `loc.title_key`, kind: sortText, desc: true},
	domain.SortDurationAsc:   {value: `videos.duration_ms`, nullable: true},
	domain.SortDurationDesc:  {value: `videos.duration_ms`, desc: true, nullable: true},
	domain.SortSizeAsc:       {value: `loc.size_bytes`},
	domain.SortSizeDesc:      {value: `loc.size_bytes`, desc: true},
	domain.SortPlayedAsc:     {value: `p.updated_at`, nullable: true},
	domain.SortPlayedDesc:    {value: `p.updated_at`, desc: true, nullable: true},
	domain.SortFavoritedAsc:  {value: `fav.favorited_at`, nullable: true},
	domain.SortFavoritedDesc: {value: `fav.favorited_at`, desc: true, nullable: true},
	// Shuffle order depends only on seed and ID, with ID breaking ties.
	domain.SortRandom: {value: shuffleFunction + `(?, videos.id)`, seeded: true},
}

func (o listOrder) orderBy() string {
	direction := ` asc`
	if o.desc {
		direction = ` desc`
	}
	clause := `sort_value` + direction + `, id` + direction
	if o.nullable {
		clause = `(sort_value is null) asc, ` + clause
	}
	return clause
}

// NULL values follow all non-NULL values; after a NULL cursor only later NULL IDs remain.
func (o listOrder) after(isNull bool, value any, id int64) (string, []any) {
	op := ` > `
	if o.desc {
		op = ` < `
	}
	if isNull {
		return `sort_value is null and id` + op + `?`, []any{id}
	}
	clause := `(sort_value, id)` + op + `(?, ?)`
	if o.nullable {
		clause = `(sort_value is null or ` + clause + `)`
	}
	return clause, []any{value, id}
}

// Reject cursors created for another order or seed.
func (o listOrder) cursorCondition(spec listSpec, cursor string) (string, []any, error) {
	c, err := decodeCursor(cursor)
	if err != nil {
		return "", nil, err
	}
	if c.sort != string(spec.sort) {
		return "", nil, fmt.Errorf("%w: cursor is for a different sort order", domain.ErrInvalidCursor)
	}
	if c.seed != o.seedText(spec) {
		return "", nil, fmt.Errorf("%w: cursor is for a different seed", domain.ErrInvalidCursor)
	}
	if c.isNull {
		if !o.nullable {
			return "", nil, fmt.Errorf("%w: cursor has no value", domain.ErrInvalidCursor)
		}
		clause, args := o.after(true, nil, c.id)
		return clause, args, nil
	}
	var value any = c.value
	if o.kind == sortInteger {
		parsed, err := strconv.ParseInt(c.value, 10, 64)
		if err != nil {
			return "", nil, fmt.Errorf("%w: cannot parse the sort value", domain.ErrInvalidCursor)
		}
		value = parsed
	}
	clause, args := o.after(false, value, c.id)
	return clause, args, nil
}

func (o listOrder) seedText(spec listSpec) string {
	if !o.seeded {
		return ""
	}
	return strconv.FormatInt(spec.seed, 10)
}

func (o listOrder) encodeCursor(spec listSpec, value any, id int64) (string, error) {
	nullFlag, text := "0", ""
	switch v := value.(type) {
	case nil:
		nullFlag = "1"
	case int64:
		text = strconv.FormatInt(v, 10)
	case string:
		text = v
	case []byte:
		text = string(v)
	default:
		return "", fmt.Errorf("cannot encode the sort value: %T", value)
	}
	return packCursor(string(spec.sort), o.seedText(spec), nullFlag, strconv.FormatInt(id, 10), text), nil
}

// Place fields that can contain separators last.
func packCursor(fields ...string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Join(fields, cursorSeparator)))
}

// Require exactly n fields; the final field may contain separators.
func unpackCursor(cursor string, n int) ([]string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidCursor, err)
	}
	parts := strings.SplitN(string(raw), cursorSeparator, n)
	if len(parts) != n {
		return nil, fmt.Errorf("%w: too few fields", domain.ErrInvalidCursor)
	}
	return parts, nil
}

func encodeCursorFields(c cursorFields) string {
	nullFlag := "0"
	if c.isNull {
		nullFlag = "1"
	}
	return packCursor(c.sort, c.seed, nullFlag, strconv.FormatInt(c.id, 10), c.value)
}

// The value is last so embedded separators cannot split it incorrectly.
const cursorSeparator = "\x1f"

type cursorFields struct {
	sort, seed string
	isNull     bool
	id         int64
	value      string
}

func decodeCursor(cursor string) (cursorFields, error) {
	parts, err := unpackCursor(cursor, 5)
	if err != nil {
		return cursorFields{}, err
	}
	if parts[2] != "0" && parts[2] != "1" {
		return cursorFields{}, fmt.Errorf("%w: cannot parse whether a value is present", domain.ErrInvalidCursor)
	}
	id, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return cursorFields{}, fmt.Errorf("%w: cannot parse the id", domain.ErrInvalidCursor)
	}
	return cursorFields{sort: parts[0], seed: parts[1], isNull: parts[2] == "1", id: id, value: parts[4]}, nil
}

func normalizeLimit(limit int) int {
	switch {
	case limit <= 0:
		return domain.DefaultLimit
	case limit > domain.MaxLimit:
		return domain.MaxLimit
	default:
		return limit
	}
}
