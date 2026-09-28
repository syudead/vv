package app

import (
	"strconv"
	"sync"

	"github.com/syudead/vv/internal/domain"
)

// scanActivityKey は走査の登録の処理のキーである。仕事は "job:<id>" を使う。
const scanActivityKey = "scan"

// activities は動いている処理をメモリに持つ（specs/024-import-progress/research.md
// R-8）。保存しないので、再起動のあとはそのとき動いている処理だけが示される。
//
// 複数が同時に動くときは、最後に始まったものを示す。それが終わったら、動いている
// 残りのうち最後に始まったものに戻す。示すものが変わるたびに
// domain.ScanActivityChanged を発行する。
type activities struct {
	mu      sync.Mutex
	seq     uint64
	running map[string]runningActivity
	publish func()
}

type runningActivity struct {
	seq      uint64
	activity domain.ScanActivity
}

func newActivities(publish func()) *activities {
	return &activities{running: map[string]runningActivity{}, publish: publish}
}

// begin は key の処理が始まったことを記録する。同じ key が動いていれば置き換え、
// 新しく始まったものとして扱う。
func (a *activities) begin(key string, activity domain.ScanActivity) {
	a.mu.Lock()
	a.seq++
	a.running[key] = runningActivity{seq: a.seq, activity: activity}
	a.mu.Unlock()
	// 始まったものは必ず最後に始まったものなので、示すものが変わる。
	a.changed()
}

// end は key の処理が終わったことを記録する。示していたものが終わったときだけ
// 発行する。
func (a *activities) end(key string) {
	a.mu.Lock()
	ended, ok := a.running[key]
	top := a.topLocked()
	delete(a.running, key)
	a.mu.Unlock()
	if ok && ended.seq == top.seq {
		a.changed()
	}
}

// current は今示す処理を返す。何も動いていなければ Kind が空である。
func (a *activities) current() domain.ScanActivity {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.topLocked().activity
}

func (a *activities) topLocked() runningActivity {
	var top runningActivity
	for _, running := range a.running {
		if running.seq > top.seq {
			top = running
		}
	}
	return top
}

func (a *activities) changed() {
	if a.publish != nil {
		a.publish()
	}
}

func jobActivityKey(job domain.Job) string {
	return "job:" + strconv.FormatInt(job.ID, 10)
}
