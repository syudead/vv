package store

// ホームの棚（specs/057-home/data-model.md、research.md R-7、R-8、R-16）。棚はホームの位置
// （home_position）を持つスマートフォルダで、位置の小さい順、同じなら id の順に並ぶ。HomeStore は
// 棚の一覧、所有者の棚の並びの書き込み、整理役の案の適用と、整理役の settings のキーを受け持つ。
// フォルダの挿入と削除は SmartFolderStore と同じ非公開の関数（insertSmartFolder・deleteSmartFolder）で
// 行う。棚の集合か並びか名前が変わった取引だけが、確定後に domain.HomeChanged を 1 つ発行する。

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// 整理役の settings のキー（data-model.md「Settings」）。
const (
	homeOrganizePendingKey = "home.organize.pending"
	homeOrganizeLastKey    = "home.organize.last"
)

// Shelves はホームの棚のフォルダを並び（home_position、次に id）の順に返す。
func (s *HomeStore) Shelves(ctx context.Context) ([]domain.SmartFolder, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot read the home shelves: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	shelves, err := homeShelves(ctx, tx)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cannot read the home shelves: %w", err)
	}
	return shelves, nil
}

// SetShelves はホームの棚を ids のフォルダにし、上から順に位置を 1, 2, 3… と振り直す（PUT
// /api/home/shelves、research.md R-7）。棚から外したフォルダと、足したか動かしたフォルダ
// （domain.MovedShelves）の owner_kept を 1 にする。ids が規則に合わなければ
// domain.ErrInvalidHomeShelves、名指すフォルダが無ければ domain.ErrNotFound で、どちらも何も書かない。
// 並びが変わらなければ何も書かず、何も発行しない。
func (s *HomeStore) SetShelves(ctx context.Context, ids []int64) error {
	if err := domain.ValidateHomeShelfOrder(ids); err != nil {
		return err
	}
	return writeWithEvents(ctx, s.db, "set the home shelves", func(tx *sql.Tx) ([]domain.Event, error) {
		for _, id := range ids {
			if _, err := readSmartFolder(ctx, tx, id); err != nil {
				return nil, err
			}
		}
		current, err := homeShelves(ctx, tx)
		if err != nil {
			return nil, err
		}
		before := shelfIDs(current)
		if slices.Equal(before, ids) {
			return nil, nil
		}
		for _, id := range before {
			if !slices.Contains(ids, id) {
				if _, err := tx.ExecContext(ctx, `update smart_folders set home_position = null, owner_kept = 1 where id = ?`,
					id); err != nil {
					return nil, fmt.Errorf("cannot remove the home shelf: %w", err)
				}
			}
		}
		if err := writeShelfPositions(ctx, tx, ids); err != nil {
			return nil, err
		}
		for _, id := range domain.MovedShelves(before, ids) {
			if _, err := tx.ExecContext(ctx, `update smart_folders set owner_kept = 1 where id = ?`, id); err != nil {
				return nil, fmt.Errorf("cannot keep the home shelf: %w", err)
			}
		}
		return []domain.Event{domain.HomeChanged{}}, nil
	})
}

// ApplyProposal は整理役の案 proposal を 1 つの取引で適用する（research.md R-8）。どちらの実行でも、
// domain.ProposedShelf.Validate に通らない案、見せる動画に 1 本も一致しない案、前の案と同じ条件の
// 案は捨て、棚は domain.MaxHomeShelves を超えて足さない。
//
// rebuild は、案が条件を繰り返す owner_kept 0 のフォルダを同じ id のまま案の名前で残し、owner_kept 1
// の棚と同じ条件の案を捨て、残りを挿入し、案が繰り返さなかった owner_kept 0 のフォルダを消し（子孫に
// owner_kept 1 のフォルダがあれば消さずにホームから外し）、owner_kept 1 の棚を今の順で上に、整理役の
// 棚を案の順でその後ろに置いて位置を振る。extend は、
// どれかの棚と同じ条件の案を捨て、今の棚の後ろへ挿入だけをし、名前も条件も変えない。
func (s *HomeStore) ApplyProposal(ctx context.Context, run domain.HomeOrganizeRun, proposal []domain.ProposedShelf) (domain.HomeApplied, error) {
	if !run.Valid() {
		return domain.HomeApplied{}, fmt.Errorf("unknown home organize run %q", run)
	}
	var applied domain.HomeApplied
	err := writeWithEvents(ctx, s.db, "apply the home proposal", func(tx *sql.Tx) ([]domain.Event, error) {
		shelves, err := usableProposals(ctx, tx, proposal)
		if err != nil {
			return nil, err
		}
		apply := homeApply{tx: tx}
		if err := apply.load(ctx); err != nil {
			return nil, err
		}
		if run == domain.HomeRebuild {
			err = apply.rebuild(ctx, shelves)
		} else {
			err = apply.extend(ctx, shelves)
		}
		if err != nil {
			return nil, err
		}
		applied = apply.result()
		return apply.events(ctx)
	})
	if err != nil {
		return domain.HomeApplied{}, err
	}
	return applied, nil
}

