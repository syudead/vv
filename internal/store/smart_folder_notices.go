package store

// スマートフォルダの知らせ（specs/052-smart-folders/data-model.md「Notices」、research.md R-12）。
// 一致が変わりうる書き込みは、確定の前に同じ取引ですべてのフォルダの id を読み、確定後に
// domain.SmartFoldersChanged を 1 つ発行する。フォルダが 1 つも無ければ何も発行しない。どの
// フォルダの一致が変わったかは求めない（求めると書き込みごとにすべての条件を前後で評価する、R-2）。

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// smartFoldersChangedEvents はフォルダの id を、確定後に発行する変化にする。空なら何も返さない。
func smartFoldersChangedEvents(ids []int64) []domain.Event {
	if len(ids) == 0 {
		return nil
	}
	return []domain.Event{domain.SmartFoldersChanged{IDs: normalizeIDs(ids)}}
}

// allSmartFolderIDs はすべてのスマートフォルダの id を小さい順に読む。
func allSmartFolderIDs(ctx context.Context, tx *sql.Tx) ([]int64, error) {
	ids, err := queryInt64s(ctx, tx, `select id from smart_folders order by id`)
	if err != nil {
		return nil, fmt.Errorf("cannot read the smart folders: %w", err)
	}
	return ids, nil
}

// changesMatches は、知らせのうちスマートフォルダの一致を変えうるものがあるかを返す
// （data-model.md「Notices」）。グループの変化、束ねと外し（後継の引き継ぎを含む）、自動タグ付けの
// 結果、動画の作品情報（人物・団体の付け外し）がそれにあたる。
func changesMatches(events []domain.Event) bool {
	for _, event := range events {
		switch event.(type) {
		case domain.GroupsChanged, domain.VideoBundleChanged, domain.AutoTagApplied, domain.VideoWorkInfoChanged:
			return true
		}
	}
	return false
}

// withEveryFolderNotice は events に、すべてのフォルダを名指す知らせを 1 つ足す。すべてを名指すので、
// events が既に持つスマートフォルダの知らせは除く。フォルダが無ければ events をそのまま返す。
func withEveryFolderNotice(ctx context.Context, tx *sql.Tx, events []domain.Event) ([]domain.Event, error) {
	ids, err := allSmartFolderIDs(ctx, tx)
	if err != nil || len(ids) == 0 {
		return events, err
	}
	kept := make([]domain.Event, 0, len(events)+1)
	for _, event := range events {
		if _, ok := event.(domain.SmartFoldersChanged); !ok {
			kept = append(kept, event)
		}
	}
	return append(kept, smartFoldersChangedEvents(ids)...), nil
}

// withMatchNotice は、events に一致を変えうる知らせがあれば（changesMatches）すべてのフォルダを
// 名指す知らせを足す。
func withMatchNotice(ctx context.Context, tx *sql.Tx, events []domain.Event) ([]domain.Event, error) {
	if !changesMatches(events) {
		return events, nil
	}
	return withEveryFolderNotice(ctx, tx, events)
}

// commitMatchChange は、一致を変えうる書き込みの取引 tx を確定し、確定できたときだけ events と、
// すべてのフォルダを名指す知らせを発行する。フォルダの id は確定の前に同じ取引で読む。
func (db *DB) commitMatchChange(ctx context.Context, tx *sql.Tx, events ...domain.Event) error {
	events, err := withEveryFolderNotice(ctx, tx, events)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	db.publishEvents(events...)
	return nil
}

// commitEvents は取引 tx を確定し、確定できたときだけ events を発行する。events に一致を変えうる
// 知らせがあれば、すべてのフォルダを名指す知らせも発行する（withMatchNotice）。
func (db *DB) commitEvents(ctx context.Context, tx *sql.Tx, events ...domain.Event) error {
	events, err := withMatchNotice(ctx, tx, events)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	db.publishEvents(events...)
	return nil
}

// smartFolderEditNotice は、フォルダの編集で名指すフォルダの知らせを返す。ids のフォルダとその
// 子孫と、parents（親。0 は根で除く）を名指す。子孫は取引 tx の今の行から読むので、消す前に呼ぶ。
func smartFolderEditNotice(ctx context.Context, tx *sql.Tx, ids []int64, parents ...int64) ([]domain.Event, error) {
	named, err := smartFolderSubtrees(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	for _, parent := range parents {
		if parent != 0 {
			named = append(named, parent)
		}
	}
	return smartFoldersChangedEvents(named), nil
}

// smartFolderSubtrees は ids のフォルダとその子孫の id を返す。ids のうち今無いフォルダもそのまま返す。
func smartFolderSubtrees(ctx context.Context, tx *sql.Tx, ids []int64) ([]int64, error) {
	named := append([]int64(nil), ids...)
	for _, id := range ids {
		descendants, err := queryInt64s(ctx, tx, `with recursive down (id) as (
				select id from smart_folders where parent_id = ?
				union
				select f.id from smart_folders f join down on f.parent_id = down.id)
			select id from down`, id)
		if err != nil {
			return nil, fmt.Errorf("cannot read the child smart folders: %w", err)
		}
		named = append(named, descendants...)
	}
	return named, nil
}
