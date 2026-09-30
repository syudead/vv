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
// 全体を 1 つのトランザクションで行う。videos の指定のうち、登録フォルダの下に所在を持つ
// 今の動画へ引けないもの（内容を読めていない動画を含む）が 1 つでもあれば
// *domain.VideoRefNotFoundError（domain.ErrNotFound を包む）で失敗し、何も反映しない。
// names は domain.NormalizeTagName で整え、規則に合わないものは *domain.TagNameAtError。
// 同じタグに当たる名前は 1 つにまとめる。件数の上限は呼び出し側が確かめる。
func (s *TagStore) ApplyVideoTags(ctx context.Context, videos []domain.VideoRef, action domain.VideoTagsAction, names []string) ([]domain.VideoTagsResult, error) {
	if !action.Valid() {
		return nil, fmt.Errorf("unknown video tags action %q", action)
	}
	normalized := make([]string, 0, len(names))
	for i, name := range names {
		n, err := domain.NormalizeTagName(name)
		if err != nil {
			return nil, &domain.TagNameAtError{Index: i, Err: err}
		}
		normalized = append(normalized, n)
	}

	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	targets, err := resolveTaggableVideos(ctx, tx, videos)
	if err != nil {
		return nil, err
	}
	tagIDs, err := resolveTagNames(ctx, tx, action, normalized)
	if err != nil {
		return nil, err
	}

	keys := uniqueUserKeys(targets)
	if err := applyManualTags(ctx, tx, keys, action, tagIDs); err != nil {
		return nil, err
	}

	tags, err := tagsByContentKeys(ctx, tx, keys)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cannot update the video tags: %w", err)
	}

	out := make([]domain.VideoTagsResult, 0, len(targets))
	for _, target := range targets {
		result := domain.VideoTagsResult{VideoID: target.id, ContentKey: target.contentKey, Tags: []domain.VideoTag{}}
		if got := tags[target.userKey]; got != nil {
			result.Tags = got
		}
		out = append(out, result)
	}
	return out, nil
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
// タグを作り（findOrCreateTag）、remove はどのタグにも当たらない名前を飛ばす。同じタグに
// 当たる名前は 1 つにまとめる。
func resolveTagNames(ctx context.Context, tx *sql.Tx, action domain.VideoTagsAction, names []string) ([]int64, error) {
	seen := make(map[int64]bool, len(names))
	ids := make([]int64, 0, len(names))
	for _, name := range names {
		var id int64
		if action == domain.VideoTagsRemove {
			lookup, found, err := lookupTagName(ctx, tx, name)
			if err != nil {
				return nil, err
			}
			if !found {
				continue
			}
			id = lookup.tagID
		} else {
			ref, _, err := findOrCreateTag(ctx, tx, name)
			if err != nil {
				return nil, err
			}
			id = ref.ID
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
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
// 書き換える。
// 動画とタグの組を 1 行ずつ送らず、json_each で渡した集合に対する 1〜2 文で済ませる。上限の
// 20000 件 × 100 件でも文の数が増えず、書き込みの鍵を持つ時間を SQLite の中の処理だけに抑える。
func applyManualTags(ctx context.Context, tx *sql.Tx, keys []string, action domain.VideoTagsAction, tagIDs []int64) error {
	encodedKeys, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("cannot build content keys: %w", err)
	}
	encodedTags, err := json.Marshal(tagIDs)
	if err != nil {
		return fmt.Errorf("cannot build tag ids: %w", err)
	}
	switch action {
	case domain.VideoTagsReplace:
		if _, err := tx.ExecContext(ctx,
			`delete from video_tags
			 where content_key in (select value from json_each(?))
			   and tag_id not in (select value from json_each(?))`,
			string(encodedKeys), string(encodedTags),
		); err != nil {
			return fmt.Errorf("cannot replace the video tags: %w", err)
		}
		fallthrough
	case domain.VideoTagsAdd:
		if _, err := tx.ExecContext(ctx,
			`insert or ignore into video_tags (content_key, tag_id, created_at)
			 select k.value, t.value, ? from json_each(?) as k cross join json_each(?) as t`,
			time.Now().Unix(), string(encodedKeys), string(encodedTags),
		); err != nil {
			return fmt.Errorf("cannot add the video tags: %w", err)
		}
	case domain.VideoTagsRemove:
		if _, err := tx.ExecContext(ctx,
			`delete from video_tags
			 where content_key in (select value from json_each(?))
			   and tag_id in (select value from json_each(?))`,
			string(encodedKeys), string(encodedTags),
		); err != nil {
			return fmt.Errorf("cannot remove the video tags: %w", err)
		}
	}
	return nil
}
