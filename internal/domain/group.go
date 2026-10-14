package domain

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// 任意のグループ（specs/048-groups/data-model.md）。グループは所有者のデータで、id は
// 変わらない（research.md R-1）。動画の項目は利用者データの鍵で、子グループの項目は
// グループの id で持つ（R-2）。フォルダから自動でできるグループは origin_folder を持ち、
// スキャンのたびにフォルダ由来のメンバーだけを足し引きする（R-3、PlanFolderGroupSync）。

// GroupNameMaxLength はグループの名前の上限（符号位置数）である。SQLite の length() と
// 同じ数え方で、表の check と同じ値である（research.md R-14）。
const GroupNameMaxLength = 100

// Group はグループ 1 つの要約である。
type Group struct {
	ID   int64
	Name string
	// OriginFolder は自動グループのフォルダの鍵（FolderKey）。手で作ったグループでは空である。
	// 登録フォルダ `/` の自動グループの鍵も空なので、自動かどうかは Automatic で分ける。
	OriginFolder string
	// Automatic はフォルダから自動でできたグループ（origin_folder が NULL でない）か。
	Automatic bool
	// UpdatedAt は名前か項目を最後に変えた時刻（Unix ミリ秒）である。
	UpdatedAt int64
	// Favorite は所有者がグループをお気に入りにしたか（specs/035-favorites/data-model.md §2）。
	// 見せる動画が無くなっても残る。
	Favorite bool
}

// GroupRef はグループを指す id と名前である。
type GroupRef struct {
	ID   int64
	Name string
}

// GroupItemsDefaultLimit と GroupItemsMaxLimit は、グループの項目 1 ページの件数の既定と上限である
// （specs/048-groups/contracts/screen-api.md「Statuses」）。
const (
	GroupItemsDefaultLimit = 100
	GroupItemsMaxLimit     = 200
)

// GroupListDefaultLimit と GroupListMaxLimit は、グループの一覧（GET /api/groups）1 ページの件数の
// 既定と上限である（specs/048-groups/contracts/screen-api.md「Statuses」）。
const (
	GroupListDefaultLimit = 100
	GroupListMaxLimit     = 200
)

// GroupListPage はグループの一覧 1 ページである（GET /api/groups）。
type GroupListPage struct {
	// Groups は見る人に見せるグループを、どの深さのものも自然な名前の順（同じなら id の順）に持つ。
	Groups []LibraryGroup
	// NextCursor は次のページの取得に渡す。これ以上無ければ空。
	NextCursor string
	// Roots は登録フォルダの一覧である。グループと同じ読み取りスナップショットから読む。
	Roots []MediaFolder
}

// GroupItem はグループの項目 1 件である。Video と Group のちょうど一方を持つ。
type GroupItem struct {
	// ID は項目（group_items.id）の id で、動画やグループの id ではない。
	ID    int64
	Video *Video
	Group *LibraryGroup
}

// GroupPage はグループ 1 つと、その項目 1 ページである（GET /api/groups/{id} と
// GET /api/groups/{id}/items）。
type GroupPage struct {
	// Group はグループの値（LibraryGroup）である。
	Group LibraryGroup
	// Items は見せる項目を位置の順に持つ 1 ページである。見る人に引けない動画の項目と、
	// 見る人に見せる動画の無い子グループの項目は入らない（data-model.md「Visibility」）。
	Items []GroupItem
	// Trail は要求の trail のうち、親と子の組が続く部分である（research.md R-8）。外側から順に
	// 持ち、最後はグループの親である。
	Trail []GroupRef
	// NextCursor は次のページの取得に渡す。これ以上無ければ空。
	NextCursor string
	// Roots は登録フォルダの一覧である。項目と同じ読み取りスナップショットから読む。
	Roots []MediaFolder
}

// GroupsChanged はグループの名前か項目が変わったことを表す（research.md R-13）。GroupIDs は
// 変わったグループと、その祖先のグループの id で、小さい順に重複なく持つ。取引の確定後に
// 発行する。
type GroupsChanged struct {
	GroupIDs []int64
}

func (GroupsChanged) event() {}

