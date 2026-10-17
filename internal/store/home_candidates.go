package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// ShelfCandidates は規則の整理役の元を 1 つの読み取りの取引で数える（docs/design-docs/home.md、
// R-10）。数える動画はライブラリの一覧と同じ所有者に見せる動画で、unshelvedOnly なら
// domain.VideoQuery.Unshelved と同じくどの棚にも一致しない動画だけである。候補は仮でない
// （tags.tentative = 0 の）タグ・人物・団体のうち、domain.RuleShelfMinVideos 本以上の動画に当たるもので、
// 本数はそれぞれの棚の条件（{"tags": [id]}・{"person": id}・{"organization": id}）が当たる動画で数える。
// タグは手で付けた分とフォルダ名から付いている分のどちらでも当たる（videoHasTagCondition と同じ）。
func (s *HomeStore) ShelfCandidates(ctx context.Context, unshelvedOnly bool) (domain.ShelfCandidates, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.ShelfCandidates{}, fmt.Errorf("cannot start reading the shelf candidates: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	spec := libraryListSpec(domain.AudienceOwner, domain.VideoQuery{Unshelved: unshelvedOnly}, nil)
	if err := fillShelved(ctx, tx, &spec); err != nil {
		return domain.ShelfCandidates{}, err
	}
	cte, args := libraryItemsCTE(spec)
	// pool は数える動画と、その利用者データの鍵である。内容の識別子が空の動画はタグも作品情報も持たない。
	cte += `,
	pool as materialized (
		select videos.id as id, ` + userKeyExpr("videos") + ` as user_key
		from matched join videos on videos.id = matched.video_id
		where videos.content_key <> '')`
	var found domain.ShelfCandidates
	if err := tx.QueryRowContext(ctx, cte+` select count(*) from matched`, args...).Scan(&found.Videos); err != nil {
		return domain.ShelfCandidates{}, fmt.Errorf("cannot count the videos for the shelf candidates: %w", err)
	}
	found.Candidates, err = readShelfCandidates(ctx, tx, cte, args)
	if err != nil {
		return domain.ShelfCandidates{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.ShelfCandidates{}, fmt.Errorf("cannot finish reading the shelf candidates: %w", err)
	}
	return found, nil
}

// shelfCandidatesSelect は pool の動画から候補（kind, id, name, tag_kind_id, videos）を数える select 文で、
// 引数は 3 つの段ごとに本数の下限を 1 つ取る。
const shelfCandidatesSelect = `,
	tagged as (
		select vt.tag_id as tag_id, pool.id as video_id from pool join video_tags vt on vt.content_key = pool.user_key
		union
		select tn.tag_id, vfn.video_id from video_folder_names vfn
		join tag_names tn on tn.name = vfn.name
		where vfn.video_id in (select id from pool))
	select 'tag', t.id, tn.name, coalesce(t.kind_id, 0), count(*)
	from tagged join tags t on t.id = tagged.tag_id
	join tag_names tn on tn.tag_id = t.id and tn.canonical = 1
	where t.tentative = 0
	group by t.id having count(*) >= ?
	union all
	select 'person', vp.person_id, pn.name, 0, count(distinct pool.id)
	from pool join video_work_people vp on vp.content_key = pool.user_key
	join people_names pn on pn.person_id = vp.person_id and pn.canonical = 1
	group by vp.person_id having count(distinct pool.id) >= ?
	union all
	select 'organization', vo.organization_id, onm.name, 0, count(distinct pool.id)
	from pool join video_work_organizations vo on vo.content_key = pool.user_key
	join organization_names onm on onm.organization_id = vo.organization_id and onm.canonical = 1
	group by vo.organization_id having count(distinct pool.id) >= ?`

// readShelfCandidates は with 句 cte（pool を定める）とその引数 args で候補を読む。
func readShelfCandidates(ctx context.Context, tx *sql.Tx, cte string, args []any) ([]domain.ShelfCandidate, error) {
	minVideos := domain.RuleShelfMinVideos
	rows, err := tx.QueryContext(ctx, cte+shelfCandidatesSelect, append(args, minVideos, minVideos, minVideos)...)
	if err != nil {
		return nil, fmt.Errorf("cannot read the shelf candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var candidates []domain.ShelfCandidate
	for rows.Next() {
		var c domain.ShelfCandidate
		if err := rows.Scan(&c.Kind, &c.ID, &c.Name, &c.TagKindID, &c.Videos); err != nil {
			return nil, fmt.Errorf("cannot read the shelf candidates: %w", err)
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the shelf candidates: %w", err)
	}
	return candidates, nil
}
