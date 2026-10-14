package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// RelatedCandidates は関連動画の近さを測る手がかりを、見る人に見せる動画すべてについて
// 返す（issue 931）。見せる動画は一覧と同じで、見る人に見せてよい所在のある動画のうち、
// 集まりの代表以外のバージョン（shownVideoCondition）を除く。基準の動画 selfID は、
// 代表以外のバージョンでも見せてよければ含め、その集まりの他のバージョン（代表を含む）は
// 除く。並べ方は internal/domain の RankRelated が
// 決める。
//
// 手がかりは代表の所在のパスと題名（表示名があればそれ）、追加日時、タグ、映像の指紋で
// ある。タグは手で付けた分（確定と仮）とフォルダ名から付いた分を合わせる
// （videoHasTagCondition と同じ 2 つの出所）。指紋は今の版でなくても返し、比べるかは
// domain が決める。すべてを 1 つの読み取りのスナップショットで読む。
func (s *LibraryStore) RelatedCandidates(ctx context.Context, audience domain.Audience, selfID int64) ([]domain.RelatedCandidate, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot start reading related video candidates: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `select videos.id, rep.path, rep.title,
		(select ov.display_name from video_overrides ov
		  where ov.content_key = videos.content_key and videos.content_key <> ''),
		videos.added_at, f.version, f.interval_ms, f.hashes
		from videos`+representativeJoin(audience)+`
		left join video_fingerprints f on f.content_key = videos.content_key and videos.content_key <> ''
		where `+visibleVideoCondition("videos", audience)+`
		  and (videos.id = ?1 or (`+shownVideoCondition("videos", audience)+`
		  and not exists (select 1 from videos self
		       join video_bundle_members sm on sm.content_key = self.content_key and self.content_key <> ''
		       join video_bundle_members om on om.bundle_id = sm.bundle_id
		      where self.id = ?1 and om.content_key = videos.content_key)))`, selfID)
	if err != nil {
		return nil, fmt.Errorf("cannot read related video candidates: %w", err)
	}
	candidates, byID, err := scanRelatedCandidates(rows)
	if err != nil {
		return nil, err
	}

	tagRows, err := tx.QueryContext(ctx, `select v.id, vt.tag_id from videos v
		join video_tags vt on vt.content_key = `+userKeyExpr("v")+`
		where v.content_key <> ''
		union
		select v.id, tn.tag_id from video_folder_names vfn
		join tag_names tn on tn.name = vfn.name
		join videos v on v.id = vfn.video_id and v.content_key <> ''`)
	if err != nil {
		return nil, fmt.Errorf("cannot read tags of related video candidates: %w", err)
	}
	defer func() { _ = tagRows.Close() }()
	for tagRows.Next() {
		var videoID, tagID int64
		if err := tagRows.Scan(&videoID, &tagID); err != nil {
			return nil, fmt.Errorf("cannot read tags of related video candidates: %w", err)
		}
		if index, ok := byID[videoID]; ok {
			candidates[index].TagIDs = append(candidates[index].TagIDs, tagID)
		}
	}
	if err := tagRows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read tags of related video candidates: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cannot finish reading related video candidates: %w", err)
	}
	return candidates, nil
}