// ErrInvalidGroupName はグループの名前として受け付けられない入力を表す。ValidateGroupName は
// 具体的な理由を運ぶ *InvalidGroupNameError を返し、errors.Is(err, ErrInvalidGroupName) は
// 真になる。
var ErrInvalidGroupName = errors.New("invalid group name")

// GroupNameProblem はグループの名前の規則違反の種類である。
type GroupNameProblem int

const (
	// GroupNameEmpty は前後の空白を取り除くと空になることを表す。
	GroupNameEmpty GroupNameProblem = iota + 1
	// GroupNameTooLong は GroupNameMaxLength 符号位置を超えることを表す。
	GroupNameTooLong
)

// InvalidGroupNameError はグループの名前の規則違反の具体的な理由を運ぶ。
type InvalidGroupNameError struct {
	Problem GroupNameProblem
}

func (e *InvalidGroupNameError) Error() string {
	switch e.Problem {
	case GroupNameEmpty:
		return "group name is empty"
	case GroupNameTooLong:
		return fmt.Sprintf("group name must be at most %d characters", GroupNameMaxLength)
	default:
		return ErrInvalidGroupName.Error()
	}
}

func (e *InvalidGroupNameError) Unwrap() error { return ErrInvalidGroupName }

// ValidateGroupName は前後の Unicode White_Space を取り除いた名前を返す。空か
// GroupNameMaxLength 符号位置を超えれば *InvalidGroupNameError である。同じ名前は
// 重複してよい（research.md R-14）。
func ValidateGroupName(input string) (string, error) {
	trimmed, problem := checkName(input, GroupNameMaxLength)
	switch problem {
	case nameEmpty:
		return "", &InvalidGroupNameError{Problem: GroupNameEmpty}
	case nameTooLong:
		return "", &InvalidGroupNameError{Problem: GroupNameTooLong}
	}
	return trimmed, nil
}

// nameProblem は名前の規則（前後の空白を除いて 1 〜 上限の符号位置）の違反の種類である。
type nameProblem int

const (
	nameOK nameProblem = iota
	nameEmpty
	nameTooLong
)

// checkName は前後の Unicode White_Space を取り除いた名前と、規則違反の種類を返す。
// プレイリストとグループの名前が共有する（research.md R-14）。
func checkName(input string, maxLength int) (string, nameProblem) {
	trimmed := strings.TrimFunc(input, func(r rune) bool {
		return unicode.Is(unicode.White_Space, r)
	})
	if trimmed == "" {
		return "", nameEmpty
	}
	if utf8.RuneCountInString(trimmed) > maxLength {
		return "", nameTooLong
	}
	return trimmed, nameOK
}

// cutGroupName は自動グループの最初の名前を、フォルダ名の先頭 GroupNameMaxLength 符号位置に
// する（research.md R-14）。
func cutGroupName(name string) string {
	if utf8.RuneCountInString(name) <= GroupNameMaxLength {
		return name
	}
	runes := []rune(name)
	return string(runes[:GroupNameMaxLength])
}

// FolderSyncFolder は同期の入力にするフォルダ1つである（FolderVideos の動画を利用者データの
// 鍵へ引き直したもの）。
type FolderSyncFolder struct {
	// Key はフォルダの鍵（FolderKey）。
	Key string
	// Name はフォルダ名。
	Name string
	// Grouped は 017 の規則がこのフォルダをグループにするかどうかである。
	Grouped bool
	// ContentKeys は直下の動画の利用者データの鍵を、ファイル名の自然順に重複なく持つ。
	ContentKeys []string
}

// AutoGroup は同期の入力にする自動グループ1つである。
type AutoGroup struct {
	ID int64
	// OriginFolder はフォルダの鍵（FolderKey）。
	OriginFolder string
	// Items は動画の項目である。子グループの項目は同期に関わらないので持たない。
	Items []AutoGroupItem
	// Exclusions は所有者が外したフォルダ由来のメンバーの鍵である。
	Exclusions []string
}

// AutoGroupItem は自動グループの動画の項目1つである。
type AutoGroupItem struct {
	ContentKey string
	// FromFolder は同期が足した項目かどうかである（group_items.from_folder）。
	FromFolder bool
}

