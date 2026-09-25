package store

import (
	"context"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 公開フラグ（specs/016-single-account-auth/data-model.md §1・§3・§5）。
// 公開は content_key に結ぶ public_videos の行の有無で表し、見る人（domain.Audience）
// がゲストのときは、動画・所在・フォルダを返す LibraryStore の読み出しが
// visibleLocationCondition を通して公開の動画だけに絞る。

// publicVideoCondition は、所在（別名 alias）の動画の content_key が空でなく、
// public_videos にあることを表す条件句を返す。空の content_key はまだ内容を読めて
// いない動画で、public_videos に空の行があっても当てない。
func publicVideoCondition(alias string) string {
	return `exists (select 1 from videos pub_v join public_videos pub on pub.content_key = pub_v.content_key ` +
		`where pub_v.id = ` + alias + `.video_id and pub_v.content_key <> '')`
}

// visibleLocationCondition は、所在（別名 alias）を見る人に見せてよいかの条件句を
// 返す（data-model.md §3、plan.md Structural Decisions 11）。所有者では登録フォルダの
// 下にあることだけを、ゲストではそれに加えて動画が公開であることを求める。
// Audience のゼロ値はゲストなので、付け忘れは狭い側に倒れる。
//
// ゲストに見せる読み出しは、すべてこの関数（か、これを使う visibleVideoCondition）を
// 通す。取り込み・ジョブ・タグの本数が使う registeredLocationCondition と
// registeredVideoCondition は所有者のものとして残し、ゲストの条件を入れない。
func visibleLocationCondition(alias string, audience domain.Audience) string {
	registered := registeredLocationCondition(alias)
	if audience.IsOwner() {
		return registered
	}
	return registered + ` and ` + publicVideoCondition(alias)
}

// visibleVideoCondition は、動画（別名 alias）に見る人に見せてよい所在が1つでも
// あることを表す条件句を返す。
func visibleVideoCondition(alias string, audience domain.Audience) string {
	return `exists (select 1 from video_locations l where l.video_id = ` + alias + `.id and ` +
		visibleLocationCondition("l", audience) + `)`
}

// publicColumn は動画（videos）が公開かを 0/1 で返す列の式である。domain.Video.Public
// に写す。
const publicColumn = `exists (select 1 from public_videos pub where pub.content_key = videos.content_key and videos.content_key <> '')`

// SetVideosPublic は videoIDs のうちいまライブラリにある動画の公開フラグを
// public にそろえ、反映した動画の content_key を返す。その数が反映した本数である
// （data-model.md §5「公開・非公開を切り替える」）。呼び出し側は非公開にした
// content_key で、ゲストとして処理中の応答を打ち切る（contracts/guest-api.md §6）。
//
// id は content_key へ引き直し（タグの付け外しと同じ
// registeredContentKeysForVideoIDs、specs/014-video-tags/contracts/tags-api.md §4）、
// ライブラリに無い id と空の content_key の動画は数えない。既に同じ状態の動画も
// 反映した本数に入り、誤りにしない。全体を1つの取引で行い、途中で失敗したら
// 何も残さない。
func (s *VisibilityStore) SetVideosPublic(ctx context.Context, videoIDs []int64, public bool) ([]string, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("公開フラグを書き換えられません: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	keys, err := registeredContentKeysForVideoIDs(ctx, tx, videoIDs)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	for _, key := range keys {
		if public {
			_, err = tx.ExecContext(ctx,
				`insert or ignore into public_videos (content_key, published_at) values (?, ?)`, key, now)
		} else {
			_, err = tx.ExecContext(ctx, `delete from public_videos where content_key = ?`, key)
		}
		if err != nil {
			return nil, fmt.Errorf("公開フラグを書き換えられません: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("公開フラグを書き換えられません: %w", err)
	}
	return keys, nil
}
