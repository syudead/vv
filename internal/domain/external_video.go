package domain

// 外部連携 API の動画の読み出し（specs/026-external-api/data-model.md §2、
// contracts/external-api.md §2、research.md R-6）。

const (
	// ExternalVideoDefaultLimit は外部連携 API の一覧で limit が指定されなかったときの件数。
	ExternalVideoDefaultLimit = 100
)

// VideoRef は外部ツールが動画を指す値で、ID・ContentKey・Path のちょうど 1 つを持つ。
// Path は正規化せず、所在の path とバイト列で比べる（NFD の綴りもそのまま）。
type VideoRef struct {
	ID         int64
	ContentKey string
	Path       string
}

// Valid は ID・ContentKey・Path のちょうど 1 つが指定されていることを返す。
// ID は 1 以上、ContentKey と Path は空でないときに指定があるとみなす。
func (r VideoRef) Valid() bool {
	count := 0
	if r.ID != 0 {
		count++
	}
	if r.ContentKey != "" {
		count++
	}
	if r.Path != "" {
		count++
	}
	return count == 1 && r.ID >= 0
}

// ExternalVideoQuery は外部連携 API の動画の一覧 1 ページの条件である。並びは
// (AddedAt, ID) の昇順だけで、順序の指定は持たない。
type ExternalVideoQuery struct {
	// Cursor は前のページの NextCursor。空なら先頭から。
	Cursor string
	// Limit は 1 ページの件数。0 以下は ExternalVideoDefaultLimit、MaxLimit を越える値は
	// MaxLimit に丸める（範囲外を誤りにするのは入口の仕事）。
	Limit int
}

// ExternalVideo は外部連携 API に返す動画 1 本である。見る人は常に所有者で、
// 非公開の動画も含む。
type ExternalVideo struct {
	// Video は動画の属性。Path・Title などの代表の所在の値は、登録フォルダの下の
	// 所在のうちパスの最小の 1 件のもの（画面の API と同じ）。
	Video Video
	// Locations は登録フォルダの下の今の所在の path をパスの順に並べたもの。
	// 代表（Video.Path）が先頭になる。
	Locations []string
	// Tags は動画のタグ（手で付けた分とフォルダ名から付く分、出所つき）。
	Tags []VideoTag
}

// ExternalVideoPage は外部連携 API の動画の一覧 1 ページである。
type ExternalVideoPage struct {
	Items []ExternalVideo
	// NextCursor は続きの取得に渡す値。続きが無ければ空。
	NextCursor string
}
