package domain

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
)

// オンボード（specs/057-home/research.md R-3、R-5、R-6）。進み具合は settings の onboarding.step に
// 段階 1 つで持ち、分野の一覧 InterestCatalog はコードで持つ（表にしない）。

// OnboardingStep はオンボードの段階である（data-model.md「Settings」）。
type OnboardingStep string

const (
	// OnboardingFolders はメディアフォルダを足す段階である。
	OnboardingFolders OnboardingStep = "folders"
	// OnboardingScan は最初のスキャンを始める段階である。
	OnboardingScan OnboardingStep = "scan"
	// OnboardingInterests は分野を選ぶ段階である。
	OnboardingInterests OnboardingStep = "interests"
	// OnboardingDone はオンボードを終えたことを表す。
	OnboardingDone OnboardingStep = "done"
)

// Valid は既知の段階かを返す。
func (s OnboardingStep) Valid() bool {
	switch s {
	case OnboardingFolders, OnboardingScan, OnboardingInterests, OnboardingDone:
		return true
	}
	return false
}

// DefaultOnboardingStep は settings に段階が無いときの段階である（research.md R-3）。メディアフォルダが
// あれば OnboardingDone（ライブラリのある既存の環境を更新でオンボードに戻さない）、無ければ
// OnboardingFolders である。
func DefaultOnboardingStep(hasMediaFolder bool) OnboardingStep {
	if hasMediaFolder {
		return OnboardingDone
	}
	return OnboardingFolders
}

// Interest は分野の一覧の 1 件である（data-model.md「Interest catalog」）。
type Interest struct {
	// ID は変わらない識別子である。
	ID string
	// Name はタグの名前（英語）、Synonym はタグの同義語（日本語）である。
	Name    string
	Synonym string
}

// InterestTagKind は選んだ分野のタグに付ける種類の名前である（research.md R-5）。051 が入れた
// プリセットの種類で、所有者が消していれば作り直す。
const InterestTagKind = "Genre"

// interestCatalogJSON は分野の一覧の正本である（data-model.md「Interest catalog」）。同義語は日本語で、
// サーバーの出力の文言ではなく利用者のデータなので、Go の文字列リテラルにせず埋め込みのファイルに置く。
//
//go:embed interest_catalog.json
var interestCatalogJSON []byte

// InterestCatalog は分野の一覧で、画面はこの順に並べる（data-model.md「Interest catalog」）。
var InterestCatalog = mustInterestCatalog(interestCatalogJSON)

// mustInterestCatalog は埋め込みの一覧を読む。読めなければプログラムの誤りなので panic する。
func mustInterestCatalog(data []byte) []Interest {
	var entries []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Synonym string `json:"synonym"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		panic(fmt.Sprintf("the built-in interest catalog is broken: %v", err))
	}
	catalog := make([]Interest, len(entries))
	for i, entry := range entries {
		catalog[i] = Interest{ID: entry.ID, Name: entry.Name, Synonym: entry.Synonym}
	}
	return catalog
}

// ErrInvalidInterestPick は分野の選び方が規則に合わないこと（id が無い、知らない id、同じ id を
// 2 度）を表す。API では 400 invalid_request である（contracts/screen-api.md「Errors」）。
var ErrInvalidInterestPick = errors.New("invalid interest pick")

// ErrHomeShelvesFull は棚を足すとホームが MaxHomeShelves を超えることを表す。API では
// 409 conflict である。
var ErrHomeShelvesFull = errors.New("the home has no room for more shelves")

// PickInterests は選んだ分野の id を一覧の分野にして、選んだ順に返す。id が無いか、知らない id か、
// 同じ id が 2 度あれば ErrInvalidInterestPick を包む誤りである。
func PickInterests(ids []string) ([]Interest, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("%w: no interest", ErrInvalidInterestPick)
	}
	picked := make([]Interest, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return nil, fmt.Errorf("%w: interest %q twice", ErrInvalidInterestPick, id)
		}
		seen[id] = true
		interest, ok := interestByID(id)
		if !ok {
			return nil, fmt.Errorf("%w: unknown interest %q", ErrInvalidInterestPick, id)
		}
		picked = append(picked, interest)
	}
	return picked, nil
}

// interestByID は一覧から id の分野を引く。
func interestByID(id string) (Interest, bool) {
	for _, interest := range InterestCatalog {
		if interest.ID == id {
			return interest, true
		}
	}
	return Interest{}, false
}

// InterestStatus は分野 1 件と、その分野のタグの条件の棚がもうホームにあるかである（GET
// /api/onboarding/interests）。
type InterestStatus struct {
	Interest
	OnHome bool
}

// InterestPickResult は分野を選んだ結果である（POST /api/onboarding/interests）。
type InterestPickResult struct {
	// Shelves は足した棚で、もうホームにあった分野の分は含まない。
	Shelves []SmartFolder
	// Queued は自動タグ付けに積んだ動画の件数で、自動タグ付けが切なら 0 である。
	Queued int
}
