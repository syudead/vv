package store

// 所有者へのおすすめの棚の元（docs/design-docs/home.md）。種はよく再生した動画と新しい
// お気に入りで、どちらも所有者に見せる動画（集まりでは代表）に引き直す。並べるのは internal/app で、
// 近さの手がかりは関連動画と同じ RelatedCandidates（related.go）から読む。

import (
	"context"
	"database/sql"
	"fmt"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// recommendPlayedSQL は、期間の始まり（Unix ミリ秒、?1）からの視聴履歴を利用者データの鍵ごとに数え、
// 所有者に見せる動画の id を再生の多い順、同じなら最後の再生の新しい順に返す。視聴履歴の鍵は
// 再生したバージョンの content_key なので、集まりのメンバーは集まりの鍵に引き直してから数える。
var recommendPlayedSQL = `with played as (
		select coalesce((select b.user_key from video_bundle_members m join video_bundles b on b.id = m.bundle_id
		       where m.content_key = h.content_key), h.content_key) as user_key,
		       count(*) as plays, max(h.played_at) as last_played
		  from watch_history h where h.played_at >= ?1 group by 1),
	shown as (select ` + shownVideoForKeyExpr("played.user_key", domain.AudienceOwner) + ` as id, plays, last_played from played)
	select id from shown where id is not null order by plays desc, last_played desc, id desc limit ?2`

// recommendFavoritesSQL は、お気に入りの動画を所有者に見せる動画の id にして、お気に入りにした
// 新しい順に返す。
var recommendFavoritesSQL = `select id from (
		select ` + shownVideoForKeyExpr("f.content_key", domain.AudienceOwner) + ` as id, f.favorited_at
		  from video_favorites f)
	where id is not null order by favorited_at desc, id desc limit ?1`

// recommendWatchedSQL は見終わった動画の id を返す。
var recommendWatchedSQL = userKeyVideosSQL(`select content_key from playback_progress where completed = 1`)

// RecommendInputs はおすすめの種と見終わった動画を 1 つの読み取りのスナップショットから返す。
func (s *HomeStore) RecommendInputs(ctx context.Context) (domain.RecommendInputs, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.RecommendInputs{}, fmt.Errorf("cannot start reading the recommendation seeds: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	inputs := domain.RecommendInputs{}
	if inputs.Seeds, err = recommendSeeds(ctx, tx); err != nil {
		return domain.RecommendInputs{}, err
	}
	if inputs.Watched, err = queryInt64s(ctx, tx, recommendWatchedSQL); err != nil {
		return domain.RecommendInputs{}, fmt.Errorf("cannot read the watched videos: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.RecommendInputs{}, fmt.Errorf("cannot finish reading the recommendation seeds: %w", err)
	}
	return inputs, nil
}

// recommendSeeds は種を、よく再生した動画、新しいお気に入りの順に、同じ id は最初の 1 つだけにして
// 返す（domain.RecommendInputs の Seeds）。
func recommendSeeds(ctx context.Context, tx *sql.Tx) ([]int64, error) {
	since := listNow().Add(-domain.RecommendPlayWindow).UnixMilli()
	played, err := queryInt64s(ctx, tx, recommendPlayedSQL, since, domain.RecommendSeedPlays)
	if err != nil {
		return nil, fmt.Errorf("cannot read the most played videos: %w", err)
	}
	favorites, err := queryInt64s(ctx, tx, recommendFavoritesSQL, domain.RecommendSeedFavorites)
	if err != nil {
		return nil, fmt.Errorf("cannot read the latest favorite videos: %w", err)
	}
	seeds := slices.Clone(played)
	for _, id := range favorites {
		if !slices.Contains(seeds, id) {
			seeds = append(seeds, id)
		}
	}
	return seeds, nil
}
