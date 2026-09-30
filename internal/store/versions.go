package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 同じ動画の別バージョンの集まり（specs/030-video-versions/data-model.md §1・§3・§8、
// research.md R-1）。集まりは利用者データの表 video_bundles・video_bundle_members に置き、
// 集まりのタグ・再生位置・公開の設定は video_tags・playback_progress・public_videos に
// 集まりの user_key を鍵として置く。各メンバーの content_key の行は触らずに残すので、
// 外したメンバーは束ねる前の値に戻る。
//
// 束ねる・代表を替える・外す操作は、見せる動画（data-model.md §4）が変わり、代表の所在の
// フォルダの直下の本数が変わるので、同じ取引の中でフォルダの索引を作り直す（research.md R-4）。

// userDataTables は利用者データの鍵で引く表と、鍵以外の列である。束ねる・解くときに
// 値を別の鍵へ写すのに使う。
var userDataTables = []struct{ table, columns string }{
	{"playback_progress", "position_ms, duration_ms, completed, updated_at"},
	{"video_tags", "tag_id, created_at"},
	{"public_videos", "published_at"},
}

// bundleRef は動画が属する集まりである。
type bundleRef struct {
	id                int64
	userKey           string
	representativeKey string
}

// Bundle は videoIDs の動画を 1 つの集まりに束ね、representativeID を代表にする
// （data-model.md §8）。id はいまライブラリにある動画の content_key へ引き直し、引けない id が
// あれば domain.ErrNotFound。重複を除いて 2 本未満なら domain.ErrTooFewVersions、代表が
// videoIDs に無ければ domain.ErrRepresentativeNotSelected。
//
// 既に集まりに属する動画は、その集まりの全メンバーごと新しい集まりへ移す。集まりの値は
// 代表の利用者データの鍵（代表が集まりに属していればその集まりの鍵）の行を新しい鍵へ写す。
// メンバーを移し終えた吸収した集まりの行は消すが、その鍵の値は消さない（Edge Case
// 「集まり同士を束ねる」）。同じ集まりになった組の候補を消す。全体を 1 つの取引で行い、確定後に domain.VideoBundleChanged を
// 1 回発行する。
func (s *VersionStore) Bundle(ctx context.Context, videoIDs []int64, representativeID int64) (domain.VideoVersions, error) {
	ids := uniqueIDs(videoIDs)
	if len(ids) < 2 {
		return domain.VideoVersions{}, domain.ErrTooFewVersions
	}
	if !slices.Contains(ids, representativeID) {
		return domain.VideoVersions{}, domain.ErrRepresentativeNotSelected
	}

	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot bundle the videos: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var members []string
	absorbed := map[int64]bool{}
	var representativeKey, representativeUserKey string
	for _, id := range ids {
		key, err := bundlableContentKey(ctx, tx, id)
		if err != nil {
			return domain.VideoVersions{}, err
		}
		bundle, bundled, err := bundleOf(ctx, tx, key)
		if err != nil {
			return domain.VideoVersions{}, err
		}
		if id == representativeID {
			representativeKey, representativeUserKey = key, key
			if bundled {
				representativeUserKey = bundle.userKey
			}
		}
		if !bundled {
			members = append(members, key)
			continue
		}
		if absorbed[bundle.id] {
			continue
		}
		absorbed[bundle.id] = true
		keys, err := bundleMemberKeys(ctx, tx, bundle.id)
		if err != nil {
			return domain.VideoVersions{}, err
		}
		members = append(members, keys...)
	}
	slices.Sort(members)
	members = slices.Compact(members)

	now := time.Now().Unix()
	result, err := tx.ExecContext(ctx,
		`insert into video_bundles (user_key, representative_key, created_at, updated_at) values ('bundle:new', ?, ?, ?)`,
		representativeKey, now, now)
	if err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot create the bundle: %w", err)
	}
	bundleID, err := result.LastInsertId()
	if err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot create the bundle: %w", err)
	}
	userKey := bundleUserKey(bundleID)
	if _, err := tx.ExecContext(ctx, `update video_bundles set user_key = ? where id = ?`, userKey, bundleID); err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot create the bundle: %w", err)
	}
	if err := copyUserData(ctx, tx, representativeUserKey, userKey); err != nil {
		return domain.VideoVersions{}, err
	}
	for _, key := range members {
		if _, err := tx.ExecContext(ctx,
			`insert into video_bundle_members (content_key, bundle_id, added_at) values (?, ?, ?)
			 on conflict (content_key) do update set bundle_id = excluded.bundle_id`,
			key, bundleID, now); err != nil {
			return domain.VideoVersions{}, fmt.Errorf("cannot add a version to the bundle: %w", err)
		}
	}
	for id := range absorbed {
		if _, err := tx.ExecContext(ctx, `delete from video_bundles where id = ?`, id); err != nil {
			return domain.VideoVersions{}, fmt.Errorf("cannot remove the absorbed bundle: %w", err)
		}
	}
	// 同じ集まりになった組は候補から消す（data-model.md §7）。
	if err := pruneVersionCandidates(ctx, tx); err != nil {
		return domain.VideoVersions{}, err
	}

	if err := rebuildFolderIndex(ctx, tx); err != nil {
		return domain.VideoVersions{}, err
	}
	versions, err := videoVersions(ctx, tx, domain.AudienceOwner, representativeID)
	if err != nil {
		return domain.VideoVersions{}, err
	}
	changed, err := videoIDsForContentKeys(ctx, tx, members)
	if err != nil {
		return domain.VideoVersions{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot bundle the videos: %w", err)
	}
	s.db.publishEvents(domain.VideoBundleChanged{VideoIDs: changed})
	return versions, nil
}

