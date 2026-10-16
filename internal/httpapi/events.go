package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// eventsKeepAlive は、変化が無いあいだに接続を保つためのコメント行を送る間隔
// である。途中の中継や OS が無通信の接続を切るのを防ぐだけで、状態は送らない。
const eventsKeepAlive = 30 * time.Second

// eventsRetryMs は、切れたときにブラウザがつなぎ直すまでの待ち時間である。
const eventsRetryMs = 3000

// Events は画面へ送る変化の知らせを配る。
//
// 知らせるのは「何が変わったか」だけで、送る内容は送る直前に読み直す。
// 知らせが続けて届いても、まだ送っていない分は1つにまとめるので、遅い
// 接続のために知らせを捨てたり、送る側を待たせたりしない。
type Events struct {
	mu     sync.Mutex
	subs   map[*eventSubscriber]struct{}
	closed chan struct{}
	once   sync.Once
}

// NewEvents は配り先の無い Events を返す。
func NewEvents() *Events {
	return &Events{subs: map[*eventSubscriber]struct{}{}, closed: make(chan struct{})}
}

// eventSubscriber は1本の接続が、まだ送っていない変化を持つ。
type eventSubscriber struct {
	mu   sync.Mutex
	scan bool
	// candidates は重複の候補の一覧が変わったかもしれないことを表す（specs/054-auto-dup/research.md R-10）。
	candidates bool
	// activity はアクティビティの記録が書かれたか変わったことを表す（specs/055-activity/research.md R-8）。
	activity bool
	videos   map[int64]struct{}
	groups   map[int64]struct{}
	// smartFolders はまだ送っていないスマートフォルダの知らせの id である。
	smartFolders map[int64]struct{}
	// ready は送るものがあることを知らせる。容量1で、重なった知らせはまとまる。
	ready chan struct{}
}

func (s *eventSubscriber) mark(update func(*eventSubscriber)) {
	s.mu.Lock()
	update(s)
	s.mu.Unlock()
	select {
	case s.ready <- struct{}{}:
	default:
	}
}

// pendingEvents はまだ送っていない変化である。
type pendingEvents struct {
	scan         bool
	candidates   bool
	activity     bool
	videos       []int64
	groups       []int64
	smartFolders []int64
}

// take はまだ送っていない変化を取り出して空にする。
func (s *eventSubscriber) take() pendingEvents {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending := pendingEvents{
		scan: s.scan, candidates: s.candidates, activity: s.activity, videos: sortedIDs(s.videos),
		groups: sortedIDs(s.groups), smartFolders: sortedIDs(s.smartFolders),
	}
	s.scan = false
	s.candidates = false
	s.activity = false
	clear(s.videos)
	clear(s.groups)
	clear(s.smartFolders)
	return pending
}

