package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 所有者の操作の記録（specs/055-activity/research.md R-6、data-model.md「Kinds」）。動画のタグと版の集まりを
// 変える操作、メディアフォルダの登録の解除、作品情報の取得と適用を、decider owner の done の記録として、
// 操作と同じ取引で書く（R-2）。所有者の記録はどれも done で書くので、同じ種類と対象の attention の記録は
// 無く、recordActivity の閉じる手順（supersedeActivity）が閉じるものも無い。動画ごとの記録は 1 文で
// まとめて書き、選んだ 20000 本へのタグの付与でも文の数を増やさない。

// ownerVideoChange は所有者の操作が変えた動画 1 本の記録である。key は内容の鍵か集まりの利用者データの鍵で、
// 集まりの鍵は代表の内容の鍵を対象にする。tagID が 0 でなければ、detail はそのタグの名前である。
type ownerVideoChange struct {
	Key    string `json:"k"`
	TagID  int64  `json:"t,omitempty"`
	Detail string `json:"d,omitempty"`
}

// ownerVideoChanges は鍵 keys のそれぞれを、同じ detail の変更にする。
func ownerVideoChanges(keys []string, detail string) []ownerVideoChange {
	out := make([]ownerVideoChange, 0, len(keys))
	for _, key := range keys {
		out = append(out, ownerVideoChange{Key: key, Detail: detail})
	}
	return out
}

// ownerTagChanges は鍵 keys のそれぞれの、タグ tagID の変更にする。
func ownerTagChanges(keys []string, tagID int64) []ownerVideoChange {
	out := make([]ownerVideoChange, 0, len(keys))
	for _, key := range keys {
		out = append(out, ownerVideoChange{Key: key, TagID: tagID})
	}
	return out
}

// recordOwnerVideoActivity は changes の各件を、種類 kind の動画の記録として changes の順に書く
// （insertOwnerVideoEntries）。
func recordOwnerVideoActivity(
	ctx context.Context, tx activityWriter, c *changes, kind domain.ActivityKind, changes []ownerVideoChange, now time.Time,
) error {
	if len(changes) == 0 {
		return nil
	}
	feature, ok := kind.Feature()
	if !ok {
		return fmt.Errorf("cannot record the owner's activity: unknown kind %q", kind)
	}
	encoded, err := json.Marshal(changes)
	if err != nil {
		return fmt.Errorf("cannot record the owner's activity: %w", err)
	}
	return insertOwnerVideoEntries(ctx, tx, c, feature, `select j.key, json_extract(j.value, '$.k'),
		json_extract(j.value, '$.t'), nullif(json_extract(j.value, '$.d'), ''), ? from json_each(?) j`,
		now, string(kind), string(encoded))
}