// MakeRepresentative は videoID の動画をその集まりの代表にする（data-model.md §8）。
// 集まりの値には触らない（要件 7）。動画がいまライブラリに無ければ domain.ErrNotFound、
// 集まりに属さなければ domain.ErrNotBundled。確定後に domain.VideoBundleChanged を
// 1 回発行する。
func (s *VersionStore) MakeRepresentative(ctx context.Context, videoID int64) (domain.VideoVersions, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot change the representative: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	key, err := bundlableContentKey(ctx, tx, videoID)
	if err != nil {
		return domain.VideoVersions{}, err
	}
	bundle, bundled, err := bundleOf(ctx, tx, key)
	if err != nil {
		return domain.VideoVersions{}, err
	}
	if !bundled {
		return domain.VideoVersions{}, domain.ErrNotBundled
	}
	if _, err := tx.ExecContext(ctx,
		`update video_bundles set representative_key = ?, updated_at = ? where id = ?`,
		key, time.Now().Unix(), bundle.id); err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot change the representative: %w", err)
	}

	if err := rebuildFolderIndex(ctx, tx); err != nil {
		return domain.VideoVersions{}, err
	}
	versions, err := videoVersions(ctx, tx, domain.AudienceOwner, videoID)
	if err != nil {
		return domain.VideoVersions{}, err
	}
	members, err := bundleMemberKeys(ctx, tx, bundle.id)
	if err != nil {
		return domain.VideoVersions{}, err
	}
	changed, err := videoIDsForContentKeys(ctx, tx, members)
	if err != nil {
		return domain.VideoVersions{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot change the representative: %w", err)
	}
	s.db.publishEvents(domain.VideoBundleChanged{VideoIDs: changed})
	return versions, nil
}

