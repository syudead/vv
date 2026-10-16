package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
	"unicode/utf8"
)

// スマートフォルダ（specs/052-smart-folders/data-model.md）。スマートフォルダは所有者のデータで、
// 中身を持たず、保存した条件（SmartRule）に今当たる動画を見せる（research.md R-1、R-3）。
// 条件はライブラリの絞り込みと同じ項目で、JSON の 1 つのオブジェクトとして smart_folders.rule に
// 置く。親を持てば入れ子になる。

// SmartRuleMaxQueryLength は SmartRule.Q の上限（符号位置数）である。
const SmartRuleMaxQueryLength = 500

// SmartRuleMaxTags は SmartRule.Tags の件数の上限である。
const SmartRuleMaxTags = 16

var (
	// ErrInvalidSmartRule は条件か並べ順がスマートフォルダの規則に合わないことを表す。API では
	// 400 invalid_smart_rule である（contracts/screen-api.md）。
	ErrInvalidSmartRule = errors.New("invalid smart rule")
	// ErrSmartFolderNotFound は、作成か変更が親に名指したスマートフォルダが無いことを表す。API では
	// 404 smart_folder_not_found で、フォルダそのものが無い ErrNotFound（404 not_found）と区別する
	// ため ErrNotFound を包まない。
	ErrSmartFolderNotFound = errors.New("smart folder not found")
	// ErrSmartFolderCycle は、フォルダを自分自身か自分の子孫の下へ移そうとしたことを表す。
	ErrSmartFolderCycle = errors.New("smart folder cycle")
)

// SmartRule はスマートフォルダの条件である（data-model.md「smart_folders.rule」）。どの項目も
// 省けて、省いた項目は絞り込まない。空の条件はどの動画にも当たる。id の項目の 0 と、長さと追加日の
// nil は省いたことを表す。JSON の名前は smart_folders.rule と画面 API の SmartRule と同じである。
type SmartRule struct {
	// Q は検索欄の入力（SmartRuleMaxQueryLength 符号位置以下）である。
	Q string `json:"q,omitempty"`
	// Watch は視聴状態の絞り込みで、空か WatchUnwatched・WatchInProgress・WatchWatched である。
	Watch WatchFilter `json:"watch,omitempty"`
	// Playable はブラウザで再生できる動画だけにする。
	Playable bool `json:"playable,omitempty"`
	// Favorite はお気に入りの動画だけにする。
	Favorite bool `json:"favorite,omitempty"`
	// Tags はすべてを持つ動画だけにするタグの id（1〜SmartRuleMaxTags 件、重複なし）である。
	Tags []int64 `json:"tags,omitempty"`
	// TagKind はその種類のタグをどれか持つ動画だけにするタグの種類の id である（#1136）。
	TagKind int64 `json:"tagKind,omitempty"`
	// Person と Organization は、どの役割でも結び付いた人物・団体の id である。
	Person       int64 `json:"person,omitempty"`
	Organization int64 `json:"organization,omitempty"`
	// AddedWithin は追加からの日数（1〜MaxAddedWithinDays）である。
	AddedWithin *int `json:"addedWithin,omitempty"`
	// MinDuration と MaxDuration は長さの範囲（秒、両端を含む、0〜MaxDurationBoundSec、
	// 下限 ≤ 上限）である。
	MinDuration *int `json:"minDuration,omitempty"`
	MaxDuration *int `json:"maxDuration,omitempty"`
}

// smartRuleInput は JSON の条件を読む形である。id の項目は、0 や負の値を省いた項目と取り違え
// ないよう、ポインタで受ける。
type smartRuleInput struct {
	Q            string      `json:"q"`
	Watch        WatchFilter `json:"watch"`
	Playable     bool        `json:"playable"`
	Favorite     bool        `json:"favorite"`
	Tags         *[]int64    `json:"tags"`
	TagKind      *int64      `json:"tagKind"`
	Person       *int64      `json:"person"`
	Organization *int64      `json:"organization"`
	AddedWithin  *int        `json:"addedWithin"`
	MinDuration  *int        `json:"minDuration"`
	MaxDuration  *int        `json:"maxDuration"`
}

