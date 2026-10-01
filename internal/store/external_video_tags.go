package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 外部連携 API の名前でのタグの一括操作（specs/026-external-api/research.md R-7、
// contracts/external-api.md §4）。画面の付与（video_tags.go）と違い、引けない動画を
// 飛ばさずに要求全体を失敗させる。書き換えるのは手で付けたタグ（video_tags の行）だけで、
// 祖先のフォルダ名から付くタグは変えない。

// ApplyVideoTags は videos の各動画に、names のタグを action の通りに付ける・外す・置き換え、
// 各動画の操作後のタグを videos の順に返す（同じ動画を 2 度指せば 2 度返す）。
//
// tentative が真の add・replace は、どのタグの名前にもシノニムにも当たらない名前を仮のタグとして
// 作る。その名前が却下した名前なら作らず付けず、飛ばした名前として返す（replace の置き換え後の
// 集合にも入らない）。tentative が偽なら確定したタグとして作る。既存のタグに当たる名前は
// tentative を問わずそのタグを付け、その状態を変えない（specs/031-tentative-tags/data-model.md §3）。
//
// 全体を 1 つのトランザクションで行う。videos の指定のうち、登録フォルダの下に所在を持つ
// 今の動画へ引けないもの（内容を読めていない動画を含む）が 1 つでもあれば
// *domain.VideoRefNotFoundError（domain.ErrNotFound を包む）で失敗し、何も反映しない。
// names は domain.NormalizeTagName で整え、規則に合わないものは *domain.TagNameAtError。
// 同じタグに当たる名前は 1 つにまとめる。件数の上限は呼び出し側が確かめる。
func (s *TagStore) ApplyVideoTags(ctx context.Context, videos []domain.VideoRef, action domain.VideoTagsAction, names []string, tentative bool) (domain.VideoTagsOutcome, error) {
	if !action.Valid() {
		return domain.VideoTagsOutcome{}, fmt.Errorf("unknown video tags action %q", action)
	}
	normalized := make([]string, 0, len(names))
	for i, name := range names {
		n, err := domain.NormalizeTagName(name)
		if err != nil {
			return domain.VideoTagsOutcome{}, &domain.TagNameAtError{Index: i, Err: err}
		}
		normalized = append(normalized, n)
	}

	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.VideoTagsOutcome{}, err
	}
	defer func() { _ = tx.Rollback() }()

	targets, err := resolveTaggableVideos(ctx, tx, videos)
	if err != nil {
		return domain.VideoTagsOutcome{}, err
	}
	tagIDs, skipped, err := resolveTagNames(ctx, tx, action, normalized, tentative)
	if err != nil {
		return domain.VideoTagsOutcome{}, err
	}

	keys := uniqueUserKeys(targets)
	now := time.Now()
	changed, err := applyManualTags(ctx, tx, keys, action, tagIDs, now)
	if err != nil {
		return domain.VideoTagsOutcome{}, err
	}
	// 更新日時は手で付けたタグが実際に変わった鍵（集まりなら全メンバー）だけを進める
	// （specs/033-video-dates/research.md R-3）。
	if err := touchEditedAtForUserKeys(ctx, tx, changed, now); err != nil {
		return domain.VideoTagsOutcome{}, err
	}

	tags, err := tagsByContentKeys(ctx, tx, keys)
	if err != nil {
		return domain.VideoTagsOutcome{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.VideoTagsOutcome{}, fmt.Errorf("cannot update the video tags: %w", err)
	}

	out := make([]domain.VideoTagsResult, 0, len(targets))
	for _, target := range targets {
		result := domain.VideoTagsResult{VideoID: target.id, ContentKey: target.contentKey, Tags: []domain.VideoTag{}}
		if got := tags[target.userKey]; got != nil {
			result.Tags = got
		}
		out = append(out, result)
	}
	return domain.VideoTagsOutcome{Items: out, SkippedNames: skipped}, nil
}

// taggableVideo は一括操作の対象に引き当てた動画である。userKey は利用者データの鍵
// （specs/030-video-versions/data-model.md §3）で、タグはこの鍵に書き、この鍵で読む。
type taggableVideo struct {
	id         int64
	contentKey string
	userKey    string
}

