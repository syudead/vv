package store

// ドロップで「バージョンとしてまとめる」（specs/050-drag-organize/data-model.md「Operations」「Undo
// image」の Bundle）。束ねる本体は VersionStore.Bundle と共有し（planBundle・applyBundle）、落とした
// 先の動画を代表にする。像は吸い込む集まりと新しい集まりの行、付け替える項目を持つグループと
// プレイリストの全体、新しい鍵へ写した利用者データ、刈り取る候補である。

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// Bundle は動画 videoIDs を落とした先の動画 targetID と 1 つの集まりに束ね、targetID を代表にする。
// 既に集まりに属する動画はその集まりの全メンバーごと移る（Edge Case）。取り消しの像を返し、確定後に
// domain.VideoBundleChanged と domain.GroupsChanged を発行する。重複を除いて 2 本未満なら
// domain.ErrTooFewVersions、動画がいまライブラリに無ければ domain.ErrGroupTargetNotFound、集まりを
// 展開して domain.MaxOrganizeBundleVideos 本を超えれば domain.ErrTooManyVersions で、どれも何も
// 書かない。
func (s *OrganizeStore) Bundle(ctx context.Context, videoIDs []int64, targetID int64) ([]byte, error) {
	ids := uniqueIDs(append([]int64{targetID}, videoIDs...))
	if len(ids) < 2 {
		return nil, domain.ErrTooFewVersions
	}
	var encoded []byte
	err := writeWithEvents(ctx, s.db, "bundle the videos", func(tx *sql.Tx) ([]domain.Event, error) {
		plan, err := planBundle(ctx, tx, ids, targetID)
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrGroupTargetNotFound
		}
		if err != nil {
			return nil, err
		}
		if len(plan.members) > domain.MaxOrganizeBundleVideos {
			return nil, domain.ErrTooManyVersions
		}
		image, err := bundleImage(ctx, tx, plan)
		if err != nil {
			return nil, err
		}
		if err := image.captureBefore(ctx, tx); err != nil {
			return nil, err
		}
		outcome, err := applyBundle(ctx, tx, plan)
		if err != nil {
			return nil, err
		}
		if err := image.addCreatedBundle(outcome); err != nil {
			return nil, err
		}
		if err := image.captureAfter(ctx, tx); err != nil {
			return nil, err
		}
		image.finishGroupsImage(outcome.changedGroups)
		image.RebuildFolders = true
		image.Videos = outcome.changedVideos
		if encoded, err = image.encode(); err != nil {
			return nil, err
		}
		return outcome.events(), nil
	})
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

// bundleImage は束ねる前に決まる像の範囲を、親の表を先にして並べる。吸い込む集まりの video_bundles と
// video_bundle_members、吸い込む鍵の項目か除外を持つグループの group_items と group_exclusions、
// 吸い込む鍵の項目を持つプレイリストの playlist_items（どれも全体）、新しい集まりのメンバーの鍵の
// video_version_candidates（key_a で引く。候補の組は key_a < key_b なので、刈り取る組はどれも入る）。
func bundleImage(ctx context.Context, tx *sql.Tx, plan bundlePlan) (undoImage, error) {
	keys, err := json.Marshal(plan.absorbedKeys)
	if err != nil {
		return undoImage{}, fmt.Errorf("cannot build content keys: %w", err)
	}
	groups, err := queryInt64s(ctx, tx, `select group_id from group_items where content_key in (select value from json_each(?1))
		union select group_id from group_exclusions where content_key in (select value from json_each(?1))
		order by 1`, string(keys))
	if err != nil {
		return undoImage{}, fmt.Errorf("cannot read the groups: %w", err)
	}
	playlists, err := queryInt64s(ctx, tx, `select distinct playlist_id from playlist_items
		where content_key in (select value from json_each(?)) order by playlist_id`, string(keys))
	if err != nil {
		return undoImage{}, fmt.Errorf("cannot read the playlists: %w", err)
	}
	var specs []imageScopeSpec
	for _, spec := range []imageScopeSpec{{"video_bundles", "id", nil}, {"video_bundle_members", "bundle_id", nil}} {
		for _, id := range plan.absorbedBundles {
			specs = append(specs, imageScopeSpec{spec.table, spec.column, id})
		}
	}
	for _, spec := range []imageScopeSpec{{"group_items", "group_id", nil}, {"group_exclusions", "group_id", nil}} {
		for _, id := range groups {
			specs = append(specs, imageScopeSpec{spec.table, spec.column, id})
		}
	}
	for _, id := range playlists {
		specs = append(specs, imageScopeSpec{"playlist_items", "playlist_id", id})
	}
	for _, key := range plan.members {
		specs = append(specs, imageScopeSpec{"video_version_candidates", "key_a", key})
	}
	var image undoImage
	if err := image.addScopes(specs); err != nil {
		return undoImage{}, err
	}
	return image, nil
}

// addCreatedBundle は束ねた後で、新しい集まりの video_bundles と video_bundle_members、新しい鍵へ
// 写した利用者データの範囲を足す。どれも束ねる前には無かった行なので、前の像は空である。
func (image *undoImage) addCreatedBundle(outcome bundleOutcome) error {
	specs := []imageScopeSpec{{"video_bundles", "id", outcome.bundleID}, {"video_bundle_members", "bundle_id", outcome.bundleID}}
	for _, t := range userDataTables {
		specs = append(specs, imageScopeSpec{t.table, "content_key", outcome.userKey})
	}
	return image.addScopes(specs)
}

// addScopes は specs の範囲を像の終わりに足す。
func (image *undoImage) addScopes(specs []imageScopeSpec) error {
	for _, spec := range specs {
		scope, err := newImageScope(spec.table, spec.column, spec.key)
		if err != nil {
			return err
		}
		image.Scopes = append(image.Scopes, scope)
	}
	return nil
}