// PendingOrganize は待っている整理役の実行を返す。無ければ found は偽である。知らない値は無いものと
// して扱う。
func (s *HomeStore) PendingOrganize(ctx context.Context) (run domain.HomeOrganizeRun, found bool, err error) {
	value, found, err := readSetting(ctx, s.db.sql, homeOrganizePendingKey)
	if err != nil || !found {
		return "", false, err
	}
	run = domain.HomeOrganizeRun(value)
	if !run.Valid() {
		return "", false, nil
	}
	return run, true, nil
}

// SavePendingOrganize は待っている実行を run にする。
func (s *HomeStore) SavePendingOrganize(ctx context.Context, run domain.HomeOrganizeRun) error {
	if !run.Valid() {
		return fmt.Errorf("unknown home organize run %q", run)
	}
	return writeSetting(ctx, s.db.sql, homeOrganizePendingKey, string(run))
}

// ClearPendingOrganize は待っている実行を消す。
func (s *HomeStore) ClearPendingOrganize(ctx context.Context) error {
	if _, err := s.db.sql.ExecContext(ctx, `delete from settings where key = ?`, homeOrganizePendingKey); err != nil {
		return fmt.Errorf("cannot clear setting %s: %w", homeOrganizePendingKey, err)
	}
	return nil
}

// LastOrganize は最後に終わった実行を返す。無いか読めない値なら found は偽である。
func (s *HomeStore) LastOrganize(ctx context.Context) (last domain.HomeOrganizeLast, found bool, err error) {
	value, found, err := readSetting(ctx, s.db.sql, homeOrganizeLastKey)
	if err != nil || !found {
		return domain.HomeOrganizeLast{}, false, err
	}
	if json.Unmarshal([]byte(value), &last) != nil || last.Validate() != nil {
		return domain.HomeOrganizeLast{}, false, nil
	}
	return last, true, nil
}

// SaveLastOrganize は最後に終わった実行を last にする。
func (s *HomeStore) SaveLastOrganize(ctx context.Context, last domain.HomeOrganizeLast) error {
	if err := last.Validate(); err != nil {
		return err
	}
	encoded, err := json.Marshal(last)
	if err != nil {
		return fmt.Errorf("cannot encode the home organize run: %w", err)
	}
	return writeSetting(ctx, s.db.sql, homeOrganizeLastKey, string(encoded))
}

// homeShelves は取引 tx で棚のフォルダを並びの順に読む。
func homeShelves(ctx context.Context, tx *sql.Tx) ([]domain.SmartFolder, error) {
	ids, err := queryInt64s(ctx, tx, `select id from smart_folders where home_position is not null
		order by home_position, id`)
	if err != nil {
		return nil, fmt.Errorf("cannot read the home shelves: %w", err)
	}
	return readSmartFolders(ctx, tx, ids)
}

// readSmartFolders は取引 tx でフォルダ ids の行を ids の順に読む。
func readSmartFolders(ctx context.Context, tx *sql.Tx, ids []int64) ([]domain.SmartFolder, error) {
	folders := make([]domain.SmartFolder, 0, len(ids))
	for _, id := range ids {
		folder, err := readSmartFolder(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		folders = append(folders, folder)
	}
	return folders, nil
}

// shelfIDs はフォルダの id を順に返す。
func shelfIDs(folders []domain.SmartFolder) []int64 {
	ids := make([]int64, len(folders))
	for i, folder := range folders {
		ids[i] = folder.ID
	}
	return ids
}

// writeShelfPositions はフォルダ ids の位置を上から 1, 2, 3… にする。
func writeShelfPositions(ctx context.Context, tx *sql.Tx, ids []int64) error {
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `update smart_folders set home_position = ? where id = ?`, i+1, id); err != nil {
			return fmt.Errorf("cannot set the home shelf position: %w", err)
		}
	}
	return nil
}

