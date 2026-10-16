package store

// スマートフォルダの中身の読み出し（specs/052-smart-folders/data-model.md「Contents」）。中身は
// 保存せず、読むたびにライブラリの項目の問い合わせ（libraryItemsCTE）を、フォルダと祖先の条件を
// 1 段ずつ重ねた絞り込み（listSpec.and）で求める（research.md R-2・R-3・R-5）。条件は所有者の
// 事実（タグ・再生の記録・お気に入り・検索の意味）で決め、見せる範囲は見る人のものにする（R-11）。

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// SmartFolderContents は見る人（audience）に見せるスマートフォルダ id と、その中身 1 ページを返す。
// cursor が空なら最初のページで、子フォルダを名前の順に先に置き、続けてライブラリの項目を limit 件
// 置く。cursor は前のページの NextCursor で、読めなければ domain.ErrInvalidCursor である。
//
// フォルダが無いときと、ゲストに見せる動画の無いフォルダは domain.ErrNotFound である（research.md
// R-8）。所有者は空のフォルダも読める。経路の条件が無い実体を名指すと、中身は子フォルダだけになる
// （R-9）。フォルダ・祖先・子フォルダ・中身・登録フォルダを同じ読み取りスナップショットから返す。
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

// smartFolderPageRequest は中身のページの求めである。cursor は前のページの NextCursor（最初の
// ページなら空）、limit は件数である。
type smartFolderPageRequest struct {
	cursor string
	limit  int
}

// smartFolderView はスマートフォルダ 1 つを読んだ結果である。
type smartFolderView struct {
	// card はフォルダのカード（再生順と値）である。
	card domain.LibrarySmartFolder
	// missing は経路の条件が名指す実体のうち今無いものである。
	missing []domain.SmartRuleRef
	// items と nextCursor は求めたページの項目と次のページのカーソルである。
	items      []domain.LibraryItem
	nextCursor string
}

// shown は、フォルダを見る人に見せるかを返す。所有者には常に、ゲストには再生順が空でないときだけ
// 見せる（research.md R-8）。
func (v smartFolderView) shown(audience domain.Audience) bool {
	return audience.IsOwner() || len(v.card.Members) > 0
}

// viewSmartFolder はフォルダ folder を、経路の条件 rules（外側の祖先から順、最後が folder の条件）
// で読む。now は追加日の絞り込みの基準時刻で、1 回の読み出しのどのフォルダにも同じ値を使う。
// request が nil でなければ、そのページの項目も読む。再生順とページは同じ 1 回の問い合わせから
// 求める（research.md R-13）。
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
		// 中身を求めなくても、読めないカーソルは domain.ErrInvalidCursor にする。
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

// smartFolderSpec は経路の条件 rules（外側の祖先から順）を、ライブラリの項目の問い合わせの絞り込みに
// する。最後の条件を本体に、祖先の条件を 1 段ずつ and に置く（research.md R-5）。どの段も見る人の
// 範囲で読み、検索式は所有者の意味で照合する（R-11）。条件の名指すタグはどれも有ると確かめてから
// 使う（無ければ中身を求めない）。
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

// smartFolderContents はフォルダの中身を 1 回読んだ結果である。
type smartFolderContents struct {
	// playOrder は再生順の動画の id である（data-model.md「Contents」の Play order）。
	playOrder []int64
	// keys と values は、カーソルより後ろの項目の鍵と並べ替えの値を並べ順に limit+1 件まで持つ。
	// order と limit はそのページの並べ順と件数である。
	keys   []itemRow
	values []any
	order  listOrder
	limit  int
}

// readSmartFolderContents は中身 spec の項目を並べ順に 1 回読み、再生順を求める。項目を並べ、
// グループの項目をそのグループの再生順に置き換え、同じ動画は最初に出たところだけに置く。withPage
// なら、spec のカーソルより後ろの項目の鍵も spec の件数より 1 件多くまで集める。
func readSmartFolderContents(ctx context.Context, tx *sql.Tx, spec listSpec, withPage bool) (smartFolderContents, error) {
	contents := smartFolderContents{order: itemOrder(spec.sort), limit: normalizeLimit(spec.limit)}
	cte, args := libraryItemsCTE(spec)
	if contents.order.seeded {
		args = append(args, spec.seed)
	}
	watchClause, watchArgs := itemWatchConditions(spec)
	where := ""
	if watchClause != "" {
		where = ` where ` + watchClause
	}
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
		// グループの項目はメンバーの数だけ続けて出る。項目の鍵は最初の行で 1 度だけ数える。
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

// playOrderMembers は再生順 ids の動画と、それぞれの再生の記録（無ければ nil、ゲストには全部 nil）を
// 同じ順に返す。動画の値は loadGroups のメンバーと同じく代表の所在から読む。
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

// smartFolderChildren はフォルダ parent の子フォルダを、名前の順（title_key、同じなら id）に項目に
// する。子の経路の条件は parentRules に子の条件を足したものである。ゲストには見せる動画の無い子を
// 出さない。
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
		rules := append(slices.Clone(parentRules), child.Rule)
		view, err := viewSmartFolder(ctx, tx, audience, child, rules, now, nil)
		if err != nil {
			return nil, err
		}
		if view.shown(audience) {
			items = append(items, domain.LibraryItem{SmartFolder: &view.card})
		}
	}
	return items, nil
}

// smartFolderTrail はフォルダ id の祖先を、外側から順に名前とともに返す。
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

// ownerSmartFolderCard はスマートフォルダ id のカードを、所有者として今の再生順で読む（プレイリストの
// スマートフォルダの項目、specs/052-smart-folders/contracts/screen-api.md）。now は追加日の絞り込みの
// 基準時刻である。
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

// smartFolderVideoGroup はスマートフォルダ id を、見る人（audience）に見せる再生順の動画の文脈
// （domain.VideoGroup の SmartFolder）にする（GET /api/videos/{id} などの smartFolder、
// contracts/screen-api.md「Schemas and parameters changed」）。フォルダが無ければ false である。
// 経路の条件が無い実体を名指すフォルダの再生順は空である（research.md R-9）。
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
