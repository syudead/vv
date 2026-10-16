package store

// オンボードの段階と分野（specs/057-home/research.md R-3、R-5、R-6、R-16）。段階は settings の
// onboarding.step に置き、書き込みは名前の段階をそのまま書く（後の書き込みが残る）。分野を選ぶと、
// 1 つの取引で分野のタグを探すか作り、種類の無いタグに Genre を付け、そのタグの条件の所有者の棚を
// 作る。タグの作成と同義語は tags.go・tag_lookup.go の関数を使う。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// onboardingStepKey はオンボードの段階の settings のキーである（data-model.md「Settings」）。
const onboardingStepKey = "onboarding.step"

// OnboardingStep はオンボードの段階を返す。settings に無いか知らない値なら
// domain.DefaultOnboardingStep（メディアフォルダがあれば done、無ければ folders）である。
func (s *HomeStore) OnboardingStep(ctx context.Context) (domain.OnboardingStep, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return "", fmt.Errorf("cannot read the onboarding step: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	step, err := onboardingStep(ctx, tx)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("cannot read the onboarding step: %w", err)
	}
	return step, nil
}

// SaveOnboardingStep は段階を step にする。知らない段階なら何も書かない誤りである。段階が変わったとき
// だけ、確定後に domain.HomeChanged を発行する（research.md R-16）。
func (s *HomeStore) SaveOnboardingStep(ctx context.Context, step domain.OnboardingStep) error {
	if !step.Valid() {
		return fmt.Errorf("unknown onboarding step %q", step)
	}
	return writeWithEvents(ctx, s.db, "save the onboarding step", func(tx *sql.Tx) ([]domain.Event, error) {
		before, err := onboardingStep(ctx, tx)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
			insert into settings (key, value, updated_at) values (?, ?, ?)
			on conflict (key) do update set value = excluded.value, updated_at = excluded.updated_at`,
			onboardingStepKey, string(step), time.Now().Unix()); err != nil {
			return nil, fmt.Errorf("cannot save setting %s: %w", onboardingStepKey, err)
		}
		if before == step {
			return nil, nil
		}
		return []domain.Event{domain.HomeChanged{}}, nil
	})
}

// onboardingStep は取引 tx で段階を読む。
func onboardingStep(ctx context.Context, tx *sql.Tx) (domain.OnboardingStep, error) {
	var value string
	err := tx.QueryRowContext(ctx, `select value from settings where key = ?`, onboardingStepKey).Scan(&value)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("cannot read setting %s: %w", onboardingStepKey, err)
	}
	if step := domain.OnboardingStep(value); err == nil && step.Valid() {
		return step, nil
	}
	var hasFolder bool
	if err := tx.QueryRowContext(ctx, `select exists (select 1 from media_folders)`).Scan(&hasFolder); err != nil {
		return "", fmt.Errorf("cannot read media folders: %w", err)
	}
	return domain.DefaultOnboardingStep(hasFolder), nil
}

// Interests は分野の一覧を一覧の順に、それぞれの分野のタグの条件の棚がもうホームにあるかを添えて返す。
func (s *HomeStore) Interests(ctx context.Context) ([]domain.InterestStatus, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot read the interests: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	shelves, err := homeShelves(ctx, tx)
	if err != nil {
		return nil, err
	}
	statuses := make([]domain.InterestStatus, 0, len(domain.InterestCatalog))
	for _, interest := range domain.InterestCatalog {
		lookup, found, err := lookupInterestTag(ctx, tx, interest)
		if err != nil {
			return nil, err
		}
		onHome := found && sameRuleAs(shelves, interestRule(lookup.tagID))
		statuses = append(statuses, domain.InterestStatus{Interest: interest, OnHome: onHome})
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cannot read the interests: %w", err)
	}
	return statuses, nil
}

// PickInterests は選んだ分野 interests を 1 つの取引でタグと所有者の棚にし、足した棚を返す
// （research.md R-6、data-model.md「Rules」）。分野ごとに、名前か同義語でタグを探し、無ければ同義語
// 付きで作る。種類の無いタグには domain.InterestTagKind を付け（その種類が無ければ作り直し）、今の
// 種類は変えない。仮のタグは確定する。そのタグの条件の棚がまだホームに無ければ、owner_kept 1 の根の
// フォルダを今の棚の後ろに作る。棚が domain.MaxHomeShelves を超えるなら domain.ErrHomeShelvesFull で、
// 何も書かない。同じ条件の棚が整理役の棚なら、足さずに所有者の棚（owner_kept 1）にする。確定後、
// 何か変えたときだけ、すべてのフォルダを名指す domain.SmartFoldersChanged と、棚を足すか所有者の棚に
// したときは domain.HomeChanged を発行する。
func (s *HomeStore) PickInterests(ctx context.Context, interests []domain.Interest) ([]domain.SmartFolder, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot pick the interests: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	pick := interestPick{tx: tx}
	if pick.shelves, err = homeShelves(ctx, tx); err != nil {
		return nil, err
	}
	for _, interest := range interests {
		if err := pick.add(ctx, interest); err != nil {
			return nil, err
		}
	}
	if len(pick.shelves) > domain.MaxHomeShelves {
		return nil, fmt.Errorf("%w: %d shelves", domain.ErrHomeShelvesFull, len(pick.shelves))
	}
	added, err := readSmartFolders(ctx, tx, pick.inserted)
	if err != nil {
		return nil, err
	}
	if !pick.changed {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("cannot pick the interests: %w", err)
		}
		return added, nil
	}
	var events []domain.Event
	if len(pick.inserted) > 0 || pick.kept {
		events = append(events, domain.HomeChanged{})
	}
	if err := s.db.commitMatchChange(ctx, tx, events...); err != nil {
		return nil, fmt.Errorf("cannot pick the interests: %w", err)
	}
	return added, nil
}

// interestPick は分野を選ぶ取引 1 つの状態である。
type interestPick struct {
	tx *sql.Tx
	// shelves は今の棚に、足した棚を加えたものである。
	shelves []domain.SmartFolder
	// inserted は足した棚の id で、changed はタグか棚のどれかを書いたかである。
	inserted []int64
	changed  bool
	// kept は整理役の棚を所有者の棚にしたかである。
	kept bool
	// genre は domain.InterestTagKind の種類の id で、まだ引いていなければ 0 である。
	genre int64
}

// add は分野 1 つをタグと棚にする。
func (p *interestPick) add(ctx context.Context, interest domain.Interest) error {
	tagID, err := p.tag(ctx, interest)
	if err != nil {
		return err
	}
	rule := interestRule(tagID)
	if i := slices.IndexFunc(p.shelves, func(shelf domain.SmartFolder) bool {
		return domain.SameSmartRule(shelf.Rule, rule)
	}); i >= 0 {
		return p.keep(ctx, i)
	}
	position := 1
	if len(p.shelves) > 0 {
		position = p.shelves[len(p.shelves)-1].HomePosition + 1
	}
	id, err := insertShelf(ctx, p.tx, domain.ProposedShelf{Name: interest.Name, Rule: rule}, true, position)
	if err != nil {
		return err
	}
	p.shelves = append(p.shelves, domain.SmartFolder{ID: id, Rule: rule, HomePosition: position, OwnerKept: true})
	p.inserted = append(p.inserted, id)
	p.changed = true
	return nil
}

// keep は分野と同じ条件の棚 p.shelves[i] を所有者の棚にする。整理役の棚（owner_kept 0）のままだと、
// 動画が案の条件に満たなくなった再整理で消える（research.md R-6、R-8）。
func (p *interestPick) keep(ctx context.Context, i int) error {
	if p.shelves[i].OwnerKept {
		return nil
	}
	if _, err := p.tx.ExecContext(ctx, `update smart_folders set owner_kept = 1 where id = ?`, p.shelves[i].ID); err != nil {
		return fmt.Errorf("cannot keep the home shelf: %w", err)
	}
	p.shelves[i].OwnerKept = true
	p.changed = true
	p.kept = true
	return nil
}

// tag は分野のタグを探すか作り、種類と確定を整えて id を返す。
func (p *interestPick) tag(ctx context.Context, interest domain.Interest) (int64, error) {
	lookup, found, err := lookupInterestTag(ctx, p.tx, interest)
	if err != nil {
		return 0, err
	}
	if !found {
		id, err := insertTag(ctx, p.tx, interest.Name, false)
		if err != nil {
			return 0, err
		}
		if err := insertTagName(ctx, p.tx, interest.Synonym, id, false); err != nil {
			return 0, err
		}
		p.changed = true
		return id, p.setGenre(ctx, id)
	}
	if lookup.tentative {
		// 分野を選んだのは所有者なので、自動タグ付けの推量の仮のタグを確定する。
		if err := confirmTagInTx(ctx, p.tx, lookup.tagID); err != nil {
			return 0, err
		}
		p.changed = true
	}
	if lookup.kind == nil {
		p.changed = true
		return lookup.tagID, p.setGenre(ctx, lookup.tagID)
	}
	return lookup.tagID, nil
}

// setGenre はタグ id の種類を domain.InterestTagKind にする。その種類が無ければ作る。
func (p *interestPick) setGenre(ctx context.Context, id int64) error {
	if p.genre == 0 {
		genre, err := ensureTagKind(ctx, p.tx, domain.InterestTagKind)
		if err != nil {
			return err
		}
		p.genre = genre
	}
	if _, err := p.tx.ExecContext(ctx, `update tags set kind_id = ? where id = ?`, p.genre, id); err != nil {
		return fmt.Errorf("cannot set the kind of the tag: %w", err)
	}
	return nil
}

// ensureTagKind は名前 name の種類の id を返し、無ければ作る。
func ensureTagKind(ctx context.Context, tx *sql.Tx, name string) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `select id from tag_kinds where name = ?`, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("cannot read the tag kind: %w", err)
	}
	result, err := tx.ExecContext(ctx, `insert into tag_kinds (name, created_at) values (?, ?)`, name, time.Now().Unix())
	if err != nil {
		return 0, fmt.Errorf("cannot create the tag kind: %w", err)
	}
	if id, err = result.LastInsertId(); err != nil {
		return 0, fmt.Errorf("cannot create the tag kind: %w", err)
	}
	return id, nil
}

// lookupInterestTag は分野のタグを、名前、次に同義語で引く（どちらもタグの元の名前か同義語に当たる）。
func lookupInterestTag(ctx context.Context, tx *sql.Tx, interest domain.Interest) (nameLookup, bool, error) {
	for _, name := range []string{interest.Name, interest.Synonym} {
		lookup, found, err := lookupTagName(ctx, tx, name)
		if err != nil || found {
			return lookup, found, err
		}
	}
	return nameLookup{}, false, nil
}

// interestRule は分野のタグ tagID の棚の条件である。
func interestRule(tagID int64) domain.SmartRule {
	return domain.SmartRule{Tags: []int64{tagID}}
}