// insertOwnerVideoEntries は問い合わせ source（引数は args）の各行を、機能 feature の動画の記録として位置と
// タグの id の順に 1 文で書く。source の列は順に、並びの位置、内容の鍵か集まりの利用者データの鍵、タグの id（無ければ null）、
// detail、種類である。集まりの鍵は代表の内容の鍵を対象にし、タグの id があれば detail はそのタグの名前に
// する。状態は done/completed、判断したものは owner で、表示名は書いた時点のもの（contentLabelExpr）である。
// 書いた行があれば c に残し、確定後に domain.ActivityChanged を発行させる。
func insertOwnerVideoEntries(
	ctx context.Context, tx activityWriter, c *changes, feature domain.ActivityFeature, source string, now time.Time,
	args ...any,
) error {
	at := now.UnixMilli()
	// source は ? か ?NNN で引数を受けるので、この文の値は名前で渡して番号が重ならないようにする。
	args = append(args, sql.Named("feature", string(feature)), sql.Named("subject_kind", string(domain.ActivitySubjectVideo)),
		sql.Named("status", string(domain.ActivityDone)), sql.Named("outcome", string(domain.ActivityCompleted)),
		sql.Named("decider", string(domain.ActivityByOwner)), sql.Named("at", at))
	result, err := tx.ExecContext(ctx, `with o (position, user_key, tag_id, detail, kind) as (`+source+`)
		insert into activity_entries (kind, feature, subject_kind, subject_key, subject_label, status, outcome, detail,
			decider, created_at, updated_at)
		select s.kind, :feature, :subject_kind, s.subject_key, `+contentLabelExpr("s.subject_key")+`, :status, :outcome,
			s.detail, :decider, :at, :at
		from (select o.position, o.tag_id, o.kind,
				coalesce((select b.representative_key from video_bundles b where b.user_key = o.user_key), o.user_key)
					as subject_key,
				case when o.tag_id is not null
					then (select tn.name from tag_names tn where tn.tag_id = o.tag_id and tn.canonical = 1)
					else o.detail end as detail
			from o) s
		order by s.position, s.tag_id`, args...)
	if err != nil {
		return fmt.Errorf("cannot record the owner's activity: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("cannot record the owner's activity: %w", err)
	} else if affected > 0 {
		c.activityChanged()
	}
	return nil
}

// recordManualTagActivity は、鍵の集合 encodedKeys（JSON の配列）とタグの集合 encodedTags への action の書き込みで
// 手で付けたタグの行が変わる鍵とタグの組ごとに、付くなら tag_attach、外れるなら tag_detach の所有者の記録を
// 書く。書き込みの前に同じ取引で呼ぶ。組は SQLite の中で求めて 1 文で書くので、上限の一括操作でも Go へ
// 鍵 × タグの行を返さない（changedManualTagKeys と同じ条件）。add は付いていないタグ（keepSuggested が偽なら
// 確定する提案のタグも含む、unheldManualTag）、remove は集合のタグで付いているもの、replace はその add に加えて
// 集合に無いタグで付いているものである。
func recordManualTagActivity(
	ctx context.Context, tx *sql.Tx, c *changes, encodedKeys string, action domain.VideoTagsAction, encodedTags string,
	keepSuggested bool, now time.Time,
) error {
	missing := `select k.key, k.value, t.value, null, ?3 from json_each(?1) k cross join json_each(?2) t
		where ` + unheldManualTag(keepSuggested)
	const present = `select k.key, k.value, vt.tag_id, null, ?4 from json_each(?1) k
		join video_tags vt on vt.content_key = k.value where vt.tag_id in (select value from json_each(?2))`
	const extra = `select k.key, k.value, vt.tag_id, null, ?4 from json_each(?1) k
		join video_tags vt on vt.content_key = k.value where vt.tag_id not in (select value from json_each(?2))`
	var source string
	switch action {
	case domain.VideoTagsAdd:
		source = missing
	case domain.VideoTagsRemove:
		source = present
	case domain.VideoTagsReplace:
		source = missing + ` union all ` + extra
	default:
		return fmt.Errorf("unknown video tags action %q", action)
	}
	feature, _ := domain.ActivityTagAttach.Feature() // tag_attach と tag_detach は同じ機能である。
	return insertOwnerVideoEntries(ctx, tx, c, feature, source, now, encodedKeys, encodedTags,
		string(domain.ActivityTagAttach), string(domain.ActivityTagDetach))
}

// recordOwnerBundle は所有者が members を 1 つの集まりに束ねたことを記録する。その中の組の開いている
// version_judgement の記録を所有者が決めたとして閉じ、メンバーごとに bundle の記録を書く。
// VersionStore.Bundle と OrganizeStore.Bundle が共有する。
func recordOwnerBundle(ctx context.Context, tx *sql.Tx, c *changes, members []string) error {
	now := time.Now()
	if err := closePairActivities(ctx, tx, c, members, domain.ActivityDecidedByOwner, domain.ActivityByOwner, 0,
		now); err != nil {
		return err
	}
	return recordOwnerVideoActivity(ctx, tx, c, domain.ActivityBundle, ownerVideoChanges(members, ""), now)
}

// recordOwnerDismissal は所有者の「違う動画」の組 keys（key_a < key_b）の dismiss の記録を 1 件書く。
func recordOwnerDismissal(ctx context.Context, tx *sql.Tx, c *changes, keys [2]string, now time.Time) error {
	subject := domain.ActivitySubject{Kind: domain.ActivitySubjectPair, Key: keys[0], KeyB: keys[1]}
	var err error
	if subject.Label, err = contentLabel(ctx, tx, keys[0]); err != nil {
		return err
	}
	if subject.LabelB, err = contentLabel(ctx, tx, keys[1]); err != nil {
		return err
	}
	_, err = recordActivity(ctx, tx, c, ownerDoneEntry(domain.ActivityDismiss, subject, ""), now)
	return err
}

// recordMediaFolderRemoval はメディアフォルダ path の登録の解除の記録を 1 件書く。detail は外れた動画の数である。
func recordMediaFolderRemoval(ctx context.Context, tx *sql.Tx, c *changes, path string, removed int, now time.Time) error {
	subject := domain.ActivitySubject{Kind: domain.ActivitySubjectFolder, Key: path, Label: path}
	_, err := recordActivity(ctx, tx, c,
		ownerDoneEntry(domain.ActivityMediaFolderRemove, subject, strconv.Itoa(removed)), now)
	return err
}

// ownerDoneEntry は所有者が決めた done/completed の記録である。
func ownerDoneEntry(kind domain.ActivityKind, subject domain.ActivitySubject, detail string) domain.ActivityEntry {
	return domain.ActivityEntry{
		Kind: kind, Subject: subject, Status: domain.ActivityDone, Outcome: domain.ActivityCompleted,
		Detail: detail, Decider: domain.ActivityByOwner,
	}
}

// RecordWorkInfoFetch は動画 videoID の作品情報の取得 1 回を work_info_fetch の記録にする（research.md R-11）。
// failure が空なら done/completed、そうでなければ done/failed で、failure（プラグインの文）を detail に書く。
// やり直しは出さない。動画が無ければ domain.ErrNotFound。確定後に domain.ActivityChanged を発行する。
func (s *OverrideStore) RecordWorkInfoFetch(ctx context.Context, videoID int64, failure string) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot record the work-information fetch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	keys, err := registeredContentKeysForVideoIDs(ctx, tx, []int64{videoID})
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return domain.ErrNotFound
	}
	subject, err := videoActivitySubject(ctx, tx, videoID)
	if err != nil {
		return err
	}
	entry := ownerDoneEntry(domain.ActivityWorkInfoFetch, subject, failure)
	if failure != "" {
		entry.Outcome = domain.ActivityOutcomeFailed
	}
	c := &changes{}
	if _, err := recordActivity(ctx, tx, c, entry, time.Now()); err != nil {
		return err
	}
	if err := s.db.commit(ctx, tx, c); err != nil {
		return fmt.Errorf("cannot record the work-information fetch: %w", err)
	}
	return nil
}
