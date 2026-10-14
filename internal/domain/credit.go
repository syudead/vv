package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// 人物と団体（specs/047-video-work-info/data-model.md「Entities」、research.md R-3・R-8）。
// どちらも名前が重なってよい id の実体で、名前・別名・外部 ID を持ち、団体だけが親の団体を持つ。

// CreditKind は実体の種類である。保存層はこれで表の名前を決める。
type CreditKind string

const (
	// CreditPerson は人物（出演者・監督・撮影・配信者）。
	CreditPerson CreditKind = "person"
	// CreditOrganization は団体（メーカー・レーベル）。
	CreditOrganization CreditKind = "organization"
)

// Valid は k が既知の種類であることを返す。
func (k CreditKind) Valid() bool {
	return k == CreditPerson || k == CreditOrganization
}

const (
	// CreditNameMaxLength は実体の名前と別名の上限（符号位置数）である。表示名と同じ。
	CreditNameMaxLength = 200
	// CreditAliasesMaxCount は 1 つの実体の別名の件数の上限である。
	CreditAliasesMaxCount = 50
	// CreditLanguageMaxLength は別名の language の上限（文字数）である。
	CreditLanguageMaxLength = 35
)

// Alias は実体の別名 1 件である。Language は BCP 47 の構文の言語タグで、無ければ空。
type Alias struct {
	Name     string
	Language string
}

// Credit は人物か団体 1 件である。
type Credit struct {
	Kind CreditKind
	ID   int64
	// Name は正規の名前（canonical = 1 の行）。
	Name string
	// Aliases は名前の自然順（CompareNatural、同じなら元の文字列）。
	Aliases []Alias
	// ExternalIDs は (source, value) の順。
	ExternalIDs []ExternalID
	// Parent は団体の親の団体。人物と根の団体では nil。
	Parent *CreditRef
	// VideoCount はこの実体をどれかの役割で結ぶ、表示中の動画の利用者データの鍵の数。
	VideoCount int
	CreatedAt  time.Time
}

// CreditInput は実体を作るときの入力である。ParentID は団体だけが使い、人物では読まない。
type CreditInput struct {
	Name        string
	Aliases     []Alias
	ExternalIDs []ExternalID
	ParentID    *int64
}

// CreditPatch は実体の更新の入力である。nil のフィールドは今の値のまま残す。一覧は丸ごと
// 置き換え、nil の配列を指すポインタは一覧を空にする。ParentSet が真なら親を ParentID
// （nil なら親を外す）にし、偽なら今の親のまま残す。親は団体だけが使う。
type CreditPatch struct {
	Name        *string
	Aliases     *[]Alias
	ExternalIDs *[]ExternalID
	ParentSet   bool
	ParentID    *int64
}

// CreditQuery は実体の一覧の条件である。
type CreditQuery struct {
	// Q は生の検索語。保存層が FoldForMatch を掛けて前後の空白を落とし、空でなければ名前か
	// 別名の照合形に部分一致する実体だけにする。
	Q string
	// Sort はタグの一覧と同じ 5 つの並び順（specs/036-tag-admin-scale/research.md R-1）。
	// 空なら TagSortName。
	Sort TagSort
	// Cursor は前のページの CreditPage.NextCursor。解釈できないもの・別の並び順や別の種類の
	// ものは ErrInvalidCursor。
	Cursor string
	// Limit は 1 ページの件数（1 から MaxTagPageLimit）。0 なら全件を返し、Cursor は無視する。
	Limit int
}

// CreditPage は実体の一覧の 1 ページである。
type CreditPage struct {
	Items []Credit
	// Total は条件に合う実体の数。
	Total int
	// TotalAll はその種類の実体の数。
	TotalAll int
	// NextCursor は続きの取得に渡す。続きが無ければ空。
	NextCursor string
}

// CreditMergeOutcome は統合の結果である。
type CreditMergeOutcome struct {
	// Credit は統合後の統合先。
	Credit Credit
	// NotFoundIDs はもう無かった統合元の id。重複を除いた統合元に現れた順で、無ければ空の配列。
	NotFoundIDs []int64
}

// ErrCreditParentCycle は親の団体の付け替えか統合で、親の連鎖が輪になることを表す。API では
// organization_parent_cycle（400）になる。
var ErrCreditParentCycle = errors.New("the parent organization would form a cycle")

// ErrMergeSameCredit は統合元に統合先そのものが含まれることを表す。API では merge_same_person か
// merge_same_organization（400）になる。
var ErrMergeSameCredit = errors.New("cannot merge an entity into itself")

// ErrExternalIDTaken は外部 ID を同じ種類の別の実体が既に持つことを表す。API では
// external_id_taken（409）になる。errors.As で *CreditExternalIDTakenError を取り出せる。
var ErrExternalIDTaken = errors.New("external ID is held by another entity")

// CreditExternalIDTakenError は他の実体が持つ外部 ID を運ぶ。Field は呼び手が送った一覧での
// 経路（externalIds[1]）、HolderID はその外部 ID を持つ実体の id である。
type CreditExternalIDTakenError struct {
	Kind     CreditKind
	Field    string
	HolderID int64
}

func (e *CreditExternalIDTakenError) Error() string {
	return fmt.Sprintf("%s is held by %s %d", e.Field, e.Kind, e.HolderID)
}