// sortedIDs は集合の id を小さい順に返す。空なら nil である。
func sortedIDs(set map[int64]struct{}) []int64 {
	var ids []int64
	for id := range set {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func (e *Events) subscribe() (*eventSubscriber, func()) {
	sub := &eventSubscriber{
		videos: map[int64]struct{}{}, groups: map[int64]struct{}{}, smartFolders: map[int64]struct{}{},
		ready: make(chan struct{}, 1),
	}
	// つないだ直後に今の状態を1回送る。切れていた間の変化を取り戻すためである。
	sub.mark(func(s *eventSubscriber) { s.scan = true })
	e.mu.Lock()
	e.subs[sub] = struct{}{}
	e.mu.Unlock()
	return sub, func() {
		e.mu.Lock()
		delete(e.subs, sub)
		e.mu.Unlock()
	}
}

func (e *Events) publish(update func(*eventSubscriber)) {
	e.mu.Lock()
	subs := make([]*eventSubscriber, 0, len(e.subs))
	for sub := range e.subs {
		subs = append(subs, sub)
	}
	e.mu.Unlock()
	for _, sub := range subs {
		sub.mark(update)
	}
}

// ScanChanged は直近の取り込みが変わったかもしれないことを知らせる。
func (e *Events) ScanChanged() {
	e.publish(func(s *eventSubscriber) { s.scan = true })
}

// ProcessingChanged は残りの仕事が変わったかもしれないことを知らせる。仕事の成否で
// 直近の取り込みの済みの本数と状態が変わるので、scan を送る
// （specs/024-import-progress/contracts/scan-api.md §4）。
func (e *Events) ProcessingChanged() {
	e.publish(func(s *eventSubscriber) { s.scan = true })
}

// ScanActivityChanged は取り込み中の今の処理が変わったことを知らせる。今の処理は
// scan が運ぶので、scan だけを送る（specs/024-import-progress/contracts/scan-api.md §4）。
// 1ファイルごとに届くが、接続ごとにまとまるので、遅い接続に古い値は積もらない。
func (e *Events) ScanActivityChanged() {
	e.publish(func(s *eventSubscriber) { s.scan = true })
}

// VideoChanged は動画の状態が変わったことを知らせる。
func (e *Events) VideoChanged(id int64) {
	e.publish(func(s *eventSubscriber) { s.videos[id] = struct{}{} })
}

// CandidatesChanged は重複の候補の一覧が変わったかもしれないことを知らせる。中身は運ばず、
// 重複の候補の画面が取り直す（specs/054-auto-dup/research.md R-10）。
func (e *Events) CandidatesChanged() {
	e.publish(func(s *eventSubscriber) { s.candidates = true })
}

// ActivityChanged はアクティビティの記録が書かれたか変わったことを知らせる。中身は運ばず、アクティビティの
// 画面と入口の件数が取り直す（specs/055-activity/research.md R-8）。
func (e *Events) ActivityChanged() {
	e.publish(func(s *eventSubscriber) { s.activity = true })
}

// GroupsChanged はグループの名前か項目が変わったことを知らせる（specs/048-groups/research.md R-13）。
// ids は変わったグループとその祖先の id である。
func (e *Events) GroupsChanged(ids []int64) {
	if len(ids) == 0 {
		return
	}
	e.publish(func(s *eventSubscriber) {
		for _, id := range ids {
			s.groups[id] = struct{}{}
		}
	})
}

// SmartFoldersChanged はスマートフォルダの中身かカードが変わったかもしれないことを知らせる
// （specs/052-smart-folders/research.md R-12）。ids は名指すフォルダの id である。
func (e *Events) SmartFoldersChanged(ids []int64) {
	if len(ids) == 0 {
		return
	}
	e.publish(func(s *eventSubscriber) {
		for _, id := range ids {
			s.smartFolders[id] = struct{}{}
		}
	})
}

// Handle は状態の変化を、画面へ送る知らせに置き換える。cmd/mdm が
// domain.Event の購読として登録する。知らせる対象ではない変化は無視する。
func (e *Events) Handle(event domain.Event) {
	switch event := event.(type) {
	case domain.ScanChanged:
		e.ScanChanged()
	case domain.ProcessingChanged:
		e.ProcessingChanged()
	case domain.ScanActivityChanged:
		e.ScanActivityChanged()
	case domain.VideoIngestChanged:
		e.VideoChanged(event.VideoID)
	case domain.VideoOverrideChanged:
		// 表示名・代表サムネイルの位置の変化も、一覧と動画ページに読み直させる
		// （specs/029-video-overrides/research.md R-7）。
		e.VideoChanged(event.VideoID)
	case domain.VideoBundleChanged:
		// 束ねの変化は、影響した各動画の video の知らせにする。一覧・動画ページが集まりの
		// 値と代表を読み直す（specs/030-video-versions/research.md R-9）。
		for _, id := range event.VideoIDs {
			e.VideoChanged(id)
		}
	case domain.VideoWorkInfoChanged:
		// 作品情報の変化は、同じ利用者データの鍵の各動画の video の知らせにする。/api/events の種類は
		// 増やさない（specs/047-video-work-info/research.md R-11）。
		for _, id := range event.VideoIDs {
			e.VideoChanged(id)
		}
	case domain.AutoTagApplied:
		// 自動タグ付けの結果は、一覧と動画ページにタグを読み直させる（docs/design-docs/auto-tagging.md）。
		for _, id := range event.VideoIDs {
			e.VideoChanged(id)
		}
	case domain.VersionCandidatesChanged:
		// 判定の結果や版の取り外しで候補が変わったら candidates の知らせにする
		// （specs/054-auto-dup/research.md R-10）。
		e.CandidatesChanged()
	case domain.ActivityChanged:
		// 記録の書き込みと変化は activity の知らせにする（specs/055-activity/research.md R-8）。
		e.ActivityChanged()
	case domain.GroupsChanged:
		// グループの変化は group の知らせにする。グループの画面・カード・動画ページのグループの行が
		// 読み直す（specs/048-groups/research.md R-13）。
		e.GroupsChanged(event.GroupIDs)
	case domain.SmartFoldersChanged:
		// スマートフォルダの変化は smartFolder の知らせにする。フォルダの画面とライブラリのフォルダの
		// カードが読み直す（specs/052-smart-folders/research.md R-12）。
		e.SmartFoldersChanged(event.IDs)
	}
}

// Close はすべての接続を終わらせる。停止時に呼ぶ。流れ続ける応答が残ると、
// HTTP サーバーの停止が猶予時間いっぱいまで待たされる。
func (e *Events) Close() {
	e.once.Do(func() { close(e.closed) })
}

// StreamEvents は変化を Server-Sent Events で送る（GET /api/events）。
func (s *server) StreamEvents(w http.ResponseWriter, r *http.Request) {
	if s.events == nil {
		s.internalError(w, "Change notifications are not configured.", nil)
		return
	}
	// 応答を返し始める前に受け口を作る。ブラウザは最初の送信を受けた時点で
	// つながったとみなして取り直すので、そのあとに登録すると、間に起きた
	// 動画の変化を取りこぼす。
	sub, unsubscribe := s.events.subscribe()
	defer unsubscribe()
	// ゲストには group と smartFolder だけを送る。scan と video は所有者の取り込みと非公開の動画を
	// 運び、candidates と activity は所有者だけが開く重複の候補とアクティビティの画面のものである
	// （specs/048-groups/contracts/screen-api.md「/api/events」、specs/054-auto-dup/research.md R-10、
	// specs/055-activity/research.md R-8）。
	guest := !audienceFrom(r.Context()).IsOwner()

	controller := http.NewResponseController(w)

	header := w.Header()
	header.Set("Content-Type", "text/event-stream; charset=utf-8")
	header.Set("Cache-Control", cacheNoStore)
	// 途中の中継がまとめて送ろうと溜め込むのを止める。
	header.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprintf(w, "retry: %d\n\n", eventsRetryMs); err != nil {
		return
	}
	if err := controller.Flush(); err != nil {
		s.logger.Warn("cannot send a change notification", slog.Any("error", err))
		return
	}

	keepAlive := time.NewTicker(eventsKeepAlive)
	defer keepAlive.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.events.closed:
			return
		case <-keepAlive.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
		case <-sub.ready:
			if err := s.writePendingEvents(ctx, w, sub, guest); err != nil {
				if ctx.Err() == nil {
					s.logger.Warn("cannot send a change notification", slog.Any("error", err))
				}
				return
			}
		}
		if err := controller.Flush(); err != nil {
			return
		}
	}
}

