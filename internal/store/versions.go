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
// 集まりのタグ・再生位置・公開の設定・お気に入りは video_tags・playback_progress・public_videos・
// video_favorites に集まりの user_key を鍵として置く。各メンバーの content_key の行は触らずに残すので、
// 外したメンバーは束ねる前の値に戻る。
//
// 束ねる・代表を替える・外す操作は、見せる動画（data-model.md §4）が変わり、代表の所在の
// フォルダの直下の本数が変わるので、同じ取引の中でフォルダの索引を作り直す（research.md R-4）。

// userDataTables は利用者データの鍵で引く表と、鍵以外の列である。束ねる・解くときに
// 値を別の鍵へ写すのに使う。作品情報の 6 表（specs/047-video-work-info/data-model.md「Bundles and
// succession」）も表ごとに写す。束ねる先の鍵には行が無く、解いて残る 1 本へ戻すときは replaceUserData が
// 6 表とも消してから写すので、2 本の動画の値が混ざらない。
var userDataTables = []struct{ table, columns string }{
	{"playback_progress", "position_ms, duration_ms, completed, updated_at"},
	{"video_tags", "tag_id, created_at"},
	{"public_videos", "published_at"},
	{"video_favorites", "favorited_at"},
	{"video_work_info", "code, code_key, description, search_version"},
	{"video_work_dates", "kind, date"},
	{"video_work_urls", "position, url"},
	{"video_work_external_ids", "source, value"},
	{"video_work_people", "role, position, person_id"},
	{"video_work_organizations", "role, position, organization_id"},
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
// 「集まり同士を束ねる」）。プレイリストとグループの項目は、吸い込んだ各鍵から新しい鍵へ移す（写さない）。
// 同じ集まりになった組の候補を消す。全体を 1 つの取引で行い、確定後に domain.VideoBundleChanged を
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

	plan, err := planBundle(ctx, tx, ids, representativeID)
	if err != nil {
		return domain.VideoVersions{}, err
	}
	outcome, err := applyBundle(ctx, tx, plan)
	if err != nil {
		return domain.VideoVersions{}, err
	}
	versions, err := videoVersions(ctx, tx, domain.AudienceOwner, representativeID)
	if err != nil {
		return domain.VideoVersions{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.VideoVersions{}, fmt.Errorf("cannot bundle the videos: %w", err)
	}
	s.db.publishEvents(outcome.events()...)
	return versions, nil
}

// bundlePlan は束ねる前に読んだ、束ねる内容である。members は新しい集まりの全メンバーの content_key
// （重複なし、順に並ぶ）、absorbedKeys は新しい集まりの鍵へ項目を移す利用者データの鍵（メンバーの
// content_key と吸い込む集まりの鍵）、absorbedBundles は吸い込んで消す集まりの id（初めに現れた順）、
// representativeKey は代表の content_key、representativeUserKey は新しい集まりへ値を写す鍵である。
type bundlePlan struct {
	members               []string
	absorbedKeys          []string
	absorbedBundles       []int64
	representativeKey     string
	representativeUserKey string
}

// planBundle は ids の動画（重複なし、2 本以上）を representativeID を代表に束ねる内容を読む
// （VersionStore.Bundle と OrganizeStore.Bundle が共有する）。何も書かない。いまライブラリに無い
// 動画があれば domain.ErrNotFound。
func planBundle(ctx context.Context, tx *sql.Tx, ids []int64, representativeID int64) (bundlePlan, error) {
	var plan bundlePlan
	// absorbed は absorbedBundles の集合である。POST /api/video-bundles は 20000 本を受け付けるので、
	// 並びを走査すると二乗になる。
	absorbed := map[int64]struct{}{}
	for _, id := range ids {
		key, err := bundlableContentKey(ctx, tx, id)
		if err != nil {
			return bundlePlan{}, err
		}
		bundle, bundled, err := bundleOf(ctx, tx, key)
		if err != nil {
			return bundlePlan{}, err
		}
		if id == representativeID {
			plan.representativeKey, plan.representativeUserKey = key, key
			if bundled {
				plan.representativeUserKey = bundle.userKey
			}
		}
		if !bundled {
			plan.members = append(plan.members, key)
			plan.absorbedKeys = append(plan.absorbedKeys, key)
			continue
		}
		if _, seen := absorbed[bundle.id]; seen {
			continue
		}
		absorbed[bundle.id] = struct{}{}
		plan.absorbedBundles = append(plan.absorbedBundles, bundle.id)
		keys, err := bundleMemberKeys(ctx, tx, bundle.id)
		if err != nil {
			return bundlePlan{}, err
		}
		plan.members = append(plan.members, keys...)
		plan.absorbedKeys = append(plan.absorbedKeys, bundle.userKey)
		plan.absorbedKeys = append(plan.absorbedKeys, keys...)
	}
	slices.Sort(plan.members)
	plan.members = slices.Compact(plan.members)
	return plan, nil
}

// bundleOutcome は applyBundle が書いた結果である。bundleID と userKey は新しい集まりの id と鍵、
// changedGroups は項目を付け替えたかフォルダの索引で変わったグループとその祖先、changedVideos は
// 新しい集まりのメンバーの動画の id である。
type bundleOutcome struct {
	bundleID      int64
	userKey       string
	changedGroups []int64
	changedVideos []int64
}

// events は確定後に発行する知らせである。
func (o bundleOutcome) events() []domain.Event {
	return append([]domain.Event{domain.VideoBundleChanged{VideoIDs: o.changedVideos}},
		groupsChangedEvents(o.changedGroups)...)
}

// applyBundle は planBundle の内容で束ねる（data-model.md §8）。新しい集まりを作って代表の値を写し、
// プレイリストとグループの項目を吸い込んだ各鍵から新しい鍵へ移し（写さない）、メンバーを移して
// 吸い込んだ集まりの行を消す（その鍵の値は消さない）。同じ集まりになった組の候補を消し、フォルダの
// 索引を作り直し、形の変わったプレイリストの更新日時を進める。呼び出し側の取引の中で行う。
func applyBundle(ctx context.Context, tx *sql.Tx, plan bundlePlan) (bundleOutcome, error) {
	// 項目がまとまった、見せるかが変わったプレイリストは、終わりに更新日時を進める。
	shapes, err := snapshotPlaylistShapes(ctx, tx, plan.absorbedKeys)
	if err != nil {
		return bundleOutcome{}, err
	}
	now := time.Now().Unix()
	bundleID, userKey, err := createBundle(ctx, tx, plan.representativeKey, now)
	if err != nil {
		return bundleOutcome{}, err
	}
	if err := copyUserData(ctx, tx, plan.representativeUserKey, userKey); err != nil {
		return bundleOutcome{}, err
	}
	var rekeyedGroups []int64
	for _, key := range plan.absorbedKeys {
		ids, err := rekeyUserItems(ctx, tx, key, userKey)
		if err != nil {
			return bundleOutcome{}, err
		}
		rekeyedGroups = append(rekeyedGroups, ids...)
	}
	if err := moveBundleMembers(ctx, tx, plan, bundleID, now); err != nil {
		return bundleOutcome{}, err
	}
	// 同じ集まりになった組は候補から消す（data-model.md §7）。
	if err := pruneVersionCandidates(ctx, tx); err != nil {
		return bundleOutcome{}, err
	}
	changedGroups, err := rebuildFolderIndex(ctx, tx)
	if err != nil {
		return bundleOutcome{}, err
	}
	if err := touchReshapedPlaylists(ctx, tx, shapes); err != nil {
		return bundleOutcome{}, err
	}
	changed, err := videoIDsForContentKeys(ctx, tx, plan.members)
	if err != nil {
		return bundleOutcome{}, err
	}
	return bundleOutcome{bundleID: bundleID, userKey: userKey,
		changedGroups: normalizeIDs(append(rekeyedGroups, changedGroups...)), changedVideos: changed}, nil
}

// createBundle は代表 representativeKey の新しい集まりの行を作り、その id と利用者データの鍵を返す。
func createBundle(ctx context.Context, tx *sql.Tx, representativeKey string, now int64) (int64, string, error) {
	result, err := tx.ExecContext(ctx,
		`insert into video_bundles (user_key, representative_key, created_at, updated_at) values ('bundle:new', ?, ?, ?)`,
		representativeKey, now, now)
	if err != nil {
		return 0, "", fmt.Errorf("cannot create the bundle: %w", err)
	}
	bundleID, err := result.LastInsertId()
	if err != nil {
		return 0, "", fmt.Errorf("cannot create the bundle: %w", err)
	}
	userKey := bundleUserKey(bundleID)
	if _, err := tx.ExecContext(ctx, `update video_bundles set user_key = ? where id = ?`, userKey, bundleID); err != nil {
		return 0, "", fmt.Errorf("cannot create the bundle: %w", err)
	}
	return bundleID, userKey, nil
}

// moveBundleMembers は plan のメンバーを集まり bundleID へ移し、吸い込んだ集まりの行を消す。
func moveBundleMembers(ctx context.Context, tx *sql.Tx, plan bundlePlan, bundleID, now int64) error {
	for _, key := range plan.members {
		if _, err := tx.ExecContext(ctx,
			`insert into video_bundle_members (content_key, bundle_id, added_at) values (?, ?, ?)
			 on conflict (content_key) do update set bundle_id = excluded.bundle_id`,
			key, bundleID, now); err != nil {
			return fmt.Errorf("cannot add a version to the bundle: %w", err)
		}
	}
	for _, id := range plan.absorbedBundles {
		if _, err := tx.ExecContext(ctx, `delete from video_bundles where id = ?`, id); err != nil {
			return fmt.Errorf("cannot remove the absorbed bundle: %w", err)
		}
	}
	return nil
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

	changedGroups, err := rebuildFolderIndex(ctx, tx)
	if err != nil {
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
	s.db.publishEvents(append([]domain.Event{domain.VideoBundleChanged{VideoIDs: changed}}, groupsChangedEvents(changedGroups)...)...)
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
	// 項目が見せるかが変わったプレイリストは、終わりに更新日時を進める。
	shapes, err := snapshotPlaylistShapes(ctx, tx, append(slices.Clone(members), bundle.userKey))
	if err != nil {
		return domain.Video{}, err
	}

	if _, err := tx.ExecContext(ctx, `delete from video_bundle_members where content_key = ?`, key); err != nil {
		return domain.Video{}, fmt.Errorf("cannot unbundle the video: %w", err)
	}
	remaining := slices.DeleteFunc(slices.Clone(members), func(member string) bool { return member == key })
	var rekeyedGroups []int64
	switch {
	case len(remaining) <= 1:
		if len(remaining) == 1 {
			if err := replaceUserData(ctx, tx, bundle.userKey, remaining[0]); err != nil {
				return domain.Video{}, err
			}
			// プレイリストとグループの項目は集まりの鍵から残った 1 本へ移す。他のメンバーが残るときは集まりに
			// 残し、外したメンバーには項目を持たせない（specs/044-playlists/research.md R-2）。
			if rekeyedGroups, err = rekeyUserItems(ctx, tx, bundle.userKey, remaining[0]); err != nil {
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

	changedGroups, err := rebuildFolderIndex(ctx, tx)
	if err != nil {
		return domain.Video{}, err
	}
	if err := touchReshapedPlaylists(ctx, tx, shapes); err != nil {
		return domain.Video{}, err
	}
	video, err := getVideo(ctx, tx, domain.AudienceOwner, videoID)
	if err != nil {
		return domain.Video{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Video{}, fmt.Errorf("cannot unbundle the video: %w", err)
	}
	s.db.publishEvents(append([]domain.Event{domain.VideoBundleChanged{VideoIDs: changed}},
		groupsChangedEvents(normalizeIDs(append(rekeyedGroups, changedGroups...)))...)...)
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
	rows, err := tx.QueryContext(ctx, `select `+videoColumns(audience)+` from videos`+representativeJoin(audience)+`
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
			  join videos v on v.content_key = m.content_key and v.content_key <> ''
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
		join videos v on v.content_key = m.content_key and v.content_key <> ''
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

// copyUserData は利用者データの鍵 from の userDataTables の表の行を、鍵 to の行として写す。to に行が
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

// replaceUserData は鍵 to の userDataTables の表の行を消し、鍵 from の行で置き換える。
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
