package httpapi

import (
	"context"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// transcodeStartRetention は、変換の要求が終わってから台帳の行を残す時間である。
// 再読み込みの直後に届いた報告の要求が、終わった変換の値をまだ引けるようにする
// （specs/018-live-transcode-seek/contracts/transcode-start-api.md §2）。
// テストが短くできるよう変数にしている。
var transcodeStartRetention = 60 * time.Second

// transcodeAttemptPattern は attempt の形式である（契約 §1）。
var transcodeAttemptPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// validTranscodeAttempt は attempt が契約の形式に合うかを返す。
func validTranscodeAttempt(attempt string) bool {
	return transcodeAttemptPattern.MatchString(attempt)
}

// transcodeStartKey は台帳の鍵である。動画の識別子も含め、別の動画の経路から
// 同じ attempt を引けないようにする。
type transcodeStartKey struct {
	videoID int64
	attempt string
}

// transcodeStartSlot は 1 つの attempt の状態である。changed は状態が変わるたびに
// 閉じて作り直し、待っている報告の要求を起こす。
type transcodeStartSlot struct {
	// active は進行中の変換の要求の数、waiters は待っている報告の要求の数である。
	active  int
	waiters int
	// resolved は開始位置が決まったこと、failed は最初のデータの前に要求が終わった
	// ことを表す。決まった値は後の要求の失敗で消さない。
	resolved bool
	failed   bool
	startMs  int64
	// generation は変換の要求が始まるたびに進み、古い消去の予約を無効にする。
	generation uint64
	changed    chan struct{}
}

// transcodeStarts は attempt ごとの実際の開始位置の台帳である
// （plan.md Structural Decision 4）。
type transcodeStarts struct {
	mu    sync.Mutex
	slots map[transcodeStartKey]*transcodeStartSlot
}

func newTranscodeStarts() *transcodeStarts {
	return &transcodeStarts{slots: map[transcodeStartKey]*transcodeStartSlot{}}
}

// slotLocked は鍵の行を返し、無ければ作る。mu を持って呼ぶ。
func (l *transcodeStarts) slotLocked(key transcodeStartKey) *transcodeStartSlot {
	slot, ok := l.slots[key]
	if !ok {
		slot = &transcodeStartSlot{changed: make(chan struct{})}
		l.slots[key] = slot
	}
	return slot
}

func (slot *transcodeStartSlot) notifyLocked() {
	close(slot.changed)
	slot.changed = make(chan struct{})
}

// begin は変換の要求が始まったことを載せ、開始位置を記録する関数と、要求の終わりに
// 呼ぶ関数を返す。終わりまでに記録が無ければ失敗として報告の要求を起こす。
func (l *transcodeStarts) begin(key transcodeStartKey) (resolve func(startMs int64), end func()) {
	l.mu.Lock()
	slot := l.slotLocked(key)
	slot.active++
	slot.generation++
	slot.notifyLocked()
	l.mu.Unlock()

	resolved := false
	resolve = func(startMs int64) {
		l.mu.Lock()
		defer l.mu.Unlock()
		resolved = true
		slot.resolved, slot.failed, slot.startMs = true, false, startMs
		slot.notifyLocked()
	}
	end = func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		slot.active--
		if !resolved && !slot.resolved {
			slot.failed = true
		}
		slot.notifyLocked()
		if slot.active > 0 {
			return
		}
		generation := slot.generation
		time.AfterFunc(transcodeStartRetention, func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			if l.slots[key] == slot && slot.active == 0 && slot.generation == generation {
				delete(l.slots, key)
			}
		})
	}
	return resolve, end
}

// wait は開始位置が決まるまで待つ。attempt がまだ載っていなければ載るまで待つ。
// 決まらずに要求が失敗したとき、期限を過ぎたとき、ctx が終わったときは false を返す。
func (l *transcodeStarts) wait(ctx context.Context, key transcodeStartKey, timeout time.Duration) (int64, bool) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	l.mu.Lock()
	slot := l.slotLocked(key)
	slot.waiters++
	defer func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		slot.waiters--
		// 報告の要求が作っただけの行（変換の要求が来なかった）は、待ち手が居なくなったら消す。
		if slot.waiters == 0 && slot.active == 0 && slot.generation == 0 && l.slots[key] == slot {
			delete(l.slots, key)
		}
	}()
	for {
		if slot.resolved {
			startMs := slot.startMs
			l.mu.Unlock()
			return startMs, true
		}
		if slot.failed && slot.active == 0 {
			l.mu.Unlock()
			return 0, false
		}
		changed := slot.changed
		l.mu.Unlock()
		select {
		case <-changed:
		case <-timer.C:
			return 0, false
		case <-ctx.Done():
			return 0, false
		}
		l.mu.Lock()
	}
}

// GetTranscodeStart は同じ attempt を付けた変換の実際の開始位置を返す
// （contracts/transcode-start-api.md §2）。
func (s *server) GetTranscodeStart(w http.ResponseWriter, r *http.Request, id gen.VideoId, params gen.GetTranscodeStartParams) {
	// 公開でない動画へのゲストの要求は、変換の経路と同じ判定で 404 にする。
	video, r, release, ok := s.lookupServedVideo(w, r, id)
	if !ok {
		return
	}
	defer release()
	if !validTranscodeAttempt(params.Attempt) {
		s.invalidRequest(w, "attemptの形式が正しくありません")
		return
	}
	startMs, ok := s.transcodeStarts.wait(r.Context(),
		transcodeStartKey{videoID: video.ID, attempt: params.Attempt}, transcodeStartupTimeout)
	if !ok {
		if r.Context().Err() != nil {
			return
		}
		s.notFound(w, "この変換の開始位置はありません")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.TranscodeStart{StartMs: startMs}, s.logger)
}
