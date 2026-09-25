package app

import (
	"container/list"
	"net/netip"
	"sync"
	"time"
)

// ログインの試行制限の定数（specs/016-single-account-auth/plan.md Structural Decisions 6）。
const (
	// loginFailureWindow は失敗を数える期間である。
	loginFailureWindow = 5 * time.Minute
	// loginFailureLimit は、期間内の「照合中の予約 + 失敗」がこの数に達すると制限する数である。
	loginFailureLimit = 5
	// defaultMaxThrottleSources は覚える送信元の数の既定の上限である。
	defaultMaxThrottleSources = 10000
	// ipv6SourcePrefix は IPv6 で1つの送信元とみなす接頭辞の長さである。
	ipv6SourcePrefix = 64
)

// loginThrottle は送信元ごとのログインの試行をメモリ上で数える。
//
// 照合の前に reserve で1回分を予約し、照合の結果で fail（失敗の記録に変える）・
// succeed（予約と記録を消す）・cancel（数えずに予約を外す）のどれかを呼ぶ。
// 予約も数えるので、同時に送られた要求でも照合は期間内に loginFailureLimit 回を超えない。
//
// 送信元の数には上限を置き、超えたら最も長く使われていない送信元から捨てる
// （照合中の予約がある送信元は捨てない）。
type loginThrottle struct {
	mu         sync.Mutex
	maxSources int
	sources    map[string]*list.Element
	// order は送信元を最近使った順に並べる。先頭が最も新しい。
	order *list.List
}

type throttleSource struct {
	key     string
	pending int
	// failures は期間内の失敗の時刻で、古い順に並ぶ。loginFailureLimit 件を超えたら
	// 古いものから捨てる。
	failures []time.Time
}

func newLoginThrottle(maxSources int) *loginThrottle {
	if maxSources <= 0 {
		maxSources = defaultMaxThrottleSources
	}
	return &loginThrottle{maxSources: maxSources, sources: map[string]*list.Element{}, order: list.New()}
}

// throttleKey は送信元のアドレスを試行制限の単位にする。IPv4（IPv4 射影の IPv6 を
// 含む）はアドレスごと、IPv6 は /64 ごとである。解釈できないアドレスはまとめて1つにする。
func throttleKey(addr netip.Addr) string {
	addr = addr.Unmap()
	switch {
	case addr.Is4():
		return addr.String()
	case addr.Is6():
		prefix, err := addr.WithZone("").Prefix(ipv6SourcePrefix)
		if err != nil {
			return "invalid"
		}
		return prefix.String()
	default:
		return "invalid"
	}
}

// reserve は key の1回分を予約する。予約と期間内の失敗が既に上限なら予約せず、
// 再び試せるまでの目安と false を返す。
func (t *loginThrottle) reserve(key string, now time.Time) (time.Duration, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	source := t.touch(key)
	source.prune(now)
	if source.pending+len(source.failures) >= loginFailureLimit {
		if len(source.failures) > 0 {
			return source.failures[0].Add(loginFailureWindow).Sub(now), false
		}
		// 照合中の予約だけで埋まっている。結果はすぐに出るので短い目安にする。
		return time.Second, false
	}
	source.pending++
	return 0, true
}

// fail は key の予約を1つ失敗の記録に変える。
func (t *loginThrottle) fail(key string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	source := t.touch(key)
	source.release()
	source.prune(now)
	source.failures = append(source.failures, now)
	if over := len(source.failures) - loginFailureLimit; over > 0 {
		source.failures = append([]time.Time(nil), source.failures[over:]...)
	}
}

// succeed は key の予約と失敗の記録を消す。照合中の他の予約は残す。
func (t *loginThrottle) succeed(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	element, ok := t.sources[key]
	if !ok {
		return
	}
	source := element.Value.(*throttleSource)
	source.release()
	source.failures = nil
	if source.pending == 0 {
		t.order.Remove(element)
		delete(t.sources, key)
	}
}

// cancel は key の予約を数えずに外す。
func (t *loginThrottle) cancel(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	element, ok := t.sources[key]
	if !ok {
		return
	}
	source := element.Value.(*throttleSource)
	source.release()
	if source.pending == 0 && len(source.failures) == 0 {
		t.order.Remove(element)
		delete(t.sources, key)
	}
}

// touch は key の記録を最近使ったものにして返す。無ければ作り、上限を超えたら
// 古いものから捨てる。t.mu を持って呼ぶ。
func (t *loginThrottle) touch(key string) *throttleSource {
	if element, ok := t.sources[key]; ok {
		t.order.MoveToFront(element)
		return element.Value.(*throttleSource)
	}
	source := &throttleSource{key: key}
	t.sources[key] = t.order.PushFront(source)
	for element := t.order.Back(); len(t.sources) > t.maxSources && element != nil; {
		prev := element.Prev()
		old := element.Value.(*throttleSource)
		if old.pending == 0 && old != source {
			t.order.Remove(element)
			delete(t.sources, old.key)
		}
		element = prev
	}
	return source
}

func (s *throttleSource) release() {
	if s.pending > 0 {
		s.pending--
	}
}

// prune は期間を過ぎた失敗を捨てる。
func (s *throttleSource) prune(now time.Time) {
	cutoff := now.Add(-loginFailureWindow)
	keep := 0
	for keep < len(s.failures) && !s.failures[keep].After(cutoff) {
		keep++
	}
	if keep > 0 {
		s.failures = append([]time.Time(nil), s.failures[keep:]...)
	}
}