// Unbundle は videoID の動画をその集まりから外す（data-model.md §8）。外した動画は自分の
// content_key の値に戻る。代表を外したときは、残りのうち実効の代表の規則（data-model.md §4、
// 所有者から見て）で次の代表を選ぶ。残りが 1 本なら集まりを解き、集まりの鍵の値を残った
// 1 本の content_key へ写す（既存の行は置き換える。Edge Case）。動画がいまライブラリに
// 無ければ domain.ErrNotFound、集まりに属さなければ domain.ErrNotBundled。確定後に、元の
// 全メンバーの domain.VideoBundleChanged を 1 回発行する。
func (s *VersionStore) Unbundle(ctx context.Context, videoID int64) (domain.Video, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Video{}, fmt.Errorf("cannot unbundle the video: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	key, err := bundlableContentKey(ctx, tx, videoID)
	if err != nil {
		return domain.Video{}, err
	}
	bundle, bundled, err := bundleOf(ctx, tx, key)
	if err != nil {
		return domain.Video{}, err
	}
	if !bundled {
		return domain.Video{}, domain.ErrNotBundled
	}
	members, err := bundleMemberKeys(ctx, tx, bundle.id)
	if err != nil {
		return domain.Video{}, err
	}
	changed, err := videoIDsForContentKeys(ctx, tx, members)
	if err != nil {
		return domain.Video{}, err
	}

	if _, err := tx.ExecContext(ctx, `delete from video_bundle_members where content_key = ?`, key); err != nil {
		return domain.Video{}, fmt.Errorf("cannot unbundle the video: %w", err)
	}
	remaining := slices.DeleteFunc(slices.Clone(members), func(member string) bool { return member == key })
	switch {
	case len(remaining) <= 1:
		if len(remaining) == 1 {
			if err := replaceUserData(ctx, tx, bundle.userKey, remaining[0]); err != nil {
				return domain.Video{}, err
			}
		}
		if _, err := tx.ExecContext(ctx, `delete from video_bundles where id = ?`, bundle.id); err != nil {
			return domain.Video{}, fmt.Errorf("cannot dissolve the bundle: %w", err)
		}
	case bundle.representativeKey == key:
		next, err := nextRepresentativeKey(ctx, tx, bundle.id)
		if err != nil {
			return domain.Video{}, err
		}
		if _, err := tx.ExecContext(ctx,
			`update video_bundles set representative_key = ?, updated_at = ? where id = ?`,
			next, time.Now().Unix(), bundle.id); err != nil {
			return domain.Video{}, fmt.Errorf("cannot change the representative: %w", err)
		}
	}

	if err := rebuildFolderIndex(ctx, tx); err != nil {
		return domain.Video{}, err
	}
	video, err := getVideo(ctx, tx, domain.AudienceOwner, videoID)
	if err != nil {
		return domain.Video{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Video{}, fmt.Errorf("cannot unbundle the video: %w", err)
	}
	s.db.publishEvents(domain.VideoBundleChanged{VideoIDs: changed})
	return video, nil
}

// Versions は videoID の動画の集まりの全バージョンのうち、見る人（audience）に見せてよい
// 所在を持つものを、実効の代表を先頭に返す（data-model.md §8）。動画が見せられなければ
// domain.ErrNotFound。集まりに属さなければその 1 本だけを返す。
func (s *VersionStore) Versions(ctx context.Context, audience domain.Audience, videoID int64) (domain.VideoVersions, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot read the versions: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	versions, err := videoVersions(ctx, tx, audience, videoID)
	if err != nil {
		return domain.VideoVersions{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot read the versions: %w", err)
	}
	return versions, nil
}

// videoVersions は VersionStore.Versions の本体で、呼び出し側の取引の中で読む。
func videoVersions(ctx context.Context, tx *sql.Tx, audience domain.Audience, videoID int64) (domain.VideoVersions, error) {
	video, err := getVideo(ctx, tx, audience, videoID)
	if err != nil {
		return domain.VideoVersions{}, err
	}
	if video.Versions == nil {
		return domain.VideoVersions{RepresentativeID: video.ID, Items: []domain.Video{video}}, nil
	}
	rows, err := tx.QueryContext(ctx, `select `+videoColumns(audience)+` from videos
		join video_bundle_members vm on vm.content_key = videos.content_key
		where vm.bundle_id = (select bundle_id from video_bundle_members where content_key = ?) and `+
		visibleVideoCondition("videos", audience), video.ContentKey)
	if err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot read the versions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := []domain.Video{}
	for rows.Next() {
		item, err := scanVideo(rows)
		if err != nil {
			return domain.VideoVersions{}, fmt.Errorf("cannot read the versions: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot read the versions: %w", err)
	}
	if err := rows.Close(); err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot read the versions: %w", err)
	}
	for i := range items {
		ref := *video.Versions
		items[i].Versions = &ref
	}
	representativeID := video.Versions.RepresentativeID
	domain.SortVideoVersions(items, representativeID)
	return domain.VideoVersions{RepresentativeID: representativeID, Items: items}, nil
}

// bundleVersionsRef は content_key の動画が集まりのメンバーなら、見る人（audience）から見た
// 集まりの要約（見せてよい所在を持つメンバーの本数と実効の代表、data-model.md §4）を返す。
// メンバーでなければ nil。getVideo（詳細）が Video.Versions を埋めるのに使う。
func bundleVersionsRef(ctx context.Context, q rowQueryer, audience domain.Audience, contentKey string) (*domain.VideoVersionsRef, error) {
	var count int
	var representativeID int64
	err := q.QueryRowContext(ctx, `
		with shown(id, is_representative) as (
			select v.id, v.content_key = b.representative_key
			  from video_bundle_members self
			  join video_bundle_members m on m.bundle_id = self.bundle_id
			  join video_bundles b on b.id = m.bundle_id
			  join videos v on v.content_key = m.content_key
			 where self.content_key = ? and `+visibleVideoCondition("v", audience)+`)
		select count(*), coalesce((select id from shown order by is_representative desc, id limit 1), 0) from shown`,
		contentKey).Scan(&count, &representativeID)
	if err != nil {
		return nil, fmt.Errorf("cannot read the bundle: %w", err)
	}
	if count == 0 {
		return nil, nil
	}
	return &domain.VideoVersionsRef{Count: count, RepresentativeID: representativeID}, nil
}

// bundlableContentKey は id の動画の content_key を返す。いまライブラリ（登録フォルダの下）に
// 所在を持たないか、内容を読めていない動画は domain.ErrNotFound。
func bundlableContentKey(ctx context.Context, tx *sql.Tx, videoID int64) (string, error) {
	var key string
	err := tx.QueryRowContext(ctx, `select v.content_key from videos v
		where v.id = ? and v.content_key <> '' and `+registeredVideoCondition("v"), videoID).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("cannot read the video (id=%d): %w", videoID, err)
	}
	return key, nil
}

// bundleOf は content_key が属する集まりを返す。属さなければ ok は false。
func bundleOf(ctx context.Context, tx *sql.Tx, contentKey string) (bundleRef, bool, error) {
	var ref bundleRef
	err := tx.QueryRowContext(ctx, `select b.id, b.user_key, b.representative_key
		from video_bundle_members m join video_bundles b on b.id = m.bundle_id
		where m.content_key = ?`, contentKey).Scan(&ref.id, &ref.userKey, &ref.representativeKey)
	if errors.Is(err, sql.ErrNoRows) {
		return bundleRef{}, false, nil
	}
	if err != nil {
		return bundleRef{}, false, fmt.Errorf("cannot read the bundle: %w", err)
	}
	return ref, true, nil
}

// bundleMemberKeys は集まりの全メンバーの content_key を順に返す。
func bundleMemberKeys(ctx context.Context, tx *sql.Tx, bundleID int64) ([]string, error) {
	return queryStrings(ctx, tx, `select content_key from video_bundle_members where bundle_id = ? order by content_key`, bundleID)
}

// nextRepresentativeKey は代表を外した集まりの次の代表を選ぶ。所有者から見た実効の代表の
// 規則（data-model.md §4）どおり、登録フォルダの下に所在を持つメンバーのうち videos.id の
// 最小のもの。1 本も無ければ content_key の最小のもの（代表はメンバーである、の不変条件を保つ）。
func nextRepresentativeKey(ctx context.Context, tx *sql.Tx, bundleID int64) (string, error) {
	var key string
	err := tx.QueryRowContext(ctx, `select v.content_key from video_bundle_members m
		join videos v on v.content_key = m.content_key
		where m.bundle_id = ? and `+registeredVideoCondition("v")+` order by v.id limit 1`, bundleID).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `select min(content_key) from video_bundle_members where bundle_id = ?`, bundleID).Scan(&key)
	}
	if err != nil {
		return "", fmt.Errorf("cannot choose the next representative: %w", err)
	}
	return key, nil
}