// resolveTaggableVideos は videos の各指定を、登録フォルダの下に所在を持ち内容の識別子の
// ある動画へ引き当てる。引けない指定があれば、その位置を持つ *domain.VideoRefNotFoundError。
// 内容を読めていない動画（空の content_key）はタグを持てないので、引けない動画と同じに扱う。
func resolveTaggableVideos(ctx context.Context, tx *sql.Tx, videos []domain.VideoRef) ([]taggableVideo, error) {
	out := make([]taggableVideo, 0, len(videos))
	for i, ref := range videos {
		if !ref.Valid() {
			return nil, &domain.VideoRefNotFoundError{Index: i}
		}
		id, err := resolveVideoRef(ctx, tx, ref)
		if errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.VideoRefNotFoundError{Index: i}
		}
		if err != nil {
			return nil, err
		}
		var key, userKey string
		if err := tx.QueryRowContext(ctx, `select v.content_key, `+userKeyExpr("v")+` from videos v where v.id = ?`, id).
			Scan(&key, &userKey); err != nil {
			return nil, fmt.Errorf("cannot read the content key (video=%d): %w", id, err)
		}
		if key == "" {
			return nil, &domain.VideoRefNotFoundError{Index: i}
		}
		out = append(out, taggableVideo{id: id, contentKey: key, userKey: userKey})
	}
	return out, nil
}