// usableProposals は案のうち、規則に合い、見せる動画に 1 本でも一致し、前の案と条件が重ならないものを
// 名前を整えて順に返す（data-model.md「Rules」）。
func usableProposals(ctx context.Context, tx *sql.Tx, proposal []domain.ProposedShelf) ([]domain.ProposedShelf, error) {
	now := listNow().Unix()
	usable := make([]domain.ProposedShelf, 0, len(proposal))
	for _, shelf := range proposal {
		shelf, err := shelf.Validate()
		if err != nil {
			continue
		}
		if slices.ContainsFunc(usable, func(earlier domain.ProposedShelf) bool {
			return domain.SameSmartRule(earlier.Rule, shelf.Rule)
		}) {
			continue
		}
		spec := smartFolderSpec(domain.AudienceOwner, []domain.SmartRule{shelf.Rule}, domain.SortAddedDesc, now)
		matches, err := smartFolderHasItems(ctx, tx, spec)
		if err != nil {
			return nil, err
		}
		if matches {
			usable = append(usable, shelf)
		}
	}
	return usable, nil
}

// homeApply は案の適用の取引 1 つの状態である。
type homeApply struct {
	tx *sql.Tx
	// before は適用の前の棚で、organizer は owner_kept 0 のフォルダ（id の順）である。
	before    []domain.SmartFolder
	organizer []domain.SmartFolder
	// after は適用の後の棚の id を並びの順に持つ。
	after []int64
	// inserted・renamed・deleted は挿入・改名・削除したフォルダの id で、unshelved は所有者の子孫を
	// 持つので消さずにホームから外した整理役のフォルダの id である。
	inserted, renamed, deleted, unshelved []int64
	// deleteEvents は削除が返した知らせである。
	deleteEvents []domain.Event
}

// load は今の棚と整理役のフォルダを読む。
func (a *homeApply) load(ctx context.Context) error {
	var err error
	if a.before, err = homeShelves(ctx, a.tx); err != nil {
		return err
	}
	ids, err := queryInt64s(ctx, a.tx, `select id from smart_folders where owner_kept = 0 order by id`)
	if err != nil {
		return fmt.Errorf("cannot read the organizer folders: %w", err)
	}
	a.organizer, err = readSmartFolders(ctx, a.tx, ids)
	return err
}

// rebuild は整理役のフォルダを組み直す。
func (a *homeApply) rebuild(ctx context.Context, proposal []domain.ProposedShelf) error {
	owner := make([]domain.SmartFolder, 0, len(a.before))
	for _, shelf := range a.before {
		if shelf.OwnerKept {
			owner = append(owner, shelf)
		}
	}
	a.after = shelfIDs(owner)
	kept := map[int64]bool{}
	for _, shelf := range proposal {
		if len(a.after) >= domain.MaxHomeShelves {
			break
		}
		if sameRuleAs(owner, shelf.Rule) {
			continue
		}
		id, err := a.keepOrInsert(ctx, shelf, kept)
		if err != nil {
			return err
		}
		kept[id] = true
		a.after = append(a.after, id)
	}
	for _, folder := range a.organizer {
		if kept[folder.ID] {
			continue
		}
		if err := a.drop(ctx, folder); err != nil {
			return err
		}
	}
	if !slices.Equal(shelfIDs(a.before), a.after) {
		return writeShelfPositions(ctx, a.tx, a.after)
	}
	return nil
}

// drop は案が繰り返さなかった整理役のフォルダ folder を消す。子孫に owner_kept 1 のフォルダがあれば、
// 消すと外部キーで所有者のフォルダまで消えるので（data-model.md「Rules」の「rebuild は owner_kept 0 の
// フォルダだけを消す」）、folder を消さずにホームから外すだけにする。
func (a *homeApply) drop(ctx context.Context, folder domain.SmartFolder) error {
	var owned bool
	if err := a.tx.QueryRowContext(ctx, `with recursive down (id) as (
			select id from smart_folders where parent_id = ?
			union
			select f.id from smart_folders f join down on f.parent_id = down.id)
		select exists (select 1 from smart_folders where id in (select id from down) and owner_kept = 1)`,
		folder.ID).Scan(&owned); err != nil {
		return fmt.Errorf("cannot read the organizer folder's descendants: %w", err)
	}
	if !owned {
		events, err := deleteSmartFolder(ctx, a.tx, folder.ID)
		if err != nil {
			return err
		}
		a.deleted = append(a.deleted, folder.ID)
		a.deleteEvents = append(a.deleteEvents, events...)
		return nil
	}
	if folder.HomePosition == 0 {
		return nil
	}
	if _, err := a.tx.ExecContext(ctx, `update smart_folders set home_position = null where id = ?`, folder.ID); err != nil {
		return fmt.Errorf("cannot remove the organizer shelf: %w", err)
	}
	a.unshelved = append(a.unshelved, folder.ID)
	return nil
}