// ParseSmartRule は JSON のオブジェクト raw を条件にし、規則（Validate）を確かめる。オブジェクト
// でない値、知らない項目、型の違う値、範囲の外の値は ErrInvalidSmartRule を包む誤りである。
func ParseSmartRule(raw []byte) (SmartRule, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return SmartRule{}, fmt.Errorf("%w: the rule is not an object", ErrInvalidSmartRule)
	}
	// json.Unmarshal は入力全体が 1 つの値であることを求めるので、後ろに残る文字（閉じ区切りも）を
	// 拒む。
	var members map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &members); err != nil {
		return SmartRule{}, fmt.Errorf("%w: %w", ErrInvalidSmartRule, err)
	}
	// encoding/json は null を省いた項目と同じに読むので、型の違う値として先に拒む。
	for name, value := range members {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return SmartRule{}, fmt.Errorf("%w: %s is null", ErrInvalidSmartRule, name)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	var input smartRuleInput
	if err := decoder.Decode(&input); err != nil {
		return SmartRule{}, fmt.Errorf("%w: %w", ErrInvalidSmartRule, err)
	}
	rule := SmartRule{
		Q: input.Q, Watch: input.Watch, Playable: input.Playable, Favorite: input.Favorite,
		AddedWithin: input.AddedWithin, MinDuration: input.MinDuration, MaxDuration: input.MaxDuration,
	}
	if input.Tags != nil {
		if len(*input.Tags) == 0 {
			return SmartRule{}, fmt.Errorf("%w: tags is empty", ErrInvalidSmartRule)
		}
		rule.Tags = *input.Tags
	}
	for _, member := range []struct {
		name string
		in   *int64
		out  *int64
	}{
		{"tagKind", input.TagKind, &rule.TagKind},
		{"person", input.Person, &rule.Person},
		{"organization", input.Organization, &rule.Organization},
	} {
		if member.in == nil {
			continue
		}
		if *member.in <= 0 {
			return SmartRule{}, fmt.Errorf("%w: %s is not a positive id", ErrInvalidSmartRule, member.name)
		}
		*member.out = *member.in
	}
	if err := rule.Validate(); err != nil {
		return SmartRule{}, err
	}
	return rule, nil
}

// Validate は条件が規則に合うかを確かめる。合わなければ ErrInvalidSmartRule を包む誤りである。
func (r SmartRule) Validate() error {
	if utf8.RuneCountInString(r.Q) > SmartRuleMaxQueryLength {
		return fmt.Errorf("%w: q is longer than %d characters", ErrInvalidSmartRule, SmartRuleMaxQueryLength)
	}
	switch r.Watch {
	case "", WatchUnwatched, WatchInProgress, WatchWatched:
	default:
		return fmt.Errorf("%w: watch %q is unknown", ErrInvalidSmartRule, r.Watch)
	}
	if err := validateSmartRuleTags(r.Tags); err != nil {
		return err
	}
	for _, id := range []int64{r.TagKind, r.Person, r.Organization} {
		if id < 0 {
			return fmt.Errorf("%w: an id is negative", ErrInvalidSmartRule)
		}
	}
	if err := CheckAddedAndLength(r.AddedWithin, r.MinDuration, r.MaxDuration); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidSmartRule, err)
	}
	return nil
}