// resolveTagNames は整えた名前をシノニムを含めてタグの id へ引く。add・replace は無い名前の
// タグを作り、remove はどのタグにも当たらない名前を飛ばす（tentative は読まない）。同じタグに
// 当たる名前は 1 つにまとめる。
//
// 作るタグは tentative が真なら仮のタグで、名前が却下した名前なら作らずに skipped へ入れる
// （names の順、重複なし）。偽なら確定したタグで、名前を却下した名前から外す
// （specs/031-tentative-tags/data-model.md §3）。判定は名前の引き当てと同じ取引で行う（R-6）。
func resolveTagNames(ctx context.Context, tx *sql.Tx, action domain.VideoTagsAction, names []string, tentative bool) ([]int64, []string, error) {
	seen := make(map[int64]bool, len(names))
	ids := make([]int64, 0, len(names))
	skipped := []string{}
	skippedSeen := make(map[string]bool)
	for _, name := range names {
		lookup, found, err := lookupTagName(ctx, tx, name)
		if err != nil {
			return nil, nil, err
		}
		var id int64
		switch {
		case found:
			id = lookup.tagID
		case action == domain.VideoTagsRemove:
			continue
		case tentative:
			rejected, err := isRejectedTagName(ctx, tx, name)
			if err != nil {
				return nil, nil, err
			}
			if rejected {
				if !skippedSeen[name] {
					skippedSeen[name] = true
					skipped = append(skipped, name)
				}
				continue
			}
			if id, err = insertTag(ctx, tx, name, true); err != nil {
				return nil, nil, err
			}
		default:
			if id, err = insertTag(ctx, tx, name, false); err != nil {
				return nil, nil, err
			}
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, skipped, nil
}

// isRejectedTagName は name が却下した名前かどうかを返す。
func isRejectedTagName(ctx context.Context, q rowQueryer, name string) (bool, error) {
	var n int
	if err := q.QueryRowContext(ctx, `select count(*) from rejected_tag_names where name = ?`, name).Scan(&n); err != nil {
		return false, fmt.Errorf("cannot read rejected tag names (%s): %w", name, err)
	}
	return n > 0, nil
}

// uniqueContentKeys は対象の content_key を重複なく最初に現れた順で返す（表示名の一括操作）。
func uniqueContentKeys(targets []taggableVideo) []string {
	seen := make(map[string]bool, len(targets))
	keys := make([]string, 0, len(targets))
	for _, target := range targets {
		if !seen[target.contentKey] {
			seen[target.contentKey] = true
			keys = append(keys, target.contentKey)
		}
	}
	return keys
}

// uniqueUserKeys は対象の利用者データの鍵を重複なく最初に現れた順で返す。同じ集まりの
// 動画は 1 つの鍵になる。
func uniqueUserKeys(targets []taggableVideo) []string {
	seen := make(map[string]bool, len(targets))
	keys := make([]string, 0, len(targets))
	for _, target := range targets {
		if !seen[target.userKey] {
			seen[target.userKey] = true
			keys = append(keys, target.userKey)
		}
	}
	return keys
}

// applyManualTags は利用者データの鍵 keys の手で付けたタグ（video_tags の行）を action の通りに
// 書き換え、行が実際に変わる鍵を重複なく keys の順で返す。
// 動画とタグの組を 1 行ずつ送らず、json_each で渡した集合に対する文の数を一定に保つ。上限の
// 20000 件 × 100 件でも文の数が増えず、書き込みの鍵を持つ時間を SQLite の中の処理だけに抑える。
// 変わる鍵は書き込みの前に 1 文で求め（changedManualTagKeys）、返る行を鍵の数までに抑える
// （specs/033-video-dates/research.md R-3）。書き込みは同じ取引で行うので、求めた鍵と実際に
// 変わる行は食い違わない。
func applyManualTags(ctx context.Context, tx *sql.Tx, keys []string, action domain.VideoTagsAction, tagIDs []int64, now time.Time) ([]string, error) {
	encodedKeys, err := json.Marshal(keys)
	if err != nil {
		return nil, fmt.Errorf("cannot build content keys: %w", err)
	}
	encodedTags, err := json.Marshal(tagIDs)
	if err != nil {
		return nil, fmt.Errorf("cannot build tag ids: %w", err)
	}
	changed, err := changedManualTagKeys(ctx, tx, string(encodedKeys), action, string(encodedTags))
	if err != nil {
		return nil, err
	}
	switch action {
	case domain.VideoTagsReplace:
		if _, err := tx.ExecContext(ctx,
			`delete from video_tags
			 where content_key in (select value from json_each(?))
			   and tag_id not in (select value from json_each(?))`,
			string(encodedKeys), string(encodedTags),
		); err != nil {
			return nil, fmt.Errorf("cannot replace the video tags: %w", err)
		}
		fallthrough
	case domain.VideoTagsAdd:
		if _, err := tx.ExecContext(ctx,
			`insert or ignore into video_tags (content_key, tag_id, created_at)
			 select k.value, t.value, ? from json_each(?) as k cross join json_each(?) as t`,
			now.Unix(), string(encodedKeys), string(encodedTags),
		); err != nil {
			return nil, fmt.Errorf("cannot add the video tags: %w", err)
		}
	case domain.VideoTagsRemove:
		if _, err := tx.ExecContext(ctx,
			`delete from video_tags
			 where content_key in (select value from json_each(?))
			   and tag_id in (select value from json_each(?))`,
			string(encodedKeys), string(encodedTags),
		); err != nil {
			return nil, fmt.Errorf("cannot remove the video tags: %w", err)
		}
	}
	return changed, nil
}

// changedManualTagKeys は、鍵の集合 encodedKeys（JSON の配列）のうち、action の書き込みで
// 手で付けたタグの行が変わる鍵を、書き込みの前に 1 文で返す（1 鍵につき高々 1 行）。
// add はタグの集合 encodedTags に付いていないタグがある鍵、remove は集合のタグが付いている鍵、
// replace はその両方に加えて集合に無いタグが付いている鍵である。
func changedManualTagKeys(ctx context.Context, tx *sql.Tx, encodedKeys string, action domain.VideoTagsAction, encodedTags string) ([]string, error) {
	const missing = `exists (select 1 from json_each(:tags) t where not exists
		(select 1 from video_tags vt where vt.content_key = k.value and vt.tag_id = t.value))`
	const present = `exists (select 1 from video_tags vt where vt.content_key = k.value
		and vt.tag_id in (select value from json_each(:tags)))`
	const extra = `exists (select 1 from video_tags vt where vt.content_key = k.value
		and vt.tag_id not in (select value from json_each(:tags)))`
	var condition string
	switch action {
	case domain.VideoTagsAdd:
		condition = missing
	case domain.VideoTagsRemove:
		condition = present
	case domain.VideoTagsReplace:
		condition = missing + ` or ` + extra
	default:
		return nil, fmt.Errorf("unknown video tags action %q", action)
	}
	rows, err := tx.QueryContext(ctx,
		`select k.value from json_each(:keys) k where `+condition+` order by k.key`,
		sql.Named("keys", encodedKeys), sql.Named("tags", encodedTags))
	if err != nil {
		return nil, fmt.Errorf("cannot read the changed video tags: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("cannot read the changed video tags: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the changed video tags: %w", err)
	}
	return keys, nil
}