// writePendingEvents はまだ送っていない変化を、今の内容で書き出す。guest なら group と smartFolder
// だけを書く。
func (s *server) writePendingEvents(ctx context.Context, w http.ResponseWriter, sub *eventSubscriber, guest bool) error {
	pending := sub.take()
	if guest {
		pending.scan = false
		pending.candidates = false
		pending.activity = false
		pending.videos = nil
	}
	if pending.scan && s.scans != nil {
		current, err := s.scans.CurrentScan(ctx)
		switch {
		case errors.Is(err, domain.ErrNotFound):
		case err != nil:
			return fmt.Errorf("cannot read the scan status: %w", err)
		default:
			if err := writeEvent(w, "scan", toAPIScan(current)); err != nil {
				return err
			}
		}
	}
	if pending.candidates {
		// 中身の無い知らせである。EventSource は data の無いイベントを届けないので、空の
		// オブジェクトを data にする。
		if err := writeEvent(w, "candidates", struct{}{}); err != nil {
			return err
		}
	}
	if pending.activity {
		// candidates と同じく中身の無い知らせである。
		if err := writeEvent(w, "activity", struct{}{}); err != nil {
			return err
		}
	}
	for _, id := range pending.videos {
		if err := writeEvent(w, "video", gen.VideoChanged{Id: id}); err != nil {
			return err
		}
	}
	if len(pending.groups) > 0 {
		// 変わったグループは 1 つの知らせにまとめる。名前は載せない（ゲストに見せないグループの
		// 名前を運ばない）。
		if err := writeEvent(w, "group", gen.GroupChanged{Ids: pending.groups}); err != nil {
			return err
		}
	}
	if len(pending.smartFolders) > 0 {
		// group と同じく 1 つにまとめ、名前は載せない。
		if err := writeEvent(w, "smartFolder", gen.SmartFolderChanged{Ids: pending.smartFolders}); err != nil {
			return err
		}
	}
	return nil
}

func writeEvent(w http.ResponseWriter, name string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data)
	return err
}
