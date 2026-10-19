package store

import (
	"fmt"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// Check representative-derived attributes after every fixture operation, excluding intermediate migration schemas and videos without registered locations.
func checkInvariants(t *testing.T, db *DB) *DB {
	t.Helper()
	t.Cleanup(func() {
		assertRepresentativeInvariant(t, db)
		assertTagCanonicalNameInvariant(t, db)
		assertVideoOverrideInvariant(t, db)
		assertThumbnailImageInvariant(t, db)
		assertVideoEditInvariant(t, db)
		assertVideoBundleInvariant(t, db)
		assertVersionCandidateInvariant(t, db)
		assertTentativeTagInvariant(t, db)
		assertTagSortKeyInvariant(t, db)
		assertFavoriteInvariant(t, db)
		assertPlaylistInvariant(t, db)
		assertGroupInvariant(t, db)
		assertSmartFolderInvariant(t, db)
		assertCreditInvariant(t, db)
		assertWorkInfoInvariant(t, db)
	})
	return db
}

// Every tag must retain exactly one canonical name across creation, renaming and merging.
func assertTagCanonicalNameInvariant(t *testing.T, db *DB) {
	t.Helper()

	// Down-migration tests may already have dropped this table.
	var present int
	if err := db.sql.QueryRow(
		`select count(*) from sqlite_master where type = 'table' and name = 'tags'`,
	).Scan(&present); err != nil {
		t.Fatalf("タグの不変条件を検査できない（スキーマを確認できない）: %v", err)
	}
	if present == 0 {
		return
	}

	rows, err := db.sql.Query(`
		select t.id, (select count(*) from tag_names tn where tn.tag_id = t.id and tn.canonical = 1)
		  from tags t`)
	if err != nil {
		t.Fatalf("タグの不変条件を検査できない: %v", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id int64
		var canonicalCount int
		if err := rows.Scan(&id, &canonicalCount); err != nil {
			t.Fatalf("タグの不変条件を検査できない: %v", err)
		}
		if canonicalCount != 1 {
			t.Errorf("タグ %d の canonical = 1 の tag_names が %d 行ある。ちょうど1行のはず。", id, canonicalCount)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("タグの不変条件を検査できない: %v", err)
	}
}

// Tentative tags have no synonyms, and rejected names cannot also exist in tag_names.
func assertTentativeTagInvariant(t *testing.T, db *DB) {
	t.Helper()

	// Down-migration tests may already have dropped this table.
	var present int
	if err := db.sql.QueryRow(
		`select count(*) from sqlite_master where type = 'table' and name = 'rejected_tag_names'`,
	).Scan(&present); err != nil {
		t.Fatalf("仮のタグの不変条件を検査できない（スキーマを確認できない）: %v", err)
	}
	if present == 0 {
		return
	}

	for name, query := range map[string]string{
		"シノニムを持つ仮のタグ": `select count(*) from tags t
			 where t.tentative = 1
			   and exists (select 1 from tag_names tn where tn.tag_id = t.id and tn.canonical = 0)`,
		"tag_names にもある却下した名前": `select count(*) from rejected_tag_names r
			 join tag_names tn on tn.name = r.name`,
	} {
		var count int
		if err := db.sql.QueryRow(query).Scan(&count); err != nil {
			t.Fatalf("仮のタグの不変条件を検査できない: %v", err)
		}
		if count != 0 {
			t.Errorf("%sが %d 件ある", name, count)
		}
	}
}

// Only current-version rows must match current natural-sort rules; startup repairs older versions.
func assertTagSortKeyInvariant(t *testing.T, db *DB) {
	t.Helper()

	// Down-migration tests may already have dropped these columns or tables.
	var present int
	if err := db.sql.QueryRow(
		`select count(*) from pragma_table_info('rejected_tag_names') where name in ('sort_key', 'search_version')`,
	).Scan(&present); err != nil {
		t.Fatalf("名前の自然順の鍵の不変条件を検査できない（スキーマを確認できない）: %v", err)
	}
	if present < 2 {
		return
	}

	for _, table := range []string{"tag_names", "rejected_tag_names"} {
		assertTableSortKeys(t, db, table)
	}
}

func assertTableSortKeys(t *testing.T, db *DB, table string) {
	t.Helper()
	rows, err := db.sql.Query(`select name, sort_key from `+table+` where search_version = ?`, domain.SearchKeyVersion)
	if err != nil {
		t.Fatalf("名前の自然順の鍵の不変条件を検査できない: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name, key string
		if err := rows.Scan(&name, &key); err != nil {
			t.Fatalf("名前の自然順の鍵の不変条件を検査できない: %v", err)
		}
		if want := domain.NaturalSortKey(name); key != want {
			t.Errorf("%s の %q の sort_key = %q, want %q", table, name, key, want)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("名前の自然順の鍵の不変条件を検査できない: %v", err)
	}
}

// Overrides cannot have empty keys or both values absent; revisions exist exactly when positions do.
func assertVideoOverrideInvariant(t *testing.T, db *DB) {
	t.Helper()

	var present int
	if err := db.sql.QueryRow(
		`select count(*) from sqlite_master where type = 'table' and name = 'video_overrides'`,
	).Scan(&present); err != nil {
		t.Fatalf("上書きの不変条件を検査できない（スキーマを確認できない）: %v", err)
	}
	if present == 0 {
		return
	}

	for name, condition := range map[string]string{
		"表示名と位置の両方が null の行": `display_name is null and thumbnail_position_ms is null`,
		"位置が無いのに改版番号を持つ行":    `thumbnail_position_ms is null and thumbnail_revision is not null`,
		"位置があるのに改版番号を持たない行":  `thumbnail_position_ms is not null and thumbnail_revision is null`,
		"空の content_key の行":  `content_key = ''`,
		"空文字の表示名の行":          `display_name = ''`,
	} {
		var count int
		if err := db.sql.QueryRow(`select count(*) from video_overrides where ` + condition).Scan(&count); err != nil {
			t.Fatalf("上書きの不変条件を検査できない: %v", err)
		}
		if count != 0 {
			t.Errorf("video_overrides に%sが %d 行ある", name, count)
		}
	}
}

// Applied thumbnails require nonempty keys/images and a valid origin; a source may be NULL but never empty.
func assertThumbnailImageInvariant(t *testing.T, db *DB) {
	t.Helper()
	violations, err := thumbnailImageViolations(db)
	if err != nil {
		t.Fatalf("適用した画像の不変条件を検査できない: %v", err)
	}
	for _, violation := range violations {
		t.Errorf("thumbnail_images に%s", violation)
	}
}

// Skip constraints whose columns are absent during down-migration tests.
func thumbnailImageViolations(db *DB) ([]string, error) {
	columns, err := thumbnailImageColumns(db)
	if err != nil {
		return nil, err
	}

	var violations []string
	for _, check := range []struct{ name, column, condition string }{
		{"空の content_key の行", "content_key", `content_key = ''`},
		{"空の画像の行", "image", `length(image) = 0`},
		{"空の元の画像の行", "source", `source is not null and length(source) = 0`},
		{"出どころが未知の行", "origin", `origin not in ('generated', 'uploaded')`},
	} {
		if !columns[check.column] {
			continue
		}
		var count int
		if err := db.sql.QueryRow(`select count(*) from thumbnail_images where ` + check.condition).Scan(&count); err != nil {
			return nil, err
		}
		if count != 0 {
			violations = append(violations, fmt.Sprintf("%sが %d 行ある", check.name, count))
		}
	}
	return violations, nil
}

func thumbnailImageColumns(db *DB) (map[string]bool, error) {
	rows, err := db.sql.Query(`select name from pragma_table_info('thumbnail_images')`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	columns := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

func assertVideoEditInvariant(t *testing.T, db *DB) {
	t.Helper()

	var present int
	if err := db.sql.QueryRow(
		`select count(*) from sqlite_master where type = 'table' and name = 'video_edits'`,
	).Scan(&present); err != nil {
		t.Fatalf("更新日時の不変条件を検査できない（スキーマを確認できない）: %v", err)
	}
	if present == 0 {
		return
	}

	var count int
	if err := db.sql.QueryRow(`select count(*) from video_edits where content_key = ''`).Scan(&count); err != nil {
		t.Fatalf("更新日時の不変条件を検査できない: %v", err)
	}
	if count != 0 {
		t.Errorf("video_edits に空の content_key の行が %d 行ある", count)
	}
}

// Bundles require at least two identified members, a member representative and a bundle-prefixed user key.
func assertVideoBundleInvariant(t *testing.T, db *DB) {
	t.Helper()

	var present int
	if err := db.sql.QueryRow(
		`select count(*) from sqlite_master where type = 'table' and name = 'video_bundles'`,
	).Scan(&present); err != nil {
		t.Fatalf("集まりの不変条件を検査できない（スキーマを確認できない）: %v", err)
	}
	if present == 0 {
		return
	}

	for name, query := range map[string]string{
		"代表がメンバーでない集まり": `select count(*) from video_bundles b where not exists (
			select 1 from video_bundle_members m where m.bundle_id = b.id and m.content_key = b.representative_key)`,
		"メンバーが 2 本未満の集まり": `select count(*) from video_bundles b
			where (select count(*) from video_bundle_members m where m.bundle_id = b.id) < 2`,
		"bundle: で始まらない user_key": `select count(*) from video_bundles where user_key not like 'bundle:%'`,
		"空の content_key のメンバー":    `select count(*) from video_bundle_members where content_key = ''`,
	} {
		var count int
		if err := db.sql.QueryRow(query).Scan(&count); err != nil {
			t.Fatalf("集まりの不変条件を検査できない: %v", err)
		}
		if count != 0 {
			t.Errorf("%sが %d 件ある", name, count)
		}
	}
}

// Candidates cannot contain dismissed pairs or members of the same bundle.
func assertVersionCandidateInvariant(t *testing.T, db *DB) {
	t.Helper()

	var present int
	if err := db.sql.QueryRow(
		`select count(*) from sqlite_master where type = 'table' and name = 'video_version_candidates'`,
	).Scan(&present); err != nil {
		t.Fatalf("候補の不変条件を検査できない（スキーマを確認できない）: %v", err)
	}
	if present == 0 {
		return
	}

	for name, query := range map[string]string{
		"却下した組の候補": `select count(*) from video_version_candidates c where exists (
			select 1 from video_version_dismissals d where d.key_a = c.key_a and d.key_b = c.key_b)`,
		"同じ集まりの 2 本の候補": `select count(*) from video_version_candidates c where exists (
			select 1 from video_bundle_members ma join video_bundle_members mb on mb.bundle_id = ma.bundle_id
			 where ma.content_key = c.key_a and mb.content_key = c.key_b)`,
	} {
		var count int
		if err := db.sql.QueryRow(query).Scan(&count); err != nil {
			t.Fatalf("候補の不変条件を検査できない: %v", err)
		}
		if count != 0 {
			t.Errorf("%sが %d 件ある", name, count)
		}
	}
}

func assertRepresentativeInvariant(t *testing.T, db *DB) {
	t.Helper()

	// Distinguish absent migration tables from query failures so invariant checks cannot silently skip errors.
	var present int
	if err := db.sql.QueryRow(
		`select count(*) from sqlite_master where type = 'table' and name = 'video_locations'`,
	).Scan(&present); err != nil {
		t.Fatalf("代表場所の不変条件を検査できない（スキーマを確認できない）: %v", err)
	}
	if present == 0 {
		return
	}

	condition := registeredLocationCondition("l")
	rows, err := db.sql.Query(`
		select v.id, coalesce(v.container, ''), v.playable, coalesce(v.unplayable_reason, ''),
		       v.probe_state, coalesce(v.video_codec, ''), coalesce(v.audio_codec, ''),
		       (select l.path from video_locations l
		         where l.video_id = v.id and ` + condition + ` order by l.path limit 1)
		  from videos v
		 where exists (select 1 from video_locations l
		                where l.video_id = v.id and ` + condition + `)`)
	if err != nil {
		t.Fatalf("代表場所の不変条件を検査できない: %v", err)
	}
	defer func() { _ = rows.Close() }()

	type violation struct {
		id                          int64
		path                        string
		wantContainer, gotContainer string
		wantPlayable, gotPlayable   int
		wantReason, gotReason       string
	}
	var violations []violation

	for rows.Next() {
		var id int64
		var gotContainer, gotReason, probeState, videoCodec, audioCodec, path string
		var gotPlayable int
		if err := rows.Scan(&id, &gotContainer, &gotPlayable, &gotReason,
			&probeState, &videoCodec, &audioCodec, &path); err != nil {
			t.Fatalf("代表場所の不変条件を検査できない: %v", err)
		}

		wantContainer := domain.ContainerFromPath(path)
		wantPlayable := 0
		wantReason := ""
		if probeState == string(domain.ProbeStateDone) {
			play := domain.EvaluatePlayability(wantContainer,
				domain.Probe{VideoCodec: videoCodec, AudioCodec: audioCodec})
			wantPlayable = boolToInt(play.Playable)
			wantReason = string(play.Reason)
		}

		if gotContainer != wantContainer || gotPlayable != wantPlayable || gotReason != wantReason {
			violations = append(violations, violation{
				id: id, path: path,
				wantContainer: wantContainer, gotContainer: gotContainer,
				wantPlayable: wantPlayable, gotPlayable: gotPlayable,
				wantReason: wantReason, gotReason: gotReason,
			})
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("代表場所の不変条件を検査できない: %v", err)
	}

	for _, v := range violations {
		t.Errorf("動画 %d の派生属性が代表場所 %s と一致しない:\n"+
			"  container:         得 %q / 期待 %q\n"+
			"  playable:          得 %d / 期待 %d\n"+
			"  unplayable_reason: 得 %q / 期待 %q\n"+
			"  代表場所が変わる経路で syncRepresentativeContainer を呼んでいない可能性がある。",
			v.id, v.path,
			v.gotContainer, v.wantContainer,
			v.gotPlayable, v.wantPlayable,
			v.gotReason, v.wantReason)
	}
}

func assertFavoriteInvariant(t *testing.T, db *DB) {
	t.Helper()

	var present int
	if err := db.sql.QueryRow(
		`select count(*) from sqlite_master where type = 'table' and name = 'video_favorites'`,
	).Scan(&present); err != nil {
		t.Fatalf("お気に入りの不変条件を検査できない（スキーマを確認できない）: %v", err)
	}
	if present == 0 {
		return
	}

	var count int
	if err := db.sql.QueryRow(`select count(*) from video_favorites where content_key = ''`).Scan(&count); err != nil {
		t.Fatalf("お気に入りの不変条件を検査できない: %v", err)
	}
	if count != 0 {
		t.Errorf("video_favorites に空の content_key の行が %d 行ある", count)
	}
}

// Playlist positions remain contiguous from zero; foreign keys protect group references.
func assertPlaylistInvariant(t *testing.T, db *DB) {
	t.Helper()

	var present int
	if err := db.sql.QueryRow(
		`select count(*) from sqlite_master where type = 'table' and name = 'playlist_items'`,
	).Scan(&present); err != nil {
		t.Fatalf("プレイリストの不変条件を検査できない（スキーマを確認できない）: %v", err)
	}
	if present == 0 {
		return
	}

	rows, err := db.sql.Query(`select playlist_id, count(*), min(position), max(position), count(distinct position)
		from playlist_items group by playlist_id`)
	if err != nil {
		t.Fatalf("プレイリストの不変条件を検査できない: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, count, low, high, distinct int64
		if err := rows.Scan(&id, &count, &low, &high, &distinct); err != nil {
			t.Fatalf("プレイリストの不変条件を検査できない: %v", err)
		}
		if low != 0 || high != count-1 || distinct != count {
			t.Errorf("プレイリスト %d の位置が 0 … %d に詰まっていない（min %d、max %d、異なる位置 %d）",
				id, count-1, low, high, distinct)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("プレイリストの不変条件を検査できない: %v", err)
	}
}

// Groups require contiguous positions, normalized automatic folder keys and acyclic nesting.
func assertGroupInvariant(t *testing.T, db *DB) {
	t.Helper()

	var present int
	if err := db.sql.QueryRow(
		`select count(*) from sqlite_master where type = 'table' and name = 'group_items'`,
	).Scan(&present); err != nil {
		t.Fatalf("グループの不変条件を検査できない（スキーマを確認できない）: %v", err)
	}
	if present == 0 {
		return
	}
	assertGroupPositions(t, db)
	assertGroupOriginFolders(t, db)
	assertGroupAcyclic(t, db)
}

func assertGroupPositions(t *testing.T, db *DB) {
	t.Helper()
	rows, err := db.sql.Query(`select group_id, count(*), min(position), max(position), count(distinct position)
		from group_items group by group_id`)
	if err != nil {
		t.Fatalf("グループの不変条件を検査できない: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, count, low, high, distinct int64
		if err := rows.Scan(&id, &count, &low, &high, &distinct); err != nil {
			t.Fatalf("グループの不変条件を検査できない: %v", err)
		}
		if low != 0 || high != count-1 || distinct != count {
			t.Errorf("グループ %d の位置が 0 … %d に詰まっていない（min %d、max %d、異なる位置 %d）",
				id, count-1, low, high, distinct)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("グループの不変条件を検査できない: %v", err)
	}
}

func assertGroupOriginFolders(t *testing.T, db *DB) {
	t.Helper()
	rows, err := db.sql.Query(`select origin_folder from groups where origin_folder is not null`)
	if err != nil {
		t.Fatalf("グループの不変条件を検査できない: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatalf("グループの不変条件を検査できない: %v", err)
		}
		if folded := domain.FolderKey(key); folded != key {
			t.Errorf("groups の origin_folder %q が FolderKey %q と違う", key, folded)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("グループの不変条件を検査できない: %v", err)
	}
}

func assertGroupAcyclic(t *testing.T, db *DB) {
	t.Helper()
	rows, err := db.sql.Query(`with recursive down(root, id) as (
			select group_id, child_id from group_items where child_id is not null
			union
			select down.root, i.child_id from down join group_items i on i.group_id = down.id where i.child_id is not null)
		select distinct root from down where root = id`)
	if err != nil {
		t.Fatalf("グループの不変条件を検査できない: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("グループの不変条件を検査できない: %v", err)
		}
		t.Errorf("グループ %d が自分の子孫になっている", id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("グループの不変条件を検査できない: %v", err)
	}
}

// Smart-folder rules and sorts must parse, and parents must remain acyclic.
func assertSmartFolderInvariant(t *testing.T, db *DB) {
	t.Helper()

	var present int
	if err := db.sql.QueryRow(
		`select count(*) from sqlite_master where type = 'table' and name = 'smart_folders'`,
	).Scan(&present); err != nil {
		t.Fatalf("スマートフォルダの不変条件を検査できない（スキーマを確認できない）: %v", err)
	}
	if present == 0 {
		return
	}
	rows, err := db.sql.Query(`select id, rule, sort from smart_folders`)
	if err != nil {
		t.Fatalf("スマートフォルダの不変条件を検査できない: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var rule, sort string
		if err := rows.Scan(&id, &rule, &sort); err != nil {
			t.Fatalf("スマートフォルダの不変条件を検査できない: %v", err)
		}
		if _, err := domain.ParseSmartRule([]byte(rule)); err != nil {
			t.Errorf("スマートフォルダ %d の rule %s が読めない: %v", id, rule, err)
		}
		if err := domain.ValidateSmartSort(domain.VideoSort(sort)); err != nil {
			t.Errorf("スマートフォルダ %d の sort %q が規則に合わない: %v", id, sort, err)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("スマートフォルダの不変条件を検査できない: %v", err)
	}
	_ = rows.Close()
	for _, violation := range violations(t, db, `
		with recursive up (start, id, depth) as (
			select id, parent_id, 1 from smart_folders where parent_id is not null
			union all
			select up.start, f.parent_id, up.depth + 1 from up join smart_folders f on f.id = up.id
			 where f.parent_id is not null and up.depth <= (select count(*) from smart_folders)
		)
		select distinct 'スマートフォルダ ' || start || ' が自分の祖先になっている' from up where id = start`) {
		t.Error(violation)
	}
}

// Check canonical names, acyclic parents, limits, external-ID uniqueness, contiguous link positions and current name keys.
func assertCreditInvariant(t *testing.T, db *DB) {
	t.Helper()

	var present int
	if err := db.sql.QueryRow(
		`select count(*) from sqlite_master where type = 'table' and name = 'people'`,
	).Scan(&present); err != nil {
		t.Fatalf("人物・団体の不変条件を検査できない（スキーマを確認できない）: %v", err)
	}
	if present == 0 {
		return
	}

	var checks []string
	for _, tables := range creditTablesByKind {
		// A provisional person has no names at all; every other credit has exactly one canonical name.
		minCanonical := `1`
		if tables.kind == domain.CreditPerson {
			minCanonical = `(case when count(n.name) = 0 and not exists (select 1 from {names} a where a.{id} = e.id) then 0 else 1 end)`
		}
		checks = append(checks, tables.q(`
			select '{entity} ' || e.id || ' の正規の名前の行が ' || count(n.name) || ' 行'
			  from {entity} e left join {names} n on n.{id} = e.id and n.canonical = 1
			 group by e.id having count(n.name) not between `+minCanonical+` and 1`),
			tables.q(`select '{names} の正規の行 ' || {id} || ' に language がある' from {names}
			 where canonical = 1 and language is not null`),
			tables.q(`select '{entity} ' || {id} || ' の別名が ' || count(*) || ' 件' from {names}
			 where canonical = 0 group by {id} having count(*) > 50`),
			tables.q(`select '{entity} ' || {id} || ' の外部 ID が ' || count(*) || ' 件' from {ext}
			 group by {id} having count(*) > 20`),
			tables.q(`select '{ext} の ' || source || ':' || value || ' を ' || count(*) || ' 実体が持つ' from {ext}
			 group by source, value having count(*) > 1`),
			tables.q(`select '{links} の ' || content_key || ' ' || role || ' の位置が詰まっていない' from {links}
			 group by content_key, role
			having min(position) <> 0 or max(position) <> count(*) - 1 or count(distinct position) <> count(*)`))
	}
	checks = append(checks, `
		with recursive chain (start, id, depth) as (
			select id, parent_id, 1 from organizations where parent_id is not null
			union all
			select c.start, o.parent_id, c.depth + 1 from chain c join organizations o on o.id = c.id
			 where o.parent_id is not null and c.depth <= (select count(*) from organizations)
		)
		select distinct '団体 ' || start || ' の親の連鎖が輪になる' from chain where id = start`,
		`select 'video_work_urls の ' || content_key || ' の位置が詰まっていない' from video_work_urls
		  group by content_key
		 having min(position) <> 0 or max(position) <> count(*) - 1 or count(distinct position) <> count(*)`)
	for _, check := range checks {
		for _, violation := range violations(t, db, check) {
			t.Error(violation)
		}
	}

	for _, tables := range creditTablesByKind {
		assertCreditNameKeys(t, db, tables)
	}
}

// Scalar work-info rows need code or description; current code keys must match normalization.
func assertWorkInfoInvariant(t *testing.T, db *DB) {
	t.Helper()

	var present int
	if err := db.sql.QueryRow(
		`select count(*) from sqlite_master where type = 'table' and name = 'video_work_info'`,
	).Scan(&present); err != nil {
		t.Fatalf("作品情報の不変条件を検査できない（スキーマを確認できない）: %v", err)
	}
	if present == 0 {
		return
	}
	for _, violation := range violations(t, db, `select 'video_work_info の ' || content_key || ' が品番も説明も持たない'
		from video_work_info where coalesce(code, '') = '' and coalesce(description, '') = ''`) {
		t.Error(violation)
	}
	rows, err := db.sql.Query(`select content_key, coalesce(code, ''), code_key from video_work_info where search_version = ?`,
		domain.SearchKeyVersion)
	if err != nil {
		t.Fatalf("作品情報の不変条件を検査できない: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var key, code, codeKey string
		if err := rows.Scan(&key, &code, &codeKey); err != nil {
			t.Fatalf("作品情報の不変条件を検査できない: %v", err)
		}
		if codeKey != domain.FoldForMatch(code) {
			t.Errorf("video_work_info の %s の code_key %q が品番 %q の畳み込みと違う", key, codeKey, code)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("作品情報の不変条件を検査できない: %v", err)
	}
}

func assertCreditNameKeys(t *testing.T, db *DB, tables creditTables) {
	t.Helper()
	rows, err := db.sql.Query(tables.q(`select name, search_key, sort_key from {names} where search_version = ?`),
		domain.SearchKeyVersion)
	if err != nil {
		t.Fatalf("人物・団体の不変条件を検査できない: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name, searchKey, sortKey string
		if err := rows.Scan(&name, &searchKey, &sortKey); err != nil {
			t.Fatalf("人物・団体の不変条件を検査できない: %v", err)
		}
		if searchKey != domain.FoldForMatch(name) || sortKey != domain.NaturalSortKey(name) {
			t.Errorf("%s の %q の鍵が今の版の畳み込みと違う", tables.names, name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("人物・団体の不変条件を検査できない: %v", err)
	}
}

func violations(t *testing.T, db *DB, query string) []string {
	t.Helper()
	rows, err := db.sql.Query(query)
	if err != nil {
		t.Fatalf("問い合わせに失敗した: %v\n%s", err, query)
	}
	defer func() { _ = rows.Close() }()
	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatalf("問い合わせの行を読めない: %v", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("問い合わせの行を読めない: %v", err)
	}
	return values
}