// videoIDsForContentKeys は content_key の動画の id を小さい順に返す。
func videoIDsForContentKeys(ctx context.Context, tx *sql.Tx, keys []string) ([]int64, error) {
	encoded, err := json.Marshal(keys)
	if err != nil {
		return nil, fmt.Errorf("cannot build content keys: %w", err)
	}
	rows, err := tx.QueryContext(ctx,
		`select id from videos where content_key in (select value from json_each(?)) order by id`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the bundled videos: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cannot read the bundled videos: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the bundled videos: %w", err)
	}
	return ids, nil
}

// copyUserData は利用者データの鍵 from の 3 つの表の行を、鍵 to の行として写す。to に行が
// あれば残す（新しい集まりの鍵には行が無い）。
func copyUserData(ctx context.Context, tx *sql.Tx, from, to string) error {
	for _, t := range userDataTables {
		if _, err := tx.ExecContext(ctx,
			`insert or ignore into `+t.table+` (content_key, `+t.columns+`)
			 select ?, `+t.columns+` from `+t.table+` where content_key = ?`, to, from); err != nil {
			return fmt.Errorf("cannot copy the %s values: %w", t.table, err)
		}
	}
	return nil
}

// replaceUserData は鍵 to の 3 つの表の行を消し、鍵 from の行で置き換える。
func replaceUserData(ctx context.Context, tx *sql.Tx, from, to string) error {
	for _, t := range userDataTables {
		if _, err := tx.ExecContext(ctx, `delete from `+t.table+` where content_key = ?`, to); err != nil {
			return fmt.Errorf("cannot replace the %s values: %w", t.table, err)
		}
	}
	return copyUserData(ctx, tx, from, to)
}

// queryStrings は 1 列の文字列の結果を読み切る。
func queryStrings(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]string, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read the bundle: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, fmt.Errorf("cannot read the bundle: %w", err)
		}
		out = append(out, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the bundle: %w", err)
	}
	return out, nil
}

// uniqueIDs は id の重複を除き、最初に現れた順で返す。
func uniqueIDs(ids []int64) []int64 {
	seen := make(map[int64]bool, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
