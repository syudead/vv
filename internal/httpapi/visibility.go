package httpapi

import (
	"context"
	"net/http"
	"sync"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 公開フラグの切り替えと、非公開にした動画のゲストへの配信の打ち切り
// （specs/016-single-account-auth/contracts/guest-api.md §4・§6、
// plan.md Structural Decisions 5）。

// Visibility は動画の公開フラグの保存先である。internal/store の *VisibilityStore が
// これを満たす。1つのトランザクションで済むので、internal/app は通さない。
type Visibility interface {
	// SetVideosPublic は videoIDs のうちいまライブラリにある動画の公開フラグを
	// public にそろえ、反映した動画の content_key を返す。
	SetVideosPublic(ctx context.Context, videoIDs []int64, public bool) ([]string, error)
}

// UpdateVideoVisibility は動画の公開・非公開を切り替える（PUT /api/video-visibility）。
// videoIds の数の上限・重複の扱い・applied の意味はタグの付け外しと同じにする
// （contracts/guest-api.md §4）。非公開にしたら、その動画をゲストとして処理中の
// 応答を打ち切る（§6）。
func (s *server) UpdateVideoVisibility(w http.ResponseWriter, r *http.Request) {
	// gen.VideoVisibilityRequest の public は bool なので、欠けても false に読めてしまう。
	// 欠けた本文で非公開にしないよう、有無を区別して読む。
	var body struct {
		VideoIds []int64 `json:"videoIds"`
		Public   *bool   `json:"public"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if !validVideoTagsIDs(body.VideoIds) {
		s.invalidRequest(w, "videoIdsは1件以上20000件以下で指定してください")
		return
	}
	if body.Public == nil {
		s.invalidRequest(w, "publicを指定してください")
		return
	}
	public := *body.Public
	if s.visibility == nil {
		s.internalError(w, "公開フラグの保存先が設定されていません", nil)
		return
	}

	keys, err := s.visibility.SetVideosPublic(r.Context(), body.VideoIds, public)
	if err != nil {
		s.internalError(w, "公開フラグを切り替えられませんでした", err)
		return
	}
	// 反映を確定させてから打ち切る。確定の前に打ち切ると、その間に始まったゲストの
	// 要求が公開のままの状態を読んで配信を続けうる。
	if !public {
		s.guests.revoke(keys)
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.VideoVisibilityResponse{Applied: len(keys)}, s.logger)
}

// lookupServedVideo は配信・ライブ変換・ホバープレビューの対象の動画を引く。
// ゲストとして処理する要求は、その動画の content_key と一緒に guestLedger へ載せ、
// 打ち切れる context を持つ要求と、処理を終えたときに呼ぶ release を返す。
// 所有者として処理する要求は台帳に載せない（非公開にしても打ち切らない）。
func (s *server) lookupServedVideo(
	w http.ResponseWriter, r *http.Request, id int64,
) (domain.Video, *http.Request, func(), bool) {
	if audienceFrom(r.Context()).IsOwner() {
		video, ok := s.lookupVideo(w, r, id)
		return video, r, func() {}, ok
	}
	for {
		// 引く前の世代を覚え、載せるときに打ち切りが挟まっていたら引き直す。
		// 公開を読んでから台帳に載るまでの間に非公開へ切り替わった要求を逃さない。
		since := s.guests.generation()
		video, ok := s.lookupVideo(w, r, id)
		if !ok {
			return domain.Video{}, r, func() {}, false
		}
		ctx, release, tracked := s.guests.track(r.Context(), w, video.ContentKey, since)
		if tracked {
			return video, r.WithContext(ctx), release, true
		}
	}
}

// guestLedger はゲストとして処理中の配信の応答を、動画の content_key ごとに覚える
// 台帳である。非公開への切り替えは、その content_key の要求を打ち切る。
// 打ち切りの仕組み（context の取り消しと書き込みの締め切り）はセッションの台帳と
// 同じ trackedRequest を使う。
type guestLedger struct {
	mu sync.Mutex
	// gen は打ち切りのたびに進む世代である。
	gen      uint64
	requests map[string]map[*trackedRequest]struct{}
}

func newGuestLedger() *guestLedger {
	return &guestLedger{requests: map[string]map[*trackedRequest]struct{}{}}
}

// generation は今の世代を返す。track に渡す。
func (l *guestLedger) generation() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.gen
}

// track は要求を content_key と一緒に台帳に載せ、打ち切れる context を返す。
// since の後に打ち切りがあったら載せずに ok = false を返し、呼び出し側は動画を
// 引き直す。release は要求の処理を終えたとき（ハンドラから戻る前）に呼ぶ。
func (l *guestLedger) track(
	ctx context.Context, w http.ResponseWriter, contentKey string, since uint64,
) (context.Context, func(), bool) {
	ctx, cancel := context.WithCancel(ctx)
	req := &trackedRequest{cancel: cancel, controller: http.NewResponseController(w)}

	l.mu.Lock()
	if l.gen != since {
		l.mu.Unlock()
		cancel()
		return nil, nil, false
	}
	if l.requests[contentKey] == nil {
		l.requests[contentKey] = map[*trackedRequest]struct{}{}
	}
	l.requests[contentKey][req] = struct{}{}
	l.mu.Unlock()

	release := func() {
		req.finish()
		cancel()
		l.mu.Lock()
		delete(l.requests[contentKey], req)
		if len(l.requests[contentKey]) == 0 {
			delete(l.requests, contentKey)
		}
		l.mu.Unlock()
	}
	return ctx, release, true
}

// revoke は contentKeys の動画をゲストとして処理中の要求をすべて打ち切る。
func (l *guestLedger) revoke(contentKeys []string) {
	var requests []*trackedRequest
	l.mu.Lock()
	l.gen++
	for _, key := range contentKeys {
		for req := range l.requests[key] {
			requests = append(requests, req)
		}
	}
	l.mu.Unlock()
	for _, req := range requests {
		req.abort()
	}
}