func (e *CreditExternalIDTakenError) Unwrap() error { return ErrExternalIDTaken }

// CreditNotFoundError は参照した実体が無いことを表す。Field はその id を運んだプロパティの経路
// （parentId・people[0].person.id）である。errors.Is(err, ErrNotFound) は真になる。
type CreditNotFoundError struct {
	Kind  CreditKind
	ID    int64
	Field string
}

func (e *CreditNotFoundError) Error() string {
	return fmt.Sprintf("%s %d named by %s not found", e.Kind, e.ID, e.Field)
}

func (e *CreditNotFoundError) Unwrap() error { return ErrNotFound }

// NormalizeCredit は実体の入力を data-model.md「Fields」の規則で整える。誤りは名前・別名・
// 外部 ID の順に、最初の 1 件を *InvalidWorkInfoError で返す。
//
//   - 名前: 表示名と同じ手順で整え、空なら name の WorkInfoEmpty、CreditNameMaxLength を超えれば TooLong。
//   - 別名: 名前の空になった件は language があっても落とす。language は整えて空なら無し、
//     BCP 47 の構文（isLanguageTagSyntax）でなければ InvalidLanguage。名前か前の別名と同じ綴りは
//     aliases[i].name の Duplicate、落とした後の件数が CreditAliasesMaxCount を超えれば aliases の TooMany。
//   - 外部 ID: 両方が空の件は落とし、片方だけ空なら空の側の Empty。同じ (source, value) は Duplicate。
//
// 綴りは畳み込まない（Anime と anime は別の別名）。親の有無と輪は保存層が確かめる。
func NormalizeCredit(input CreditInput) (CreditInput, error) {
	name, err := normalizeWorkInfoText("name", input.Name, CreditNameMaxLength, false)
	if err != nil {
		return CreditInput{}, err
	}
	if name == "" {
		return CreditInput{}, invalidWorkInfo("name", WorkInfoEmpty, 0)
	}
	aliases, err := normalizeAliases(name, input.Aliases)
	if err != nil {
		return CreditInput{}, err
	}
	externalIDs, err := normalizeExternalIDs("externalIds", input.ExternalIDs)
	if err != nil {
		return CreditInput{}, err
	}
	return CreditInput{Name: name, Aliases: aliases, ExternalIDs: externalIDs, ParentID: input.ParentID}, nil
}

// normalizeAliases は NormalizeCredit の別名の規則である。
func normalizeAliases(name string, aliases []Alias) ([]Alias, error) {
	normalized := make([]Alias, 0, len(aliases))
	seen := map[string]bool{name: true}
	for i, raw := range aliases {
		field := fmt.Sprintf("aliases[%d]", i)
		alias, err := normalizeWorkInfoText(field+".name", raw.Name, CreditNameMaxLength, false)
		if err != nil {
			return nil, err
		}
		if alias == "" {
			// 名前の無い別名は language を確かめずに落とす（language の誤りでも拒まない）。
			continue
		}
		language, err := normalizeWorkInfoText(field+".language", raw.Language, CreditLanguageMaxLength, false)
		if err != nil {
			return nil, err
		}
		if language != "" && !isLanguageTagSyntax(language) {
			return nil, invalidWorkInfo(field+".language", WorkInfoInvalidLanguage, 0)
		}
		if seen[alias] {
			return nil, invalidWorkInfo(field+".name", WorkInfoDuplicate, 0)
		}
		seen[alias] = true
		normalized = append(normalized, Alias{Name: alias, Language: language})
	}
	if len(normalized) > CreditAliasesMaxCount {
		return nil, invalidWorkInfo("aliases", WorkInfoTooMany, CreditAliasesMaxCount)
	}
	return normalized, nil
}

// isLanguageTagSyntax は s が BCP 47 の言語タグの構文に合うかを返す。2〜35 文字の ASCII の
// 英字・数字・- で、先頭の副タグが 2 か 3 文字の英字（その後は終わりか -）である。登録された
// 言語かどうかは確かめない（data-model.md「Fields」）。長すぎる値は呼び手が TooLong で先に拒む。
func isLanguageTagSyntax(s string) bool {
	if len(s) < 2 || len(s) > CreditLanguageMaxLength {
		return false
	}
	letters := 0
	for letters < len(s) && isASCIILetter(s[letters]) {
		letters++
	}
	if letters < 2 || letters > 3 || (letters < len(s) && s[letters] != '-') {
		return false
	}
	for i := letters; i < len(s); i++ {
		c := s[i]
		if !isASCIILetter(c) && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// SortAliases は別名を名前の自然順に並べる。同順位は元の文字列で決着させる（SortTagNames と同じ）。
func SortAliases(aliases []Alias) {
	slices.SortStableFunc(aliases, func(a, b Alias) int {
		if order := CompareNatural(a.Name, b.Name); order != 0 {
			return order
		}
		return strings.Compare(a.Name, b.Name)
	})
}

// SortExternalIDs は外部 ID を (source, value) の順に並べる。
func SortExternalIDs(ids []ExternalID) {
	slices.SortFunc(ids, func(a, b ExternalID) int {
		if order := strings.Compare(a.Source, b.Source); order != 0 {
			return order
		}
		return strings.Compare(a.Value, b.Value)
	})
}
