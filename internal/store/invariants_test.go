package store

import (
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// assertRepresentativeInvariant は、登録済みメディアフォルダ配下に場所を持つ
// すべての動画について、container と再生可否が代表場所から導かれる値と一致する
// ことを確かめる。代表場所は登録済みの場所をパス順に並べた先頭で、
// syncRepresentativeContainer の選び方と同じである。
//
// この不変条件は、代表場所が変わりうる経路ごとに syncRepresentativeContainer を
// 呼ぶことで保たれる。呼び忘れは PR #77 と #79 で4つの経路から1件ずつ見つかり、
// そのたびにレビュー1往復と修正専用PRを要した。呼び出し点を数え上げる代わりに、
// 結果の側を検査する。新しい経路が加わっても、この検査は追加の変更なしに効く。
//
// 登録済みの場所を1つも持たない動画は対象外である。syncRepresentativeContainer は
// 代表場所が無いとき何も書き換えないので、保存済みの値は最後に登録されていた
// ときのまま残る（videos.go の sql.ErrNoRows 分岐）。
// checkInvariants は db を返しつつ、テスト終了時の検査を登録する。
// ライブラリの行を作るテスト用フィクスチャはこれを通す。
//
// スキーマの遷移そのものを検査するテスト（migrate_test.go が個別に Open する
// もの）は対象外である。そこでは videos や video_locations が中間状態にあり、
// 派生属性の一致はまだ意味を持たない。
func checkInvariants(t *testing.T, db *DB) *DB {
	t.Helper()
	t.Cleanup(func() {
		assertRepresentativeInvariant(t, db)
		assertTagCanonicalNameInvariant(t, db)
		assertVideoOverrideInvariant(t, db)
		assertVideoEditInvariant(t, db)
		assertVideoBundleInvariant(t, db)
		assertVersionCandidateInvariant(t, db)
		assertTentativeTagInvariant(t, db)
		assertFavoriteInvariant(t, db)
	})
	return db
}

// assertTagCanonicalNameInvariant は、どの tags の行にも canonical = 1 の
// tag_names がちょうど1行あることを確かめる（specs/014-video-tags/data-model.md
// §1）。作成は必ず canonical = 1 の行を1つ添え、改名は canonical = 1 の行を
// 書き換えるだけで、統合は統合元の名前を canonical = 0 に落としてから統合先へ
// 付け替える。どの経路も、この不変条件を崩さずに保つ設計になっている
// （internal/store/tags.go）。
func assertTagCanonicalNameInvariant(t *testing.T, db *DB) {
	t.Helper()

	// down migration を検査するテストは、この時点で表を落としている。
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

// assertTentativeTagInvariant は仮のタグと却下した名前の不変条件を確かめる
// （specs/031-tentative-tags/data-model.md §1）。仮のタグはシノニムを持たず（シノニムを
// 足すと確定する。R-4）、却下した名前は tag_names に無い（名前を書く入口が同じ取引で
// 外す。R-3）。
func assertTentativeTagInvariant(t *testing.T, db *DB) {
	t.Helper()

	// down migration を検査するテストは、この時点で表を落としている。
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

// assertVideoOverrideInvariant は video_overrides の行の不変条件を確かめる
// （specs/029-video-overrides/data-model.md §1）。表示名と位置の両方が null の行は無く
// （書く側が消す）、改版番号は位置があるときだけ持ち、空の content_key の行は無い。
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

// assertVideoEditInvariant は video_edits の行の不変条件を確かめる
// （specs/033-video-dates/data-model.md §1）。空の content_key の行は無い（touchEditedAt が飛ばす）。
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

// assertVideoBundleInvariant は集まりの不変条件を確かめる（specs/030-video-versions/
// data-model.md §1）。代表はその集まりのメンバーで、メンバーが 2 本未満の集まりは無く
// （書く側が解く）、user_key は bundle: で始まり、メンバーの content_key は空でない。
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

// assertVersionCandidateInvariant は候補の不変条件を確かめる（specs/030-video-versions/
// data-model.md §1）。候補の組は「違う動画」と記録した組（video_version_dismissals）に無く、
// 同じ集まりの 2 本でもない。候補を作る取引が除き、束ねる操作・却下・スキャン時の引き継ぎが
// 消す。
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

	// down migration を検査するテストは、この時点で新しい表を落としている。
	// スキーマそのものはこの不変条件の対象ではない。問い合わせ自体の失敗は
	// 表の不在と区別する。区別しないと、検査が黙って空振りする。
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

// assertFavoriteInvariant はお気に入りの不変条件を確かめる（specs/035-favorites/data-model.md §1）。
// video_favorites に空の content_key の行は無く、folder_favorites.path は domain.FolderKey(path) と
// 等しい（整えた鍵しか書かない）。
func assertFavoriteInvariant(t *testing.T, db *DB) {
	t.Helper()

	var present int
	if err := db.sql.QueryRow(
		`select count(*) from sqlite_master where type = 'table' and name in ('video_favorites', 'folder_favorites')`,
	).Scan(&present); err != nil {
		t.Fatalf("お気に入りの不変条件を検査できない（スキーマを確認できない）: %v", err)
	}
	if present < 2 {
		return
	}

	var count int
	if err := db.sql.QueryRow(`select count(*) from video_favorites where content_key = ''`).Scan(&count); err != nil {
		t.Fatalf("お気に入りの不変条件を検査できない: %v", err)
	}
	if count != 0 {
		t.Errorf("video_favorites に空の content_key の行が %d 行ある", count)
	}

	rows, err := db.sql.Query(`select path from folder_favorites`)
	if err != nil {
		t.Fatalf("お気に入りの不変条件を検査できない: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			t.Fatalf("お気に入りの不変条件を検査できない: %v", err)
		}
		if key := domain.FolderKey(path); key != path {
			t.Errorf("folder_favorites の path %q が FolderKey %q と違う", path, key)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("お気に入りの不変条件を検査できない: %v", err)
	}
}