// keepOrInsert は、案と同じ条件のまだ残していない整理役のフォルダがあれば案の名前にして残し、無ければ
// 案のフォルダを挿入し、その id を返す。
func (a *homeApply) keepOrInsert(ctx context.Context, shelf domain.ProposedShelf, kept map[int64]bool) (int64, error) {
	for _, folder := range a.organizer {
		if kept[folder.ID] || !domain.SameSmartRule(folder.Rule, shelf.Rule) {
			continue
		}
		if folder.Name != shelf.Name {
			// 所有者の編集ではないので owner_kept と updated_at は変えない。
			if _, err := a.tx.ExecContext(ctx, `update smart_folders set name = ?, title_key = ? where id = ?`,
				shelf.Name, domain.NaturalSortKey(shelf.Name), folder.ID); err != nil {
				return 0, fmt.Errorf("cannot rename the organizer folder: %w", err)
			}
			a.renamed = append(a.renamed, folder.ID)
		}
		return folder.ID, nil
	}
	return a.insert(ctx, shelf, 0)
}

// extend は今の棚の後ろへ棚を足す。
func (a *homeApply) extend(ctx context.Context, proposal []domain.ProposedShelf) error {
	a.after = shelfIDs(a.before)
	last := 0
	if len(a.before) > 0 {
		last = a.before[len(a.before)-1].HomePosition
	}
	for _, shelf := range proposal {
		if len(a.after) >= domain.MaxHomeShelves {
			break
		}
		if sameRuleAs(a.before, shelf.Rule) {
			continue
		}
		last++
		id, err := a.insert(ctx, shelf, last)
		if err != nil {
			return err
		}
		a.after = append(a.after, id)
	}
	return nil
}

// insert は案を整理役の根のフォルダとして位置 position（0 なら後で振る）に挿入する。
func (a *homeApply) insert(ctx context.Context, shelf domain.ProposedShelf, position int) (int64, error) {
	create := shelf.Folder()
	folder := domain.SmartFolder{Name: create.Name, Rule: create.Rule, Sort: create.Sort}
	rule, err := checkSmartFolder(&folder)
	if err != nil {
		return 0, err
	}
	id, err := insertSmartFolder(ctx, a.tx, folder, rule, false, position)
	if err != nil {
		return 0, err
	}
	a.inserted = append(a.inserted, id)
	return id, nil
}

// result は適用の結果を返す。
func (a *homeApply) result() domain.HomeApplied {
	return domain.HomeApplied{
		Added:   len(a.inserted),
		Removed: len(a.deleted) + len(a.unshelved),
		Changed: a.changed(),
	}
}

// changed は棚の集合・並び・名前のどれかが変わったかを返す。
func (a *homeApply) changed() bool {
	return len(a.inserted)+len(a.renamed)+len(a.deleted) > 0 || !slices.Equal(shelfIDs(a.before), a.after)
}

// events は確定後に発行する知らせを返す。変わったときだけ、挿入・改名・削除したフォルダ（子孫を含む）を
// 名指す domain.SmartFoldersChanged と domain.HomeChanged を 1 つずつ返す。
func (a *homeApply) events(ctx context.Context) ([]domain.Event, error) {
	if !a.changed() {
		return nil, nil
	}
	named, err := smartFolderSubtrees(ctx, a.tx, append(slices.Clone(a.inserted), a.renamed...))
	if err != nil {
		return nil, err
	}
	var events []domain.Event
	for _, event := range a.deleteEvents {
		switch event := event.(type) {
		case domain.SmartFoldersChanged:
			named = append(named, event.IDs...)
		case domain.HomeChanged:
		default:
			events = append(events, event)
		}
	}
	events = append(events, smartFoldersChangedEvents(named)...)
	return append(events, domain.HomeChanged{}), nil
}

// sameRuleAs は folders のどれかが rule と同じ条件かを返す。
func sameRuleAs(folders []domain.SmartFolder, rule domain.SmartRule) bool {
	return slices.ContainsFunc(folders, func(folder domain.SmartFolder) bool {
		return domain.SameSmartRule(folder.Rule, rule)
	})
}