// FolderGroupCreate は同期が作るグループ1つである。
type FolderGroupCreate struct {
	OriginFolder string
	Name         string
	// ContentKeys は項目にする動画の鍵を並びの順に持つ。どれも from_folder = 1 で入る。
	ContentKeys []string
}

// FolderGroupChange は既存の自動グループ1つへの変更である。
type FolderGroupChange struct {
	GroupID int64
	// Remove はフォルダ由来で直下でなくなった項目の鍵である。
	Remove []string
	// Append は末尾に from_folder = 1 で足す鍵を並びの順に持つ。
	Append []string
}

// FolderGroupSync は同期で行う変更である。
type FolderGroupSync struct {
	// Create は作るグループをフォルダの鍵の順に持つ。
	Create []FolderGroupCreate
	// Change は変わる自動グループを id の順に持つ。変わらないグループは入らない。
	Change []FolderGroupChange
}

// PlanFolderGroupSync は、フォルダの索引（folders）、今の自動グループ（groups）、フォルダごとの
// 例外（overrides）から、自動グループをフォルダとそろえる変更を決める
// （specs/048-groups/data-model.md「Automatic group sync」、research.md R-3）。
//
//   - 規則がグループにするフォルダで、それを origin_folder に持つグループが無く、ungroup の
//     例外も無ければ、フォルダ名（GroupNameMaxLength で切る）で直下の動画を持つグループを作る。
//   - 自動グループのフォルダの直下の動画で、項目にも除外にも無いものを末尾に足す。規則が
//     もうグループにしないフォルダでも足す。
//   - フォルダ由来の項目で直下でなくなったもの（フォルダが無くなったときはすべて）を外す。
//   - 手で足した項目、名前、並び順には触れず、グループを消さない。
func PlanFolderGroupSync(folders []FolderSyncFolder, groups []AutoGroup, overrides map[string]FolderGroupMode) FolderGroupSync {
	byKey := make(map[string]FolderSyncFolder, len(folders))
	for _, folder := range folders {
		byKey[folder.Key] = folder
	}
	plan := FolderGroupSync{Create: []FolderGroupCreate{}, Change: []FolderGroupChange{}}
	origins := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		origins[group.OriginFolder] = struct{}{}
		if change, changed := syncAutoGroup(group, byKey[group.OriginFolder].ContentKeys); changed {
			plan.Change = append(plan.Change, change)
		}
	}
	for _, folder := range folders {
		if _, exists := origins[folder.Key]; exists || !folder.Grouped || overrides[folder.Key] == FolderGroupUngroup {
			continue
		}
		plan.Create = append(plan.Create, FolderGroupCreate{
			OriginFolder: folder.Key, Name: cutGroupName(folder.Name), ContentKeys: folder.ContentKeys,
		})
	}
	slices.SortFunc(plan.Create, func(a, b FolderGroupCreate) int { return strings.Compare(a.OriginFolder, b.OriginFolder) })
	slices.SortFunc(plan.Change, func(a, b FolderGroupChange) int { return cmp.Compare(a.GroupID, b.GroupID) })
	return plan
}

// syncAutoGroup は自動グループ1つを、そのフォルダの直下の動画 direct とそろえる変更を決める。
func syncAutoGroup(group AutoGroup, direct []string) (FolderGroupChange, bool) {
	directSet := make(map[string]struct{}, len(direct))
	for _, key := range direct {
		directSet[key] = struct{}{}
	}
	known := make(map[string]struct{}, len(group.Items)+len(group.Exclusions))
	change := FolderGroupChange{GroupID: group.ID}
	for _, item := range group.Items {
		known[item.ContentKey] = struct{}{}
		if _, still := directSet[item.ContentKey]; item.FromFolder && !still {
			change.Remove = append(change.Remove, item.ContentKey)
		}
	}
	for _, key := range group.Exclusions {
		known[key] = struct{}{}
	}
	for _, key := range direct {
		if _, held := known[key]; !held {
			change.Append = append(change.Append, key)
		}
	}
	return change, len(change.Remove) > 0 || len(change.Append) > 0
}
