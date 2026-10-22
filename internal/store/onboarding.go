package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const onboardingStepKey = "onboarding.step"

func (s *OnboardingStore) OnboardingStep(ctx context.Context) (domain.OnboardingStep, error) {
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

func (s *OnboardingStore) SaveOnboardingStep(ctx context.Context, step domain.OnboardingStep) error {
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
		return []domain.Event{domain.OnboardingChanged{}}, nil
	})
}

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

func (s *OnboardingStore) Interests(ctx context.Context) ([]domain.InterestStatus, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot read the interests: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	folders, err := onboardingFolders(ctx, tx)
	if err != nil {
		return nil, err
	}
	statuses := make([]domain.InterestStatus, 0, len(domain.InterestCatalog))
	for _, interest := range domain.InterestCatalog {
		lookup, found, err := lookupInterestTag(ctx, tx, interest)
		if err != nil {
			return nil, err
		}
		hasFolder := found && sameRuleAs(folders, interestRule(lookup.tagID))
		statuses = append(statuses, domain.InterestStatus{Interest: interest, HasFolder: hasFolder})
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cannot read the interests: %w", err)
	}
	return statuses, nil
}

func (s *OnboardingStore) PickInterests(ctx context.Context, interests []domain.Interest) ([]domain.SmartFolder, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot pick the interests: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	pick := interestPick{tx: tx}
	if pick.folders, err = onboardingFolders(ctx, tx); err != nil {
		return nil, err
	}
	for _, interest := range interests {
		if err := pick.add(ctx, interest); err != nil {
			return nil, err
		}
	}
	added := make([]domain.SmartFolder, 0, len(pick.inserted))
	for _, id := range pick.inserted {
		folder, err := readSmartFolder(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		added = append(added, folder)
	}
	if !pick.changed {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("cannot pick the interests: %w", err)
		}
		return added, nil
	}
	if err := s.db.commitMatchChange(ctx, tx); err != nil {
		return nil, fmt.Errorf("cannot pick the interests: %w", err)
	}
	return added, nil
}

type interestPick struct {
	tx *sql.Tx

	folders []domain.SmartFolder

	inserted []int64
	changed  bool

	genre int64
}

func (p *interestPick) add(ctx context.Context, interest domain.Interest) error {
	tagID, err := p.tag(ctx, interest)
	if err != nil {
		return err
	}
	rule := interestRule(tagID)
	if i := slices.IndexFunc(p.folders, func(folder domain.SmartFolder) bool {
		return domain.SameSmartRule(folder.Rule, rule)
	}); i >= 0 {
		return nil
	}
	folder := domain.SmartFolder{Name: interest.Name, Rule: rule, Sort: domain.SortAddedDesc}
	encoded, err := checkSmartFolder(&folder)
	if err != nil {
		return err
	}
	id, err := insertSmartFolder(ctx, p.tx, folder, encoded)
	if err != nil {
		return err
	}
	p.folders = append(p.folders, domain.SmartFolder{ID: id, Rule: rule, Name: folder.Name})
	p.inserted = append(p.inserted, id)
	p.changed = true
	return nil
}

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

func ensureTagKind(ctx context.Context, tx *sql.Tx, name string) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `select id from tag_kinds where name = ?`, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("cannot read the tag kind: %w", err)
	}
	// The interest kind holds genres, which cards list first.
	result, err := tx.ExecContext(ctx, `insert into tag_kinds (name, card_first, created_at) values (?, 1, ?)`, name, time.Now().Unix())
	if err != nil {
		return 0, fmt.Errorf("cannot create the tag kind: %w", err)
	}
	if id, err = result.LastInsertId(); err != nil {
		return 0, fmt.Errorf("cannot create the tag kind: %w", err)
	}
	return id, nil
}

func lookupInterestTag(ctx context.Context, tx *sql.Tx, interest domain.Interest) (nameLookup, bool, error) {
	for _, name := range []string{interest.Name, interest.Synonym} {
		lookup, found, err := lookupTagName(ctx, tx, name)
		if err != nil || found {
			return lookup, found, err
		}
	}
	return nameLookup{}, false, nil
}

func interestRule(tagID int64) domain.SmartRule {
	return domain.SmartRule{Tags: []int64{tagID}}
}

// onboardingFolders reads ordinary root smart folders for interest deduplication.
func onboardingFolders(ctx context.Context, tx *sql.Tx) ([]domain.SmartFolder, error) {
	rows, err := tx.QueryContext(ctx, `select id from smart_folders where parent_id is null order by id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	if err != nil {
		return nil, err
	}
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
func sameRuleAs(folders []domain.SmartFolder, rule domain.SmartRule) bool {
	return slices.ContainsFunc(folders, func(folder domain.SmartFolder) bool { return domain.SameSmartRule(folder.Rule, rule) })
}