// validateSmartRuleTags は SmartRule.Tags の規則（省くか 1〜SmartRuleMaxTags 件、正の id、重複なし）を
// 確かめる。
func validateSmartRuleTags(tags []int64) error {
	if tags == nil {
		return nil
	}
	if len(tags) == 0 || len(tags) > SmartRuleMaxTags {
		return fmt.Errorf("%w: tags holds %d ids, not 1 to %d", ErrInvalidSmartRule, len(tags), SmartRuleMaxTags)
	}
	seen := make(map[int64]bool, len(tags))
	for _, id := range tags {
		if id <= 0 {
			return fmt.Errorf("%w: tags holds a non-positive id", ErrInvalidSmartRule)
		}
		if seen[id] {
			return fmt.Errorf("%w: tags holds %d twice", ErrInvalidSmartRule, id)
		}
		seen[id] = true
	}
	return nil
}

// Encode は条件を確かめ（Validate）、smart_folders.rule に置く JSON にする。省いた項目は書かない。
func (r SmartRule) Encode() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("cannot encode the smart rule: %w", err)
	}
	return string(encoded), nil
}

// Query は条件と同じ絞り込みの VideoQuery を返す（research.md R-3）。並べ順・カーソル・件数は
// 呼び出し側が決める。TagKind はタグの種類の絞り込み（VideoQuery.TagKindID、#1136）に写す。
func (r SmartRule) Query() VideoQuery {
	return VideoQuery{
		Query:           r.Q,
		Watch:           r.Watch,
		PlayableOnly:    r.Playable,
		FavoriteOnly:    r.Favorite,
		TagIDs:          slices.Clone(r.Tags),
		TagKindID:       r.TagKind,
		Person:          r.Person,
		Organization:    r.Organization,
		AddedWithinDays: derefInt(r.AddedWithin),
		MinDurationSec:  cloneInt(r.MinDuration),
		MaxDurationSec:  cloneInt(r.MaxDuration),
	}
}

func derefInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func cloneInt(v *int) *int {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

// SmartRuleRefKind は条件が名指す実体の種類である。値は画面 API の SmartRuleRef.kind と同じである。
type SmartRuleRefKind string

const (
	SmartRefTag          SmartRuleRefKind = "tag"
	SmartRefTagKind      SmartRuleRefKind = "tagKind"
	SmartRefPerson       SmartRuleRefKind = "person"
	SmartRefOrganization SmartRuleRefKind = "organization"
)

// SmartRuleRef は条件が名指す実体 1 つである。
type SmartRuleRef struct {
	Kind SmartRuleRefKind
	ID   int64
}

// Refs は条件が名指す実体を、タグ（条件の順）、タグの種類、人物、団体の順に返す。欠けた参照の
// 確かめと、統合による書き換えに使う（research.md R-9）。
func (r SmartRule) Refs() []SmartRuleRef {
	refs := make([]SmartRuleRef, 0, len(r.Tags)+3)
	for _, id := range r.Tags {
		refs = append(refs, SmartRuleRef{Kind: SmartRefTag, ID: id})
	}
	for _, ref := range []SmartRuleRef{
		{Kind: SmartRefTagKind, ID: r.TagKind},
		{Kind: SmartRefPerson, ID: r.Person},
		{Kind: SmartRefOrganization, ID: r.Organization},
	} {
		if ref.ID != 0 {
			refs = append(refs, ref)
		}
	}
	return refs
}

// ReplaceRefs は、kind の実体のうち sources が名指すものを target へ書き換えた条件と、書き換えたか
// どうかを返す（data-model.md「Merges」、research.md R-9）。タグは条件の順を保って重複を除くので、
// [A, B] の A を B へ統合すると [B] になる。タグの種類・人物・団体は id を差し替える。r は変えない。
func (r SmartRule) ReplaceRefs(kind SmartRuleRefKind, sources []int64, target int64) (SmartRule, bool) {
	out := r
	changed := false
	switch kind {
	case SmartRefTag:
		out.Tags, changed = replaceSmartRuleTags(r.Tags, sources, target)
	case SmartRefTagKind:
		out.TagKind, changed = replaceSmartRuleID(r.TagKind, sources, target)
	case SmartRefPerson:
		out.Person, changed = replaceSmartRuleID(r.Person, sources, target)
	case SmartRefOrganization:
		out.Organization, changed = replaceSmartRuleID(r.Organization, sources, target)
	}
	return out, changed
}

// replaceSmartRuleTags は tags の sources を target にし、最初に現れた位置を残して重複を除く。
// 書き換えなければ tags をそのまま返す。
func replaceSmartRuleTags(tags, sources []int64, target int64) ([]int64, bool) {
	if !slices.ContainsFunc(tags, func(id int64) bool { return slices.Contains(sources, id) }) {
		return tags, false
	}
	out := make([]int64, 0, len(tags))
	for _, id := range tags {
		if slices.Contains(sources, id) {
			id = target
		}
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out, true
}

// replaceSmartRuleID は id が sources にあれば target を返す。
func replaceSmartRuleID(id int64, sources []int64, target int64) (int64, bool) {
	if id != 0 && slices.Contains(sources, id) {
		return target, true
	}
	return id, false
}

// ValidateSmartSort はスマートフォルダの並べ順を確かめる。既知の VideoSort で SortRandom でない
// ものだけを受け付け、ほかは ErrInvalidSmartRule を包む誤りである（data-model.md「Rules」）。
func ValidateSmartSort(sort VideoSort) error {
	if !sort.Valid() || sort == SortRandom {
		return fmt.Errorf("%w: sort %q", ErrInvalidSmartRule, sort)
	}
	return nil
}

// ValidateSmartFolderParent はフォルダ id を親 parent の下へ置いてよいかを、今の親 parents（フォルダの
// id から親の id、根は入らないか 0）から決める。parent が 0 なら根にするので常によい。parent が id
// そのものか id の子孫（parent から親をたどって id に着く）なら ErrSmartFolderCycle である
// （data-model.md「Edits」）。新しく作るフォルダ（id が 0）は子孫を持たない。
func ValidateSmartFolderParent(id, parent int64, parents map[int64]int64) error {
	if parent == 0 || id == 0 {
		return nil
	}
	seen := map[int64]bool{}
	for at := parent; at != 0 && !seen[at]; at = parents[at] {
		if at == id {
			return ErrSmartFolderCycle
		}
		seen[at] = true
	}
	return nil
}

// SmartFolder はスマートフォルダ 1 つの行である。
type SmartFolder struct {
	ID int64
	// ParentID は親のフォルダの id で、根なら 0 である。
	ParentID int64
	Name     string
	Rule     SmartRule
	Sort     VideoSort
	// OwnerKept は所有者が作ったか、名前・条件・並べ順・親を直したフォルダか（research.md R-10）。
	OwnerKept bool
	// CreatedAt と UpdatedAt は Unix ミリ秒である。
	CreatedAt int64
	UpdatedAt int64
}

// SmartFolderCreate はスマートフォルダの作成の入力である。Sort が空なら SortAddedDesc、ParentID が
// 0 なら根に作る。
type SmartFolderCreate struct {
	Name     string
	Rule     SmartRule
	Sort     VideoSort
	ParentID int64
}

// SmartFolderChange はスマートフォルダの変更の入力である。nil の項目は今の値のまま残す。ParentSet
// が真なら親を ParentID（0 は根）にする。
type SmartFolderChange struct {
	Name      *string
	Rule      *SmartRule
	Sort      *VideoSort
	ParentSet bool
	ParentID  int64
}

// LibrarySmartFolder はスマートフォルダのカードである（contracts/screen-api.md の
// LibrarySmartFolder）。カードの値は LibraryGroup と同じ値を、フォルダの再生順（Members）から
// 作る（data-model.md「Contents」）。埋め込んだ LibraryGroup の ID と Name はフォルダのもので、
// Path は空、Favorite はフォルダのお気に入りである。
type LibrarySmartFolder struct {
	LibraryGroup
	// ParentID は親のフォルダの id で、根なら 0 である。
	ParentID int64
}

// SmartFolderSummary はスマートフォルダの要約で、ライブラリと中身のページのフォルダの項目が持つ
// （contracts/screen-api.md の SmartFolderSummary、research.md R-14）。カードの値は持たず、画面が
// カードの読み出し（SmartFolderCard）で読む。
type SmartFolderSummary struct {
	ID int64
	// ParentID は親のフォルダの id で、根なら 0 である。
	ParentID int64
	Name     string
	// Favorite は所有者がフォルダをお気に入りにしたかである。
	Favorite bool
}

// MaxSmartFolderCards はカードの読み出し 1 回で名指せるフォルダの数の上限である
// （contracts/screen-api.md「Statuses」）。
const MaxSmartFolderCards = 60

// SmartFolderCard はカードの読み出しのフォルダ 1 つの値である（GET /api/smart-folders/cards、
// data-model.md「Cards read」）。値はどれも、フォルダの一致する動画（グループにまとめず動画ごとに
// 条件を確かめた、見る人に見せる動画）から作る。
type SmartFolderCard struct {
	SmartFolderSummary
	// VideoCount は一致する動画の本数である。
	VideoCount int
	// WatchState と WatchedCount は動画ごとの視聴状態から GroupWatch と同じく決める。
	WatchState   WatchState
	WatchedCount int
	// OpenVideoID は押したときに開く動画で、フォルダの並べ順で GroupOpenIndex と同じく選ぶ。一致する
	// 動画が無ければ 0 である。
	OpenVideoID int64
	// DurationMs は長さの分かっている動画の合計で、1 本も分からなければ nil である。
	DurationMs *int64
	// SizeBytes は大きさの合計である。
	SizeBytes int64
	// AddedAt は追加日時の最大で、一致する動画が無ければゼロ値である。
	AddedAt time.Time
	// LastPlayedAt は最後に再生した時刻の最大で、記録が無ければ nil である。
	LastPlayedAt *time.Time
	// Previews はサムネイルのある動画をフォルダの並べ順に MaxFolderPreviews 本まで持つ。
	Previews []Video
	// Tags は一致する動画のタグの和集合である。ゲストには空である。
	Tags []VideoTag
}

// SmartFolderPage はスマートフォルダ 1 つと、その中身 1 ページである
// （GET /api/smart-folders/{id} と GET /api/smart-folders/{id}/items）。
type SmartFolderPage struct {
	// SmartFolder はフォルダのカードである。
	SmartFolder LibrarySmartFolder
	// Folder はフォルダの行（条件と並べ順）である。応答の settings は所有者にだけ返す。
	Folder SmartFolder
	// Missing は、フォルダと祖先の条件が名指す実体のうち今無いものである（外側の祖先から順）。
	// 1 つでもあれば、中身は子フォルダだけである（research.md R-9）。
	Missing []SmartRuleRef
	// Trail は祖先のフォルダを外側から順に持つ。
	Trail []GroupRef
	// Items は最初のページなら子フォルダ（名前の順、要約）を先に、続けてライブラリの項目を持つ。
	Items []LibraryItem
	// NextCursor は次のページの取得に渡す。これ以上無ければ空。
	NextCursor string
	// Roots は登録フォルダの一覧である。中身と同じ読み取りスナップショットから読む。
	Roots []MediaFolder
}

// SmartFoldersChanged は、中身かカードが変わったかもしれないスマートフォルダを表す
// （research.md R-12、data-model.md「Notices」）。フォルダの編集はそのフォルダと子孫と親を、一致が
// 変わりうる書き込みはすべてのフォルダを名指す。IDs は小さい順に重複なく持ち、空では発行しない。
// 取引の確定後に発行する。
type SmartFoldersChanged struct {
	IDs []int64
}

func (SmartFoldersChanged) event() {}
