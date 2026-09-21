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
	t.Cleanup(func() { assertRepresentativeInvariant(t, db) })
	return db
}

func assertRepresentativeInvariant(t *testing.T, db *DB) {
	t.Helper()

	// down migration を検査するテストは、この時点で新しい表を落としている。
	// スキーマそのものはこの不変条件の対象ではない。問い合わせ自体の失敗は
	// 表の不在と区別する。区別しないと、検査が黙って空振りする。
	var present int
	if err := db.SQL().QueryRow(
		`select count(*) from sqlite_master where type = 'table' and name = 'video_locations'`,
	).Scan(&present); err != nil {
		t.Fatalf("代表場所の不変条件を検査できない（スキーマを確認できない）: %v", err)
	}
	if present == 0 {
		return
	}

	condition := registeredLocationCondition("l")
	//nolint:gosec // registeredLocationCondition は定型SQLだけを返す。
	rows, err := db.SQL().Query(`
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