// scanRelatedCandidates は RelatedCandidates の動画の行を読み、id から位置への索引も返す。
func scanRelatedCandidates(rows *sql.Rows) ([]domain.RelatedCandidate, map[int64]int, error) {
	defer func() { _ = rows.Close() }()
	candidates := []domain.RelatedCandidate{}
	byID := map[int64]int{}
	for rows.Next() {
		var item domain.RelatedCandidate
		var displayName sql.NullString
		var addedAt int64
		var version, intervalMs sql.NullInt64
		var hashes []byte
		if err := rows.Scan(&item.VideoID, &item.Path, &item.Title, &displayName, &addedAt,
			&version, &intervalMs, &hashes); err != nil {
			return nil, nil, fmt.Errorf("cannot read related video candidates: %w", err)
		}
		if displayName.Valid && displayName.String != "" {
			item.Title = displayName.String
		}
		item.AddedAt = time.Unix(addedAt, 0)
		if version.Valid && intervalMs.Valid {
			// 読めない指紋は無いものとして扱う。指紋の点が 0 になるだけで、並びは作れる。
			if fingerprint, err := domain.DecodeFingerprint(hashes, int(version.Int64), intervalMs.Int64); err == nil {
				item.Fingerprint = &fingerprint
			}
		}
		byID[item.VideoID] = len(candidates)
		candidates = append(candidates, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("cannot read related video candidates: %w", err)
	}
	return candidates, byID, nil
}

// VideosByIDs は指定した動画の本体を返す。並びは ids と同じで、見る人に見せて
// よい所在が無い動画（登録フォルダの下に所在が無くなった動画と、ゲストには
// 非公開の動画）と、集まりの代表以外のバージョン（shownVideoCondition）は抜ける。
func (s *LibraryStore) VideosByIDs(ctx context.Context, audience domain.Audience, ids []int64) ([]domain.Video, error) {
	if len(ids) == 0 {
		return []domain.Video{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}

	rows, err := s.db.sql.QueryContext(ctx, `select `+videoColumns(audience)+` from videos`+representativeJoin(audience)+` where videos.id in (`+
		placeholders+`) and `+visibleVideoCondition("videos", audience)+` and `+shownVideoCondition("videos", audience), args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read related videos: %w", err)
	}
	defer func() { _ = rows.Close() }()

	byID := make(map[int64]domain.Video, len(ids))
	for rows.Next() {
		video, err := scanVideo(rows)
		if err != nil {
			return nil, fmt.Errorf("cannot read related videos: %w", err)
		}
		byID[video.ID] = video
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read related videos: %w", err)
	}

	videos := make([]domain.Video, 0, len(ids))
	for _, id := range ids {
		if video, ok := byID[id]; ok {
			videos = append(videos, video)
		}
	}
	return videos, nil
}

// VideoGroup は動画 id の文脈のグループを、見る人に見せてよい動画の再生順で返す
// （GET /api/videos/{id} の group、関連動画の group、GET /api/videos/{id}/group-members。
// specs/048-groups/research.md R-9、data-model.md「Play order」）。文脈のグループは、contextID が
// 0 でなく、その再生順が動画を持てばそれ、そうでなければ動画を直接持ち見る人に見せるグループの
// うち id のいちばん小さいものである（groupContext）。再生順の id はすべてを返し、詳細は window
// の範囲だけを読む。グループが大きくても、読む詳細と応答は再生順の本数に比例しない（issue 674）。
// どのグループにも無い動画と、この動画自身を見せられないときは false を返す。
func (s *LibraryStore) VideoGroup(ctx context.Context, audience domain.Audience, id, contextID int64, window domain.GroupWindow) (domain.VideoGroup, bool, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.VideoGroup{}, false, fmt.Errorf("cannot start reading the video's group: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	groupID, found, err := groupContext(ctx, tx, audience, id, contextID)
	if err != nil || !found {
		return domain.VideoGroup{}, false, err
	}
	var path, name string
	if err := tx.QueryRowContext(ctx, `select coalesce(origin_folder, ''), name from groups where id = ?`,
		groupID).Scan(&path, &name); err != nil {
		return domain.VideoGroup{}, false, fmt.Errorf("cannot read the video's group: %w", err)
	}
	memberIDs, err := groupPlayOrderIDs(ctx, tx, audience, groupID)
	if err != nil {
		return domain.VideoGroup{}, false, err
	}
	group := domain.VideoGroup{ID: groupID, Name: name, MemberIDs: memberIDs}
	position := group.Position(id)
	if position == 0 {
		return domain.VideoGroup{}, false, nil
	}
	if window.Limit > 0 {
		group.Offset = window.Start(len(memberIDs), position)
		end := min(group.Offset+window.Limit, len(memberIDs))
		group.Members, err = groupMembers(ctx, tx, audience, memberIDs[group.Offset:end])
		if err != nil {
			return domain.VideoGroup{}, false, err
		}
	}
	if path != "" {
		roots, err := listMediaFolders(ctx, tx)
		if err != nil {
			return domain.VideoGroup{}, false, err
		}
		if folder, located := domain.LocateFolder(roots, path); located {
			group.Folder = &folder
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.VideoGroup{}, false, fmt.Errorf("cannot finish reading the video's group: %w", err)
	}
	return group, true, nil
}

// groupContext は動画 id の文脈のグループの id を返す（research.md R-9）。contextID が 0 でなく、
// そのグループの見る人に見せる再生順が動画を持てばそれを返す。そうでなければ、動画を直接の項目に
// 持つグループのうち id のいちばん小さいものを返す。直接の項目が引く動画が id そのものであることを
// 確かめるので、そのグループは見る人に見せる。どちらも無ければ false である。
func groupContext(ctx context.Context, tx *sql.Tx, audience domain.Audience, id, contextID int64) (int64, bool, error) {
	if contextID > 0 {
		var held bool
		err := tx.QueryRowContext(ctx, `with `+groupDescendantsCTE(`select ?1 as id`, audience)+`
			select exists (select 1 from gdesc where video_id = ?2)`, contextID, id).Scan(&held)
		if err != nil {
			return 0, false, fmt.Errorf("cannot read the video's group: %w", err)
		}
		if held {
			return contextID, true, nil
		}
	}
	var groupID sql.NullInt64
	err := tx.QueryRowContext(ctx, `select min(gi.group_id) from group_items gi
		where gi.content_key = (select `+userKeyExpr("videos")+` from videos where videos.id = ?1 and videos.content_key <> '')
		and `+shownVideoForKeyExpr("gi.content_key", audience)+` = ?1`, id).Scan(&groupID)
	if err != nil {
		return 0, false, fmt.Errorf("cannot read the video's group: %w", err)
	}
	return groupID.Int64, groupID.Valid, nil
}

// groupPlayOrderIDs はグループ id の見る人に見せる動画の id を再生順で返す（data-model.md
// 「Play order」。loadGroups と同じ with 句から読む）。
func groupPlayOrderIDs(ctx context.Context, tx *sql.Tx, audience domain.Audience, groupID int64) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `with `+groupDescendantsCTE(`select ? as id`, audience)+`
		select video_id from gdesc order by sort_key`, groupID)
	if err != nil {
		return nil, fmt.Errorf("cannot read the group play order: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cannot read the group play order: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the group play order: %w", err)
	}
	return ids, nil
}

// groupMembers はメンバーの詳細を ids の並びで返す。ids は同じ取引の groupPlayOrderIDs から
// 取ったものなので、どれも見せてよい。
func groupMembers(ctx context.Context, tx *sql.Tx, audience domain.Audience, ids []int64) ([]domain.Video, error) {
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build group member ids: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `select `+videoColumns(audience)+` from videos`+representativeJoin(audience)+`
		where videos.id in (select value from json_each(?))`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read group members: %w", err)
	}
	defer func() { _ = rows.Close() }()
	byID := make(map[int64]domain.Video, len(ids))
	for rows.Next() {
		video, err := scanVideo(rows)
		if err != nil {
			return nil, fmt.Errorf("cannot read group members: %w", err)
		}
		byID[video.ID] = video
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read group members: %w", err)
	}
	members := make([]domain.Video, 0, len(ids))
	for _, id := range ids {
		if video, ok := byID[id]; ok {
			members = append(members, video)
		}
	}
	return members, nil
}
