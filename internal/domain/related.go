package domain

// RelatedVideos は関連動画の 1 ページである。並べ方は relatedness.go の RankRelated が決める。
type RelatedVideos struct {
	// Items はこのページの関連動画を並びの順に持つ。
	Items []Video
	// NextCursor は続きのページの位置。並びの終わりまで返したら nil。
	NextCursor *RelatedCursor
	// NextID は次の動画。グループのメンバーならグループの中の並びの次、そうでなければ
	// 関連動画の並びの先頭。最初のページだけが持ち、無ければ 0。
	NextID int64
	// PrevID はグループの中の並びの前の動画。グループのメンバーの最初のページだけが持ち、
	// 無ければ 0。グループに属さない動画の前の動画は、画面が見てきた順から決める。
	PrevID int64
	// Group は基準の動画が属するグループ。最初のページだけが持ち、見る人に見せる
	// グループが無ければ nil。
	Group *VideoGroup
}

// RelatedPage は関連動画のどのページを読むかである。After が nil なら最初のページ。
type RelatedPage struct {
	After *RelatedCursor
	Limit int
}

// GroupMemberWindow は、関連動画の応答に載せるグループのメンバーの本数の上限である。
// 基準の動画を中ほどに置いた窓で、残りは画面がページ（MaxGroupMemberPage）で読む。
// メンバーが数千本のグループでも応答がメンバーの本数に比例して膨らまないようにする
// （issue 674）。
const GroupMemberWindow = 100

// MaxGroupMemberPage は、グループのメンバーを1回に読む本数の上限である
// （GET /api/videos/{id}/group-members）。
const MaxGroupMemberPage = 200

// GroupWindow はグループのメンバーのうち詳細を読む範囲である。Limit が 0 なら詳細を
// 読まない（位置と本数だけが要るとき）。Around が true なら Offset を使わず、基準の動画が
// 中ほどに来るよう窓を置く。
type GroupWindow struct {
	Offset int
	Limit  int
	Around bool
}

// Start は本数 total のグループで、基準の動画の位置 position（1 始まり）に対する
// 窓の先頭（0 始まり）を返す。
func (w GroupWindow) Start(total, position int) int {
	if w.Limit <= 0 {
		return 0
	}
	start := w.Offset
	if w.Around {
		start = position - 1 - w.Limit/2
		start = min(start, total-w.Limit)
	}
	return max(0, min(start, total))
}

// VideoGroup は動画1本が属するグループを、見る人に見せてよいメンバーだけで表したもの
// である（specs/017-folder-groups/contracts/folder-groups-api.md §3・data-model.md §7）。
type VideoGroup struct {
	// Folder はグループのフォルダそのもの（LocateFolder）。
	Folder VideoFolder
	// Name はフォルダ名。
	Name string
	// MemberIDs は見せてよいメンバーの id をグループの中の並びで持つ。基準の動画を含む。
	MemberIDs []int64
	// Offset は Members の先頭が MemberIDs の何番目（0 始まり）かである。
	Offset int
	// Members は読んだ窓（GroupWindow）のメンバーの詳細を、グループの中の並びで持つ。
	Members []Video
}

// Total は見せてよいメンバーの本数である。
func (g VideoGroup) Total() int {
	return len(g.MemberIDs)
}

// Position は動画 id のグループの中の位置（1 始まり）を返す。メンバーでなければ 0。
func (g VideoGroup) Position(id int64) int {
	for i, member := range g.MemberIDs {
		if member == id {
			return i + 1
		}
	}
	return 0
}

// Neighbors はメンバー id のグループの中の前と次の動画 id を返す。最初のメンバーに
// 前は無く、最後のメンバーに次は無い（無ければ 0）。
func (g VideoGroup) Neighbors(id int64) (prev, next int64) {
	position := g.Position(id)
	if position == 0 {
		return 0, 0
	}
	if position > 1 {
		prev = g.MemberIDs[position-2]
	}
	if position < len(g.MemberIDs) {
		next = g.MemberIDs[position]
	}
	return prev, next
}
